package app

import (
	"os"
	"path/filepath"
	"testing"

	"papierleicht.de/e-rechnung-studio/internal/model"
	"papierleicht.de/e-rechnung-studio/internal/store"
)

func TestMigrationArchivesFilesOnceAndTakesOverLists(t *testing.T) {
	invoices, err := NewInvoiceService(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer invoices.Close()
	if err = invoices.store.SaveCompany(model.Company{Name: "Beispiel Kunde", Address: model.Address{Country: "DE"}}); err != nil {
		t.Fatal(err)
	}
	m := NewMigrationService(invoices)
	overview, err := m.Overview()
	if err != nil || len(overview.Sources) == 0 || overview.Total != 0 {
		t.Fatalf("overview %+v err=%v", overview, err)
	}

	dir := filepath.Join(t.TempDir(), "invoiz-Rechnungen")
	xml, err := os.ReadFile(filepath.Join("..", "einvoice", "testdata", "cii.xml"))
	if err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string][]byte{"2025/lieferant.xml": xml, "2025/scan.pdf": []byte("%PDF-1.4"), "notiz.txt": []byte("x")} {
		path := filepath.Join(dir, name)
		os.MkdirAll(filepath.Dir(path), 0o700)
		if err = os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
	}

	if _, err = m.PreviewFiles([]string{"relativ"}); err == nil {
		t.Fatal("relative path accepted")
	}
	preview, err := m.PreviewFiles([]string{dir})
	if err != nil || preview.New != 2 || preview.EInvoices != 1 || len(preview.Skipped) != 1 {
		t.Fatalf("preview %+v err=%v", preview, err)
	}
	if _, err = m.ImportFiles("unbekannt", []string{dir}); err == nil {
		t.Fatal("unknown source accepted")
	}
	result, err := m.ImportFiles("invoiz", []string{dir})
	if err != nil || result.Imported != 2 || result.Duplicates != 0 {
		t.Fatalf("import %+v err=%v", result, err)
	}
	again, err := m.ImportFiles("invoiz", []string{dir})
	if err != nil || again.Imported != 0 || again.Duplicates != 2 {
		t.Fatalf("second import %+v err=%v", again, err)
	}
	if p, _ := m.PreviewFiles([]string{dir}); p.Duplicates != 2 || p.New != 0 {
		t.Fatalf("preview after import %+v", p)
	}
	archive, err := m.Archive(store.LegacyFilter{Direction: "incoming"})
	if err != nil || len(archive) != 1 || archive[0].Source != "invoiz" || archive[0].Partner != "Beispiel Lieferant" {
		t.Fatalf("archive %+v err=%v", archive, err)
	}
	target := filepath.Join(t.TempDir(), "kopie.xml")
	if err = m.SaveArchiveFile(archive[0].ID, archive[0].Files[0].SHA256, target); err != nil {
		t.Fatal(err)
	}
	if copied, _ := os.ReadFile(target); string(copied) != string(xml) {
		t.Fatal("saved copy differs")
	}
	if err = m.SaveArchiveFile(archive[0].ID, archive[0].Files[0].SHA256, target); err == nil {
		t.Fatal("existing file overwritten")
	}

	list := filepath.Join(t.TempDir(), "kunden.csv")
	if err = os.WriteFile(list, []byte("Firma;PLZ;Ort\nNeu GmbH;10115;Berlin\nNeu GmbH;10115;Berlin\n;;\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	lp, err := m.PreviewList("customers", list, nil)
	if err != nil || lp.New != 1 || lp.Mapping["name"] != 0 {
		t.Fatalf("list preview %+v err=%v", lp, err)
	}
	saved, err := m.ImportList("customers", list, lp.Mapping)
	if err != nil || saved.Created != 1 || saved.Existing != 1 {
		t.Fatalf("list import %+v err=%v", saved, err)
	}
	customers, _ := invoices.store.Customers()
	if len(customers) != 1 || customers[0].Address.City != "Berlin" {
		t.Fatalf("customers %+v", customers)
	}
	if report, err := invoices.store.VerifyAudit(nil); err != nil || !report.OK {
		t.Fatalf("audit %+v err=%v", report, err)
	}
}
