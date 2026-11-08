package mail

import (
	"context"
	"crypto/x509"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/zalando/go-keyring"
	_ "modernc.org/sqlite"
)

const (
	keyringService = "de.papierleicht.smtp"
	keyringID      = "default"
	StatusSent     = "sent"
	StatusFailed   = "failed"
)

// Vault stores the SMTP password outside the database.
type Vault interface {
	Get(string) (string, error)
	Set(string, string) error
	Delete(string) error
}

type systemVault struct{}

func (systemVault) Get(id string) (string, error) { return keyring.Get(keyringService, id) }
func (systemVault) Set(id, secret string) error   { return keyring.Set(keyringService, id, secret) }
func (systemVault) Delete(id string) error {
	e := keyring.Delete(keyringService, id)
	if errors.Is(e, keyring.ErrNotFound) {
		return nil
	}
	return e
}

// Settings is the account and template configuration shown in the settings
// page. HasPassword is computed from the keyring and never persisted.
type Settings struct {
	Account     Account             `json:"account"`
	Templates   map[string]Template `json:"templates"`
	HasPassword bool                `json:"hasPassword"`
}

// LogEntry records one delivery attempt for a document.
type LogEntry struct {
	ID           string           `json:"id"`
	DocumentType string           `json:"documentType"`
	DocumentID   string           `json:"documentId"`
	SentAt       string           `json:"sentAt"`
	Status       string           `json:"status"`
	From         string           `json:"from"`
	To           []string         `json:"to"`
	Cc           []string         `json:"cc"`
	Bcc          []string         `json:"bcc"`
	Subject      string           `json:"subject"`
	Attachments  []AttachmentInfo `json:"attachments"`
	MessageID    string           `json:"messageId"`
	Error        string           `json:"error,omitempty"`
}

// Outgoing is a message for a document. DocumentType and DocumentID link the
// send log to the document; leave both empty for messages without a log.
type Outgoing struct {
	DocumentType string
	DocumentID   string
	Message      Message
}

// Options replace system integrations, mainly for tests.
type Options struct {
	Vault   Vault
	RootCAs *x509.CertPool
	Now     func() time.Time
}

type Service struct {
	mu        sync.Mutex
	sendMu    sync.Mutex
	db        *sql.DB
	vault     Vault
	transport transport
	now       func() time.Time
}

// New opens the mail tables in the application database in dir.
func New(dir string) (*Service, error) { return NewWithOptions(dir, Options{}) }

func NewWithOptions(dir string, o Options) (*Service, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", filepath.Join(dir, "e-rechnung-studio.db"))
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	// Additive migration: existing databases simply gain the two tables.
	_, err = db.Exec(`PRAGMA busy_timeout=5000;
 CREATE TABLE IF NOT EXISTS mail_settings (id INTEGER PRIMARY KEY CHECK(id=1), data BLOB NOT NULL);
 CREATE TABLE IF NOT EXISTS mail_log (id TEXT PRIMARY KEY, document_type TEXT NOT NULL, document_id TEXT NOT NULL, sent_at TEXT NOT NULL, status TEXT NOT NULL CHECK(status IN ('sent','failed')), data BLOB NOT NULL);
 CREATE INDEX IF NOT EXISTS mail_log_document ON mail_log(document_type,document_id,sent_at);`)
	if err != nil {
		db.Close()
		return nil, err
	}
	s := &Service{db: db, vault: o.Vault, transport: transport{rootCAs: o.RootCAs}, now: o.Now}
	if s.vault == nil {
		s.vault = systemVault{}
	}
	if s.now == nil {
		s.now = time.Now
	}
	return s, nil
}

// SetReadOnly follows the read-only view of a tax audit. The service keeps its
// own connection, so the switch of the store does not reach it.
func (s *Service) SetReadOnly(on bool) error {
	mode := "OFF"
	if on {
		mode = "ON"
	}
	_, err := s.db.Exec("PRAGMA query_only=" + mode)
	return err
}

