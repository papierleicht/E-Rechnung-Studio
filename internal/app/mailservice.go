package app

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/shopspring/decimal"
	"papierleicht.de/e-rechnung-studio/internal/engine"
	"papierleicht.de/e-rechnung-studio/internal/invoicexml"
	"papierleicht.de/e-rechnung-studio/internal/mail"
	"papierleicht.de/e-rechnung-studio/internal/model"
)

// MailService is the Wails API for e-mail settings and sending invoices. The
// delivery itself is document-agnostic (internal/mail).
type MailService struct {
	mail     *mail.Service
	invoices *InvoiceService
	today    func() string // nil: the current day
}

func NewMailService(dataDir string, invoices *InvoiceService) (*MailService, error) {
	m, err := mail.New(dataDir)
	if err != nil {
		return nil, err
	}
	return &MailService{mail: m, invoices: invoices}, nil
}

//wails:ignore
func (s *MailService) Close() error { return s.mail.Close() }

func (s *MailService) Settings() (mail.Settings, error) { return s.mail.Settings() }
func (s *MailService) Presets() []mail.Preset           { return mail.Presets() }

// Placeholders lists the template fields for a document type, e.g. "invoice".
func (s *MailService) Placeholders(documentType string) []string {
	return append([]string{}, mail.Placeholders[documentType]...)
}

// SaveSettings stores account and templates. A non-empty password is written
// to the operating-system keyring; an empty password keeps the stored one.
// errMailReadOnly stops sending during an audit: a mail that goes out could not
// be noted in the protocol, and a sent reminder would be missing from the history.
var errMailReadOnly = errors.New("Die Nur-Lese-Ansicht ist aktiv. Beende sie, um wieder etwas zu ändern.")

func (s *MailService) readOnly() bool { return s.invoices != nil && s.invoices.ReadOnly() }

func (s *MailService) SaveSettings(v mail.Settings, password string) (mail.Settings, error) {
	if s.readOnly() {
		return mail.Settings{}, errMailReadOnly
	}
	return s.mail.SaveSettings(v, password)
}

func (s *MailService) ForgetPassword() (mail.Settings, error) {
	if s.readOnly() {
		return mail.Settings{}, errMailReadOnly
	}
	if err := s.mail.ForgetPassword(); err != nil {
		return mail.Settings{}, err
	}
	return s.mail.Settings()
}

// SendTestMail sends a test message to the sender address of the given,
// possibly unsaved account. An empty password uses the stored password.
func (s *MailService) SendTestMail(ctx context.Context, account mail.Account, password string) error {
	return s.mail.TestAccount(ctx, account, password)
}

// InvoiceMailDraft is the editable preview of an invoice e-mail.
type InvoiceMailDraft struct {
	InvoiceID       string                `json:"invoiceId"`
	Number          string                `json:"number"`
	CustomerID      string                `json:"customerId"`
	CustomerName    string                `json:"customerName"`
	To              string                `json:"to"`
	CanSaveEmail    bool                  `json:"canSaveEmail"`
	Subject         string                `json:"subject"`
	Body            string                `json:"body"`
	From            string                `json:"from"`
	ReplyTo         string                `json:"replyTo"`
	BCCSelf         bool                  `json:"bccSelf"`
	XRechnung       bool                  `json:"xrechnung"`
	IncludePDF      bool                  `json:"includePdf"`
	Attachments     []mail.AttachmentInfo `json:"attachments"`
	PDFAttachment   mail.AttachmentInfo   `json:"pdfAttachment"`
	Configured      bool                  `json:"configured"`
	HasPassword     bool                  `json:"hasPassword"`
	TemplateWarning string                `json:"templateWarning,omitempty"`
	Log             []mail.LogEntry       `json:"log"`
}

// InvoiceMailRequest is the confirmed message the user wants to send.
type InvoiceMailRequest struct {
	InvoiceID     string `json:"invoiceId"`
	To            string `json:"to"`
	Cc            string `json:"cc"`
	Subject       string `json:"subject"`
	Body          string `json:"body"`
	IncludePDF    bool   `json:"includePdf"`
	SaveRecipient bool   `json:"saveRecipient"`
	Confirmed     bool   `json:"confirmed"`
}

