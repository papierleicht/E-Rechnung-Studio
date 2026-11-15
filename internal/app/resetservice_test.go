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
	"papierleicht.de/e-rechnung-studio/internal/store"
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
	if _, err := s.ResetApp("ja", false); err == nil {
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
	if _, err := s.ResetApp(" "+ResetConfirmation+" ", false); err != nil {
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
	r, err := s.ResetApp(ResetConfirmation, false)
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

func TestResetKeepingTheSetupStartsFreshWithTheSetupAnswers(t *testing.T) {
	s, acc, deleted, dir := newResetFixture(t)
	inv := s.invoices
	issueNumbered(t, inv, "RST-2026-001")
	c, err := inv.store.Company()
	if err != nil {
		t.Fatal(err)
	}
	c.SetupCompleted = true
	c.Logo = "bG9nbw=="
	if err = inv.store.SaveCompany(c); err != nil {
		t.Fatal(err)
	}
	f, err := inv.store.FinanceSettings()
	if err != nil {
		t.Fatal(err)
	}
	f.Activity, f.Filing, f.VATPeriod = "freelance", "advisor", "quarterly"
	f.Balances = []store.AccountBalance{{AccountID: "a", Amount: "12.00", Date: "2026-11-01"}}
	if _, err = inv.store.SaveFinanceSettings(f); err != nil {
		t.Fatal(err)
	}
	db := inv.store.DB()
	if _, err = db.Exec(`CREATE TABLE IF NOT EXISTS mail_settings (id INTEGER PRIMARY KEY CHECK(id=1), data BLOB NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO mail_settings(id,data) VALUES(1,?)`, []byte(`{"account":{"host":"smtp.example.de"}}`)); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(dir, "account-device-id"), []byte("device"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err = s.ResetApp(ResetConfirmation, true); err != nil {
		t.Fatal(err)
	}
	if acc.signOuts != 0 {
		t.Fatal("keeping the setup keeps the sign-in")
	}
	if slices.Contains(*deleted, mailKeyringService) || !slices.Contains(*deleted, shopKeyringService) {
		t.Fatalf("keychain services %v: the mail password stays, the rest goes", *deleted)
	}
	if err = finishPendingReset(dir); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	names := []string{}
	for _, e := range entries {
		names = append(names, e.Name())
	}
	slices.Sort(names)
	if !slices.Equal(names, []string{resetKeepFile, "account-device-id"}) {
		t.Fatalf("after the reset the folder holds %v", names)
	}

	fresh, err := NewInvoiceService(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { fresh.Close() })
	if _, err = fresh.store.DB().Exec(`CREATE TABLE IF NOT EXISTS mail_settings (id INTEGER PRIMARY KEY CHECK(id=1), data BLOB NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	if err = restoreKeptSetup(dir, fresh); err != nil {
		t.Fatal(err)
	}
	got, err := fresh.store.Company()
	if err != nil || got.Name != c.Name || got.Logo != c.Logo || !got.SetupCompleted || got.IBAN != c.IBAN {
		t.Fatalf("company %+v %v", got, err)
	}
	gf, err := fresh.store.FinanceSettings()
	if err != nil || gf.Activity != "freelance" || gf.Filing != "advisor" || gf.VATPeriod != "quarterly" || len(gf.Balances) != 0 {
		t.Fatalf("finance %+v %v", gf, err)
	}
	var mail []byte
	if err = fresh.store.DB().QueryRow(`SELECT data FROM mail_settings WHERE id=1`).Scan(&mail); err != nil || string(mail) != `{"account":{"host":"smtp.example.de"}}` {
		t.Fatalf("mail %s %v", mail, err)
	}
	o, err := (&ResetService{dataDir: dir, invoices: fresh}).ResetOverview()
	if err != nil || o.IssuedInvoices != 0 {
		t.Fatalf("the invoices must be gone: %+v %v", o, err)
	}
	if _, err = os.Stat(filepath.Join(dir, resetKeepFile)); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("the keep file stays after the restore")
	}
}
