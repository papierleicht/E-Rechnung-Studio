package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"papierleicht.de/e-rechnung-studio/internal/invoicexml"
	"papierleicht.de/e-rechnung-studio/internal/model"
	"papierleicht.de/e-rechnung-studio/internal/pdfinvoice"
	"papierleicht.de/e-rechnung-studio/internal/profile"
)

func TestAdvancedInvoiceFormats(t *testing.T) {
	s, err := NewInvoiceService(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	c := testCompany()
	c.Phone, c.Endpoint, c.EndpointScheme = "+49 30 1234567", c.Email, "EM"
	c.TaxNumber = "12/345/67890"
	c.AdditionalAccounts = []model.BankAccount{{IBAN: "DE02120300000000202051", BIC: "BYLADEM1001", Name: "Zweites Konto"}}
	if _, err := s.SaveCompany(c); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"facturx-en16931", "facturx-basic", "facturx-extended", "xrechnung-cii", "xrechnung-ubl"} {
		for _, scenario := range []string{"adjustments", "zero-rate", "mixed", "margin-goods", "margin-art", "margin-collectibles", "out-of-scope", "cash", "agreement", "corrected-invoice"} {
			t.Run(id+"/"+scenario, func(t *testing.T) {
				f, _ := profile.Get(id)
				d := testDraft()
				d.Profile, d.Syntax = id, f.Syntax
				d.BuyerReference = "04011000-12345-03"
				d.Customer.Endpoint, d.Customer.EndpointScheme = d.Customer.Email, "EM"
				switch scenario {
				case "zero-rate":
					d.TaxTreatment = "mixed"
					d.Lines = append(d.Lines,
						model.InvoiceLine{ID: "2", Name: "Ermäßigter Steuersatz", Quantity: "1", Unit: "C62", UnitPrice: "100", TaxRate: "7", TaxCategory: "S"},
						model.InvoiceLine{ID: "3", Name: "Nullsteuersatz", Quantity: "1", Unit: "C62", UnitPrice: "100", TaxRate: "0,00", TaxCategory: "S"})
					d.Adjustments = []model.InvoiceAdjustment{{ID: "a", Amount: "10", Reason: "Nachlass Nullsteuersatz", TaxCategory: "Z", TaxRate: "0"}}
				case "adjustments":
					d.Adjustments = []model.InvoiceAdjustment{{ID: "a", Amount: "20", Reason: "Treuerabatt", TaxCategory: "S", TaxRate: "19"}, {ID: "b", Charge: true, Amount: "5", Reason: "Versand", TaxCategory: "S", TaxRate: "19"}}
				case "mixed":
					d.TaxTreatment = "mixed"
					d.Lines = append(d.Lines, model.InvoiceLine{ID: "2", Name: "Gebrauchtware", Quantity: "1", Unit: "C62", UnitPrice: "50", TaxTreatment: "margin-goods"}, model.InvoiceLine{ID: "3", Name: "Kunst", Quantity: "1", Unit: "C62", UnitPrice: "70", TaxTreatment: "margin-art"})
					d.Adjustments = []model.InvoiceAdjustment{{ID: "a", Amount: "10", Reason: "Nachlass", TaxCategory: "E", TaxRate: "0"}}
				case "cash":
					d.PaymentMeansCode = "10"
				case "agreement":
					d.PaymentMeansCode = "97"
				case "corrected-invoice":
					d.Kind, d.PrecedingInvoiceNumber, d.PrecedingInvoiceDate = "corrected-invoice", "OLD-42", "2026-09-01"
					d.Lines[0].Quantity = "-1"
				default:
					d.TaxTreatment = scenario
					if scenario == "margin-goods" {
						d.Lines[0].Name = "Gebrauchtware"
						d.Lines[0].Quantity, d.Lines[0].Unit = "1", "C62"
					}
				}
				v, err := s.ValidateInvoice(d)
				if err != nil || v.Validation.State != "valid" {
					t.Fatalf("%v: %+v", err, v.Validation)
				}
				parsed, err := invoicexml.Parse([]byte(v.XML))
				if err != nil {
					t.Fatal(err)
				}
				if scenario == "adjustments" && (v.Totals.Net != "185.00" || v.Totals.Tax != "35.15" || v.Totals.Gross != "220.15" || len(parsed.SpecifiedTradeAllowanceCharge) != 2) {
					t.Fatalf("wrong adjusted totals: %+v", v.Totals)
				}
				if scenario == "mixed" && (v.Totals.Net != "310.00" || v.Totals.Tax != "38.00" || len(parsed.TradeTaxes) != 2) {
					t.Fatalf("wrong mixed totals: %+v", v.Totals)
				}
				if scenario == "zero-rate" && (v.Totals.Net != "390.00" || v.Totals.Tax != "45.00" || len(parsed.TradeTaxes) != 3 || parsed.InvoiceLines[2].TaxCategoryCode != "Z") {
					t.Fatalf("wrong zero-rate mapping/totals: %+v, %+v", v.Totals, parsed.InvoiceLines)
				}
				if strings.HasPrefix(scenario, "margin-") && !strings.Contains(parsed.InvoiceLines[0].Note, "/Sonderregelung") {
					t.Fatalf("statutory margin notice missing on position: %+v", parsed.InvoiceLines[0])
				}
				if scenario != "cash" && scenario != "agreement" && len(parsed.PaymentMeans) != 2 {
					t.Fatalf("additional account lost: %+v", parsed.PaymentMeans)
				}
				if scenario == "corrected-invoice" && (parsed.InvoiceTypeCode != 384 || v.Totals.Gross != "-119.00" || len(parsed.InvoiceReferencedDocument) != 1) {
					t.Fatalf("correction lost: %+v", parsed)
				}
				writeFormatFixture(t, id+"-advanced-"+scenario, []byte(v.XML), v.Validation)
				if root := os.Getenv("E_RECHNUNG_STUDIO_FORMAT_TEST_OUTPUT"); root != "" && (scenario == "mixed" || scenario == "adjustments" || scenario == "zero-rate" || scenario == "margin-goods") {
					pdf, err := pdfinvoice.RenderWithSnapshot([]byte(v.XML), &c, &d.Customer)
					if err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(filepath.Join(root, id+"-advanced-"+scenario+".pdf"), pdf, 0600); err != nil {
						t.Fatal(err)
					}
				}
			})
		}
	}
}

