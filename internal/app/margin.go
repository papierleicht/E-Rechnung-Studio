package app

import (
	"strings"

	"github.com/shopspring/decimal"
	"papierleicht.de/e-rechnung-studio/internal/model"
)

// marginRate is the rate of the Differenzbesteuerung: always the standard
// rate, also for art and collectibles (§ 25a Abs. 5 Satz 1 UStG).
var marginRate = decimal.NewFromInt(19)

// marginTreatmentCodes maps the tax treatments of the Differenzbesteuerung to
// the VATEX code of their tax group.
var marginTreatmentCodes = map[string]string{"margin-goods": "VATEX-EU-F", "margin-art": "VATEX-EU-I", "margin-collectibles": "VATEX-EU-J"}

// draftMargin computes the taxable margin of one Differenzbesteuerung tax
// group of an invoice: per item the sale price minus the purchase price
// (Einzeldifferenz, § 25a Abs. 3 UStG), a negative margin counting as zero.
// The Umsatzsteuer is contained in the margin: base = margin / 1.19.
// basis is the group's amount in the invoice, as the document states it; it
// spreads document-level allowances and charges over the items. code is the
// group's VATEX code; an empty one takes every margin item. ok is false while
// an item of the group has no purchase price.
func draftMargin(d model.InvoiceDraft, code string, basis decimal.Decimal) (base, tax decimal.Decimal, ok bool) {
	type item struct{ sale, purchase decimal.Decimal }
	var items []item
	total := decimal.Zero
	for _, l := range d.Lines {
		treatment := d.TaxTreatment
		if treatment == "mixed" {
			treatment = l.TaxTreatment
		}
		c, margin := marginTreatmentCodes[treatment]
		if !margin || (code != "" && c != code) {
			continue
		}
		purchase, valid := parsePurchasePrice(l.PurchasePrice)
		// Drafts keep the decimal comma the user typed, as the engine reads it.
		qty, err1 := decimal.NewFromString(strings.ReplaceAll(strings.TrimSpace(l.Quantity), ",", "."))
		price, err2 := decimal.NewFromString(strings.ReplaceAll(strings.TrimSpace(l.UnitPrice), ",", "."))
		if !valid || err1 != nil || err2 != nil {
			return decimal.Zero, decimal.Zero, false
		}
		sale := qty.Mul(price)
		items = append(items, item{sale, qty.Mul(purchase)})
		total = total.Add(sale)
	}
	if len(items) == 0 {
		return decimal.Zero, decimal.Zero, false
	}
	factor := decimal.NewFromInt(1)
	if !total.IsZero() {
		factor = basis.Div(total)
	}
	margin := decimal.Zero
	for _, it := range items {
		m := it.sale.Mul(factor).Sub(it.purchase)
		if it.sale.IsNegative() {
			// A returned item takes back at most the margin it carried.
			m = decimal.Min(m, decimal.Zero)
		} else {
			m = decimal.Max(m, decimal.Zero)
		}
		margin = margin.Add(m)
	}
	margin = margin.Round(2)
	hundred := decimal.NewFromInt(100)
	base = margin.Mul(hundred).Div(hundred.Add(marginRate)).Round(2)
	return base, margin.Sub(base), true
}

// parsePurchasePrice reads a purchase price; German decimal commas are fine.
// An empty or negative price is not a price.
func parsePurchasePrice(s string) (decimal.Decimal, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return decimal.Zero, false
	}
	if strings.Contains(s, ",") {
		s = strings.ReplaceAll(strings.ReplaceAll(s, ".", ""), ",", ".")
	}
	v, err := decimal.NewFromString(s)
	if err != nil || v.IsNegative() {
		return decimal.Zero, false
	}
	return v, true
}
