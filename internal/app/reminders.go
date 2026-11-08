package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/shopspring/decimal"
	"papierleicht.de/e-rechnung-studio/internal/model"
	"papierleicht.de/e-rechnung-studio/internal/pdfinvoice"
	"papierleicht.de/e-rechnung-studio/internal/store"
)

// Serializes creating and recording reminders, independent of other services.
var reminderMu sync.Mutex

// InvoiceReminderState is shown with an issued customer invoice.
type InvoiceReminderState struct {
	Proposal store.ReminderProposal `json:"proposal"`
	History  []store.Reminder       `json:"history"`
	// Preview of the subject and text for the proposed level.
	Subject string `json:"subject"`
	Text    string `json:"text"`
}

// PreparedReminder is a complete, not yet recorded reminder: PDF, subject and
// body text. A sending integration attaches PDF, sends Subject/Body and then
// calls RecordReminder with its own channel. Nothing is recorded before that.
type PreparedReminder struct {
	Proposal  store.ReminderProposal `json:"proposal"`
	Level     int                    `json:"level"`
	Title     string                 `json:"title"`
	Subject   string                 `json:"subject"`
	Body      string                 `json:"body"`
	FileName  string                 `json:"fileName"`
	Recipient string                 `json:"recipient"`
	Date      string                 `json:"date"`
	PDF       []byte                 `json:"-"`
}

func reminderToday() string { return time.Now().Format(time.DateOnly) }

func (s *InvoiceService) ReminderSettings() (store.ReminderSettings, error) {
	return s.store.ReminderSettings()
}

func (s *InvoiceService) SaveReminderSettings(v store.ReminderSettings) (store.ReminderSettings, error) {
	return s.store.SaveReminderSettings(v)
}

// DefaultReminderSettings returns the built-in texts and deadlines for "reset".
func (s *InvoiceService) DefaultReminderSettings() store.ReminderSettings {
	return store.DefaultReminderSettings()
}

// InvoiceReminders returns the proposal and immutable history of one invoice.
func (s *InvoiceService) InvoiceReminders(id string) (InvoiceReminderState, error) {
	return s.invoiceReminders(id, reminderToday())
}

func (s *InvoiceService) invoiceReminders(id, today string) (InvoiceReminderState, error) {
	p, err := s.store.InvoiceReminderProposal(id, today)
	if err != nil {
		return InvoiceReminderState{}, err
	}
	history, err := s.store.Reminders(id)
	if err != nil {
		return InvoiceReminderState{}, err
	}
	state := InvoiceReminderState{Proposal: p, History: history}
	if p.Eligible {
		prepared, e := s.composeReminder(id, p.Level, p, false)
		if e != nil {
			return state, e
		}
		state.Subject, state.Text = prepared.Subject, prepared.Body
	}
	return state, nil
}

// DueReminders lists invoices whose next reminder is due today, oldest first.
func (s *InvoiceService) DueReminders() ([]store.ReminderProposal, error) {
	return s.dueReminders(reminderToday())
}

func (s *InvoiceService) dueReminders(today string) ([]store.ReminderProposal, error) {
	all, err := s.store.ReminderProposals(today)
	if err != nil {
		return nil, err
	}
	out := []store.ReminderProposal{}
	for _, p := range all {
		if p.Due {
			out = append(out, p)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].NextDate < out[j].NextDate })
	return out, nil
}

// PrepareReminder builds the reminder for one invoice without recording it.
// Level 0 uses the proposed level. This is the hand-off point for sending.
//
//wails:ignore
func (s *InvoiceService) PrepareReminder(id string, level int) (PreparedReminder, error) {
	return s.prepareReminder(id, level, reminderToday())
}

func (s *InvoiceService) prepareReminder(id string, level int, today string) (PreparedReminder, error) {
	p, err := s.store.InvoiceReminderProposal(id, today)
	if err != nil {
		return PreparedReminder{}, err
	}
	if !p.Eligible {
		return PreparedReminder{}, errors.New(p.Reason)
	}
	if level == 0 {
		level = p.Level
	}
	return s.composeReminder(id, level, p, true)
}