func TestAdvancedInputsRejectInvalidRelationships(t *testing.T) {
	s, err := NewInvoiceService(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err := s.SaveCompany(testCompany()); err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []string{"excess-discount", "missing-reason", "wrong-tax-group", "mixed-outside", "negative-ordinary", "missing-reference", "unsupported-payment"} {
		t.Run(scenario, func(t *testing.T) {
			d := testDraft()
			switch scenario {
			case "excess-discount":
				d.Adjustments = []model.InvoiceAdjustment{{Amount: "201", Reason: "Rabatt", TaxCategory: "S", TaxRate: "19"}}
			case "missing-reason":
				d.Adjustments = []model.InvoiceAdjustment{{Amount: "10", TaxCategory: "S", TaxRate: "19"}}
			case "wrong-tax-group":
				d.Adjustments = []model.InvoiceAdjustment{{Amount: "10", Reason: "Rabatt", TaxCategory: "S", TaxRate: "7"}}
			case "mixed-outside":
				d.TaxTreatment = "mixed"
				extra := d.Lines[0]
				extra.TaxTreatment = "out-of-scope"
				d.Lines = append(d.Lines, extra)
			case "negative-ordinary":
				d.Lines[0].Quantity = "-1"
			case "missing-reference":
				d.Kind = "corrected-invoice"
			case "unsupported-payment":
				d.PaymentMeansCode = "bogus"
			}
			v, err := s.ValidateInvoice(d)
			if err != nil || v.Validation.State == "valid" {
				t.Fatalf("invalid input accepted: %v %+v", err, v.Validation)
			}
		})
	}
}

func TestAdvancedDefaultsAndImmutableExports(t *testing.T) {
	s, err := NewInvoiceService(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	c := testCompany()
	c.DefaultTaxTreatment = "margin-goods"
	c.AdditionalAccounts = []model.BankAccount{{IBAN: "DE02120300000000202051", Name: "Zweites Konto"}}
	if _, err := s.SaveCompany(c); err != nil {
		t.Fatal(err)
	}
	b, err := s.Bootstrap()
	if err != nil || b.Draft.TaxTreatment != "margin-goods" {
		t.Fatalf("merchant default missing: %v %+v", err, b.Draft)
	}
	customer, err := s.SaveCustomer(testDraft().Customer)
	if err != nil {
		t.Fatal(err)
	}
	d := testDraft()
	d.Customer, d.CustomerID, d.PaymentMeansCode = customer, customer.ID, "30"
	d.Adjustments = []model.InvoiceAdjustment{{ID: "discount", Amount: "20", Reason: "Treuerabatt", TaxCategory: "S", TaxRate: "19"}}
	saved, err := s.SaveInvoice(d)
	if err != nil {
		t.Fatal(err)
	}
	b, err = s.Bootstrap()
	if err != nil || len(b.Customers) != 1 || b.Customers[0].InvoiceSettings.PaymentMeansCode != "30" {
		t.Fatalf("customer defaults missing: %v %+v", err, b.Customers)
	}
	opened, err := s.OpenInvoice(saved.Draft.ID)
	if err != nil || len(opened.Draft.Adjustments) != 1 || opened.Totals.Gross != "214.20" {
		t.Fatalf("discount lost after save: %v %+v", err, opened.Totals)
	}
	issued, err := s.IssueInvoice(opened.Draft)
	if err != nil {
		t.Fatal(err)
	}
	originalPDF, err := s.invoicePDF(issued.Draft.ID)
	if err != nil {
		t.Fatal(err)
	}
	correction, err := s.CreateCorrectionDraft(issued.Draft.ID)
	if err != nil || len(correction.Draft.Adjustments) != 1 || correction.Totals.Gross != "214.20" {
		t.Fatalf("correction lost discount: %v %+v", err, correction.Totals)
	}
	c.AdditionalAccounts[0].Name = "Geändertes Konto"
	if _, err := s.SaveCompany(c); err != nil {
		t.Fatal(err)
	}
	afterPDF, err := s.invoicePDF(issued.Draft.ID)
	if err != nil || string(afterPDF) != string(originalPDF) {
		t.Fatal("issued PDF changed with bank settings")
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
	r, err := restored.OpenInvoice(issued.Draft.ID)
	if err != nil || r.XML != issued.XML || len(r.Draft.Adjustments) != 1 {
		t.Fatal("backup changed issued invoice")
	}
}
