package app

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"papierleicht.de/e-rechnung-studio/internal/store"
)

func TestReminderPDFExportAndSendingHandOff(t *testing.T) {
	s, err := NewInvoiceService(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err = s.SaveCompany(testCompany()); err != nil {
		t.Fatal(err)
	}
	d := testDraft()
	d.Number = "RE-MAHN-001"
	issued, err := s.IssueInvoice(d) // 238,00 EUR, due 2026-09-23
	if err != nil {
		t.Fatal(err)
	}
	id := issued.Draft.ID
	if _, err = s.RecordPayment(store.Payment{InvoiceID: id, Direction: "outgoing", Amount: "38.00", Currency: "EUR", Date: "2026-09-20", Source: "manual", Reason: "Teilzahlung"}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.prepareReminder(id, 0, "2026-09-23"); err == nil {
		t.Fatal("reminder prepared before the due date passed")
	}
	state, err := s.invoiceReminders(id, "2026-10-26")
	if err != nil {
		t.Fatal(err)
	}
	if !state.Proposal.Eligible || state.Proposal.Amount != "200.00" || state.Subject != "Zahlungserinnerung zur Rechnung RE-MAHN-001" ||
		!strings.Contains(state.Text, "Offen sind noch 200,00 EUR, fällig seit 23.09.2026") || !strings.Contains(state.Text, "bis zum 02.11.2026") || !strings.Contains(state.Text, "Muster GmbH") {
		t.Fatalf("unexpected reminder state: %+v", state)
	}
	due, err := s.dueReminders("2026-10-26")
	if err != nil || len(due) != 1 || due[0].InvoiceID != id {
		t.Fatal(due, err)
	}

	// Sending integrations get the full document without anything being recorded.
	prepared, err := s.prepareReminder(id, 0, "2026-10-26")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(prepared.PDF, []byte("%PDF-")) || prepared.Recipient != "kunde@example.eu" || prepared.FileName != "zahlungserinnerung-RE-MAHN-001.pdf" || prepared.Level != 1 {
		t.Fatalf("unexpected prepared reminder: %q %q %d %q", prepared.Recipient, prepared.FileName, prepared.Level, prepared.PDF[:min(8, len(prepared.PDF))])
	}
	if history, _ := s.store.Reminders(id); len(history) != 0 {
		t.Fatal("preparing recorded a reminder")
	}
	if _, err = s.recordReminder(prepared, store.ReminderChannelEmail, "2026-10-27"); err == nil {
		t.Fatal("reminder prepared on another day recorded")
	}

	if _, err = s.exportReminderPDF(id, 0, filepath.Join(t.TempDir(), "erinnerung.txt"), "2026-10-26"); err == nil {
		t.Fatal("wrong extension accepted")
	}
	path := filepath.Join(t.TempDir(), "erinnerung.pdf")
	r, err := s.exportReminderPDF(id, 0, path, "2026-10-26")
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil || !bytes.HasPrefix(data, []byte("%PDF-")) {
		t.Fatal("reminder PDF not written", err)
	}
	if r.Channel != store.ReminderChannelManual || r.Level != 1 || r.Amount != "200.00" || r.Number != "RE-MAHN-001" || r.FileName != "erinnerung.pdf" || r.Subject != state.Subject {
		t.Fatalf("unexpected history entry: %+v", r)
	}
	again := filepath.Join(t.TempDir(), "kopie.pdf")
	if _, err = s.ExportStoredReminderPDF(r.ID, again); err != nil {
		t.Fatal(err)
	}
	if copyData, _ := os.ReadFile(again); !bytes.Equal(copyData, data) {
		t.Fatal("stored reminder differs from exported document")
	}

	// A chosen higher level keeps the amount and uses that level's texts and fee.
	cfg := store.DefaultReminderSettings()
	cfg.Levels[2].Fee = "5"
	if _, err = s.SaveReminderSettings(cfg); err != nil {
		t.Fatal(err)
	}
	prepared, err = s.prepareReminder(id, 3, "2026-10-28")
	if err != nil {
		t.Fatal(err)
	}
	if prepared.Title != "2. Mahnung" || prepared.Proposal.Total != "205.00" || prepared.Proposal.Amount != "200.00" || !strings.Contains(prepared.Body, "205,00 EUR") {
		t.Fatalf("unexpected level override: %+v", prepared.Proposal)
	}

	// Once paid, no reminder can be created and a failed record leaves no file.
	if err = s.SetPaid(id, true); err != nil {
		t.Fatal(err)
	}
	blocked := filepath.Join(t.TempDir(), "bezahlt.pdf")
	if _, err = s.exportReminderPDF(id, 0, blocked, "2026-11-30"); err == nil {
		t.Fatal("reminder for paid invoice exported")
	}
	if _, statErr := os.Stat(blocked); !os.IsNotExist(statErr) {
		t.Fatal("file written for a rejected reminder")
	}
	if due, _ = s.dueReminders("2026-11-30"); len(due) != 0 {
		t.Fatal(due)
	}
	backup, err := s.ExportBackup()
	if err != nil || !strings.Contains(backup, r.ID) {
		t.Fatal("reminder missing in backup", err)
	}
}

func TestReminderFileNameAndMoney(t *testing.T) {
	if got := reminderFileName("1. Mahnung", "RE/2026:01"); got != "1-mahnung-RE-2026-01.pdf" {
		t.Fatal(got)
	}
	if got := reminderMoney("1234567.5", "EUR"); got != "1.234.567,50 EUR" {
		t.Fatal(got)
	}
	if got := reminderMoney("12", "JPY"); got != "12 JPY" {
		t.Fatal(got)
	}
}
