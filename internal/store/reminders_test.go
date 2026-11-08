package store

import (
	"bytes"
	"database/sql"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"papierleicht.de/e-rechnung-studio/internal/model"
)

func reminderStore(t *testing.T, due string) (*Store, string) {
	t.Helper()
	s, id := paymentStore(t)
	if _, e := s.db.Exec("UPDATE invoices SET data=CAST(json_set(data,'$.dueDate',?) AS BLOB) WHERE id=?", due, id); e != nil {
		t.Fatal(e)
	}
	return s, id
}

func record(t *testing.T, s *Store, p ReminderProposal) Reminder {
	t.Helper()
	r, e := s.RecordReminder(Reminder{InvoiceID: p.InvoiceID, Level: p.Level, Title: p.Title, Currency: p.Currency, Amount: p.Amount,
		Fee: p.Fee, Interest: p.Interest, Total: p.Total, DueSince: p.DueSince, PaymentDue: p.PaymentDue, Channel: ReminderChannelManual}, []byte("%PDF-reminder"), p.Today)
	if e != nil {
		t.Fatal(e)
	}
	return r
}

func TestReminderLevelsWaitingTimesAndHintAmounts(t *testing.T) {
	s, id := reminderStore(t, "2026-09-30")
	p, e := s.InvoiceReminderProposal(id, "2026-09-30")
	if e != nil || p.Eligible || !strings.Contains(p.Reason, "noch nicht fällig") {
		t.Fatal("not yet due invoice proposed", p, e)
	}
	// Overdue, but within the seven days of the friendly reminder.
	p, _ = s.InvoiceReminderProposal(id, "2026-10-03")
	if !p.Eligible || p.Due || p.Level != 1 || p.NextDate != "2026-10-07" || p.Amount != "1000.00" || p.Title != "Zahlungserinnerung" {
		t.Fatal(p)
	}
	cfg := DefaultReminderSettings()
	cfg.Levels[1].Fee = "5,00"
	cfg.InterestRate = "9.12"
	if _, e = s.SaveReminderSettings(cfg); e != nil {
		t.Fatal(e)
	}
	p, _ = s.InvoiceReminderProposal(id, "2026-10-07")
	if !p.Due || p.PaymentDue != "2026-10-14" || p.DaysOverdue != 7 || p.Interest != "1.75" || p.Total != "1001.75" {
		t.Fatal(p)
	}
	first := record(t, s, p)
	if first.Number == "" || first.DocumentSHA256 == "" || first.Date != "2026-10-07" {
		t.Fatal(first)
	}
	// The next level waits for the deadline plus its own waiting period.
	p, _ = s.InvoiceReminderProposal(id, "2026-10-15")
	if p.Level != 2 || p.Due || p.NextDate != "2026-10-17" || p.Fee != "5.00" || p.Title != "1. Mahnung" || p.Last == nil || p.Last.ID != first.ID {
		t.Fatal(p)
	}
	p, _ = s.InvoiceReminderProposal(id, "2026-10-17")
	if !p.Due {
		t.Fatal("second level not due", p)
	}
	record(t, s, p)
	p, _ = s.InvoiceReminderProposal(id, "2026-11-01")
	if p.Level != 3 || !p.Due || p.Title != "2. Mahnung" {
		t.Fatal(p)
	}
	record(t, s, p)
	p, _ = s.InvoiceReminderProposal(id, "2026-12-01")
	if !p.Complete || p.Due || !p.Eligible || p.Level != 3 {
		t.Fatal("no further escalation expected", p)
	}
	due, e := s.ReminderProposals("2026-12-01")
	if e != nil || len(due) != 1 || due[0].Due {
		t.Fatal(due, e)
	}
	for _, bad := range []func(*ReminderSettings){
		func(v *ReminderSettings) { v.Levels = v.Levels[:2] },
		func(v *ReminderSettings) { v.Levels[0].Text = " " },
		func(v *ReminderSettings) { v.Levels[2].PaymentDays = 0 },
		func(v *ReminderSettings) { v.Levels[1].Fee = "-1" },
		func(v *ReminderSettings) { v.Levels[1].Fee = "1.234" },
		func(v *ReminderSettings) { v.InterestRate = "31" },
	} {
		v := DefaultReminderSettings()
		bad(&v)
		if _, e = s.SaveReminderSettings(v); e == nil {
			t.Fatal("invalid settings accepted", v)
		}
	}
}

