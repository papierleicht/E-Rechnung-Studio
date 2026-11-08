package app

import (
	"bytes"
	"context"
	"encoding/base64"
	"io"
	"mime"
	"mime/multipart"
	netmail "net/mail"
	"strings"
	"testing"

	"papierleicht.de/e-rechnung-studio/internal/mail"
	"papierleicht.de/e-rechnung-studio/internal/mail/mailtest"
	"papierleicht.de/e-rechnung-studio/internal/model"
	"papierleicht.de/e-rechnung-studio/internal/profile"
)

const testSMTPPassword = "smtp-geheim-987"

func newMailTestServices(t *testing.T, config mailtest.Config) (*InvoiceService, *MailService, *mailtest.Server) {
	t.Helper()
	dir := t.TempDir()
	invoices, err := NewInvoiceService(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { invoices.Close() })
	config.Username, config.Password = "konto@example.de", testSMTPPassword
	server := mailtest.Start(t, config)
	m, err := mail.NewWithOptions(dir, mail.Options{Vault: &mailtest.Vault{}, RootCAs: server.RootCAs})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { m.Close() })
	invoices.mail = m
	service := &MailService{mail: m, invoices: invoices}
	c := testCompany()
	c.Phone = "+49 30 1234567"
	c.Endpoint, c.EndpointScheme = c.Email, "EM"
	if _, err = invoices.SaveCompany(c); err != nil {
		t.Fatal(err)
	}
	account := mail.Account{Host: server.Host, Port: server.Port, Security: mail.SecuritySTARTTLS, Username: "konto@example.de",
		FromName: "Muster GmbH", FromAddress: "rechnung@example.de", BCCSelf: true}
	if _, err = service.SaveSettings(mail.Settings{Account: account}, testSMTPPassword); err != nil {
		t.Fatal(err)
	}
	return invoices, service, server
}

func issueTestInvoice(t *testing.T, s *InvoiceService, d model.InvoiceDraft) model.InvoiceDraft {
	t.Helper()
	d.Number = ""
	saved, err := s.SaveInvoice(d)
	if err != nil {
		t.Fatal(err)
	}
	issued, err := s.IssueInvoice(saved.Draft)
	if err != nil {
		t.Fatal(err)
	}
	return issued.Draft
}

type mimePart struct {
	filename, contentType string
	data                  []byte
}

func readMIME(t *testing.T, raw []byte) (netmail.Header, []mimePart) {
	t.Helper()
	msg, err := netmail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	_, params, err := mime.ParseMediaType(msg.Header.Get("Content-Type"))
	if err != nil {
		t.Fatal(err)
	}
	reader := multipart.NewReader(msg.Body, params["boundary"])
	var parts []mimePart
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		data, _ := io.ReadAll(part)
		mediaType, _, _ := mime.ParseMediaType(part.Header.Get("Content-Type"))
		parts = append(parts, mimePart{part.FileName(), mediaType, data})
	}
	return msg.Header, parts
}

