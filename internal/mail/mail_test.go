package mail

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"mime"
	"mime/multipart"
	netmail "net/mail"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"papierleicht.de/e-rechnung-studio/internal/mail/mailtest"
)

const secret = "Geheim-Passwort-42"

func testAccount(server *mailtest.Server, security string) Account {
	return Account{Host: server.Host, Port: server.Port, Security: security, Username: "user@example.de",
		FromName: "Müller & Söhne GmbH", FromAddress: "rechnung@example.de", ReplyTo: "buchhaltung@example.de", BCCSelf: true}
}

func newTestService(t *testing.T, server *mailtest.Server, security string) (*Service, string) {
	t.Helper()
	dir := t.TempDir()
	options := Options{Vault: &mailtest.Vault{}, Now: func() time.Time { return time.Date(2026, 10, 26, 9, 30, 0, 0, time.UTC) }}
	if server != nil {
		options.RootCAs = server.RootCAs
	}
	s, err := NewWithOptions(dir, options)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	if server != nil {
		if _, err = s.SaveSettings(Settings{Account: testAccount(server, security)}, secret); err != nil {
			t.Fatal(err)
		}
	}
	return s, dir
}

func invoiceMessage() Message {
	return Message{
		To:      []string{"Kunde <kunde@example.org>"},
		Cc:      []string{"kopie@example.org"},
		Subject: "Rechnung RE-2026-0001 über 1.190,00 € – Größe",
		Text:    "Guten Tag,\nanbei die Rechnung.\n.\nEine sehr lange Zeile " + strings.Repeat("äöü ", 40) + "\nGrüße",
		Attachments: []Attachment{
			{Filename: "Rechnung-RE-2026-0001.pdf", ContentType: "application/pdf", Data: bytes.Repeat([]byte("%PDF-1.7 binary\x00\xff"), 500)},
			{Filename: "Prüfbericht ä.xml", ContentType: "application/xml", Data: []byte("<?xml version=\"1.0\"?><Invoice/>")},
		},
	}
}

