package app

import (
	"errors"
	"strings"
	"testing"
	"time"

	"papierleicht.de/e-rechnung-studio/internal/model"
	"papierleicht.de/e-rechnung-studio/internal/store"
)

func issuedForRecurring(t *testing.T, s *InvoiceService) model.InvoiceDraft {
	t.Helper()
	if _, err := s.SaveCompany(testCompany()); err != nil {
		t.Fatal(err)
	}
	d := testDraft()
	d.Number, d.AutomaticNumber = "", true
	today := time.Now().Format(time.DateOnly)
	d.IssueDate, d.DeliveryDate, d.DueDate = today, today, time.Now().AddDate(0, 0, 30).Format(time.DateOnly)
	saved, err := s.SaveInvoice(d)
	if err != nil {
		t.Fatal(err)
	}
	issued, err := s.IssueInvoice(saved.Draft)
	if err != nil {
		t.Fatal(err)
	}
	return issued.Draft
}

func savedRecurring(t *testing.T, s *InvoiceService, change func(*model.RecurringInvoice)) model.RecurringInvoice {
	t.Helper()
	plan, err := s.RecurringFromInvoice(issuedForRecurring(t, s).ID)
	if err != nil {
		t.Fatal(err)
	}
	if change != nil {
		change(&plan)
	}
	if plan, err = s.SaveRecurringInvoice(plan); err != nil {
		t.Fatal(err)
	}
	return plan
}

func countDrafts(t *testing.T, s *InvoiceService) []model.InvoiceSummary {
	t.Helper()
	all, err := s.ListInvoices()
	if err != nil {
		t.Fatal(err)
	}
	drafts := []model.InvoiceSummary{}
	for _, item := range all {
		if item.Status == "draft" {
			drafts = append(drafts, item)
		}
	}
	return drafts
}

func TestProposalContinuesTheInvoicePeriod(t *testing.T) {
	plan, err := proposeRecurring(model.InvoiceDraft{IssueDate: "2026-11-03", PeriodStart: "2026-10-01", PeriodEnd: "2026-10-31", Lines: []model.InvoiceLine{{Name: " Hosting "}}}, "2026-11-05")
	if err != nil {
		t.Fatal(err)
	}
	if plan.Name != "Hosting" || plan.NextDate != "2026-12-03" || plan.PeriodStart != "2026-11-01" || plan.PeriodEnd != "2026-11-30" {
		t.Fatalf("proposal: %+v", plan)
	}
}

func TestProposalWithoutPeriodBillsTheFullMonthBefore(t *testing.T) {
	plan, err := proposeRecurring(model.InvoiceDraft{IssueDate: "2026-11-15"}, "2026-11-15")
	if err != nil {
		t.Fatal(err)
	}
	if plan.NextDate != "2026-12-15" || plan.PeriodStart != "2026-11-01" || plan.PeriodEnd != "2026-11-30" {
		t.Fatalf("proposal: %+v", plan)
	}
}

func TestOldInvoiceProposesAFutureDate(t *testing.T) {
	plan, err := proposeRecurring(model.InvoiceDraft{IssueDate: "2026-01-10"}, "2026-10-31")
	if err != nil {
		t.Fatal(err)
	}
	if plan.NextDate != "2026-11-10" || plan.PeriodStart != "2026-10-01" {
		t.Fatalf("proposal: %+v", plan)
	}
}

func TestRunCreatesOneDraftPerPeriodOnce(t *testing.T) {
	s := newTestService(t)
	plan := savedRecurring(t, s, func(p *model.RecurringInvoice) {
		p.NextDate, p.PeriodStart = time.Now().AddDate(0, -2, 0).Format("2006-01")+"-01", time.Now().AddDate(0, -3, 0).Format("2006-01")+"-01"
	})
	today := time.Now().Format(time.DateOnly)
	run, err := s.runRecurring(today)
	if err != nil {
		t.Fatal(err)
	}
	if run.Created != 3 || len(run.Failed) != 0 {
		t.Fatalf("run: %+v", run)
	}
	again, err := s.runRecurring(today)
	if err != nil {
		t.Fatal(err)
	}
	if again.Created != 0 {
		t.Fatalf("second run created %d drafts", again.Created)
	}
	drafts := countDrafts(t, s)
	if len(drafts) != 3 {
		t.Fatalf("%d drafts", len(drafts))
	}
	periods := map[string]bool{}
	for _, item := range drafts {
		d, err := s.store.Invoice(item.ID)
		if err != nil {
			t.Fatal(err)
		}
		// Nothing is issued or numbered for good, and the date is the real one.
		if d.Status != "draft" || d.IssueDate != today || d.RecurringID != plan.ID || d.PeriodEnd == "" || d.DeliveryDate != d.PeriodEnd {
			t.Fatalf("draft: %+v", d)
		}
		periods[d.PeriodStart] = true
	}
	if len(periods) != 3 {
		t.Fatalf("periods: %v", periods)
	}
	list, err := s.ListRecurringInvoices()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].OpenDrafts != 3 || list[0].NextDate <= today {
		t.Fatalf("list: %+v", list)
	}
}