func TestReminderUsesRemainingBalanceAfterPartialPayment(t *testing.T) {
	s, id := reminderStore(t, "2026-09-01")
	if _, e := s.AddPayment(Payment{InvoiceID: id, Direction: "outgoing", Amount: "400", Currency: "EUR", Date: "2026-09-10", Source: "manual", Reason: "Teilzahlung"}); e != nil {
		t.Fatal(e)
	}
	p, _ := s.InvoiceReminderProposal(id, "2026-10-26")
	if !p.Eligible || p.Amount != "600.00" || p.Remaining != "600.00" || p.DueSince != "2026-09-01" {
		t.Fatal(p)
	}
	// A payment arriving after the proposal makes the prepared letter stale.
	if _, e := s.AddPayment(Payment{InvoiceID: id, Direction: "outgoing", Amount: "100", Currency: "EUR", Date: "2026-10-26", Source: "manual", Reason: "Teilzahlung"}); e != nil {
		t.Fatal(e)
	}
	if _, e := s.RecordReminder(Reminder{InvoiceID: id, Level: 1, Currency: "EUR", Amount: p.Amount, DueSince: p.DueSince, Channel: ReminderChannelManual}, []byte("pdf"), "2026-10-26"); e == nil {
		t.Fatal("stale reminder recorded")
	}
	if e := s.SetPaid(id, true); e != nil {
		t.Fatal(e)
	}
	p, _ = s.InvoiceReminderProposal(id, "2026-10-26")
	if p.Eligible || !strings.Contains(p.Reason, "bezahlt") {
		t.Fatal("paid invoice proposed", p)
	}
	if _, e := s.RecordReminder(Reminder{InvoiceID: id, Level: 1, Currency: "EUR", Amount: "500.00", DueSince: "2026-09-01", Channel: ReminderChannelManual}, []byte("pdf"), "2026-10-26"); e == nil {
		t.Fatal("reminder for paid invoice recorded")
	}
	all, _ := s.ReminderProposals("2026-10-26")
	for _, v := range all {
		if v.Due {
			t.Fatal("paid invoice listed", v)
		}
	}
}

func TestReminderWithInstallmentPlanUsesDueRatesOnly(t *testing.T) {
	s, id := reminderStore(t, "2026-01-01")
	plan := InstallmentPlan{InvoiceID: id, Active: true, Rates: []Installment{{Date: "2026-09-15", Amount: "300.00"}, {Date: "2026-10-15", Amount: "300.00"}, {Date: "2026-11-15", Amount: "400.00"}}}
	if e := s.SaveInstallmentPlan(plan); e != nil {
		t.Fatal(e)
	}
	// Plan-conform: the first rate is not yet due despite the original due date.
	p, _ := s.InvoiceReminderProposal(id, "2026-09-10")
	if p.Eligible || !strings.Contains(p.Reason, "plangemäß") {
		t.Fatal(p)
	}
	if _, e := s.AddPayment(Payment{InvoiceID: id, Direction: "outgoing", Amount: "300", Currency: "EUR", Date: "2026-09-15", Source: "manual", Reason: "Rate 1"}); e != nil {
		t.Fatal(e)
	}
	p, _ = s.InvoiceReminderProposal(id, "2026-10-01")
	if p.Eligible {
		t.Fatal("paid rate reminded", p)
	}
	// The second rate is overdue: remind exactly that rate, not the whole remainder.
	p, _ = s.InvoiceReminderProposal(id, "2026-10-26")
	if !p.Eligible || !p.Installment || p.Amount != "300.00" || p.Remaining != "700.00" || p.DueSince != "2026-10-15" || p.Level != 1 {
		t.Fatal(p)
	}
	r := record(t, s, p)
	if !r.Installment {
		t.Fatal(r)
	}
	// Both remaining rates overdue: the cycle of the oldest open rate continues.
	p, _ = s.InvoiceReminderProposal(id, "2026-11-20")
	if p.Amount != "700.00" || p.DueSince != "2026-10-15" || p.Level != 2 {
		t.Fatal(p)
	}
	// Once rate 2 is paid, the late rate 3 starts a new, friendly cycle.
	if _, e := s.AddPayment(Payment{InvoiceID: id, Direction: "outgoing", Amount: "300", Currency: "EUR", Date: "2026-11-20", Source: "manual", Reason: "Rate 2"}); e != nil {
		t.Fatal(e)
	}
	p, _ = s.InvoiceReminderProposal(id, "2026-11-20")
	if p.Amount != "400.00" || p.DueSince != "2026-11-15" || p.Level != 1 {
		t.Fatal(p)
	}
}

