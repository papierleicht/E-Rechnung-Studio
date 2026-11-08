package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"papierleicht.de/e-rechnung-studio/internal/engine"
	"papierleicht.de/e-rechnung-studio/internal/model"
	"papierleicht.de/e-rechnung-studio/internal/pdfinvoice"
	validator "papierleicht.de/e-rechnung-studio/internal/validation"
)

// Quotes reuse the invoice engine for positions, taxes and totals. The XML
// built here is a private rendering input; it is never stored or exported.

func (s *InvoiceService) ListQuotes() ([]model.QuoteSummary, error) {
	quotes, err := s.store.Quotes()
	today := time.Now().Format(time.DateOnly)
	for i := range quotes {
		quotes[i].Status = model.EffectiveQuoteStatus(quotes[i].Status, quotes[i].ValidUntil, today)
	}
	return quotes, err
}

func (s *InvoiceService) PreviewQuote(q model.Quote) (model.QuoteView, error) {
	return s.quoteView(q)
}

func (s *InvoiceService) SaveQuote(q model.Quote) (model.QuoteView, error) {
	view, err := s.quoteView(q)
	if err != nil {
		return view, err
	}
	saved, err := s.store.SaveQuote(view.Quote, view.Totals.Gross)
	if err != nil {
		return view, err
	}
	view.Quote = saved
	return view, nil
}

func (s *InvoiceService) OpenQuote(id string) (model.QuoteView, error) {
	q, err := s.store.Quote(id)
	if err != nil {
		return model.QuoteView{}, err
	}
	return s.quoteView(q)
}

func (s *InvoiceService) DeleteQuote(id string) error { return s.store.DeleteQuote(id) }

// MarkQuoteSent saves the draft, reserves its number and keeps the issuer
// snapshot, so later PDFs show the quote as it was sent.
func (s *InvoiceService) MarkQuoteSent(q model.Quote) (model.QuoteView, error) {
	view, err := s.SaveQuote(q)
	if err != nil {
		return view, err
	}
	if view.Validation.State != "valid" {
		return view, errors.New("Das Angebot ist noch unvollständig. Behebe zuerst die angezeigten Hinweise.")
	}
	company, err := s.store.Company()
	if err != nil {
		return view, err
	}
	sent := view.Quote
	sent.Document.IssuerSnapshot = &company
	if _, err = s.store.MarkQuoteSent(sent, view.Totals.Gross); err != nil {
		return view, err
	}
	return s.OpenQuote(sent.ID)
}

// SetQuoteStatus records acceptance or rejection, or reopens a quote for editing.
func (s *InvoiceService) SetQuoteStatus(id, status string) (model.QuoteView, error) {
	if _, err := s.store.ChangeQuoteStatus(id, status); err != nil {
		return model.QuoteView{}, err
	}
	return s.OpenQuote(id)
}

// ConvertQuoteToInvoice creates an invoice draft with all positions and a
// visible reference to the quote. The invoice is issued in the usual editor.
func (s *InvoiceService) ConvertQuoteToInvoice(id string) (model.InvoiceView, error) {
	q, err := s.store.Quote(id)
	if err != nil {
		return model.InvoiceView{}, err
	}
	if q.Status != "sent" && q.Status != "accepted" {
		return model.InvoiceView{}, errors.New("Du kannst nur versendete oder angenommene Angebote in eine Rechnung umwandeln.")
	}
	today := time.Now().Format(time.DateOnly)
	d := q.Document
	d.ID, d.Number, d.AutomaticNumber, d.Kind, d.Status = "", "", true, "invoice", "draft"
	d.IssuerSnapshot = nil
	if d.Profile == "" {
		d.Profile, d.Syntax = "facturx-en16931", "cii"
	}
	d.CorrectsInvoiceID, d.PrecedingInvoiceNumber, d.PrecedingInvoiceDate = "", "", ""
	d.IssueDate, d.DeliveryDate, d.PaymentReference = today, today, ""
	days := 14
	if d.Customer.PaymentTermDays != nil {
		days = *d.Customer.PaymentTermDays
	}
	d.DueDate = time.Now().AddDate(0, 0, days).Format(time.DateOnly)
	d.Notes = fmt.Sprintf("Gemäß Angebot %s vom %s.", q.Number, germanDate(q.Document.IssueDate))
	d.Lines = append([]model.InvoiceLine(nil), q.Document.Lines...)
	d.Adjustments = append([]model.InvoiceAdjustment(nil), q.Document.Adjustments...)
	view, err := s.view(d, false)
	if err != nil {
		return view, err
	}
	saved, err := s.store.ConvertQuote(id, view.Draft, view.Totals.Gross)
	if err != nil {
		return view, err
	}
	view.Draft = saved
	return view, nil
}

