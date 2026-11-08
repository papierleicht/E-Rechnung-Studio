package account

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewsletterSignUpIsRemembered(t *testing.T) {
	var got map[string]string
	status := http.StatusAccepted
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/newsletter/subscribe" || r.Header.Get("Authorization") != "" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(&got)
		writeJSON(w, status, map[string]string{"status": "pending", "message": "Zu viele Anmeldungen."})
	}))
	defer srv.Close()
	at := time.Date(2026, 11, 2, 9, 30, 0, 0, time.UTC)
	// No public key: the newsletter works without the account setup.
	m, err := New(Config{ServerURL: srv.URL, DataDir: t.TempDir(), Vault: &memVault{m: map[string]string{}}, Now: func() time.Time { return at }})
	if err != nil {
		t.Fatal(err)
	}
	if n := m.Newsletter(); n.Email != "" {
		t.Fatalf("fresh install: %+v", n)
	}
	for _, bad := range []string{"", "kein-at", "a@b", "Name <a@example.com>"} {
		if _, err := m.SubscribeNewsletter(context.Background(), bad, "app-onboarding"); err == nil {
			t.Fatalf("%q accepted", bad)
		}
	}
	if got != nil {
		t.Fatal("invalid addresses reached the server")
	}
	n, err := m.SubscribeNewsletter(context.Background(), " kunde@example.com ", "app-onboarding")
	if err != nil {
		t.Fatal(err)
	}
	if got["email"] != "kunde@example.com" || got["source"] != "app-onboarding" {
		t.Fatalf("sent %v", got)
	}
	want := Newsletter{Email: "kunde@example.com", RequestedAt: "2026-11-02T09:30:00Z"}
	if n != want || m.Newsletter() != want {
		t.Fatalf("remembered %+v / %+v", n, m.Newsletter())
	}
	status = http.StatusTooManyRequests
	if _, err := m.SubscribeNewsletter(context.Background(), "neu@example.com", "app-settings"); err == nil || err.Error() != "Zu viele Anmeldungen." {
		t.Fatalf("rate limit: %v", err)
	}
	if m.Newsletter() != want {
		t.Fatal("a failed request replaced the note")
	}
	srv.Close()
	if _, err := m.SubscribeNewsletter(context.Background(), "neu@example.com", "app-settings"); err != errNewsletterOffline {
		t.Fatalf("offline: %v", err)
	}
}
