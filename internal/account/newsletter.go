package account

import (
	"context"
	"encoding/json"
	"errors"
	"net/mail"
	"os"
	"path/filepath"
	"strings"
)

// Newsletter is this installation's note of a newsletter sign-up. The server
// keeps the subscription; the app only remembers where it sent the
// confirmation mail, so it does not ask again.
type Newsletter struct {
	Email       string `json:"email"`
	RequestedAt string `json:"requestedAt"`
}

const newsletterFile = "newsletter.json"

var errNewsletterOffline = errors.New("Gerade keine Verbindung zum Server. Prüfe deine Internetverbindung und versuche es noch einmal.")

// Newsletter returns the last sign-up of this installation, if any.
func (m *Manager) Newsletter() Newsletter {
	var n Newsletter
	if m.cfg.DataDir == "" {
		return n
	}
	raw, err := os.ReadFile(filepath.Join(m.cfg.DataDir, newsletterFile))
	if err == nil {
		_ = json.Unmarshal(raw, &n)
	}
	return n
}

// SubscribeNewsletter asks the server to send a confirmation mail. Nothing is
// subscribed until the link in that mail is used (double opt-in). It needs
// no account; source names the place in the app, e.g. "app-onboarding".
func (m *Manager) SubscribeNewsletter(ctx context.Context, email, source string) (Newsletter, error) {
	email = strings.TrimSpace(email)
	if addr, err := mail.ParseAddress(email); err != nil || addr.Address != email || !strings.Contains(email[strings.LastIndex(email, "@"):], ".") {
		return Newsletter{}, errors.New("Bitte gib eine gültige E-Mail-Adresse ein.")
	}
	if m.cfg.ServerURL == "" {
		return Newsletter{}, ErrNotConfigured
	}
	err := m.client.json(ctx, "POST", "/api/newsletter/subscribe", "", map[string]string{"email": email, "source": source}, nil)
	if errors.Is(err, ErrOffline) || errors.Is(err, context.DeadlineExceeded) {
		return Newsletter{}, errNewsletterOffline
	}
	if err != nil {
		return Newsletter{}, err
	}
	n := Newsletter{Email: email, RequestedAt: m.cfg.Now().UTC().Format("2006-01-02T15:04:05Z")}
	if m.cfg.DataDir != "" {
		raw, _ := json.Marshal(n)
		if err := os.WriteFile(filepath.Join(m.cfg.DataDir, newsletterFile), raw, 0o600); err != nil {
			return n, err
		}
	}
	return n, nil
}
