package app

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"papierleicht.de/e-rechnung-studio/internal/model"
)

func TestAutomaticInvoiceExport(t *testing.T) {
	s, err := NewInvoiceService(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err = s.SaveCompany(testCompany()); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "Neue Ablage", "Rechnungen")
	if _, err = s.SaveAppSettings(model.AppSettings{ExportDirectory: dir}); err != nil {
		t.Fatal(err)
	}
	d := testDraft()
	d.Number = "RE/2026:42"
	wanted := filepath.Join(dir, "Rechnung-RE-2026-42.pdf")
	if err = os.WriteFile(wanted, []byte("existing customer file"), 0600); err != nil {
		t.Fatal(err)
	}
	issued, err := s.IssueInvoice(d)
	if err != nil || issued.Export.Error != "" || issued.Export.PDFPath == "" {
		t.Fatalf("automatic export: %v %+v", err, issued.Export)
	}
	if issued.Export.PDFPath == wanted || filepath.Dir(issued.Export.PDFPath) != dir {
		t.Fatal("unsafe export path", issued.Export.PDFPath)
	}
	pdf, _ := os.ReadFile(issued.Export.PDFPath)
	immutable, _ := s.invoicePDF(issued.Draft.ID)
	if !bytes.Equal(pdf, immutable) {
		t.Fatal("export differs from issued PDF")
	}
	original, _ := os.ReadFile(wanted)
	if string(original) != "existing customer file" {
		t.Fatal("existing file overwritten")
	}
	again, err := s.ExportIssuedInvoice(issued.Draft.ID)
	if err != nil || again.PDFPath != issued.Export.PDFPath {
		t.Fatal("retry not idempotent", err)
	}
	if err = os.Remove(again.PDFPath); err != nil {
		t.Fatal(err)
	}
	again, err = s.ExportIssuedInvoice(issued.Draft.ID)
	if err != nil {
		t.Fatal(err)
	}
	regenerated, _ := os.ReadFile(again.PDFPath)
	if !bytes.Equal(regenerated, immutable) {
		t.Fatal("missing export not restored")
	}
	opened, err := s.OpenInvoice(issued.Draft.ID)
	if err != nil || opened.Export.PDFPath != again.PDFPath {
		t.Fatal("export path not retained", err)
	}
	boot, err := s.Bootstrap()
	if err != nil || boot.Settings.ExportDirectory != dir {
		t.Fatal("folder not retained", err)
	}
	if _, err = s.SaveAppSettings(model.AppSettings{ExportDirectory: "relative"}); err == nil {
		t.Fatal("relative directory accepted")
	}
	// An existing PDF remains openable even when the current export preference
	// is unavailable. Replaced files must never be opened as this invoice.
	blocked := filepath.Join(t.TempDir(), "not-a-directory")
	os.WriteFile(blocked, []byte("blocked"), 0600)
	s.store.SaveAppSettings(model.AppSettings{ExportDirectory: blocked})
	path, err := s.invoicePDFExportPath(issued.Draft.ID)
	if err != nil || path != again.PDFPath {
		t.Fatal("existing PDF depends on current folder", err)
	}
	s.store.SaveAppSettings(model.AppSettings{ExportDirectory: dir})
	os.WriteFile(path, []byte("replaced file"), 0600)
	repaired, err := s.invoicePDFExportPath(issued.Draft.ID)
	if err != nil || repaired == path {
		t.Fatal("replaced PDF opened", err)
	}
	replacement, _ := os.ReadFile(path)
	if string(replacement) != "replaced file" {
		t.Fatal("replaced file overwritten")
	}
}