func TestMessageMIMEStructure(t *testing.T) {
	account := Account{FromName: "Müller & Söhne", FromAddress: "rechnung@example.de", ReplyTo: "antwort@example.de", BCCSelf: true}
	msg := invoiceMessage()
	msg.Bcc = []string{"geheim@example.org"}
	p, err := prepare(account, msg, time.Date(2026, 10, 26, 10, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"kunde@example.org", "kopie@example.org", "geheim@example.org", "rechnung@example.de"}
	if strings.Join(p.recipients, ",") != strings.Join(want, ",") {
		t.Fatalf("envelope recipients = %v", p.recipients)
	}
	for _, line := range strings.SplitAfter(string(p.data), "\r\n") {
		if len(line) > 1000 {
			t.Fatalf("line exceeds SMTP limit: %d", len(line))
		}
	}
	parsed, err := netmail.ReadMessage(bytes.NewReader(p.data))
	if err != nil {
		t.Fatal(err)
	}
	h := parsed.Header
	if h.Get("Bcc") != "" || strings.Contains(string(p.data), "geheim@example.org") {
		t.Fatal("BCC recipients must not appear in the message")
	}
	decoder := new(mime.WordDecoder)
	subject, err := decoder.DecodeHeader(h.Get("Subject"))
	if err != nil || subject != msg.Subject {
		t.Fatalf("subject = %q, %v", subject, err)
	}
	from, err := netmail.ParseAddress(h.Get("From"))
	if err != nil || from.Name != "Müller & Söhne" || from.Address != "rechnung@example.de" {
		t.Fatalf("from = %+v, %v", from, err)
	}
	if h.Get("Reply-To") != "<antwort@example.de>" || h.Get("Cc") != "<kopie@example.org>" || h.Get("To") != "<kunde@example.org>" {
		t.Fatalf("address headers: %v", h)
	}
	if h.Get("MIME-Version") != "1.0" || !strings.HasPrefix(h.Get("Message-ID"), "<") || !strings.HasSuffix(h.Get("Message-ID"), "@example.de>") {
		t.Fatalf("headers: %v", h)
	}
	if date, err := h.Date(); err != nil || !date.Equal(time.Date(2026, 10, 26, 10, 0, 0, 0, time.UTC)) {
		t.Fatalf("date = %v, %v", date, err)
	}
	mediaType, params, err := mime.ParseMediaType(h.Get("Content-Type"))
	if err != nil || mediaType != "multipart/mixed" {
		t.Fatalf("content type %q %v", mediaType, err)
	}
	reader := multipart.NewReader(parsed.Body, params["boundary"])
	part, err := reader.NextPart() // decodes quoted-printable transparently
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(part)
	if !strings.HasPrefix(part.Header.Get("Content-Type"), "text/plain; charset=utf-8") || strings.ReplaceAll(string(body), "\r\n", "\n") != msg.Text {
		t.Fatalf("text part = %q (%v)", body, part.Header)
	}
	for _, attachment := range msg.Attachments {
		part, err = reader.NextPart()
		if err != nil {
			t.Fatal(err)
		}
		if part.FileName() != attachment.Filename {
			t.Fatalf("filename = %q, want %q", part.FileName(), attachment.Filename)
		}
		if mediaType, _, _ := mime.ParseMediaType(part.Header.Get("Content-Type")); mediaType != attachment.ContentType {
			t.Fatalf("attachment type = %q", mediaType)
		}
		if part.Header.Get("Content-Transfer-Encoding") != "base64" {
			t.Fatal("attachment must be base64 encoded")
		}
		raw, _ := io.ReadAll(part)
		decoded, err := base64Decode(raw)
		if err != nil || !bytes.Equal(decoded, attachment.Data) {
			t.Fatalf("attachment %s content differs: %v", attachment.Filename, err)
		}
	}
	if _, err = reader.NextPart(); err != io.EOF {
		t.Fatalf("expected end of multipart, got %v", err)
	}
}

func TestPlainMessageWithoutAttachmentsAndHeaderInjection(t *testing.T) {
	p, err := prepare(Account{FromAddress: "a@example.de"}, Message{To: []string{"b@example.org"}, Subject: "Hallo\r\nBcc: evil@example.org", Text: "Text"}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := netmail.ReadMessage(bytes.NewReader(p.data))
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Header.Get("Bcc") != "" || parsed.Header.Get("Subject") != "Hallo Bcc: evil@example.org" || !strings.HasPrefix(parsed.Header.Get("Content-Type"), "text/plain") {
		t.Fatalf("headers = %v", parsed.Header)
	}
	if strings.Join(p.recipients, ",") != "b@example.org" {
		t.Fatalf("recipients = %v", p.recipients)
	}
	for _, msg := range []Message{{Subject: "x"}, {To: []string{"kein-empfänger"}, Subject: "x"}, {To: []string{"b@example.org"}}} {
		if _, err := prepare(Account{FromAddress: "a@example.de"}, msg, time.Now()); err == nil {
			t.Fatalf("invalid message accepted: %+v", msg)
		}
	}
	big := Message{To: []string{"b@example.org"}, Subject: "x", Attachments: []Attachment{{Filename: "a.pdf", Data: make([]byte, MaxAttachmentBytes+1)}}}
	if _, err := prepare(Account{FromAddress: "a@example.de"}, big, time.Now()); err == nil || !strings.Contains(err.Error(), "zu groß") {
		t.Fatalf("oversized attachment: %v", err)
	}
}

func base64Decode(raw []byte) ([]byte, error) {
	var buf bytes.Buffer
	_, err := io.Copy(&buf, base64.NewDecoder(base64.StdEncoding, bytes.NewReader(raw)))
	return buf.Bytes(), err
}

func TestSendSTARTTLSPlainAuthAndLog(t *testing.T) {
	server := mailtest.Start(t, mailtest.Config{Username: "user@example.de", Password: secret})
	s, _ := newTestService(t, server, SecuritySTARTTLS)
	entry, err := s.Send(context.Background(), Outgoing{DocumentType: DocumentInvoice, DocumentID: "inv-1", Message: invoiceMessage()})
	if err != nil {
		t.Fatal(err)
	}
	messages := server.Messages()
	if len(messages) != 1 {
		t.Fatalf("messages = %d", len(messages))
	}
	got := messages[0]
	if got.Auth != "PLAIN" || got.From != "rechnung@example.de" || strings.Join(got.To, ",") != "kunde@example.org,kopie@example.org,rechnung@example.de" {
		t.Fatalf("envelope = %+v", got)
	}
	if !bytes.Contains(got.Data, []byte("Content-Disposition: attachment; filename=Rechnung-RE-2026-0001.pdf")) {
		t.Fatalf("attachment missing:\n%s", got.Data[:400])
	}
	if entry.Status != StatusSent || entry.Error != "" || len(entry.Attachments) != 2 || entry.Attachments[0].Size != 8500 || entry.SentAt != "2026-10-26T09:30:00Z" {
		t.Fatalf("entry = %+v", entry)
	}
	log, err := s.Log(DocumentInvoice, "inv-1")
	if err != nil || len(log) != 1 || log[0].ID != entry.ID || log[0].Subject != entry.Subject || strings.Join(log[0].Bcc, ",") != "rechnung@example.de" {
		t.Fatalf("log = %+v, %v", log, err)
	}
	sent, err := s.LastSent(DocumentInvoice)
	if err != nil || sent["inv-1"] != entry.SentAt {
		t.Fatalf("last sent = %v, %v", sent, err)
	}
}

func TestSendImplicitTLSWithLoginAuth(t *testing.T) {
	server := mailtest.Start(t, mailtest.Config{Security: "tls", AuthMechanisms: "LOGIN XOAUTH2", Username: "user@example.de", Password: secret})
	s, _ := newTestService(t, server, SecurityTLS)
	if _, err := s.Send(context.Background(), Outgoing{Message: Message{To: []string{"x@example.org"}, Subject: "Hallo", Text: "Test"}}); err != nil {
		t.Fatal(err)
	}
	if got := server.Messages(); len(got) != 1 || got[0].Auth != "LOGIN" {
		t.Fatalf("messages = %+v", got)
	}
	// A message without document reference is not logged.
	if sent, _ := s.LastSent(DocumentInvoice); len(sent) != 0 {
		t.Fatalf("unexpected log: %v", sent)
	}
	if err := s.TestAccount(context.Background(), testAccount(server, SecurityTLS), ""); err != nil {
		t.Fatal(err)
	}
	if got := server.Messages(); len(got) != 2 || strings.Join(got[1].To, ",") != "rechnung@example.de" {
		t.Fatalf("test mail = %+v", got)
	}
}

func TestSendFailuresAreLogged(t *testing.T) {
	cases := []struct {
		name   string
		config mailtest.Config
		want   string
	}{
		{"wrong password", mailtest.Config{Username: "user@example.de", Password: "anders"}, "Anmeldung am SMTP-Server ist fehlgeschlagen"},
		{"rejected recipient", mailtest.Config{Username: "user@example.de", Password: secret, RejectRecipient: "kopie@example.org"}, "Empfänger: kopie@example.org"},
		{"rejected data", mailtest.Config{Username: "user@example.de", Password: secret, RejectData: true}, "554"},
		{"no starttls", mailtest.Config{Username: "user@example.de", Password: secret, NoSTARTTLS: true}, "STARTTLS"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			server := mailtest.Start(t, c.config)
			s, _ := newTestService(t, server, SecuritySTARTTLS)
			entry, err := s.Send(context.Background(), Outgoing{DocumentType: DocumentInvoice, DocumentID: "inv-2", Message: invoiceMessage()})
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("error = %v, want %q", err, c.want)
			}
			if len(server.Messages()) != 0 {
				t.Fatal("no message may be accepted")
			}
			log, logErr := s.Log(DocumentInvoice, "inv-2")
			if logErr != nil || len(log) != 1 || log[0].Status != StatusFailed || log[0].Error != err.Error() || log[0].ID != entry.ID {
				t.Fatalf("log = %+v, %v", log, logErr)
			}
			if sent, _ := s.LastSent(DocumentInvoice); len(sent) != 0 {
				t.Fatalf("failed attempt must not count as sent: %v", sent)
			}
		})
	}
}

