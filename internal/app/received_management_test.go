package app

import (
	"bytes"
	"encoding/base64"
	"testing"
)

func TestReceivedPreviewDoesNotImport(t *testing.T) {
	s, err := NewInvoiceService(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err := s.SaveCompany(testCompany()); err != nil {
		t.Fatal(err)
	}
	view, err := s.ValidateInvoice(testDraft())
	if err != nil {
		t.Fatal(err)
	}
	encoded := base64.StdEncoding.EncodeToString([]byte(view.XML))
	preview, err := s.PrepareReceivedInvoice("test.xml", encoded)
	if err != nil || preview.Inspection.Validation.State != "valid" || preview.PDF == "" || preview.Duplicate != nil {
		t.Fatalf("preview=%+v error=%v", preview, err)
	}
	items, err := s.ListReceivedInvoices()
	if err != nil || len(items) != 0 {
		t.Fatal("preview inserted original", err)
	}
	for _, source := range []string{"<broken", `<!DOCTYPE Invoice SYSTEM "https://example.invalid/never"><Invoice/>`} {
		preview, err := s.PrepareReceivedInvoice("broken.xml", base64.StdEncoding.EncodeToString([]byte(source)))
		if err != nil || preview.PDF != "" || preview.PreviewError == "" || preview.Inspection.Validation.State == "valid" {
			t.Fatalf("invalid preview: %+v %v", preview, err)
		}
	}
	pdf := receivedTestPDF(false)
	pdfPreview, err := s.PrepareReceivedInvoice("ordinary.pdf", base64.StdEncoding.EncodeToString(pdf))
	if err != nil || pdfPreview.PDF != base64.StdEncoding.EncodeToString(pdf) || pdfPreview.Inspection.Validation.State != "unsupported" {
		t.Fatal("ordinary PDF preview", err)
	}
}

func TestReceivedArchiveMisimportRestoreAndDelete(t *testing.T) {
	s, err := NewInvoiceService(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	original := receivedTestPDF(false)
	encoded := base64.StdEncoding.EncodeToString(original)
	result, err := s.ImportReceivedInvoice("original.pdf", encoded)
	if err != nil {
		t.Fatal(err)
	}
	id := result.Invoice.ID
	if result.Invoice.Status != "active" || len(result.Invoice.History) != 1 {
		t.Fatal("missing initial status/history")
	}
	if err := s.DeleteReceivedMisimport(id, "original.pdf", true); err == nil {
		t.Fatal("deleted active document")
	}
	archived, err := s.ChangeReceivedStatus(id, "archived", "")
	if err != nil || archived.Status != "archived" || len(archived.History) != 2 {
		t.Fatal(archived, err)
	}
	preview, err := s.PrepareReceivedInvoice("renamed.pdf", encoded)
	if err != nil || preview.Duplicate == nil || preview.Duplicate.Status != "archived" {
		t.Fatal("archive duplicate not detected", err)
	}
	duplicate, err := s.ImportReceivedInvoice("renamed.pdf", encoded)
	if err != nil || !duplicate.Duplicate || duplicate.Invoice.Status != "archived" || len(duplicate.Invoice.History) != 2 {
		t.Fatal("duplicate changed archive", err)
	}
	if err := s.DeleteReceivedMisimport(id, "original.pdf", true); err == nil {
		t.Fatal("deleted archived document")
	}
	if _, err := s.ChangeReceivedStatus(id, "misimport", "  "); err == nil {
		t.Fatal("accepted empty reason")
	}
	if _, err := s.ChangeReceivedStatus(id, "invalid-status", ""); err == nil {
		t.Fatal("accepted unknown status")
	}
	misimport, err := s.ChangeReceivedStatus(id, "misimport", "Testdatei")
	if err != nil || misimport.Status != "misimport" || misimport.History[2].Reason != "Testdatei" {
		t.Fatal(misimport, err)
	}
	if _, err := s.ChangeReceivedStatus(id, "archived", ""); err == nil {
		t.Fatal("silently converted misimport to archive")
	}
	if err := s.DeleteReceivedMisimport(id, "original.pdf", false); err == nil {
		t.Fatal("deleted without confirmation")
	}
	if err := s.DeleteReceivedMisimport(id, "wrong.pdf", true); err == nil {
		t.Fatal("deleted with wrong filename")
	}
	backup, err := s.ExportBackup()
	if err != nil {
		t.Fatal(err)
	}
	restored, err := NewInvoiceService(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	if err := restored.RestoreBackup(backup); err != nil {
		t.Fatal(err)
	}
	stored, data, err := restored.store.GetReceived(id)
	if err != nil || stored.Status != "misimport" || len(stored.History) != 3 || !bytes.Equal(original, data) {
		t.Fatal("backup lost original or history", err)
	}
	active, err := s.ChangeReceivedStatus(id, "active", "")
	if err != nil || active.Status != "active" || len(active.History) != 4 || active.History[2].Reason != "Testdatei" {
		t.Fatal("restore lost history", err)
	}
	if _, err := s.ChangeReceivedStatus(id, "misimport", "Testdatei"); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteReceivedMisimport(id, "original.pdf", true); err != nil {
		t.Fatal(err)
	}
	items, err := s.ListReceivedInvoices()
	if err != nil || len(items) != 0 {
		t.Fatal("document not deleted", err)
	}
	if _, _, err := s.store.GetReceived(id); err == nil {
		t.Fatal("deleted original still accessible")
	}
	if err := s.DeleteReceivedMisimport(id, "original.pdf", true); err == nil {
		t.Fatal("missing document reported as deleted")
	}
}