// RecordReminder appends a prepared reminder to the immutable history after
// it was handed over through the given channel (store.ReminderChannel*).
//
//wails:ignore
func (s *InvoiceService) RecordReminder(prepared PreparedReminder, channel string) (store.Reminder, error) {
	reminderMu.Lock()
	defer reminderMu.Unlock()
	return s.recordReminder(prepared, channel, reminderToday())
}

func (s *InvoiceService) recordReminder(prepared PreparedReminder, channel, today string) (store.Reminder, error) {
	if prepared.Date != today {
		return store.Reminder{}, errors.New("Die Erinnerung wurde an einem anderen Tag vorbereitet. Erstelle sie noch einmal neu.")
	}
	p := prepared.Proposal
	return s.store.RecordReminder(store.Reminder{
		InvoiceID: p.InvoiceID, Level: prepared.Level, Title: prepared.Title, Currency: p.Currency,
		Amount: p.Amount, Fee: p.Fee, Interest: p.Interest, Total: p.Total, DueSince: p.DueSince,
		PaymentDue: p.PaymentDue, Channel: channel, Subject: prepared.Subject, Text: prepared.Body, FileName: prepared.FileName,
	}, prepared.PDF, today)
}

// ExportReminderPDF creates the reminder, saves it at path and records it as
// manually exported. If recording fails, the written file is removed again.
func (s *InvoiceService) ExportReminderPDF(id string, level int, path string) (store.Reminder, error) {
	return s.exportReminderPDF(id, level, path, reminderToday())
}

func (s *InvoiceService) exportReminderPDF(id string, level int, path, today string) (store.Reminder, error) {
	if !strings.EqualFold(filepath.Ext(path), ".pdf") {
		return store.Reminder{}, errors.New("Speichere die Erinnerung bitte mit der Dateiendung .pdf.")
	}
	reminderMu.Lock()
	defer reminderMu.Unlock()
	prepared, err := s.prepareReminder(id, level, today)
	if err != nil {
		return store.Reminder{}, err
	}
	prepared.FileName = filepath.Base(path)
	if _, err = writeExport(path, prepared.PDF, "Erinnerung"); err != nil {
		return store.Reminder{}, err
	}
	r, err := s.recordReminder(prepared, store.ReminderChannelManual, today)
	if err != nil {
		os.Remove(path)
		return r, err
	}
	return r, nil
}

// ExportStoredReminderPDF saves an already recorded reminder again, unchanged.
func (s *InvoiceService) ExportStoredReminderPDF(reminderID, path string) (string, error) {
	if !strings.EqualFold(filepath.Ext(path), ".pdf") {
		return "", errors.New("Speichere die Erinnerung bitte mit der Dateiendung .pdf.")
	}
	_, data, err := s.store.ReminderPDF(reminderID)
	if err != nil {
		return "", err
	}
	return writeExport(path, data, "Erinnerung")
}

var reminderPlaceholder = regexp.MustCompile(`\{[a-zäöü]+\}`)
var fileNameUnsafe = regexp.MustCompile(`[^\p{L}\p{N}]+`)

func reminderFileName(title, number string) string {
	name := strings.Trim(fileNameUnsafe.ReplaceAllString(strings.ToLower(title)+"-"+number, "-"), "-")
	if name == "" {
		name = "erinnerung"
	}
	return name + ".pdf"
}

func reminderDate(v string) string {
	t, err := time.Parse(time.DateOnly, v)
	if err != nil {
		return v
	}
	return t.Format("02.01.2006")
}

func reminderMoney(v, currency string) string {
	d, err := decimal.NewFromString(v)
	if err != nil {
		return v + " " + currency
	}
	scale, _ := model.CurrencyScale(currency)
	text := d.StringFixed(scale)
	whole, fraction, _ := strings.Cut(strings.TrimPrefix(text, "-"), ".")
	for i := len(whole) - 3; i > 0; i -= 3 {
		whole = whole[:i] + "." + whole[i:]
	}
	if fraction != "" {
		whole += "," + fraction
	}
	if d.IsNegative() {
		whole = "-" + whole
	}
	return whole + " " + currency
}

