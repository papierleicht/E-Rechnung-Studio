package app

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	validator "papierleicht.de/e-rechnung-studio/internal/validation"
)

// Integration tests must never export sample invoices to the user's Documents.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "erechnungstudio-test-exports-*")
	if err != nil {
		panic(err)
	}
	os.Setenv("E_RECHNUNG_STUDIO_EXPORT_DIR", dir)
	// Every test service shares one validator, so the rules are unpacked and
	// compiled once per run. Doing it per service exceeds the test timeout on
	// the Windows runner.
	var once sync.Once
	var shared *validator.Validator
	var sharedErr error
	newValidator = func(string) (*validator.Validator, error) {
		once.Do(func() { shared, sharedErr = validator.New(filepath.Join(dir, "validator-cache")) })
		return shared, sharedErr
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}
