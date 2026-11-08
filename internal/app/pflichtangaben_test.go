package app

import (
	"testing"

	"papierleicht.de/e-rechnung-studio/internal/model"
)

func fieldOf(view model.InvoiceView, field string) *model.ValidationIssue {
	for i := range view.Validation.Issues {
		if view.Validation.Issues[i].Field == field {
			return &view.Validation.Issues[i]
		}
	}
	return nil
}

// § 14 Abs. 4 Satz 1 Nr. 1 und 6 UStG: the e-invoice rules need no date of
// supply and only a country in the addresses, so the app checks them itself.
func TestInvoiceNeedsTheTimeOfSupplyAndACompleteAddress(t *testing.T) {
	s := receivedService(t)
	if view, err := s.ValidateInvoice(testDraft()); err != nil || view.Validation.State != "valid" {
		t.Fatalf("baseline: %v %+v", err, view.Validation)
	}
	for _, tc := range []struct {
		name, field string
		change      func(*model.InvoiceDraft)
	}{
		{"no date of supply", "deliveryDate", func(d *model.InvoiceDraft) { d.DeliveryDate = "" }},
		{"no street", "customer.address.street", func(d *model.InvoiceDraft) { d.Customer.Address.Street = "" }},
		{"no city", "customer.address.city", func(d *model.InvoiceDraft) { d.Customer.Address.City = " " }},
		{"no German postcode", "customer.address.postcode", func(d *model.InvoiceDraft) {
			d.Customer.Address = model.Address{Street: "Weg 1", City: "Berlin", Country: "de"}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := testDraft()
			tc.change(&d)
			view, err := s.ValidateInvoice(d)
			if err != nil {
				t.Fatal(err)
			}
			if view.Validation.State != "invalid" || fieldOf(view, tc.field) == nil {
				t.Fatalf("expected a finding on %s: %+v", tc.field, view.Validation)
			}
			if _, err := s.IssueInvoice(d); err == nil {
				t.Fatal("issued an invoice without a Pflichtangabe")
			}
		})
	}
	t.Run("a period of supply comes in addition to the date", func(t *testing.T) {
		d := testDraft()
		d.DeliveryDate, d.PeriodStart, d.PeriodEnd = "2026-09-30", "2026-09-01", "2026-09-30"
		if view, err := s.ValidateInvoice(d); err != nil || view.Validation.State != "valid" {
			t.Fatalf("%v %+v", err, view.Validation)
		}
		d.DeliveryDate = ""
		if view, err := s.ValidateInvoice(d); err != nil || view.Validation.State != "invalid" || fieldOf(view, "deliveryDate") == nil {
			t.Fatalf("the period alone must not pass: %v %+v", err, view.Validation)
		}
	})
	t.Run("the seller's German address needs a postcode as well", func(t *testing.T) {
		c := testCompany()
		c.Endpoint, c.EndpointScheme, c.Phone = c.Email, "EM", "030123456"
		c.Address.Postcode = ""
		if _, err := s.SaveCompany(c); err != nil {
			t.Fatal(err)
		}
		view, err := s.ValidateInvoice(testDraft())
		if err != nil || view.Validation.State != "invalid" || fieldOf(view, "company.address.postcode") == nil {
			t.Fatalf("%v %+v", err, view.Validation)
		}
	})
}

func TestQuotesAreNotHeldToTheInvoicePflichtangaben(t *testing.T) {
	s := receivedService(t)
	d := testDraft()
	d.DeliveryDate = ""
	d.Customer.Address = model.Address{Country: "DE"}
	quote := model.Quote{Document: d, ValidUntil: "2026-12-31"}
	view, err := s.PreviewQuote(quote)
	if err != nil {
		t.Fatal(err)
	}
	for _, issue := range view.Validation.Issues {
		if issue.Field == "deliveryDate" || issue.Field == "customer.address.street" || issue.Field == "customer.address.postcode" {
			t.Fatalf("a quote needs no date of supply or full address: %+v", issue)
		}
	}
}

// BMF letter of 15.10.2026, Rn. 10 to 12: the app points out a brief description
// of the supply, but never blocks issuing because of it.
func TestBriefDescriptionIsAdviceNotAnError(t *testing.T) {
	s := receivedService(t)
	d := testDraft()
	d.Lines[0].Name = "Beratung"
	view, err := s.ValidateInvoice(d)
	if err != nil {
		t.Fatal(err)
	}
	issue := fieldOf(view, "lines.0.description")
	if view.Validation.State != "valid" || issue == nil || issue.RuleID != "APP-DESCRIPTION" || issue.Severity != "information" {
		t.Fatalf("expected advice on a valid invoice: %+v", view.Validation)
	}
	issued, err := s.IssueInvoice(d)
	if err != nil || issued.Draft.Status != "issued" {
		t.Fatalf("advice must not block issuing: %v", err)
	}
	d = testDraft()
	d.Lines[0].Name = "Beratung zur Umsatzsteuer, 2 Std."
	if view, err = s.ValidateInvoice(d); err != nil || fieldOf(view, "lines.0.description") != nil {
		t.Fatalf("a described supply needs no advice: %v %+v", err, view.Validation)
	}
}
