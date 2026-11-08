package app

import (
	"testing"

	"papierleicht.de/e-rechnung-studio/internal/store"
)

// pendingInput has one open customer invoice of 1,000 € net (1,190 € gross),
// one open supplier invoice of 500 € net (595 € gross) and one open supplier
// document without readable tax data.
func pendingInput(basis string) financeInput {
	in := baseInput()
	in.Settings.TaxBasis = basis
	in.Settings.ReservePercent = "30"
	in.Issued = []issuedTax{invoice19("a", "RE-1", "2026-10-05")}
	in.ReceivedTax = map[string]receivedTax{"r1": {Rates: []taxRate{{Category: "S", Percent: d("19"), Basis: d("500"), Amount: d("95")}}}}
	in.Overview = store.PaymentOverview{Invoices: []store.PaymentBalance{
		{ID: "a", Direction: "outgoing", Currency: "EUR", Remaining: "1190.00"},
		{ID: "r1", Direction: "incoming", Currency: "EUR", Remaining: "595.00"},
		{ID: "r2", Direction: "incoming", Currency: "EUR", Remaining: "1000.00"},
	}}
	return in
}

func TestPendingTaxOnOpenInvoices(t *testing.T) {
	for _, c := range []struct {
		name   string
		in     func() financeInput
		amount string
	}{
		// Soll: the Umsatzsteuer is already in the open VAT; the reserve
		// grows by 30 % of 1,000 − 500 € net. r2 has no tax data and stays out.
		{"soll", func() financeInput { return pendingInput("soll") }, "150.00"},
		// Ist: the 190 € Umsatzsteuer arises only with the payment.
		{"ist", func() financeInput { return pendingInput("ist") }, "340.00"},
		{"small business", func() financeInput {
			in := pendingInput("ist")
			in.SmallBusiness = true
			return in
		}, "150.00"},
		{"no reserve rate", func() financeInput {
			in := pendingInput("soll")
			in.Settings.ReservePercent = ""
			return in
		}, "0.00"},
		// An Anzahlungsrechnung owes its tax with the payment even under Soll.
		{"advance under soll", func() financeInput {
			in := pendingInput("soll")
			in.Issued[0].Advance = true
			return in
		}, "340.00"},
	} {
		t.Run(c.name, func(t *testing.T) {
			r := buildFinanceReport(c.in())
			if r.PendingTax.Amount != c.amount {
				t.Fatalf("pending tax %s, want %s: %+v", r.PendingTax.Amount, c.amount, r.PendingTax.Entries)
			}
			sum := d("0")
			for _, e := range r.PendingTax.Entries {
				sum = sum.Add(d(e.Amount))
			}
			if !sum.Equal(d(r.PendingTax.Amount)) {
				t.Fatalf("entries add up to %s", sum)
			}
			// The pot for "frei verfügbar" stays as it was.
			if r.Available.Available && r.Available.Amount != buildFinanceReport(c.in()).Available.Amount {
				t.Fatal("available changed")
			}
		})
	}
}

// More open supplier than customer invoices never release reserve.
func TestPendingTaxNeverNegative(t *testing.T) {
	in := pendingInput("soll")
	in.Overview.Invoices[0].Remaining = "119.00"
	if r := buildFinanceReport(in); r.PendingTax.Amount != "0.00" || len(r.PendingTax.Entries) != 0 {
		t.Fatalf("pending tax %+v", r.PendingTax)
	}
}
