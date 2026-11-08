package app

import (
	"papierleicht.de/e-rechnung-studio/internal/model"
	"papierleicht.de/e-rechnung-studio/internal/store"
	"path/filepath"
	"testing"
)

func reconciliationService(t *testing.T) (*InvoiceService, string) {
	t.Helper()
	db, e := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { db.Close() })
	d := model.NewDraft()
	d.Customer.Name = "Musterkunde"
	d.Number = "RE-2026-001"
	d, e = db.SaveInvoice(d, "1000.00")
	if e != nil {
		t.Fatal(e)
	}
	d, e = db.Issue(d.ID, d, "1000.00", func(model.InvoiceDraft) ([]byte, []byte, error) { return []byte("xml"), []byte("pdf"), nil })
	if e != nil {
		t.Fatal(e)
	}
	return &InvoiceService{store: db}, d.ID
}
func TestAutomaticPartialPaymentsAndRevocation(t *testing.T) {
	s, id := reconciliationService(t)
	o, _ := s.Payments()
	number := o.Invoices[0].Number
	text := "Datum;Betrag;Währung;Partner;IBAN;Verwendungszweck;Referenz\n2026-10-15;400;EUR;Musterkunde;;" + number + ";bank1\n2026-10-15;600;EUR;Musterkunde;;" + number + ";bank2\n"
	if _, e := s.ImportPaymentCSV("DE123", text); e != nil {
		t.Fatal(e)
	}
	r, e := s.ReconcilePayments()
	if e != nil || r.Automatic != 2 || len(r.Pending) != 0 || r.Overview.Invoices[0].State != "bezahlt" {
		t.Fatal(r, e)
	}
	r, e = s.ReconcilePayments()
	if e != nil || r.Automatic != 0 || len(r.Overview.Payments) != 2 {
		t.Fatal(r, e)
	}
	if e = s.SetBookingTreatment(r.Overview.Payments[0].BookingID, "private", ""); e == nil {
		t.Fatal("assigned booking reclassified")
	}
	if e = s.RevokePayment(r.Overview.Payments[0].ID, "Falsche Zuordnung"); e != nil {
		t.Fatal(e)
	}
	r, e = s.ReconcilePayments()
	if e != nil || r.Automatic != 0 || len(r.Pending) != 1 {
		t.Fatal("revocation rebooked", r, e)
	}
	if r.Overview.Invoices[0].ID != id {
		t.Fatal(r)
	}
}
func TestUncertainAndAmbiguousMatchesRequireConfirmation(t *testing.T) {
	s, _ := reconciliationService(t)
	o, _ := s.Payments()
	n := o.Invoices[0].Number
	text := "Datum;Betrag;Währung;Partner;IBAN;Verwendungszweck;Referenz\n2026-10-15;1000;EUR;Unbekannt;;" + n + ";bank1\n"
	s.ImportPaymentCSV("DE123", text)
	r, e := s.ReconcilePayments()
	if e != nil || r.Automatic != 0 || len(r.Pending) != 1 || len(r.Pending[0].Suggestions) == 0 {
		t.Fatal(r, e)
	}
	// A bank reference naming two existing invoices remains a manual decision.
	d := model.NewDraft()
	d.Customer.Name = "Musterkunde"
	d, err := s.store.SaveInvoice(d, "1000.00")
	if err != nil {
		t.Fatal(err)
	}
	d, err = s.store.Issue(d.ID, d, "1000.00", func(model.InvoiceDraft) ([]byte, []byte, error) { return []byte("xml"), []byte("pdf"), nil })
	if err != nil {
		t.Fatal(err)
	}
	mixed := "Datum;Betrag;Währung;Partner;IBAN;Verwendungszweck;Referenz\n2026-10-15;1000;EUR;Musterkunde;;" + n + " " + d.Number + ";bank2\n"
	if _, err = s.ImportPaymentCSV("DE123", mixed); err != nil {
		t.Fatal(err)
	}
	r, err = s.ReconcilePayments()
	if err != nil || r.Automatic != 0 || len(r.Pending) != 2 {
		t.Fatal(r, err)
	}
	if referenceMatches("RE-2026-0012", "RE-2026-001") {
		t.Fatal("number prefix matched")
	}
	if !referenceMatches("Zahlung RE-2026-001 / Danke", "RE-2026-001") {
		t.Fatal("valid reference missed")
	}
	invoices := []store.PaymentBalance{{ID: "a", Direction: "outgoing", Number: "RE-1", Partner: "Musterkunde", Remaining: "1000", Currency: "EUR", State: "offen"}, {ID: "b", Direction: "outgoing", Number: "RE-2", Partner: "Musterkunde", Remaining: "1000", Currency: "EUR", State: "offen"}}
	rows, _ := s.PaymentBookings()
	b := rows[0]
	b.Data.CounterpartyName = "Musterkunde"
	b.Data.Description = "RE-1 RE-2"
	left, _ := bookingUnallocated(b, nil)
	suggestions := suggestionsFor(b, left, invoices)
	if len(suggestions) != 2 || !suggestions[0].Certain || !suggestions[1].Certain {
		t.Fatal(suggestions)
	}
}