func (s *Service) Close() error { return s.db.Close() }

func (s *Service) settings() (Settings, error) {
	v := Settings{Account: Account{Security: SecuritySTARTTLS, Port: 587}, Templates: map[string]Template{}}
	var raw []byte
	err := s.db.QueryRow("SELECT data FROM mail_settings WHERE id=1").Scan(&raw)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return v, err
	}
	if err == nil {
		if err = json.Unmarshal(raw, &v); err != nil {
			return v, err
		}
	}
	if v.Templates == nil {
		v.Templates = map[string]Template{}
	}
	for kind, t := range DefaultTemplates {
		if _, ok := v.Templates[kind]; !ok {
			v.Templates[kind] = t
		}
	}
	v.HasPassword = false
	return v, nil
}

func (s *Service) password() (string, error) {
	secret, err := s.vault.Get(keyringID)
	if errors.Is(err, keyring.ErrNotFound) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("Das SMTP-Passwort konnte nicht aus dem Schlüsselbund gelesen werden: %w", err)
	}
	return secret, nil
}

// Settings returns the stored configuration with default templates filled in.
func (s *Service) Settings() (Settings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, err := s.settings()
	if err != nil {
		return v, err
	}
	secret, err := s.password()
	v.HasPassword = err == nil && secret != ""
	return v, nil
}

// Template returns the effective template for a document type.
func (s *Service) Template(documentType string) (Template, error) {
	v, err := s.Settings()
	if err != nil {
		return Template{}, err
	}
	t, ok := v.Templates[documentType]
	if !ok {
		return Template{}, fmt.Errorf("Für „%s“ gibt es keine E-Mail-Vorlage.", documentType)
	}
	return t, nil
}

// SaveSettings validates and stores the configuration. A non-empty password
// replaces the keyring entry; an empty one keeps the existing entry.
func (s *Service) SaveSettings(v Settings, password string) (Settings, error) {
	account, err := NormalizeAccount(v.Account)
	if err != nil {
		return v, err
	}
	v.Account = account
	for kind, t := range v.Templates {
		if err = ValidateTemplate(kind, t); err != nil {
			return v, err
		}
	}
	v.HasPassword = false
	raw, err := json.Marshal(v)
	if err != nil {
		return v, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if password != "" {
		if err = s.vault.Set(keyringID, password); err != nil {
			return v, fmt.Errorf("Das Passwort konnte nicht im Schlüsselbund gespeichert werden: %w", err)
		}
	}
	if _, err = s.db.Exec("INSERT INTO mail_settings(id,data) VALUES(1,?) ON CONFLICT(id) DO UPDATE SET data=excluded.data", raw); err != nil {
		return v, err
	}
	saved, err := s.settings()
	if err != nil {
		return saved, err
	}
	secret, _ := s.password()
	saved.HasPassword = secret != ""
	return saved, nil
}

// ForgetPassword removes the SMTP password from the keyring.
func (s *Service) ForgetPassword() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.vault.Delete(keyringID)
}

// TestAccount sends a short test message to the account's own address using
// the given (possibly unsaved) account. An empty password uses the stored one.
// Nothing is logged or persisted.
func (s *Service) TestAccount(ctx context.Context, account Account, password string) error {
	account, err := NormalizeAccount(account)
	if err != nil {
		return err
	}
	if password == "" {
		s.mu.Lock()
		password, err = s.password()
		s.mu.Unlock()
		if err != nil {
			return err
		}
	}
	account.BCCSelf = false
	p, err := prepare(account, Message{
		To:      []string{account.FromAddress},
		Subject: "Testnachricht von E-Rechnung-Studio",
		Text:    "Diese Testnachricht zeigt: E-Rechnung-Studio kann über dein SMTP-Konto E-Mails versenden.\n",
	}, s.now())
	if err != nil {
		return err
	}
	s.sendMu.Lock()
	defer s.sendMu.Unlock()
	return s.transport.send(ctx, account, password, p)
}

