package app

import (
	"strings"
	"testing"
	"time"

	"papierleicht.de/e-rechnung-studio/internal/model"
)

func TestCustomerSeriesContinuesAndStaysUnique(t *testing.T) {
	s := newTestService(t)
	customer, err := s.SaveCustomer(model.Customer{Name: "Müller GmbH", Address: model.Address{Street: "Weg 1", Postcode: "10115", City: "Berlin", Country: "DE"}})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.StartNumberSeries(NumberSeriesStart{Series: "MUE-{JJJJ}-{NNN}", IssueDate: "2026-09-09", Next: 50, CustomerID: customer.ID}); err != nil {
		t.Fatal(err)
	}
	draft := testDraft()
	draft.Number, draft.AutomaticNumber, draft.CustomerID = "", true, customer.ID
	preview, err := s.PreviewInvoice(draft)
	if err != nil || preview.Draft.Number != "MUE-2026-050" {
		t.Fatalf("customer series not used: %q %v", preview.Draft.Number, err)
	}
	issued, err := s.IssueInvoice(draft)
	if err != nil || issued.Draft.Number != "MUE-2026-050" {
		t.Fatalf("issued %q %v", issued.Draft.Number, err)
	}
	if preview, _ = s.PreviewInvoice(draft); preview.Draft.Number != "MUE-2026-051" {
		t.Fatalf("customer series did not continue: %q", preview.Draft.Number)
	}
	// The customer series is remembered across drafts, also after a later save.
	if saved, err := s.SaveInvoice(draft); err != nil || saved.Draft.Number != "MUE-2026-051" {
		t.Fatalf("saved %q %v", saved.Draft.Number, err)
	}
	if series, _ := s.store.CustomerSeries(customer.ID); series != "MUE-{JJJJ}-{NNN}" {
		t.Fatalf("saving a draft changed the customer series: %q", series)
	}
	standard := draft
	standard.NumberSeries = model.SeriesStandard
	if preview, _ = s.PreviewInvoice(standard); preview.Draft.Number != "RE-2026-0001" {
		t.Fatalf("explicit standard: %q", preview.Draft.Number)
	}
	other := testDraft()
	other.Number, other.AutomaticNumber = "", true
	if preview, _ = s.PreviewInvoice(other); preview.Draft.Number != "RE-2026-0001" {
		t.Fatalf("other customers follow the standard: %q", preview.Draft.Number)
	}
	options, err := s.InvoiceNumberOptions(draft)
	if err != nil || len(options.Options) < 2 || options.Selected != "" || options.CustomerSeries == "" {
		t.Fatalf("options %+v %v", options, err)
	}
	if options.Options[0].Kind != "standard" || options.Options[0].Value != model.SeriesStandard || options.Options[1].Next != "MUE-2026-051" || options.Options[1].Last.Number != "MUE-2026-050" {
		t.Fatalf("option details %+v / %+v", options.Options[0], options.Options[1])
	}
	// A manual number that is taken blocks issuing, in any spelling.
	manual := testDraft()
	manual.Number = "mue-2026-050"
	view, err := s.PreviewInvoice(manual)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, issue := range view.Validation.Issues {
		found = found || issue.Field == "number" && strings.Contains(issue.Message, "schon")
	}
	if !found || view.Validation.State == "valid" {
		t.Fatalf("taken manual number not reported: %+v", view.Validation)
	}
	if _, err = s.IssueInvoice(manual); err == nil {
		t.Fatal("taken number issued")
	}
	if err = s.StartNumberSeries(NumberSeriesStart{Series: "MUE-{JJJJ}-{NNN}", IssueDate: "2026-09-09", Next: 20, CustomerID: customer.ID}); err == nil {
		t.Fatal("series went back below a number in use")
	}
	if err = s.RemoveCustomerNumberSeries(customer.ID); err != nil {
		t.Fatal(err)
	}
	if preview, _ = s.PreviewInvoice(draft); preview.Draft.Number != "RE-2026-0001" {
		t.Fatalf("removed customer series still used: %q", preview.Draft.Number)
	}
}

func TestCheckInvoiceNumberExplainsGapsAndOrder(t *testing.T) {
	s := newTestService(t)
	issueNumbered(t, s, "2026/118")
	draft := testDraft()
	check := func(number, state, hint string) NumberCheck {
		t.Helper()
		result, err := s.CheckInvoiceNumber(draft, number)
		if err != nil {
			t.Fatal(err)
		}
		if result.State != state || !strings.Contains(strings.Join(result.Hints, " ")+result.Message, hint) {
			t.Fatalf("%s: %+v", number, result)
		}
		return result
	}
	taken := check("2026/118", "taken", "schon vergeben")
	if taken.Next != "2026/119" || taken.Pattern != "JJJJ/NNN" {
		t.Fatalf("no suggestion for a taken number: %+v", taken)
	}
	next := check("2026/119", "ok", "frei")
	if !next.CanContinue || next.Counter != 119 || len(next.Hints) != 0 {
		t.Fatalf("next number: %+v", next)
	}
	check("2026/125", "ok", "bleiben 6 Nummern frei")
	if lower := check("2026/100", "ok", "schon 2026/118 vergeben"); lower.CanContinue {
		t.Fatal("a series may not continue below its last number")
	}
	check("K12-001", "ok", "beginnt der Nummernkreis K12-NNN")
	check("RE 1", "invalid", "Leerzeichen")
	check("", "invalid", "Gib eine Rechnungsnummer ein")
	draft.Number = "2026/130"
	if _, err := s.SaveInvoice(draft); err != nil {
		t.Fatal(err)
	}
	check("2026/130", "ok", "Der Entwurf an Beispielkunde AG nutzt dieselbe Nummer")
}

func TestStandardSeriesSettings(t *testing.T) {
	s := newTestService(t)
	settings, err := s.InvoiceNumberSettings()
	today := time.Now()
	if err != nil || settings.Series != model.DefaultNumberSeries || settings.NextValue != 1 {
		t.Fatalf("defaults %+v %v", settings, err)
	}
	if _, err = s.SaveInvoiceNumberSettings("{JJJJ}{NNN}", 0); err == nil {
		t.Fatal("ambiguous series accepted")
	}
	if settings, err = s.SaveInvoiceNumberSettings("{JJJJ}/{NNN}", 118); err != nil {
		t.Fatal(err)
	}
	want := today.Format("2006") + "/118"
	if settings.Next != want || settings.Pattern != "JJJJ/NNN" {
		t.Fatalf("settings %+v", settings)
	}
	draft := testDraft()
	draft.Number, draft.AutomaticNumber, draft.IssueDate = "", true, today.Format(time.DateOnly)
	draft.DeliveryDate, draft.DueDate = draft.IssueDate, today.AddDate(0, 0, 14).Format(time.DateOnly)
	issued, err := s.IssueInvoice(draft)
	if err != nil || issued.Draft.Number != want {
		t.Fatalf("issued %q %v", issued.Draft.Number, err)
	}
	if _, err = s.SaveInvoiceNumberSettings("{JJJJ}/{NNN}", 100); err == nil || !strings.Contains(err.Error(), "mindestens 119") {
		t.Fatalf("count went back: %v", err)
	}
	if preview, err := s.PreviewNumberSeries("RE-{JJJJ}-{NNNN}", 0); err != nil || preview.NextValue != 1 {
		t.Fatalf("preview %+v %v", preview, err)
	}
}