func TestExportFailureDoesNotUndoIssuance(t *testing.T) {
	s, err := NewInvoiceService(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	s.SaveCompany(testCompany())
	dir := filepath.Join(t.TempDir(), "exports")
	s.SaveAppSettings(model.AppSettings{ExportDirectory: dir})
	os.Remove(dir)
	os.WriteFile(dir, []byte("blocked folder"), 0600)
	issued, err := s.IssueInvoice(testDraft())
	if err != nil || issued.Draft.Status != "issued" || issued.Export.Error == "" {
		t.Fatalf("issuance/export state lost: %v %+v", err, issued.Export)
	}
	if _, err := s.IssueInvoice(issued.Draft); err == nil {
		t.Fatal("invoice issued twice")
	}
	if _, err = s.invoicePDF(issued.Draft.ID); err != nil {
		t.Fatal("immutable PDF missing", err)
	}
	os.Remove(dir)
	result, err := s.ExportIssuedInvoice(issued.Draft.ID)
	if err != nil || result.Error != "" || result.PDFPath == "" {
		t.Fatalf("retry failed: %v %+v", err, result)
	}
	items, _ := s.ListInvoices()
	if len(items) != 1 || items[0].Number != issued.Draft.Number {
		t.Fatal("retry created another invoice")
	}
}

func TestXRechnungAutomaticallyExportsBothArtifacts(t *testing.T) {
	s, err := NewInvoiceService(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	c := testCompany()
	c.Endpoint, c.EndpointScheme, c.Phone = c.Email, "EM", "+49 30 1234567"
	s.SaveCompany(c)
	s.SaveAppSettings(model.AppSettings{ExportDirectory: t.TempDir()})
	d := testDraft()
	d.Profile, d.Syntax = "xrechnung-ubl", "ubl"
	d.Customer.Endpoint, d.Customer.EndpointScheme = "kunde@example.de", "EM"
	d.BuyerReference = "KUNDE-42"
	issued, err := s.IssueInvoice(d)
	if err != nil || issued.Export.Error != "" || issued.Export.XMLPath == "" || issued.Export.PDFPath == "" {
		t.Fatalf("XRechnung auto export: %v %+v", err, issued.Export)
	}
	xml, _ := os.ReadFile(issued.Export.XMLPath)
	if string(xml) != issued.XML || !strings.Contains(string(xml), "<Invoice") {
		t.Fatal("XML export mismatch")
	}
}

func TestCustomerInvoiceSettingsRemainSeparateFromIssuedInvoice(t *testing.T) {
	s, err := NewInvoiceService(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	s.SaveCompany(testCompany())
	c, err := s.SaveCustomer(testDraft().Customer)
	if err != nil {
		t.Fatal(err)
	}
	d := testDraft()
	d.Customer, d.CustomerID = c, c.ID
	d.Profile = "facturx-basic"
	saved, err := s.SaveInvoice(d)
	if err != nil {
		t.Fatal(err)
	}
	boot, _ := s.Bootstrap()
	defaults := boot.Customers[0].InvoiceSettings
	if defaults == nil || defaults.Profile != "facturx-basic" {
		t.Fatal("defaults missing", defaults)
	}
	issued, err := s.IssueInvoice(saved.Draft)
	if err != nil {
		t.Fatal(err)
	}
	before, _ := s.invoicePDF(issued.Draft.ID)
	d.ID, d.Number = "", "NEXT-42"
	d.Profile = "facturx-en16931"
	if _, err = s.SaveInvoice(d); err != nil {
		t.Fatal(err)
	}
	// A contact edit from an older customer object must retain new defaults.
	c.Name = "Updated customer name"
	if _, err = s.SaveCustomer(c); err != nil {
		t.Fatal(err)
	}
	boot, _ = s.Bootstrap()
	opened, err := s.OpenInvoice(issued.Draft.ID)
	if err != nil || opened.Draft.Profile != "facturx-basic" {
		t.Fatal("issued settings changed", err)
	}
	after, _ := s.invoicePDF(issued.Draft.ID)
	if !bytes.Equal(before, after) {
		t.Fatal("issued PDF changed")
	}
}

func TestDefaultExportDirectory(t *testing.T) {
	t.Setenv("E_RECHNUNG_STUDIO_EXPORT_DIR", "")
	s, err := NewInvoiceService(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	settings, err := s.GetAppSettings()
	documents, docErr := documentsDirectory()
	if err != nil || docErr != nil || settings.ExportDirectory != filepath.Join(documents, "Rechnungen") || settings.Appearance != "system" {
		t.Fatal(settings, err, docErr)
	}
}

func TestSaveAppSettingsValidatesAppearance(t *testing.T) {
	s, err := NewInvoiceService(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	dir := t.TempDir()
	settings, err := s.SaveAppSettings(model.AppSettings{ExportDirectory: dir, Appearance: "dark"})
	if err != nil || settings.Appearance != "dark" {
		t.Fatal(settings, err)
	}
	if _, err = s.SaveAppSettings(model.AppSettings{ExportDirectory: dir, Appearance: "sepia"}); err == nil {
		t.Fatal("invalid appearance was accepted")
	}
}

func TestMenuSectionsAreSavedAndUnknownOnesDropped(t *testing.T) {
	s, err := NewInvoiceService(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	dir := t.TempDir()
	if _, err := s.SaveAppSettings(model.AppSettings{ExportDirectory: dir, Sections: map[string]bool{"shops": false, "products": true, "invoices": false}}); err != nil {
		t.Fatal(err)
	}
	saved, err := s.GetAppSettings()
	if err != nil || len(saved.Sections) != 2 || saved.Sections["shops"] || !saved.Sections["products"] {
		t.Fatalf("sections %v %v", saved.Sections, err)
	}
	// Only unknown keys: nothing to keep.
	if v, _ := s.SaveAppSettings(model.AppSettings{ExportDirectory: dir, Sections: map[string]bool{"dashboard": false}}); v.Sections != nil {
		t.Fatalf("unknown sections kept: %v", v.Sections)
	}
}

func TestAppearanceSaveDoesNotRequireExportFolder(t *testing.T) {
	s, err := NewInvoiceService(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	// Simulate a formerly valid export location that is no longer a directory.
	path := filepath.Join(t.TempDir(), "unavailable")
	if err := os.WriteFile(path, []byte("occupied"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := s.store.SaveAppSettings(model.AppSettings{ExportDirectory: path, Appearance: "light"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveAppSettings(model.AppSettings{ExportDirectory: path, Appearance: "dark"}); err != nil {
		t.Fatal(err)
	}
	saved, err := s.GetAppSettings()
	if err != nil || saved.Appearance != "dark" {
		t.Fatal(saved, err)
	}
	// A newly selected invalid location must still be rejected.
	if _, err := s.SaveAppSettings(model.AppSettings{ExportDirectory: filepath.Join(path, "child"), Appearance: "light"}); err == nil {
		t.Fatal("invalid new export folder accepted")
	}
}