func TestDraftFromPlanCanBeIssuedAndCarriesANewPriceBack(t *testing.T) {
	s := newTestService(t)
	today := time.Now().Format(time.DateOnly)
	plan := savedRecurring(t, s, func(p *model.RecurringInvoice) {
		p.NextDate, p.PeriodStart = today, time.Now().AddDate(0, -1, 0).Format("2006-01")+"-01"
	})
	if run, err := s.runRecurring(today); err != nil || run.Created != 1 {
		t.Fatalf("run %+v, %v", run, err)
	}
	draft, err := s.OpenInvoice(countDrafts(t, s)[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	draft.Draft.Lines[0].UnitPrice = "250.00"
	saved, err := s.SaveInvoice(draft.Draft)
	if err != nil {
		t.Fatal(err)
	}
	issued, err := s.IssueInvoice(saved.Draft)
	if err != nil {
		t.Fatal(err)
	}
	if issued.Draft.Status != "issued" || issued.Draft.RecurringID != plan.ID {
		t.Fatalf("issued: %+v", issued.Draft)
	}
	proposal, err := s.RecurringFromInvoice(issued.Draft.ID)
	if err != nil {
		t.Fatal(err)
	}
	if proposal.ID != plan.ID || proposal.Template.Lines[0].UnitPrice != "250.00" {
		t.Fatalf("proposal: %+v", proposal)
	}
	updated, err := s.SaveRecurringInvoice(proposal)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Gross == plan.Gross {
		t.Fatalf("gross stayed %s", updated.Gross)
	}
}

func TestCatchUpIsLimitedPerRun(t *testing.T) {
	s := newTestService(t)
	plan := savedRecurring(t, s, nil)
	stored, err := s.store.RecurringInvoice(plan.ID)
	if err != nil {
		t.Fatal(err)
	}
	stored.NextDate, stored.PeriodStart = "2020-01-01", "2019-12-01"
	stored.InvoiceDay, stored.PeriodDay = 1, 1
	if _, err = s.store.SaveRecurringInvoice(stored); err != nil {
		t.Fatal(err)
	}
	run, err := s.runRecurring("2026-10-31")
	if err != nil {
		t.Fatal(err)
	}
	if run.Created != recurringCatchUp {
		t.Fatalf("created %d", run.Created)
	}
}

func TestSecondRunOverStaleDateIsRejected(t *testing.T) {
	s := newTestService(t)
	plan := savedRecurring(t, s, nil)
	next, err := plan.Advance()
	if err != nil {
		t.Fatal(err)
	}
	draft, err := plan.Draft(plan.NextDate, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.store.AddRecurringDraft(next, plan.NextDate, draft, "1.00"); err != nil {
		t.Fatal(err)
	}
	if err = s.store.AddRecurringDraft(next, plan.NextDate, draft, "1.00"); !errors.Is(err, store.ErrRecurringChanged) {
		t.Fatalf("second add: %v", err)
	}
	if n := len(countDrafts(t, s)); n != 1 {
		t.Fatalf("%d drafts", n)
	}
}

func TestPauseSkipsThePeriodsInBetween(t *testing.T) {
	s := newTestService(t)
	plan := savedRecurring(t, s, nil)
	today := plan.NextDate
	if _, err := s.SetRecurringPaused(plan.ID, true); err != nil {
		t.Fatal(err)
	}
	if run, err := s.runRecurring(today); err != nil || run.Created != 0 {
		t.Fatalf("paused run %+v, %v", run, err)
	}
	later := time.Now().AddDate(0, 4, 0).Format(time.DateOnly)
	resumed, err := s.setRecurringPaused(plan.ID, false, later)
	if err != nil {
		t.Fatal(err)
	}
	if resumed.Paused || resumed.NextDate < later {
		t.Fatalf("resumed: %+v", resumed)
	}
	// Only a date that falls exactly on the resume day is due; the skipped periods never are.
	due := 0
	if resumed.NextDate == later {
		due = 1
	}
	if run, err := s.runRecurring(later); err != nil || run.Created != due {
		t.Fatalf("resumed run %+v, %v", run, err)
	}
}

func TestEndDateStopsTheDrafts(t *testing.T) {
	s := newTestService(t)
	plan := savedRecurring(t, s, nil)
	plan.EndDate = plan.NextDate
	plan, err := s.SaveRecurringInvoice(plan)
	if err != nil {
		t.Fatal(err)
	}
	run, err := s.runRecurring(time.Now().AddDate(1, 0, 0).Format(time.DateOnly))
	if err != nil {
		t.Fatal(err)
	}
	if run.Created != 1 {
		t.Fatalf("created %d", run.Created)
	}
	list, err := s.ListRecurringInvoices()
	if err != nil {
		t.Fatal(err)
	}
	if list[0].Status != model.RecurringEnded {
		t.Fatalf("status %s", list[0].Status)
	}
}

func TestStoredCustomerReachesTheDraft(t *testing.T) {
	s := newTestService(t)
	if _, err := s.SaveCompany(testCompany()); err != nil {
		t.Fatal(err)
	}
	customer := testDraft().Customer
	customer.ID = ""
	stored, err := s.SaveCustomer(customer)
	if err != nil {
		t.Fatal(err)
	}
	today := time.Now().Format(time.DateOnly)
	d := testDraft()
	d.Number, d.AutomaticNumber, d.CustomerID, d.Customer = "", true, stored.ID, stored
	d.IssueDate, d.DeliveryDate = today, today
	saved, err := s.SaveInvoice(d)
	if err != nil {
		t.Fatal(err)
	}
	issued, err := s.IssueInvoice(saved.Draft)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := s.RecurringFromInvoice(issued.Draft.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.SaveRecurringInvoice(plan); err != nil {
		t.Fatal(err)
	}
	stored.Address.City = "Neustadt"
	if _, err = s.SaveCustomer(stored); err != nil {
		t.Fatal(err)
	}
	if run, err := s.runRecurring(plan.NextDate); err != nil || run.Created != 1 {
		t.Fatalf("run %+v, %v", run, err)
	}
	view, err := s.OpenInvoice(countDrafts(t, s)[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if view.Draft.Customer.Address.City != "Neustadt" {
		t.Fatalf("city %s", view.Draft.Customer.Address.City)
	}
	// The customer's own payment term is not touched by a generated draft.
	after, err := s.store.Customer(stored.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after == nil || after.Address.City != "Neustadt" {
		t.Fatalf("customer: %+v", after)
	}
}

func TestSaveRejectsWrongInput(t *testing.T) {
	s := newTestService(t)
	valid, err := s.RecurringFromInvoice(issuedForRecurring(t, s).ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		change  func(*model.RecurringInvoice)
		message string
	}{
		{func(p *model.RecurringInvoice) { p.Name = " " }, "Namen"},
		{func(p *model.RecurringInvoice) { p.Interval = "weekly" }, "Rhythmus"},
		{func(p *model.RecurringInvoice) { p.NextDate = "" }, "nächsten Rechnung"},
		{func(p *model.RecurringInvoice) { p.PeriodStart = "" }, "Leistungszeitraums"},
		{func(p *model.RecurringInvoice) { p.EndDate = "2000-01-01" }, "Enddatum"},
		{func(p *model.RecurringInvoice) { p.NextDate = "2000-01-01" }, "Jahr zurück"},
		{func(p *model.RecurringInvoice) { p.Template.Lines = nil }, "Position"},
	} {
		plan := valid
		tt.change(&plan)
		if _, err := s.SaveRecurringInvoice(plan); err == nil || !strings.Contains(err.Error(), tt.message) {
			t.Fatalf("want %q, got %v", tt.message, err)
		}
	}
}

func TestOnlyIssuedInvoicesRepeat(t *testing.T) {
	s := newTestService(t)
	if _, err := s.SaveCompany(testCompany()); err != nil {
		t.Fatal(err)
	}
	saved, err := s.SaveInvoice(testDraft())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.RecurringFromInvoice(saved.Draft.ID); err == nil {
		t.Fatal("draft repeated")
	}
}

func TestDeletingAPlanKeepsItsDrafts(t *testing.T) {
	s := newTestService(t)
	plan := savedRecurring(t, s, nil)
	if run, err := s.runRecurring(plan.NextDate); err != nil || run.Created != 1 {
		t.Fatalf("run %+v, %v", run, err)
	}
	if err := s.DeleteRecurringInvoice(plan.ID); err != nil {
		t.Fatal(err)
	}
	if n := len(countDrafts(t, s)); n != 1 {
		t.Fatalf("%d drafts", n)
	}
}

func TestBackupKeepsRecurringInvoices(t *testing.T) {
	s := newTestService(t)
	plan := savedRecurring(t, s, nil)
	backup, err := s.ExportBackup()
	if err != nil {
		t.Fatal(err)
	}
	if err = s.DeleteRecurringInvoice(plan.ID); err != nil {
		t.Fatal(err)
	}
	if err = s.RestoreBackup(backup); err != nil {
		t.Fatal(err)
	}
	list, err := s.ListRecurringInvoices()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].ID != plan.ID || list[0].NextDate != plan.NextDate {
		t.Fatalf("list: %+v", list)
	}
}
