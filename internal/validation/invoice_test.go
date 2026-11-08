package validation

import (
	"strings"
	"testing"

	"papierleicht.de/e-rechnung-studio/internal/model"
)

func fieldsOf(issues []model.ValidationIssue) string {
	var fields []string
	for _, issue := range issues {
		fields = append(fields, issue.Field)
	}
	return strings.Join(fields, ",")
}

func completeInvoice() (model.InvoiceDraft, model.Company) {
	d := model.NewDraft()
	d.Customer = model.Customer{Name: "Kunde", Address: model.Address{Street: "Weg 1", Postcode: "10115", City: "Berlin", Country: "DE"}}
	c := model.Company{Name: "Firma", Address: model.Address{Street: "Straße 2", Postcode: "20095", City: "Hamburg", Country: "DE"}}
	return d, c
}

func TestInvoiceIssuesAcceptsACompleteInvoice(t *testing.T) {
	d, c := completeInvoice()
	if issues := InvoiceIssues(d, c); len(issues) != 0 {
		t.Fatalf("unexpected issues: %+v", issues)
	}
}

func TestInvoiceIssuesRequiresTheDateOfSupply(t *testing.T) {
	d, c := completeInvoice()
	d.DeliveryDate = ""
	if got := fieldsOf(InvoiceIssues(d, c)); got != "deliveryDate" {
		t.Fatalf("got %q", got)
	}
	// A period of supply does not replace the date; it comes in addition.
	d.PeriodStart, d.PeriodEnd = "2026-09-01", "2026-09-30"
	if got := fieldsOf(InvoiceIssues(d, c)); got != "deliveryDate" {
		t.Fatalf("with a period: got %q", got)
	}
	d.DeliveryDate = "2026-09-30"
	if issues := InvoiceIssues(d, c); len(issues) != 0 {
		t.Fatalf("date and period: %+v", issues)
	}
}

func TestInvoiceIssuesRequiresACompleteAddress(t *testing.T) {
	d, c := completeInvoice()
	d.Customer.Address = model.Address{Country: "de"}
	c.Address.Postcode = " "
	got := fieldsOf(InvoiceIssues(d, c))
	want := "customer.address.street,customer.address.city,customer.address.postcode,company.address.postcode"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	for _, issue := range InvoiceIssues(d, c) {
		if issue.Stage != "input" || issue.Severity != "error" || issue.Message == "" {
			t.Fatalf("a missing address part must block with a message: %+v", issue)
		}
	}
}

func TestInvoiceIssuesNeedsNoPostcodeOutsideGermany(t *testing.T) {
	d, c := completeInvoice()
	d.Customer.Address = model.Address{Street: "Rue Exemple 2", City: "Dublin", Country: "IE"}
	if issues := InvoiceIssues(d, c); len(issues) != 0 {
		t.Fatalf("unexpected issues: %+v", issues)
	}
}

func TestAdviceIssuesPointsOutBriefSupplies(t *testing.T) {
	line := func(name, description string) model.InvoiceLine {
		return model.InvoiceLine{Name: name, Description: description}
	}
	cases := []struct {
		name  string
		lines []model.InvoiceLine
		want  string
	}{
		{"one generic word", []model.InvoiceLine{line("Beratung", "")}, "lines.0.description"},
		{"one word of any kind", []model.InvoiceLine{line("Webdesign", "")}, "lines.0.description"},
		{"generic words only", []model.InvoiceLine{line("Leistung 03/2026", ""), line("Beratung und Support", "")}, "lines.0.description,lines.1.description"},
		{"substantial name", []model.InvoiceLine{line("Beratung zur Umsatzsteuer, 2 Std.", "")}, ""},
		{"two specific words", []model.InvoiceLine{line("Webdesign Relaunch", "")}, ""},
		{"brief name with a description", []model.InvoiceLine{line("Beratung", "Umsatzsteuerliche Prüfung des Vertriebsmodells")}, ""},
		{"second line only", []model.InvoiceLine{line("Relaunch der Startseite", ""), line("Support", "")}, "lines.1.description"},
		{"blank line is the form's job", []model.InvoiceLine{line("  ", "")}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := model.NewDraft()
			d.Lines = tc.lines
			issues := AdviceIssues(d)
			if got := fieldsOf(issues); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
			for _, issue := range issues {
				if issue.Severity != "information" || issue.Stage != "advice" || issue.RuleID != "APP-DESCRIPTION" {
					t.Fatalf("a hint must not look like an error: %+v", issue)
				}
			}
		})
	}
}

func TestAdviceIssuesOnlyForInvoicesAndShortensLongNames(t *testing.T) {
	d := model.NewDraft()
	d.Lines = []model.InvoiceLine{{Name: strings.Repeat("Programmierung", 10)}}
	issues := AdviceIssues(d)
	if len(issues) != 1 || strings.Count(issues[0].Message, "Programmierung") > 3 {
		t.Fatalf("long names must be cut in the message: %+v", issues)
	}
	for _, kind := range []string{"credit-note", "corrected-invoice"} {
		d.Kind = kind
		if got := AdviceIssues(d); got != nil {
			t.Fatalf("%s: %+v", kind, got)
		}
	}
}
