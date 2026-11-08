package app

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"papierleicht.de/e-rechnung-studio/internal/mail"
	"papierleicht.de/e-rechnung-studio/internal/store"
)

// Reminders and quotes are sent through the same delivery and send log as
// invoices. A reminder is recorded in its immutable history only after the
// message was accepted by the mail server.

// DocumentMailDraft is the editable preview of a reminder or quote e-mail.
type DocumentMailDraft struct {
	DocumentType    string                `json:"documentType"`
	DocumentID      string                `json:"documentId"`
	Level           int                   `json:"level,omitempty"`
	Title           string                `json:"title"`
	CustomerID      string                `json:"customerId"`
	CustomerName    string                `json:"customerName"`
	To              string                `json:"to"`
	CanSaveEmail    bool                  `json:"canSaveEmail"`
	Subject         string                `json:"subject"`
	Body            string                `json:"body"`
	From            string                `json:"from"`
	ReplyTo         string                `json:"replyTo"`
	BCCSelf         bool                  `json:"bccSelf"`
	Attachments     []mail.AttachmentInfo `json:"attachments"`
	Configured      bool                  `json:"configured"`
	HasPassword     bool                  `json:"hasPassword"`
	TemplateWarning string                `json:"templateWarning,omitempty"`
	Log             []mail.LogEntry       `json:"log"`
}

// DocumentMailRequest is the confirmed reminder or quote e-mail.
type DocumentMailRequest struct {
	DocumentType  string `json:"documentType"`
	DocumentID    string `json:"documentId"`
	Level         int    `json:"level,omitempty"`
	To            string `json:"to"`
	Cc            string `json:"cc"`
	Subject       string `json:"subject"`
	Body          string `json:"body"`
	SaveRecipient bool   `json:"saveRecipient"`
	Confirmed     bool   `json:"confirmed"`
}

// reminderDay is the day reminders are prepared and recorded; tests fix it.
func (s *MailService) reminderDay() string {
	if s.today != nil {
		return s.today()
	}
	return reminderToday()
}

// documentMail is what both document types provide for a message.
type documentMail struct {
	title, customerID, customerName, snapshotEmail, subject, body string
	values                                                        map[string]string
	attachment                                                    mail.Attachment
	extras                                                        []mail.Attachment
	reminder                                                      *PreparedReminder
}

func (s *MailService) documentMail(documentType, id string, level int) (documentMail, error) {
	switch documentType {
	case mail.DocumentReminder:
		return s.reminderMail(id, level)
	case mail.DocumentQuote:
		return s.quoteMail(id)
	}
	return documentMail{}, fmt.Errorf("Diese Belegart lässt sich nicht per E-Mail versenden: %q", documentType)
}

func (s *MailService) reminderMail(invoiceID string, level int) (documentMail, error) {
	p, err := s.invoices.prepareReminder(invoiceID, level, s.reminderDay())
	if err != nil {
		return documentMail{}, err
	}
	d, err := s.invoices.store.Invoice(invoiceID)
	if err != nil {
		return documentMail{}, fmt.Errorf("Die Rechnung wurde nicht gefunden: %w", err)
	}
	// The customer may never have received the invoice itself.
	invoice, err := s.invoiceAttachments(d, true)
	if err != nil {
		return documentMail{}, err
	}
	return documentMail{title: p.Title + " zu Rechnung " + d.Number, customerID: d.CustomerID, extras: invoice,
		customerName: d.Customer.Name, snapshotEmail: p.Recipient, subject: p.Subject, body: p.Body,
		attachment: mail.Attachment{Filename: p.FileName, ContentType: "application/pdf", Data: p.PDF},
		reminder:   &p}, nil
}

func (s *MailService) quoteMail(quoteID string) (documentMail, error) {
	pdf, q, err := s.invoices.quotePDF(quoteID)
	if err != nil {
		return documentMail{}, err
	}
	view, err := s.invoices.quoteView(q)
	if err != nil {
		return documentMail{}, err
	}
	settings, err := s.mail.Settings()
	if err != nil {
		return documentMail{}, err
	}
	valid := "—"
	if parts := strings.Split(q.ValidUntil, "-"); len(parts) == 3 {
		valid = parts[2] + "." + parts[1] + "." + parts[0]
	}
	d := q.Document
	values := map[string]string{
		"Angebotsnummer": q.Number,
		"Betrag":         germanAmount(view.Totals.Gross, d.Currency),
		"Gültig bis":     valid,
		"Kunde":          d.Customer.Name,
		"Firma":          view.Company.Name,
	}
	template := settings.Templates[mail.DocumentQuote]
	return documentMail{title: "Angebot " + q.Number, customerID: d.CustomerID, customerName: d.Customer.Name,
		snapshotEmail: d.Customer.Email, subject: template.Subject, body: template.Body, values: values,
		attachment: mail.Attachment{Filename: "Angebot-" + exportFilename(q.Number) + ".pdf",
			ContentType: "application/pdf", Data: pdf}}, nil
}

