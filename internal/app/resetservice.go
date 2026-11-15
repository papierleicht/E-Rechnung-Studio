package app

import (
	"context"
	"errors"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/zalando/go-keyring"
	"papierleicht.de/e-rechnung-studio/internal/account"
)

// Resetting returns the app to its first start, e.g. after trying it out.
// The data folder cannot be removed while the app runs (Windows keeps open
// files locked, and every service holds its database), so ResetApp only
// marks it. The folder goes after shutdown (PostShutdown in Run), or at the
// next start if the app ended before that.

// resetMarker in the data folder marks it for removal. Only a folder that
// carries the marker is ever removed, so a wrong E_RECHNUNG_STUDIO_DATA_DIR
// cannot cost foreign files.
const resetMarker = ".zuruecksetzen"

// ResetConfirmation must be typed by the customer.
const ResetConfirmation = "ZURÜCKSETZEN"

// resetKeyringServices hold the passwords and keys the customer entered for
// mail, bank and shops. The account keychain is left alone: it also holds the
// recovery key of the cloud backup, without which existing backups stay
// unreadable; signing out removes only the session.
var resetKeyringServices = []string{
	"de.papierleicht.smtp",  // internal/mail
	"de.papierleicht.fints", // internal/banking
	shopKeyringService,
}

// ResetOverview tells the UI what a reset would delete and what stays.
type ResetOverview struct {
	IssuedInvoices    int    `json:"issuedInvoices"`
	ReceivedDocuments int    `json:"receivedDocuments"`
	SignedIn          bool   `json:"signedIn"`
	BackupKey         bool   `json:"backupKey"`
	ExportDirectory   string `json:"exportDirectory"`
	DataDirectory     string `json:"dataDirectory"`
}

// ResetResult names what could not be removed, so the customer can do it by
// hand.
type ResetResult struct {
	KeychainLeft []string `json:"keychainLeft"`
}

type resetSignOut interface {
	SignOut(context.Context) (account.Status, error)
	Status() account.Status
}

type ResetService struct {
	dataDir  string
	invoices *InvoiceService
	accounts resetSignOut
	vault    account.Vault
	// deleteSecrets removes every entry of a keychain service.
	deleteSecrets func(service string) error
	quit          func()
}

func NewResetService(dataDir string, invoices *InvoiceService, accounts resetSignOut, vault account.Vault) *ResetService {
	return &ResetService{dataDir: dataDir, invoices: invoices, accounts: accounts, vault: vault, deleteSecrets: deleteKeyringService}
}

func deleteKeyringService(service string) error {
	if err := keyring.DeleteAll(service); err != nil && !errors.Is(err, keyring.ErrNotFound) {
		return err
	}
	return nil
}

// ResetOverview counts what the customer would lose.
func (s *ResetService) ResetOverview() (ResetOverview, error) {
	o := ResetOverview{DataDirectory: s.dataDir}
	db := s.invoices.store.DB()
	if err := db.QueryRow(`SELECT COUNT(*) FROM invoices WHERE status<>'draft'`).Scan(&o.IssuedInvoices); err != nil {
		return o, err
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM received_invoices`).Scan(&o.ReceivedDocuments); err != nil {
		return o, err
	}
	if settings, err := s.invoices.GetAppSettings(); err == nil {
		o.ExportDirectory = settings.ExportDirectory
	}
	if s.accounts != nil {
		o.SignedIn = s.accounts.Status().State != account.StateSignedOut
	}
	if s.vault != nil {
		key, err := s.vault.Get(backupKeyName)
		o.BackupKey = err == nil && key != ""
	}
	return o, nil
}

// ResetApp signs out, removes the stored mail, bank and shop secrets and
// marks the data folder for removal. The caller ends the app with
// QuitAfterReset; until then nothing is deleted from the folder.
func (s *ResetService) ResetApp(confirmation string) (ResetResult, error) {
	result := ResetResult{KeychainLeft: []string{}}
	if strings.TrimSpace(confirmation) != ResetConfirmation {
		return result, errors.New("Gib zur Bestätigung ZURÜCKSETZEN ein.")
	}
	if err := resettableDir(s.dataDir); err != nil {
		return result, err
	}
	if s.accounts != nil {
		// Frees this device's slot when online; offline only the local
		// session goes, and the slot frees itself when it expires.
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		_, _ = s.accounts.SignOut(ctx)
		cancel()
	}
	for _, service := range resetKeyringServices {
		if err := s.deleteSecrets(service); err != nil {
			log.Printf("reset: keychain %s: %v", service, err)
			result.KeychainLeft = append(result.KeychainLeft, service)
		}
	}
	if err := os.WriteFile(filepath.Join(s.dataDir, resetMarker), []byte(time.Now().Format(time.RFC3339)+"\n"), 0o600); err != nil {
		return result, errors.New("Die App konnte das Zurücksetzen nicht vormerken. Prüfe, ob der Datenordner beschreibbar ist.")
	}
	// Changes made after this point are lost anyway; no "unsaved" prompt.
	s.invoices.close.setDirty(false)
	return result, nil
}

// QuitAfterReset ends the app; the data folder is removed on the way out.
func (s *ResetService) QuitAfterReset() error {
	if _, err := os.Stat(filepath.Join(s.dataDir, resetMarker)); err != nil {
		return errors.New("Es ist kein Zurücksetzen vorgemerkt.")
	}
	if s.quit != nil {
		go s.quit()
	}
	return nil
}

// resettableDir refuses folders whose removal would hit more than the app's
// own data: the file system root, the home folder or one of its parents.
func resettableDir(dir string) error {
	refuse := errors.New("Dieser Datenordner wird nicht automatisch gelöscht. Lösche ihn bei Bedarf selbst.")
	if dir == "" || !filepath.IsAbs(dir) {
		return refuse
	}
	dir = filepath.Clean(dir)
	if filepath.Dir(dir) == dir {
		return refuse
	}
	if home, err := os.UserHomeDir(); err == nil {
		home = filepath.Clean(home)
		if dir == home || strings.HasPrefix(home, dir+string(filepath.Separator)) {
			return refuse
		}
	}
	return nil
}

func resetPending(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, resetMarker))
	return err == nil
}

// finishPendingReset removes a data folder marked by ResetApp. The app's
// PostShutdown hook calls it after every service has shut down; when
// something still holds a file (an antivirus scan on Windows, say), the
// folder stays marked and the next start finishes the job.
func finishPendingReset(dir string) error {
	if !resetPending(dir) {
		return nil
	}
	if err := resettableDir(dir); err != nil {
		return err
	}
	if err := os.RemoveAll(dir); err != nil {
		// RemoveAll may have taken the marker with it; mark again.
		_ = os.MkdirAll(dir, 0o700)
		_ = os.WriteFile(filepath.Join(dir, resetMarker), nil, 0o600)
		return err
	}
	return nil
}

// finishResetAtStart runs before anything opens the data. If the folder
// still cannot be removed, the marker goes: otherwise the next shutdown
// would delete the data the customer enters from now on.
func finishResetAtStart(dir string) {
	var err error
	for try := 0; try < 3; try++ {
		if err = finishPendingReset(dir); err == nil {
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
	log.Printf("reset: %v", err)
	_ = os.Remove(filepath.Join(dir, resetMarker))
}
