package account

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	_ "modernc.org/sqlite"
	"papierleicht.de/e-rechnung-studio/internal/banking"
	"papierleicht.de/e-rechnung-studio/internal/entitlement"
)

type memVault struct {
	mu sync.Mutex
	m  map[string]string
}

func (v *memVault) Get(k string) (string, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.m[k], nil
}
func (v *memVault) Set(k, s string) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.m[k] = s
	return nil
}
func (v *memVault) Delete(k string) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	delete(v.m, k)
	return nil
}

const testProductID = "PAPIERLEICHT0000000000001"

// fakeServer mimics the license server routes the app uses.
type fakeServer struct {
	t          *testing.T
	srv        *httptest.Server
	key        ed25519.PrivateKey
	mu         sync.Mutex
	now        time.Time
	plan       string
	entStatus  int
	device     string // overrides the device in issued tokens
	devices    map[string]string
	calls      map[string]int
	refreshBad bool
	// signOutStatus makes sign-out-everywhere fail; revoked is set once it
	// succeeded and ends every session.
	signOutStatus int
	revoked       bool
}

func newFake(t *testing.T) (*fakeServer, ed25519.PublicKey) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	f := &fakeServer{t: t, key: priv, now: time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC), plan: entitlement.PlanPlusBackup, entStatus: http.StatusOK, devices: map[string]string{}, calls: map[string]int{}}
	f.srv = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.srv.Close)
	return f, pub
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func (f *fakeServer) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls[r.URL.Path]++
	var body map[string]string
	_ = json.NewDecoder(r.Body).Decode(&body)
	authed := r.Header.Get("Authorization") == "auth-token" && !f.revoked
	switch {
	case r.URL.Path == "/api/license/sign-out-everywhere" && r.Method == "POST":
		switch {
		case !authed:
			writeJSON(w, 401, map[string]string{"message": "The request requires valid record authorization token."})
		case f.signOutStatus != 0:
			writeJSON(w, f.signOutStatus, map[string]string{"message": "Status " + http.StatusText(f.signOutStatus)})
		default:
			clear(f.devices)
			f.revoked = true
			w.WriteHeader(http.StatusNoContent)
		}
	case r.URL.Path == "/api/collections/users/request-otp":
		writeJSON(w, 200, map[string]string{"otpId": "otp-1"})
	case r.URL.Path == "/api/collections/users/auth-with-otp":
		if body["otpId"] != "otp-1" || body["password"] != "123456" {
			writeJSON(w, 400, map[string]string{"message": "Failed to authenticate."})
			return
		}
		writeJSON(w, 200, map[string]any{"token": "auth-token", "record": map[string]string{"id": "user1", "email": "kunde@example.com"}})
	case r.URL.Path == "/api/collections/users/auth-refresh":
		if !authed || f.refreshBad {
			writeJSON(w, 401, map[string]string{"message": "The request requires valid record authorization token."})
			return
		}
		writeJSON(w, 200, map[string]any{"token": "auth-token", "record": map[string]string{"id": "user1", "email": "kunde@example.com"}})
	case r.URL.Path == "/api/license/entitlement":
		if !authed {
			writeJSON(w, 401, map[string]string{"message": "unauthorized"})
			return
		}
		if f.entStatus != http.StatusOK {
			if f.entStatus == http.StatusPaymentRequired && f.plan == PlanSupport {
				w.Header().Set("X-Papierleicht-Plan", PlanSupport)
			}
			writeJSON(w, f.entStatus, map[string]string{"message": "Status " + http.StatusText(f.entStatus)})
			return
		}
		dev := body["deviceId"]
		f.devices["rec-"+dev] = dev
		if f.device != "" {
			dev = f.device
		}
		exp := f.now.Add(14 * 24 * time.Hour)
		token, err := entitlement.Sign(f.key, entitlement.Claims{Subject: "user1", Device: dev, Plan: f.plan, Features: entitlement.FeaturesForPlan(f.plan), IssuedAt: f.now.Unix(), Expires: exp.Unix()})
		if err != nil {
			f.t.Fatal(err)
		}
		writeJSON(w, 200, map[string]any{"token": token, "productId": testProductID, "expiresAt": exp, "refreshAfter": f.now.Add(24 * time.Hour)})
	case r.URL.Path == "/api/audit/anchors" && r.Method == "GET":
		if !authed {
			writeJSON(w, 401, map[string]string{"message": "unauthorized"})
			return
		}
		writeJSON(w, 200, map[string]any{"items": []map[string]any{{"deviceId": "geraet", "seq": 3, "hash": "abc", "at": f.now, "receipt": "r"}}})
	case r.URL.Path == "/api/collections/devices/records" && r.Method == "GET":
		items := []map[string]string{}
		for id, dev := range f.devices {
			items = append(items, map[string]string{"id": id, "device_id": dev, "name": "Gerät"})
		}
		writeJSON(w, 200, map[string]any{"items": items})
	case strings.HasPrefix(r.URL.Path, "/api/collections/devices/records/") && r.Method == "DELETE":
		delete(f.devices, strings.TrimPrefix(r.URL.Path, "/api/collections/devices/records/"))
		w.WriteHeader(http.StatusNoContent)
	default:
		writeJSON(w, 404, map[string]string{"message": "not found"})
	}
}

