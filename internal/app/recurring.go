package app

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"papierleicht.de/e-rechnung-studio/internal/model"
	"papierleicht.de/e-rechnung-studio/internal/store"
)

// recurringCatchUp limits the drafts one run creates per recurring invoice.
// After a long pause the rest follows with the next run.
const recurringCatchUp = 12

func (s *InvoiceService) ListRecurringInvoices() ([]model.RecurringInvoice, error) {
	plans, err := s.store.RecurringInvoices()
	if err != nil {
		return plans, err
	}
	for i := range plans {
		if plans[i], err = plans[i].Decorate(); err != nil {
			return plans, err
		}
	}
	return plans, nil
}

// RecurringFromInvoice proposes a recurring invoice from an issued invoice. An
// invoice that came from one proposes that one with the content of this
// invoice, which is how a new price reaches the following months.
func (s *InvoiceService) RecurringFromInvoice(invoiceID string) (model.RecurringInvoice, error) {
	source, err := s.store.Invoice(invoiceID)
	if err != nil {
		return model.RecurringInvoice{}, err
	}
	if source.Status == "draft" || source.Kind != "invoice" {
		return model.RecurringInvoice{}, errors.New("Du kannst nur eine ausgestellte Rechnung wiederholen.")
	}
	view, err := s.view(model.RecurringTemplate(source), false)
	if err != nil {
		return model.RecurringInvoice{}, err
	}
	if source.RecurringID != "" {
		if plan, err := s.store.RecurringInvoice(source.RecurringID); err == nil {
			plan.Template, plan.SourceInvoiceID, plan.Gross = model.RecurringTemplate(source), source.ID, view.Totals.Gross
			return plan.Decorate()
		}
	}
	plan, err := proposeRecurring(source, time.Now().Format(time.DateOnly))
	plan.Gross = view.Totals.Gross
	return plan, err
}

// proposeRecurring suggests the following date of a monthly rhythm. It keeps
// the invoice's period, and without one bills the full month before the invoice.
func proposeRecurring(source model.InvoiceDraft, today string) (model.RecurringInvoice, error) {
	issued, err := time.Parse(time.DateOnly, source.IssueDate)
	if err != nil {
		return model.RecurringInvoice{}, err
	}
	period := time.Date(issued.Year(), issued.Month()-1, 1, 0, 0, 0, 0, time.UTC)
	if start, err := time.Parse(time.DateOnly, source.PeriodStart); err == nil {
		period = start
	}
	plan := model.RecurringInvoice{
		Name: recurringName(source), Interval: "monthly",
		NextDate: source.IssueDate, PeriodStart: period.Format(time.DateOnly),
		InvoiceDay: issued.Day(), PeriodDay: period.Day(),
		Template: model.RecurringTemplate(source), SourceInvoiceID: source.ID,
	}
	for plan.NextDate == source.IssueDate || plan.NextDate < today {
		if plan, err = plan.Advance(); err != nil {
			return plan, err
		}
	}
	return plan.Decorate()
}

func recurringName(d model.InvoiceDraft) string {
	if len(d.Lines) > 0 && strings.TrimSpace(d.Lines[0].Name) != "" {
		return strings.TrimSpace(d.Lines[0].Name)
	}
	return d.Customer.Name
}

// RecurringPreview shows the period the entered dates lead to.
func (s *InvoiceService) RecurringPreview(r model.RecurringInvoice) (model.RecurringInvoice, error) {
	r, err := withAnchors(r, nil)
	if err != nil {
		return r, err
	}
	return r.Decorate()
}

// withAnchors keeps the days of the month unless the dates were changed.
func withAnchors(r model.RecurringInvoice, stored *model.RecurringInvoice) (model.RecurringInvoice, error) {
	if !model.ValidRecurringInterval(r.Interval) {
		return r, errors.New("Wähle einen Rhythmus für die Dauerrechnung.")
	}
	next, err := time.Parse(time.DateOnly, r.NextDate)
	if err != nil {
		return r, errors.New("Gib das Datum der nächsten Rechnung an.")
	}
	period, err := time.Parse(time.DateOnly, r.PeriodStart)
	if err != nil {
		return r, errors.New("Gib den Beginn des Leistungszeitraums an.")
	}
	if stored == nil || stored.NextDate != r.NextDate {
		r.InvoiceDay = next.Day()
	}
	if stored == nil || stored.PeriodStart != r.PeriodStart {
		r.PeriodDay = period.Day()
	}
	return r, nil
}

