package app

import (
	"github.com/shopspring/decimal"
)

// pendingTax estimates what the open invoices add to the tax pot once they are
// paid, for "Reicht das Geld?". Under Soll-Versteuerung their Umsatzsteuer is
// already part of the open VAT, except for Anzahlungsrechnungen, whose tax
// follows the payment; under Ist-Versteuerung it arises with the payment. The
// income tax reserve follows the result, which counts payments only, so it
// grows by the user's rate on the net of what is still to come in, less what
// is still to pay. Without a reserve rate that part stays out, as above.
//
// The estimate never lowers the pot: open supplier invoices that exceed the
// open customer invoices leave it at zero instead of releasing reserve.
func pendingTax(in financeInput, issued map[string]issuedTax, receivables, payables []FinanceEntry, result, taxPaid decimal.Decimal) FinanceFigure {
	f := FinanceFigure{
		Label:       "Steuern auf offene Rechnungen",
		Area:        areaLiquidity,
		Available:   true,
		Amount:      "0.00",
		Entries:     []FinanceEntry{},
		Explanation: "Was die offenen Rechnungen dem Steuer-Topf hinzufügen, sobald sie bezahlt sind. Eine Schätzung mit deinem Rücklagensatz, keine Steuerberechnung.",
	}
	ist := in.Settings.TaxBasis == "ist"
	var net, vat decimal.Decimal
	for _, e := range receivables {
		amount, _ := decimal.NewFromString(e.Amount)
		// Without tax data the whole amount counts as net, which errs high.
		n, t, _ := openShares(in, issued, e, amount, ist)
		net, vat = net.Add(n), vat.Add(t)
	}
	for _, e := range payables {
		amount, _ := decimal.NewFromString(e.Amount)
		// Without tax data a payable is left out rather than taken as net.
		if n, t, known := openShares(in, issued, e, amount, ist); known {
			net, vat = net.Sub(n), vat.Sub(t)
		}
	}
	total := decimal.Zero
	if !in.SmallBusiness && vat.IsPositive() {
		vat = vat.Round(2)
		total = total.Add(vat)
		note := "Ist-Versteuerung: entsteht mit dem Zahlungseingang"
		if !ist {
			note = "Anzahlungsrechnungen: entsteht mit dem Zahlungseingang"
		}
		f.Entries = append(f.Entries, FinanceEntry{Kind: "figure", ID: "pendingVAT", Label: "Umsatzsteuer auf offene Rechnungen", Amount: money(vat), Note: note})
	}
	if in.Settings.ReservePercent != "" && net.IsPositive() {
		pct, _ := decimal.NewFromString(in.Settings.ReservePercent)
		now := reserveAfter(result, pct, taxPaid)
		later := reserveAfter(result.Add(net), pct, taxPaid)
		if grow := later.Sub(now).Round(2); grow.IsPositive() {
			total = total.Add(grow)
			f.Entries = append(f.Entries, FinanceEntry{Kind: "figure", ID: "pendingReserve", Label: pct.String() + " % Rücklage Einkommensteuer", Amount: money(grow), Note: "auf " + money(net.Round(2)) + " € netto, offene Ausgaben abgezogen"})
		}
	}
	f.Amount = money(total)
	return f
}

// reserveAfter is the income tax reserve for a result, as the report computes
// it: the rate on a positive result, less income tax already paid.
func reserveAfter(result, pct, paid decimal.Decimal) decimal.Decimal {
	reserve := decimal.Max(result, decimal.Zero).Mul(pct).Div(decimal.NewFromInt(100)).Round(2)
	return reserve.Sub(decimal.Min(decimal.Max(paid, decimal.Zero), reserve))
}

// openShares splits the open amount of one invoice into its net part and the
// Umsatzsteuer that arises only with the payment. Amounts keep their sign
// relative to the entry: a receivable adds, a payable subtracts at the caller.
// known is false when the invoice has no tax data and the split is a guess.
func openShares(in financeInput, issued map[string]issuedTax, e FinanceEntry, open decimal.Decimal, ist bool) (net, vat decimal.Decimal, known bool) {
	if e.Kind == "received" {
		t, ok := in.ReceivedTax[e.ID]
		if !ok {
			return open, decimal.Zero, false
		}
		var basis, tax decimal.Decimal
		for _, r := range t.Rates {
			basis = basis.Add(r.Basis.Abs())
			if !t.owesTax() {
				tax = tax.Add(r.Amount.Abs())
			}
		}
		if gross := basis.Add(tax); gross.IsPositive() {
			return open.Mul(basis).Div(gross), decimal.Zero, true
		}
		return open, decimal.Zero, false
	}
	v, ok := issued[e.ID]
	gross := v.Gross.Abs()
	if !ok || !gross.IsPositive() {
		return open, decimal.Zero, false
	}
	var basis, later decimal.Decimal
	for _, r := range v.Rates {
		basis = basis.Add(r.Basis.Abs())
		if !v.documentDated(r) && !v.margin(r) {
			later = later.Add(r.Amount.Abs())
		}
	}
	net = open.Mul(basis).Div(gross)
	if ist || v.Advance {
		vat = open.Mul(later).Div(gross)
	}
	return net, vat, true
}
