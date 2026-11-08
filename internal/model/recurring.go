package model

import (
	"errors"
	"time"
)

// Rhythms of a recurring invoice, with their length in months.
var recurringMonths = map[string]int{"monthly": 1, "quarterly": 3, "half-yearly": 6, "yearly": 12}

// States of a recurring invoice, derived on reading.
const (
	RecurringActive = "active"
	RecurringPaused = "paused"
	RecurringEnded  = "ended"
)

// RecurringInvoice repeats an issued invoice. Each due date creates a draft
// for the user to check; nothing is ever issued or sent on its own.
type RecurringInvoice struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Interval string `json:"interval"`
	// NextDate is the day the next draft is due, PeriodStart the first day of
	// the period it bills. Billing in advance or in arrears only differs in
	// how the two relate, so the user sets both.
	NextDate    string `json:"nextDate"`
	PeriodStart string `json:"periodStart"`
	// The days of the month the plan was set up on. They survive short months:
	// the 31.01. continues with 28.02. and then 31.03.
	InvoiceDay int `json:"invoiceDay"`
	PeriodDay  int `json:"periodDay"`
	// EndDate is the last day a draft may be due, empty for no end.
	EndDate string `json:"endDate"`
	Paused  bool   `json:"paused"`
	// Template holds what repeats. Its IssueDate and DueDate only keep the
	// payment term.
	Template        InvoiceDraft `json:"template"`
	SourceInvoiceID string       `json:"sourceInvoiceId"`
	Gross           string       `json:"gross"`
	CreatedAt       string       `json:"createdAt"`
	UpdatedAt       string       `json:"updatedAt"`
	// Derived on reading, never relied on from storage.
	Status     string `json:"status"`
	PeriodEnd  string `json:"periodEnd"`
	OpenDrafts int    `json:"openDrafts"`
}

// RecurringRun reports what a run over the due recurring invoices did.
type RecurringRun struct {
	Created int `json:"created"`
	// Failed names the recurring invoices without a new draft, with the reason.
	Failed []string `json:"failed"`
}

// ValidRecurringInterval tells whether the rhythm exists.
func ValidRecurringInterval(interval string) bool { return recurringMonths[interval] > 0 }

func parseDay(date string) (time.Time, error) { return time.Parse(time.DateOnly, date) }

// addMonths moves a date by whole months to the given day of the month, or to
// the last day of a shorter month.
func addMonths(date time.Time, months, day int) time.Time {
	first := time.Date(date.Year(), date.Month()+time.Month(months), 1, 0, 0, 0, 0, time.UTC)
	return first.AddDate(0, 0, min(day, first.AddDate(0, 1, -1).Day())-1)
}

// Decorate fills the derived fields.
func (r RecurringInvoice) Decorate() (RecurringInvoice, error) {
	start, err := parseDay(r.PeriodStart)
	if err != nil {
		return r, err
	}
	r.PeriodEnd = addMonths(start, recurringMonths[r.Interval], r.PeriodDay).AddDate(0, 0, -1).Format(time.DateOnly)
	switch {
	case r.EndDate != "" && r.NextDate > r.EndDate:
		r.Status = RecurringEnded
	case r.Paused:
		r.Status = RecurringPaused
	default:
		r.Status = RecurringActive
	}
	return r, nil
}

// Due tells whether a draft is waiting to be created.
func (r RecurringInvoice) Due(today string) bool {
	return !r.Paused && r.NextDate <= today && (r.EndDate == "" || r.NextDate <= r.EndDate)
}

// Advance moves to the following due date and period.
func (r RecurringInvoice) Advance() (RecurringInvoice, error) {
	next, err := parseDay(r.NextDate)
	if err != nil {
		return r, err
	}
	period, err := parseDay(r.PeriodStart)
	if err != nil {
		return r, err
	}
	months := recurringMonths[r.Interval]
	if months == 0 {
		return r, errors.New("Der Rhythmus der Dauerrechnung ist unbekannt.")
	}
	r.NextDate = addMonths(next, months, r.InvoiceDay).Format(time.DateOnly)
	r.PeriodStart = addMonths(period, months, r.PeriodDay).Format(time.DateOnly)
	return r, nil
}

// Draft builds the invoice for the current period. The customer is the stored
// one, so a new address arrives; nil keeps the customer of the template.
func (r RecurringInvoice) Draft(today string, customer *Customer) (InvoiceDraft, error) {
	r, err := r.Decorate()
	if err != nil {
		return InvoiceDraft{}, err
	}
	d := r.Template
	d.Lines = append([]InvoiceLine(nil), r.Template.Lines...)
	d.Adjustments = append([]InvoiceAdjustment(nil), r.Template.Adjustments...)
	if customer != nil {
		d.Customer = *customer
	}
	d.Status, d.IssueDate = "draft", today
	d.PeriodStart, d.PeriodEnd = r.PeriodStart, r.PeriodEnd
	// With a period the date of supply is its last day, see InvoiceIssues.
	d.DeliveryDate = r.PeriodEnd
	d.RecurringID, d.RecurringName = r.ID, r.Name
	if d.DueDate != "" {
		issued, err := parseDay(today)
		if err != nil {
			return InvoiceDraft{}, err
		}
		d.DueDate = issued.AddDate(0, 0, r.paymentDays(d.Customer)).Format(time.DateOnly)
	}
	return d, nil
}

// paymentDays keeps the term of the template, or falls back like a new invoice.
func (r RecurringInvoice) paymentDays(c Customer) int {
	issued, issuedErr := parseDay(r.Template.IssueDate)
	due, dueErr := parseDay(r.Template.DueDate)
	if issuedErr == nil && dueErr == nil && !due.Before(issued) {
		return int(due.Sub(issued).Hours() / 24)
	}
	if c.PaymentTermDays != nil {
		return *c.PaymentTermDays
	}
	return 14
}

// RecurringTemplate keeps what repeats from an invoice and drops its identity
// and accounting history, like invoiceFromTemplate in the frontend.
func RecurringTemplate(source InvoiceDraft) InvoiceDraft {
	d := source
	d.ID, d.Number, d.Kind, d.Status = "", "", "invoice", "draft"
	d.AutomaticNumber = true
	d.PeriodStart, d.PeriodEnd, d.DeliveryDate = "", "", ""
	d.PaymentReference = ""
	d.CorrectsInvoiceID, d.PrecedingInvoiceNumber, d.PrecedingInvoiceDate, d.CorrectionReason = "", "", "", ""
	d.AdvancePayment = false
	d.PrepaidAmount, d.SourceTaxes = "", nil
	d.QuoteID, d.QuoteNumber = "", ""
	d.RecurringID, d.RecurringName = "", ""
	d.IssuerSnapshot = nil
	d.MarkedSentAt = ""
	d.CreatedAt, d.UpdatedAt, d.IssuedAt, d.PaidAt = "", "", "", ""
	d.OriginalXML, d.GeneratedXML, d.GeneratedAt, d.LastValidationState = "", "", "", ""
	d.Lines = append([]InvoiceLine(nil), source.Lines...)
	d.Adjustments = append([]InvoiceAdjustment(nil), source.Adjustments...)
	return d
}