func (f *fakeServer) set(fn func()) {
	f.mu.Lock()
	defer f.mu.Unlock()
	fn()
}

func (f *fakeServer) count(path string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls[path]
}

type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *clock) add(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

func newManager(t *testing.T, url string, pub ed25519.PublicKey, vault Vault, dir string, c *clock) *Manager {
	t.Helper()
	m, err := New(Config{ServerURL: url, PublicKey: pub, DeviceName: "Testgerät", DataDir: dir, Vault: vault, Now: c.now, ManageURL: "https://example.com/konto"})
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func signIn(t *testing.T, m *Manager) (Status, error) {
	t.Helper()
	ctx := context.Background()
	if _, err := m.RequestCode(ctx, "kunde@example.com"); err != nil {
		t.Fatal(err)
	}
	if st := m.Status(); !st.CodePending || st.PendingMail != "kunde@example.com" {
		t.Fatalf("code not pending: %+v", st)
	}
	return m.SignIn(ctx, " 123 456 ")
}

func TestNotConfigured(t *testing.T) {
	m, err := New(Config{DataDir: t.TempDir(), Vault: &memVault{m: map[string]string{}}})
	if err != nil {
		t.Fatal(err)
	}
	if st := m.Status(); st.Configured || st.State != StateSignedOut {
		t.Fatalf("%+v", st)
	}
	if _, err := m.RequestCode(context.Background(), "kunde@example.com"); !errors.Is(err, ErrNotConfigured) {
		t.Fatal(err)
	}
	if id, _ := m.ProductID(); id != "" {
		t.Fatal("product ID without configuration")
	}
}

func TestSignInPlanAndOfflineValidity(t *testing.T) {
	f, pub := newFake(t)
	vault := &memVault{m: map[string]string{}}
	dir := t.TempDir()
	c := &clock{t: f.now}
	m := newManager(t, f.srv.URL, pub, vault, dir, c)

	if _, err := m.RequestCode(context.Background(), "keine-adresse"); err == nil {
		t.Fatal("invalid e-mail accepted")
	}
	if _, err := m.SignIn(context.Background(), "123456"); err == nil {
		t.Fatal("sign-in without requested code")
	}
	m.RequestCode(context.Background(), "kunde@example.com")
	if _, err := m.SignIn(context.Background(), "000000"); err == nil || !strings.Contains(err.Error(), "Code ist falsch") {
		t.Fatalf("wrong code: %v", err)
	}
	st, err := signIn(t, m)
	if err != nil {
		t.Fatal(err)
	}
	if st.State != StatePlusBackup || st.Email != "kunde@example.com" || st.ExpiresAt != "2026-11-09T12:00:00Z" || st.CodePending {
		t.Fatalf("status %+v", st)
	}
	if id, _ := m.ProductID(); id != testProductID || !m.HasFeature(entitlement.FeatureBackup) {
		t.Fatal("product ID or backup feature missing")
	}
	if !strings.Contains(vault.m[sessionKey], testProductID) {
		t.Fatal("session not stored in the keychain")
	}

	// A restart without network uses the keychain entry until expiry.
	f.srv.Close()
	offline := newManager(t, f.srv.URL, pub, vault, dir, c)
	if st := offline.Status(); st.State != StatePlusBackup {
		t.Fatalf("offline restart: %+v", st)
	}
	c.add(25 * time.Hour)
	if st, err := offline.Refresh(context.Background(), false); !errors.Is(err, ErrOffline) || st.State != StatePlusBackup || st.NoticeKind != NoticeOffline {
		t.Fatalf("offline refresh: %+v %v", st, err)
	}
	c.add(14 * 24 * time.Hour)
	if st := offline.Status(); st.State != StateSignedIn || st.NoticeKind != NoticeSessionExpired {
		t.Fatalf("after expiry: %+v", st)
	}
	if id, _ := offline.ProductID(); id != "" {
		t.Fatal("expired entitlement still delivers the product ID")
	}
}

func TestRefreshOnlyWhenDue(t *testing.T) {
	f, pub := newFake(t)
	c := &clock{t: f.now}
	m := newManager(t, f.srv.URL, pub, &memVault{m: map[string]string{}}, t.TempDir(), c)
	if _, err := signIn(t, m); err != nil {
		t.Fatal(err)
	}
	m.Refresh(context.Background(), false)
	if n := f.count("/api/license/entitlement"); n != 1 {
		t.Fatalf("refreshed before refreshAfter: %d calls", n)
	}
	c.add(24 * time.Hour)
	f.set(func() { f.plan = entitlement.PlanPlus })
	st, err := m.Refresh(context.Background(), false)
	if err != nil || st.State != StatePlus || f.count("/api/license/entitlement") != 2 || f.count("/api/collections/users/auth-refresh") != 1 {
		t.Fatalf("due refresh: %+v %v", st, err)
	}
	if m.HasFeature(entitlement.FeatureBackup) {
		t.Fatal("downgrade kept the backup feature")
	}
}

func TestRejectsForeignTokens(t *testing.T) {
	for name, setup := range map[string]func(f *fakeServer){
		"other device": func(f *fakeServer) { f.device = "other-device-aaaaaaaaa" },
		"other key":    func(f *fakeServer) { _, f.key, _ = ed25519.GenerateKey(rand.Reader) },
	} {
		f, pub := newFake(t)
		f.set(func() { setup(f) })
		m := newManager(t, f.srv.URL, pub, &memVault{m: map[string]string{}}, t.TempDir(), &clock{t: f.now})
		st, err := signIn(t, m)
		if err == nil || st.State != StateSignedIn || st.NoticeKind != NoticeInvalidToken {
			t.Errorf("%s: %+v %v", name, st, err)
		}
		if id, _ := m.ProductID(); id != "" {
			t.Errorf("%s: product ID from a foreign token", name)
		}
	}
}

func TestNoSubscriptionAndDeviceLimit(t *testing.T) {
	f, pub := newFake(t)
	c := &clock{t: f.now}
	m := newManager(t, f.srv.URL, pub, &memVault{m: map[string]string{}}, t.TempDir(), c)
	if _, err := signIn(t, m); err != nil {
		t.Fatal(err)
	}
	// Cancellation: the server answers 402 and access ends immediately.
	f.set(func() { f.entStatus = http.StatusPaymentRequired })
	st, err := m.Refresh(context.Background(), true)
	if err != nil || st.State != StateSignedIn || st.NoticeKind != NoticeNoSubscription || !strings.Contains(st.Notice, "Community") {
		t.Fatalf("402: %+v %v", st, err)
	}
	if id, _ := m.ProductID(); id != "" {
		t.Fatal("product ID kept after 402")
	}

	// Community Support: still no app features, but the plan is named, also
	// after a restart; a later Plus booking clears it.
	f.set(func() { f.plan = PlanSupport })
	st, err = m.Refresh(context.Background(), true)
	if err != nil || st.State != StateSignedIn || st.Plan != PlanSupport || st.NoticeKind != NoticeSupportPlan || !strings.Contains(st.Notice, "Chat & Markt") {
		t.Fatalf("support: %+v %v", st, err)
	}
	if again := newManager(t, f.srv.URL, pub, m.cfg.Vault, m.cfg.DataDir, c).Status(); again.Plan != PlanSupport {
		t.Fatalf("support after restart: %+v", again)
	}
	f.set(func() { f.plan, f.entStatus = entitlement.PlanPlusBackup, http.StatusOK })
	if st, _ = m.Refresh(context.Background(), true); st.Plan != entitlement.PlanPlusBackup || st.State != StatePlusBackup || st.NoticeKind != "" {
		t.Fatalf("support to plus: %+v", st)
	}

	f.set(func() {
		f.entStatus = http.StatusConflict
		f.devices["rec-x"] = "fremdes-geraet-aaaaaaaa"
	})
	st, _ = m.Refresh(context.Background(), true)
	if st.NoticeKind != NoticeDeviceLimit {
		t.Fatalf("409: %+v", st)
	}
	devices, err := m.Devices(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var current, other int
	for _, d := range devices {
		if d.Current {
			current++
		} else {
			other++
		}
	}
	if current != 1 || other != 1 {
		t.Fatalf("devices %+v", devices)
	}
	f.set(func() { f.entStatus = http.StatusOK })
	if st, err := m.RemoveDevice(context.Background(), "rec-x"); err != nil || st.State != StatePlusBackup || st.NoticeKind != "" {
		t.Fatalf("after removing a device: %+v %v", st, err)
	}
}

func TestExpiredSessionKeepsOfflineEntitlement(t *testing.T) {
	f, pub := newFake(t)
	c := &clock{t: f.now}
	m := newManager(t, f.srv.URL, pub, &memVault{m: map[string]string{}}, t.TempDir(), c)
	if _, err := signIn(t, m); err != nil {
		t.Fatal(err)
	}
	f.set(func() { f.refreshBad = true })
	c.add(25 * time.Hour)
	st, err := m.Refresh(context.Background(), false)
	if err != nil || st.State != StatePlusBackup || st.NoticeKind != NoticeSessionExpired {
		t.Fatalf("%+v %v", st, err)
	}
	if _, err := m.ListBackups(context.Background()); err == nil {
		t.Fatal("backup access without session")
	}
}

func TestSignOutRemovesDeviceAndSession(t *testing.T) {
	f, pub := newFake(t)
	vault := &memVault{m: map[string]string{"backup-key": "bleibt"}}
	m := newManager(t, f.srv.URL, pub, vault, t.TempDir(), &clock{t: f.now})
	if _, err := signIn(t, m); err != nil {
		t.Fatal(err)
	}
	st, err := m.SignOut(context.Background())
	if err != nil || st.State != StateSignedOut {
		t.Fatalf("%+v %v", st, err)
	}
	if _, ok := vault.m[sessionKey]; ok || vault.m["backup-key"] != "bleibt" {
		t.Fatalf("vault after sign-out: %v", vault.m)
	}
	f.mu.Lock()
	n := len(f.devices)
	f.mu.Unlock()
	if n != 0 {
		t.Fatal("device slot not freed")
	}
}

func TestSignOutEverywhereEndsEverySession(t *testing.T) {
	f, pub := newFake(t)
	vault := &memVault{m: map[string]string{"backup-key": "bleibt"}}
	m := newManager(t, f.srv.URL, pub, vault, t.TempDir(), &clock{t: f.now})
	if _, err := signIn(t, m); err != nil {
		t.Fatal(err)
	}
	f.set(func() { f.devices["rec-fremd"] = "fremdes-geraet" })
	st, err := m.SignOutEverywhere(context.Background())
	if err != nil || st.State != StateSignedOut {
		t.Fatalf("%+v %v", st, err)
	}
	if _, ok := vault.m[sessionKey]; ok || vault.m["backup-key"] != "bleibt" {
		t.Fatalf("vault after signing out everywhere: %v", vault.m)
	}
	f.mu.Lock()
	n, revoked := len(f.devices), f.revoked
	f.mu.Unlock()
	if n != 0 || !revoked {
		t.Fatalf("server kept %d devices, revoked %v", n, revoked)
	}
}

func TestSignOutEverywhereKeepsTheSessionWhenItFails(t *testing.T) {
	for name, tc := range map[string]struct {
		setup func(f *fakeServer)
		want  string
	}{
		"server error":    {func(f *fakeServer) { f.signOutStatus = http.StatusInternalServerError }, ""},
		"session expired": {func(f *fakeServer) { f.revoked = true }, "noch einmal „Überall abmelden“"},
	} {
		t.Run(name, func(t *testing.T) {
			f, pub := newFake(t)
			vault := &memVault{m: map[string]string{}}
			m := newManager(t, f.srv.URL, pub, vault, t.TempDir(), &clock{t: f.now})
			if _, err := signIn(t, m); err != nil {
				t.Fatal(err)
			}
			f.set(func() { tc.setup(f) })
			// The other sessions may live on, so this one is not dropped
			// as if it had worked.
			st, err := m.SignOutEverywhere(context.Background())
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %v", err)
			}
			if st.State == StateSignedOut {
				t.Fatalf("signed out locally: %+v", st)
			}
			if _, ok := vault.m[sessionKey]; !ok {
				t.Fatal("session removed from the keychain")
			}
		})
	}
}

func TestSignOutEverywhereNeedsASession(t *testing.T) {
	f, pub := newFake(t)
	m := newManager(t, f.srv.URL, pub, &memVault{m: map[string]string{}}, t.TempDir(), &clock{t: f.now})
	if _, err := m.SignOutEverywhere(context.Background()); err == nil {
		t.Fatal("signed out everywhere without a session")
	}
	if f.count("/api/license/sign-out-everywhere") != 0 {
		t.Fatal("called the server without a session")
	}
}

func TestBankingUsesSubscriptionProductID(t *testing.T) {
	f, pub := newFake(t)
	c := &clock{t: f.now}
	m := newManager(t, f.srv.URL, pub, &memVault{m: map[string]string{}}, t.TempDir(), c)
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	bank, err := banking.New(db, t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	defer bank.Close()
	bank.SetSubscriptionSource(m.ProductID)
	source := func() string {
		o, err := bank.Overview()
		if err != nil {
			t.Fatal(err)
		}
		return o.ProductSource
	}
	if got := source(); got != "" {
		t.Fatalf("signed out: %q", got)
	}
	if _, err := signIn(t, m); err != nil {
		t.Fatal(err)
	}
	if got := source(); got != banking.ProductSourceSubscription {
		t.Fatalf("with Plus: %q", got)
	}
	c.add(15 * 24 * time.Hour)
	if got := source(); got != "" {
		t.Fatalf("after offline expiry: %q", got)
	}
}

func TestDeviceIDIsStable(t *testing.T) {
	dir := t.TempDir()
	a, _ := loadDeviceID(dir)
	b, _ := loadDeviceID(dir)
	if a != b || !deviceIDPattern.MatchString(a) {
		t.Fatalf("%q %q", a, b)
	}
}

func TestServerListOfReceiptsNeedsSignIn(t *testing.T) {
	f, pub := newFake(t)
	m := newManager(t, f.srv.URL, pub, &memVault{m: map[string]string{}}, t.TempDir(), &clock{t: f.now})
	ctx := context.Background()
	if _, err := m.AuditReceipts(ctx); err == nil {
		t.Fatal("list without sign-in")
	}
	if _, err := signIn(t, m); err != nil {
		t.Fatal(err)
	}
	list, err := m.AuditReceipts(ctx)
	if err != nil || len(list) != 1 || list[0].Seq != 3 || list[0].DeviceID != "geraet" {
		t.Fatalf("list: %+v %v", list, err)
	}
}
