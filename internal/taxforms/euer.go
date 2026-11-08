package taxforms

import (
	"sort"
	"time"

	"github.com/shopspring/decimal"
)

// Income is a payment received in the year, split like its invoice. Refunds
// on credit notes are negative. Margin marks the Differenzbesteuerung.
type Income struct {
	Entry    Entry
	Category string // EN 16931 VAT category of the share
	Net, Tax decimal.Decimal
	Margin   bool
}

// Expense is a deductible payment of the year. Category is a Categories ID.
type Expense struct {
	Entry    Entry
	Category string
	Net, Tax decimal.Decimal
}

// Asset is a purchase used for several years.
type Asset struct {
	Entry    Entry
	Acquired string // date of purchase (Anschaffung)
	Years    int    // useful life (Nutzungsdauer)
	Net, Tax decimal.Decimal
}

// VATMovement is a payment to (negative) or refund from (positive) the tax
// office for Umsatzsteuer, booked in the year it belongs to.
type VATMovement struct {
	Entry  Entry
	Amount decimal.Decimal
}

// TradeTaxMovement is a Gewerbesteuer payment to (negative) or refund from
// (positive) the municipality, booked in the year it was paid.
type TradeTaxMovement struct {
	Entry  Entry
	Amount decimal.Decimal
}

// AssetLine is one row of the asset list (Anlagenverzeichnis).
type AssetLine struct {
	Entry        Entry  `json:"entry"`
	Acquired     string `json:"acquired"`
	Years        int    `json:"years"`
	Cost         string `json:"cost"`
	Depreciation string `json:"depreciation"`
	BookValue    string `json:"bookValue"`
	GWG          bool   `json:"gwg"`
}

// EUR is a prepared Anlage EÜR.
type EUR struct {
	Year         int         `json:"year"`
	Income       []Field     `json:"income"`
	Expenses     []Field     `json:"expenses"`
	IncomeTotal  string      `json:"incomeTotal"`
	ExpenseTotal string      `json:"expenseTotal"`
	Profit       string      `json:"profit"`
	Assets       []AssetLine `json:"assets"`
	Issues       []Issue     `json:"issues"`
}

type eurLine struct {
	zeile, kz, label string
	sum              decimal.Decimal
	entries          []Entry
	// outside marks a value typed into the form that is no part of the
	// totals, like the column "nicht abziehbar".
	outside bool
}