func TestOtherMoneyMovementsStayOutOfInvoiceTasksAndRestore(t *testing.T) {
	s, _ := reconciliationService(t)
	text := "Datum;Betrag;Währung;Partner;IBAN;Verwendungszweck;Referenz\n2026-10-15;-1000;EUR;Eigenes Konto;;Rücklage;transfer1\n"
	if _, e := s.ImportPaymentCSV("DE123", text); e != nil {
		t.Fatal(e)
	}
	rows, _ := s.PaymentBookings()
	if e := s.SetBookingTreatment(rows[0].ID, "reserve", "Rücklagenkonto"); e != nil {
		t.Fatal(e)
	}
	r, e := s.ReconcilePayments()
	if e != nil || len(r.Pending) != 0 || len(r.Other) != 1 || len(r.Overview.Payments) != 0 {
		t.Fatal(r, e)
	}
	backup, e := s.ExportBackup()
	if e != nil {
		t.Fatal(e)
	}
	if e = s.RestoreBackup(backup); e != nil {
		t.Fatal(e)
	}
	r, e = s.ReconcilePayments()
	if e != nil || len(r.Other) != 1 {
		t.Fatal(r, e)
	}
	if r.Other[0].Treatment.Kind != "reserve" || r.Other[0].Treatment.Note != "Rücklagenkonto" || r.Other[0].Booking.Data.Amount != -100000 {
		t.Fatal("hidden booking data was not preserved", r.Other)
	}
	if n, err := s.ImportPaymentCSV("DE123", text); err != nil || n != 0 {
		t.Fatal("hidden booking imported again", n, err)
	}
	r, e = s.ReconcilePayments()
	if e != nil || len(r.Other) != 1 || len(r.Pending) != 0 || len(r.Overview.Payments) != 0 {
		t.Fatal("reimport changed hidden state", r, e)
	}
	if e = s.SetBookingTreatment(rows[0].ID, "", "Erneut prüfen"); e != nil {
		t.Fatal(e)
	}
	r, e = s.ReconcilePayments()
	if e != nil || len(r.Pending) != 1 || len(r.Other) != 0 || r.Pending[0].Booking.ID != rows[0].ID {
		t.Fatal(r, e)
	}
}

func TestRestoredHiddenBookingRequiresManualConfirmation(t *testing.T) {
	s, id := reconciliationService(t)
	o, _ := s.Payments()
	text := "Datum;Betrag;Währung;Partner;IBAN;Verwendungszweck;Referenz\n2026-10-15;1000;EUR;Musterkunde;;" + o.Invoices[0].Number + ";hidden1\n"
	if _, err := s.ImportPaymentCSV("DE123", text); err != nil {
		t.Fatal(err)
	}
	rows, err := s.PaymentBookings()
	if err != nil || len(rows) != 1 {
		t.Fatal(rows, err)
	}
	b := rows[0]
	if err = s.SetBookingTreatment(b.ID, "other", "Keine Zuordnung nötig"); err != nil {
		t.Fatal(err)
	}
	r, err := s.ReconcilePayments()
	if err != nil || r.Automatic != 0 || len(r.Pending) != 0 || len(r.Other) != 1 || len(r.Overview.Payments) != 0 {
		t.Fatal("hidden booking automatically allocated", r, err)
	}
	if _, err = s.RecordPayment(store.Payment{InvoiceID: id, Direction: "outgoing", BookingID: b.ID, Source: b.Source, Reason: "Test"}); err == nil {
		t.Fatal("hidden booking manually allocated without restoring")
	}
	if err = s.SetBookingTreatment(b.ID, "", "Wieder anzeigen"); err != nil {
		t.Fatal(err)
	}
	// A restored booking stays visible on subsequent refreshes, even if it is
	// an otherwise certain match. The user decides whether to allocate it.
	for range 2 {
		r, err = s.ReconcilePayments()
		if err != nil || r.Automatic != 0 || len(r.Pending) != 1 || len(r.Other) != 0 || len(r.Overview.Payments) != 0 {
			t.Fatal("restored booking disappeared into automatic allocation", r, err)
		}
	}
	if _, err = s.RecordPayment(store.Payment{InvoiceID: id, Direction: "outgoing", BookingID: b.ID, Source: b.Source, Reason: "Nach Prüfung"}); err != nil {
		t.Fatal(err)
	}
	r, err = s.ReconcilePayments()
	if err != nil || len(r.Pending) != 0 || len(r.Overview.Payments) != 1 || r.Overview.Invoices[0].State != "bezahlt" {
		t.Fatal("restored booking could not be allocated", r, err)
	}
}
