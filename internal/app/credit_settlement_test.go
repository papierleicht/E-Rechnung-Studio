package app

import (
	"testing"

	"papierleicht.de/e-rechnung-studio/internal/model"
	"papierleicht.de/e-rechnung-studio/internal/store"
)

func issueTestDoc(t *testing.T, s *InvoiceService, number, kind, corrects, gross string) string {
	t.Helper()
	d := model.NewDraft()
	d.Customer.Name, d.Number, d.Kind, d.CorrectsInvoiceID = "Musterkunde", number, kind, corrects
	d, err := s.store.SaveInvoice(d, gross)
	if err != nil {
		t.Fatal(err)
	}
	d, err = s.store.Issue(d.ID, d, gross, func(model.InvoiceDraft) ([]byte, []byte, error) { return []byte("xml"), []byte("pdf"), nil })
	if err != nil {
		t.Fatal(err)
	}
	return d.ID
}

func balanceOf(t *testing.T, s *InvoiceService, id string) store.PaymentBalance {
	t.Helper()
	o, err := s.Payments()
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range o.Invoices {
		if b.ID == id {
			return b
		}
	}
	t.Fatalf("balance %s missing", id)
	return store.PaymentBalance{}
}

func TestCreditNoteSettlesTheOpenInvoice(t *testing.T) {
	s, original := reconciliationService(t)
	credit := issueTestDoc(t, s, "RE-2026-002", "credit-note", original, "1000.00")
	if b := balanceOf(t, s, original); b.Remaining != "0.00" || b.State != "verrechnet" || b.Credited != "1000.00" || b.Credit {
		t.Fatalf("original %+v", b)
	}
	if b := balanceOf(t, s, credit); b.Remaining != "0.00" || b.State != "verrechnet" || !b.Credit {
		t.Fatalf("credit %+v", b)
	}
	if v, err := s.store.InvoiceReminderProposal(original, "2027-06-01"); err != nil || v.Eligible {
		t.Fatalf("reminder for a settled invoice: %+v %v", v, err)
	}
}

func TestPartialCreditLeavesTheRestOpen(t *testing.T) {
	s, original := reconciliationService(t)
	issueTestDoc(t, s, "RE-2026-002", "credit-note", original, "300.00")
	if b := balanceOf(t, s, original); b.Remaining != "700.00" || b.State == "verrechnet" {
		t.Fatalf("original %+v", b)
	}
}

func TestCreditBeyondTheOpenAmountIsOwedToTheCustomer(t *testing.T) {
	s, original := reconciliationService(t)
	if _, err := s.RecordPayment(store.Payment{InvoiceID: original, Direction: "outgoing", Amount: "400.00", Currency: "EUR", Date: "2026-10-01", Source: "manual", Reason: "Teilzahlung"}); err != nil {
		t.Fatal(err)
	}
	credit := issueTestDoc(t, s, "RE-2026-002", "credit-note", original, "1000.00")
	if b := balanceOf(t, s, original); b.Remaining != "0.00" || b.Credited != "600.00" {
		t.Fatalf("original %+v", b)
	}
	if b := balanceOf(t, s, credit); b.Remaining != "400.00" || b.Credited != "600.00" {
		t.Fatalf("credit %+v", b)
	}
}

func TestDashboardCountsACreditNoteNotAsReceivable(t *testing.T) {
	s, original := reconciliationService(t)
	issueTestDoc(t, s, "RE-2026-002", "credit-note", original, "1000.00")
	summary, err := s.DashboardSummary()
	if err != nil {
		t.Fatal(err)
	}
	if summary.Receivable.Count != 0 || summary.Payable.Count != 0 {
		t.Fatalf("receivable %+v payable %+v", summary.Receivable, summary.Payable)
	}
}

