package store

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func openTest(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func anyCategory(string) bool { return true }

func saveExpense(t *testing.T, s *Store, supplier string, file []byte) Expense {
	t.Helper()
	e, err := s.SaveExpense(Expense{Date: "2026-10-01", Supplier: supplier, Category: "sonstige", Gross: "11.90", TaxRate: "19", PaidWith: "cash",
		AttachmentName: "foto.jpg", AttachmentType: "image/jpeg", Attachment: file}, anyCategory, "asset")
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func TestExpenseFilesLeaveTheExpenseRows(t *testing.T) {
	s := openTest(t)
	photo := bytes.Repeat([]byte("foto"), 1000)
	e := saveExpense(t, s, "Bäckerei", photo)
	var raw []byte
	if err := s.db.QueryRow("SELECT value FROM settings WHERE key=?", "expense:"+e.ID).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte("attachment\":")) {
		t.Fatalf("file stayed in the row: %s", raw)
	}
	list, err := s.Expenses()
	if err != nil || len(list) != 1 || list[0].Attachment != nil || list[0].AttachmentName != "foto.jpg" {
		t.Fatalf("list %+v %v", list, err)
	}
	if got, err := s.ExpenseAttachment(e.ID); err != nil || !bytes.Equal(got.Attachment, photo) {
		t.Fatalf("attachment: %v", err)
	}
	// Saving without a new file keeps it, a removed name drops it.
	e.Note, e.Attachment = "geändert", nil
	if _, err = s.SaveExpense(e, anyCategory, "asset"); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.ExpenseAttachment(e.ID); !bytes.Equal(got.Attachment, photo) {
		t.Fatal("file lost on save")
	}
	e.AttachmentName, e.AttachmentType = "", ""
	if _, err = s.SaveExpense(e, anyCategory, "asset"); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.ExpenseAttachment(e.ID); len(got.Attachment) != 0 {
		t.Fatal("removed file kept")
	}
	saved := saveExpense(t, s, "Tankstelle", photo)
	if _, err = s.CancelExpense(saved.ID, "Doppelt erfasst"); err != nil {
		t.Fatal(err)
	}
	if files, err := s.ExpenseFiles(saved.ID); err != nil || len(files) != 1 {
		t.Fatalf("a cancelled receipt keeps its file: %v %v", files, err)
	}
}

func TestOldExpenseRowsAreMigrated(t *testing.T) {
	s := openTest(t)
	photo := []byte("altes Foto")
	old, err := json.Marshal(Expense{ID: "exp-alt", Date: "2025-01-01", Supplier: "Alt", Attachment: photo, AttachmentName: "a.jpg", AttachmentType: "image/jpeg"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.db.Exec("INSERT INTO settings(key,value) VALUES('expense:exp-alt',?)", old); err != nil {
		t.Fatal(err)
	}
	if err = migrateExpenseFiles(s.db); err != nil {
		t.Fatal(err)
	}
	if got, err := s.ExpenseAttachment("exp-alt"); err != nil || !bytes.Equal(got.Attachment, photo) || got.Supplier != "Alt" {
		t.Fatalf("migrated %+v %v", got, err)
	}
	var raw []byte
	s.db.QueryRow("SELECT value FROM settings WHERE key='expense:exp-alt'").Scan(&raw)
	if bytes.Contains(raw, []byte(base64.StdEncoding.EncodeToString(photo))) {
		t.Fatal("file still in the row")
	}
}

func TestOldJSONBackupWithInlineFilesRestores(t *testing.T) {
	s := openTest(t)
	photo := []byte("Foto aus alter Sicherung")
	row, _ := json.Marshal(Expense{ID: "exp-1", Date: "2025-01-01", Supplier: "Alt", Attachment: photo, AttachmentName: "a.jpg", AttachmentType: "image/jpeg"})
	doc, _ := json.Marshal(map[string]any{"format": "e-rechnung-studio-backup", "version": 4, "settings": []map[string]any{{"key": "expense:exp-1", "value": json.RawMessage(row)}}})
	if err := s.RestoreBackup(string(doc)); err != nil {
		t.Fatal(err)
	}
	if got, err := s.ExpenseAttachment("exp-1"); err != nil || !bytes.Equal(got.Attachment, photo) {
		t.Fatalf("restored %+v %v", got, err)
	}
}

// fill stores one document of every kind.
func fill(t *testing.T, s *Store) map[string][]byte {
	t.Helper()
	docs := map[string][]byte{
		"received": []byte("%PDF Eingangsbeleg"), "artifact": []byte("%PDF Rechnung"), "reminder": []byte("%PDF Mahnung"),
		"deleted-xml": []byte("<xml>gelöscht</xml>"), "deleted-pdf": []byte("%PDF gelöscht"), "photo": []byte("Foto"),
	}
	for _, stmt := range []struct {
		q    string
		args []any
	}{
		{`INSERT INTO received_invoices(id,data,original) VALUES(?,?,?)`, []any{sumOf(docs["received"]), []byte(`{"id":"r"}`), docs["received"]}},
		{`INSERT INTO invoices(id,number,status,data,created_at,updated_at) VALUES('i1','RE-1','sent',?,'x','x')`, []any{[]byte(`{}`)}},
		{`INSERT INTO artifacts(invoice_id,kind,data,created_at) VALUES('i1','facturx-pdf',?,'x')`, []any{docs["artifact"]}},
		{`INSERT INTO invoice_reminders(id,invoice_id,level,created_at,data,pdf) VALUES('m1','i1',1,'x',?,?)`, []any{[]byte(`{"id":"m1"}`), docs["reminder"]}},
		{`INSERT INTO deleted_invoices(id,number,customer_name,issue_date,gross,data,xml,pdf,reason,deleted_at) VALUES('d1','RE-0','Kunde','2026-01-01','1.00',?,?,?,'Fehler','x')`, []any{[]byte(`{}`), docs["deleted-xml"], docs["deleted-pdf"]}},
	} {
		if _, err := s.db.Exec(stmt.q, stmt.args...); err != nil {
			t.Fatalf("%s: %v", stmt.q, err)
		}
	}
	saveExpense(t, s, "Bäckerei", docs["photo"])
	saveExpense(t, s, "Doppelt", docs["photo"])
	return docs
}

func TestSnapshotRoundTripKeepsEveryDocument(t *testing.T) {
	s := openTest(t)
	docs := fill(t, s)
	snap, err := s.ExportSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	// Six different files; the photo used twice is one document.
	if len(snap.Blobs) != 6 {
		t.Fatalf("%d blobs: %+v", len(snap.Blobs), snap.Blobs)
	}
	if bytes.Contains(snap.Document, []byte(base64.StdEncoding.EncodeToString(docs["received"]))) {
		t.Fatal("document stayed inline")
	}
	if snap.Counts.Expenses != 2 || snap.Counts.Received != 1 || snap.Counts.Invoices != 1 {
		t.Fatalf("counts %+v", snap.Counts)
	}
	stored := map[string][]byte{}
	for _, b := range snap.Blobs {
		data, err := s.ReadBlob(b)
		if err != nil {
			t.Fatal(err)
		}
		stored[b.Sum] = data
	}
	// A second snapshot finds everything in the index and reads no document.
	if _, err = s.db.Exec("UPDATE received_invoices SET original=x'00'"); err != nil {
		t.Fatal(err)
	}
	again, err := s.ExportSnapshot()
	if err != nil || len(again.Blobs) != 6 {
		t.Fatalf("second snapshot: %d %v", len(again.Blobs), err)
	}
	for _, b := range again.Blobs {
		if strings.HasPrefix(b.Source, sourceReceived) {
			if _, err = s.ReadBlob(b); err == nil {
				t.Fatal("changed document not detected")
			}
		}
	}

	target := openTest(t)
	event := BackupEvent{Kind: BackupEventRestore, OK: true, Summary: "Wiederhergestellt"}
	open := func(sum string) ([]byte, error) { return stored[sum], nil }
	if err = target.RestoreSnapshot(snap.Document, open, event); err != nil {
		t.Fatal(err)
	}
	check := func(query string, want []byte) {
		t.Helper()
		var got []byte
		if err := target.db.QueryRow(query).Scan(&got); err != nil || !bytes.Equal(got, want) {
			t.Fatalf("%s: %q %v", query, got, err)
		}
	}
	check("SELECT original FROM received_invoices", docs["received"])
	check("SELECT data FROM artifacts", docs["artifact"])
	check("SELECT pdf FROM invoice_reminders", docs["reminder"])
	check("SELECT xml FROM deleted_invoices", docs["deleted-xml"])
	check("SELECT pdf FROM deleted_invoices", docs["deleted-pdf"])
	var files int
	target.db.QueryRow("SELECT COUNT(*) FROM expense_files").Scan(&files)
	if files != 2 {
		t.Fatalf("%d expense files", files)
	}
	// The history tables stay protected after a restore.
	if _, err = target.db.Exec("DELETE FROM invoice_reminders"); err == nil {
		t.Fatal("reminder history lost its protection")
	}
	if _, err = target.db.Exec("DELETE FROM backup_log"); err == nil {
		t.Fatal("protocol can be deleted")
	}
	if log, _ := target.BackupEvents(); len(log) != 1 || log[0].Summary != "Wiederhergestellt" {
		t.Fatalf("protocol %+v", log)
	}
}

func TestSnapshotRestoreRejectsChangedDocument(t *testing.T) {
	s := openTest(t)
	fill(t, s)
	snap, err := s.ExportSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	target := openTest(t)
	if _, err = target.db.Exec(`INSERT INTO customers(id,name,data,created_at,updated_at) VALUES('c1','Bleibt',?,'x','x')`, []byte(`{}`)); err != nil {
		t.Fatal(err)
	}
	open := func(string) ([]byte, error) { return []byte("verändert"), nil }
	if err = target.RestoreSnapshot(snap.Document, open, BackupEvent{Kind: BackupEventRestore}); err == nil || !strings.Contains(err.Error(), "verändert") {
		t.Fatalf("changed document accepted: %v", err)
	}
	var n int
	target.db.QueryRow("SELECT COUNT(*) FROM customers").Scan(&n)
	if n != 1 {
		t.Fatal("failed restore changed local data")
	}
	if log, _ := target.BackupEvents(); len(log) != 0 {
		t.Fatalf("failed restore logged: %+v", log)
	}
}

func TestRestoreCannotRewindCounters(t *testing.T) {
	s := openTest(t)
	if _, err := s.db.Exec("INSERT INTO counters(name,value) VALUES('invoice-2026',7)"); err != nil {
		t.Fatal(err)
	}
	backup, err := s.ExportBackup()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.db.Exec("UPDATE counters SET value=12"); err != nil {
		t.Fatal(err)
	}
	if err = s.RestoreBackup(backup); err != nil {
		t.Fatal(err)
	}
	var v int
	s.db.QueryRow("SELECT value FROM counters WHERE name='invoice-2026'").Scan(&v)
	if v != 12 {
		t.Fatalf("counter went back to %d", v)
	}
}