func (s *InvoiceService) SaveRecurringInvoice(r model.RecurringInvoice) (model.RecurringInvoice, error) {
	var stored *model.RecurringInvoice
	if r.ID != "" {
		plan, err := s.store.RecurringInvoice(r.ID)
		if err != nil {
			return r, err
		}
		stored = &plan
		r.Paused = plan.Paused
	}
	r, err := withAnchors(r, stored)
	if err != nil {
		return r, err
	}
	if r.Name = strings.TrimSpace(r.Name); r.Name == "" {
		return r, errors.New("Gib der Dauerrechnung einen Namen, zum Beispiel „Hosting“.")
	}
	if r.EndDate != "" {
		if _, err = time.Parse(time.DateOnly, r.EndDate); err != nil {
			return r, errors.New("Das Enddatum ist ungültig.")
		}
		if r.EndDate < r.NextDate {
			return r, errors.New("Das Enddatum liegt vor der nächsten Rechnung.")
		}
	}
	limit := time.Now().AddDate(-1, 0, 0).Format(time.DateOnly)
	if r.NextDate < limit && (stored == nil || stored.NextDate != r.NextDate) {
		return r, errors.New("Die nächste Rechnung liegt mehr als ein Jahr zurück. Prüfe das Datum.")
	}
	if strings.TrimSpace(r.Template.Customer.Name) == "" || len(r.Template.Lines) == 0 {
		return r, errors.New("Die Dauerrechnung braucht einen Kunden und mindestens eine Position.")
	}
	r.Template = model.RecurringTemplate(r.Template)
	view, err := s.view(r.Template, false)
	if err != nil {
		return r, err
	}
	r.Gross = view.Totals.Gross
	if r, err = s.store.SaveRecurringInvoice(r); err != nil {
		return r, err
	}
	return r.Decorate()
}

// SetRecurringPaused pauses a recurring invoice. Resuming skips the dates in
// between: nobody wants invoices for the time it was paused.
func (s *InvoiceService) SetRecurringPaused(id string, paused bool) (model.RecurringInvoice, error) {
	return s.setRecurringPaused(id, paused, time.Now().Format(time.DateOnly))
}

func (s *InvoiceService) setRecurringPaused(id string, paused bool, today string) (model.RecurringInvoice, error) {
	s.recurringMu.Lock()
	defer s.recurringMu.Unlock()
	plan, err := s.store.RecurringInvoice(id)
	if err != nil {
		return plan, err
	}
	plan.Paused = paused
	for !paused && plan.NextDate < today {
		if plan, err = plan.Advance(); err != nil {
			return plan, err
		}
	}
	if plan, err = s.store.SaveRecurringInvoice(plan); err != nil {
		return plan, err
	}
	return plan.Decorate()
}

func (s *InvoiceService) DeleteRecurringInvoice(id string) error {
	s.recurringMu.Lock()
	defer s.recurringMu.Unlock()
	return s.store.DeleteRecurringInvoice(id)
}

// RunRecurringInvoices creates the drafts that are due. A recurring invoice
// that fails never stops the others.
func (s *InvoiceService) RunRecurringInvoices() (model.RecurringRun, error) {
	return s.runRecurring(time.Now().Format(time.DateOnly))
}

func (s *InvoiceService) runRecurring(today string) (model.RecurringRun, error) {
	s.recurringMu.Lock()
	defer s.recurringMu.Unlock()
	run := model.RecurringRun{Failed: []string{}}
	plans, err := s.store.RecurringInvoices()
	if err != nil {
		return run, err
	}
	for _, plan := range plans {
		for n := 0; n < recurringCatchUp && plan.Due(today); n++ {
			next, err := s.createRecurringDraft(plan, today)
			if errors.Is(err, store.ErrRecurringChanged) {
				break
			}
			if err != nil {
				run.Failed = append(run.Failed, fmt.Sprintf("%s: %s", plan.Name, err))
				break
			}
			plan = next
			run.Created++
		}
	}
	return run, nil
}

func (s *InvoiceService) createRecurringDraft(plan model.RecurringInvoice, today string) (model.RecurringInvoice, error) {
	customer, err := s.store.Customer(plan.Template.CustomerID)
	if err != nil {
		return plan, err
	}
	draft, err := plan.Draft(today, customer)
	if err != nil {
		return plan, err
	}
	view, err := s.view(draft, false)
	if err != nil {
		return plan, err
	}
	next, err := plan.Advance()
	if err != nil {
		return plan, err
	}
	return next, s.store.AddRecurringDraft(next, plan.NextDate, view.Draft, view.Totals.Gross)
}
