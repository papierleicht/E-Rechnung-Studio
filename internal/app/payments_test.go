package app

import (
	"papierleicht.de/e-rechnung-studio/internal/model"
	"papierleicht.de/e-rechnung-studio/internal/store"
	"path/filepath"
	"testing"
)

func TestCSVPreviewImportIdempotencyAndBackup(t *testing.T) {
	db, e := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	s := &InvoiceService{store: db}
	text := "Datum;Betrag;Währung;Partner;IBAN;Verwendungszweck;Referenz\n2026-10-15;400,00;EUR;Kunde;DE123;RE-123;bank-1\n2026-10-15;-20.00;EUR;Lieferant;DE456;RE-456;bank-2\n"
	rows, e := s.PreviewPaymentCSV("DE999", text)
	if e != nil || len(rows) != 2 || rows[0].Data.Amount != 40000 {
		t.Fatal(rows, e)
	}
	stored, _ := s.PaymentBookings()
	if len(stored) != 0 {
		t.Fatal("preview imported data")
	}
	for i := 0; i < 2; i++ {
		n, e := s.ImportPaymentCSV("DE999", text)
		want := 2
		if i == 1 {
			want = 0
		}
		if e != nil || n != want {
			t.Fatal(n, e)
		}
	}
	rows, e = s.PreviewPaymentCSV("DE999", text)
	if e != nil || !rows[0].Duplicate || !rows[1].Duplicate {
		t.Fatal(rows, e)
	}
	backup, e := s.ExportBackup()
	if e != nil {
		t.Fatal(e)
	}
	if e = db.RestoreBackup(backup); e != nil {
		t.Fatal(e)
	}
	stored, e = s.PaymentBookings()
	if e != nil || len(stored) != 2 {
		t.Fatal(stored, e)
	}
	if _, e = s.PreviewPaymentCSV("DE999", "Datum;Betrag;Währung;Partner;IBAN;Verwendungszweck;Referenz\n2026-02-30;1;EUR;;;;\n"); e == nil {
		t.Fatal("invalid date accepted")
	}
}

func TestBankPaymentSplitAndTrustedProvenance(t *testing.T) {
	db, e := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	s := &InvoiceService{store: db}
	makeInvoice := func() string {
		t.Helper()
		d := model.NewDraft()
		d, e = db.SaveInvoice(d, "1000.00")
		if e != nil {
			t.Fatal(e)
		}
		d, e = db.Issue(d.ID, d, "1000.00", func(model.InvoiceDraft) ([]byte, []byte, error) { return []byte("xml"), []byte("pdf"), nil })
		if e != nil {
			t.Fatal(e)
		}
		return d.ID
	}
	first, second := makeInvoice(), makeInvoice()
	text := "Datum;Betrag;Währung;Partner;IBAN;Verwendungszweck;Referenz\n2026-10-15;1000;EUR;Kunde;DE123;Sammelzahlung;bank-1\n"
	if _, e = s.ImportPaymentCSV("DE999", text); e != nil {
		t.Fatal(e)
	}
	rows, _ := s.PaymentBookings()
	p := store.Payment{InvoiceID: first, Direction: "outgoing", Amount: "400", Currency: "USD", Date: "1900-01-01", Source: "csv", BookingID: rows[0].ID, Reason: "Sammelzahlung"}
	saved, e := s.RecordPayment(p)
	if e != nil {
		t.Fatal(e)
	}
	if saved.Currency != "EUR" || saved.Date != "2026-10-15" || saved.Amount != "400.00" || len(saved.Booking) == 0 {
		t.Fatal(saved)
	}
	p.InvoiceID = second
	p.Amount = "400.005"
	if _, e = s.RecordPayment(p); e == nil {
		t.Fatal("sub-cent bank allocation accepted")
	}
	p.Amount = "700"
	if _, e = s.RecordPayment(p); e == nil {
		t.Fatal("overallocated booking")
	}
	p.Amount = "600"
	if _, e = s.RecordPayment(p); e != nil {
		t.Fatal(e)
	}
	p.InvoiceID = first
	p.Amount = "1"
	if _, e = s.RecordPayment(p); e == nil {
		t.Fatal("duplicate payment accepted")
	}
	if e = s.RevokePayment(saved.ID, "Falsche Rechnung"); e != nil {
		t.Fatal(e)
	}
	p.Amount = "400"
	if _, e = s.RecordPayment(p); e != nil {
		t.Fatal(e)
	}
	n, e := s.ImportPaymentCSV("DE999", text)
	if e != nil || n != 0 {
		t.Fatal(n, e)
	}
	o, _ := s.Payments()
	if len(o.Payments) != 3 {
		t.Fatal(o)
	}
}
