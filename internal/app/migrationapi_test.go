package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"papierleicht.de/e-rechnung-studio/internal/store"
	"papierleicht.de/e-rechnung-studio/internal/umzug"
)

type fakeProvider struct{ key string }

func (f fakeProvider) Check(context.Context) (string, error) {
	if f.key != "richtiger-schluessel" {
		return "", umzug.ErrUnauthorized
	}
	return "Testfirma", nil
}

func (fakeProvider) Customers(context.Context) (umzug.Table, error) {
	return umzug.Table{Columns: []string{"Firma", "PLZ", "Ort"}, Rows: [][]string{{"API Kunde GmbH", "10115", "Berlin"}}}, nil
}

func (fakeProvider) Documents(context.Context) ([]umzug.RemoteRef, error) {
	return []umzug.RemoteRef{
		{ID: "a1", Meta: umzug.Booking{Number: "RE-1", Date: "2025-01-02", Gross: "119.00", Direction: "outgoing", Text: "API Kunde GmbH", Source: "api"}},
		{ID: "a2", Meta: umzug.Booking{Number: "RE-2"}},
	}, nil
}

func (fakeProvider) Files(_ context.Context, ref umzug.RemoteRef) ([]umzug.RemoteFile, error) {
	if ref.ID == "a2" {
		return nil, errors.New("Datei fehlt")
	}
	return []umzug.RemoteFile{{Name: "RE-1.pdf", Data: []byte("%PDF eins")}}, nil
}

func waitAPI(t *testing.T, m *MigrationService) APIStatus {
	t.Helper()
	for i := 0; i < 200; i++ {
		st := m.APIStatus()
		if st.State != "running" && st.State != "checking" {
			return st
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("run did not end")
	return APIStatus{}
}

func TestMigrationOverAnAPI(t *testing.T) {
	umzug.RegisterProvider("andere", func(key string) umzug.Provider { return fakeProvider{key} })
	t.Cleanup(func() { umzug.RegisterProvider("andere", nil) })
	invoices, err := NewInvoiceService(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer invoices.Close()
	m := NewMigrationService(invoices)
	if st, err := m.ConnectAPI("andere", "falscher-schluessel"); err == nil || st.State != "failed" {
		t.Fatalf("wrong key accepted: %+v", st)
	}
	if _, err := m.ConnectAPI("invoiz", "richtiger-schluessel"); err == nil {
		t.Fatal("source without API accepted")
	}
	if _, err = m.ConnectAPI("andere", "richtiger-schluessel"); err != nil {
		t.Fatal(err)
	}
	st := waitAPI(t, m)
	if st.State != "done" || st.Account != "Testfirma" || st.Total != 2 || st.Imported != 1 || len(st.Failed) != 1 || st.Customers != 1 {
		t.Fatalf("status %+v", st)
	}
	docs, _ := m.Archive(store.LegacyFilter{})
	if len(docs) != 1 || docs[0].Basis != "api" || docs[0].Number != "RE-1" || docs[0].Gross != "119.00" || docs[0].SourceID != "andere:a1" {
		t.Fatalf("archive %+v", docs)
	}
	// A second run skips what is archived without loading it again.
	if _, err = m.ConnectAPI("andere", "richtiger-schluessel"); err != nil {
		t.Fatal(err)
	}
	if st = waitAPI(t, m); st.Duplicates != 1 || st.Imported != 0 {
		t.Fatalf("second run %+v", st)
	}
	list, err := m.PreviewAPIList(nil)
	if err != nil || list.New != 1 {
		t.Fatalf("customers %+v err=%v", list, err)
	}
	if saved, err := m.ImportAPIList(list.Mapping); err != nil || saved.Created != 1 {
		t.Fatalf("saved %+v err=%v", saved, err)
	}
}