func TestSendRequiresAccountAndPassword(t *testing.T) {
	s, _ := newTestService(t, nil, "")
	if _, err := s.Send(context.Background(), Outgoing{Message: Message{To: []string{"a@example.org"}, Subject: "x"}}); err == nil || !strings.Contains(err.Error(), "E-Mail-Konto") {
		t.Fatalf("error = %v", err)
	}
	if _, err := s.SaveSettings(Settings{Account: Account{Host: "smtp.example.de", Port: 587, FromAddress: "a@example.de"}}, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Send(context.Background(), Outgoing{Message: Message{To: []string{"a@example.org"}, Subject: "x"}}); err == nil || !strings.Contains(err.Error(), "Passwort") {
		t.Fatalf("error = %v", err)
	}
	for _, account := range []Account{
		{Host: "https://smtp.example.de", Port: 587, FromAddress: "a@example.de"},
		{Host: "smtp.example.de", Port: 0, FromAddress: "a@example.de"},
		{Host: "smtp.example.de", Port: 25, Security: "none", FromAddress: "a@example.de"},
		{Host: "smtp.example.de", Port: 587, FromAddress: "kein-absender"},
	} {
		if _, err := s.SaveSettings(Settings{Account: account}, ""); err == nil {
			t.Fatalf("invalid account accepted: %+v", account)
		}
	}
}

func TestRenderPlaceholders(t *testing.T) {
	values := map[string]string{"Rechnungsnummer": "RE-1", "Betrag": "119,00 €", "Fälligkeit": "09.11.2026", "Kunde": "ACME", "Firma": "Muster GmbH"}
	got, err := Render(DefaultTemplates[DocumentInvoice].Body, values)
	if err != nil || !strings.Contains(got, "Rechnung RE-1 über 119,00 €") || !strings.Contains(got, "Fällig: 09.11.2026") || !strings.HasSuffix(got, "Muster GmbH") {
		t.Fatalf("render = %q, %v", got, err)
	}
	if got, err = Render("Hallo { Kunde }, {Kunde}", values); err != nil || got != "Hallo ACME, ACME" {
		t.Fatalf("render = %q, %v", got, err)
	}
	if _, err = Render("Hallo {Kundenname}", values); err == nil || !strings.Contains(err.Error(), "{Kundenname}") {
		t.Fatalf("unknown placeholder: %v", err)
	}
	if err = ValidateTemplate(DocumentInvoice, Template{Subject: "Rechnung {Nummer}"}); err == nil {
		t.Fatal("invalid template accepted")
	}
	if err = ValidateTemplate(DocumentInvoice, DefaultTemplates[DocumentInvoice]); err != nil {
		t.Fatal(err)
	}
}

func TestBackupExcludesPassword(t *testing.T) {
	server := mailtest.Start(t, mailtest.Config{Username: "user@example.de", Password: secret})
	s, dir := newTestService(t, server, SecuritySTARTTLS)
	settings, err := s.Settings()
	if err != nil || !settings.HasPassword {
		t.Fatalf("settings = %+v, %v", settings, err)
	}
	settings.Templates[DocumentInvoice] = Template{Subject: "Ihre Rechnung {Rechnungsnummer}", Body: "Hallo {Kunde}"}
	if _, err = s.SaveSettings(settings, ""); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Send(context.Background(), Outgoing{DocumentType: DocumentInvoice, DocumentID: "inv-1", Message: invoiceMessage()}); err != nil {
		t.Fatal(err)
	}
	raw, err := s.ExportBackup()
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte(secret)) || bytes.Contains(raw, []byte("hasPassword\":true")) {
		t.Fatalf("backup leaks password: %s", raw)
	}
	db, _ := os.ReadFile(filepath.Join(dir, "e-rechnung-studio.db"))
	if bytes.Contains(db, []byte(secret)) {
		t.Fatal("database contains SMTP password")
	}
	var doc backupData
	if err = json.Unmarshal(raw, &doc); err != nil || doc.Settings == nil || len(doc.Log) != 1 || doc.Settings.Account.Host != server.Host {
		t.Fatalf("backup = %s, %v", raw, err)
	}

	// Restoring on another machine brings account and log, but no password.
	other, _ := newTestService(t, nil, "")
	if err = other.RestoreBackup(raw); err != nil {
		t.Fatal(err)
	}
	restored, err := other.Settings()
	if err != nil || restored.HasPassword || restored.Account.FromAddress != "rechnung@example.de" || restored.Templates[DocumentInvoice].Subject != "Ihre Rechnung {Rechnungsnummer}" {
		t.Fatalf("restored = %+v, %v", restored, err)
	}
	if log, _ := other.Log(DocumentInvoice, "inv-1"); len(log) != 1 {
		t.Fatalf("restored log = %+v", log)
	}
	// Older backups without a mail section clear the log and keep the account.
	if err = other.RestoreBackup(nil); err != nil {
		t.Fatal(err)
	}
	if log, _ := other.Log(DocumentInvoice, "inv-1"); len(log) != 0 {
		t.Fatalf("log not cleared: %+v", log)
	}
	if kept, _ := other.Settings(); kept.Account.FromAddress != "rechnung@example.de" {
		t.Fatalf("account lost: %+v", kept)
	}
	if err = other.RestoreBackup(json.RawMessage(`{"version":2}`)); err == nil {
		t.Fatal("unsupported backup accepted")
	}
	if err = s.ForgetPassword(); err != nil {
		t.Fatal(err)
	}
	if v, _ := s.Settings(); v.HasPassword {
		t.Fatal("password still present")
	}
}

func TestPresetsAreComplete(t *testing.T) {
	seen := map[string]bool{}
	for _, p := range Presets() {
		if seen[p.ID] || p.Host == "" || p.Label == "" || (p.Port != 587 && p.Port != 465) || (p.Security != SecuritySTARTTLS && p.Security != SecurityTLS) {
			t.Fatalf("invalid preset %+v", p)
		}
		if (p.Port == 465) != (p.Security == SecurityTLS) {
			t.Fatalf("port and security mismatch: %+v", p)
		}
		seen[p.ID] = true
	}
	for _, id := range []string{"gmail", "microsoft365", "ionos", "strato", "gmx", "webde", "telekom"} {
		if !seen[id] {
			t.Fatalf("missing preset %s", id)
		}
	}
}
