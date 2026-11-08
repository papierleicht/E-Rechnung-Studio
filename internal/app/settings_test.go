package app

import (
	"papierleicht.de/e-rechnung-studio/internal/model"
	"path/filepath"
	"reflect"
	"testing"
)

func TestSettingsPageSavesAtomically(t *testing.T) {
	s, err := NewInvoiceService(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	c := testCompany()
	initial := model.AppSettings{Appearance: "light", ExportDirectory: t.TempDir()}
	if _, err := s.SaveSettings(initial, &c); err != nil {
		t.Fatal(err)
	}
	changed := initial
	changed.Appearance = "dark"
	invalid := c
	invalid.IBAN = "invalid"
	if _, err := s.SaveSettings(changed, &invalid); err == nil {
		t.Fatal("invalid company accepted")
	}
	stored, _ := s.GetAppSettings()
	if !reflect.DeepEqual(stored, initial) {
		t.Fatal("settings persisted despite invalid company", stored)
	}
	invalidSettings := changed
	invalidSettings.ExportDirectory = filepath.Join("relative", "folder")
	c.Name = "Changed company"
	if _, err := s.SaveSettings(invalidSettings, &c); err == nil {
		t.Fatal("invalid folder accepted")
	}
	company, _ := s.store.Company()
	if company.Name == c.Name {
		t.Fatal("company persisted despite invalid settings")
	}
	saved, err := s.SaveSettings(changed, &c)
	if err != nil || saved.Company.Name != c.Name || saved.Settings.Appearance != "dark" {
		t.Fatal(saved, err)
	}
}

func TestSettingsPageAppearanceOnlyPreservesCompany(t *testing.T) {
	s, err := NewInvoiceService(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	legacy := testCompany()
	legacy.IBAN = "legacy-invalid-value"
	if err := s.store.SaveCompany(legacy); err != nil {
		t.Fatal(err)
	}
	saved, err := s.SaveSettings(model.AppSettings{Appearance: "dark", ExportDirectory: t.TempDir()}, nil)
	if err != nil || saved.Company.IBAN != legacy.IBAN {
		t.Fatal(saved, err)
	}
}
