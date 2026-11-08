package app

import (
	"fmt"
	"time"

	"github.com/shopspring/decimal"
	"papierleicht.de/e-rechnung-studio/internal/taxforms"
)

// TaxSummary is the tax page's view of one year by kind of tax: what went to
// the tax office or the municipality, what is still open or set aside, and
// the next prepayment dates. All of it comes from the user's own records and
// settings; the app does not compute Einkommen- or Gewerbesteuer.
type TaxSummary struct {
	// Paid figures add up bank bookings classified as such, by booking date;
	// refunds reduce them.
	VATPaid       FinanceFigure `json:"vatPaid"`
	IncomeTaxPaid FinanceFigure `json:"incomeTaxPaid"`
	TradeTaxPaid  FinanceFigure `json:"tradeTaxPaid"`
	// VATOpen is the Umsatzsteuer still owed as of today, IncomeTaxReserve the
	// user's own rate on the year's result less what was paid.
	VATOpen          FinanceFigure `json:"vatOpen"`
	IncomeTaxReserve FinanceFigure `json:"incomeTaxReserve"`
	// Profit is the EÜR profit of the year, the starting point of the
	// Gewerbeertrag.
	Profit string `json:"profit"`
	// The next dates on which prepayments are due if the tax office set any
	// (§ 37 Abs. 1 EStG, § 19 Abs. 1 GewStG), moved off weekends.
	IncomeTaxNext string `json:"incomeTaxNext"`
	TradeTaxNext  string `json:"tradeTaxNext"`
}

var (
	incomeTaxDates = [][2]int{{3, 10}, {6, 10}, {9, 10}, {12, 10}}
	tradeTaxDates  = [][2]int{{2, 15}, {5, 15}, {8, 15}, {11, 15}}
)

// nextDate is the first of the yearly dates on or after today.
func nextDate(dates [][2]int, today string) string {
	t, err := time.Parse(time.DateOnly, today)
	if err != nil {
		return ""
	}
	for y := t.Year(); y <= t.Year()+1; y++ {
		for _, d := range dates {
			due := taxforms.NextWeekday(time.Date(y, time.Month(d[0]), d[1], 0, 0, 0, 0, time.UTC)).Format(time.DateOnly)
			if due >= today {
				return due
			}
		}
	}
	return ""
}

// paidAs adds up the bank bookings of the period classified as kind. A
// payment counts positive, a refund negative.
func paidAs(in financeInput, kind, label, explanation string) FinanceFigure {
	var entries []FinanceEntry
	for _, b := range in.Bookings {
		if in.Treatments[b.ID].Kind != kind || b.Data.Currency != "EUR" || !inPeriod(b.Data.BookingDate, in.From, in.To) {
			continue
		}
		amount := decimal.NewFromInt(b.Data.Amount).Shift(-2).Neg()
		e := FinanceEntry{Kind: "booking", ID: b.ID, Date: b.Data.BookingDate, Label: firstNonEmpty(b.Data.CounterpartyName, b.Data.Description, "Bankumsatz"), Amount: money(amount)}
		if amount.IsNegative() {
			e.Note = "Erstattung"
		}
		entries = append(entries, e)
	}
	return figure(label, areaLiquidity, explanation, entries)
}

// taxSummary reads year out of in, which must cover the whole year.
func taxSummary(in financeInput, year int, profit string) TaxSummary {
	in.From, in.To = fmt.Sprintf("%d-01-01", year), fmt.Sprintf("%d-12-31", year)
	report := buildFinanceReport(in)
	return TaxSummary{
		VATPaid:          paidAs(in, "vat", "Umsatzsteuer gezahlt", "Bankumsätze, die du als „Umsatzsteuer an oder vom Finanzamt“ eingeordnet hast, nach Buchungsdatum. Erstattungen sind abgezogen."),
		IncomeTaxPaid:    paidAs(in, "tax", "Einkommensteuer gezahlt", "Bankumsätze, die du als „Private Steuer“ eingeordnet hast, nach Buchungsdatum: Vorauszahlungen und Nachzahlungen, Erstattungen abgezogen."),
		TradeTaxPaid:     paidAs(in, "tradetax", "Gewerbesteuer gezahlt", "Bankumsätze, die du als „Gewerbesteuer“ eingeordnet hast, nach Buchungsdatum. Erstattungen sind abgezogen."),
		VATOpen:          report.VATReserve,
		IncomeTaxReserve: report.IncomeTaxReserve,
		Profit:           profit,
		IncomeTaxNext:    nextDate(incomeTaxDates, in.Today),
		TradeTaxNext:     nextDate(tradeTaxDates, in.Today),
	}
}
