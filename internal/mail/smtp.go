package mail

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/smtp"
	"net/textproto"
	"strconv"
	"strings"
	"time"
)

const (
	SecuritySTARTTLS = "starttls"
	SecurityTLS      = "tls"
	dialTimeout      = 20 * time.Second
	sessionTimeout   = 3 * time.Minute
)

// Account is the non-secret SMTP configuration. The password is stored only in
// the operating-system keyring and is never part of this struct.
type Account struct {
	Preset      string `json:"preset"`
	Host        string `json:"host"`
	Port        int    `json:"port"`
	Security    string `json:"security"`
	Username    string `json:"username"`
	FromName    string `json:"fromName"`
	FromAddress string `json:"fromAddress"`
	ReplyTo     string `json:"replyTo"`
	BCCSelf     bool   `json:"bccSelf"`
}

// Configured reports whether the account has the minimum data for delivery.
func (a Account) Configured() bool {
	return strings.TrimSpace(a.Host) != "" && a.Port > 0 && strings.TrimSpace(a.FromAddress) != ""
}

// NormalizeAccount trims user input and validates the account.
func NormalizeAccount(a Account) (Account, error) {
	a.Preset = strings.TrimSpace(a.Preset)
	a.Host = strings.ToLower(strings.TrimSpace(a.Host))
	a.Username = strings.TrimSpace(a.Username)
	a.FromName = singleLine(a.FromName)
	a.FromAddress = strings.TrimSpace(a.FromAddress)
	a.ReplyTo = strings.TrimSpace(a.ReplyTo)
	if a.Security == "" {
		a.Security = SecuritySTARTTLS
	}
	if a.Host == "" || strings.ContainsAny(a.Host, " /:@") {
		return a, errors.New("Gib den SMTP-Server ohne Protokoll und Port ein, zum Beispiel smtp.example.de.")
	}
	if a.Port < 1 || a.Port > 65535 {
		return a, errors.New("Gib einen gültigen Port ein, meist 587 oder 465.")
	}
	if a.Security != SecuritySTARTTLS && a.Security != SecurityTLS {
		return a, errors.New("Wähle STARTTLS oder SSL/TLS als Verschlüsselung.")
	}
	var err error
	if a.FromAddress, err = parseAddress(a.FromAddress, "Absenderadresse"); err != nil {
		return a, err
	}
	if a.ReplyTo != "" {
		if a.ReplyTo, err = parseAddress(a.ReplyTo, "Antwortadresse"); err != nil {
			return a, err
		}
	}
	if a.Username == "" {
		a.Username = a.FromAddress
	}
	return a, nil
}

// transport delivers prepared messages. RootCAs is only replaced in tests.
type transport struct{ rootCAs *x509.CertPool }

