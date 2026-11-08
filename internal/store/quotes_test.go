package store

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	"papierleicht.de/e-rechnung-studio/internal/model"
)

func testQuote() model.Quote {
	d := model.NewDraft()
	d.IssueDate = "2026-09-15"
	d.Customer = model.Customer{Name: "Kunde GmbH", Address: model.Address{Country: "DE"}}
	return model.Quote{Document: d, ValidUntil: "2026-10-15"}
}

func TestQuoteNumberSequence(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	refresh := func(q model.Quote) model.Quote {
		t.Helper()
		q, e := s.RefreshQuoteNumber(q)
		if e != nil {
			t.Fatal(e)
		}
		return q
	}
	send := func(q model.Quote) model.Quote {
		t.Helper()
		q, e := s.SaveQuote(refresh(q), "100.00")
		if e != nil {
			t.Fatal(e)
		}
		if q, e = s.MarkQuoteSent(q, "100.00"); e != nil {
			t.Fatal(e)
		}
		return q
	}
	a, b := refresh(testQuote()), refresh(testQuote())
	if a.Number != "AN-2026-0001" || !a.AutomaticNumber || b.Number != a.Number {
		t.Fatalf("bad previews: %+v / %+v", a, b)
	}
	// Saving a draft never reserves a number.
	if _, err = s.SaveQuote(a, "100.00"); err != nil {
		t.Fatal(err)
	}
	if refresh(testQuote()).Number != "AN-2026-0001" {
		t.Fatal("draft reserved a quote number")
	}
	first := send(testQuote())
	second := send(b) // stale preview is renumbered when sent
	if first.Number != "AN-2026-0001" || second.Number != "AN-2026-0002" || first.AutomaticNumber {
		t.Fatalf("sent numbers: %s / %s", first.Number, second.Number)
	}
	// Quotes and invoices use separate number ranges.
	invoice := model.NewDraft()
	invoice.IssueDate = "2026-09-15"
	if invoice, err = s.RefreshDraftNumber(invoice); err != nil || invoice.Number != "RE-2026-0001" {
		t.Fatalf("invoice range affected: %s %v", invoice.Number, err)
	}
	next := testQuote()
	next.Document.IssueDate = "2027-01-02"
	if refresh(next).Number != "AN-2027-0001" {
		t.Fatal("quote numbers do not restart per year")
	}
	// Reopening keeps the reserved number, and a manual number is respected.
	reopened, err := s.ChangeQuoteStatus(first.ID, "draft")
	if err != nil || reopened.Number != first.Number || reopened.AutomaticNumber {
		t.Fatalf("reopen: %+v %v", reopened, err)
	}
	if refresh(reopened).Number != first.Number {
		t.Fatal("reopened quote was renumbered")
	}
	if err = s.DeleteQuote(second.ID); err == nil {
		t.Fatal("sent quote deleted")
	}
	if err = s.DeleteQuote(reopened.ID); err != nil {
		t.Fatal(err)
	}
	if refresh(testQuote()).Number != "AN-2026-0003" {
		t.Fatal("deleted quote number reused")
	}
}

