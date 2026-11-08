package app

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"papierleicht.de/e-rechnung-studio/internal/model"
	"papierleicht.de/e-rechnung-studio/internal/profile"
	"path/filepath"
	"strings"
	"testing"
)

func TestInvoiceExportSupportsAllProfiles(t *testing.T) {
	s, err := NewInvoiceService(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	c := testCompany()
	c.Phone = "+49 30 1234567"
	c.Endpoint, c.EndpointScheme = c.Email, "EM"
	if _, err = s.SaveCompany(c); err != nil {
		t.Fatal(err)
	}
	for _, format := range []string{"facturx-en16931", "facturx-basic", "facturx-extended", "xrechnung-cii", "xrechnung-ubl"} {
		t.Run(format, func(t *testing.T) {
			d := testDraft()
			// Exercise QR image loading during export.
			d.PaymentMeansCode = "58"
			p, _ := profile.Get(format)
			d.Profile, d.Syntax = format, p.Syntax
			d.Number = "PARITY-" + format
			d.BuyerReference = "04011000-12345-03"
			d.Customer.Endpoint, d.Customer.EndpointScheme = d.Customer.Email, "EM"
			d.Lines[0].Description = "Konzeption und Umsetzung\nMit vollständiger Dokumentation."
			issued, err := s.IssueInvoice(d)
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(t.TempDir(), "invoice.pdf")
			if _, err = s.ExportInvoicePDF(issued.Draft.ID, path); err != nil {
				t.Fatal(err)
			}
			exported, err := os.ReadFile(path)
			if err != nil || !bytes.HasPrefix(exported, []byte("%PDF-")) {
				t.Fatal("invalid exported PDF", err)
			}
			if dir := os.Getenv("E_RECHNUNG_STUDIO_PDF_TEST_OUTPUT"); dir != "" {
				if err = os.MkdirAll(dir, 0700); err != nil {
					t.Fatal(err)
				}
				stem := filepath.Join(dir, "parity-"+format)
				for suffix, data := range map[string][]byte{".pdf": exported, ".xml": []byte(issued.XML)} {
					if err = os.WriteFile(stem+suffix, data, 0600); err != nil {
						t.Fatal(err)
					}
				}
			}
		})
	}
}

func TestIncompleteHTMLPreviewDoesNotIssueOrChangeDraft(t *testing.T) {
	s, err := NewInvoiceService(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err = s.SaveCompany(testCompany()); err != nil {
		t.Fatal(err)
	}
	d := model.NewDraft()
	preview, err := s.InvoiceDocumentHTML(d)
	if err != nil || preview == "" {
		t.Fatal("empty draft cannot be edited", err)
	}
	if d.Number != "" || d.Lines[0].Name != "" {
		t.Fatal("display placeholder mutated draft")
	}
	if _, err = s.IssueInvoice(d); err == nil {
		t.Fatal("preview made an invalid draft issuable")
	}
}

func TestHybridPDFExportAndBackup(t *testing.T) {
	s, err := NewInvoiceService(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	c := testCompany()
	if _, err = s.SaveCompany(c); err != nil {
		t.Fatal(err)
	}
	d := testDraft()
	d.Lines[0].Name = "Beratung & Gestaltung – Größe"
	d.Lines[0].Description = "Erste Zeile\nZweite Zeile"
	d.Number = "PDF-2026-001"
	issued, err := s.IssueInvoice(d)
	if err != nil {
		t.Fatal(err)
	}
	generated, err := s.store.Artifact(issued.Draft.ID, "facturx-pdf")
	if err != nil || !bytes.HasPrefix(generated, []byte("%PDF-")) {
		t.Fatal("PDF was not generated while issuing", err)
	}
	if !bytes.Contains(generated, []byte("/Creator (papierleicht E-Rechnung-Studio)")) || !bytes.Contains(generated, []byte("<xmp:CreatorTool>papierleicht E-Rechnung-Studio</xmp:CreatorTool>")) {
		t.Fatal("issued PDF does not name the product in its metadata")
	}
	path := filepath.Join(t.TempDir(), "invoice.pdf")
	if _, err = s.ExportInvoicePDF(issued.Draft.ID, path); err != nil {
		t.Fatal(err)
	}
	pdf, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(pdf, generated) {
		t.Fatal("export differs from automatically generated PDF")
	}
	xmlPath := filepath.Join(t.TempDir(), "invoice.xml")
	if _, err = s.ExportInvoiceXML(issued.Draft.ID, xmlPath); err != nil {
		t.Fatal(err)
	}
	exportedXML, err := os.ReadFile(xmlPath)
	if err != nil || string(exportedXML) != issued.XML {
		t.Fatal("XML export differs from immutable invoice", err)
	}
	for _, token := range []string{"%PDF-1.7", "factur-x.xml", "Alternative", "pdfaid:part", "EN 16931"} {
		if !bytes.Contains(pdf, []byte(token)) {
			t.Errorf("missing PDF marker %s", token)
		}
	}
	c.Name = "Changed seller"
	if _, err = s.SaveCompany(c); err != nil {
		t.Fatal(err)
	}
	reopened, err := s.OpenInvoice(issued.Draft.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reopened.Company.Name != issued.Company.Name || reopened.Company.Address != issued.Company.Address || reopened.Company.IBAN != issued.Company.IBAN || reopened.Totals != issued.Totals || reopened.XML != issued.XML {
		t.Fatal("issued invoice preview changed after company edit")
	}
	again, err := s.invoicePDF(issued.Draft.ID)
	if err != nil || !bytes.Equal(pdf, again) {
		t.Fatal("PDF changed after company edit", err)
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
	if err = restored.RestoreBackup(backup); err != nil {
		t.Fatal(err)
	}
	again, err = restored.invoicePDF(issued.Draft.ID)
	if err != nil || !bytes.Equal(pdf, again) {
		t.Fatal("binary PDF backup changed", err)
	}
	if dir := os.Getenv("E_RECHNUNG_STUDIO_PDF_TEST_OUTPUT"); dir != "" {
		if err = os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(dir, "invoice.pdf"), pdf, 0600); err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(dir, "invoice.xml"), []byte(issued.XML), 0600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestPDFDraftCannotOverwriteFile(t *testing.T) {
	s, err := NewInvoiceService(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	saved, err := s.SaveInvoice(model.NewDraft())
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "existing.pdf")
	os.WriteFile(path, []byte("original"), 0600)
	if _, err = s.ExportInvoicePDF(saved.Draft.ID, path); err == nil {
		t.Fatal("draft exported")
	}
	got, _ := os.ReadFile(path)
	if string(got) != "original" {
		t.Fatal("existing file overwritten")
	}
}

func TestHybridPDFScenarios(t *testing.T) {
	cases := []struct {
		name   string
		adjust func(*model.InvoiceDraft)
	}{
		{"small-business", func(d *model.InvoiceDraft) { d.TaxTreatment = "small-business" }},
		{"credit-note", func(d *model.InvoiceDraft) { d.Kind = "credit-note" }},
		{"reverse-charge", func(d *model.InvoiceDraft) { d.TaxTreatment = "reverse-charge" }},
		{"many-lines", func(d *model.InvoiceDraft) {
			first := d.Lines[0]
			d.Lines = nil
			for i := 1; i <= 65; i++ {
				line := first
				line.ID = fmt.Sprint(i)
				line.Name = fmt.Sprintf("Leistung Nr. %03d", i)
				line.Description = "Konzeption, technische Ausarbeitung und Dokumentation."
				d.Lines = append(d.Lines, line)
			}
		}},
		{"long-description", func(d *model.InvoiceDraft) {
			d.Lines[0].Description = strings.Repeat("Ausführliche Leistungsbeschreibung mit Umlauten ä, ö, ü und mehreren Details. ", 150) + " ENDE-DER-BESCHREIBUNG"
		}},
		{"long-notes", func(d *model.InvoiceDraft) {
			d.Notes = strings.Repeat("Ausführlicher Hinweis mit vollständigen Angaben und mehreren Zeilen.\n", 100) + " ENDE-DER-HINWEISE"
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, err := NewInvoiceService(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			if _, err = s.SaveCompany(testCompany()); err != nil {
				t.Fatal(err)
			}
			d := testDraft()
			d.Number = "PDF-" + tc.name
			tc.adjust(&d)
			issued, err := s.IssueInvoice(d)
			if err != nil {
				t.Fatal(err)
			}
			pdf, err := s.invoicePDF(issued.Draft.ID)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.HasPrefix(pdf, []byte("%PDF-")) {
				t.Fatal("invalid PDF header")
			}
			if dir := os.Getenv("E_RECHNUNG_STUDIO_PDF_TEST_OUTPUT"); dir != "" {
				if err = os.WriteFile(filepath.Join(dir, tc.name+".pdf"), pdf, 0600); err != nil {
					t.Fatal(err)
				}
				if err = os.WriteFile(filepath.Join(dir, tc.name+".xml"), []byte(issued.XML), 0600); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

func TestLegacyXMLBackupStillRestores(t *testing.T) {
	s, err := NewInvoiceService(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	s.SaveCompany(testCompany())
	issued, err := s.IssueInvoice(testDraft())
	if err != nil {
		t.Fatal(err)
	}
	backup, err := s.ExportBackup()
	if err != nil {
		t.Fatal(err)
	}
	var legacy map[string]any
	if err = json.Unmarshal([]byte(backup), &legacy); err != nil {
		t.Fatal(err)
	}
	legacy["version"] = 1
	legacyArtifacts := make([]any, 0, 1)
	for _, raw := range legacy["artifacts"].([]any) {
		a := raw.(map[string]any)
		if a["kind"] != "cii-xml" {
			continue
		}
		decoded, e := base64.StdEncoding.DecodeString(a["data"].(string))
		if e != nil {
			t.Fatal(e)
		}
		a["data"] = string(decoded)
		delete(a, "encoding")
		legacyArtifacts = append(legacyArtifacts, a)
	}
	legacy["artifacts"] = legacyArtifacts
	encoded, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.RestoreBackup(string(encoded)); err != nil {
		t.Fatal(err)
	}
	restored, err := s.OpenInvoice(issued.Draft.ID)
	if err != nil || restored.XML != issued.XML {
		t.Fatal("legacy backup changed XML", err)
	}
}

func TestPDFMissingGlyphDoesNotOverwriteFile(t *testing.T) {
	s, err := NewInvoiceService(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err = s.SaveCompany(testCompany()); err != nil {
		t.Fatal(err)
	}
	d := testDraft()
	d.Number = ""
	d.Lines[0].Name = "合同"
	if _, err = s.IssueInvoice(d); err == nil {
		t.Fatal("invoice with unsupported PDF glyphs was issued")
	}
	invoices, err := s.ListInvoices()
	if err != nil || len(invoices) != 1 || invoices[0].Status != "draft" {
		t.Fatal("failed PDF generation did not leave a draft", err)
	}
	path := filepath.Join(t.TempDir(), "existing.pdf")
	if err = os.WriteFile(path, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = s.ExportInvoicePDF(invoices[0].ID, path); err == nil {
		t.Fatal("unsupported glyphs silently exported")
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != "original" {
		t.Fatal("existing file overwritten", err)
	}
	reopened, err := s.OpenInvoice(invoices[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	reopened.Draft.Lines[0].Name = "Vertrag"
	issued, err := s.IssueInvoice(reopened.Draft)
	if err != nil {
		t.Fatal(err)
	}
	if issued.Draft.Number != "RE-2026-0001" {
		t.Fatalf("failed PDF generation consumed an invoice number: %s", issued.Draft.Number)
	}
}
