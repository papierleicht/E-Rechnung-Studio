package store

import (
	"path/filepath"
	"testing"
)

func TestInstallmentPlanDueDatesAndPaymentProgress(t *testing.T) {
	s, id := paymentStore(t)
	if _, e := s.db.Exec("UPDATE invoices SET data=CAST(json_set(data,'$.dueDate','2000-01-01') AS BLOB) WHERE id=?", id); e != nil {
		t.Fatal(e)
	}
	p, e := s.PreviewInstallments(id, "300", "2099-01-31")
	if e != nil {
		t.Fatal(e)
	}
	if len(p.Rates) != 4 || p.Rates[1].Date != "2099-02-28" || p.Rates[2].Date != "2099-03-31" || p.Rates[3].Amount != "100.00" {
		t.Fatal(p)
	}
	p.Note = "Monatlich vereinbart"
	if e = s.SaveInstallmentPlan(p); e != nil {
		t.Fatal(e)
	}
	o, _ := s.PaymentOverview()
	b := o.Invoices[0]
	if !b.Installments || b.State == "überfällig" || b.NextDate != "2099-01-31" || b.NextAmount != "300.00" {
		t.Fatal(b)
	}
	payment, e := s.AddPayment(Payment{InvoiceID: id, Direction: "outgoing", Amount: "400", Currency: "EUR", Date: "2026-10-15", Source: "manual", Reason: "Zahlung"})
	if e != nil {
		t.Fatal(e)
	}
	o, _ = s.PaymentOverview()
	b = o.Invoices[0]
	if b.NextDate != "2099-02-28" || b.NextAmount != "200.00" || b.Remaining != "600.00" {
		t.Fatal(b)
	}
	data, e := s.ExportBackup()
	if e != nil {
		t.Fatal(e)
	}
	other, e := Open(filepath.Join(t.TempDir(), "restore.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer other.Close()
	if e = other.RestoreBackup(data); e != nil {
		t.Fatal(e)
	}
	o, _ = other.PaymentOverview()
	if o.Invoices[0].NextAmount != "200.00" {
		t.Fatal(o)
	}
	// Editing a plan covers only the remaining balance, with prior payments credited.
	next, e := s.PreviewInstallments(id, "200", "2099-04-01")
	if e != nil {
		t.Fatal(e)
	}
	if len(next.Rates) != 3 {
		t.Fatal(next)
	}
	if e = s.SaveInstallmentPlan(next); e != nil {
		t.Fatal(e)
	}
	if e = s.RevokePayment(payment.ID, "Falsche Zahlung"); e != nil {
		t.Fatal(e)
	}
	o, _ = s.PaymentOverview()
	if !o.Invoices[0].PlanNeedsReview {
		t.Fatal("revoked baseline invisible", o)
	}
	next.Active = false
	next.Note = "Vereinbarung beendet"
	if e = s.SaveInstallmentPlan(next); e != nil {
		t.Fatal(e)
	}
	o, _ = s.PaymentOverview()
	if o.Invoices[0].Installments {
		t.Fatal(o)
	}
}
func TestInvalidAndOverdueInstallmentPlans(t *testing.T) {
	s, id := paymentStore(t)
	p, e := s.PreviewInstallments(id, "250", "2000-01-01")
	if e != nil {
		t.Fatal(e)
	}
	p.Rates[0].Amount = "1"
	if e = s.SaveInstallmentPlan(p); e == nil {
		t.Fatal("wrong total accepted")
	}
	p, _ = s.PreviewInstallments(id, "250", "2000-01-01")
	if e = s.SaveInstallmentPlan(p); e != nil {
		t.Fatal(e)
	}
	o, _ := s.PaymentOverview()
	if o.Invoices[0].State != "überfällig" || o.Invoices[0].OverdueAmount != "1000.00" {
		t.Fatal(o)
	}
	if _, e = s.PreviewInstallments(id, "0.01", "2099-01-01"); e == nil {
		t.Fatal("unbounded plan")
	}
	if _, e = s.PreviewInstallments(id, "250", "2026-02-30"); e == nil {
		t.Fatal("invalid date")
	}
}