func (s *MailService) issuedInvoice(id string) (model.InvoiceDraft, error) {
	d, err := s.invoices.store.Invoice(id)
	if err != nil {
		return d, fmt.Errorf("Die Rechnung wurde nicht gefunden: %w", err)
	}
	if d.Status == "draft" {
		return d, errors.New("Stelle die Rechnung zuerst verbindlich aus, bevor du sie versendest.")
	}
	return d, nil
}

func (s *MailService) customer(id string) (model.Customer, bool, error) {
	if id == "" {
		return model.Customer{}, false, nil
	}
	customers, err := s.invoices.store.Customers()
	if err != nil {
		return model.Customer{}, false, err
	}
	for _, c := range customers {
		if c.ID == id {
			return c, true, nil
		}
	}
	return model.Customer{}, false, nil
}

// invoiceValues reads amounts and issuer from the immutable issued XML.
func (s *MailService) invoiceValues(d model.InvoiceDraft) (map[string]string, error) {
	artifact, err := s.invoices.store.Artifact(d.ID, "cii-xml")
	if err != nil {
		return nil, fmt.Errorf("Die gespeicherte Rechnungsdatei konnte nicht geladen werden: %w", err)
	}
	inv, err := invoicexml.Parse(artifact)
	if err != nil {
		return nil, fmt.Errorf("Die gespeicherte Rechnungsdatei konnte nicht gelesen werden: %w", err)
	}
	totals := engine.Totals(inv)
	amount := totals.Due
	if amount == "" || amount == "0.00" {
		amount = totals.Gross
	}
	due := "sofort"
	if parts := strings.Split(d.DueDate, "-"); len(parts) == 3 {
		due = parts[2] + "." + parts[1] + "." + parts[0]
	}
	return map[string]string{
		"Rechnungsnummer": d.Number,
		"Betrag":          germanAmount(amount, d.Currency),
		"Fälligkeit":      due,
		"Kunde":           d.Customer.Name,
		"Firma":           inv.Seller.Name,
	}, nil
}

func germanAmount(value, currency string) string {
	amount, err := decimal.NewFromString(value)
	if err != nil {
		return value
	}
	fixed := amount.Abs().StringFixed(2)
	whole, fraction, _ := strings.Cut(fixed, ".")
	var grouped strings.Builder
	for i, digit := range whole {
		if i > 0 && (len(whole)-i)%3 == 0 {
			grouped.WriteByte('.')
		}
		grouped.WriteRune(digit)
	}
	sign := ""
	if amount.IsNegative() {
		sign = "-"
	}
	symbol := strings.ToUpper(strings.TrimSpace(currency))
	if symbol == "" || symbol == "EUR" {
		symbol = "€"
	}
	return sign + grouped.String() + "," + fraction + " " + symbol
}

func (s *MailService) invoiceAttachments(d model.InvoiceDraft, includePDF bool) ([]mail.Attachment, error) {
	prefix := "Rechnung-"
	if d.Kind == "credit-note" {
		prefix = "Gutschrift-"
	}
	name := prefix + exportFilename(d.Number)
	attachments := []mail.Attachment{}
	if strings.HasPrefix(d.Profile, "xrechnung-") {
		xml, err := s.invoices.store.Artifact(d.ID, "cii-xml")
		if err != nil {
			return nil, err
		}
		attachments = append(attachments, mail.Attachment{Filename: name + ".xml", ContentType: "application/xml", Data: xml})
		if !includePDF {
			return attachments, nil
		}
	}
	pdf, err := s.invoices.invoicePDF(d.ID)
	if err != nil {
		return nil, err
	}
	return append(attachments, mail.Attachment{Filename: name + ".pdf", ContentType: "application/pdf", Data: pdf}), nil
}

