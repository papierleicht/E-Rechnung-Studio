package app

import (
	"bytes"
	"encoding/json"
	"github.com/beevik/etree"
	"os"
	"papierleicht.de/e-rechnung-studio/internal/einvoice"
	"papierleicht.de/e-rechnung-studio/internal/invoicexml"
	"papierleicht.de/e-rechnung-studio/internal/model"
	"papierleicht.de/e-rechnung-studio/internal/profile"
	"path/filepath"
	"strings"
	"testing"
)

func TestSupportedFormats(t *testing.T) {
	s, err := NewInvoiceService(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	c := testCompany()
	c.Phone = "+49 30 1234567"
	c.Endpoint = c.Email
	c.EndpointScheme = "EM"
	if _, err := s.SaveCompany(c); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"facturx-en16931", "facturx-basic", "facturx-extended", "xrechnung-cii", "xrechnung-ubl"} {
		for _, kind := range []string{"invoice", "credit-note"} {
			for _, tax := range []string{"standard", "small-business", "zero-rated", "reverse-charge", "intra-community", "export"} {
				t.Run(id+"/"+kind+"/"+tax, func(t *testing.T) {
					f, _ := profile.Get(id)
					d := testDraft()
					d.Profile, d.Syntax, d.Kind = id, f.Syntax, kind
					d.TaxTreatment = tax
					company := c
					if tax == "small-business" {
						company.VATID = ""
						company.TaxNumber = "12/345/67890"
					}
					if _, err := s.SaveCompany(company); err != nil {
						t.Fatal(err)
					}
					d.BuyerReference = "04011000-12345-03"
					d.Customer.Endpoint, d.Customer.EndpointScheme = d.Customer.Email, "EM"
					v, err := s.ValidateInvoice(d)
					if err != nil || v.Validation.State != "valid" {
						t.Fatalf("%v: %+v", err, v.Validation)
					}
					if !strings.Contains(v.XML, f.Specification) {
						t.Fatal("wrong specification")
					}
					inspection, err := s.InspectXML(v.XML)
					if err != nil || inspection.Validation.State != "valid" {
						t.Fatalf("inspection: %v %+v", err, inspection)
					}
					parsed, err := invoicexml.Parse([]byte(v.XML))
					if err != nil || len(parsed.SpecifiedTradePaymentTerms) == 0 || parsed.SpecifiedTradePaymentTerms[0].DueDate.Format("2006-01-02") != d.DueDate {
						t.Fatalf("due date lost: %v %+v", err, parsed)
					}
					if tax == "zero-rated" {
						assertZeroRatedInvoice(t, parsed)
					}
					if tax == "intra-community" && f.XRechnung {
						if parsed.ShipTo == nil || parsed.ShipTo.PostalAddress == nil || parsed.ShipTo.PostalAddress.CountryID != "FR" || parsed.ShipTo.PostalAddress.City != "Paris" {
							t.Fatal("delivery location lost in XML round-trip")
						}
					}
					fixture := id + "-" + kind + "-" + tax
					writeFormatFixture(t, fixture, []byte(v.XML), v.Validation)
					if tax != "standard" {
						return
					}
					// Verify issuing, automatic PDF, immutable export and reopening for
					// every profile, including the different UBL CreditNote schema.
					d.Number = ""
					issued, err := s.IssueInvoice(d)
					if err != nil {
						t.Fatal(err)
					}
					pdf, err := s.invoicePDF(issued.Draft.ID)
					if err != nil || !bytes.HasPrefix(pdf, []byte("%PDF-")) {
						t.Fatalf("automatic PDF: %v", err)
					}
					if path := os.Getenv("E_RECHNUNG_STUDIO_FORMAT_TEST_OUTPUT"); path != "" {
						if err := os.WriteFile(filepath.Join(path, fixture+".pdf"), pdf, 0600); err != nil {
							t.Fatal(err)
						}
						if err := os.WriteFile(filepath.Join(path, fixture+"-embedded.xml"), []byte(issued.XML), 0600); err != nil {
							t.Fatal(err)
						}
					}
					path := filepath.Join(t.TempDir(), "invoice.xml")
					if _, err := s.ExportInvoiceXML(issued.Draft.ID, path); err != nil {
						t.Fatal(err)
					}
					exported, err := os.ReadFile(path)
					if err != nil || string(exported) != issued.XML {
						t.Fatal("XML export changed")
					}
					backup, err := s.ExportBackup()
					if err != nil {
						t.Fatal(err)
					}
					restored, err := NewInvoiceService(t.TempDir())
					if err != nil {
						t.Fatal(err)
					}
					defer restored.Close()
					if err := restored.RestoreBackup(backup); err != nil {
						t.Fatal(err)
					}
					opened, err := restored.OpenInvoice(issued.Draft.ID)
					if err != nil || opened.XML != issued.XML || opened.Draft.Profile != id || opened.Company.Name != c.Name {
						t.Fatalf("restored invoice changed: %v", err)
					}
					cached, err := restored.invoicePDF(issued.Draft.ID)
					if err != nil || !bytes.Equal(pdf, cached) {
						t.Fatal("restored PDF changed")
					}
				})
			}
		}
	}
}

