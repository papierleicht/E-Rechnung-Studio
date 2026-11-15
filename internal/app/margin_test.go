package app

import (
	"strings"
	"testing"

	"github.com/shopspring/decimal"
	"papierleicht.de/e-rechnung-studio/internal/model"
)

func marginDraft(lines ...model.InvoiceLine) model.InvoiceDraft {
	return model.InvoiceDraft{TaxTreatment: "margin-goods", Lines: lines}
}

func marginLine(id, qty, price, purchase string) model.InvoiceLine {
	return model.InvoiceLine{ID: id, Quantity: qty, UnitPrice: price, PurchasePrice: purchase, TaxCategory: "E"}
}

func TestDraftMarginPerItem(t *testing.T) {
	cases := []struct {
		name      string
		draft     model.InvoiceDraft
		code      string
		basis     string
		base, tax string
		ok        bool
	}{
		// 1.190 sale, 0 purchase: the whole price is margin, 190 of it tax.
		{"whole price", marginDraft(marginLine("a", "1", "1190", "0")), "VATEX-EU-F", "1190", "1000.00", "190.00", true},
		// Einzeldifferenz: the loss on b does not reduce the margin on a.
		{"loss counts as zero", marginDraft(marginLine("a", "1", "1500", "1000"), marginLine("b", "1", "200", "300")), "", "1700", "420.17", "79.83", true},
		{"quantity and comma", marginDraft(marginLine("a", "2", "119,00", "59,50")), "", "238", "100.00", "19.00", true},
		// A document allowance of 10 % lowers the sale price of every item.
		{"allowance", marginDraft(marginLine("a", "1", "1000", "500")), "", "900", "336.13", "63.87", true},
		{"missing price", marginDraft(marginLine("a", "1", "1190", ""), marginLine("b", "1", "100", "50")), "", "1290", "0", "0", false},
		{"other group", marginDraft(marginLine("a", "1", "1190", "100")), "VATEX-EU-I", "1190", "0", "0", false},
		{"returned item", model.InvoiceDraft{TaxTreatment: "margin-goods", Lines: []model.InvoiceLine{marginLine("a", "-1", "1190", "990")}}, "", "-1190", "-168.07", "-31.93", true},
	}
	for _, c := range cases {
		base, tax, ok := draftMargin(c.draft, c.code, decimal.RequireFromString(c.basis))
		if ok != c.ok || (ok && (base.StringFixed(2) != c.base || tax.StringFixed(2) != c.tax)) {
			t.Errorf("%s: base %s tax %s ok %v, want %s %s %v", c.name, base.StringFixed(2), tax.StringFixed(2), ok, c.base, c.tax, c.ok)
		}
	}
}

func TestDraftMarginInMixedInvoice(t *testing.T) {
	d := model.InvoiceDraft{TaxTreatment: "mixed", Lines: []model.InvoiceLine{
		{ID: "used", TaxTreatment: "margin-goods", Quantity: "1", UnitPrice: "595", PurchasePrice: "357"},
		{ID: "service", TaxTreatment: "standard", Quantity: "1", UnitPrice: "100"},
	}}
	base, tax, ok := draftMargin(d, "VATEX-EU-F", decimal.RequireFromString("595"))
	if !ok || base.StringFixed(2) != "200.00" || tax.StringFixed(2) != "38.00" {
		t.Fatalf("base %s tax %s ok %v", base, tax, ok)
	}
}

func TestKnownMarginEntersVATFigures(t *testing.T) {
	in := baseInput()
	used := issuedTax{ID: "m", Number: "RE-7", Partner: "Kunde", Currency: "EUR", TaxDate: "2026-10-05", Gross: d("1190"), Rates: []taxRate{{Category: "E", Percent: d("0"), Basis: d("1190"), Amount: d("0"), ReasonCode: "VATEX-EU-F",
		MarginKnown: true, MarginBase: d("400"), MarginTax: d("76")}}}
	in.Issued = []issuedTax{used}
	r := buildFinanceReport(in)
	if hasIssue(r, "margin_scheme") || r.VATPayable.Incomplete {
		t.Fatalf("known margin still flagged: %+v", r.Issues)
	}
	if r.OutputTax.Amount != "76.00" {
		t.Fatalf("output tax %s", r.OutputTax.Amount)
	}
	lines := map[string]string{}
	for _, l := range r.OutputTaxRates {
		lines[l.Category+"|"+l.Percent] = l.Net + "/" + l.Tax
	}
	if lines["S|19"] != "400.00/76.00" || lines["E|0"] != "714.00/0.00" {
		t.Fatalf("rate lines %v", lines)
	}
}

func TestSetPurchasePricesRejectsBadPrice(t *testing.T) {
	s := &InvoiceService{}
	if err := s.SetPurchasePrices("x", map[string]string{"a": "zwölf"}); err == nil || !strings.Contains(err.Error(), "zwölf") {
		t.Fatalf("err %v", err)
	}
	if err := s.SetPurchasePrices("x", map[string]string{"a": "-5"}); err == nil {
		t.Fatal("negative price accepted")
	}
}