func TestReminderHistoryIsImmutable(t *testing.T) {
	s, id := reminderStore(t, "2026-09-01")
	p, _ := s.InvoiceReminderProposal(id, "2026-10-26")
	r := record(t, s, p)
	if _, e := s.db.Exec("UPDATE invoice_reminders SET level=3 WHERE id=?", r.ID); e == nil {
		t.Fatal("history updated")
	}
	if _, e := s.db.Exec("DELETE FROM invoice_reminders WHERE id=?", r.ID); e == nil {
		t.Fatal("history deleted")
	}
	if _, e := s.RecordReminder(Reminder{InvoiceID: id, Level: 1, Currency: "EUR", Amount: p.Amount, DueSince: p.DueSince, Channel: "fax"}, []byte("pdf"), "2026-10-26"); e == nil {
		t.Fatal("unknown channel accepted")
	}
	history, e := s.Reminders(id)
	if e != nil || len(history) != 1 || !reflect.DeepEqual(history[0], r) {
		t.Fatal(history, e)
	}
	stored, pdf, e := s.ReminderPDF(r.ID)
	if e != nil || stored.ID != r.ID || string(pdf) != "%PDF-reminder" {
		t.Fatal(stored, e)
	}
}

func TestReminderBackupRoundTrip(t *testing.T) {
	s, id := reminderStore(t, "2026-09-01")
	cfg := DefaultReminderSettings()
	cfg.Levels[0].Title = "Freundliche Erinnerung"
	if _, e := s.SaveReminderSettings(cfg); e != nil {
		t.Fatal(e)
	}
	p, _ := s.InvoiceReminderProposal(id, "2026-10-26")
	r := record(t, s, p)
	data, e := s.ExportBackup()
	if e != nil || !strings.Contains(data, `"reminders"`) {
		t.Fatal(e)
	}
	other, e := Open(filepath.Join(t.TempDir(), "restore.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer other.Close()
	for i := 0; i < 2; i++ { // A second restore replaces, never duplicates, history.
		if e = other.RestoreBackup(data); e != nil {
			t.Fatal(e)
		}
	}
	history, e := other.Reminders(id)
	if e != nil || len(history) != 1 || !reflect.DeepEqual(history[0], r) {
		t.Fatal(history, e)
	}
	_, pdf, e := other.ReminderPDF(r.ID)
	if e != nil || !bytes.Equal(pdf, []byte("%PDF-reminder")) {
		t.Fatal(e)
	}
	restored, _ := other.ReminderSettings()
	if restored.Levels[0].Title != "Freundliche Erinnerung" {
		t.Fatal(restored)
	}
	if _, e = other.db.Exec("DELETE FROM invoice_reminders"); e == nil {
		t.Fatal("restored history is not protected")
	}
	p, _ = other.InvoiceReminderProposal(id, "2026-11-30")
	if p.Level != 2 {
		t.Fatal("restored history not used", p)
	}
	// Older backups without reminders restore with an empty history.
	if e = other.RestoreBackup(strings.Replace(data, `"reminders"`, `"ignored"`, 1)); e != nil {
		t.Fatal(e)
	}
	if history, _ = other.Reminders(""); len(history) != 0 {
		t.Fatal(history)
	}
}

func TestReminderMigrationOfExistingDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	db, e := sql.Open("sqlite", path)
	if e != nil {
		t.Fatal(e)
	}
	// Schema of databases created before reminders existed.
	if _, e = db.Exec(`
CREATE TABLE settings (key TEXT PRIMARY KEY, value BLOB NOT NULL);
CREATE TABLE customers (id TEXT PRIMARY KEY, name TEXT NOT NULL, data BLOB NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL);
CREATE TABLE products (id TEXT PRIMARY KEY, name TEXT NOT NULL, data BLOB NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL);
CREATE TABLE invoices (id TEXT PRIMARY KEY, number TEXT NOT NULL DEFAULT '', customer_name TEXT NOT NULL DEFAULT '', status TEXT NOT NULL, issue_date TEXT NOT NULL DEFAULT '', gross TEXT NOT NULL DEFAULT '0.00', data BLOB NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL);
CREATE TABLE artifacts (id INTEGER PRIMARY KEY AUTOINCREMENT, invoice_id TEXT NOT NULL REFERENCES invoices(id), kind TEXT NOT NULL, data BLOB NOT NULL, created_at TEXT NOT NULL);
CREATE TABLE counters (name TEXT PRIMARY KEY, value INTEGER NOT NULL);
CREATE TABLE received_invoices (id TEXT PRIMARY KEY, data BLOB NOT NULL, original BLOB NOT NULL);
INSERT INTO invoices VALUES('inv-legacy','RE-2026-0001','Altkunde','issued','2026-08-01','500.00','{"id":"inv-legacy","number":"RE-2026-0001","kind":"invoice","status":"issued","currency":"EUR","dueDate":"2026-08-15","customer":{"name":"Altkunde"}}','2026-08-01','2026-08-01');`); e != nil {
		t.Fatal(e)
	}
	db.Close()
	s, e := Open(path)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	cfg, e := s.ReminderSettings()
	if e != nil || !reflect.DeepEqual(cfg, DefaultReminderSettings()) {
		t.Fatal("defaults missing after migration", cfg, e)
	}
	p, e := s.InvoiceReminderProposal("inv-legacy", "2026-10-26")
	if e != nil || !p.Eligible || p.Amount != "500.00" || !p.Due {
		t.Fatal(p, e)
	}
	record(t, s, p)
	s.Close()
	// Reopening an already migrated database keeps the history.
	s, e = Open(path)
	if e != nil {
		t.Fatal(e)
	}
	// The earlier defer holds the first store. Windows cannot remove the
	// temporary folder while this one is still open.
	defer s.Close()
	if history, _ := s.Reminders("inv-legacy"); len(history) != 1 {
		t.Fatal(history)
	}
}

func TestCreditNotesAreNeverReminded(t *testing.T) {
	s, e := Open(filepath.Join(t.TempDir(), "test.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	d := model.NewDraft()
	d.Kind = "credit-note"
	d.DueDate = "2026-01-01"
	d, e = s.SaveInvoice(d, "100.00")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.Issue(d.ID, d, "100.00", func(model.InvoiceDraft) ([]byte, []byte, error) { return []byte("xml"), []byte("pdf"), nil }); e != nil {
		t.Fatal(e)
	}
	p, e := s.InvoiceReminderProposal(d.ID, "2026-10-26")
	if e != nil || p.Eligible || p.Due {
		t.Fatal(p, e)
	}
}