// QuoteDocumentHTML renders the preview without saving anything.
func (s *InvoiceService) QuoteDocumentHTML(q model.Quote) (string, error) {
	q, err := s.store.RefreshQuoteNumber(normalizeQuote(q))
	if err != nil {
		return "", err
	}
	if q.ID != "" {
		stored, err := s.store.Quote(q.ID)
		if err != nil {
			return "", err
		}
		if stored.Status != "draft" {
			q = stored // Sent quotes always show their stored content.
		}
	}
	xml, d, c, err := s.quoteXML(q, true)
	if err != nil {
		return "", err
	}
	return pdfinvoice.QuoteHTML(xml, &c, &d.Customer, quoteDetails(q))
}

// ExportQuotePDF writes the PDF of a sent quote atomically after file selection.
func (s *InvoiceService) ExportQuotePDF(id, path string) (string, error) {
	if !strings.EqualFold(filepath.Ext(path), ".pdf") {
		return "", errors.New("Speichere das Angebot bitte mit der Dateiendung .pdf.")
	}
	data, _, err := s.quotePDF(id)
	if err != nil {
		return "", err
	}
	return writeExport(path, data, "PDF")
}

// OpenQuotePDF stores the PDF in the export folder and opens it.
func (s *InvoiceService) OpenQuotePDF(id string) (string, error) {
	data, q, err := s.quotePDF(id)
	if err != nil {
		return "", err
	}
	settings, err := s.GetAppSettings()
	if err != nil {
		return "", err
	}
	if err = os.MkdirAll(settings.ExportDirectory, 0o700); err != nil {
		return "", fmt.Errorf("Der Exportordner konnte nicht angelegt werden. Prüfe den Pfad: %w", err)
	}
	path, err := exportArtifact(settings.ExportDirectory, "Angebot-"+exportFilename(q.Number), ".pdf", "", data)
	if err != nil {
		return "", err
	}
	app := application.Get()
	if app == nil {
		return "", errors.New("Die PDF kannst du nur in der App öffnen.")
	}
	if err = app.Browser.OpenFile(path); err != nil {
		return "", fmt.Errorf("Die PDF konnte nicht geöffnet werden: %w", err)
	}
	return path, nil
}

func (s *InvoiceService) quotePDF(id string) ([]byte, model.Quote, error) {
	q, err := s.store.Quote(id)
	if err != nil {
		return nil, q, err
	}
	if q.Status == "draft" {
		return nil, q, errors.New("Schließe das Angebot zuerst ab. Dabei vergibt die App die Angebotsnummer.")
	}
	xml, d, c, err := s.quoteXML(q, false)
	if err != nil {
		return nil, q, err
	}
	data, err := pdfinvoice.RenderQuote(xml, &c, &d.Customer, quoteDetails(q))
	if err != nil {
		return nil, q, fmt.Errorf("Die Angebots-PDF konnte nicht erstellt werden: %w", err)
	}
	return data, q, nil
}

