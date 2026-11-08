package app

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"papierleicht.de/e-rechnung-studio/internal/model"
	"papierleicht.de/e-rechnung-studio/internal/pdfinvoice"
	"papierleicht.de/e-rechnung-studio/internal/store"
)

// An issued invoice may only disappear while nobody has it: not sent, not
// paid, not reminded, not corrected and not part of a transmitted return.
// Even then it stays in the log of deleted invoices with its files and
// reason, and its number is never issued again (GoBD: traceability). An
// invoice that reached someone is cancelled with a credit note instead.

// InvoiceDeletion tells the UI whether an issued invoice can be deleted.
type InvoiceDeletion struct {
	Allowed  bool     `json:"allowed"`
	Number   string   `json:"number"`
	Kind     string   `json:"kind"`
	Blockers []string `json:"blockers"`
	// Files are the automatic exports removed together with the invoice.
	Files []string `json:"files"`
}

// DeletedInvoiceResult reports the log entry and the export copies.
type DeletedInvoiceResult struct {
	Invoice      store.DeletedInvoice `json:"invoice"`
	RemovedFiles []string             `json:"removedFiles"`
	// KeptFiles changed after the export and are left for the user.
	KeptFiles []string `json:"keptFiles"`
}

func germanDay(value string) string {
	if t, err := time.Parse(time.RFC3339, value); err == nil {
		return t.Local().Format("02.01.2006")
	}
	return germanDate(value)
}

// filedReturns names transmitted returns that cover the invoice.
func (s *InvoiceService) filedReturns(d model.InvoiceDraft) ([]string, error) {
	dates := []string{firstNonEmpty(d.DeliveryDate, d.PeriodEnd, d.IssueDate), d.IssueDate}
	out := []string{}
	for _, kind := range []struct{ key, name string }{
		{store.FilingVAT, "Umsatzsteuer-Voranmeldung"},
		{store.FilingZM, "Zusammenfassende Meldung"},
		{store.FilingAnnual, "Jahreserklärung"},
	} {
		filings, err := s.store.Filings(kind.key)
		if err != nil {
			return nil, err
		}
		for id, f := range filings {
			from, to, label, ok := store.FilingPeriod(id)
			if ok && (inPeriod(dates[0], from, to) || inPeriod(dates[1], from, to)) {
				out = append(out, fmt.Sprintf("Sie gehört zur %s %s, die du am %s als übermittelt vermerkt hast.", kind.name, label, germanDate(f.SubmittedOn)))
			}
		}
	}
	closed, err := s.store.ClosedByHand(d.IssuedAt, dates...)
	if err != nil {
		return nil, err
	}
	if closed != "" {
		out = append(out, closed)
	}
	return out, nil
}

func deletionBlockers(links store.InvoiceLinks, filed []string) []string {
	out := []string{}
	if links.MailedAt != "" {
		out = append(out, fmt.Sprintf("Du hast sie am %s per E-Mail verschickt.", germanDay(links.MailedAt)))
	}
	if links.MarkedSentAt != "" {
		out = append(out, fmt.Sprintf("Du hast sie am %s als verschickt markiert.", germanDay(links.MarkedSentAt)))
	}
	if links.Status == "paid" || links.Payments > 0 {
		out = append(out, "Für sie sind Zahlungen erfasst.")
	}
	if links.Installments {
		out = append(out, "Für sie ist eine Ratenzahlung vereinbart.")
	}
	if links.Reminders > 0 {
		out = append(out, "Es gibt schon Zahlungserinnerungen oder Mahnungen dazu.")
	}
	if len(links.Corrections) > 0 {
		out = append(out, fmt.Sprintf("Es gibt eine Korrektur dazu (%s).", strings.Join(links.Corrections, ", ")))
	}
	return append(out, filed...)
}

func (s *InvoiceService) InvoiceDeletionCheck(id string) (InvoiceDeletion, error) {
	links, d, err := s.store.InvoiceLinks(id)
	if err != nil {
		return InvoiceDeletion{}, err
	}
	result := InvoiceDeletion{Number: d.Number, Kind: d.Kind, Blockers: []string{}, Files: []string{}}
	if links.Status == "draft" {
		result.Blockers = append(result.Blockers, "Das ist ein Entwurf. Lösche ihn über „Entwurf löschen“.")
		return result, nil
	}
	filed, err := s.filedReturns(d)
	if err != nil {
		return result, err
	}
	result.Blockers = deletionBlockers(links, filed)
	result.Allowed = len(result.Blockers) == 0
	export, err := s.store.InvoiceExport(id)
	if err != nil {
		return result, err
	}
	for _, path := range []string{export.PDFPath, export.XMLPath} {
		if _, statErr := os.Stat(path); path != "" && statErr == nil {
			result.Files = append(result.Files, path)
		}
	}
	return result, nil
}

