package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"

	"papierleicht.de/e-rechnung-studio/internal/account"
)

type fakeResetAccount struct {
	state    string
	signOuts int
}

func (a *fakeResetAccount) SignOut(context.Context) (account.Status, error) {
	a.signOuts++
	a.state = account.StateSignedOut
	return a.Status(), nil
}
func (a *fakeResetAccount) Status() account.Status { return account.Status{State: a.state} }

func newResetFixture(t *testing.T) (*ResetService, *fakeResetAccount, *[]string, string) {
	t.Helper()
	inv := newTestService(t)
	dir := filepath.Join(t.TempDir(), "ERechnungStudio")
	if err := os.MkdirAll(filepath.Join(dir, "cache"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "e-rechnung-studio.db"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	acc := &fakeResetAccount{state: account.StateSignedIn}
	vault := &memoryVault{m: map[string]string{backupKeyName: "key"}}
	s := NewResetService(dir, inv, acc, vault)
	deleted := &[]string{}
	s.deleteSecrets = func(service string) error {
		*deleted = append(*deleted, service)
		return nil
	}
	return s, acc, deleted, dir
}

func TestResetOverviewCountsWhatGoes(t *testing.T) {
	s, _, _, _ := newResetFixture(t)
	issueNumbered(t, s.invoices, "RST-2026-001")
	o, err := s.ResetOverview()
	if err != nil {
		t.Fatal(err)
	}
	if o.IssuedInvoices != 1 || o.ReceivedDocuments != 0 || !o.SignedIn || !o.BackupKey || o.ExportDirectory == "" {
		t.Fatalf("overview: %+v", o)
	}
}

func TestResetNeedsTheTypedConfirmation(t *testing.T) {
	s, acc, deleted, dir := newResetFixture(t)
	if _, err := s.ResetApp("ja"); err == nil {
		t.Fatal("reset without confirmation")
	}
	if acc.signOuts != 0 || len(*deleted) != 0 {
		t.Fatal("refused reset changed something")
	}
	if _, err := os.Stat(filepath.Join(dir, resetMarker)); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("refused reset left a marker")
	}
	if err := s.QuitAfterReset(); err == nil {
		t.Fatal("quit without a pending reset")
	}
}

func TestResetSignsOutClearsSecretsAndRemovesTheFolder(t *testing.T) {
	s, acc, deleted, dir := newResetFixture(t)
	s.invoices.close.setDirty(true)
	quit := make(chan struct{})
	s.quit = func() { close(quit) }
	if _, err := s.ResetApp(" " + ResetConfirmation + " "); err != nil {
		t.Fatal(err)
	}
	if acc.signOuts != 1 {
		t.Fatal("not signed out")
	}
	want := []string{"de.papierleicht.smtp", "de.papierleicht.fints", shopKeyringService}
	if !slices.Equal(*deleted, want) {
		t.Fatalf("keychain services %v, want %v", *deleted, want)
	}
	if key, _ := s.vault.Get(backupKeyName); key != "key" {
		t.Fatal("the cloud backup recovery key must stay")
	}
	if s.invoices.close.needsConfirmation() {
		t.Fatal("quitting after a reset must not ask about unsaved changes")
	}
	if err := s.QuitAfterReset(); err != nil {
		t.Fatal(err)
	}
	<-quit
	if err := finishPendingReset(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("data folder still there: %v", err)
	}
}

func TestResetReportsKeychainLeftovers(t *testing.T) {
	s, _, _, _ := newResetFixture(t)
	s.deleteSecrets = func(service string) error {
		if service == "de.papierleicht.fints" {
			return errors.New("locked")
		}
		return nil
	}
	r, err := s.ResetApp(ResetConfirmation)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(r.KeychainLeft, []string{"de.papierleicht.fints"}) {
		t.Fatalf("left: %v", r.KeychainLeft)
	}
}

func TestFinishPendingResetOnlyRemovesMarkedFolders(t *testing.T) {
	dir := t.TempDir()
	if err := finishPendingReset(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Fatal("unmarked folder removed")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip(err)
	}
	for _, d := range []string{"", "relative", "/", home, filepath.Dir(home)} {
		if resettableDir(d) == nil {
			t.Errorf("%q accepted for removal", d)
		}
	}
	if err := resettableDir(filepath.Join(home, "Library", "Application Support", "ERechnungStudio")); err != nil {
		t.Errorf("default folder refused: %v", err)
	}
}

// A folder that cannot be removed at start loses its marker, so the data
// entered afterwards is not deleted at the next shutdown.
func TestResetAtStartDropsTheMarkerWhenFilesStay(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permissions cannot block removal on Windows")
	}
	dir := filepath.Join(t.TempDir(), "ERechnungStudio")
	locked := filepath.Join(dir, "locked")
	if err := os.MkdirAll(locked, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(locked, "file"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, resetMarker), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(locked, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(locked, 0o700) })
	if finishPendingReset(dir) == nil {
		t.Skip("the file system ignores directory permissions")
	}
	if !resetPending(dir) {
		t.Fatal("failed removal at shutdown must stay marked for the next start")
	}
	finishResetAtStart(dir)
	if resetPending(dir) {
		t.Fatal("marker kept after the start gave up")
	}
}
