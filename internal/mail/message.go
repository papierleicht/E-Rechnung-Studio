// Package mail sends documents through the user's own SMTP account. It is
// document-agnostic: invoices, payment reminders and quotes share the same
// account, MIME builder, delivery and send log.
package mail

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	netmail "net/mail"
	"net/textproto"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	// MaxAttachmentBytes keeps base64-encoded messages below the common 25 MB
	// provider limit.
	MaxAttachmentBytes = 18 << 20
	maxRecipients      = 20
)

// Attachment is one file sent with a message. ContentType defaults to
// application/octet-stream.
type Attachment struct {
	Filename    string `json:"filename"`
	ContentType string `json:"contentType"`
	Data        []byte `json:"-"`
}

// AttachmentInfo describes an attachment without its contents (UI and log).
type AttachmentInfo struct {
	Filename    string `json:"filename"`
	ContentType string `json:"contentType"`
	Size        int    `json:"size"`
}

func (a Attachment) Info() AttachmentInfo {
	return AttachmentInfo{Filename: a.Filename, ContentType: a.contentType(), Size: len(a.Data)}
}

func (a Attachment) contentType() string {
	if strings.TrimSpace(a.ContentType) == "" {
		return "application/octet-stream"
	}
	return strings.TrimSpace(a.ContentType)
}

// Message is a plain-text e-mail with optional attachments. Sender, reply-to
// default and BCC-to-self come from the configured Account.
type Message struct {
	To          []string     `json:"to"`
	Cc          []string     `json:"cc"`
	Bcc         []string     `json:"bcc"`
	ReplyTo     string       `json:"replyTo"`
	Subject     string       `json:"subject"`
	Text        string       `json:"text"`
	Attachments []Attachment `json:"-"`
}

// ParseAddressList accepts comma- or semicolon-separated addresses as typed by
// users and returns normalized addresses without display names.
func ParseAddressList(value string) ([]string, error) {
	value = strings.ReplaceAll(value, ";", ",")
	if strings.TrimSpace(strings.ReplaceAll(value, ",", "")) == "" {
		return []string{}, nil
	}
	list, err := netmail.ParseAddressList(value)
	if err != nil {
		return nil, fmt.Errorf("Prüfe die E-Mail-Adresse(n) „%s“.", strings.TrimSpace(value))
	}
	out := make([]string, 0, len(list))
	for _, address := range list {
		out = append(out, address.Address)
	}
	return out, nil
}

func parseAddress(value, label string) (string, error) {
	address, err := netmail.ParseAddress(strings.TrimSpace(value))
	if err != nil || !strings.Contains(address.Address, "@") {
		return "", fmt.Errorf("Gib eine gültige %s ein.", label)
	}
	return address.Address, nil
}

func normalizeList(values []string) ([]string, error) {
	out := []string{}
	seen := map[string]bool{}
	for _, value := range values {
		parsed, err := ParseAddressList(value)
		if err != nil {
			return nil, err
		}
		for _, address := range parsed {
			key := strings.ToLower(address)
			if !seen[key] {
				seen[key] = true
				out = append(out, address)
			}
		}
	}
	return out, nil
}

func singleLine(value string) string {
	return strings.Join(strings.Fields(strings.Map(func(r rune) rune {
		if r == '\r' || r == '\n' || r == '\t' {
			return ' '
		}
		return r
	}, value)), " ")
}

// prepared is a validated message ready for delivery.
type prepared struct {
	from       string
	recipients []string
	data       []byte
	messageID  string
	message    Message
}

