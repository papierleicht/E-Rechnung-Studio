package model

import "testing"

func TestRecurringDatesSurviveShortMonths(t *testing.T) {
	r := RecurringInvoice{Interval: "monthly", NextDate: "2027-01-31", PeriodStart: "2027-01-01", InvoiceDay: 31, PeriodDay: 1}
	want := []string{"2027-02-28", "2027-03-31", "2027-04-30"}
	for _, date := range want {
		var err error
		if r, err = r.Advance(); err != nil {
			t.Fatal(err)
		}
		if r.NextDate != date {
			t.Fatalf("next date %s, want %s", r.NextDate, date)
		}
	}
	if r.PeriodStart != "2027-04-01" {
		t.Fatalf("period start %s", r.PeriodStart)
	}
}

func TestRecurringPeriodEndsBeforeTheNextOne(t *testing.T) {
	for _, tt := range []struct{ interval, start, end string }{
		{"monthly", "2028-02-01", "2028-02-29"},
		{"quarterly", "2027-10-01", "2027-12-31"},
		{"half-yearly", "2027-07-01", "2027-12-31"},
		{"yearly", "2027-01-01", "2027-12-31"},
	} {
		r, err := RecurringInvoice{Interval: tt.interval, NextDate: "2027-01-01", PeriodStart: tt.start, PeriodDay: 1}.Decorate()
		if err != nil {
			t.Fatal(err)
		}
		if r.PeriodEnd != tt.end {
			t.Fatalf("%s: period end %s, want %s", tt.interval, r.PeriodEnd, tt.end)
		}
	}
}

func TestRecurringStatus(t *testing.T) {
	base := RecurringInvoice{Interval: "monthly", NextDate: "2027-03-01", PeriodStart: "2027-02-01", PeriodDay: 1}
	ended := base
	ended.EndDate = "2027-02-15"
	paused := base
	paused.Paused = true
	for _, tt := range []struct {
		plan   RecurringInvoice
		status string
		due    bool
	}{{base, RecurringActive, true}, {ended, RecurringEnded, false}, {paused, RecurringPaused, false}} {
		r, err := tt.plan.Decorate()
		if err != nil {
			t.Fatal(err)
		}
		if r.Status != tt.status || r.Due("2027-03-01") != tt.due {
			t.Fatalf("%+v: status %s, due %v", tt.plan, r.Status, r.Due("2027-03-01"))
		}
	}
	if base.Due("2027-02-28") {
		t.Fatal("due before its date")
	}
}

func TestRecurringDraftKeepsThePaymentTermAndNoIdentity(t *testing.T) {
	source := InvoiceDraft{ID: "inv-1", Number: "RE-2026-0001", Status: "paid", IssueDate: "2026-10-01", DueDate: "2026-10-31", DeliveryDate: "2026-10-01", PaymentReference: "RE-2026-0001", AdvancePayment: true, IssuerSnapshot: &Company{}, GeneratedXML: "<x/>", Lines: []InvoiceLine{{ID: "1", Name: "Hosting"}}}
	r := RecurringInvoice{ID: "rec-1", Name: "Hosting", Interval: "monthly", NextDate: "2026-12-01", PeriodStart: "2026-11-01", PeriodDay: 1, Template: RecurringTemplate(source)}
	days := 7
	d, err := r.Draft("2026-12-03", &Customer{Name: "Neue Anschrift", PaymentTermDays: &days})
	if err != nil {
		t.Fatal(err)
	}
	if d.ID != "" || d.Number != "" || d.Status != "draft" || !d.AutomaticNumber || d.PaymentReference != "" || d.AdvancePayment || d.IssuerSnapshot != nil || d.GeneratedXML != "" {
		t.Fatalf("identity left over: %+v", d)
	}
	if d.IssueDate != "2026-12-03" || d.DueDate != "2027-01-02" {
		t.Fatalf("dates: issue %s, due %s", d.IssueDate, d.DueDate)
	}
	if d.PeriodStart != "2026-11-01" || d.PeriodEnd != "2026-11-30" || d.DeliveryDate != "2026-11-30" {
		t.Fatalf("period: %s to %s, supply %s", d.PeriodStart, d.PeriodEnd, d.DeliveryDate)
	}
	if d.Customer.Name != "Neue Anschrift" || d.RecurringID != "rec-1" || d.RecurringName != "Hosting" {
		t.Fatalf("draft: %+v", d)
	}
	d.Lines[0].Name = "Geändert"
	if r.Template.Lines[0].Name != "Hosting" {
		t.Fatal("draft shares its lines with the template")
	}
}

func TestRecurringDraftWithoutDueDateStaysWithout(t *testing.T) {
	r := RecurringInvoice{Interval: "yearly", PeriodStart: "2026-01-01", PeriodDay: 1, Template: InvoiceDraft{IssueDate: "2026-01-01"}}
	d, err := r.Draft("2027-01-01", nil)
	if err != nil {
		t.Fatal(err)
	}
	if d.DueDate != "" {
		t.Fatalf("due date %s", d.DueDate)
	}
}
