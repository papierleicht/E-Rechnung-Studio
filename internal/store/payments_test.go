package store

import (
	"papierleicht.de/e-rechnung-studio/internal/model"
	"path/filepath"
	"testing"
)

func paymentStore(t *testing.T) (*Store, string) {
	t.Helper()
	s, e := Open(filepath.Join(t.TempDir(), "test.db"))
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { s.Close() })
	d := model.NewDraft()
	d.Currency = "EUR"
	d.DueDate = "2099-01-01"
	d, e = s.SaveInvoice(d, "1000.00")
	if e != nil {
		t.Fatal(e)
	}
	d, e = s.Issue(d.ID, d, "1000.00", func(model.InvoiceDraft) ([]byte, []byte, error) { return []byte("xml"), []byte("pdf"), nil })
	if e != nil {
		t.Fatal(e)
	}
	return s, d.ID
}
func TestPaymentsPartialOverpaymentRevokeAndBackup(t *testing.T) {
	s, id := paymentStore(t)
	add := func(amount, booking string) Payment {
		t.Helper()
		p, e := s.AddPayment(Payment{InvoiceID: id, Direction: "outgoing", Amount: amount, Currency: "EUR", Date: "2026-10-15", Source: "bank", BookingID: booking, Reason: "Kontoauszug"})
		if e != nil {
			t.Fatal(e)
		}
		return p
	}
	check := func(paid, remaining, over, state string) {
		t.Helper()
		o, e := s.PaymentOverview()
		if e != nil {
			t.Fatal(e)
		}
		b := o.Invoices[0]
		if b.Paid != paid || b.Remaining != remaining || b.Overpaid != over || b.State != state {
			t.Fatalf("unexpected balance %+v", b)
		}
	}
	p := add("400", "bank-1")
	check("400.00", "600.00", "0.00", "teilbezahlt")
	if _, e := s.AddPayment(Payment{InvoiceID: id, Direction: "outgoing", Amount: "400", Currency: "EUR", Date: "2026-10-15", Source: "bank", BookingID: "bank-1", Reason: "duplicate"}); e == nil {
		t.Fatal("duplicate accepted")
	}
	add("700", "bank-2")
	check("1100.00", "0.00", "100.00", "bezahlt")
	if e := s.RevokePayment(p.ID, "Falsche Rechnung"); e != nil {
		t.Fatal(e)
	}
	check("700.00", "300.00", "0.00", "teilbezahlt")
	add("-100", "refund")
	check("600.00", "400.00", "0.00", "teilbezahlt")
	backup, e := s.ExportBackup()
	if e != nil {
		t.Fatal(e)
	}
	other, e := Open(filepath.Join(t.TempDir(), "restored.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer other.Close()
	if e = other.RestoreBackup(backup); e != nil {
		t.Fatal(e)
	}
	o, e := other.PaymentOverview()
	if e != nil || o.Invoices[0].Remaining != "400.00" || len(o.Payments) != 3 || o.Payments[0].RevocationReason != "Falsche Rechnung" {
		t.Fatalf("restore: %+v %v", o, e)
	}
	var events []paymentEvent
	if e = other.readSetting("payment-events-v1", &events); e != nil || len(events) != 4 {
		t.Fatalf("restored audit: %+v %v", events, e)
	}
}
func TestPaidShortcutUsesPayments(t *testing.T) {
	s, id := paymentStore(t)
	if e := s.SetPaid(id, true); e != nil {
		t.Fatal(e)
	}
	if e := s.SetPaid(id, true); e != nil {
		t.Fatal(e)
	}
	o, _ := s.PaymentOverview()
	if len(o.Payments) != 1 || o.Invoices[0].Remaining != "0.00" {
		t.Fatal(o)
	}
	if e := s.SetPaid(id, false); e != nil {
		t.Fatal(e)
	}
	o, _ = s.PaymentOverview()
	if o.Invoices[0].Remaining != "1000.00" || o.Payments[0].RevokedAt == "" {
		t.Fatal(o)
	}
}
func TestIncomingAndInvalidPayments(t *testing.T) {
	s, _ := paymentStore(t)
	_, e := s.SaveReceived(model.ReceivedInvoice{ID: "received", Inspection: model.ImportInspection{Currency: "EUR", Totals: model.Totals{Gross: "1000.00"}}}, []byte("original"))
	if e != nil {
		t.Fatal(e)
	}
	if e = s.UpdateReceivedInspection("received", model.ImportInspection{Currency: "EUR", IBAN: "DE123", DueDate: "2099-01-01", Totals: model.Totals{Gross: "1000.00"}}); e != nil {
		t.Fatal(e)
	}
	if _, e = s.ExportBackup(); e != nil {
		t.Fatal("rechecked receipt backup", e)
	}
	p := Payment{InvoiceID: "received", Direction: "incoming", Currency: "EUR", Amount: "400", Date: "2026-10-15", Source: "manual", Reason: "Teilzahlung"}
	if _, e = s.AddPayment(p); e != nil {
		t.Fatal(e)
	}
	o, _ := s.PaymentOverview()
	for _, b := range o.Invoices {
		if b.ID == "received" && b.Remaining != "600.00" {
			t.Fatal(b)
		}
	}
	for _, value := range []string{"0", "1.001", "garbage"} {
		p.Amount = value
		if _, e = s.AddPayment(p); e == nil {
			t.Fatal("accepted", value)
		}
	}
}

func TestLegacyPaidMigrationAndOverdue(t *testing.T) {
	s, id := paymentStore(t)
	if _, e := s.db.Exec("DELETE FROM settings WHERE key=?", paymentKey); e != nil {
		t.Fatal(e)
	}
	if _, e := s.db.Exec("UPDATE invoices SET status='paid',data=CAST(json_set(data,'$.status','paid','$.paidAt','2026-10-10T12:00:00Z') AS BLOB) WHERE id=?", id); e != nil {
		t.Fatal(e)
	}
	if e := s.migratePayments(); e != nil {
		t.Fatal(e)
	}
	o, _ := s.PaymentOverview()
	if len(o.Payments) != 1 || o.Payments[0].Date != "2026-10-10" || o.Invoices[0].Remaining != "0.00" {
		t.Fatal(o)
	}
	if e := s.migratePayments(); e != nil {
		t.Fatal(e)
	}
	o, _ = s.PaymentOverview()
	if len(o.Payments) != 1 {
		t.Fatal("migration duplicated payment")
	}
	if e := s.SetPaid(id, false); e != nil {
		t.Fatal(e)
	}
	if _, e := s.db.Exec("UPDATE invoices SET data=CAST(json_set(data,'$.dueDate','2000-01-01') AS BLOB) WHERE id=?", id); e != nil {
		t.Fatal(e)
	}
	o, _ = s.PaymentOverview()
	if o.Invoices[0].State != "überfällig" {
		t.Fatal(o)
	}
}
func TestPaymentCurrencyPrecision(t *testing.T) {
	s, id := paymentStore(t)
	if _, e := s.db.Exec("UPDATE invoices SET data=CAST(json_set(data,'$.currency','JPY') AS BLOB) WHERE id=?", id); e != nil {
		t.Fatal(e)
	}
	p := Payment{InvoiceID: id, Direction: "outgoing", Amount: "400", Currency: "JPY", Date: "2026-10-15", Source: "manual", Reason: "Barzahlung"}
	if _, e := s.AddPayment(p); e != nil {
		t.Fatal(e)
	}
	o, _ := s.PaymentOverview()
	if o.Invoices[0].Remaining != "600" {
		t.Fatal(o)
	}
	p.Amount = "0.5"
	if _, e := s.AddPayment(p); e == nil {
		t.Fatal("fractional yen accepted")
	}
	p.Amount = "100"
	p.Currency = "EUR"
	if _, e := s.AddPayment(p); e == nil {
		t.Fatal("currency mismatch accepted")
	}
}
func TestPaidReceivedCannotLoseAuditTrail(t *testing.T) {
	s, _ := paymentStore(t)
	_, e := s.SaveReceived(model.ReceivedInvoice{ID: "received", Filename: "invoice.xml", Inspection: model.ImportInspection{Currency: "EUR", Totals: model.Totals{Gross: "1000"}}}, []byte("original"))
	if e != nil {
		t.Fatal(e)
	}
	p, e := s.AddPayment(Payment{InvoiceID: "received", Direction: "incoming", Amount: "400", Currency: "EUR", Date: "2026-10-15", Source: "manual", Reason: "Teilzahlung"})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.ChangeReceivedStatus("received", "misimport", "Fehlimport"); e == nil {
		t.Fatal("paid receipt hidden")
	}
	if e = s.RevokePayment(p.ID, "Korrektur"); e != nil {
		t.Fatal(e)
	}
	if _, e = s.ChangeReceivedStatus("received", "misimport", "Fehlimport"); e != nil {
		t.Fatal(e)
	}
	if e = s.DeleteReceivedMisimport("received", "invoice.xml", true); e == nil {
		t.Fatal("payment history lost receipt")
	}
}

func TestPaymentCorrectionAppendsImmutableFollowup(t *testing.T) {
	s, id := paymentStore(t)
	p, e := s.AddPayment(Payment{InvoiceID: id, Direction: "outgoing", Amount: "400", Currency: "EUR", Date: "2026-10-15", Source: "manual", Reason: "Teilzahlung"})
	if e != nil {
		t.Fatal(e)
	}
	if e = s.RevokePayment(p.ID, "Falscher Beleg"); e != nil {
		t.Fatal(e)
	}
	events := []paymentEvent{}
	if e = s.readSetting("payment-events-v1", &events); e != nil {
		t.Fatal(e)
	}
	if len(events) != 2 || events[0].Action != "recorded" || events[0].Payment.RevokedAt != "" || events[1].Action != "revoked" || events[1].Reason != "Falscher Beleg" {
		t.Fatal(events)
	}
}