func TestQuoteConversionLinksOneInvoice(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	q, err := s.SaveQuote(testQuote(), "119.00")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.ConvertQuote(q.ID, model.NewDraft(), "119.00"); err == nil {
		t.Fatal("draft quote converted")
	}
	if q, err = s.MarkQuoteSent(q, "119.00"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.ChangeQuoteStatus(q.ID, "accepted"); err != nil {
		t.Fatal(err)
	}
	invoice, err := s.ConvertQuote(q.ID, model.NewDraft(), "119.00")
	if err != nil || invoice.QuoteID != q.ID || invoice.QuoteNumber != q.Number || invoice.Status != "draft" {
		t.Fatalf("conversion: %+v %v", invoice, err)
	}
	if _, err = s.ConvertQuote(q.ID, model.NewDraft(), "119.00"); err == nil {
		t.Fatal("quote converted twice")
	}
	if _, err = s.ChangeQuoteStatus(q.ID, "rejected"); err == nil {
		t.Fatal("converted quote changed")
	}
	list, err := s.Quotes()
	if err != nil || len(list) != 1 || list[0].Status != "converted" || list[0].InvoiceID != invoice.ID {
		t.Fatalf("list: %+v %v", list, err)
	}
	// Deleting the generated draft releases the quote for a new conversion.
	if err = s.DeleteDraft(invoice.ID); err != nil {
		t.Fatal(err)
	}
	released, err := s.Quote(q.ID)
	if err != nil || released.Status != "accepted" || released.InvoiceID != "" {
		t.Fatalf("released: %+v %v", released, err)
	}
	if list, _ = s.Quotes(); list[0].Status != "accepted" {
		t.Fatalf("list after delete: %+v", list)
	}
	if _, err = s.ConvertQuote(q.ID, model.NewDraft(), "119.00"); err != nil {
		t.Fatal(err)
	}
}

// Databases created before quotes existed gain the table on open.
func TestQuoteMigrationKeepsExistingData(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`
CREATE TABLE settings (key TEXT PRIMARY KEY, value BLOB NOT NULL);
CREATE TABLE invoices (id TEXT PRIMARY KEY, number TEXT NOT NULL DEFAULT '', customer_name TEXT NOT NULL DEFAULT '', status TEXT NOT NULL, issue_date TEXT NOT NULL DEFAULT '', gross TEXT NOT NULL DEFAULT '0.00', data BLOB NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL);
CREATE TABLE counters (name TEXT PRIMARY KEY, value INTEGER NOT NULL);
INSERT INTO invoices VALUES('inv-old','RE-2026-0007','Alt','issued','2026-01-02','10.00','{"id":"inv-old","number":"RE-2026-0007","status":"issued"}','2026-01-02','2026-01-02');
INSERT INTO counters VALUES('invoice-2026',7);
PRAGMA user_version=1;`)
	db.Close()
	if err != nil {
		t.Fatal(err)
	}
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { s.Close() }()
	invoices, err := s.Invoices()
	if err != nil || len(invoices) != 1 || invoices[0].Number != "RE-2026-0007" {
		t.Fatalf("existing invoices: %+v %v", invoices, err)
	}
	quotes, err := s.Quotes()
	if err != nil || len(quotes) != 0 {
		t.Fatalf("quotes after migration: %+v %v", quotes, err)
	}
	q, err := s.SaveQuote(testQuote(), "1.00")
	if err != nil {
		t.Fatal(err)
	}
	if q, err = s.MarkQuoteSent(q, "1.00"); err != nil || q.Number != "AN-2026-0001" {
		t.Fatalf("quote after migration: %s %v", q.Number, err)
	}
	// Reopening the migrated database is idempotent.
	s.Close()
	if s, err = Open(path); err != nil {
		t.Fatal(err)
	}
}

func TestQuoteBackupRoundTrip(t *testing.T) {
	source, err := Open(filepath.Join(t.TempDir(), "source.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	q, err := source.SaveQuote(testQuote(), "119.00")
	if err != nil {
		t.Fatal(err)
	}
	if q, err = source.MarkQuoteSent(q, "119.00"); err != nil {
		t.Fatal(err)
	}
	draft, err := source.SaveQuote(testQuote(), "5.00")
	if err != nil {
		t.Fatal(err)
	}
	backup, err := source.ExportBackup()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(backup, `"version": 9`) || !strings.Contains(backup, `"quotes"`) {
		t.Fatal("backup does not contain quotes")
	}
	target, err := Open(filepath.Join(t.TempDir(), "target.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer target.Close()
	if _, err = target.SaveQuote(testQuote(), "1.00"); err != nil {
		t.Fatal(err)
	}
	if err = target.RestoreBackup(backup); err != nil {
		t.Fatal(err)
	}
	list, err := target.Quotes()
	if err != nil || len(list) != 2 {
		t.Fatalf("restored quotes: %+v %v", list, err)
	}
	restored, err := target.Quote(q.ID)
	if err != nil || restored.Number != q.Number || restored.Status != "sent" || restored.ValidUntil != q.ValidUntil || restored.Document.Customer.Name != "Kunde GmbH" {
		t.Fatalf("restored quote: %+v %v", restored, err)
	}
	if _, err = target.Quote(draft.ID); err != nil {
		t.Fatal(err)
	}
	// The counter is restored too, so the next quote continues the range.
	next, err := target.RefreshQuoteNumber(testQuote())
	if err != nil || next.Number != "AN-2026-0002" {
		t.Fatalf("next number after restore: %s %v", next.Number, err)
	}
	// Older backups have no quotes and replace the current ones completely.
	old := strings.Replace(backup, `"version": 9`, `"version": 3`, 1)
	old = old[:strings.Index(old, `"quotes"`)]
	old = strings.TrimRight(old, " \n,") + "\n}"
	if err = target.RestoreBackup(old); err != nil {
		t.Fatal(err)
	}
	if list, err = target.Quotes(); err != nil || len(list) != 0 {
		t.Fatalf("quotes after old backup: %+v %v", list, err)
	}
}
