package store

import (
	"testing"

	"papierleicht.de/e-rechnung-studio/internal/model"
)

func incomingBalance(t *testing.T, s *Store, id string) PaymentBalance {
	t.Helper()
	overview, err := s.PaymentOverview()
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range overview.Invoices {
		if b.ID == id && b.Direction == "incoming" {
			return b
		}
	}
	t.Fatalf("no incoming balance for %s", id)
	return PaymentBalance{}
}

// A final invoice deducts earlier payments (BT-113). What stays to pay is the
// amount due (BT-115), so paying that settles the invoice.
func TestIncomingBalanceFollowsTheAmountDue(t *testing.T) {
	s, _ := paymentStore(t)
	totals := model.Totals{Net: "1000.00", Tax: "190.00", Gross: "1190.00", Due: "590.00", Prepaid: "600.00"}
	if _, err := s.SaveReceived(model.ReceivedInvoice{ID: "final", Inspection: model.ImportInspection{Currency: "EUR", Totals: totals}}, []byte("original")); err != nil {
		t.Fatal(err)
	}
	if b := incomingBalance(t, s, "final"); b.Gross != "590.00" || b.Remaining != "590.00" || b.State != "offen" {
		t.Fatalf("before payment: %+v", b)
	}
	payment := Payment{InvoiceID: "final", Direction: "incoming", Currency: "EUR", Amount: "590", Date: "2026-10-15", Source: "manual", Reason: "Restzahlung"}
	if _, err := s.AddPayment(payment); err != nil {
		t.Fatal(err)
	}
	if b := incomingBalance(t, s, "final"); b.Remaining != "0.00" || b.State != "bezahlt" {
		t.Fatalf("after payment: %+v", b)
	}
}

func TestIncomingBalanceFallsBackToGross(t *testing.T) {
	s, _ := paymentStore(t)
	for id, due := range map[string]string{"no-due": "", "bad-due": "garbage"} {
		totals := model.Totals{Gross: "1000.00", Due: due}
		if _, err := s.SaveReceived(model.ReceivedInvoice{ID: id, Inspection: model.ImportInspection{Currency: "EUR", Totals: totals}}, []byte(id)); err != nil {
			t.Fatal(err)
		}
		if b := incomingBalance(t, s, id); b.Gross != "1000.00" || b.Remaining != "1000.00" {
			t.Fatalf("%s: %+v", id, b)
		}
	}
}
