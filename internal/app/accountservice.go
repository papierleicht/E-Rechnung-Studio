package app

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"papierleicht.de/e-rechnung-studio/internal/account"
)

// Production defaults. The public key is not a secret; the matching signing
// key lives only in the operator's secret store. Builds may still override both with
// -ldflags="-X papierleicht.de/e-rechnung-studio/internal/app.licenseServerURL=https://… -X papierleicht.de/e-rechnung-studio/internal/app.licensePublicKey=<base64>".
// Without both values the "Konto & Abo" page reports that sign-in is not set
// up and the app runs as the Community version.
var (
	licenseServerURL = "https://lizenz.papierleicht.de"
	licensePublicKey = "sx8mbOy+lb4hsXX7n45agOuwtt2ZuB1ZW/TpLR8UjSU="
	accountManageURL = "https://papierleicht.de/konto"
)

// Development overrides. The public key can only be overridden when the
// build embeds none, so a release cannot be pointed at a self-signed key.
const (
	envAccountServer    = "PAPIERLEICHT_ACCOUNT_SERVER"
	envLicensePublicKey = "PAPIERLEICHT_LICENSE_PUBLIC_KEY"
	envAccountManageURL = "PAPIERLEICHT_ACCOUNT_MANAGE_URL"
)

// accountRefreshInterval only decides how often refreshAfter is checked;
// the server decides when a renewal is due.
const accountRefreshInterval = 30 * time.Minute

type AccountService struct {
	manager *account.Manager
	stop    chan struct{}
	once    sync.Once
}

func accountConfig() (string, ed25519.PublicKey, string) {
	server, key, manage := licenseServerURL, licensePublicKey, accountManageURL
	if v := strings.TrimSpace(os.Getenv(envAccountServer)); v != "" {
		server = v
	}
	if key == "" {
		key = os.Getenv(envLicensePublicKey)
	}
	if v := strings.TrimSpace(os.Getenv(envAccountManageURL)); v != "" {
		manage = v
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(key))
	if err != nil || len(raw) != ed25519.PublicKeySize {
		raw = nil
	}
	return server, ed25519.PublicKey(raw), manage
}

func deviceName() string {
	name, err := os.Hostname()
	if err != nil || strings.TrimSpace(name) == "" {
		return "papierleicht"
	}
	return strings.TrimSuffix(name, ".local")
}

func NewAccountService(dataDir string) (*AccountService, error) {
	server, key, manage := accountConfig()
	m, err := account.New(account.Config{ServerURL: server, PublicKey: key, ManageURL: manage, DeviceName: deviceName(), DataDir: dataDir})
	if err != nil {
		return nil, err
	}
	return &AccountService{manager: m, stop: make(chan struct{})}, nil
}

func accountContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 30*time.Second)
}

//wails:ignore
func (s *AccountService) ServiceStartup(ctx context.Context, _ application.ServiceOptions) error {
	s.manager.OnChange(func(st account.Status) {
		if app := application.Get(); app != nil {
			app.Event.Emit("account:status", st)
		}
	})
	go func() {
		ticker := time.NewTicker(accountRefreshInterval)
		defer ticker.Stop()
		for {
			c, cancel := accountContext()
			_, _ = s.manager.Refresh(c, false)
			cancel()
			select {
			case <-s.stop:
				return
			case <-ticker.C:
			}
		}
	}()
	return nil
}

//wails:ignore
func (s *AccountService) ServiceShutdown() error {
	s.once.Do(func() { close(s.stop) })
	return nil
}

// Status reports the locally verified state without network access.
func (s *AccountService) Status() account.Status { return s.manager.Status() }

// Prices returns the bookable plans with their final amounts.
func (s *AccountService) Prices() ([]account.Price, error) {
	c, cancel := accountContext()
	defer cancel()
	return s.manager.Prices(c)
}

// RequestCode mails a one-time sign-in code; unknown addresses get an account.
func (s *AccountService) RequestCode(email string) (account.Status, error) {
	c, cancel := accountContext()
	defer cancel()
	return s.manager.RequestCode(c, email)
}

func (s *AccountService) CancelCode() account.Status { return s.manager.CancelCode() }

// Newsletter reports where this installation last requested the newsletter.
func (s *AccountService) Newsletter() account.Newsletter { return s.manager.Newsletter() }

// SubscribeNewsletter sends the confirmation mail of the newsletter. It needs
// no account; source is "app-onboarding" or "app-settings".
func (s *AccountService) SubscribeNewsletter(email, source string) (account.Newsletter, error) {
	c, cancel := accountContext()
	defer cancel()
	return s.manager.SubscribeNewsletter(c, email, source)
}

// SignIn completes the e-mail code sign-in and activates this device. The
// error explains a missing subscription only for network failures; 402 and
// 409 are reported in Status.Notice.
func (s *AccountService) SignIn(code string) (account.Status, error) {
	c, cancel := accountContext()
	defer cancel()
	return s.manager.SignIn(c, code)
}

// Refresh checks the subscription with the server now.
func (s *AccountService) Refresh() (account.Status, error) {
	c, cancel := accountContext()
	defer cancel()
	return s.manager.Refresh(c, true)
}

func (s *AccountService) Devices() ([]account.Device, error) {
	c, cancel := accountContext()
	defer cancel()
	return s.manager.Devices(c)
}

func (s *AccountService) RemoveDevice(id string) (account.Status, error) {
	c, cancel := accountContext()
	defer cancel()
	return s.manager.RemoveDevice(c, id)
}

func (s *AccountService) SignOut() (account.Status, error) {
	c, cancel := accountContext()
	defer cancel()
	return s.manager.SignOut(c)
}

// SignOutEverywhere ends every session of the account, this one included.
func (s *AccountService) SignOutEverywhere() (account.Status, error) {
	c, cancel := accountContext()
	defer cancel()
	return s.manager.SignOutEverywhere(c)
}