func (m documentMail) files() []mail.Attachment {
	return append([]mail.Attachment{m.attachment}, m.extras...)
}

func attachmentInfos(files []mail.Attachment) []mail.AttachmentInfo {
	out := make([]mail.AttachmentInfo, 0, len(files))
	for _, f := range files {
		out = append(out, f.Info())
	}
	return out
}

// render fills quote placeholders; reminder texts are already final.
func (m documentMail) render(subject, body string) (string, string, error) {
	if m.values == nil {
		return subject, body, nil
	}
	s, subjectErr := mail.Render(subject, m.values)
	b, bodyErr := mail.Render(body, m.values)
	return s, b, errors.Join(subjectErr, bodyErr)
}

// PrepareDocumentMail renders a reminder (level 0 = proposed level) or quote
// e-mail. Nothing is sent or recorded.
func (s *MailService) PrepareDocumentMail(documentType, id string, level int) (DocumentMailDraft, error) {
	m, err := s.documentMail(documentType, id, level)
	if err != nil {
		return DocumentMailDraft{}, err
	}
	settings, err := s.mail.Settings()
	if err != nil {
		return DocumentMailDraft{}, err
	}
	result := DocumentMailDraft{DocumentType: documentType, DocumentID: id, Title: m.title,
		CustomerID: m.customerID, CustomerName: m.customerName,
		From: settings.Account.FromAddress, ReplyTo: settings.Account.ReplyTo, BCCSelf: settings.Account.BCCSelf,
		Configured: settings.Account.Configured(), HasPassword: settings.HasPassword,
		Attachments: attachmentInfos(m.files())}
	if m.reminder != nil {
		result.Level = m.reminder.Level
	}
	result.Subject, result.Body, err = m.render(m.subject, m.body)
	if err != nil {
		result.TemplateWarning = err.Error()
	}
	current, found, err := s.customer(m.customerID)
	if err != nil {
		return result, err
	}
	result.To = strings.TrimSpace(current.Email)
	if result.To == "" {
		result.To = strings.TrimSpace(m.snapshotEmail)
	}
	result.CanSaveEmail = found && strings.TrimSpace(current.Email) == ""
	result.Log, err = s.mail.Log(documentType, id)
	return result, err
}

// SendDocumentMail sends a reminder or quote after explicit confirmation. A
// sent reminder is added to the reminder history with the e-mail channel.
func (s *MailService) SendDocumentMail(ctx context.Context, r DocumentMailRequest) (mail.LogEntry, error) {
	if s.readOnly() {
		return mail.LogEntry{}, errMailReadOnly
	}
	if !r.Confirmed {
		return mail.LogEntry{}, errors.New("Bestätige den Versand ausdrücklich.")
	}
	m, err := s.documentMail(r.DocumentType, r.DocumentID, r.Level)
	if err != nil {
		return mail.LogEntry{}, err
	}
	to, err := mail.ParseAddressList(r.To)
	if err != nil {
		return mail.LogEntry{}, err
	}
	if len(to) == 0 {
		return mail.LogEntry{}, errors.New("Gib die E-Mail-Adresse des Empfängers ein.")
	}
	cc, err := mail.ParseAddressList(r.Cc)
	if err != nil {
		return mail.LogEntry{}, err
	}
	subject, body, err := m.render(r.Subject, r.Body)
	if err != nil {
		return mail.LogEntry{}, err
	}
	if r.SaveRecipient {
		if err = s.saveCustomerEmail(m.customerID, to[0]); err != nil {
			return mail.LogEntry{}, err
		}
	}
	entry, err := s.mail.Send(ctx, mail.Outgoing{DocumentType: r.DocumentType, DocumentID: r.DocumentID,
		Message: mail.Message{To: to, Cc: cc, Subject: subject, Text: body, Attachments: m.files()}})
	if err != nil || m.reminder == nil {
		return entry, err
	}
	m.reminder.Subject, m.reminder.Body = subject, body
	reminderMu.Lock()
	_, err = s.invoices.recordReminder(*m.reminder, store.ReminderChannelEmail, s.reminderDay())
	reminderMu.Unlock()
	if err != nil {
		return entry, fmt.Errorf("Die E-Mail wurde versendet, aber nicht in der Mahnhistorie eingetragen: %w", err)
	}
	return entry, nil
}