// BuildEUR maps the year's payments to the Anlage EÜR. Regular VAT payers
// report net amounts with VAT in separate lines; small businesses (§ 19 UStG)
// report gross amounts. Umsatzsteuer paid to or refunded by the tax office
// counts for both, because small businesses can owe it too (§ 13b UStG).
// Gewerbesteuer never reduces the profit (§ 4 Abs. 5b EStG): it goes into the
// column "nicht abziehbar" of Zeile 67, as the Anleitung zur Anlage EÜR 2025
// asks, and stays out of the totals.
func BuildEUR(year int, smallBusiness bool, income []Income, vat []VATMovement, tradeTax []TradeTaxMovement, expenses []Expense, assets []Asset) EUR {
	lines := map[string]*eurLine{}
	issues := issueList{}
	var order []string
	put := func(zeile, kz, label string, outside bool, v decimal.Decimal, e Entry, note string) {
		key := zeile + "|" + kz
		if lines[key] == nil {
			lines[key] = &eurLine{zeile: zeile, kz: kz, label: label, outside: outside}
			order = append(order, key)
		}
		l := lines[key]
		l.sum = l.sum.Add(v)
		e.Amount = v.StringFixed(2)
		if note != "" {
			e.Note = note
		}
		l.entries = append(l.entries, e)
	}
	r := EUR{Year: year, Income: []Field{}, Expenses: []Field{}, Assets: []AssetLine{}, Issues: []Issue{}}

	for _, in := range income {
		switch {
		case smallBusiness:
			put("12", "111", "Betriebseinnahmen als Kleinunternehmer", false, in.Net.Add(in.Tax), in.Entry, "")
			if in.Category == "O" {
				// Zeile 13 breaks out what does not count towards the limits of § 19.
				put("13", "119", "davon nicht steuerbare Umsätze", true, in.Net.Add(in.Tax), in.Entry, "")
			}
		case in.Margin:
			put("15", "112", "Umsatzsteuerpflichtige Betriebseinnahmen (netto)", false, in.Net.Add(in.Tax), in.Entry, "")
			issues.add("margin_scheme", "Bei Differenzbesteuerung steht hier der volle Verkaufspreis. Rechne die Umsatzsteuer auf deine Marge heraus: Zeile 15 sinkt um diesen Betrag, Zeile 17 steigt um ihn.", in.Entry)
		case in.Category == "S" && !in.Tax.IsZero():
			// Refunds on credit notes are negative and reduce both lines.
			put("15", "112", "Umsatzsteuerpflichtige Betriebseinnahmen (netto)", false, in.Net, in.Entry, "")
			put("17", "140", "Vereinnahmte Umsatzsteuer", false, in.Tax, in.Entry, "")
		default:
			put("16", "103", "Steuerfreie, nicht steuerbare oder § 13b-Betriebseinnahmen", false, in.Net.Add(in.Tax), in.Entry, "")
		}
	}
	for _, m := range vat {
		if m.Amount.IsPositive() {
			put("18", "141", "Vom Finanzamt erstattete Umsatzsteuer", false, m.Amount, m.Entry, "")
		}
	}

	for _, ex := range expenses {
		c, ok := CategoryByID(ex.Category)
		if !ok || c.ID == AssetCategory {
			c, _ = CategoryByID("sonstige")
		}
		amount := ex.Net
		if smallBusiness {
			amount = ex.Net.Add(ex.Tax)
		}
		deductible := amount.Mul(c.Deductible).Round(2)
		put(c.Zeile, c.Kz, firstLabel(c.Line, c.Label), false, deductible, ex.Entry, "")
		if c.NonDeductibleKz != "" {
			put(c.Zeile, c.NonDeductibleKz, c.Label+" (nicht abziehbarer Teil)", true, amount.Sub(deductible), ex.Entry, "")
		}
		// Refunds on credit notes are negative and reduce the paid input tax.
		if !smallBusiness && !ex.Tax.IsZero() {
			put("57", "185", "Gezahlte Vorsteuer", false, ex.Tax, ex.Entry, "")
		}
	}

	yearEnd := time.Date(year, time.December, 31, 0, 0, 0, 0, time.UTC)
	for _, a := range assets {
		cost := a.Net
		if smallBusiness {
			cost = a.Net.Add(a.Tax)
		}
		acquired, err := time.Parse(time.DateOnly, a.Acquired)
		if err != nil || acquired.After(yearEnd) {
			continue
		}
		line := AssetLine{Entry: a.Entry, Acquired: a.Acquired, Years: a.Years, Cost: cost.StringFixed(2)}
		if !smallBusiness && a.Tax.IsPositive() && acquired.Year() == year {
			put("57", "185", "Gezahlte Vorsteuer", false, a.Tax, a.Entry, "")
		}
		if cost.LessThanOrEqual(GWGLimit) || a.Years <= 0 {
			line.GWG = true
			line.BookValue = "0.00"
			if acquired.Year() == year {
				line.Depreciation = cost.StringFixed(2)
				put("36", "132", "Geringwertige Wirtschaftsgüter (bis 800 € netto)", false, cost, a.Entry, "")
			} else {
				line.Depreciation = "0.00"
			}
			r.Assets = append(r.Assets, line)
			continue
		}
		this, rest := Depreciation(cost, acquired, a.Years, year)
		line.Depreciation, line.BookValue = this.StringFixed(2), rest.StringFixed(2)
		if this.IsPositive() {
			put("33", "130", "Abschreibung (AfA) auf bewegliche Wirtschaftsgüter", false, this, a.Entry, "")
		}
		r.Assets = append(r.Assets, line)
	}
	for _, m := range vat {
		if m.Amount.IsNegative() {
			put("58", "186", "An das Finanzamt gezahlte Umsatzsteuer", false, m.Amount.Neg(), m.Entry, "")
		}
	}
	// A refund reduces what was paid in the year; it is no Betriebseinnahme,
	// because the payment never reduced the profit either.
	var refunds []Entry
	for _, m := range tradeTax {
		note := ""
		if m.Amount.IsPositive() {
			note = "Erstattung"
			e := m.Entry
			e.Amount = m.Amount.StringFixed(2)
			refunds = append(refunds, e)
		}
		put("67", "168", "Gewerbesteuer (Sonstige beschränkt abziehbare Betriebsausgaben, Spalte „nicht abziehbar“)", true, m.Amount.Neg(), m.Entry, note)
	}
	if l := lines["67|168"]; l != nil && l.sum.IsNegative() {
		l.sum = decimal.Zero
		for _, e := range refunds {
			issues.add("trade_tax_refund", "Im Jahr kam mehr Gewerbesteuer zurück, als du gezahlt hast. Erstattungen sind keine Betriebseinnahmen; in Zeile 67 steht dann nichts.", e)
		}
	}

	sort.SliceStable(order, func(i, j int) bool { return zeileLess(lines[order[i]].zeile, lines[order[j]].zeile) })
	incomeTotal, expenseTotal := decimal.Zero, decimal.Zero
	for _, key := range order {
		l := lines[key]
		f := Field{Zeile: l.zeile, Kz: l.kz, Label: l.label, Amount: l.sum.StringFixed(2), Cents: true, Entries: l.entries}
		sort.SliceStable(f.Entries, func(i, j int) bool { return f.Entries[i].Date < f.Entries[j].Date })
		if zeileLess(l.zeile, "24") {
			r.Income = append(r.Income, f)
			if !l.outside {
				incomeTotal = incomeTotal.Add(l.sum)
			}
		} else {
			r.Expenses = append(r.Expenses, f)
			if !l.outside {
				expenseTotal = expenseTotal.Add(l.sum)
			}
		}
	}
	r.IncomeTotal, r.ExpenseTotal = incomeTotal.StringFixed(2), expenseTotal.StringFixed(2)
	r.Profit = incomeTotal.Sub(expenseTotal).StringFixed(2)
	r.Income = append(r.Income, Field{Zeile: "23", Kz: "159", Label: "Summe Betriebseinnahmen", Amount: r.IncomeTotal, Cents: true, Entries: []Entry{}, Info: true})
	r.Expenses = append(r.Expenses, Field{Zeile: "75", Kz: "199", Label: "Summe Betriebsausgaben", Amount: r.ExpenseTotal, Cents: true, Entries: []Entry{}, Info: true})
	r.Issues = issues.sorted()
	return r
}

