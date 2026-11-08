package app

import (
	"testing"

	"papierleicht.de/e-rechnung-studio/internal/store"
)

func TestTaxSummaryByKind(t *testing.T) {
	in := baseInput()
	in.Today = "2026-11-05"
	in.Settings.ReservePercent = "30"
	in.Issued = []issuedTax{invoice19("a", "RE-1", "2026-10-05")}
	in.Overview = store.PaymentOverview{Payments: []store.Payment{pay("p1", "a", "outgoing", "1190.00", "2026-10-08", "")}}
	in.Bookings = []PaymentBooking{
		booking("ust", "2026-05-10", -30_000), booking("ust-back", "2026-08-10", 5_000),
		booking("est", "2026-09-10", -40_000), booking("gewst", "2026-08-17", -20_000),
		booking("old", "2025-12-10", -99_900), booking("priv", "2026-06-01", -10_000),
	}
	in.Treatments = map[string]store.BookingTreatment{
		"ust": {Kind: "vat"}, "ust-back": {Kind: "vat"}, "est": {Kind: "tax"},
		"gewst": {Kind: "tradetax"}, "old": {Kind: "tax"}, "priv": {Kind: "private"},
	}
	s := taxSummary(in, 2026, "1190.00")
	// Refunds reduce what was paid; last year's payment stays out.
	if s.VATPaid.Amount != "250.00" || len(s.VATPaid.Entries) != 2 || s.VATPaid.Entries[1].Note != "Erstattung" {
		t.Fatalf("vat paid %+v", s.VATPaid)
	}
	if s.IncomeTaxPaid.Amount != "400.00" || s.TradeTaxPaid.Amount != "200.00" {
		t.Fatalf("income tax %s, trade tax %s", s.IncomeTaxPaid.Amount, s.TradeTaxPaid.Amount)
	}
	// 30 % of the year's result of 1,190 € is 357 €, all covered by the 400 € paid.
	if !s.IncomeTaxReserve.Available || s.IncomeTaxReserve.Amount != "0.00" || s.Profit != "1190.00" {
		t.Fatalf("reserve %+v profit %s", s.IncomeTaxReserve, s.Profit)
	}
	// 10.12.2026 is a Thursday; 15.11.2026 a Sunday, so the Monday after.
	if s.IncomeTaxNext != "2026-12-10" || s.TradeTaxNext != "2026-11-16" {
		t.Fatalf("next dates %s %s", s.IncomeTaxNext, s.TradeTaxNext)
	}
}

func TestNextPrepaymentDateCrossesTheYear(t *testing.T) {
	if got := nextDate(incomeTaxDates, "2026-12-11"); got != "2027-03-10" {
		t.Fatalf("after the last date of the year: %s", got)
	}
	if got := nextDate(tradeTaxDates, "2026-11-16"); got != "2026-11-16" {
		t.Fatalf("the due date itself: %s", got)
	}
}