func (t transport) send(ctx context.Context, account Account, password string, p prepared) error {
	ctx, cancel := context.WithTimeout(ctx, sessionTimeout)
	defer cancel()
	address := net.JoinHostPort(account.Host, strconv.Itoa(account.Port))
	tlsConfig := &tls.Config{ServerName: account.Host, MinVersion: tls.VersionTLS12, RootCAs: t.rootCAs}
	dialer := &net.Dialer{Timeout: dialTimeout}
	var conn net.Conn
	var err error
	if account.Security == SecurityTLS {
		conn, err = (&tls.Dialer{NetDialer: dialer, Config: tlsConfig}).DialContext(ctx, "tcp", address)
	} else {
		conn, err = dialer.DialContext(ctx, "tcp", address)
	}
	if err != nil {
		return friendlyError("connect", err)
	}
	deadline, _ := ctx.Deadline()
	_ = conn.SetDeadline(deadline)
	stop := context.AfterFunc(ctx, func() { conn.Close() })
	defer stop()
	client, err := smtp.NewClient(conn, account.Host)
	if err != nil {
		conn.Close()
		return friendlyError("connect", err)
	}
	defer client.Close()
	if err = client.Hello("localhost"); err != nil {
		return friendlyError("connect", err)
	}
	if account.Security == SecuritySTARTTLS {
		// Never fall back to plaintext: the password would travel unencrypted.
		if ok, _ := client.Extension("STARTTLS"); !ok {
			return errors.New("Der Server bietet kein STARTTLS an. Prüfe Port und Verschlüsselung, zum Beispiel Port 465 mit SSL/TLS.")
		}
		if err = client.StartTLS(tlsConfig); err != nil {
			return friendlyError("tls", err)
		}
	}
	if account.Username != "" && password != "" {
		ok, mechanisms := client.Extension("AUTH")
		if !ok {
			return errors.New("Der Server bietet keine Anmeldung an. Prüfe Server und Port.")
		}
		var auth smtp.Auth
		switch upper := " " + strings.ToUpper(mechanisms) + " "; {
		case strings.Contains(upper, " PLAIN "):
			auth = smtp.PlainAuth("", account.Username, password, account.Host)
		case strings.Contains(upper, " LOGIN "):
			auth = loginAuth{account.Username, password}
		default:
			return fmt.Errorf("Der Server bietet kein passendes Anmeldeverfahren an (%s). Anmeldung per OAuth wird nicht unterstützt.", strings.TrimSpace(mechanisms))
		}
		if err = client.Auth(auth); err != nil {
			return friendlyError("auth", err)
		}
	}
	if err = client.Mail(p.from); err != nil {
		return friendlyError("sender", err)
	}
	for _, recipient := range p.recipients {
		if err = client.Rcpt(recipient); err != nil {
			return fmt.Errorf("%s Empfänger: %s", friendlyError("recipient", err), recipient)
		}
	}
	writer, err := client.Data()
	if err != nil {
		return friendlyError("data", err)
	}
	if _, err = writer.Write(p.data); err != nil {
		writer.Close()
		return friendlyError("data", err)
	}
	if err = writer.Close(); err != nil {
		return friendlyError("data", err)
	}
	// The message is accepted once DATA completes; a failed QUIT is harmless.
	_ = client.Quit()
	return nil
}

// loginAuth implements the widely used AUTH LOGIN mechanism (e.g. Microsoft).
// It is only offered after TLS has been established.
type loginAuth struct{ username, password string }

func (a loginAuth) Start(server *smtp.ServerInfo) (string, []byte, error) {
	if !server.TLS {
		return "", nil, errors.New("unverschlüsselte Verbindung")
	}
	return "LOGIN", nil, nil
}

func (a loginAuth) Next(challenge []byte, more bool) ([]byte, error) {
	if !more {
		return nil, nil
	}
	prompt := strings.ToLower(strings.TrimSpace(string(challenge)))
	switch {
	case strings.Contains(prompt, "user"):
		return []byte(a.username), nil
	case strings.Contains(prompt, "pass"):
		return []byte(a.password), nil
	}
	return nil, fmt.Errorf("unerwartete Serverantwort: %s", prompt)
}

func friendlyError(stage string, err error) error {
	var protocol *textproto.Error
	code := 0
	if errors.As(err, &protocol) {
		code = protocol.Code
	}
	var netErr net.Error
	timeout := errors.As(err, &netErr) && netErr.Timeout()
	var message string
	switch {
	case errors.Is(err, context.Canceled):
		message = "Der Versand wurde abgebrochen."
	case timeout || errors.Is(err, context.DeadlineExceeded):
		message = "Der SMTP-Server antwortet nicht. Prüfe Server, Port und deine Internetverbindung."
	case stage == "connect":
		message = "Die Verbindung zum SMTP-Server hat nicht geklappt. Prüfe Server, Port und Verschlüsselung."
	case stage == "tls":
		message = "Die verschlüsselte Verbindung kam nicht zustande. Prüfe Server und Verschlüsselung."
	case stage == "auth":
		message = "Die Anmeldung am SMTP-Server ist fehlgeschlagen. Prüfe Benutzername und Passwort. Manche Anbieter verlangen ein App-Passwort."
	case stage == "sender":
		message = "Der Server lehnt die Absenderadresse ab. Meist muss sie zum Konto passen."
	case stage == "recipient":
		message = "Der Server lehnt einen Empfänger ab."
	default:
		message = "Der Server hat die E-Mail nicht angenommen."
	}
	if code > 0 {
		return fmt.Errorf("%s (Server: %d %s)", message, code, singleLine(protocol.Msg))
	}
	return fmt.Errorf("%s (%s)", message, singleLine(err.Error()))
}
