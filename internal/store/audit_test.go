package store

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"papierleicht.de/e-rechnung-studio/internal/model"
)

func expenseOn(date, supplier string) Expense {
	return Expense{Date: date, PaidDate: date, Supplier: supplier, Category: "office", Gross: "119.00", TaxRate: "19", PaidWith: "bank"}
}

func mustSave(t *testing.T, s *Store, e Expense) Expense {
	t.Helper()
	saved, err := s.SaveExpense(e, anyCategory, "asset")
	if err != nil {
		t.Fatal(err)
	}
	return saved
}

func verified(t *testing.T, s *Store) AuditReport {
	t.Helper()
	report, err := s.VerifyAudit(nil)
	if err != nil {
		t.Fatal(err)
	}
	return report
}

func actionsOf(t *testing.T, s *Store, object, id string) []string {
	t.Helper()
	entries, err := s.AuditEntries(AuditFilter{Object: object})
	if err != nil {
		t.Fatal(err)
	}
	out := []string{}
	for i := len(entries) - 1; i >= 0; i-- {
		if entries[i].ObjectID == id {
			out = append(out, entries[i].Action)
		}
	}
	return out
}

func issueTest(t *testing.T, s *Store, number string) model.InvoiceDraft {
	t.Helper()
	d := model.NewDraft()
	d.Number, d.IssueDate = number, "2026-03-10"
	d.Customer.Name = "Kundin"
	saved, err := s.SaveInvoice(d, "119.00")
	if err != nil {
		t.Fatal(err)
	}
	issued, err := s.Issue(saved.ID, saved, "119.00", func(model.InvoiceDraft) ([]byte, []byte, error) {
		return []byte("<xml/>"), []byte("%PDF-1.7"), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return issued
}

func TestEveryChangeOfAReceiptIsLoggedAndTheOriginalStaysReadable(t *testing.T) {
	s := openTest(t)
	e := mustSave(t, s, expenseOn("2026-03-01", "Papierhandel"))
	if !strings.HasPrefix(e.Number, "B-") {
		t.Fatalf("number %q", e.Number)
	}
	e.Gross = "238.00"
	changed := mustSave(t, s, e)
	if changed.Number != e.Number {
		t.Fatal("the number of a receipt must never change")
	}
	if _, err := s.CancelExpense(e.ID, "Doppelt erfasst"); err != nil {
		t.Fatal(err)
	}
	if got := actionsOf(t, s, AuditExpense, e.ID); strings.Join(got, ",") != "created,changed,cancelled" {
		t.Fatalf("log %v", got)
	}
	entries, _ := s.AuditEntries(AuditFilter{Object: AuditExpense})
	if !strings.Contains(entries[1].Before, `"gross":"119.00"`) || !strings.Contains(entries[1].After, `"gross":"238.00"`) {
		t.Fatalf("the original content must stay visible: %+v", entries[1])
	}
	active, _ := s.ActiveExpenses()
	all, _ := s.Expenses()
	if len(active) != 0 || len(all) != 1 || all[0].CancelReason != "Doppelt erfasst" {
		t.Fatalf("active %v all %v", active, all)
	}
	if r := verified(t, s); !r.OK {
		t.Fatalf("problems: %+v", r.Problems)
	}
}

func TestSavingWithoutChangesLeavesNoEntry(t *testing.T) {
	s := openTest(t)
	e := mustSave(t, s, expenseOn("2026-03-01", "Papierhandel"))
	mustSave(t, s, e)
	if got := actionsOf(t, s, AuditExpense, e.ID); len(got) != 1 {
		t.Fatalf("log %v", got)
	}
}

func TestReceiptNumbersAreSequentialAndNeverReused(t *testing.T) {
	s := openTest(t)
	first := mustSave(t, s, expenseOn("2026-03-01", "A"))
	second := mustSave(t, s, expenseOn("2026-03-02", "B"))
	if _, err := s.CancelExpense(first.ID, "Falsch"); err != nil {
		t.Fatal(err)
	}
	third := mustSave(t, s, expenseOn("2026-03-03", "C"))
	year := time.Now().Year()
	want := []string{first.Number, second.Number, third.Number}
	for i, n := range want {
		if n != fmt.Sprintf("B-%d-%04d", year, i+1) {
			t.Fatalf("numbers %v", want)
		}
	}
	imported, err := s.SaveReceived(model.ReceivedInvoice{ID: "r1", Filename: "a.pdf", ImportedAt: now()}, []byte("original"))
	if err != nil {
		t.Fatal(err)
	}
	if imported.Invoice.ReceiptNumber != fmt.Sprintf("B-%d-0004", year) {
		t.Fatalf("received number %q", imported.Invoice.ReceiptNumber)
	}
	again, err := s.SaveReceived(model.ReceivedInvoice{ID: "r1", Filename: "a.pdf"}, []byte("original"))
	if err != nil || !again.Duplicate || again.Invoice.ReceiptNumber != imported.Invoice.ReceiptNumber {
		t.Fatalf("duplicate %+v %v", again, err)
	}
}

func TestAReplacedFileStaysStored(t *testing.T) {
	s := openTest(t)
	e := expenseOn("2026-03-01", "Papierhandel")
	e.Attachment, e.AttachmentName, e.AttachmentType = []byte("erstes Foto"), "a.jpg", "image/jpeg"
	saved := mustSave(t, s, e)
	saved.Attachment, saved.AttachmentName = []byte("zweites Foto"), "b.jpg"
	mustSave(t, s, saved)
	files, err := s.ExpenseFiles(saved.ID)
	if err != nil || len(files) != 2 {
		t.Fatalf("files %v %v", files, err)
	}
	got, _ := s.ExpenseAttachment(saved.ID)
	if string(got.Attachment) != "zweites Foto" {
		t.Fatalf("current file %q", got.Attachment)
	}
	if r := verified(t, s); !r.OK || r.Files != 2 {
		t.Fatalf("%+v", r)
	}
}

func TestForeignCurrencyKeepsAmountAndRate(t *testing.T) {
	s := openTest(t)
	e := expenseOn("2026-03-01", "Cloud Inc.")
	e.Gross, e.TaxRate, e.Currency, e.OriginalGross = "92.00", "none", "usd", "100,00"
	saved := mustSave(t, s, e)
	if saved.Currency != "USD" || saved.OriginalGross != "100" || saved.ExchangeRate != "1.086957" {
		t.Fatalf("%+v", saved)
	}
	saved.Currency, saved.OriginalGross = "EUR", "100"
	if again := mustSave(t, s, saved); again.Currency != "" || again.OriginalGross != "" || again.ExchangeRate != "" {
		t.Fatalf("%+v", again)
	}
}

func TestAClosedPeriodOnlyAllowsCancelling(t *testing.T) {
	s := openTest(t)
	old := mustSave(t, s, expenseOn("2026-03-01", "Papierhandel"))
	if _, err := s.LockPeriod("", "2026-03-31"); err != nil {
		t.Fatal(err)
	}
	listed, _ := s.Expenses()
	if listed[0].Locked == "" {
		t.Fatal("the list must explain the lock")
	}
	changed := old
	changed.Gross = "1.00"
	if _, err := s.SaveExpense(changed, anyCategory, "asset"); err == nil || !strings.Contains(err.Error(), "festgeschrieben") {
		t.Fatalf("err %v", err)
	}
	// The missing photo of an unchanged receipt is still welcome.
	photo := old
	photo.Attachment, photo.AttachmentName, photo.AttachmentType = []byte("Foto"), "a.jpg", "image/jpeg"
	if _, err := s.SaveExpense(photo, anyCategory, "asset"); err != nil {
		t.Fatalf("adding the file: %v", err)
	}
	if _, err := s.CancelExpense(old.ID, "Falscher Beleg"); err != nil {
		t.Fatal(err)
	}
	// A receipt found later can be recorded, and stays open until the next closing.
	time.Sleep(1100 * time.Millisecond)
	late := expenseOn("2026-03-02", "Nachzügler")
	saved := mustSave(t, s, late)
	saved.Note = "gefunden"
	if _, err := s.SaveExpense(saved, anyCategory, "asset"); err != nil {
		t.Fatalf("late receipt: %v", err)
	}
	if _, err := s.LockPeriod("2027-01-01", "2026-01-01"); err == nil {
		t.Fatal("a period must not end before it starts")
	}
	if r := verified(t, s); !r.OK {
		t.Fatalf("%+v", r.Problems)
	}
}

func TestATransmittedReturnClosesItsPeriodUntilTheNoteIsWithdrawn(t *testing.T) {
	s := openTest(t)
	e := mustSave(t, s, expenseOn("2026-03-01", "Papierhandel"))
	if err := s.SaveVATFiling(VATFiling{PeriodID: "2026-Q1", SubmittedOn: "2026-04-10", Payable: "19.00"}); err != nil {
		t.Fatal(err)
	}
	e.Note = "Korrektur"
	if _, err := s.SaveExpense(e, anyCategory, "asset"); err == nil {
		t.Fatal("a filed quarter must be closed")
	}
	if err := s.DeleteVATFiling("2026-Q1"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveExpense(e, anyCategory, "asset"); err != nil {
		t.Fatal(err)
	}
	if got := actionsOf(t, s, AuditFiling, FilingVAT+"2026-Q1"); strings.Join(got, ",") != "filed,unfiled" {
		t.Fatalf("log %v", got)
	}
	if r := verified(t, s); !r.OK {
		t.Fatalf("%+v", r.Problems)
	}
}

func TestReceivedDocumentsAreLoggedAndProtected(t *testing.T) {
	s := openTest(t)
	doc := model.ReceivedInvoice{ID: "r1", Filename: "a.pdf", ImportedAt: now(), Status: "active"}
	doc.Inspection.IssueDate = "2026-03-05"
	if _, err := s.SaveReceived(doc, []byte("original")); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveReceivedCategory(ReceivedCategory{ReceivedID: "r1", Category: "office"}, anyCategory, "asset"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ChangeReceivedStatus("r1", "misimport", "Falsche Datei"); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteReceivedMisimport("r1", "a.pdf", true); err != nil {
		t.Fatal(err)
	}
	if got := actionsOf(t, s, AuditReceived, "r1"); strings.Join(got, ",") != "created,changed,deleted" {
		t.Fatalf("log %v", got)
	}
	if r := verified(t, s); !r.OK {
		t.Fatalf("%+v", r.Problems)
	}

	other := model.ReceivedInvoice{ID: "r2", Filename: "b.pdf", ImportedAt: now(), Status: "active"}
	other.Inspection.IssueDate = "2026-03-05"
	if _, err := s.SaveReceived(other, []byte("zweites Original")); err != nil {
		t.Fatal(err)
	}
	if _, err := s.LockPeriod("", "2026-03-31"); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveReceivedCategory(ReceivedCategory{ReceivedID: "r2", Category: "office"}, anyCategory, "asset"); err == nil {
		t.Fatal("a closed document must not be reassigned")
	}
	if _, err := s.ChangeReceivedStatus("r2", "misimport", "Falsch"); err == nil {
		t.Fatal("a closed document must not change")
	}
	listed, _ := s.ListReceived()
	if listed[0].Locked == "" {
		t.Fatal("the list must explain the lock")
	}
}

func TestANewCheckOfAReceivedDocumentKeepsTheEarlierResult(t *testing.T) {
	s := openTest(t)
	doc := model.ReceivedInvoice{ID: "r1", Filename: "a.xml", ImportedAt: now(), Status: "active"}
	doc.Inspection.Validation = model.ValidationResult{State: "invalid", Issues: []model.ValidationIssue{{RuleID: "BR-16"}}}
	if _, err := s.SaveReceived(doc, []byte("<xml/>")); err != nil {
		t.Fatal(err)
	}
	same := doc.Inspection
	if err := s.UpdateReceivedInspection("r1", same); err != nil {
		t.Fatal(err)
	}
	if got := actionsOf(t, s, AuditReceived, "r1"); len(got) != 1 {
		t.Fatalf("an unchanged result needs no entry: %v", got)
	}
	better := doc.Inspection
	better.Validation = model.ValidationResult{State: "valid"}
	if err := s.UpdateReceivedInspection("r1", better); err != nil {
		t.Fatal(err)
	}
	entries, _ := s.AuditEntries(AuditFilter{Object: AuditReceived})
	if len(entries) != 2 || !strings.Contains(entries[0].Before, "BR-16") || !strings.Contains(entries[0].After, `"valid"`) {
		t.Fatalf("%+v", entries)
	}
	stored, original, err := s.GetReceived("r1")
	if err != nil || string(original) != "<xml/>" || stored.Inspection.Validation.State != "valid" {
		t.Fatalf("%+v %q %v", stored, original, err)
	}
	if n := len(stored.History); n != 1 || stored.History[n-1].Action != "rechecked" || !strings.Contains(stored.History[n-1].Reason, "BR-16") {
		t.Fatalf("%+v", stored.History)
	}
	if r := verified(t, s); !r.OK {
		t.Fatalf("%+v", r.Problems)
	}
}

func TestInvoiceLifecycleIsLogged(t *testing.T) {
	s := openTest(t)
	d := issueTest(t, s, "RE-2026-0001")
	if err := s.MarkSent(d.ID, true); err != nil {
		t.Fatal(err)
	}
	if err := s.MarkSent(d.ID, false); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.DeleteIssuedInvoice(d.ID, "Tippfehler", func(model.InvoiceDraft, InvoiceLinks) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if got := actionsOf(t, s, AuditInvoice, d.ID); strings.Join(got, ",") != "issued,sent,unsent,deleted" {
		t.Fatalf("log %v", got)
	}
	if r := verified(t, s); !r.OK {
		t.Fatalf("%+v", r.Problems)
	}
}

func TestPaymentsAreLogged(t *testing.T) {
	s := openTest(t)
	d := issueTest(t, s, "RE-2026-0001")
	if err := s.SetPaid(d.ID, true); err != nil {
		t.Fatal(err)
	}
	if err := s.SetPaid(d.ID, false); err != nil {
		t.Fatal(err)
	}
	entries, _ := s.AuditEntries(AuditFilter{Object: AuditPayment})
	if len(entries) != 2 || entries[0].Action != AuditRevoked || entries[1].Action != AuditRecorded {
		t.Fatalf("%+v", entries)
	}
	if r := verified(t, s); !r.OK {
		t.Fatalf("%+v", r.Problems)
	}
}

func TestChangedSettingsAreLoggedOnce(t *testing.T) {
	s := openTest(t)
	c := model.Company{Name: "Atelier"}
	if err := s.SaveCompany(c); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveCompany(c); err != nil {
		t.Fatal(err)
	}
	c.Name = "Atelier Lindner"
	if err := s.SaveSettings(model.AppSettings{}, &c); err != nil {
		t.Fatal(err)
	}
	if got := actionsOf(t, s, AuditSetting, "company"); strings.Join(got, ",") != "created,changed" {
		t.Fatalf("log %v", got)
	}
	if err := s.SaveStandardSeries("RE-{JJJJ}-{NNNN}"); err != nil {
		t.Fatal(err)
	}
	if r := verified(t, s); !r.OK {
		t.Fatalf("%+v", r.Problems)
	}
}

func TestAnEditInTheDatabaseIsFound(t *testing.T) {
	s := openTest(t)
	e := mustSave(t, s, expenseOn("2026-03-01", "Papierhandel"))
	if _, err := s.db.Exec(`UPDATE settings SET value=CAST(json_set(CAST(value AS TEXT),'$.gross','1.00') AS BLOB) WHERE key=?`, "expense:"+e.ID); err != nil {
		t.Fatal(err)
	}
	r := verified(t, s)
	if r.OK || !strings.Contains(r.Problems[0].Message, "weicht") {
		t.Fatalf("%+v", r)
	}
}

func TestAChangedOrMissingLogEntryIsFound(t *testing.T) {
	s := openTest(t)
	mustSave(t, s, expenseOn("2026-03-01", "A"))
	mustSave(t, s, expenseOn("2026-03-02", "B"))
	if _, err := s.db.Exec(`UPDATE audit_log SET label='x' WHERE seq=1`); err == nil {
		t.Fatal("the database must refuse edits of the log")
	}
	if _, err := s.db.Exec(`DELETE FROM audit_log WHERE seq=1`); err == nil {
		t.Fatal("the database must refuse deleting from the log")
	}
	if _, err := s.db.Exec(`DROP TRIGGER audit_log_immutable_update; UPDATE audit_log SET label='x' WHERE seq=1`); err != nil {
		t.Fatal(err)
	}
	if r := verified(t, s); r.OK || !strings.Contains(r.Problems[0].Message, "verändert") {
		t.Fatalf("%+v", r)
	}
	if _, err := s.db.Exec(`DROP TRIGGER audit_log_immutable_delete; DELETE FROM audit_log WHERE seq=1`); err != nil {
		t.Fatal(err)
	}
	if r := verified(t, s); r.OK || !strings.Contains(r.Problems[0].Message, "fehlen") {
		t.Fatalf("%+v", r)
	}
}

func TestAReceiptExposesARewrittenLog(t *testing.T) {
	s := openTest(t)
	mustSave(t, s, expenseOn("2026-03-01", "A"))
	seq, hash, err := s.AuditHead()
	if err != nil {
		t.Fatal(err)
	}
	if err = s.AddAuditAnchor(AuditAnchor{Seq: seq, Hash: hash, At: now(), Receipt: "signed"}); err != nil {
		t.Fatal(err)
	}
	if r := verified(t, s); !r.OK || r.Anchors != 1 || r.LastAnchor == "" {
		t.Fatalf("%+v", r)
	}
	// Somebody recalculates the whole chain after changing the entry.
	entries, _ := readAudit(s.db)
	entries[0].Label = "gefälscht"
	entries[0].Hash = auditHash(entries[0])
	if _, err = s.db.Exec(`DROP TRIGGER audit_log_immutable_delete; DELETE FROM audit_log`); err != nil {
		t.Fatal(err)
	}
	if err = insertAudit(s.db, entries[0]); err != nil {
		t.Fatal(err)
	}
	r := verified(t, s)
	if r.OK || !strings.Contains(r.Problems[0].Message, "Nachweis") {
		t.Fatalf("%+v", r)
	}
}

func TestDataFromBeforeTheLogGetsABaseline(t *testing.T) {
	s := openTest(t)
	if _, err := s.db.Exec(`INSERT INTO settings(key,value) VALUES('expense:old',?)`, []byte(`{"id":"old","supplier":"Altbestand","date":"2026-01-01"}`)); err != nil {
		t.Fatal(err)
	}
	if r := verified(t, s); r.OK {
		t.Fatal("data without a log entry must be reported")
	}
	if err := s.baselineAudit(); err != nil {
		t.Fatal(err)
	}
	if r := verified(t, s); !r.OK {
		t.Fatalf("%+v", r.Problems)
	}
}

func TestVersionsAreNotedOnce(t *testing.T) {
	s := openTest(t)
	for _, v := range []string{"1.0.0", "1.0.0", "1.1.0"} {
		if err := s.SetVersion(v); err != nil {
			t.Fatal(err)
		}
	}
	entries, _ := s.AuditEntries(AuditFilter{Object: AuditProgram})
	if len(entries) != 2 || entries[0].ObjectID != "1.1.0" || entries[0].Version != "1.1.0" {
		t.Fatalf("%+v", entries)
	}
}

func snapshotOf(t *testing.T, s *Store) ([]byte, BlobOpener) {
	t.Helper()
	snap, err := s.ExportSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	blobs := map[string][]byte{}
	for _, b := range snap.Blobs {
		data, err := s.ReadBlob(b)
		if err != nil {
			t.Fatal(err)
		}
		blobs[b.Sum] = data
	}
	return snap.Document, func(sum string) ([]byte, error) { return blobs[sum], nil }
}

func TestAFreshInstallAdoptsTheHistoryOfABackup(t *testing.T) {
	s := openTest(t)
	e := expenseOn("2026-03-01", "Papierhandel")
	e.Attachment, e.AttachmentName, e.AttachmentType = []byte("Foto"), "a.jpg", "image/jpeg"
	saved := mustSave(t, s, e)
	saved.Attachment, saved.AttachmentName = []byte("Foto 2"), "b.jpg"
	mustSave(t, s, saved)
	if _, err := s.LockPeriod("", "2026-03-31"); err != nil {
		t.Fatal(err)
	}
	seq, hash, _ := s.AuditHead()
	if err := s.AddAuditAnchor(AuditAnchor{Seq: seq, Hash: hash, At: now(), Receipt: "signed"}); err != nil {
		t.Fatal(err)
	}
	doc, open := snapshotOf(t, s)

	fresh := openTest(t)
	if err := fresh.SetVersion("1.0.0"); err != nil {
		t.Fatal(err)
	}
	if err := fresh.RestoreSnapshot(doc, open, BackupEvent{Kind: BackupEventRestore, OK: true, Summary: "Sicherung"}); err != nil {
		t.Fatal(err)
	}
	r := verified(t, fresh)
	if !r.OK || r.Anchors != 1 || r.Files != 2 {
		t.Fatalf("%+v", r)
	}
	if got := actionsOf(t, fresh, AuditExpense, saved.ID); strings.Join(got, ",") != "created,changed" {
		t.Fatalf("the history must come along: %v", got)
	}
	if locks, _ := fresh.PeriodLocks(); len(locks) != 1 {
		t.Fatalf("locks %v", locks)
	}
	if _, err := fresh.SaveExpense(saved, anyCategory, "asset"); err == nil {
		t.Fatal("the closed period must stay closed after a restore")
	}
}

func TestAnOlderBackupKeepsTheLogAndRecordsWhatWasUndone(t *testing.T) {
	s := openTest(t)
	e := mustSave(t, s, expenseOn("2026-03-01", "Papierhandel"))
	doc, open := snapshotOf(t, s)
	e.Gross = "238.00"
	mustSave(t, s, e)
	later := mustSave(t, s, expenseOn("2026-03-02", "Nach der Sicherung"))
	before, _ := s.AuditEntries(AuditFilter{})

	if err := s.RestoreSnapshot(doc, open, BackupEvent{Kind: BackupEventRestore, OK: true}); err != nil {
		t.Fatal(err)
	}
	after, _ := s.AuditEntries(AuditFilter{})
	if len(after) <= len(before) {
		t.Fatalf("a restore must never shorten the log: %d -> %d", len(before), len(after))
	}
	if r := verified(t, s); !r.OK {
		t.Fatalf("%+v", r.Problems)
	}
	if got := actionsOf(t, s, AuditExpense, e.ID); strings.Join(got, ",") != "created,changed,restored" {
		t.Fatalf("log %v", got)
	}
	if got := actionsOf(t, s, AuditExpense, later.ID); strings.Join(got, ",") != "created,restored" {
		t.Fatalf("log %v", got)
	}
}

func TestAnEditedBackupFileIsVisibleObjectByObject(t *testing.T) {
	s := openTest(t)
	e := mustSave(t, s, expenseOn("2026-03-01", "Papierhandel"))
	doc, open := snapshotOf(t, s)
	var document map[string]any
	if err := json.Unmarshal(doc, &document); err != nil {
		t.Fatal(err)
	}
	for _, v := range document["settings"].([]any) {
		row := v.(map[string]any)
		if row["key"] == "expense:"+e.ID {
			row["value"].(map[string]any)["gross"] = "11.90"
		}
	}
	changed, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.RestoreSnapshot(changed, open, BackupEvent{Kind: BackupEventRestore, OK: true}); err != nil {
		t.Fatal(err)
	}
	if got := actionsOf(t, s, AuditExpense, e.ID); strings.Join(got, ",") != "created,restored" {
		t.Fatalf("the edit must leave a trace: %v", got)
	}
}

func TestABackupWithAChangedLogIsNoted(t *testing.T) {
	s := openTest(t)
	mustSave(t, s, expenseOn("2026-03-01", "Papierhandel"))
	doc, open := snapshotOf(t, s)
	var document map[string]any
	if err := json.Unmarshal(doc, &document); err != nil {
		t.Fatal(err)
	}
	document["auditLog"].([]any)[0].(map[string]any)["label"] = "gefälscht"
	changed, _ := json.Marshal(document)
	if err := s.RestoreSnapshot(changed, open, BackupEvent{Kind: BackupEventRestore, OK: true}); err != nil {
		t.Fatal(err)
	}
	entries, _ := s.AuditEntries(AuditFilter{Object: AuditBackup})
	if len(entries) != 1 || !strings.Contains(entries[0].Reason, "verändert") {
		t.Fatalf("%+v", entries)
	}
	if r := verified(t, s); !r.OK {
		t.Fatalf("%+v", r.Problems)
	}
}

func TestTheDocumentBehindAClosedReceiptStays(t *testing.T) {
	s := openTest(t)
	// A plain PDF has no readable date, so only the receipt tells its period.
	doc := model.ReceivedInvoice{ID: "pdf", Filename: "kassenbon.pdf", ImportedAt: "2026-05-01T10:00:00Z", Status: "active"}
	if _, err := s.SaveReceived(doc, []byte("%PDF")); err != nil {
		t.Fatal(err)
	}
	receipt := expenseOn("2026-03-01", "Papierhandel")
	receipt.ReceivedID = "pdf"
	saved := mustSave(t, s, receipt)
	if _, err := s.ChangeReceivedStatus("pdf", "misimport", "Falsche Datei"); err == nil || !strings.Contains(err.Error(), saved.Number) {
		t.Fatalf("a document with a receipt must stay: %v", err)
	}
	if _, err := s.LockPeriod("", "2026-03-31"); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveReceivedCategory(ReceivedCategory{ReceivedID: "pdf", Category: "office"}, anyCategory, "asset"); err == nil {
		t.Fatal("the closed receipt must protect its document")
	}
	if _, err := s.ChangeReceivedStatus("pdf", "archived", ""); err == nil {
		t.Fatal("the closed receipt must protect its document")
	}
	if _, err := s.CancelExpense(saved.ID, "Falsch erfasst"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ChangeReceivedStatus("pdf", "misimport", "Falsche Datei"); err != nil {
		t.Fatalf("after cancelling the document is free again: %v", err)
	}
}

func TestADocumentWithoutADateFollowsItsImportDay(t *testing.T) {
	s := openTest(t)
	doc := model.ReceivedInvoice{ID: "pdf", Filename: "a.pdf", ImportedAt: "2026-03-05T10:00:00Z", Status: "active"}
	if _, err := s.SaveReceived(doc, []byte("%PDF")); err != nil {
		t.Fatal(err)
	}
	if _, err := s.LockPeriod("", "2026-03-31"); err != nil {
		t.Fatal(err)
	}
	if listed, _ := s.ListReceived(); listed[0].Locked == "" {
		t.Fatal("no lock for a document without a date")
	}
}

func TestSavingDuringACheckRaisesNoFalseAlarm(t *testing.T) {
	s := openTest(t)
	e := mustSave(t, s, expenseOn("2026-03-01", "Papierhandel"))
	var wg sync.WaitGroup
	stop := make(chan struct{})
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; ; i++ {
			select {
			case <-stop:
				return
			default:
			}
			e.Note = fmt.Sprint(i)
			if _, err := s.SaveExpense(e, anyCategory, "asset"); err != nil {
				t.Error(err)
				return
			}
			time.Sleep(time.Millisecond)
		}
	}()
	for i := 0; i < 40; i++ {
		r, err := s.VerifyAudit(nil)
		if err != nil {
			if !strings.Contains(err.Error(), "während") && !strings.Contains(err.Error(), "Während") {
				t.Error(err)
			}
			continue
		}
		if !r.OK {
			t.Errorf("false alarm: %+v", r.Problems)
		}
	}
	close(stop)
	wg.Wait()
}

func TestAMissingFileIsFound(t *testing.T) {
	s := openTest(t)
	e := expenseOn("2026-03-01", "Papierhandel")
	e.Attachment, e.AttachmentName, e.AttachmentType = []byte("Foto"), "a.jpg", "image/jpeg"
	mustSave(t, s, e)
	if _, err := s.db.Exec(`DROP TRIGGER expense_files_immutable_delete; DELETE FROM expense_files`); err != nil {
		t.Fatal(err)
	}
	r := verified(t, s)
	if r.OK || !strings.Contains(r.Problems[0].Message, "fehlt") {
		t.Fatalf("%+v", r)
	}
}

func TestEditedFieldsBesideTheFilesAreFound(t *testing.T) {
	s := openTest(t)
	doc := model.ReceivedInvoice{ID: "r1", Filename: "a.pdf", ImportedAt: now(), Status: "active"}
	doc.Inspection.IssueDate = "2026-03-05"
	if _, err := s.SaveReceived(doc, []byte("original")); err != nil {
		t.Fatal(err)
	}
	d := issueTest(t, s, "RE-2026-0001")
	if r := verified(t, s); !r.OK {
		t.Fatalf("%+v", r.Problems)
	}
	if _, err := s.db.Exec(`UPDATE received_invoices SET data=CAST(json_set(CAST(data AS TEXT),'$.inspection.issueDate','2026-09-01') AS BLOB) WHERE id='r1'`); err != nil {
		t.Fatal(err)
	}
	if r := verified(t, s); r.OK {
		t.Fatal("a changed date of a received document must be found")
	}
	if _, err := s.db.Exec(`UPDATE received_invoices SET data=CAST(json_set(CAST(data AS TEXT),'$.inspection.issueDate','2026-03-05') AS BLOB) WHERE id='r1'`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`UPDATE invoices SET gross='1.00' WHERE id=?`, d.ID); err != nil {
		t.Fatal(err)
	}
	if r := verified(t, s); r.OK {
		t.Fatal("a changed amount of an invoice must be found")
	}
}

func TestAnOlderBackupKeepsDeletedInvoicesAndTheirNumbers(t *testing.T) {
	s := openTest(t)
	gone := issueTest(t, s, "RE-2026-0001")
	doc, open := snapshotOf(t, s)
	if _, _, err := s.DeleteIssuedInvoice(gone.ID, "Tippfehler", func(model.InvoiceDraft, InvoiceLinks) error { return nil }); err != nil {
		t.Fatal(err)
	}
	other := issueTest(t, s, "RE-2026-0002")
	if _, _, err := s.DeleteIssuedInvoice(other.ID, "Doppelt", func(model.InvoiceDraft, InvoiceLinks) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if err := s.RestoreSnapshot(doc, open, BackupEvent{Kind: BackupEventRestore, OK: true}); err != nil {
		t.Fatal(err)
	}
	log, err := s.DeletedInvoices()
	if err != nil || len(log) != 2 {
		t.Fatalf("the log of deleted invoices only grows: %+v %v", log, err)
	}
	if got := actionsOf(t, s, AuditInvoice, gone.ID); strings.Join(got, ",") != "issued,deleted,restored" {
		t.Fatalf("the invoice that came back must be noted: %v", got)
	}
	if r := verified(t, s); !r.OK {
		t.Fatalf("%+v", r.Problems)
	}
	again := model.NewDraft()
	again.Number, again.IssueDate, again.Customer.Name = "RE-2026-0002", "2026-03-10", "Kundin"
	draft, err := s.SaveInvoice(again, "119.00")
	if err == nil {
		_, err = s.Issue(draft.ID, draft, "119.00", func(model.InvoiceDraft) ([]byte, []byte, error) {
			return []byte("<xml/>"), []byte("%PDF-1.7"), nil
		})
	}
	if err == nil {
		t.Fatal("the number of a deleted invoice must stay blocked after a restore")
	}
	if _, _, err = s.DeleteIssuedInvoice(gone.ID, "Wieder falsch", func(model.InvoiceDraft, InvoiceLinks) error { return nil }); err != nil {
		t.Fatalf("deleting it again: %v", err)
	}
	entries, err := s.AuditEntries(AuditFilter{Object: AuditInvoice, Query: "Wieder falsch"})
	if err != nil || len(entries) != 1 || entries[0].ObjectID != gone.ID {
		t.Fatalf("the second reason must stay in the log: %+v %v", entries, err)
	}
	if r := verified(t, s); !r.OK {
		t.Fatalf("%+v", r.Problems)
	}
}

func TestAnOlderBackupKeepsReplacedFiles(t *testing.T) {
	s := openTest(t)
	e := expenseOn("2026-03-01", "Papierhandel")
	e.Attachment, e.AttachmentName, e.AttachmentType = []byte("erstes Foto"), "a.jpg", "image/jpeg"
	saved := mustSave(t, s, e)
	doc, open := snapshotOf(t, s)
	saved.Attachment, saved.AttachmentName = []byte("zweites Foto"), "b.jpg"
	mustSave(t, s, saved)
	if err := s.RestoreSnapshot(doc, open, BackupEvent{Kind: BackupEventRestore, OK: true}); err != nil {
		t.Fatal(err)
	}
	if files, _ := s.ExpenseFiles(saved.ID); len(files) != 2 {
		t.Fatalf("no file a receipt ever had may disappear: %v", files)
	}
	if r := verified(t, s); !r.OK {
		t.Fatalf("%+v", r.Problems)
	}
}

func TestReceiptsOfAnotherHistoryRaiseNoAlarm(t *testing.T) {
	// A fresh device got a receipt for its first entries, then restores a backup.
	fresh := openTest(t)
	if err := fresh.SetVersion("1.0.0"); err != nil {
		t.Fatal(err)
	}
	seq, hash, _ := fresh.AuditHead()
	if err := fresh.AddAuditAnchor(AuditAnchor{Seq: seq, Hash: hash, At: now(), Receipt: "lokal"}); err != nil {
		t.Fatal(err)
	}
	source := openTest(t)
	mustSave(t, source, expenseOn("2026-03-01", "A"))
	seq, hash, _ = source.AuditHead()
	if err := source.AddAuditAnchor(AuditAnchor{Seq: seq, Hash: hash, At: now(), Receipt: "quelle"}); err != nil {
		t.Fatal(err)
	}
	doc, open := snapshotOf(t, source)
	if err := fresh.RestoreSnapshot(doc, open, BackupEvent{Kind: BackupEventRestore, OK: true}); err != nil {
		t.Fatal(err)
	}
	if r := verified(t, fresh); !r.OK || r.Anchors != 1 {
		t.Fatalf("%+v", r)
	}

	// A second device with its own history restores the same backup.
	other := openTest(t)
	mustSave(t, other, expenseOn("2026-04-01", "B"))
	if err := other.RestoreSnapshot(doc, open, BackupEvent{Kind: BackupEventRestore, OK: true}); err != nil {
		t.Fatal(err)
	}
	if r := verified(t, other); !r.OK || r.Anchors != 0 {
		t.Fatalf("%+v", r)
	}
}

func TestQuotesLeaveATrace(t *testing.T) {
	s := openTest(t)
	draft, err := s.SaveQuote(testQuote(), "5.00")
	if err != nil {
		t.Fatal(err)
	}
	sent, err := s.MarkQuoteSent(draft, "5.00")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.ChangeQuoteStatus(sent.ID, "draft"); err != nil {
		t.Fatal(err)
	}
	if err = s.DeleteQuote(sent.ID); err != nil {
		t.Fatal(err)
	}
	if got := actionsOf(t, s, AuditQuote, sent.ID); strings.Join(got, ",") != "sent,changed,deleted" {
		t.Fatalf("log %v", got)
	}
	never, err := s.SaveQuote(testQuote(), "5.00")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.DeleteQuote(never.ID); err != nil {
		t.Fatal(err)
	}
	if got := actionsOf(t, s, AuditQuote, never.ID); len(got) != 0 {
		t.Fatalf("a draft never sent is no business letter: %v", got)
	}
}