// PrepareInvoiceMail renders the template for an issued invoice. Nothing is sent.
func (s *MailService) PrepareInvoiceMail(invoiceID string) (InvoiceMailDraft, error) {
	d, err := s.issuedInvoice(invoiceID)
	if err != nil {
		return InvoiceMailDraft{}, err
	}
	settings, err := s.mail.Settings()
	if err != nil {
		return InvoiceMailDraft{}, err
	}
	values, err := s.invoiceValues(d)
	if err != nil {
		return InvoiceMailDraft{}, err
	}
	result := InvoiceMailDraft{InvoiceID: d.ID, Number: d.Number, CustomerID: d.CustomerID, CustomerName: d.Customer.Name,
		From: settings.Account.FromAddress, ReplyTo: settings.Account.ReplyTo, BCCSelf: settings.Account.BCCSelf,
		XRechnung: strings.HasPrefix(d.Profile, "xrechnung-"), IncludePDF: true,
		Configured: settings.Account.Configured(), HasPassword: settings.HasPassword, Attachments: []mail.AttachmentInfo{}}
	template := settings.Templates[mail.DocumentInvoice]
	subject, subjectErr := mail.Render(template.Subject, values)
	body, bodyErr := mail.Render(template.Body, values)
	if err = errors.Join(subjectErr, bodyErr); err != nil {
		result.TemplateWarning = err.Error()
	}
	result.Subject, result.Body = subject, body
	current, found, err := s.customer(d.CustomerID)
	if err != nil {
		return result, err
	}
	result.To = strings.TrimSpace(current.Email)
	if result.To == "" {
		result.To = strings.TrimSpace(d.Customer.Email)
	}
	result.CanSaveEmail = found && strings.TrimSpace(current.Email) == ""
	attachments, err := s.invoiceAttachments(d, true)
	if err != nil {
		return result, err
	}
	for _, attachment := range attachments {
		info := attachment.Info()
		if result.XRechnung && attachment.ContentType == "application/pdf" {
			result.PDFAttachment = info
			continue
		}
		result.Attachments = append(result.Attachments, info)
	}
	result.Log, err = s.mail.Log(mail.DocumentInvoice, d.ID)
	return result, err
}

// SendInvoiceMail sends an issued invoice after explicit confirmation. Failed
// attempts are recorded in the invoice's send log and returned as an error.
func (s *MailService) SendInvoiceMail(ctx context.Context, r InvoiceMailRequest) (mail.LogEntry, error) {
	if s.readOnly() {
		return mail.LogEntry{}, errMailReadOnly
	}
	if !r.Confirmed {
		return mail.LogEntry{}, errors.New("Bestätige den Versand ausdrücklich.")
	}
	d, err := s.issuedInvoice(r.InvoiceID)
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
	values, err := s.invoiceValues(d)
	if err != nil {
		return mail.LogEntry{}, err
	}
	subject, err := mail.Render(r.Subject, values)
	if err != nil {
		return mail.LogEntry{}, fmt.Errorf("Betreff: %w", err)
	}
	body, err := mail.Render(r.Body, values)
	if err != nil {
		return mail.LogEntry{}, fmt.Errorf("Text: %w", err)
	}
	attachments, err := s.invoiceAttachments(d, r.IncludePDF)
	if err != nil {
		return mail.LogEntry{}, err
	}
	if r.SaveRecipient {
		if err = s.saveCustomerEmail(d.CustomerID, to[0]); err != nil {
			return mail.LogEntry{}, err
		}
	}
	return s.mail.Send(ctx, mail.Outgoing{DocumentType: mail.DocumentInvoice, DocumentID: d.ID,
		Message: mail.Message{To: to, Cc: cc, Subject: subject, Text: body, Attachments: attachments}})
}

// saveCustomerEmail completes a missing customer e-mail; it never overwrites.
func (s *MailService) saveCustomerEmail(customerID, email string) error {
	c, found, err := s.customer(customerID)
	if err != nil || !found || strings.TrimSpace(c.Email) != "" {
		return err
	}
	c.Email = email
	if _, err = s.invoices.SaveCustomer(c); err != nil {
		return fmt.Errorf("Die E-Mail-Adresse konnte nicht beim Kunden gespeichert werden: %w", err)
	}
	return nil
}

// QuoteMailLog returns the send attempts for a quote, newest first.
func (s *MailService) QuoteMailLog(quoteID string) ([]mail.LogEntry, error) {
	return s.mail.Log(mail.DocumentQuote, quoteID)
}

// InvoiceMailLog returns the send attempts for an invoice, newest first.
func (s *MailService) InvoiceMailLog(invoiceID string) ([]mail.LogEntry, error) {
	return s.mail.Log(mail.DocumentInvoice, invoiceID)
}
