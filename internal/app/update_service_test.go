package app

import (
	"context"
	"errors"
	"testing"
	"testing/fstest"

	"github.com/wailsapp/wails/v3/pkg/updater"
	"papierleicht.de/e-rechnung-studio/internal/model"
	"papierleicht.de/e-rechnung-studio/internal/store"
)

type fakeUpdater struct {
	release    *updater.Release
	installErr error
	installs   int
	restarts   int
}

func (f *fakeUpdater) Check(context.Context) (*updater.Release, error) { return f.release, nil }
func (f *fakeUpdater) DownloadAndInstall(context.Context) error {
	f.installs++
	return f.installErr
}
func (f *fakeUpdater) Restart(context.Context) error { f.restarts++; return nil }

var testNotes = fstest.MapFS{
	"LIESMICH.md": {Data: []byte("# Versionshinweise")},
	"1.2.0.md":    {Data: []byte("## Neu\n\n- Marge")},
	"1.9.0.md":    {Data: []byte("## Behoben\n\n- Neun")},
	"1.10.0.md":   {Data: []byte("## Neu\n\n- Zehn\n")},
	"1.11.0.md":   {Data: []byte("## Neu\n\n- Zukunft")},
}

func withVersion(t *testing.T, v string) {
	t.Helper()
	old := currentVersion
	currentVersion = v
	t.Cleanup(func() { currentVersion = old })
}

func TestUpdateCheckInstallAndSkip(t *testing.T) {
	withVersion(t, "1.2.0")
	invoices, err := NewInvoiceService(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer invoices.Close()
	fake := &fakeUpdater{release: &updater.Release{Version: "1.3.0", Notes: "## Neu\n\n- Umzug\n"}}
	s := &UpdateService{updater: fake, store: invoices.store}

	status, err := s.Check()
	if err != nil || !status.UpdateAvailable || status.AvailableVersion != "1.3.0" || status.Notes != "## Neu\n\n- Umzug" || status.Skipped {
		t.Fatalf("status %+v err=%v", status, err)
	}
	if err = s.Skip("1.3.0"); err != nil {
		t.Fatal(err)
	}
	if status, _ = s.Check(); !status.Skipped {
		t.Fatalf("not skipped: %+v", status)
	}
	fake.release.Version = "1.3.1"
	if status, _ = s.Check(); status.Skipped {
		t.Fatalf("newer version skipped: %+v", status)
	}

	if s.Progress().Phase != "idle" {
		t.Fatalf("progress %+v", s.Progress())
	}
	s.onUpdaterEvent(updater.EventDownloadProgress, []any{updater.Progress{Written: 5, Total: 10}})
	if p := s.Progress(); p.Written != 5 || p.Total != 10 {
		t.Fatalf("progress %+v", p)
	}
	if err = s.Install(); err != nil || s.Progress().Phase != "ready" || fake.installs != 1 {
		t.Fatalf("install err=%v progress %+v", err, s.Progress())
	}
	if err = s.Restart(); err != nil || fake.restarts != 1 {
		t.Fatalf("restart err=%v", err)
	}
	fake.installErr = errors.New("network")
	if err = s.Install(); err == nil || s.Progress().Phase != "failed" || s.Progress().Message == "" {
		t.Fatalf("failed install err=%v progress %+v", err, s.Progress())
	}
}

func TestReleaseNotesAndWhatsNew(t *testing.T) {
	withVersion(t, "1.10.0")
	invoices, err := NewInvoiceService(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer invoices.Close()
	s := &UpdateService{store: invoices.store, notes: testNotes}

	all, err := s.ReleaseNotes()
	if err != nil || len(all) != 4 || all[0].Version != "1.11.0" || all[1].Version != "1.10.0" || all[1].Notes != "## Neu\n\n- Zehn" || all[3].Version != "1.2.0" {
		t.Fatalf("notes %+v err=%v", all, err)
	}

	// A new installation starts without notes and remembers the version.
	if news, err := s.WhatsNew(); err != nil || len(news) != 0 {
		t.Fatalf("fresh %+v err=%v", news, err)
	}
	if st, _ := invoices.store.UpdateState(); st.Seen != "1.10.0" {
		t.Fatalf("seen %+v", st)
	}

	// An update from a version that did not remember anything shows this version.
	invoices.store.SaveUpdateState(storeState(""))
	if err = invoices.store.SaveCompany(model.Company{Name: "Muster GmbH", Address: model.Address{Country: "DE"}}); err != nil {
		t.Fatal(err)
	}
	if news, _ := s.WhatsNew(); len(news) != 1 || news[0].Version != "1.10.0" {
		t.Fatalf("after old update %+v", news)
	}

	// From 1.2.0 the customer sees everything since, but nothing ahead.
	invoices.store.SaveUpdateState(storeState("1.2.0"))
	news, _ := s.WhatsNew()
	if len(news) != 2 || news[0].Version != "1.10.0" || news[1].Version != "1.9.0" {
		t.Fatalf("since 1.2.0 %+v", news)
	}
	if err = s.MarkSeen(); err != nil {
		t.Fatal(err)
	}
	if news, _ = s.WhatsNew(); len(news) != 0 {
		t.Fatalf("after seen %+v", news)
	}
}

func storeState(seen string) store.UpdateState { return store.UpdateState{Seen: seen} }
