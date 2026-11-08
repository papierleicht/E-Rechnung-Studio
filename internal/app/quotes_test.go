package app

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"papierleicht.de/e-rechnung-studio/internal/model"
)

func testQuote() model.Quote {
	d := testDraft()
	d.Number, d.DueDate, d.PaymentReference = "", "", ""
	d.Lines = append(d.Lines, model.InvoiceLine{ID: "2", Name: "Material", Quantity: "1", Unit: "C62", UnitPrice: "50.00", TaxRate: "19", TaxCategory: "S"})
	d.Adjustments = []model.InvoiceAdjustment{{ID: "a", Amount: "10.00", Reason: "Treuerabatt", TaxCategory: "S", TaxRate: "19"}}
	return model.Quote{Document: d, ValidUntil: "2099-12-31"}
}

func TestQuoteConversionCreatesLinkedInvoiceDraft(t *testing.T) {
	s, err := NewInvoiceService(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err = s.SaveCompany(testCompany()); err != nil {
		t.Fatal(err)
	}
	draft, err := s.SaveQuote(testQuote())
	if err != nil {
		t.Fatal(err)
	}
	// Totals come from the invoice engine: 2×100 + 50 − 10 = 240 net.
	if draft.Totals.Net != "240.00" || draft.Totals.Gross != "285.60" || draft.Validation.State != "valid" {
		t.Fatalf("quote totals: %+v %+v", draft.Totals, draft.Validation)
	}
	if _, err = s.ConvertQuoteToInvoice(draft.Quote.ID); err == nil {
		t.Fatal("draft quote converted")
	}
	sent, err := s.MarkQuoteSent(draft.Quote)
	if err != nil {
		t.Fatal(err)
	}
	if sent.Quote.Status != "sent" || sent.Quote.Number != "AN-2026-0001" {
		t.Fatalf("sent quote: %+v", sent.Quote)
	}
	if _, err = s.SaveQuote(sent.Quote); err == nil {
		t.Fatal("sent quote edited without reopening")
	}
	invoice, err := s.ConvertQuoteToInvoice(sent.Quote.ID)
	if err != nil {
		t.Fatal(err)
	}
	d := invoice.Draft
	if d.ID == "" || d.Status != "draft" || d.QuoteID != sent.Quote.ID || d.QuoteNumber != "AN-2026-0001" || !strings.Contains(d.Notes, "Angebot AN-2026-0001 vom 09.09.2026") {
		t.Fatalf("invoice reference: %+v", d)
	}
	if len(d.Lines) != 2 || len(d.Adjustments) != 1 || d.Customer.Name != "Beispielkunde AG" || invoice.Totals.Gross != "285.60" {
		t.Fatalf("invoice content: %+v %+v", d, invoice.Totals)
	}
	today := time.Now().Format(time.DateOnly)
	if d.IssueDate != today || d.DueDate <= today || d.Number != "RE-"+today[:4]+"-0001" {
		t.Fatalf("invoice dates/number: %s %s %s", d.IssueDate, d.DueDate, d.Number)
	}
	if invoice.Validation.State != "valid" {
		t.Fatalf("converted draft is not issuable: %+v", invoice.Validation)
	}
	opened, err := s.OpenQuote(sent.Quote.ID)
	if err != nil || opened.Quote.Status != "converted" || opened.Quote.InvoiceID != d.ID || opened.InvoiceNumber != d.Number {
		t.Fatalf("quote link: %+v %v", opened, err)
	}
	if _, err = s.ConvertQuoteToInvoice(sent.Quote.ID); err == nil {
		t.Fatal("quote converted twice")
	}
	issued, err := s.IssueInvoice(d)
	if err != nil || issued.Draft.Status != "issued" {
		t.Fatalf("issue converted invoice: %+v %v", issued.Validation, err)
	}
}

func TestQuoteStatusAndExpiry(t *testing.T) {
	s, err := NewInvoiceService(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err = s.SaveCompany(testCompany()); err != nil {
		t.Fatal(err)
	}
	q := testQuote()
	q.ValidUntil = "2026-09-01"
	if view, _ := s.PreviewQuote(q); view.Validation.State != "invalid" {
		t.Fatal("validity before quote date accepted")
	}
	// Like issuing, a failed send keeps the saved draft for correction.
	failed, err := s.MarkQuoteSent(q)
	if err == nil || failed.Quote.Status != "draft" || failed.Quote.ID == "" {
		t.Fatalf("invalid quote sent: %+v %v", failed.Quote, err)
	}
	q = failed.Quote
	q.ValidUntil = "2026-09-30" // in the past relative to the test run
	sent, err := s.MarkQuoteSent(q)
	if err != nil {
		t.Fatal(err)
	}
	if sent.Quote.Status != "expired" {
		t.Fatalf("expired quote shown as %s", sent.Quote.Status)
	}
	list, err := s.ListQuotes()
	if err != nil || len(list) != 1 || list[0].Status != "expired" {
		t.Fatalf("list: %+v %v", list, err)
	}
	rejected, err := s.SetQuoteStatus(sent.Quote.ID, "rejected")
	if err != nil || rejected.Quote.Status != "rejected" || rejected.Quote.DecidedAt == "" {
		t.Fatalf("reject: %+v %v", rejected.Quote, err)
	}
	if _, err = s.ConvertQuoteToInvoice(sent.Quote.ID); err == nil {
		t.Fatal("rejected quote converted")
	}
	if _, err = s.SetQuoteStatus(sent.Quote.ID, "unknown"); err == nil {
		t.Fatal("unknown status accepted")
	}
	reopened, err := s.SetQuoteStatus(sent.Quote.ID, "draft")
	if err != nil || reopened.Quote.Status != "draft" || reopened.Quote.Number != sent.Quote.Number {
		t.Fatalf("reopen: %+v %v", reopened.Quote, err)
	}
	reopened.Quote.Document.Notes = "Überarbeitet"
	if _, err = s.SaveQuote(reopened.Quote); err != nil {
		t.Fatal(err)
	}
}

func TestQuotePDFIsNoEInvoice(t *testing.T) {
	s, err := NewInvoiceService(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	logo, err := s.PrepareCompanyLogo(testLogoUpload(t))
	if err != nil {
		t.Fatal(err)
	}
	withLogo := testCompany()
	withLogo.Logo = logo.Logo
	if _, err = s.SaveCompany(withLogo); err != nil {
		t.Fatal(err)
	}
	draft, err := s.SaveQuote(testQuote())
	if err != nil {
		t.Fatal(err)
	}
	html, err := s.QuoteDocumentHTML(draft.Quote)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Angebot AN-2026-0001", "Angebotsdatum", "Gültig bis", "31.12.2099", "Treuerabatt", `class="invoice-logo"`} {
		if !strings.Contains(html, want) {
			t.Fatalf("quote HTML lacks %q", want)
		}
	}
	for _, unwanted := range []string{"Rechnungsdatum", "Fällig am", "Zahlungsreferenz", "SEPA-Überweisung per QR-Code"} {
		if strings.Contains(html, unwanted) {
			t.Fatalf("quote HTML contains invoice detail %q", unwanted)
		}
	}
	path := filepath.Join(t.TempDir(), "angebot.pdf")
	if _, err = s.ExportQuotePDF(draft.Quote.ID, path); err == nil {
		t.Fatal("draft quote exported before a number was reserved")
	}
	sent, err := s.MarkQuoteSent(draft.Quote)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.ExportQuotePDF(sent.Quote.ID, path); err != nil {
		t.Fatal(err)
	}
	pdf, err := os.ReadFile(path)
	if err != nil || !bytes.HasPrefix(pdf, []byte("%PDF")) {
		t.Fatalf("quote PDF: %v", err)
	}
	if bytes.Contains(pdf, []byte("factur-x.xml")) || bytes.Contains(pdf, []byte("xrechnung.xml")) || bytes.Contains(pdf, []byte("Factur-X PDFA Extension")) {
		t.Fatal("quote PDF announces an e-invoice")
	}
	// Later company changes do not alter a sent quote.
	company := testCompany()
	company.Name = "Neue Firma GmbH"
	if _, err = s.SaveCompany(company); err != nil {
		t.Fatal(err)
	}
	html, err = s.QuoteDocumentHTML(sent.Quote)
	if err != nil || !strings.Contains(html, "Muster GmbH") || strings.Contains(html, "Neue Firma GmbH") {
		t.Fatalf("sent quote lost its issuer snapshot: %v", err)
	}
}

func TestApplicationBackupIncludesQuotes(t *testing.T) {
	source, err := NewInvoiceService(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	if _, err = source.SaveCompany(testCompany()); err != nil {
		t.Fatal(err)
	}
	saved, err := source.SaveQuote(testQuote())
	if err != nil {
		t.Fatal(err)
	}
	sent, err := source.MarkQuoteSent(saved.Quote)
	if err != nil {
		t.Fatal(err)
	}
	invoice, err := source.ConvertQuoteToInvoice(sent.Quote.ID)
	if err != nil {
		t.Fatal(err)
	}
	backup, err := source.ExportBackup()
	if err != nil {
		t.Fatal(err)
	}
	target, err := NewInvoiceService(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer target.Close()
	if err = target.RestoreBackup(backup); err != nil {
		t.Fatal(err)
	}
	restored, err := target.OpenQuote(sent.Quote.ID)
	if err != nil || restored.Quote.Status != "converted" || restored.Quote.InvoiceID != invoice.Draft.ID || restored.Totals.Gross != "285.60" {
		t.Fatalf("restored quote: %+v %v", restored, err)
	}
	restoredInvoice, err := target.OpenInvoice(invoice.Draft.ID)
	if err != nil || restoredInvoice.Draft.QuoteNumber != sent.Quote.Number {
		t.Fatalf("restored invoice reference: %+v %v", restoredInvoice.Draft, err)
	}
}