func assertZeroRatedInvoice(t *testing.T, invoice *einvoice.Invoice) {
	t.Helper()
	if len(invoice.InvoiceLines) == 0 {
		t.Fatal("zero-rated invoice has no invoice line")
	}
	for i, line := range invoice.InvoiceLines {
		if line.TaxCategoryCode != "Z" || !line.TaxRateApplicablePercent.IsZero() {
			t.Fatalf("zero-rated line %d has category %q and rate %s", i+1, line.TaxCategoryCode, line.TaxRateApplicablePercent)
		}
	}
	found := false
	for _, tax := range invoice.TradeTaxes {
		if tax.CategoryCode != "Z" {
			continue
		}
		found = true
		if !tax.Percent.IsZero() || !tax.CalculatedAmount.IsZero() {
			t.Fatalf("zero-rated VAT breakdown has rate %s and tax amount %s", tax.Percent, tax.CalculatedAmount)
		}
		if tax.ExemptionReason != "" || tax.ExemptionReasonCode != "" {
			t.Fatalf("zero-rated VAT breakdown contains exemption reason %q or code %q", tax.ExemptionReason, tax.ExemptionReasonCode)
		}
	}
	if !found {
		t.Fatal("zero-rated invoice has no category Z VAT breakdown")
	}
}

func writeFormatFixture(t *testing.T, name string, xml []byte, result model.ValidationResult) {
	t.Helper()
	root := os.Getenv("E_RECHNUNG_STUDIO_FORMAT_TEST_OUTPUT")
	if root == "" {
		return
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, name+".xml"), xml, 0600); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, name+".json"), data, 0600); err != nil {
		t.Fatal(err)
	}
}

