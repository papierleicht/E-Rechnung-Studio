package app

import (
	"path/filepath"
	"strings"
	"testing"

	"papierleicht.de/e-rechnung-studio/internal/model"
	"papierleicht.de/e-rechnung-studio/internal/store"
)

func TestRemovingAnAccountKeepsAssignedBookingsInPlace(t *testing.T) {
	db, e := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	s := &InvoiceService{store: db}
	d := model.NewDraft()
	d, e = db.SaveInvoice(d, "1000.00")
	if e != nil {
		t.Fatal(e)
	}
	d, e = db.Issue(d.ID, d, "1000.00", func(model.InvoiceDraft) ([]byte, []byte, error) { return []byte("xml"), []byte("pdf"), nil })
	if e != nil {
		t.Fatal(e)
	}
	header := "Datum;Betrag;Währung;Partner;IBAN;Verwendungszweck;Referenz\n"
	first := header + "2026-10-15;1000;EUR;Kunde;DE123;Zahlung;a-1\n2026-10-01;-20;EUR;Lieferant;DE456;Gebühr;a-2\n"
	second := header + "2026-10-20;5;EUR;Kunde;DE123;Anderes Konto;b-1\n"
	for account, text := range map[string]string{"DE111": first, "DE222": second} {
		if _, e = s.ImportPaymentCSV(account, text); e != nil {
			t.Fatal(e)
		}
	}
	rows, _ := s.PaymentBookings()
	var assigned string
	for _, b := range rows {
		if b.AccountID == "DE111" && b.Data.Amount == 100000 {
			assigned = b.ID
		}
	}
	saved, e := s.RecordPayment(store.Payment{InvoiceID: d.ID, Direction: "outgoing", Amount: "1000", Source: "csv", BookingID: assigned, Reason: "Zahlung"})
	if e != nil {
		t.Fatal(e)
	}

	_, e = s.RemoveBankAccount("DE111")
	if e == nil || !strings.Contains(e.Error(), "1 Zuordnung zu Rechnungen") {
		t.Fatalf("assigned account was removed or refused without reason: %v", e)
	}
	if rows, _ = s.PaymentBookings(); len(rows) != 3 {
		t.Fatalf("refusal changed the bookings: %d", len(rows))
	}

	if e = s.RevokePayment(saved.ID, "Falsche Rechnung"); e != nil {
		t.Fatal(e)
	}
	removed, e := s.RemoveBankAccount("DE111")
	if e != nil {
		t.Fatal(e)
	}
	if removed.Bookings != 2 || removed.From != "2026-10-01" || removed.To != "2026-10-15" {
		t.Fatalf("unexpected report: %+v", removed)
	}
	rows, _ = s.PaymentBookings()
	if len(rows) != 1 || rows[0].AccountID != "DE222" {
		t.Fatalf("wrong bookings left: %+v", rows)
	}
	// The revoked payment keeps its own copy of the booking.
	if o, _ := s.Payments(); len(o.Payments) != 1 || len(o.Payments[0].Booking) == 0 {
		t.Fatalf("payment history lost its booking: %+v", o.Payments)
	}
	log, e := db.BankAccountRemovals()
	if e != nil || len(log) != 1 || log[0].AccountID != "DE111" || log[0].Bookings != 2 || log[0].Source != "csv" || log[0].At == "" {
		t.Fatalf("removal not logged: %+v %v", log, e)
	}
	if _, e = s.RemoveBankAccount("DE111"); e == nil {
		t.Fatal("removing an unknown account succeeded")
	}
	// The file can be imported again.
	if n, e := s.ImportPaymentCSV("DE111", first); e != nil || n != 2 {
		t.Fatalf("re-import: %d %v", n, e)
	}
}

func TestCSVAccountsCanBeNamed(t *testing.T) {
	db, e := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	s := &InvoiceService{store: db}
	text := "Datum;Betrag;Währung;Partner;IBAN;Verwendungszweck;Referenz\n2026-10-15;12;EUR;Bank;DE1;Zinsen;z-1\n"
	if _, e = s.ImportPaymentCSV("DE999", text); e != nil {
		t.Fatal(e)
	}
	if e = s.SetCSVAccountLabel("DE999", "  Tagesgeld   Rücklage "); e != nil {
		t.Fatal(e)
	}
	if labels, _ := s.CSVAccountLabels(); labels["DE999"] != "Tagesgeld Rücklage" {
		t.Fatalf("name not kept: %+v", labels)
	}
	if e = s.SetCSVAccountLabel("DE000", "Unbekannt"); e == nil {
		t.Fatal("an account without CSV bookings got a name")
	}
	if e = s.SetCSVAccountLabel("DE999", strings.Repeat("x", 61)); e == nil {
		t.Fatal("a name over 60 characters was accepted")
	}
	removed, e := s.RemoveBankAccount("DE999")
	if e != nil || removed.Name != "Tagesgeld Rücklage" || removed.IBAN != "DE999" {
		t.Fatalf("removal lost the name: %+v %v", removed, e)
	}
	if labels, _ := s.CSVAccountLabels(); len(labels) != 0 {
		t.Fatalf("name survived the removal: %+v", labels)
	}
}
