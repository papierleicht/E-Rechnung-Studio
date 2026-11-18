package store

import (
	"papierleicht.de/e-rechnung-studio/internal/model"
	"path/filepath"
	"testing"
)

func TestSettingsTransactionRollsBackCompanyOnFailure(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	original := model.Company{Name: "Original"}
	if err := s.SaveCompany(original); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveAppSettings(model.AppSettings{Appearance: "light"}); err != nil {
		t.Fatal(err)
	}
	_, err = s.db.Exec(`CREATE TRIGGER fail_settings BEFORE UPDATE ON settings WHEN NEW.key='app' BEGIN SELECT RAISE(ABORT, 'simulated write failure'); END`)
	if err != nil {
		t.Fatal(err)
	}
	changed := model.Company{Name: "Changed"}
	if err := s.SaveSettings(model.AppSettings{Appearance: "dark"}, &changed); err == nil {
		t.Fatal("expected write failure")
	}
	company, err := s.Company()
	if err != nil || company.Name != "Original" {
		t.Fatal("partial company update", company, err)
	}
	settings, err := s.AppSettings()
	if err != nil || settings.Appearance != "light" {
		t.Fatal("partial appearance update", settings, err)
	}
	if _, err := s.db.Exec(`DROP TRIGGER fail_settings`); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveSettings(model.AppSettings{Appearance: "dark"}, &changed); err != nil {
		t.Fatal(err)
	}
	company, _ = s.Company()
	settings, _ = s.AppSettings()
	if company.Name != "Changed" || settings.Appearance != "dark" {
		t.Fatal("retry failed", company, settings)
	}
}

func TestAutomaticUpdatesSettingPersists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "updates.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	settings, err := s.AppSettings()
	if err != nil {
		t.Fatal(err)
	}
	if !settings.AutomaticUpdates {
		t.Fatal("automatic checks must be enabled by default")
	}
	settings.AutomaticUpdates = true
	if err := s.SaveAppSettings(settings); err != nil {
		t.Fatal(err)
	}
	s.Close()
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	settings, err = s.AppSettings()
	if err != nil || !settings.AutomaticUpdates {
		t.Fatalf("setting not persisted: %+v, %v", settings, err)
	}
	settings.AutomaticUpdates = false
	if err := s.SaveAppSettings(settings); err != nil {
		t.Fatal(err)
	}
	settings, err = s.AppSettings()
	if err != nil || settings.AutomaticUpdates {
		t.Fatalf("setting not disabled: %+v, %v", settings, err)
	}
}

func TestLegacySettingsEnableAutomaticUpdates(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "legacy.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.writeSetting("app", map[string]any{"appearance": "dark"}); err != nil {
		t.Fatal(err)
	}
	settings, err := s.AppSettings()
	if err != nil || !settings.AutomaticUpdates || settings.Appearance != "dark" {
		t.Fatalf("legacy defaults not applied: %+v, %v", settings, err)
	}
}

func TestStatisticsDefaults(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "stats.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	settings, err := s.AppSettings()
	if err != nil || settings.ImprovementStatistics || settings.StatisticsAsked {
		t.Fatalf("new installation: %+v, %v", settings, err)
	}
	// Settings saved before the preferences existed keep the defaults.
	if err := s.writeSetting("app", map[string]any{"automaticUpdates": false}); err != nil {
		t.Fatal(err)
	}
	// Installations from before the update have not been asked yet.
	if settings, err = s.AppSettings(); err != nil || settings.ImprovementStatistics || settings.StatisticsAsked {
		t.Fatalf("legacy settings: %+v, %v", settings, err)
	}
	settings.ImprovementStatistics = true
	if err := s.SaveAppSettings(settings); err != nil {
		t.Fatal(err)
	}
	if settings, err = s.AppSettings(); err != nil || !settings.ImprovementStatistics {
		t.Fatalf("saved choice lost: %+v, %v", settings, err)
	}
}
