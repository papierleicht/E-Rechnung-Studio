package app

import (
	"papierleicht.de/e-rechnung-studio/internal/einvoice"
	"papierleicht.de/e-rechnung-studio/internal/model"
	"reflect"
	"strings"
	"testing"
)

func hasRule(v model.ValidationResult, rule string) bool {
	for _, i := range v.Issues {
		if i.RuleID == rule {
			return true
		}
	}
	return false
}

func TestAuditTaxAndPaymentCases(t *testing.T) {
	cases := []struct {
		name, rule string
		change     func(*model.InvoiceDraft, *model.Company)
	}{
		{"reverse-charge", "", func(d *model.InvoiceDraft, c *model.Company) { d.TaxTreatment = "reverse-charge" }},
		{"reverse-charge-buyer-missing", "BR-AE-02", func(d *model.InvoiceDraft, c *model.Company) {
			d.TaxTreatment = "reverse-charge"
			d.Customer.VATID = ""
		}},
		{"intra-community-tax-number-only", "BR-IC-02", func(d *model.InvoiceDraft, c *model.Company) {
			d.TaxTreatment = "intra-community"
			c.VATID = ""
			c.TaxNumber = "12/345/67890"
		}},
		{"intra-community", "", func(d *model.InvoiceDraft, c *model.Company) { d.TaxTreatment = "intra-community" }},
		{"export", "", func(d *model.InvoiceDraft, c *model.Company) {
			d.TaxTreatment = "export"
			d.Customer.Address.Country = "US"
			d.Customer.VATID = ""
		}},
		{"standard-zero-rate", "", func(d *model.InvoiceDraft, c *model.Company) { d.Lines[0].TaxRate = "0" }},
		{"no-payment-terms", "APP-PAYMENT-TERMS", func(d *model.InvoiceDraft, c *model.Company) { d.DueDate = ""; d.PaymentTerms = "" }},
		{"whitespace-payment-terms", "APP-PAYMENT-TERMS", func(d *model.InvoiceDraft, c *model.Company) { d.DueDate = ""; d.PaymentTerms = " \t\n" }},
		{"date-only", "", func(d *model.InvoiceDraft, c *model.Company) { d.PaymentTerms = "" }},
		{"terms-only", "", func(d *model.InvoiceDraft, c *model.Company) { d.DueDate = "" }},
		{"zero-amount-no-terms", "", func(d *model.InvoiceDraft, c *model.Company) {
			d.DueDate = ""
			d.PaymentTerms = ""
			d.Lines[0].UnitPrice = "0"
		}},
		{"credit-note-no-terms", "", func(d *model.InvoiceDraft, c *model.Company) {
			d.Kind = "credit-note"
			d.DueDate = ""
			d.PaymentTerms = ""
		}},
		{"bad-iban", "APP-IBAN", func(d *model.InvoiceDraft, c *model.Company) { c.IBAN = "not-an-iban" }},
		{"bad-iban-checksum", "APP-IBAN", func(d *model.InvoiceDraft, c *model.Company) { c.IBAN = "DE88370400440532013000" }},
		{"bad-seller-vat", "APP-VAT-FORMAT", func(d *model.InvoiceDraft, c *model.Company) { c.VATID = "DEabc" }},
		{"bad-buyer-vat", "APP-VAT-FORMAT", func(d *model.InvoiceDraft, c *model.Company) { d.Customer.VATID = "DEabc" }},
		{"bad-tax-number", "APP-TAX-FORMAT", func(d *model.InvoiceDraft, c *model.Company) { c.TaxNumber = "invalid" }},
		{"mixed-rates-rounding", "", func(d *model.InvoiceDraft, c *model.Company) {
			d.Lines = []model.InvoiceLine{{ID: "1", Name: "A", Quantity: "3", Unit: "C62", UnitPrice: "0.333", TaxRate: "19", TaxCategory: "S"}, {ID: "2", Name: "B", Quantity: "7", Unit: "C62", UnitPrice: "0.777", TaxRate: "7", TaxCategory: "S"}}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, err := NewInvoiceService(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			d, c := testDraft(), testCompany()
			tc.change(&d, &c)
			// Simulate older records / restored backups which bypass today's save checks.
			if err = s.store.SaveCompany(c); err != nil {
				t.Fatal(err)
			}
			v, err := s.ValidateInvoice(d)
			if err != nil {
				t.Fatal(err)
			}
			if tc.rule == "" {
				if v.Validation.State != "valid" {
					t.Fatalf("expected valid: %+v", v.Validation)
				}
			} else {
				if v.Validation.State == "valid" || !hasRule(v.Validation, tc.rule) {
					t.Fatalf("expected %s: %+v", tc.rule, v.Validation)
				}
				if _, err = s.IssueInvoice(d); err == nil {
					t.Fatal("invalid invoice issued")
				}
			}
			imported, err := s.InspectXML(v.XML)
			if err != nil {
				t.Fatal(err)
			}
			// The app's own input safeguards only warn for documents received from others.
			want := v.Validation.State
			if want == "invalid" {
				want = "warning"
				for _, issue := range v.Validation.Issues {
					if issue.Stage != "input" && (issue.Severity == "error" || issue.Severity == "fatal") {
						want = "invalid"
					}
				}
			}
			if imported.Validation.State != want || (tc.rule != "" && !hasRule(imported.Validation, tc.rule)) {
				t.Fatalf("import differs (want %s): %+v", want, imported.Validation)
			}
		})
	}
}

func TestCompanyAndCustomerSaveCheckBeforePersisting(t *testing.T) {
	s, err := NewInvoiceService(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	c := testCompany()
	c.VATID = "de 123 456 789"
	c.TaxNumber = " 12 / 345 / 67890 "
	c.IBAN = "de89 3704\u00a00044 0532 0130 00"
	c.BIC = " coba de ff xxx "
	saved, err := s.SaveCompany(c)
	if err != nil {
		t.Fatal(err)
	}
	if saved.VATID != "DE123456789" || saved.TaxNumber != "12/345/67890" || saved.IBAN != "DE89370400440532013000" || saved.BIC != "COBADEFFXXX" {
		t.Fatalf("normalization: %+v", saved)
	}
	for _, bad := range []model.Company{func() model.Company { x := saved; x.IBAN = "not-an-iban"; return x }(), func() model.Company { x := saved; x.VATID = "DEabc"; return x }(), func() model.Company { x := saved; x.TaxNumber = "123"; return x }()} {
		if _, err = s.SaveCompany(bad); err == nil {
			t.Fatal("bad company saved")
		}
	}
	current, err := s.store.Company()
	if err != nil || !reflect.DeepEqual(current, saved) {
		t.Fatalf("failed save changed company: %+v %v", current, err)
	}
	customer := testDraft().Customer
	customer.VATID = "DEabc"
	if _, err = s.SaveCustomer(customer); err == nil {
		t.Fatal("bad customer saved")
	}
	customer.VATID = "fr 40303265045"
	if saved, err := s.SaveCustomer(customer); err != nil || saved.VATID != "FR40303265045" {
		t.Fatalf("customer normalization: %+v %v", saved, err)
	}

	product := model.Product{Name: "  Beratung  ", Description: "  Analyse  ", Unit: " hur ", UnitPrice: "100,00", TaxRate: "19,0", TaxCategory: "s"}
	savedProduct, err := s.SaveProduct(product)
	if err != nil {
		t.Fatal(err)
	}
	if savedProduct.Name != "Beratung" || savedProduct.Description != "Analyse" || savedProduct.Unit != "HUR" || savedProduct.UnitPrice != "100" || savedProduct.TaxRate != "19" || savedProduct.TaxCategory != "S" {
		t.Fatalf("product normalization: %+v", savedProduct)
	}
	for _, bad := range []model.Product{
		{Name: " \t ", Unit: "C62", UnitPrice: "10", TaxRate: "19", TaxCategory: "S"},
		{Name: "Artikel", Unit: "", UnitPrice: "10", TaxRate: "19", TaxCategory: "S"},
		{Name: "Artikel", Unit: "C62", UnitPrice: "invalid", TaxRate: "19", TaxCategory: "S"},
		{Name: "Artikel", Unit: "C62", UnitPrice: "10", TaxRate: "0", TaxCategory: "S"},
		{Name: "Artikel", Unit: "C62", UnitPrice: "10", TaxRate: "19", TaxCategory: "E"},
		{Name: "Artikel", Unit: "C62", UnitPrice: "10", TaxRate: "0", TaxCategory: "INVALID"},
	} {
		if _, err = s.SaveProduct(bad); err == nil {
			t.Fatalf("invalid product saved: %+v", bad)
		}
	}
}

func TestZeroRateAndPeriodRoundTrip(t *testing.T) {
	s, err := NewInvoiceService(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err = s.SaveCompany(testCompany()); err != nil {
		t.Fatal(err)
	}
	d := testDraft()
	d.TaxTreatment = "zero-rated"
	d.TaxExemptionReason = "Umsatzsteuerbefreit nach § 19 UStG"
	d.PeriodStart = "2026-08-01"
	d.PeriodEnd = "2026-08-31"
	view, err := s.ValidateInvoice(d)
	if err != nil || view.Validation.State != "valid" {
		t.Fatalf("validate: %v %+v", err, view.Validation)
	}
	inv, err := einvoice.ParseReader(strings.NewReader(view.XML))
	if err != nil {
		t.Fatal(err)
	}
	if inv.InvoiceLines[0].TaxCategoryCode != "Z" || view.Totals.Tax != "0.00" || strings.Contains(view.XML, "§ 19") || strings.Contains(view.PreviewHTML, "§ 19") {
		t.Fatal("zero-rated confused with exemption")
	}
	if inv.BillingSpecifiedPeriodStart.Format("2006-01-02") != d.PeriodStart || inv.BillingSpecifiedPeriodEnd.Format("2006-01-02") != d.PeriodEnd || !strings.Contains(view.PreviewHTML, d.PeriodEnd) {
		t.Fatal("period not exported/rendered")
	}
	saved, err := s.SaveInvoice(d)
	if err != nil {
		t.Fatal(err)
	}
	opened, err := s.OpenInvoice(saved.Draft.ID)
	if err != nil || opened.Draft.PeriodStart != d.PeriodStart || opened.Draft.PeriodEnd != d.PeriodEnd {
		t.Fatal("period not saved")
	}
	issued, err := s.IssueInvoice(opened.Draft)
	if err != nil {
		t.Fatal(err)
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
	if err = restored.RestoreBackup(backup); err != nil {
		t.Fatal(err)
	}
	roundTrip, err := restored.OpenInvoice(issued.Draft.ID)
	if err != nil || roundTrip.Draft.PeriodEnd != d.PeriodEnd || roundTrip.XML != issued.XML {
		t.Fatal("issued period/artifact changed after restore")
	}
}

func TestPeriodBoundariesAndBackwardCompatibility(t *testing.T) {
	s, err := NewInvoiceService(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err = s.SaveCompany(testCompany()); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		start, end string
		valid      bool
	}{{"", "", true}, {"2026-08-01", "", true}, {"", "2026-08-31", true}, {"2026-08-01", "2026-08-01", true}, {"2026-09-10", "2026-10-01", true}, {"2026-08-31", "2026-08-01", false}, {"2026-02-30", "2026-08-01", false}} {
		d := testDraft()
		d.PeriodStart, d.PeriodEnd = tc.start, tc.end
		v, err := s.ValidateInvoice(d)
		if err != nil {
			t.Fatal(err)
		}
		if (v.Validation.State == "valid") != tc.valid {
			t.Fatalf("period %s/%s: %+v", tc.start, tc.end, v.Validation)
		}
	}
}

func TestIntraCommunityDeliveryCountryOverride(t *testing.T) {
	s, err := NewInvoiceService(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err = s.SaveCompany(testCompany()); err != nil {
		t.Fatal(err)
	}
	for _, country := range []string{"", "BE"} {
		d := testDraft()
		d.TaxTreatment = "intra-community"
		d.DeliveryCountry = country
		v, err := s.ValidateInvoice(d)
		if err != nil || v.Validation.State != "valid" {
			t.Fatalf("delivery country: %v %+v", err, v.Validation)
		}
		inv, err := einvoice.ParseReader(strings.NewReader(v.XML))
		if err != nil {
			t.Fatal(err)
		}
		expected := country
		if expected == "" {
			expected = d.Customer.Address.Country
		}
		if inv.ShipTo == nil || inv.ShipTo.PostalAddress == nil || inv.ShipTo.PostalAddress.CountryID != expected {
			t.Fatalf("incorrect delivery destination: %+v", inv.ShipTo)
		}
	}
}
