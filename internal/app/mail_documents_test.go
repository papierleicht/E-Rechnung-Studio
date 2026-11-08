package app

import (
	"context"
	"strings"
	"testing"

	"papierleicht.de/e-rechnung-studio/internal/mail"
	"papierleicht.de/e-rechnung-studio/internal/mail/mailtest"
	"papierleicht.de/e-rechnung-studio/internal/store"
)

func TestSendReminderByMailRecordsHistory(t *testing.T) {
	invoices, service, server := newMailTestServices(t, mailtest.Config{})
	service.today = func() string { return "2026-10-26" }
	d := testDraft()
	d.Number = "RE-MAHN-MAIL"
	issued, err := invoices.IssueInvoice(d) // due 2026-09-23
	if err != nil {
		t.Fatal(err)
	}
	id := issued.Draft.ID
	prepared, err := service.PrepareDocumentMail(mail.DocumentReminder, id, 0)
	if err != nil {
		t.Fatal(err)
	}
	if prepared.Level != 1 || prepared.To != "kunde@example.eu" || len(prepared.Attachments) < 2 ||
		!strings.HasSuffix(prepared.Attachments[0].Filename, ".pdf") || !strings.Contains(prepared.Subject, "RE-MAHN-MAIL") {
		t.Fatalf("unexpected reminder mail: %+v", prepared)
	}
	if history, _ := invoices.store.Reminders(id); len(history) != 0 {
		t.Fatal("preparing recorded a reminder")
	}
	request := DocumentMailRequest{DocumentType: mail.DocumentReminder, DocumentID: id, Level: prepared.Level,
		To: prepared.To, Subject: prepared.Subject, Body: prepared.Body}
	if _, err = service.SendDocumentMail(context.Background(), request); err == nil {
		t.Fatal("unconfirmed reminder sent")
	}
	request.Confirmed = true
	if _, err = service.SendDocumentMail(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if got := server.Messages(); len(got) != 1 {
		t.Fatalf("expected one message, got %d", len(got))
	}
	history, err := invoices.store.Reminders(id)
	if err != nil || len(history) != 1 || history[0].Channel != store.ReminderChannelEmail || history[0].Level != 1 {
		t.Fatalf("reminder history: %+v %v", history, err)
	}
	log, err := service.mail.Log(mail.DocumentReminder, id)
	if err != nil || len(log) != 1 || log[0].Status != "sent" {
		t.Fatalf("send log: %+v %v", log, err)
	}
}

func TestSendQuoteByMailRendersTemplate(t *testing.T) {
	invoices, service, server := newMailTestServices(t, mailtest.Config{})
	draft, err := invoices.SaveQuote(testQuote())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.PrepareDocumentMail(mail.DocumentQuote, draft.Quote.ID, 0); err == nil {
		t.Fatal("draft quote offered for sending")
	}
	sent, err := invoices.MarkQuoteSent(draft.Quote)
	if err != nil {
		t.Fatal(err)
	}
	id := sent.Quote.ID
	prepared, err := service.PrepareDocumentMail(mail.DocumentQuote, id, 0)
	if err != nil {
		t.Fatal(err)
	}
	if prepared.Subject != "Angebot "+sent.Quote.Number+" von Muster GmbH" || !strings.Contains(prepared.Body, "31.12.2099") ||
		prepared.TemplateWarning != "" || prepared.Attachments[0].Filename != "Angebot-"+exportFilename(sent.Quote.Number)+".pdf" {
		t.Fatalf("unexpected quote mail: %+v", prepared)
	}
	if _, err = service.SendDocumentMail(context.Background(), DocumentMailRequest{DocumentType: mail.DocumentQuote,
		DocumentID: id, To: "kunde@example.eu", Subject: "Angebot {Angebotsnummer}", Body: "{Nummer}", Confirmed: true}); err == nil {
		t.Fatal("unknown placeholder sent")
	}
	if _, err = service.SendDocumentMail(context.Background(), DocumentMailRequest{DocumentType: mail.DocumentQuote,
		DocumentID: id, To: "kunde@example.eu", Subject: "Angebot {Angebotsnummer}", Body: prepared.Body, Confirmed: true}); err != nil {
		t.Fatal(err)
	}
	got := server.Messages()
	if len(got) != 1 || !strings.Contains(string(got[0].Data), sent.Quote.Number) {
		t.Fatalf("quote message not delivered: %d", len(got))
	}
	if _, err = service.PrepareDocumentMail("unknown", id, 0); err == nil {
		t.Fatal("unknown document type accepted")
	}
}
