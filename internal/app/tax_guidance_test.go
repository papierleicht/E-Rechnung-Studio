package app

import (
	"testing"

	"papierleicht.de/e-rechnung-studio/internal/invoicexml"
	"papierleicht.de/e-rechnung-studio/internal/profile"
)

func TestIntraCommunityMissingVATGuidanceAndRecovery(t *testing.T) {
	s, err := NewInvoiceService(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for _, id := range []string{"facturx-en16931", "xrechnung-ubl"} {
		t.Run(id, func(t *testing.T) {
			c, d := testCompany(), testDraft()
			c.Phone, c.Endpoint, c.EndpointScheme = "+49 30 1234567", c.Email, "EM"
			vat := c.VATID
			c.VATID, c.TaxNumber = "", "12/345/67890"
			f, _ := profile.Get(id)
			d.Profile, d.Syntax, d.TaxTreatment = id, f.Syntax, "intra-community"
			d.BuyerReference = "04011000-12345-03"
			d.Customer.Endpoint, d.Customer.EndpointScheme = d.Customer.Email, "EM"
			if _, err := s.SaveCompany(c); err != nil {
				t.Fatal(err)
			}
			v, err := s.ValidateInvoice(d)
			if err != nil {
				t.Fatal(err)
			}
			parsed, err := invoicexml.Parse([]byte(v.XML))
			if err != nil {
				t.Fatal(err)
			}
			if d.Customer.VATID == "" || parsed.Buyer.VATaxRegistration != d.Customer.VATID {
				t.Fatal("customer VAT ID lost in XML")
			}
			found := false
			for _, issue := range v.Validation.Issues {
				if issue.RuleID == "BR-IC-02" {
					found = true
					if issue.Field != "company.vatId" {
						t.Fatalf("wrong destination: %+v", issue)
					}
				}
			}
			if !found {
				t.Fatalf("expected BR-IC-02: %+v", v.Validation)
			}
			c.VATID = vat
			if _, err := s.SaveCompany(c); err != nil {
				t.Fatal(err)
			}
			v, err = s.ValidateInvoice(d)
			if err != nil || v.Validation.State != "valid" {
				t.Fatalf("adding seller VAT must fix validation: %v %+v", err, v.Validation)
			}
		})
	}
}
