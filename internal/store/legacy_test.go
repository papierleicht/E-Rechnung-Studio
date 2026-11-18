package store

import (
	"testing"

	"papierleicht.de/e-rechnung-studio/internal/model"
)

func legacyDoc(source, number, date string, data string) model.LegacyDocument {
	return model.LegacyDocument{
		Source: source, SourceID: sumOf([]byte(data)), Direction: "outgoing", Kind: "invoice", Number: number, Date: date,
		Partner: "Beispiel GmbH", Currency: "EUR", Net: "100.00", Tax: "19.00", Gross: "119.00", EInvoice: true,
		Files: []model.LegacyFile{{Name: number + ".pdf", MIME: "application/pdf", Data: []byte(data)}},
	}
}

func TestLegacyDocumentsAreAddedOnceAndNeverChange(t *testing.T) {
	s := openTest(t)
	saved, added, err := s.AddLegacyDocument(legacyDoc("invoiz", "RE-1", "2025-03-01", "pdf eins"))
	if err != nil || !added {
		t.Fatalf("added=%v err=%v", added, err)
	}
	if saved.Files[0].SHA256 == "" || saved.Files[0].Data != nil {
		t.Fatalf("file not summarized: %+v", saved.Files[0])
	}
	// The same file again, even from another source, is skipped.
	again := legacyDoc("andere", "RE-1-Kopie", "2025-03-01", "pdf eins")
	if _, added, err = s.AddLegacyDocument(again); err != nil || added {
		t.Fatalf("duplicate added=%v err=%v", added, err)
	}
	if _, err = s.DB().Exec(`UPDATE legacy_documents SET number='X'`); err == nil {
		t.Fatal("legacy document changed")
	}
	if _, err = s.DB().Exec(`DELETE FROM legacy_files`); err == nil {
		t.Fatal("legacy file deleted")
	}
	f, err := s.LegacyFile(saved.ID, saved.Files[0].SHA256)
	if err != nil || string(f.Data) != "pdf eins" {
		t.Fatalf("file %q err=%v", f.Data, err)
	}
	if report := verified(t, s); !report.OK {
		t.Fatalf("audit: %+v", report.Problems)
	}
	if got := actionsOf(t, s, AuditLegacy, saved.ID); len(got) != 1 || got[0] != AuditCreated {
		t.Fatalf("audit actions %v", got)
	}
}

func TestLegacyListFiltersAndYearSums(t *testing.T) {
	s := openTest(t)
	for _, d := range []model.LegacyDocument{
		legacyDoc("invoiz", "RE-1", "2025-03-01", "a"),
		legacyDoc("invoiz", "RE-2", "2025-04-01", "b"),
		legacyDoc("invoiz", "RE-3", "2024-12-01", "c"),
	} {
		if _, _, err := s.AddLegacyDocument(d); err != nil {
			t.Fatal(err)
		}
	}
	credit := legacyDoc("invoiz", "GS-1", "2025-05-01", "d")
	credit.Kind = "credit-note"
	plain := legacyDoc("invoiz", "", "", "e")
	plain.EInvoice, plain.Direction, plain.Kind = false, "", "document"
	plain.Files[0].Name = "Scan_100_%.pdf"
	for _, d := range []model.LegacyDocument{credit, plain} {
		if _, _, err := s.AddLegacyDocument(d); err != nil {
			t.Fatal(err)
		}
	}
	list, err := s.LegacyDocuments(LegacyFilter{Year: "2025"})
	if err != nil || len(list) != 3 || list[0].Number != "GS-1" {
		t.Fatalf("list %+v err=%v", list, err)
	}
	if list, _ = s.LegacyDocuments(LegacyFilter{Query: "100_%"}); len(list) != 1 {
		t.Fatalf("file name search: %+v", list)
	}
	years, err := s.LegacyYears()
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]model.LegacyYear{
		"2025outgoing": {Count: 3, Structured: 3, Net: "100.00", Tax: "19.00", Gross: "119.00"},
		"2024outgoing": {Count: 1, Structured: 1, Net: "100.00", Tax: "19.00", Gross: "119.00"},
		"ohne Datum":   {Count: 1, Structured: 0, Net: "0.00", Tax: "0.00", Gross: "0.00"},
	}
	if len(years) != len(want) || years[len(years)-1].Year != "ohne Datum" || years[0].Year != "2025" {
		t.Fatalf("years %+v", years)
	}
	for _, y := range years {
		w := want[y.Year+y.Direction]
		if y.Count != w.Count || y.Structured != w.Structured || y.Gross != w.Gross || y.Net != w.Net {
			t.Fatalf("year %+v want %+v", y, w)
		}
	}
}

func TestLegacyArchiveSurvivesBackupsAndRestoresOnlyAdd(t *testing.T) {
	s := openTest(t)
	saved, _, err := s.AddLegacyDocument(legacyDoc("invoiz", "RE-1", "2025-03-01", "original"))
	if err != nil {
		t.Fatal(err)
	}
	doc, open := snapshotOf(t, s)
	backup, err := s.ExportBackup()
	if err != nil {
		t.Fatal(err)
	}

	// A fresh install gets the archive from either kind of backup.
	for _, restore := range []func(*Store) error{
		func(o *Store) error { return o.RestoreBackup(backup) },
		func(o *Store) error {
			return o.RestoreSnapshot(doc, open, BackupEvent{Kind: BackupEventRestore, OK: true})
		},
	} {
		other := openTest(t)
		if err = restore(other); err != nil {
			t.Fatal(err)
		}
		f, err := other.LegacyFile(saved.ID, saved.Files[0].SHA256)
		if err != nil || string(f.Data) != "original" {
			t.Fatalf("restored file %q err=%v", f.Data, err)
		}
		if report := verified(t, other); !report.OK {
			t.Fatalf("audit after restore: %+v", report.Problems)
		}
	}

	// A restore of an older state keeps what was archived since.
	later, _, err := s.AddLegacyDocument(legacyDoc("invoiz", "RE-2", "2025-04-01", "später"))
	if err != nil {
		t.Fatal(err)
	}
	if err = s.RestoreBackup(backup); err != nil {
		t.Fatal(err)
	}
	if _, err = s.LegacyFile(later.ID, later.Files[0].SHA256); err != nil {
		t.Fatalf("restore removed a legacy document: %v", err)
	}
	if report := verified(t, s); !report.OK {
		t.Fatalf("audit after restore: %+v", report.Problems)
	}
}
