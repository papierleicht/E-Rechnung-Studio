package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"papierleicht.de/e-rechnung-studio/internal/usagestats"
)

func TestSavingSettingsOffForgetsUsageCounters(t *testing.T) {
	s, err := NewInvoiceService(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	file := filepath.Join(t.TempDir(), usagestats.DetailsFileName)
	helping := true
	usage = &usagestats.Recorder{File: file, Enabled: func() bool { return helping }}
	t.Cleanup(func() { usage = nil })

	settings, err := s.store.AppSettings()
	if err != nil {
		t.Fatal(err)
	}
	settings.ImprovementStatistics = true
	settings.ExportDirectory = t.TempDir()
	if _, err := s.SaveAppSettings(settings); err != nil {
		t.Fatal(err)
	}
	(&UsageService{recorder: usage}).RecordArea("invoices")
	(&UsageService{recorder: usage}).RecordArea("nicht-im-menue")
	recordUse("rechnung_pdf")
	raw, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"bereich:invoices":1`, `"funktion:rechnung_pdf":1`} {
		if !strings.Contains(string(raw), want) {
			t.Fatalf("counters %s miss %s", raw, want)
		}
	}
	if strings.Contains(string(raw), "nicht-im-menue") {
		t.Fatalf("unknown area counted: %s", raw)
	}

	helping = false
	settings.ImprovementStatistics = false
	if _, err := s.SaveSettings(settings, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Fatalf("counters kept after saving the setting off: %v", err)
	}
}
