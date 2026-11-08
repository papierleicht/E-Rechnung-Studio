package app

import (
	"errors"
	"fmt"
	"time"

	"github.com/shopspring/decimal"
)

// FinanceMonth is one month of the year view, with the same figures as the
// Betriebseinnahmen and Betriebsausgaben of the finance report for that month.
type FinanceMonth struct {
	Month    int    `json:"month"` // 1 to 12
	From     string `json:"from"`
	To       string `json:"to"`
	Income   string `json:"income"`
	Expenses string `json:"expenses"` // positive, like the report's figure
	// VAT is the Umsatzsteuer paid to the tax office, part of Expenses.
	VAT        string `json:"vat"`
	Result     string `json:"result"`
	Incomplete bool   `json:"incomplete"`
	// Future marks months that have not started yet.
	Future bool `json:"future"`
}

type FinanceYear struct {
	Year   int            `json:"year"`
	Today  string         `json:"today"`
	Months []FinanceMonth `json:"months"`
}

// FinanceYear returns income and expenses of every month of a year. The data
// is read once; each month is the regular report for that month.
func (s *InvoiceService) FinanceYear(year int) (FinanceYear, error) {
	if year < 2000 || year > 2100 {
		return FinanceYear{}, errors.New("Wähle ein gültiges Jahr.")
	}
	in, err := s.loadFinanceInput(fmt.Sprintf("%d-01-01", year), fmt.Sprintf("%d-12-31", year))
	if err != nil {
		return FinanceYear{}, err
	}
	return buildFinanceYear(in, year), nil
}

func buildFinanceYear(in financeInput, year int) FinanceYear {
	out := FinanceYear{Year: year, Today: in.Today, Months: make([]FinanceMonth, 0, 12)}
	for m := 1; m <= 12; m++ {
		first := time.Date(year, time.Month(m), 1, 0, 0, 0, 0, time.UTC)
		month := FinanceMonth{Month: m, From: first.Format(time.DateOnly), To: first.AddDate(0, 1, -1).Format(time.DateOnly)}
		if month.From > in.Today {
			month.Income, month.Expenses, month.VAT, month.Result, month.Future = "0.00", "0.00", "0.00", "0.00", true
			out.Months = append(out.Months, month)
			continue
		}
		in.From, in.To = month.From, month.To
		r := buildFinanceReport(in)
		vat := decimal.Zero
		for _, e := range r.Expenses.Entries {
			if e.Kind == "vat" {
				amount, _ := decimal.NewFromString(e.Amount)
				vat = vat.Add(amount.Abs())
			}
		}
		month.Income, month.Expenses, month.VAT, month.Result = r.Income.Amount, r.Expenses.Amount, money(vat), r.Result.Amount
		month.Incomplete = r.Income.Incomplete || r.Expenses.Incomplete
		out.Months = append(out.Months, month)
	}
	return out
}