func TestBookingPaysARecordedExpense(t *testing.T) {
	s, _ := reconciliationService(t)
	draft := store.Expense{Supplier: "Druckerei Lindner", Date: "2026-10-15", Category: "werbung", Gross: "100,00", TaxRate: "19", PaidWith: "bank", DocumentNumber: "2026-1187"}
	x, err := s.SaveExpense(ExpenseUpload{Expense: draft})
	if err != nil {
		t.Fatal(err)
	}
	text := "Datum;Betrag;Währung;Partner;IBAN;Verwendungszweck;Referenz\n2026-10-27;-100,00;EUR;Druckerei Lindner;;Rechnung 2026-1187;\n2026-10-28;-90,00;EUR;Druckerei Lindner;;Andere Zahlung;\n"
	if _, err = s.ImportPaymentCSV("DE1", text); err != nil {
		t.Fatal(err)
	}
	r, err := s.ReconcilePayments()
	if err != nil {
		t.Fatal(err)
	}
	var exact, other BookingReview
	for _, v := range r.Pending {
		if v.Booking.Data.Amount == -10000 {
			exact = v
		} else {
			other = v
		}
	}
	if len(exact.Expenses) != 1 || exact.Expenses[0].ID != x.ID || len(exact.Expenses[0].Reasons) != 3 {
		t.Fatalf("suggestions %+v", exact.Expenses)
	}
	if _, err = s.AssignBookingToExpense(other.Booking.ID, x.ID); err == nil {
		t.Fatal("a booking with another amount paid the receipt")
	}
	if summary, err := s.DashboardSummary(); err != nil || summary.UnassignedBookings != 2 {
		t.Fatalf("open bookings before the assignment: %+v %v", summary.UnassignedBookings, err)
	}
	paid, err := s.AssignBookingToExpense(exact.Booking.ID, x.ID)
	if err != nil {
		t.Fatal(err)
	}
	if paid.PaidDate != "2026-10-27" || paid.BookingID != exact.Booking.ID || paid.PaidWith != "bank" || paid.Number != x.Number {
		t.Fatalf("paid %+v", paid)
	}
	r, err = s.ReconcilePayments()
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Pending) != 1 || len(r.Pending[0].Expenses) != 0 {
		t.Fatalf("still pending %+v", r.Pending)
	}
	if summary, err := s.DashboardSummary(); err != nil || summary.UnassignedBookings != 1 {
		t.Fatalf("the overview still counts the settled booking: %+v %v", summary.UnassignedBookings, err)
	}
}

func recordRefund(t *testing.T, s *InvoiceService, id, amount string, reduction bool) store.Payment {
	t.Helper()
	p, err := s.RecordPayment(store.Payment{InvoiceID: id, Direction: "outgoing", Amount: amount, Currency: "EUR", Date: "2026-10-02", Source: "manual", Reason: "Erstattung", Reduction: reduction})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestRefundedReductionIsNotReminded(t *testing.T) {
	s, original := reconciliationService(t)
	recordRefund(t, s, original, "1000.00", false)
	refund := recordRefund(t, s, original, "-190.00", true)
	b := balanceOf(t, s, original)
	if b.Remaining != "190.00" || b.AwaitingCredit != "190.00" || b.State != store.StateAwaitingCredit {
		t.Fatalf("balance %+v", b)
	}
	if v, err := s.store.InvoiceReminderProposal(original, "2027-06-01"); err != nil || v.Eligible {
		t.Fatalf("reminder for a refunded reduction: %+v %v", v, err)
	}
	// Withdrawing the decision brings the claim back.
	if err := s.RevokePayment(refund.ID, "Doch keine Minderung"); err != nil {
		t.Fatal(err)
	}
	if b = balanceOf(t, s, original); b.AwaitingCredit != "" || b.Remaining != "0.00" {
		t.Fatalf("after revoking %+v", b)
	}
}

func TestRefundOfAnOverpaymentStaysCollectable(t *testing.T) {
	s, original := reconciliationService(t)
	recordRefund(t, s, original, "1000.00", false)
	recordRefund(t, s, original, "-190.00", false)
	b := balanceOf(t, s, original)
	if b.Remaining != "190.00" || b.AwaitingCredit != "" || b.State == store.StateAwaitingCredit {
		t.Fatalf("balance %+v", b)
	}
	if v, err := s.store.InvoiceReminderProposal(original, "2027-06-01"); err != nil || !v.Eligible {
		t.Fatalf("reminder for an open claim: %+v %v", v, err)
	}
}

func TestPartialReductionRemindsOnlyTheRest(t *testing.T) {
	s, original := reconciliationService(t)
	recordRefund(t, s, original, "600.00", false)
	recordRefund(t, s, original, "-100.00", true)
	v, err := s.store.InvoiceReminderProposal(original, "2027-06-01")
	if err != nil || !v.Eligible || v.Remaining != "400.00" {
		t.Fatalf("proposal %+v %v", v, err)
	}
}

func TestCreditNoteSettlesARefundedReduction(t *testing.T) {
	s, original := reconciliationService(t)
	recordRefund(t, s, original, "1000.00", false)
	recordRefund(t, s, original, "-190.00", true)
	credit := issueTestDoc(t, s, "RE-2026-002", "credit-note", original, "190.00")
	if b := balanceOf(t, s, original); b.State != "verrechnet" || b.AwaitingCredit != "" || b.Remaining != "0.00" {
		t.Fatalf("original %+v", b)
	}
	if b := balanceOf(t, s, credit); b.State != "verrechnet" || b.Remaining != "0.00" {
		t.Fatalf("credit %+v", b)
	}
}

func TestOnlyACustomerRefundCanBeAReduction(t *testing.T) {
	s, original := reconciliationService(t)
	if _, err := s.RecordPayment(store.Payment{InvoiceID: original, Direction: "outgoing", Amount: "10.00", Currency: "EUR", Date: "2026-10-02", Source: "manual", Reason: "Zahlung", Reduction: true}); err == nil {
		t.Fatal("a payment must not count as a reduction")
	}
}