func TestProfileSpecificFailures(t *testing.T) {
	s, err := NewInvoiceService(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	c := testCompany()
	c.Phone = "+49 30 1234567"
	c.Endpoint, c.EndpointScheme = c.Email, "EM"
	if _, err := s.SaveCompany(c); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"xrechnung-cii", "xrechnung-ubl"} {
		f, _ := profile.Get(id)
		d := testDraft()
		d.Profile, d.Syntax = id, f.Syntax
		d.Customer.Endpoint, d.Customer.EndpointScheme = d.Customer.Email, "EM"
		d.BuyerReference = "04011000-12345-03"
		v, err := s.ValidateInvoice(d)
		if err != nil || v.Validation.State != "valid" {
			t.Fatalf("baseline: %v %+v", err, v.Validation)
		}
		mutations := []struct{ name, cii, ubl, rule string }{
			{"missing-reference", "//ram:BuyerReference", "//cbc:BuyerReference", "BR-DE-15"},
			{"missing-phone", "//ram:TelephoneUniversalCommunication", "//cac:Contact/cbc:Telephone", "BR-DE-6"},
			{"missing-process", "//ram:BusinessProcessSpecifiedDocumentContextParameter", "//cbc:ProfileID", "PEPPOL-EN16931-R001"},
			{"missing-buyer-address", "//ram:BuyerTradeParty/ram:URIUniversalCommunication", "//cac:AccountingCustomerParty/cac:Party/cbc:EndpointID", "PEPPOL-EN16931-R010"},
			{"missing-seller-address", "//ram:SellerTradeParty/ram:URIUniversalCommunication", "//cac:AccountingSupplierParty/cac:Party/cbc:EndpointID", "PEPPOL-EN16931-R020"},
			{"wrong-total", "//ram:GrandTotalAmount", "//cbc:TaxInclusiveAmount", "BR-CO-15"},
		}
		for _, m := range mutations {
			t.Run(id+"/"+m.name, func(t *testing.T) {
				x := etree.NewDocument()
				if err := x.ReadFromString(v.XML); err != nil {
					t.Fatal(err)
				}
				query := m.cii
				if f.Syntax == "ubl" {
					query = m.ubl
				}
				node := x.FindElement(query)
				if node == nil {
					t.Fatal("missing mutation target", query)
				}
				if m.name == "wrong-total" {
					node.SetText("999.99")
				} else {
					node.Parent().RemoveChild(node)
				}
				raw, _ := x.WriteToBytes()
				result := s.validator.Validate(raw)
				found := false
				for _, issue := range result.Issues {
					if issue.RuleID == m.rule {
						found = true
					}
				}
				if result.State != "invalid" || !found {
					t.Fatalf("expected %s: %+v", m.rule, result)
				}
				writeFormatFixture(t, id+"-"+m.name, raw, result)
			})
		}
		d.BuyerReference = ""
		if _, err := s.IssueInvoice(d); err == nil {
			t.Fatal("XRechnung issued without mandatory reference")
		}
		d.Profile, d.Syntax = "facturx-en16931", "cii"
		if v, err := s.ValidateInvoice(d); err != nil || v.Validation.State != "valid" {
			t.Fatalf("switch back from XRechnung: %v %+v", err, v.Validation)
		}
		d.Syntax = "ubl"
		if v, _ := s.ValidateInvoice(d); v.Validation.State == "valid" {
			t.Fatal("mismatched syntax accepted")
		}
	}
}