func TestSendIssuedFacturXInvoiceByMail(t *testing.T) {
	invoices, service, server := newMailTestServices(t, mailtest.Config{})
	customer, err := invoices.SaveCustomer(model.Customer{Name: "Beispielkunde AG", VATID: "FR40303265045", Address: model.Address{Street: "Rue Exemple 2", Postcode: "75001", City: "Paris", Country: "FR"}})
	if err != nil {
		t.Fatal(err)
	}
	d := testDraft()
	d.CustomerID, d.Customer = customer.ID, customer
	if _, err = service.PrepareInvoiceMail(""); err == nil {
		t.Fatal("unknown invoice accepted")
	}
	draftOnly, err := invoices.SaveInvoice(d)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.PrepareInvoiceMail(draftOnly.Draft.ID); err == nil || !strings.Contains(err.Error(), "verbindlich") {
		t.Fatalf("draft must not be sent: %v", err)
	}
	issued := issueTestInvoice(t, invoices, d)

	prepared, err := service.PrepareInvoiceMail(issued.ID)
	if err != nil {
		t.Fatal(err)
	}
	if prepared.To != "" || !prepared.CanSaveEmail || !prepared.Configured || !prepared.HasPassword || prepared.XRechnung {
		t.Fatalf("prepared = %+v", prepared)
	}
	if prepared.Subject != "Rechnung "+issued.Number+" von Muster GmbH" || !strings.Contains(prepared.Body, "über 238,00 €") || !strings.Contains(prepared.Body, "Fällig: 23.09.2026") {
		t.Fatalf("rendered template: %q / %q", prepared.Subject, prepared.Body)
	}
	if len(prepared.Attachments) != 1 || prepared.Attachments[0].Filename != "Rechnung-"+issued.Number+".pdf" || prepared.Attachments[0].Size == 0 {
		t.Fatalf("attachments = %+v", prepared.Attachments)
	}

	request := InvoiceMailRequest{InvoiceID: issued.ID, To: "einkauf@kunde.example", Subject: prepared.Subject, Body: prepared.Body + "\nKunde: {Kunde}", SaveRecipient: true}
	if _, err = service.SendInvoiceMail(context.Background(), request); err == nil || len(server.Messages()) != 0 {
		t.Fatal("sending without explicit confirmation must fail")
	}
	request.Confirmed = true
	entry, err := service.SendInvoiceMail(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	messages := server.Messages()
	if len(messages) != 1 || strings.Join(messages[0].To, ",") != "einkauf@kunde.example,rechnung@example.de" {
		t.Fatalf("messages = %+v", messages)
	}
	header, parts := readMIME(t, messages[0].Data)
	if header.Get("Bcc") != "" || len(parts) != 2 {
		t.Fatalf("header=%v parts=%d", header, len(parts))
	}
	if !strings.Contains(string(parts[0].data), "Kunde: Beispielkunde AG") {
		t.Fatalf("placeholder in edited body not rendered: %q", parts[0].data)
	}
	pdf, err := invoices.invoicePDF(issued.ID)
	if err != nil {
		t.Fatal(err)
	}
	if parts[1].contentType != "application/pdf" || parts[1].filename != "Rechnung-"+issued.Number+".pdf" {
		t.Fatalf("pdf part = %s %s", parts[1].contentType, parts[1].filename)
	}
	if decoded := decodeBase64(t, parts[1].data); !bytes.Equal(decoded, pdf) {
		t.Fatal("attached PDF differs from stored ZUGFeRD PDF")
	}
	if entry.Status != mail.StatusSent || entry.DocumentID != issued.ID {
		t.Fatalf("entry = %+v", entry)
	}

	customers, _ := invoices.store.Customers()
	if customers[0].Email != "einkauf@kunde.example" {
		t.Fatalf("missing customer e-mail was not completed: %+v", customers[0])
	}
	lists, err := invoices.ApplicationLists()
	if err != nil {
		t.Fatal(err)
	}
	for _, invoice := range lists.Invoices {
		if invoice.ID == issued.ID && invoice.SentAt != entry.SentAt {
			t.Fatalf("sent status missing: %+v", invoice)
		}
		if invoice.ID == draftOnly.Draft.ID && invoice.SentAt != "" {
			t.Fatal("draft marked as sent")
		}
	}
	again, err := service.PrepareInvoiceMail(issued.ID)
	if err != nil || again.To != "einkauf@kunde.example" || again.CanSaveEmail || len(again.Log) != 1 {
		t.Fatalf("second preparation = %+v, %v", again, err)
	}
	// An existing customer address is never overwritten.
	if err = service.saveCustomerEmail(customer.ID, "andere@kunde.example"); err != nil {
		t.Fatal(err)
	}
	if customers, _ = invoices.store.Customers(); customers[0].Email != "einkauf@kunde.example" {
		t.Fatal("customer e-mail was overwritten")
	}

	backup, err := invoices.ExportBackup()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(backup, testSMTPPassword) || !strings.Contains(backup, `"mail"`) || !strings.Contains(backup, entry.ID) {
		t.Fatal("backup must contain the send log but never the SMTP password")
	}
	if err = invoices.RestoreBackup(backup); err != nil {
		t.Fatal(err)
	}
	if log, _ := service.InvoiceMailLog(issued.ID); len(log) != 1 || log[0].ID != entry.ID {
		t.Fatalf("restored log = %+v", log)
	}
}

func TestSendXRechnungAttachesXMLAndOptionalPDF(t *testing.T) {
	invoices, service, server := newMailTestServices(t, mailtest.Config{})
	f, _ := profile.Get("xrechnung-ubl")
	d := testDraft()
	d.Profile, d.Syntax = "xrechnung-ubl", f.Syntax
	d.Customer.Endpoint, d.Customer.EndpointScheme = d.Customer.Email, "EM"
	d.BuyerReference = "04011000-12345-03"
	issued := issueTestInvoice(t, invoices, d)
	prepared, err := service.PrepareInvoiceMail(issued.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !prepared.XRechnung || prepared.To != "kunde@example.eu" || prepared.CanSaveEmail || len(prepared.Attachments) != 1 || prepared.Attachments[0].ContentType != "application/xml" || prepared.PDFAttachment.Filename == "" {
		t.Fatalf("prepared = %+v", prepared)
	}
	xml, _ := invoices.store.Artifact(issued.ID, "cii-xml")
	for _, includePDF := range []bool{false, true} {
		_, err = service.SendInvoiceMail(context.Background(), InvoiceMailRequest{InvoiceID: issued.ID, To: prepared.To, Subject: prepared.Subject, Body: prepared.Body, IncludePDF: includePDF, Confirmed: true})
		if err != nil {
			t.Fatal(err)
		}
		messages := server.Messages()
		_, parts := readMIME(t, messages[len(messages)-1].Data)
		want := 2
		if includePDF {
			want = 3
		}
		if len(parts) != want || parts[1].filename != "Rechnung-"+issued.Number+".xml" || !bytes.Equal(decodeBase64(t, parts[1].data), xml) {
			t.Fatalf("includePDF=%v: %d parts", includePDF, len(parts))
		}
		if includePDF && parts[2].contentType != "application/pdf" {
			t.Fatalf("pdf view missing: %+v", parts[2].contentType)
		}
	}
}

func TestFailedInvoiceMailIsLogged(t *testing.T) {
	invoices, service, server := newMailTestServices(t, mailtest.Config{RejectRecipient: "falsch@kunde.example"})
	issued := issueTestInvoice(t, invoices, testDraft())
	entry, err := service.SendInvoiceMail(context.Background(), InvoiceMailRequest{InvoiceID: issued.ID, To: "falsch@kunde.example", Subject: "Rechnung {Rechnungsnummer}", Body: "Text", Confirmed: true})
	if err == nil || entry.Status != mail.StatusFailed || len(server.Messages()) != 0 {
		t.Fatalf("entry=%+v err=%v", entry, err)
	}
	log, err := service.InvoiceMailLog(issued.ID)
	if err != nil || len(log) != 1 || log[0].Status != mail.StatusFailed || log[0].Subject != "Rechnung "+issued.Number || !strings.Contains(log[0].Error, "lehnt einen Empfänger ab") || len(log[0].Attachments) != 1 {
		t.Fatalf("log = %+v, %v", log, err)
	}
	lists, _ := invoices.ApplicationLists()
	if lists.Invoices[0].SentAt != "" {
		t.Fatal("failed delivery marked invoice as sent")
	}
	if _, err = service.SendInvoiceMail(context.Background(), InvoiceMailRequest{InvoiceID: issued.ID, To: "a@kunde.example", Subject: "{Unbekannt}", Confirmed: true}); err == nil || !strings.Contains(err.Error(), "Platzhalter") {
		t.Fatalf("unknown placeholder: %v", err)
	}
}

func TestGermanAmount(t *testing.T) {
	for value, want := range map[string]string{"238.00": "238,00 €", "1234567.5": "1.234.567,50 €", "-12.3": "-12,30 €", "999": "999,00 €"} {
		if got := germanAmount(value, "EUR"); got != want {
			t.Fatalf("%s = %q, want %q", value, got, want)
		}
	}
	if got := germanAmount("1000", "USD"); got != "1.000,00 USD" {
		t.Fatal(got)
	}
}

func decodeBase64(t *testing.T, raw []byte) []byte {
	t.Helper()
	var out bytes.Buffer
	if _, err := io.Copy(&out, base64.NewDecoder(base64.StdEncoding, bytes.NewReader(raw))); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}