// composeReminder fills the configured texts and optionally renders the PDF.
func (s *InvoiceService) composeReminder(id string, level int, p store.ReminderProposal, render bool) (PreparedReminder, error) {
	if level < 1 || level > store.ReminderLevels {
		return PreparedReminder{}, errors.New("Diese Erinnerungsstufe gibt es nicht.")
	}
	cfg, err := s.store.ReminderSettings()
	if err != nil {
		return PreparedReminder{}, err
	}
	d, err := s.store.Invoice(id)
	if err != nil {
		return PreparedReminder{}, err
	}
	issuer := d.IssuerSnapshot
	if issuer == nil {
		c, e := s.store.Company()
		if e != nil {
			return PreparedReminder{}, e
		}
		issuer = &c
	}
	if level != p.Level {
		// A different level than proposed changes fee and deadline, not the amount.
		p = store.ProposeReminder(store.ReminderBasis{Balance: store.PaymentBalance{ID: p.InvoiceID, Number: p.Number, Partner: p.Partner, Currency: p.Currency, Remaining: p.Remaining},
			Eligible: true, Amount: p.Amount, DueSince: p.DueSince, Installment: p.Installment}, levelHistory(p, level), cfg, p.Today)
	}
	l := cfg.Levels[level-1]
	values := map[string]string{
		"{kunde}": d.Customer.Name, "{firma}": issuer.Name, "{rechnungsnummer}": d.Number,
		"{rechnungsdatum}": reminderDate(d.IssueDate), "{faelligkeit}": reminderDate(p.DueSince), "{fälligkeit}": reminderDate(p.DueSince),
		"{betrag}": reminderMoney(p.Total, p.Currency), "{offen}": reminderMoney(p.Amount, p.Currency),
		"{frist}": reminderDate(p.PaymentDue), "{stufe}": l.Title,
	}
	fill := func(text string) string {
		return reminderPlaceholder.ReplaceAllStringFunc(text, func(key string) string {
			if v, ok := values[key]; ok {
				return v
			}
			return key
		})
	}
	date := p.Today
	out := PreparedReminder{Proposal: p, Level: level, Title: l.Title, Subject: fill(l.Subject), Body: fill(l.Text),
		Recipient: d.Customer.Email, Date: date,
		FileName: reminderFileName(l.Title, d.Number)}
	if !render {
		return out, nil
	}
	xml, err := s.store.Artifact(id, "cii-xml")
	if err != nil {
		return out, fmt.Errorf("Die Rechnungsdaten für die Erinnerung fehlen: %w", err)
	}
	parse := func(v string) decimal.Decimal { r, _ := decimal.NewFromString(v); return r }
	dates := func(v string) time.Time { t, _ := time.Parse(time.DateOnly, v); return t }
	out.PDF, err = pdfinvoice.RenderReminder(xml, issuer, &d.Customer, pdfinvoice.ReminderLetter{
		Title: l.Title, Text: out.Body, Date: dates(date), DueSince: dates(p.DueSince), PaymentDue: dates(p.PaymentDue),
		Currency: p.Currency, Amount: parse(p.Amount), Fee: parse(p.Fee), Interest: parse(p.Interest), Total: parse(p.Total),
		InterestRate: strings.ReplaceAll(cfg.InterestRate, ".", ","), DaysOverdue: p.DaysOverdue, Installment: p.Installment,
		PaymentQR: !d.DisablePaymentQR,
	})
	if err != nil {
		return out, fmt.Errorf("Die Erinnerung konnte nicht als PDF erstellt werden: %w", err)
	}
	return out, nil
}

// levelHistory fakes the cycle length so ProposeReminder computes the chosen level.
func levelHistory(p store.ReminderProposal, level int) []store.Reminder {
	out := make([]store.Reminder, level-1)
	for i := range out {
		out[i] = store.Reminder{InvoiceID: p.InvoiceID, DueSince: p.DueSince}
	}
	return out
}
