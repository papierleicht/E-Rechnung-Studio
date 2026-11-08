package app

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"papierleicht.de/e-rechnung-studio/internal/pdfinvoice"
)

func TestReceivedOriginalPersistenceAndBackup(t *testing.T) {
	root := t.TempDir()
	s, err := NewInvoiceService(root)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { s.Close() }()
	if _, err := s.SaveCompany(testCompany()); err != nil {
		t.Fatal(err)
	}
	view, err := s.ValidateInvoice(testDraft())
	if err != nil {
		t.Fatal(err)
	}
	pdf, err := pdfinvoice.Render([]byte(view.XML))
	if err != nil {
		t.Fatal(err)
	}
	for name, original := range map[string][]byte{"lieferant.xml": []byte(view.XML), "lieferant.pdf": pdf} {
		t.Run(name, func(t *testing.T) {
			encoded := base64.StdEncoding.EncodeToString(original)
			result, err := s.ImportReceivedInvoice(name, encoded)
			if err != nil {
				t.Fatal(err)
			}
			if result.Duplicate || result.Invoice.Inspection.Validation.State != "valid" || result.Invoice.Inspection.InvoiceNumber != testDraft().Number {
				t.Fatalf("unexpected import: %+v", result)
			}
			duplicate, err := s.ImportReceivedInvoice("renamed"+filepath.Ext(name), encoded)
			if err != nil || !duplicate.Duplicate || duplicate.Invoice.Filename != name {
				t.Fatalf("deduplication: %+v %v", duplicate, err)
			}
			inspection, err := s.RecheckReceivedInvoice(result.Invoice.ID)
			if err != nil || inspection.Validation.State != "valid" {
				t.Fatalf("recheck: %+v %v", inspection, err)
			}
			path := filepath.Join(t.TempDir(), name)
			if err := s.SaveReceivedOriginal(result.Invoice.ID, path); err != nil {
				t.Fatal(err)
			}
			saved, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(saved, original) {
				t.Fatal("export changed original", err)
			}
			if err := s.SaveReceivedOriginal(result.Invoice.ID, path); err == nil {
				t.Fatal("overwrote existing file")
			}
		})
	}
	backup, err := s.ExportBackup()
	if err != nil {
		t.Fatal(err)
	}
	before, err := s.ListReceivedInvoices()
	if err != nil || len(before) != 2 {
		t.Fatal(before, err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = NewInvoiceService(root)
	if err != nil {
		t.Fatal(err)
	}
	if rows, err := s.ListReceivedInvoices(); err != nil || len(rows) != 2 {
		t.Fatal("restart lost originals", err)
	}
	restored, err := NewInvoiceService(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	if err := restored.RestoreBackup(backup); err != nil {
		t.Fatal(err)
	}
	for _, item := range before {
		_, expected, _ := s.store.GetReceived(item.ID)
		got, actual, err := restored.store.GetReceived(item.ID)
		if err != nil || !bytes.Equal(expected, actual) || got.Filename != item.Filename {
			t.Fatal("backup lost original", err)
		}
	}
	if issued, err := restored.ListInvoices(); err != nil || len(issued) != 0 {
		t.Fatal("incoming invoices leaked into outgoing history", err)
	}
	var old map[string]any
	if err := json.Unmarshal([]byte(backup), &old); err != nil {
		t.Fatal(err)
	}
	delete(old, "received")
	old["version"] = 2
	legacy, _ := json.Marshal(old)
	if err := restored.RestoreBackup(string(legacy)); err != nil {
		t.Fatal("legacy restore failed", err)
	}
	if rows, err := restored.ListReceivedInvoices(); err != nil || len(rows) != 0 {
		t.Fatal("legacy restore retained unrelated incoming invoices", err)
	}
}

// Minimal PDF fixtures exercise name-tree and associated-file extraction without
// relying on the application's own PDF writer to produce the same structure.
func receivedTestPDF(associatedOnly bool, attachments ...string) []byte {
	objects := []string{"", "<< /Type /Pages /Kids [3 0 R] /Count 1 >>", "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] >>"}
	var names, refs strings.Builder
	for i, content := range attachments {
		specID := len(objects) + 1
		objects = append(objects, fmt.Sprintf("<< /Type /Filespec /F (invoice-%d.xml) /EF << /F %d 0 R >> >>", i, specID+1), fmt.Sprintf("<< /Type /EmbeddedFile /Length %d >>\nstream\n%s\nendstream", len(content), content))
		fmt.Fprintf(&names, "(invoice-%d.xml) %d 0 R ", i, specID)
		fmt.Fprintf(&refs, "%d 0 R ", specID)
	}
	objects[0] = fmt.Sprintf("<< /Type /Catalog /Pages 2 0 R /AF [%s]", refs.String())
	if !associatedOnly {
		objects[0] += fmt.Sprintf(" /Names << /EmbeddedFiles << /Names [%s] >> >>", names.String())
	}
	objects[0] += " >>"
	var out bytes.Buffer
	out.WriteString("%PDF-1.7\n")
	offsets := []int{0}
	for i, object := range objects {
		offsets = append(offsets, out.Len())
		fmt.Fprintf(&out, "%d 0 obj\n%s\nendobj\n", i+1, object)
	}
	xref := out.Len()
	fmt.Fprintf(&out, "xref\n0 %d\n0000000000 65535 f \n", len(offsets))
	for _, offset := range offsets[1:] {
		fmt.Fprintf(&out, "%010d 00000 n \n", offset)
	}
	fmt.Fprintf(&out, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(offsets), xref)
	return out.Bytes()
}

func TestReceivedPDFInspectionEdgeCases(t *testing.T) {
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
	for _, tc := range []struct {
		name        string
		data        []byte
		state, rule string
	}{
		{"ordinary", receivedTestPDF(false), "unsupported", "PDF-NO-XML"},
		{"name-tree-and-AF-deduplicated", receivedTestPDF(false, view.XML), "valid", ""},
		{"AF-only", receivedTestPDF(true, view.XML), "valid", ""},
		{"multiple", receivedTestPDF(false, view.XML, view.XML), "unsupported", "PDF-MULTIPLE-XML"},
		{"malformed-XML", receivedTestPDF(false, "<broken"), "invalid", "PARSE"},
		{"doctype", receivedTestPDF(false, `<!DOCTYPE Invoice SYSTEM "https://example.invalid"><Invoice/>`), "error", "DOCTYPE"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := s.InspectInvoiceFile("document.PDF", base64.StdEncoding.EncodeToString(tc.data))
			if err != nil || result.Validation.State != tc.state {
				t.Fatalf("%+v %v", result, err)
			}
			if tc.rule != "" && (len(result.Validation.Issues) == 0 || result.Validation.Issues[0].RuleID != tc.rule) {
				t.Fatalf("missing rule %s: %+v", tc.rule, result)
			}
		})
	}
	for _, tc := range []struct{ name, data string }{{"bad.pdf", "not a pdf"}, {"empty.xml", ""}, {"bad.exe", "<Invoice/>"}, {"large.xml", strings.Repeat("x", maxReceivedSize+1)}} {
		if _, err := s.ImportReceivedInvoice(tc.name, base64.StdEncoding.EncodeToString([]byte(tc.data))); err == nil {
			t.Fatalf("accepted %s", tc.name)
		}
	}
	if rows, err := s.ListReceivedInvoices(); err != nil || len(rows) != 0 {
		t.Fatal("inspection or failed import stored a document", err)
	}
}