// Depreciation returns the straight-line AfA of year and the book value at
// its end. The first year counts from the month of acquisition (§ 7 Abs. 1
// Satz 4 EStG). A useful life of one year (computer hardware and software,
// BMF-Schreiben vom 22.02.2022) is deducted in full in the year of purchase.
func Depreciation(cost decimal.Decimal, acquired time.Time, years, year int) (decimal.Decimal, decimal.Decimal) {
	if years == 1 {
		if year == acquired.Year() {
			return cost, decimal.Zero
		}
		return decimal.Zero, decimal.Zero
	}
	annual := cost.Div(decimal.NewFromInt(int64(years)))
	rest := cost
	for y := acquired.Year(); y <= year && rest.IsPositive(); y++ {
		months := int64(12)
		if y == acquired.Year() {
			months = int64(13 - int(acquired.Month()))
		}
		amount := decimal.Min(rest, annual.Mul(decimal.NewFromInt(months)).Div(decimal.NewFromInt(12)).Round(2))
		rest = rest.Sub(amount)
		if y == year {
			return amount, rest
		}
	}
	return decimal.Zero, rest
}

func firstLabel(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func zeileLess(a, b string) bool {
	num := func(s string) int {
		n := 0
		for _, c := range s {
			if c < '0' || c > '9' {
				break
			}
			n = n*10 + int(c-'0')
		}
		return n
	}
	return num(a) < num(b)
}