func TestZeroRatedOfficialRules(t *testing.T) {
	s, err := NewInvoiceService(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	c := testCompany()
	c.Phone = "+49 30 1234567"
	c.Endpoint, c.EndpointScheme = c.Email, "EM"
	if _, err := s.SaveCompany(c); err != nil {
		t.Fatal(err)
	}

	for _, id := range []string{"xrechnung-cii", "xrechnung-ubl"} {
		t.Run(id, func(t *testing.T) {
			f, _ := profile.Get(id)
			d := testDraft()
			d.Profile, d.Syntax, d.TaxTreatment = id, f.Syntax, "zero-rated"
			d.Customer.Endpoint, d.Customer.EndpointScheme = d.Customer.Email, "EM"
			d.BuyerReference = "04011000-12345-03"
			baseline, err := s.ValidateInvoice(d)
			if err != nil || baseline.Validation.State != "valid" {
				t.Fatalf("zero-rated baseline: %v %+v", err, baseline.Validation)
			}

			t.Run("non-zero-rate", func(t *testing.T) {
				x := etree.NewDocument()
				if err := x.ReadFromString(baseline.XML); err != nil {
					t.Fatal(err)
				}
				query := "//ram:SpecifiedLineTradeSettlement/ram:ApplicableTradeTax/ram:RateApplicablePercent"
				if f.Syntax == "ubl" {
					query = "//cac:ClassifiedTaxCategory/cbc:Percent"
				}
				node := x.FindElement(query)
				if node == nil {
					t.Fatal("missing zero-rated line rate", query)
				}
				node.SetText("19")
				raw, _ := x.WriteToBytes()
				assertValidationRule(t, s.validator.Validate(raw), "BR-Z-05")
			})

			t.Run("missing-seller-tax-id", func(t *testing.T) {
				x := etree.NewDocument()
				if err := x.ReadFromString(baseline.XML); err != nil {
					t.Fatal(err)
				}
				query := "//ram:SellerTradeParty/ram:SpecifiedTaxRegistration"
				if f.Syntax == "ubl" {
					query = "//cac:AccountingSupplierParty/cac:Party/cac:PartyTaxScheme"
				}
				node := x.FindElement(query)
				if node == nil {
					t.Fatal("missing seller tax identifier mutation target", query)
				}
				node.Parent().RemoveChild(node)
				raw, _ := x.WriteToBytes()
				assertValidationRule(t, s.validator.Validate(raw), "BR-Z-02")
			})

			t.Run("exemption-reason", func(t *testing.T) {
				x := etree.NewDocument()
				if err := x.ReadFromString(baseline.XML); err != nil {
					t.Fatal(err)
				}
				query, name := "//ram:ApplicableHeaderTradeSettlement/ram:ApplicableTradeTax/ram:BasisAmount", "ram:ExemptionReason"
				if f.Syntax == "ubl" {
					query, name = "//cac:TaxSubtotal/cac:TaxCategory/cac:TaxScheme", "cbc:TaxExemptionReason"
				}
				anchor := x.FindElement(query)
				if anchor == nil {
					t.Fatal("missing VAT breakdown mutation target", query)
				}
				reason := etree.NewElement(name)
				reason.SetText("must not be present for category Z")
				anchor.Parent().InsertChildAt(anchor.Index(), reason)
				raw, _ := x.WriteToBytes()
				assertValidationRule(t, s.validator.Validate(raw), "BR-Z-10")
			})
		})
	}
}

func assertValidationRule(t *testing.T, result model.ValidationResult, rule string) {
	t.Helper()
	for _, issue := range result.Issues {
		if issue.RuleID == rule {
			if result.State != "invalid" {
				t.Fatalf("%s was reported without invalid state: %+v", rule, result)
			}
			return
		}
	}
	t.Fatalf("expected %s: %+v", rule, result)
}

func TestSellerEndpointDefaultsToContactEmail(t *testing.T) {
	s, err := NewInvoiceService(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for _, tc := range []struct{ scheme, want string }{
		{"", `schemeID="EM">`},
		{"EM", `schemeID="EM">`},
		{"9930", ""},
	} {
		t.Run("scheme="+tc.scheme, func(t *testing.T) {
			c := testCompany()
			c.Endpoint, c.EndpointScheme = "  ", tc.scheme
			if _, err := s.SaveCompany(c); err != nil {
				t.Fatal(err)
			}
			d := testDraft()
			f, _ := profile.Get("facturx-en16931")
			d.Profile, d.Syntax = f.ID, f.Syntax
			v, err := s.ValidateInvoice(d)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(v.XML, "<ram:SellerTradeParty>") {
				t.Fatalf("no XML: %+v", v.Validation)
			}
			seller := v.XML[strings.Index(v.XML, "<ram:SellerTradeParty>"):strings.Index(v.XML, "</ram:SellerTradeParty>")]
			if tc.want != "" && !strings.Contains(seller, tc.want+c.Email+"</ram:URIID>") {
				t.Fatalf("contact e-mail not used as electronic address:\n%s", seller)
			}
			if tc.want == "" && strings.Contains(seller, "<ram:URIUniversalCommunication>") {
				t.Fatalf("empty electronic address written:\n%s", seller)
			}
			for _, issue := range v.Validation.Issues {
				if issue.RuleID == "PEPPOL-EN16931-R008" {
					t.Fatalf("empty element reported: %+v", issue)
				}
			}
		})
	}
}
