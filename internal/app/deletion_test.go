package app

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"papierleicht.de/e-rechnung-studio/internal/model"
	"papierleicht.de/e-rechnung-studio/internal/store"
)

func issueNumbered(t *testing.T, s *InvoiceService, number string) model.InvoiceView {
	t.Helper()
	d := testDraft()
	d.Number = number
	view, err := s.IssueInvoice(d)
	if err != nil {
		t.Fatal(err)
	}
	return view
}

func newTestService(t *testing.T) *InvoiceService {
	t.Helper()
	s, err := NewInvoiceService(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	if _, err = s.SaveCompany(testCompany()); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestDeleteUnsentInvoiceKeepsLogAndBlocksNumber(t *testing.T) {
	s := newTestService(t)
	issued := issueNumbered(t, s, "DEL-2026-001")
	id := issued.Draft.ID
	check, err := s.InvoiceDeletionCheck(id)
	if err != nil || !check.Allowed || len(check.Blockers) != 0 {
		t.Fatalf("unsent invoice not deletable: %+v %v", check, err)
	}
	if len(check.Files) != 1 || check.Files[0] != issued.Export.PDFPath {
		t.Fatalf("export not listed: %+v / %+v", check.Files, issued.Export)
	}
	if _, err = s.DeleteIssuedInvoice(id, " "); err == nil {
		t.Fatal("deleted without a reason")
	}
	before, err := s.store.IssuedDocuments()
	if err != nil {
		t.Fatal(err)
	}
	result, err := s.DeleteIssuedInvoice(id, "Tippfehler im Kundennamen")
	if err != nil {
		t.Fatal(err)
	}
	if result.Invoice.Number != "DEL-2026-001" || len(result.RemovedFiles) != 1 {
		t.Fatalf("unexpected result %+v", result)
	}
	if _, err = os.Stat(issued.Export.PDFPath); !os.IsNotExist(err) {
		t.Fatal("automatic export still exists")
	}
	after, err := s.store.IssuedDocuments()
	if err != nil || len(after) != len(before)-1 {
		t.Fatalf("deleted invoice still counts for taxes: %d -> %d %v", len(before), len(after), err)
	}
	list, err := s.ListInvoices()
	if err != nil || len(list) != 0 {
		t.Fatalf("deleted invoice still listed: %+v %v", list, err)
	}
	log, err := s.DeletedInvoices()
	if err != nil || len(log) != 1 || log[0].Reason != "Tippfehler im Kundennamen" || log[0].Gross == "" {
		t.Fatalf("log %+v %v", log, err)
	}
	html, err := s.DeletedInvoiceHTML(id)
	if err != nil || !strings.Contains(html, "DEL-2026-001") {
		t.Fatalf("archived document unavailable: %v", err)
	}
	// The number is blocked for good, also in another spelling.
	again := testDraft()
	again.Number = "del-2026-001"
	if _, err = s.IssueInvoice(again); err == nil {
		t.Fatal("deleted number issued again")
	}
	numberCheck, err := s.CheckInvoiceNumber(testDraft(), "DEL-2026-001")
	if err != nil || numberCheck.State != "taken" {
		t.Fatalf("deleted number not taken: %+v %v", numberCheck, err)
	}
	if next, _, err := s.store.SeriesNext("DEL-{JJJJ}-{NNN}", numberDate("2026-09-09")); err != nil || next != "DEL-2026-002" {
		t.Fatalf("series reuses deleted number: %s %v", next, err)
	}
	if _, err = s.store.DB().Exec("DELETE FROM deleted_invoices"); err == nil {
		t.Fatal("log entry could be removed")
	}
	draft, err := s.DraftFromDeletedInvoice(id)
	if err != nil || draft.Draft.Status != "draft" || draft.Draft.Number == "DEL-2026-001" || !draft.Draft.AutomaticNumber || draft.Draft.Customer.Name != "Beispielkunde AG" {
		t.Fatalf("draft from deleted invoice %+v %v", draft.Draft, err)
	}

	backup, err := s.ExportBackup()
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Version int               `json:"version"`
		Deleted []json.RawMessage `json:"deletedInvoices"`
	}
	if err = json.Unmarshal([]byte(backup), &doc); err != nil || doc.Version != 9 || len(doc.Deleted) != 1 {
		t.Fatalf("backup misses log: version %d, %d entries, %v", doc.Version, len(doc.Deleted), err)
	}
	restored := newTestService(t)
	if err = restored.RestoreBackup(backup); err != nil {
		t.Fatal(err)
	}
	if log, err = restored.DeletedInvoices(); err != nil || len(log) != 1 {
		t.Fatalf("restored log %+v %v", log, err)
	}
	if _, err = restored.IssueInvoice(testDraftNumbered("DEL-2026-001")); err == nil {
		t.Fatal("restored log does not block the number")
	}
	if _, err = restored.store.DB().Exec("UPDATE deleted_invoices SET reason='x'"); err == nil {
		t.Fatal("restored log is writable")
	}
}

func testDraftNumbered(number string) model.InvoiceDraft {
	d := testDraft()
	d.Number = number
	return d
}

func TestDeletionBlockedOnceInvoiceLeftTheApp(t *testing.T) {
	s := newTestService(t)
	id := issueNumbered(t, s, "DEL-2026-010").Draft.ID
	blocked := func(want string) {
		t.Helper()
		check, err := s.InvoiceDeletionCheck(id)
		if err != nil {
			t.Fatal(err)
		}
		if check.Allowed || !strings.Contains(strings.Join(check.Blockers, " "), want) {
			t.Fatalf("want blocker %q, got %+v", want, check)
		}
		if _, err = s.DeleteIssuedInvoice(id, "Versehen"); err == nil || !strings.Contains(err.Error(), "Stornorechnung") {
			t.Fatalf("deleted despite %q: %v", want, err)
		}
	}
	if err := s.MarkInvoiceSent(id, true); err != nil {
		t.Fatal(err)
	}
	blocked("als verschickt markiert")
	list, err := s.ListInvoices()
	if err != nil || list[0].SentAt == "" || list[0].SentVia != "manual" {
		t.Fatalf("sent mark not listed: %+v %v", list, err)
	}
	if err = s.MarkInvoiceSent(id, false); err != nil {
		t.Fatal(err)
	}
	if check, _ := s.InvoiceDeletionCheck(id); !check.Allowed {
		t.Fatalf("withdrawn mark still blocks: %+v", check)
	}

	if err = s.MarkVATFiled("2026-09", "2026-10-10", "38.00"); err != nil {
		t.Fatal(err)
	}
	blocked("Umsatzsteuer-Voranmeldung September 2026")
	if err = s.UnmarkVATFiled("2026-09"); err != nil {
		t.Fatal(err)
	}

	correction, err := s.CreateCorrectionDraft(id)
	if err != nil {
		t.Fatal(err)
	}
	blocked("Korrektur")
	if err = s.DeleteDraft(correction.Draft.ID); err != nil {
		t.Fatal(err)
	}

	if _, err = s.RecordPayment(store.Payment{InvoiceID: id, Direction: "outgoing", Amount: "10.00", Currency: "EUR", Date: "2026-09-20", Source: "manual", Reason: "Teilzahlung"}); err != nil {
		t.Fatal(err)
	}
	blocked("Zahlungen")
	if err = s.MarkInvoiceSent("", true); err == nil {
		t.Fatal("marked an unknown invoice")
	}
}

func TestDraftsKeepUsingDeleteDraft(t *testing.T) {
	s := newTestService(t)
	saved, err := s.SaveInvoice(testDraft())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.DeleteIssuedInvoice(saved.Draft.ID, "Versehen"); err == nil {
		t.Fatal("draft went into the log")
	}
	if err = s.MarkInvoiceSent(saved.Draft.ID, true); err == nil {
		t.Fatal("draft marked as sent")
	}
}

func TestAClosedPeriodProtectsAnUnsentInvoice(t *testing.T) {
	s := newTestService(t)
	if _, err := s.SaveCompany(testCompany()); err != nil {
		t.Fatal(err)
	}
	d := testDraft()
	d.Number, d.IssueDate, d.DeliveryDate, d.DueDate = "RE-2026-0001", "2026-03-10", "2026-03-10", "2026-04-10"
	issued, err := s.IssueInvoice(d)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.store.LockPeriod("", "2026-03-31"); err != nil {
		t.Fatal(err)
	}
	check, err := s.InvoiceDeletionCheck(issued.Draft.ID)
	if err != nil || check.Allowed || len(check.Blockers) == 0 {
		t.Fatalf("%+v %v", check, err)
	}
	if _, err = s.DeleteIssuedInvoice(issued.Draft.ID, "Tippfehler"); err == nil {
		t.Fatal("an invoice in a closed period must stay")
	}
}
