package engine

import (
	"testing"

	"github.com/shopspring/decimal"
	"papierleicht.de/e-rechnung-studio/internal/einvoice"
)

func TestTotalsShowAPaidAmountOnlyWhenOneIsDeducted(t *testing.T) {
	inv := &einvoice.Invoice{
		TaxBasisTotal: decimal.RequireFromString("200"), TaxTotal: decimal.RequireFromString("38"),
		GrandTotal: decimal.RequireFromString("238"), DuePayableAmount: decimal.RequireFromString("238"),
	}
	if got := Totals(inv); got.Prepaid != "" || got.Gross != "238.00" || got.Due != "238.00" {
		t.Fatalf("got %+v", got)
	}
	// A final invoice (BT-113) deducts what was paid before; the amount due shrinks.
	inv.TotalPrepaid = decimal.RequireFromString("100")
	inv.DuePayableAmount = decimal.RequireFromString("138")
	if got := Totals(inv); got.Prepaid != "100.00" || got.Gross != "238.00" || got.Due != "138.00" {
		t.Fatalf("got %+v", got)
	}
}