// Send delivers a message with the saved account and records the result in
// the send log when a document is referenced. On delivery failure the
// returned entry has status "failed" and the error is returned as well.
// Validation errors (e.g. no recipient) are returned without a log entry.
func (s *Service) Send(ctx context.Context, o Outgoing) (LogEntry, error) {
	s.mu.Lock()
	settings, err := s.settings()
	var secret string
	if err == nil {
		secret, err = s.password()
	}
	s.mu.Unlock()
	if err != nil {
		return LogEntry{}, err
	}
	if !settings.Account.Configured() {
		return LogEntry{}, errors.New("Richte zuerst dein E-Mail-Konto in den Einstellungen ein.")
	}
	if settings.Account.Username != "" && secret == "" {
		return LogEntry{}, errors.New("Für das E-Mail-Konto ist kein Passwort im Schlüsselbund gespeichert. Gib es in den Einstellungen ein.")
	}
	if (o.DocumentType == "") != (o.DocumentID == "") {
		return LogEntry{}, errors.New("Dokumenttyp und Dokument müssen gemeinsam angegeben werden.")
	}
	sentAt := s.now()
	p, err := prepare(settings.Account, o.Message, sentAt)
	if err != nil {
		return LogEntry{}, err
	}
	entry := LogEntry{ID: uuid.NewString(), DocumentType: o.DocumentType, DocumentID: o.DocumentID,
		SentAt: sentAt.UTC().Format(time.RFC3339), Status: StatusSent, From: p.from,
		To: p.message.To, Cc: p.message.Cc, Bcc: p.message.Bcc, Subject: p.message.Subject,
		Attachments: []AttachmentInfo{}, MessageID: p.messageID}
	for _, attachment := range p.message.Attachments {
		entry.Attachments = append(entry.Attachments, attachment.Info())
	}
	s.sendMu.Lock()
	sendErr := s.transport.send(ctx, settings.Account, secret, p)
	s.sendMu.Unlock()
	if sendErr != nil {
		entry.Status, entry.Error = StatusFailed, sendErr.Error()
	}
	if o.DocumentID != "" {
		if err = s.record(entry); err != nil {
			if sendErr == nil {
				return entry, fmt.Errorf("Die E-Mail wurde versendet, der Eintrag im Versandprotokoll konnte aber nicht gespeichert werden: %w", err)
			}
			return entry, errors.Join(sendErr, err)
		}
	}
	return entry, sendErr
}

func (s *Service) record(entry LogEntry) error {
	raw, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err = s.db.Exec("INSERT INTO mail_log(id,document_type,document_id,sent_at,status,data) VALUES(?,?,?,?,?,?)",
		entry.ID, entry.DocumentType, entry.DocumentID, entry.SentAt, entry.Status, raw)
	return err
}

// Log returns the send attempts for one document, newest first.
func (s *Service) Log(documentType, documentID string) ([]LogEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.query("SELECT data FROM mail_log WHERE document_type=? AND document_id=? ORDER BY sent_at DESC,id DESC", documentType, documentID)
}

func (s *Service) query(query string, args ...any) ([]LogEntry, error) {
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []LogEntry{}
	for rows.Next() {
		var raw []byte
		var entry LogEntry
		if err = rows.Scan(&raw); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(raw, &entry); err != nil {
			return nil, err
		}
		out = append(out, entry)
	}
	return out, rows.Err()
}

// LastSent maps document IDs of documentType to their latest successful send.
func (s *Service) LastSent(documentType string) (map[string]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.db.Query("SELECT document_id,MAX(sent_at) FROM mail_log WHERE document_type=? AND status='sent' GROUP BY document_id", documentType)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var id, at string
		if err = rows.Scan(&id, &at); err != nil {
			return nil, err
		}
		out[id] = at
	}
	return out, rows.Err()
}