// prepare validates msg for account, applies account defaults and renders RFC
// 5322 bytes. BCC recipients only appear in the SMTP envelope.
func prepare(account Account, msg Message, now time.Time) (prepared, error) {
	var p prepared
	from, err := parseAddress(account.FromAddress, "Absenderadresse")
	if err != nil {
		return p, err
	}
	if msg.To, err = normalizeList(msg.To); err != nil {
		return p, err
	}
	if len(msg.To) == 0 {
		return p, errors.New("Gib mindestens einen Empfänger ein.")
	}
	if msg.Cc, err = normalizeList(msg.Cc); err != nil {
		return p, err
	}
	bcc := append([]string{}, msg.Bcc...)
	if account.BCCSelf {
		bcc = append(bcc, from)
	}
	if msg.Bcc, err = normalizeList(bcc); err != nil {
		return p, err
	}
	if strings.TrimSpace(msg.ReplyTo) == "" {
		msg.ReplyTo = account.ReplyTo
	}
	if strings.TrimSpace(msg.ReplyTo) != "" {
		if msg.ReplyTo, err = parseAddress(msg.ReplyTo, "Antwortadresse"); err != nil {
			return p, err
		}
	}
	msg.Subject = singleLine(msg.Subject)
	if msg.Subject == "" {
		return p, errors.New("Gib einen Betreff ein.")
	}
	if !utf8.ValidString(msg.Text) || !utf8.ValidString(msg.Subject) {
		return p, errors.New("Betreff und Text enthalten Zeichen, die nicht erlaubt sind.")
	}
	total := 0
	for i, attachment := range msg.Attachments {
		name := singleLine(attachment.Filename)
		if name == "" || strings.ContainsAny(name, `/\`) {
			return p, fmt.Errorf("Der Anhang %d hat keinen gültigen Dateinamen.", i+1)
		}
		msg.Attachments[i].Filename = name
		total += len(attachment.Data)
	}
	if total > MaxAttachmentBytes {
		return p, fmt.Errorf("Die Anhänge sind zu groß (%d MB). Erlaubt sind höchstens %d MB.", (total+(1<<20)-1)>>20, MaxAttachmentBytes>>20)
	}
	recipients, _ := normalizeList(append(append(append([]string{}, msg.To...), msg.Cc...), msg.Bcc...))
	if len(recipients) > maxRecipients {
		return p, fmt.Errorf("Pro E-Mail sind höchstens %d Empfänger möglich.", maxRecipients)
	}
	p.from, p.recipients, p.message = from, recipients, msg
	p.messageID = newMessageID(from)
	p.data, err = render(account, msg, from, p.messageID, now)
	return p, err
}

func newMessageID(from string) string {
	domain := "localhost"
	if at := strings.LastIndex(from, "@"); at >= 0 && at+1 < len(from) {
		domain = from[at+1:]
	}
	random := make([]byte, 12)
	_, _ = rand.Read(random)
	return "<" + hex.EncodeToString(random) + "@" + domain + ">"
}

func formatAddresses(values []string) string {
	out := make([]string, len(values))
	for i, value := range values {
		out[i] = (&netmail.Address{Address: value}).String()
	}
	return strings.Join(out, ", ")
}

func render(account Account, msg Message, from, messageID string, now time.Time) ([]byte, error) {
	var b bytes.Buffer
	header := func(name, value string) { b.WriteString(name + ": " + value + "\r\n") }
	header("From", (&netmail.Address{Name: singleLine(account.FromName), Address: from}).String())
	header("To", formatAddresses(msg.To))
	if len(msg.Cc) > 0 {
		header("Cc", formatAddresses(msg.Cc))
	}
	if msg.ReplyTo != "" {
		header("Reply-To", formatAddresses([]string{msg.ReplyTo}))
	}
	header("Subject", mime.QEncoding.Encode("utf-8", msg.Subject))
	header("Date", now.Format(time.RFC1123Z))
	header("Message-ID", messageID)
	header("MIME-Version", "1.0")
	text := strings.ReplaceAll(strings.ReplaceAll(msg.Text, "\r\n", "\n"), "\r", "\n")
	writeText := func(w interface{ Write([]byte) (int, error) }) error {
		qp := quotedprintable.NewWriter(w)
		if _, err := qp.Write([]byte(text)); err != nil {
			return err
		}
		return qp.Close()
	}
	if len(msg.Attachments) == 0 {
		header("Content-Type", "text/plain; charset=utf-8")
		header("Content-Transfer-Encoding", "quoted-printable")
		b.WriteString("\r\n")
		if err := writeText(&b); err != nil {
			return nil, err
		}
		b.WriteString("\r\n")
		return b.Bytes(), nil
	}
	var body bytes.Buffer
	parts := multipart.NewWriter(&body)
	header("Content-Type", mime.FormatMediaType("multipart/mixed", map[string]string{"boundary": parts.Boundary()}))
	b.WriteString("\r\n")
	textPart, err := parts.CreatePart(textproto.MIMEHeader{
		"Content-Type":              {"text/plain; charset=utf-8"},
		"Content-Transfer-Encoding": {"quoted-printable"},
	})
	if err != nil {
		return nil, err
	}
	if err = writeText(textPart); err != nil {
		return nil, err
	}
	for _, attachment := range msg.Attachments {
		contentType := mime.FormatMediaType(attachment.contentType(), map[string]string{"name": attachment.Filename})
		disposition := mime.FormatMediaType("attachment", map[string]string{"filename": attachment.Filename})
		if contentType == "" || disposition == "" {
			return nil, fmt.Errorf("Der Anhang „%s“ hat einen ungültigen Dateityp.", attachment.Filename)
		}
		part, partErr := parts.CreatePart(textproto.MIMEHeader{
			"Content-Type":              {contentType},
			"Content-Disposition":       {disposition},
			"Content-Transfer-Encoding": {"base64"},
		})
		if partErr != nil {
			return nil, partErr
		}
		encoded := base64.StdEncoding.EncodeToString(attachment.Data)
		for len(encoded) > 76 {
			if _, partErr = part.Write([]byte(encoded[:76] + "\r\n")); partErr != nil {
				return nil, partErr
			}
			encoded = encoded[76:]
		}
		if _, partErr = part.Write([]byte(encoded + "\r\n")); partErr != nil {
			return nil, partErr
		}
	}
	if err = parts.Close(); err != nil {
		return nil, err
	}
	b.Write(body.Bytes())
	return b.Bytes(), nil
}
