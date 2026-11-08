package app

import (
	"testing"

	"papierleicht.de/e-rechnung-studio/internal/store"
)

func TestFinanceYearSplitsIncomeAndExpensesByMonth(t *testing.T) {
	in := baseInput()
	in.Today = "2026-11-02"
	in.Issued = []issuedTax{invoice19("a", "RE-1", "2026-02-05"), invoice19("b", "RE-2", "2026-03-06")}
	in.Received = nil
	in.Bookings = []PaymentBooking{booking("ust", "2026-03-10", -5_000)}
	in.Treatments = map[string]store.BookingTreatment{"ust": {BookingID: "ust", Kind: "vat"}}
	in.Expenses = []store.Expense{{ID: "e1", Date: "2026-03-12", PaidDate: "2026-03-12", Supplier: "Bürohandel", Category: "arbeitsmittel", Gross: "119.00", Net: "100.00", Tax: "19.00", TaxRate: "19", PaidWith: "bank"}}
	in.Overview = store.PaymentOverview{Payments: []store.Payment{
		pay("p1", "a", "outgoing", "1190.00", "2026-02-20", ""),
		pay("p2", "b", "outgoing", "400.00", "2026-03-20", ""),
		pay("p3", "b", "outgoing", "790.00", "2025-12-30", ""),
	}}
	y := buildFinanceYear(in, 2026)
	if y.Year != 2026 || len(y.Months) != 12 {
		t.Fatalf("year %+v", y)
	}
	// Each month is the regular report for that month.
	for _, m := range y.Months[:10] {
		in.From, in.To = m.From, m.To
		r := buildFinanceReport(in)
		if m.Future || m.Income != r.Income.Amount || m.Expenses != r.Expenses.Amount || m.Result != r.Result.Amount {
			t.Fatalf("month %d = %+v, report income %s expenses %s", m.Month, m, r.Income.Amount, r.Expenses.Amount)
		}
	}
	feb, mar := y.Months[1], y.Months[2]
	if feb.From != "2026-02-01" || feb.To != "2026-02-28" || feb.Income != "1190.00" || feb.Expenses != "0.00" {
		t.Fatalf("february %+v", feb)
	}
	// March: 400 € in; the receipt and the Umsatzsteuer payment out.
	if mar.Income != "400.00" || mar.Expenses != "169.00" || mar.VAT != "50.00" || mar.Result != "231.00" {
		t.Fatalf("march %+v", mar)
	}
	// November has begun, December has not.
	if y.Months[10].Future || !y.Months[11].Future || y.Months[11].Income != "0.00" {
		t.Fatalf("november %+v december %+v", y.Months[10], y.Months[11])
	}
}

func TestFinanceYearRejectsOddYears(t *testing.T) {
	s := &InvoiceService{}
	for _, year := range []int{0, 1999, 2101} {
		if _, err := s.FinanceYear(year); err == nil {
			t.Fatalf("year %d accepted", year)
		}
	}
}