func (s *InvoiceService) quoteView(q model.Quote) (model.QuoteView, error) {
	q, err := s.store.RefreshQuoteNumber(normalizeQuote(q))
	if err != nil {
		return model.QuoteView{}, err
	}
	c, err := s.quoteIssuer(q)
	if err != nil {
		return model.QuoteView{}, err
	}
	view := model.QuoteView{Quote: q, Company: c, Validation: model.ValidationResult{State: "incomplete", Issues: []model.ValidationIssue{}}}
	if q.InvoiceID != "" {
		if view.InvoiceNumber, err = s.store.QuoteInvoiceNumber(q.InvoiceID); err != nil {
			return view, err
		}
	}
	view.Quote.Status = model.EffectiveQuoteStatus(q.Status, q.ValidUntil, time.Now().Format(time.DateOnly))
	d := quoteInvoiceDraft(q)
	issues := validator.InputIssues(d, c)
	if _, err := time.Parse(time.DateOnly, q.ValidUntil); err != nil {
		issues = append(issues, model.ValidationIssue{Stage: "input", RuleID: "INPUT", Severity: "error", Field: "validUntil", Message: "Wähle ein gültiges Datum für „Gültig bis“."})
	} else if q.ValidUntil < q.Document.IssueDate {
		issues = append(issues, model.ValidationIssue{Stage: "input", RuleID: "INPUT", Severity: "error", Field: "validUntil", Message: "„Gültig bis“ darf nicht vor dem Angebotsdatum liegen."})
	}
	inv, err := s.engine.Build(d, c)
	if err != nil {
		view.Validation.Issues = issues
		if len(issues) == 0 {
			view.Validation = validator.German(model.ValidationResult{State: "incomplete", Issues: []model.ValidationIssue{{Stage: "input", RuleID: "INPUT", Severity: "error", Message: err.Error()}}})
		}
		return view, nil
	}
	view.Totals, view.LineTotals = engine.Totals(inv), engine.LineTotals(inv)
	view.Validation.Issues = issues
	view.Validation.State = "valid"
	if len(issues) > 0 {
		view.Validation.State = "invalid"
	}
	return view, nil
}

// Sent quotes keep the issuer snapshot taken when they were sent.
func (s *InvoiceService) quoteIssuer(q model.Quote) (model.Company, error) {
	if q.Status != "draft" && q.Document.IssuerSnapshot != nil {
		return *q.Document.IssuerSnapshot, nil
	}
	return s.store.Company()
}

// quoteXML builds the rendering input. Drafts get the same display-only
// placeholders as invoice previews so an incomplete quote can be shown.
func (s *InvoiceService) quoteXML(q model.Quote, placeholders bool) ([]byte, model.InvoiceDraft, model.Company, error) {
	c, err := s.quoteIssuer(q)
	if err != nil {
		return nil, model.InvoiceDraft{}, c, err
	}
	d := quoteInvoiceDraft(q)
	if placeholders {
		d.Lines = append([]model.InvoiceLine(nil), d.Lines...)
		for i := range d.Lines {
			if strings.TrimSpace(d.Lines[i].Name) == "" {
				d.Lines[i].Name = "Position hinzufügen …"
			}
			if strings.TrimSpace(d.Lines[i].UnitPrice) == "" {
				d.Lines[i].UnitPrice = "0"
			}
		}
	}
	inv, err := s.engine.Build(d, c)
	if err != nil {
		return nil, d, c, fmt.Errorf("Die Angebotsansicht ist noch nicht möglich: %w", err)
	}
	xml, err := s.engine.XML(inv)
	return xml, d, c, err
}

func normalizeQuote(q model.Quote) model.Quote {
	q.ValidUntil = strings.TrimSpace(q.ValidUntil)
	q.Document.Kind, q.Document.Status = "invoice", "draft"
	if q.Status == "" {
		q.Status = "draft"
	}
	return q
}

// quoteInvoiceDraft maps the quote onto the invoice engine. The fixed CII
// profile only serves the renderer; the customer's invoice format stays in
// the document and applies when the quote becomes an invoice.
func quoteInvoiceDraft(q model.Quote) model.InvoiceDraft {
	d := q.Document
	d.ID, d.Number, d.AutomaticNumber, d.Kind, d.Status = "", q.Number, false, "invoice", "draft"
	if d.Number == "" {
		d.Number = "ANGEBOT"
	}
	d.Profile, d.Syntax = "facturx-en16931", "cii"
	d.DueDate, d.PaymentReference = "", ""
	d.CorrectsInvoiceID, d.PrecedingInvoiceNumber, d.PrecedingInvoiceDate = "", "", ""
	d.DisablePaymentQR = true
	return d
}

func quoteDetails(q model.Quote) pdfinvoice.QuoteDetails {
	validUntil, _ := time.Parse(time.DateOnly, q.ValidUntil)
	return pdfinvoice.QuoteDetails{ValidUntil: validUntil}
}

func germanDate(value string) string {
	if date, err := time.Parse(time.DateOnly, value); err == nil {
		return date.Format("02.01.2006")
	}
	return value
}