// DeleteIssuedInvoice moves an unsent invoice into the log of deleted
// invoices and removes its unchanged automatic export.
func (s *InvoiceService) DeleteIssuedInvoice(id, reason string) (DeletedInvoiceResult, error) {
	s.issueMu.Lock()
	defer s.issueMu.Unlock()
	s.exportMu.Lock()
	defer s.exportMu.Unlock()
	result := DeletedInvoiceResult{RemovedFiles: []string{}, KeptFiles: []string{}}
	_, current, err := s.store.InvoiceLinks(id)
	if err != nil {
		return result, err
	}
	filed, err := s.filedReturns(current)
	if err != nil {
		return result, err
	}
	entry, export, err := s.store.DeleteIssuedInvoice(id, reason, func(_ model.InvoiceDraft, links store.InvoiceLinks) error {
		if blockers := deletionBlockers(links, filed); len(blockers) > 0 {
			return fmt.Errorf("Die Rechnung kann nicht gelöscht werden. %s Hebe sie mit einer Stornorechnung auf.", strings.Join(blockers, " "))
		}
		return nil
	})
	if err != nil {
		return result, err
	}
	result.Invoice = entry
	_, _, xml, pdf, err := s.store.DeletedInvoice(id)
	if err != nil {
		return result, nil
	}
	for _, file := range []struct {
		path string
		data []byte
	}{{export.PDFPath, pdf}, {export.XMLPath, xml}} {
		if file.path == "" {
			continue
		}
		existing, readErr := os.ReadFile(file.path)
		if readErr != nil {
			continue
		}
		// Only our own unchanged copy goes; the log keeps the original.
		if bytes.Equal(existing, file.data) && os.Remove(file.path) == nil {
			result.RemovedFiles = append(result.RemovedFiles, file.path)
		} else {
			result.KeptFiles = append(result.KeptFiles, file.path)
		}
	}
	return result, nil
}

func (s *InvoiceService) DeletedInvoices() ([]store.DeletedInvoice, error) {
	return s.store.DeletedInvoices()
}

// DeletedInvoiceHTML shows a deleted invoice exactly as it was issued.
func (s *InvoiceService) DeletedInvoiceHTML(id string) (string, error) {
	_, d, xml, _, err := s.store.DeletedInvoice(id)
	if err != nil {
		return "", err
	}
	return pdfinvoice.HTML(xml, d.IssuerSnapshot, &d.Customer, pdfinvoice.PaymentQROptions{Enabled: !d.DisablePaymentQR, HideDetails: d.HidePaymentQRDetails})
}

// DraftFromDeletedInvoice starts a new draft with the content of a deleted
// invoice, dated today and with a new number from the same series.
func (s *InvoiceService) DraftFromDeletedInvoice(id string) (model.InvoiceView, error) {
	_, d, _, _, err := s.store.DeletedInvoice(id)
	if err != nil {
		return model.InvoiceView{}, err
	}
	today := time.Now()
	if issue, e1 := time.Parse(time.DateOnly, d.IssueDate); e1 == nil {
		if due, e2 := time.Parse(time.DateOnly, d.DueDate); e2 == nil && !due.Before(issue) {
			d.DueDate = today.AddDate(0, 0, int(due.Sub(issue).Hours()/24)).Format(time.DateOnly)
		}
	}
	if d.PaymentReference == d.Number {
		d.PaymentReference = ""
	}
	d.ID, d.Number, d.AutomaticNumber, d.Status = "", "", true, "draft"
	d.IssueDate = today.Format(time.DateOnly)
	d.CreatedAt, d.UpdatedAt, d.IssuedAt, d.PaidAt, d.MarkedSentAt = "", "", "", "", ""
	d.OriginalXML, d.GeneratedXML, d.GeneratedAt, d.LastValidationState = "", "", "", ""
	d.IssuerSnapshot = nil
	d.RecurringID, d.RecurringName = "", ""
	return s.SaveInvoice(d)
}

// MarkInvoiceSent notes that an issued invoice left the app another way.
func (s *InvoiceService) MarkInvoiceSent(id string, sent bool) error {
	if id == "" {
		return errors.New("Die Rechnung wurde nicht gefunden.")
	}
	return s.store.MarkSent(id, sent)
}
