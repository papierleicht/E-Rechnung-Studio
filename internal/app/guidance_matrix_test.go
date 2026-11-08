package app

import (
	"fmt"
	"papierleicht.de/e-rechnung-studio/internal/model"
	"papierleicht.de/e-rechnung-studio/internal/profile"
	"testing"
)

func TestGuidanceCombinationMatrix(t *testing.T) {
	s, err := NewInvoiceService(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for _, id := range []string{"facturx-basic", "facturx-en16931", "facturx-extended", "xrechnung-cii", "xrechnung-ubl"} {
		for _, kind := range []string{"invoice", "credit-note"} {
			for _, tax := range []string{"standard", "exempt", "reverse-charge", "intra-community"} {
				for mask := 0; mask < 4; mask++ {
					t.Run(fmt.Sprintf("%s/%s/%s/identifiers-%d", id, kind, tax, mask), func(t *testing.T) {
						c, d := testCompany(), testDraft()
						c.Phone, c.Endpoint, c.EndpointScheme = "+49 30 1234567", c.Email, "EM"
						c.TaxNumber = ""
						if mask&1 == 0 {
							c.VATID = ""
						}
						if mask&2 == 0 {
							d.Customer.VATID = ""
						}
						f, _ := profile.Get(id)
						d.Profile, d.Syntax, d.Kind, d.TaxTreatment = id, f.Syntax, kind, tax
						d.TaxExemptionReason = "Steuerfreie Heilbehandlung nach § 4 Nr. 14 UStG"
						d.BuyerReference = "04011000-12345-03"
						d.Customer.Endpoint, d.Customer.EndpointScheme = d.Customer.Email, "EM"
						if err := s.store.SaveCompany(c); err != nil {
							t.Fatal(err)
						}
						v, err := s.ValidateInvoice(d)
						if err != nil {
							t.Fatal(err)
						}
						needsBuyer := tax == "reverse-charge" || tax == "intra-community"
						valid := mask&1 != 0 && (!needsBuyer || mask&2 != 0)
						if valid {
							if v.Validation.State != "valid" {
								t.Fatalf("expected valid: %+v", v.Validation)
							}
							return
						}
						rule := map[string]string{"standard": "BR-S-02", "exempt": "BR-E-02", "reverse-charge": "BR-AE-02", "intra-community": "BR-IC-02"}[tax]
						field := "company.taxNumber"
						if tax == "intra-community" {
							field = "company.vatId"
						}
						if needsBuyer && mask&1 != 0 {
							field = "customer.vatId"
						}
						found := false
						for _, issue := range v.Validation.Issues {
							if issue.RuleID == rule {
								found = true
								if issue.Field != field {
									t.Errorf("wrong action for %s: got %s, want %s: %s", rule, issue.Field, field, issue.Message)
								}
							}
						}
						if !found || v.Validation.State == "valid" {
							t.Fatalf("expected %s: %+v", rule, v.Validation)
						}
						c.VATID = testCompany().VATID
						d.Customer.VATID = testDraft().Customer.VATID
						if err := s.store.SaveCompany(c); err != nil {
							t.Fatal(err)
						}
						fixed, err := s.ValidateInvoice(d)
						if err != nil || fixed.Validation.State != "valid" {
							t.Fatalf("correction did not recover: %v %+v", err, fixed.Validation)
						}
					})
				}
			}
		}
	}
}

func TestGuidanceAllowanceChargeMatrix(t *testing.T) {
	s, err := NewInvoiceService(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for _, id := range []string{"facturx-en16931", "xrechnung-ubl"} {
		for _, tax := range []string{"reverse-charge", "intra-community"} {
			for mask := 0; mask < 3; mask++ {
				for _, charge := range []bool{false, true} {
					t.Run(fmt.Sprintf("%s/%s/ids-%d/charge-%t", id, tax, mask, charge), func(t *testing.T) {
						c, d := testCompany(), testDraft()
						c.Phone, c.Endpoint, c.EndpointScheme = "+49 30 1234567", c.Email, "EM"
						c.TaxNumber = ""
						if mask&1 == 0 {
							c.VATID = ""
						}
						if mask&2 == 0 {
							d.Customer.VATID = ""
						}
						f, _ := profile.Get(id)
						d.Profile, d.Syntax, d.TaxTreatment = id, f.Syntax, tax
						d.BuyerReference = "04011000-12345-03"
						d.Customer.Endpoint, d.Customer.EndpointScheme = d.Customer.Email, "EM"
						category, prefix, field := "AE", "BR-AE-", "company.taxNumber"
						if tax == "intra-community" {
							category, prefix, field = "K", "BR-IC-", "company.vatId"
						}
						if mask&1 != 0 {
							field = "customer.vatId"
						}
						d.Adjustments = []model.InvoiceAdjustment{{ID: "test", Charge: charge, Amount: "1", Reason: "Test", TaxCategory: category, TaxRate: "0"}}
						if err := s.store.SaveCompany(c); err != nil {
							t.Fatal(err)
						}
						v, err := s.ValidateInvoice(d)
						if err != nil {
							t.Fatal(err)
						}
						rule := prefix + "03"
						if charge {
							rule = prefix + "04"
						}
						found := false
						for _, issue := range v.Validation.Issues {
							if issue.RuleID == rule {
								found = true
								if issue.Field != field {
									t.Errorf("wrong action: %+v", issue)
								}
							}
						}
						if !found {
							t.Fatalf("missing %s: %+v", rule, v.Validation)
						}
					})
				}
			}
		}
	}
}

func TestGuidanceActualXMLLocations(t *testing.T) {
	s, err := NewInvoiceService(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for _, id := range []string{"facturx-en16931", "xrechnung-ubl"} {
		for _, scenario := range []string{"customer-vat", "seller-vat", "invoice-line", "credit-note-line"} {
			t.Run(id+"/"+scenario, func(t *testing.T) {
				c, d := testCompany(), testDraft()
				c.Phone, c.Endpoint, c.EndpointScheme = "+49 30 1234567", c.Email, "EM"
				f, _ := profile.Get(id)
				d.Profile, d.Syntax = id, f.Syntax
				d.BuyerReference = "04011000-12345-03"
				d.Customer.Endpoint, d.Customer.EndpointScheme = d.Customer.Email, "EM"
				rule, field := "BR-CO-09", "customer.vatId"
				switch scenario {
				case "customer-vat":
					d.Customer.VATID = "ZZ123456789"
				case "seller-vat":
					c.VATID = "ZZ123456789"
					field = "company.vatId"
				default:
					rule, field = "BR-25", "lines.1.name"
					next := d.Lines[0]
					next.ID = "2"
					next.Name = ""
					d.Lines = append(d.Lines, next)
					if scenario == "credit-note-line" {
						d.Kind = "credit-note"
					}
				}
				if err := s.store.SaveCompany(c); err != nil {
					t.Fatal(err)
				}
				v, err := s.ValidateInvoice(d)
				if err != nil {
					t.Fatal(err)
				}
				found := false
				for _, issue := range v.Validation.Issues {
					if issue.RuleID == rule {
						found = true
						if issue.Field != field {
							t.Errorf("wrong action from actual XML: %+v", issue)
						}
					}
				}
				if !found {
					t.Fatalf("expected %s: %+v", rule, v.Validation)
				}
			})
		}
	}
}
