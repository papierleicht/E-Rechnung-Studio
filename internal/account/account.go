// Package account signs the customer in at the papierleicht license server
// with an e-mail code and keeps the signed, offline-verifiable entitlement.
// Tokens and the FinTS product ID live in the OS keychain only.
package account

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/mail"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/zalando/go-keyring"
	"papierleicht.de/e-rechnung-studio/internal/entitlement"
)

// KeyringService groups all account secrets in the OS keychain.
const KeyringService = "de.papierleicht.account"

const sessionKey = "session"

// States reported to the UI.
const (
	StateSignedOut  = "signed-out"
	StateSignedIn   = "signed-in"
	StatePlus       = "plus"
	StatePlusBackup = "plus-backup"
)

// Notice kinds explain why the state is what it is.
const (
	NoticeNoSubscription = "no-subscription"
	NoticeDeviceLimit    = "device-limit"
	NoticeSessionExpired = "session-expired"
	NoticeOffline        = "offline"
	NoticeInvalidToken   = "invalid-token"
	// NoticeSupportPlan: Community Support is booked, which unlocks no
	// app features but e-mail support and the community.
	NoticeSupportPlan = "support-plan"
)

// PlanSupport is Community Support; Status.Plan names it while the state
// stays StateSignedIn.
const PlanSupport = "support"

const supportNotice = "Community Support ist aktiv: Software-Support per E-Mail sowie Chat und Angebote & Gesuche unter „Chat & Markt“. Die Plus-Funktionen wie der Bankabruf sind darin nicht enthalten."

// Vault stores secrets; the default is the OS keychain.
type Vault interface {
	Get(string) (string, error)
	Set(string, string) error
	Delete(string) error
}

// SystemVault is the OS keychain under KeyringService. A missing entry reads
// as "" without error.
type SystemVault struct{}

func (SystemVault) Get(id string) (string, error) {
	v, err := keyring.Get(KeyringService, id)
	if errors.Is(err, keyring.ErrNotFound) {
		return "", nil
	}
	return v, err
}
func (SystemVault) Set(id, value string) error { return keyring.Set(KeyringService, id, value) }
func (SystemVault) Delete(id string) error {
	if err := keyring.Delete(KeyringService, id); err != nil && !errors.Is(err, keyring.ErrNotFound) {
		return err
	}
	return nil
}

type Config struct {
	// ServerURL and PublicKey are required; without them Status reports
	// Configured=false and every call fails with ErrNotConfigured.
	ServerURL  string
	PublicKey  ed25519.PublicKey
	ManageURL  string
	DeviceName string
	// DataDir holds the non-secret device ID.
	DataDir string
	Vault   Vault
	HTTP    *http.Client
	Now     func() time.Time
}

var ErrNotConfigured = errors.New("Die Konto-Anmeldung ist in dieser Version nicht eingerichtet.")

// session is the keychain entry. It holds only tokens and the product ID,
// all of which the server can reissue.
type session struct {
	AuthToken string `json:"authToken,omitempty"`
	UserID    string `json:"userId,omitempty"`
	Email     string `json:"email"`
	// Entitlement and ProductID come from POST /api/license/entitlement.
	Entitlement  string    `json:"entitlement,omitempty"`
	ProductID    string    `json:"productId,omitempty"`
	RefreshAfter time.Time `json:"refreshAfter"`
	// Plan is a plan without entitlement the server named, i.e. PlanSupport.
	Plan string `json:"plan,omitempty"`
}

// Status is what the frontend shows under "Konto & Abo".
type Status struct {
	Configured   bool     `json:"configured"`
	State        string   `json:"state"`
	Email        string   `json:"email"`
	Plan         string   `json:"plan"`
	Features     []string `json:"features"`
	ExpiresAt    string   `json:"expiresAt"`
	RefreshAfter string   `json:"refreshAfter"`
	// Notice is a German explanation; NoticeKind one of the Notice* values.
	Notice      string `json:"notice"`
	NoticeKind  string `json:"noticeKind"`
	CodePending bool   `json:"codePending"`
	PendingMail string `json:"pendingEmail"`
	ManageURL   string `json:"manageUrl"`
	DeviceName  string `json:"deviceName"`
}

// Manager is safe for concurrent use. Network calls run under netMu so
// state reads never wait for the server.
type Manager struct {
	cfg      Config
	client   *client
	deviceID string

	netMu sync.Mutex
	mu    sync.Mutex
	sess  *session
	// claims is the verified entitlement of sess, if any.
	claims     *entitlement.Claims
	notice     string
	noticeKind string
	otpID      string
	otpEmail   string
	onChange   func(Status)
}

var deviceIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{16,128}$`)

func New(cfg Config) (*Manager, error) {
	if cfg.Vault == nil {
		cfg.Vault = SystemVault{}
	}
	if cfg.HTTP == nil {
		// Redirects are refused so tokens never follow to another host.
		cfg.HTTP = &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	cfg.ServerURL = strings.TrimRight(strings.TrimSpace(cfg.ServerURL), "/")
	m := &Manager{cfg: cfg, client: &client{base: cfg.ServerURL, http: cfg.HTTP}}
	if cfg.DataDir != "" {
		id, err := loadDeviceID(cfg.DataDir)
		if err != nil {
			return nil, err
		}
		m.deviceID = id
	}
	if m.configured() {
		m.load()
	}
	return m, nil
}

func (m *Manager) configured() bool {
	return m.cfg.ServerURL != "" && len(m.cfg.PublicKey) == ed25519.PublicKeySize && m.deviceID != ""
}

// loadDeviceID returns the installation's random ID, creating it once.
func loadDeviceID(dir string) (string, error) {
	path := filepath.Join(dir, "account-device-id")
	if raw, err := os.ReadFile(path); err == nil {
		if id := strings.TrimSpace(string(raw)); deviceIDPattern.MatchString(id) {
			return id, nil
		}
	}
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	id := base64.RawURLEncoding.EncodeToString(buf)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, []byte(id+"\n"), 0o600); err != nil {
		return "", err
	}
	return id, nil
}

// OnChange registers a callback for state changes (e.g. a Wails event).
func (m *Manager) OnChange(fn func(Status)) {
	m.mu.Lock()
	m.onChange = fn
	m.mu.Unlock()
}

// DeviceID is the stable, non-secret ID of this installation.
func (m *Manager) DeviceID() string { return m.deviceID }

func (m *Manager) DeviceName() string { return m.cfg.DeviceName }

func (m *Manager) load() {
	raw, err := m.cfg.Vault.Get(sessionKey)
	if err != nil || raw == "" {
		return
	}
	var s session
	if json.Unmarshal([]byte(raw), &s) != nil {
		return
	}
	m.mu.Lock()
	m.setSessionLocked(&s)
	m.mu.Unlock()
}

// setSessionLocked verifies the stored entitlement again; a token that does
// not verify (other key, other device) is treated as absent.
func (m *Manager) setSessionLocked(s *session) {
	m.sess, m.claims = s, nil
	if s == nil || s.Entitlement == "" {
		return
	}
	c, err := entitlement.Verify(m.cfg.PublicKey, s.Entitlement, m.cfg.Now())
	if (err == nil || errors.Is(err, entitlement.ErrExpired)) && c.Device == m.deviceID {
		m.claims = &c
	}
}

func (m *Manager) save(s *session) error {
	if s == nil {
		return m.cfg.Vault.Delete(sessionKey)
	}
	raw, err := json.Marshal(s)
	if err != nil {
		return err
	}
	if err := m.cfg.Vault.Set(sessionKey, string(raw)); err != nil {
		return errors.New("Die Anmeldung konnte nicht im Schlüsselbund gespeichert werden.")
	}
	return nil
}

// valid reports whether the cached entitlement is still usable offline.
func (m *Manager) validLocked() bool {
	return m.claims != nil && m.cfg.Now().Before(m.claims.ExpiresAt())
}

// Status never touches the network.
func (m *Manager) Status() Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.statusLocked()
}

func (m *Manager) statusLocked() Status {
	st := Status{Configured: m.configured(), State: StateSignedOut, ManageURL: m.cfg.ManageURL, DeviceName: m.cfg.DeviceName,
		Notice: m.notice, NoticeKind: m.noticeKind, CodePending: m.otpID != "", PendingMail: m.otpEmail, Features: []string{}}
	if m.sess == nil {
		return st
	}
	st.Email = m.sess.Email
	st.State = StateSignedIn
	if !m.sess.RefreshAfter.IsZero() {
		st.RefreshAfter = m.sess.RefreshAfter.UTC().Format(time.RFC3339)
	}
	if m.claims == nil {
		if m.sess.Plan == PlanSupport {
			st.Plan = PlanSupport
		}
		return st
	}
	st.ExpiresAt = m.claims.ExpiresAt().UTC().Format(time.RFC3339)
	if !m.validLocked() {
		if st.NoticeKind == "" || st.NoticeKind == NoticeOffline {
			st.NoticeKind, st.Notice = NoticeSessionExpired, "Die Abo-Bestätigung ist abgelaufen. Prüfe sie erneut, sobald du online bist."
		}
		return st
	}
	st.Plan, st.Features = m.claims.Plan, slices.Clone(m.claims.Features)
	switch {
	case m.claims.Has(entitlement.FeatureBackup):
		st.State = StatePlusBackup
	case m.claims.Has(entitlement.FeatureFinTS):
		st.State = StatePlus
	}
	return st
}

// HasFeature reports a verified, unexpired entitlement feature.
func (m *Manager) HasFeature(feature string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.validLocked() && m.claims.Has(feature)
}

// ProductID is the banking.ProductIDSource: the registered FinTS product ID
// of an active subscription, or "" without one.
func (m *Manager) ProductID() (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.sess == nil || !m.validLocked() || !m.claims.Has(entitlement.FeatureFinTS) {
		return "", nil
	}
	return m.sess.ProductID, nil
}

func (m *Manager) changed() Status {
	m.mu.Lock()
	st := m.statusLocked()
	fn := m.onChange
	m.mu.Unlock()
	if fn != nil {
		fn(st)
	}
	return st
}

func (m *Manager) setNotice(kind, text string) {
	m.mu.Lock()
	m.noticeKind, m.notice = kind, text
	m.mu.Unlock()
}

// RequestCode asks the server to mail a one-time code.
func (m *Manager) RequestCode(ctx context.Context, email string) (Status, error) {
	if !m.configured() {
		return m.Status(), ErrNotConfigured
	}
	email = strings.TrimSpace(email)
	if addr, err := mail.ParseAddress(email); err != nil || addr.Address != email {
		return m.Status(), errors.New("Gib eine gültige E-Mail-Adresse ein.")
	}
	m.netMu.Lock()
	defer m.netMu.Unlock()
	id, err := m.client.requestOTP(ctx, email)
	if err != nil {
		if StatusOf(err) == http.StatusTooManyRequests {
			err = errors.New("Zu viele Anfragen. Warte ein paar Minuten und versuche es dann noch einmal.")
		}
		return m.Status(), err
	}
	m.mu.Lock()
	m.otpID, m.otpEmail = id, email
	m.mu.Unlock()
	return m.changed(), nil
}

// CancelCode forgets a requested code.
func (m *Manager) CancelCode() Status {
	m.mu.Lock()
	m.otpID, m.otpEmail = "", ""
	m.mu.Unlock()
	return m.changed()
}

// SignIn exchanges the mailed code for a session and fetches the
// entitlement. A missing subscription (402) or a full device list (409)
// still leaves the customer signed in.
func (m *Manager) SignIn(ctx context.Context, code string) (Status, error) {
	if !m.configured() {
		return m.Status(), ErrNotConfigured
	}
	code = strings.Join(strings.Fields(code), "")
	m.mu.Lock()
	otpID, email := m.otpID, m.otpEmail
	m.mu.Unlock()
	if otpID == "" {
		return m.Status(), errors.New("Fordere zuerst einen Anmeldecode an.")
	}
	if code == "" {
		return m.Status(), errors.New("Gib den Code aus der E-Mail ein.")
	}
	m.netMu.Lock()
	auth, err := m.client.authWithOTP(ctx, otpID, code)
	if err != nil {
		m.netMu.Unlock()
		if s := StatusOf(err); s == http.StatusBadRequest || s == http.StatusUnauthorized || s == http.StatusNotFound {
			err = errors.New("Der Code ist falsch oder abgelaufen. Versuche es noch einmal oder fordere einen neuen Code an.")
		}
		return m.Status(), err
	}
	if auth.Record.Email != "" {
		email = auth.Record.Email
	}
	s := &session{AuthToken: auth.Token, UserID: auth.Record.ID, Email: email}
	if err := m.save(s); err != nil {
		m.netMu.Unlock()
		return m.Status(), err
	}
	m.mu.Lock()
	m.otpID, m.otpEmail = "", ""
	m.noticeKind, m.notice = "", ""
	m.setSessionLocked(s)
	m.mu.Unlock()
	err = m.fetchEntitlementLocked(ctx)
	m.netMu.Unlock()
	// Signed in either way; err only explains missing access.
	return m.changed(), err
}

// Refresh renews session and entitlement when refreshAfter has passed (or
// always with force). Network failures keep the cached entitlement, which
// stays valid until its expiry.
func (m *Manager) Refresh(ctx context.Context, force bool) (Status, error) {
	if !m.configured() {
		return m.Status(), nil
	}
	m.mu.Lock()
	s := m.sess
	due := s != nil && (force || m.claims == nil || !m.cfg.Now().Before(s.RefreshAfter))
	m.mu.Unlock()
	if !due {
		// Still notify: an entitlement can expire without any server call.
		return m.changed(), nil
	}
	m.netMu.Lock()
	err := m.refreshAuthLocked(ctx)
	if err == nil {
		err = m.fetchEntitlementLocked(ctx)
	}
	m.netMu.Unlock()
	return m.changed(), err
}

// refreshAuthLocked extends the server session. Callers hold netMu.
func (m *Manager) refreshAuthLocked(ctx context.Context) error {
	m.mu.Lock()
	s := m.sess
	m.mu.Unlock()
	if s == nil {
		return nil
	}
	if s.AuthToken == "" {
		m.setNotice(NoticeSessionExpired, "Deine Anmeldung ist abgelaufen. Melde dich bitte noch einmal mit einem E-Mail-Code an.")
		return nil
	}
	auth, err := m.client.authRefresh(ctx, s.AuthToken)
	switch {
	case err == nil:
		next := *s
		next.AuthToken = auth.Token
		return m.commit(&next)
	case errors.Is(err, ErrOffline):
		m.setNotice(NoticeOffline, "Offline: Dein Abo wird erneut geprüft, sobald du wieder online bist.")
		return err
	case StatusOf(err) == http.StatusUnauthorized || StatusOf(err) == http.StatusForbidden || StatusOf(err) == http.StatusNotFound:
		// The offline entitlement keeps working until it expires.
		next := *s
		next.AuthToken = ""
		if e := m.commit(&next); e != nil {
			return e
		}
		m.setNotice(NoticeSessionExpired, "Deine Anmeldung ist abgelaufen. Melde dich bitte noch einmal mit einem E-Mail-Code an.")
		return nil
	default:
		return err
	}
}

func (m *Manager) commit(s *session) error {
	if err := m.save(s); err != nil {
		return err
	}
	m.mu.Lock()
	m.setSessionLocked(s)
	m.mu.Unlock()
	return nil
}

// fetchEntitlementLocked calls POST /api/license/entitlement. Callers hold netMu.
func (m *Manager) fetchEntitlementLocked(ctx context.Context) error {
	m.mu.Lock()
	s := m.sess
	m.mu.Unlock()
	if s == nil || s.AuthToken == "" {
		return nil
	}
	res, err := m.client.entitlement(ctx, s.AuthToken, m.deviceID, m.cfg.DeviceName)
	next := *s
	switch status := StatusOf(err); {
	case err == nil:
		c, verr := entitlement.Verify(m.cfg.PublicKey, res.Token, m.cfg.Now())
		if verr != nil || c.Device != m.deviceID || c.Subject != s.UserID && s.UserID != "" {
			m.setNotice(NoticeInvalidToken, "Die Abo-Bestätigung des Servers ist ungültig. Aktualisiere die App.")
			return errors.New("Die Abo-Bestätigung des Servers ist ungültig.")
		}
		next.Entitlement, next.ProductID, next.RefreshAfter, next.Plan = res.Token, strings.TrimSpace(res.ProductID), res.RefreshAfter, ""
		if !c.Has(entitlement.FeatureFinTS) {
			next.ProductID = ""
		}
		if err := m.commit(&next); err != nil {
			return err
		}
		m.setNotice("", "")
		return nil
	case errors.Is(err, ErrOffline):
		m.setNotice(NoticeOffline, "Offline: Dein Abo wird erneut geprüft, sobald du wieder online bist.")
		return err
	case status == http.StatusPaymentRequired:
		// The server already applied its grace period: access ends now.
		next.Entitlement, next.ProductID, next.RefreshAfter, next.Plan = "", "", time.Time{}, ""
		if api := (*APIError)(nil); errors.As(err, &api) && api.Plan == PlanSupport {
			next.Plan = PlanSupport
		}
		if e := m.commit(&next); e != nil {
			return e
		}
		if next.Plan == PlanSupport {
			m.setNotice(NoticeSupportPlan, supportNotice)
			return nil
		}
		m.setNotice(NoticeNoSubscription, "Für dieses Konto ist kein papierleicht-Plus-Abo aktiv. Die App läuft als Community-Version mit allen lokalen Funktionen weiter.")
		return nil
	case status == http.StatusConflict:
		msg := "Dein Abo ist bereits auf der maximalen Anzahl von Geräten aktiv. Entferne unten ein anderes Gerät und prüfe dann noch einmal."
		if api := (*APIError)(nil); errors.As(err, &api) && api.Message != "" {
			msg = api.Message + " Entferne unten ein anderes Gerät und prüfe dann noch einmal."
		}
		m.setNotice(NoticeDeviceLimit, msg)
		return nil
	case status == http.StatusUnauthorized:
		next.AuthToken = ""
		if e := m.commit(&next); e != nil {
			return e
		}
		m.setNotice(NoticeSessionExpired, "Deine Anmeldung ist abgelaufen. Melde dich bitte noch einmal mit einem E-Mail-Code an.")
		return nil
	default:
		return err
	}
}

func (m *Manager) authToken() (string, error) {
	if !m.configured() {
		return "", ErrNotConfigured
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.sess == nil || m.sess.AuthToken == "" {
		return "", errors.New("Melde dich zuerst unter „Konto & Abo“ an.")
	}
	return m.sess.AuthToken, nil
}

// Devices lists the customer's activated installations.
func (m *Manager) Devices(ctx context.Context) ([]Device, error) {
	token, err := m.authToken()
	if err != nil {
		return nil, err
	}
	devices, err := m.client.devices(ctx, token)
	for i := range devices {
		devices[i].Current = devices[i].DeviceID == m.deviceID
	}
	return devices, m.authError(err)
}

// RemoveDevice frees a device slot. Removing the current device drops the
// local entitlement as well; the next check activates it again.
func (m *Manager) RemoveDevice(ctx context.Context, id string) (Status, error) {
	token, err := m.authToken()
	if err != nil {
		return m.Status(), err
	}
	if err := m.client.deleteDevice(ctx, token, id); err != nil && StatusOf(err) != http.StatusNotFound {
		return m.Status(), m.authError(err)
	}
	return m.Refresh(ctx, true)
}

// SignOut frees this device's slot when online and always removes the
// local session. The backup key stays in the keychain.
func (m *Manager) SignOut(ctx context.Context) (Status, error) {
	m.netMu.Lock()
	defer m.netMu.Unlock()
	if token, err := m.authToken(); err == nil {
		if devices, err := m.client.devices(ctx, token); err == nil {
			for _, d := range devices {
				if d.DeviceID == m.deviceID {
					_ = m.client.deleteDevice(ctx, token, d.ID)
				}
			}
		}
	}
	return m.forgetSession()
}

// SignOutEverywhere ends every session of the account, this one included,
// and frees all device slots. Removing a device only frees its slot; a
// stolen session would stay signed in. Without the server nothing changes,
// because the other sessions would live on.
func (m *Manager) SignOutEverywhere(ctx context.Context) (Status, error) {
	m.netMu.Lock()
	defer m.netMu.Unlock()
	token, err := m.authToken()
	if err != nil {
		return m.Status(), err
	}
	if err := m.client.signOutEverywhere(ctx, token); err != nil {
		if StatusOf(err) == http.StatusUnauthorized {
			// This session is gone, the others may not be.
			return m.Status(), errors.New("Deine Anmeldung ist abgelaufen. Melde dich mit einem neuen E-Mail-Code an und wähle dann noch einmal „Überall abmelden“.")
		}
		return m.Status(), m.authError(err)
	}
	return m.forgetSession()
}

// forgetSession removes the local session and entitlement. The backup key
// stays in the keychain.
func (m *Manager) forgetSession() (Status, error) {
	if err := m.save(nil); err != nil {
		return m.Status(), errors.New("Die Anmeldung konnte nicht aus dem Schlüsselbund entfernt werden.")
	}
	m.mu.Lock()
	m.setSessionLocked(nil)
	m.noticeKind, m.notice, m.otpID, m.otpEmail = "", "", "", ""
	m.mu.Unlock()
	return m.changed(), nil
}

func (m *Manager) authError(err error) error {
	switch StatusOf(err) {
	case http.StatusUnauthorized:
		return errors.New("Deine Anmeldung ist abgelaufen. Melde dich bitte noch einmal mit einem E-Mail-Code an.")
	case http.StatusPaymentRequired:
		return errors.New("Die verschlüsselte Sicherung gehört zum Backup-Zusatz von papierleicht Plus. Für dein Konto ist er nicht gebucht.")
	}
	return err
}

const (
	// MaxBackupDownload bounds the encrypted index of one version in memory.
	MaxBackupDownload = 1 << 30
	// MaxBackupObject bounds one encrypted document in memory.
	MaxBackupObject = 64 << 20
)

// BackupState reports storage use and what the customer may do.
func (m *Manager) BackupState(ctx context.Context) (BackupState, error) {
	token, err := m.authToken()
	if err != nil {
		return BackupState{}, err
	}
	s, err := m.client.backupState(ctx, token)
	return s, m.authError(err)
}

func (m *Manager) ListBackups(ctx context.Context) ([]BackupVersion, error) {
	token, err := m.authToken()
	if err != nil {
		return nil, err
	}
	v, err := m.client.listBackups(ctx, token)
	return v, m.authError(err)
}

// MissingObjects returns the ids the server does not store yet.
func (m *Manager) MissingObjects(ctx context.Context, ids []string) ([]string, error) {
	token, err := m.authToken()
	if err != nil {
		return nil, err
	}
	missing, err := m.client.missingObjects(ctx, token, ids)
	return missing, m.authError(err)
}

func (m *Manager) UploadObject(ctx context.Context, id string, data []byte) error {
	token, err := m.authToken()
	if err != nil {
		return err
	}
	return m.authError(m.client.uploadObject(ctx, token, id, data))
}

func (m *Manager) DownloadObject(ctx context.Context, id string) ([]byte, error) {
	token, err := m.authToken()
	if err != nil {
		return nil, err
	}
	data, err := m.client.downloadObject(ctx, token, id)
	return data, m.authError(err)
}

// UploadBackup stores the index of a state once all its objects are stored.
func (m *Manager) UploadBackup(ctx context.Context, format int, manifest []byte, objects []string, logicalSize int64) (BackupVersion, error) {
	token, err := m.authToken()
	if err != nil {
		return BackupVersion{}, err
	}
	v, err := m.client.uploadBackup(ctx, token, m.deviceID, m.cfg.DeviceName, format, manifest, objects, logicalSize)
	if StatusOf(err) == http.StatusRequestEntityTooLarge {
		return v, errors.New("Die Sicherung ist größer als das erlaubte Limit des Servers.")
	}
	return v, m.authError(err)
}

func (m *Manager) DownloadBackup(ctx context.Context, id string) ([]byte, error) {
	token, err := m.authToken()
	if err != nil {
		return nil, err
	}
	data, err := m.client.downloadBackup(ctx, token, id, MaxBackupDownload)
	return data, m.authError(err)
}

// RequestBackupDeletion marks all backups for deletion after a waiting period.
func (m *Manager) RequestBackupDeletion(ctx context.Context) (time.Time, error) {
	token, err := m.authToken()
	if err != nil {
		return time.Time{}, err
	}
	at, err := m.client.requestBackupDeletion(ctx, token)
	return at, m.authError(err)
}

// AnchorAudit asks the server to sign the current end of the change log.
func (m *Manager) AnchorAudit(ctx context.Context, seq int64, hash string) (AuditReceipt, error) {
	token, err := m.authToken()
	if err != nil {
		return AuditReceipt{}, err
	}
	receipt, err := m.client.anchorAudit(ctx, token, m.deviceID, seq, hash)
	return receipt, m.authError(err)
}

// StoredReceipt is one entry of the server's list of receipts.
type StoredReceipt struct {
	DeviceID string    `json:"deviceId"`
	Seq      int64     `json:"seq"`
	Hash     string    `json:"hash"`
	At       time.Time `json:"at"`
	Receipt  string    `json:"receipt"`
}

// AuditReceipts returns the server's list for all devices of the account. It
// needs a sign-in, not an active plan.
func (m *Manager) AuditReceipts(ctx context.Context) ([]StoredReceipt, error) {
	token, err := m.authToken()
	if err != nil {
		return nil, err
	}
	receipts, err := m.client.auditReceipts(ctx, token)
	return receipts, m.authError(err)
}

// CheckAuditReceipt verifies a receipt's signature.
func (m *Manager) CheckAuditReceipt(receipt string) (entitlement.Anchor, error) {
	return entitlement.VerifyAnchor(m.cfg.PublicKey, receipt)
}

func (m *Manager) CancelBackupDeletion(ctx context.Context) error {
	token, err := m.authToken()
	if err != nil {
		return err
	}
	return m.authError(m.client.cancelBackupDeletion(ctx, token))
}
