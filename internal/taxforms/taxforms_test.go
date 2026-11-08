package taxforms

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

func d(s string) decimal.Decimal { return decimal.RequireFromString(s) }

func field(fields []Field, kz string) (Field, bool) {
	for _, f := range fields {
		if f.Kz == kz {
			return f, true
		}
	}
	return Field{}, false
}

func TestVATReturnKennzahlen(t *testing.T) {
	sales := []Sale{
		{Entry: Entry{ID: "a"}, Category: "S", Percent: d("19"), Net: d("1000.60"), Tax: d("190.11")},
		{Entry: Entry{ID: "b"}, Category: "S", Percent: d("7"), Net: d("100"), Tax: d("7")},
		{Entry: Entry{ID: "c"}, Category: "AE", Net: d("500"), BuyerCountry: "FR"},
		{Entry: Entry{ID: "d"}, Category: "AE", Net: d("300"), BuyerCountry: "DE"},
		{Entry: Entry{ID: "e"}, Category: "K", Net: d("200"), BuyerCountry: "AT"},
		{Entry: Entry{ID: "f"}, Category: "G", Net: d("50")},
		{Entry: Entry{ID: "g"}, Category: "L", Net: d("10")},
	}
	purchases := []Purchase{
		{Entry: Entry{ID: "p1"}, Category: "S", Net: d("100"), Tax: d("19")},
		{Entry: Entry{ID: "p2"}, Category: "AE", Net: d("100"), SellerCountry: "IE"},
	}
	r := BuildVATReturn(sales, purchases, d("0"))
	want := map[string]string{"81": "1000", "86": "100", "21": "500", "60": "300", "41": "200", "43": "50", "66": "19.00", "46": "100", "47": "19.00", "67": "19.00"}
	for kz, amount := range want {
		f, ok := field(r.Fields, kz)
		if !ok || f.Amount != amount {
			t.Errorf("Kz %s = %q, want %q", kz, f.Amount, amount)
		}
	}
	// 1000 × 19 % + 100 × 7 % + 19 (Kz 47) − 19 (Kz 66) − 19 (Kz 67)
	if r.Payable != "178.00" {
		t.Fatalf("Kz 83 = %s", r.Payable)
	}
	if len(r.Issues) != 1 || r.Issues[0].Code != "unsupported_sale" {
		t.Fatalf("issues %+v", r.Issues)
	}
	withSpecial := BuildVATReturn(sales[:1], nil, d("50"))
	if f, _ := field(withSpecial.Fields, "39"); f.Amount != "50.00" || withSpecial.Payable != "140.00" {
		t.Fatalf("special prepayment: %+v %s", f, withSpecial.Payable)
	}
}

func issueCodes(issues []Issue) map[string]int {
	out := map[string]int{}
	for _, i := range issues {
		out[i.Code] = len(i.Entries)
	}
	return out
}

func TestVATReturnPlaceOfSupplyMarginAndCorrections(t *testing.T) {
	sales := []Sale{
		// Reverse charge: only a German customer is § 13b Abs. 5, a customer
		// outside the EU is not taxable in Germany.
		{Entry: Entry{ID: "us"}, Category: "AE", Net: d("1000"), BuyerCountry: "US"},
		{Entry: Entry{ID: "ch"}, Category: "AE", Net: d("500"), BuyerCountry: "CH"},
		{Entry: Entry{ID: "de"}, Category: "AE", Net: d("300"), BuyerCountry: "DE"},
		{Entry: Entry{ID: "damage"}, Category: "O", Net: d("80"), BuyerCountry: "DE"},
		{Entry: Entry{ID: "abroad"}, Category: "O", Net: d("70"), BuyerCountry: "US"},
		{Entry: Entry{ID: "used"}, Category: "E", Net: d("1190"), Margin: true},
		{Entry: Entry{ID: "doctor"}, Category: "E", Net: d("400")},
	}
	purchases := []Purchase{
		{Entry: Entry{ID: "bill"}, Category: "S", Percent: d("19"), Net: d("200"), Tax: d("38")},
		{Entry: Entry{ID: "credit"}, Category: "S", Percent: d("19"), Net: d("-50"), Tax: d("-9.50")},
	}
	r := BuildVATReturn(sales, purchases, d("0"))
	want := map[string]string{"60": "300", "45": "1570", "48": "400", "66": "28.50"}
	for kz, amount := range want {
		if f, ok := field(r.Fields, kz); !ok || f.Amount != amount {
			t.Errorf("Kz %s = %q, want %q", kz, f.Amount, amount)
		}
	}
	if _, ok := field(r.Fields, "81"); ok {
		t.Error("margin sale reported as if its full price were taxable")
	}
	if r.Payable != "-28.50" {
		t.Fatalf("Kz 83 = %s", r.Payable)
	}
	codes := issueCodes(r.Issues)
	if codes["margin_scheme"] != 1 || codes["domestic_out_of_scope"] != 1 || len(codes) != 2 {
		t.Fatalf("issues %+v", r.Issues)
	}
	if margin := r.Issues[1]; margin.Code != "margin_scheme" || margin.Entries[0].Note != "Verkaufspreis 1.190,00 €" {
		t.Fatalf("margin entry %+v", margin)
	}
}

func TestVATReturnSmallBusinessOwesTaxWithoutDeduction(t *testing.T) {
	purchases := []Purchase{
		{Entry: Entry{ID: "ads"}, Category: "AE", Net: d("1000"), EU: true, NoInputTax: true},
		{Entry: Entry{ID: "saas"}, Category: "AE", Net: d("100"), SellerCountry: "US", NoInputTax: true},
		{Entry: Entry{ID: "parts"}, Category: "K", Net: d("200"), SellerCountry: "AT", NoInputTax: true},
	}
	r := BuildVATReturn(nil, purchases, d("0"))
	want := map[string]string{"46": "1000", "47": "190.00", "84": "100", "85": "19.00", "89": "200"}
	for kz, amount := range want {
		if f, ok := field(r.Fields, kz); !ok || f.Amount != amount {
			t.Errorf("Kz %s = %q, want %q", kz, f.Amount, amount)
		}
	}
	for _, kz := range []string{"61", "66", "67"} {
		if _, ok := field(r.Fields, kz); ok {
			t.Errorf("small business deducts input tax in Kz %s", kz)
		}
	}
	// 190 + 19 + 200 × 19 %
	if r.Payable != "247.00" {
		t.Fatalf("Kz 83 = %s", r.Payable)
	}
}

func TestPeriodsAndDeadlines(t *testing.T) {
	q := Periods(2026, "quarterly", false)
	if len(q) != 4 || q[3].ID != "2026-Q4" || q[3].To != "2026-12-31" || !q[3].Last {
		t.Fatalf("%+v", q)
	}
	// 10 January 2027 is a Sunday.
	if q[3].Due != "2027-01-11" {
		t.Fatalf("Q4 due %s", q[3].Due)
	}
	m := Periods(2026, "monthly", true)
	if m[0].ID != "2026-01" || m[0].Due != "2026-03-10" || m[1].To != "2026-02-28" {
		t.Fatalf("%+v", m[:2])
	}
	if p, ok := PeriodByID("2026-Q2", "quarterly", false); !ok || p.From != "2026-04-01" {
		t.Fatal(p)
	}
	if AnnualDeadline(2025) != "2026-07-31" || AnnualDeadline(2026) != "2027-08-02" {
		t.Fatal(AnnualDeadline(2025), AnnualDeadline(2026))
	}
}

func TestEURRegularAndSmallBusiness(t *testing.T) {
	income := []Income{{Entry: Entry{ID: "i"}, Category: "S", Net: d("1000"), Tax: d("190")}, {Entry: Entry{ID: "j"}, Category: "AE", Net: d("500")}}
	expenses := []Expense{
		{Entry: Entry{ID: "x"}, Category: "edv", Net: d("100"), Tax: d("19")},
		{Entry: Entry{ID: "y"}, Category: "bewirtung", Net: d("100"), Tax: d("19")},
		{Entry: Entry{ID: "z"}, Category: "unbekannt", Net: d("10")},
	}
	assets := []Asset{
		{Entry: Entry{ID: "laptop"}, Acquired: "2026-10-15", Years: 3, Net: d("1800"), Tax: d("342")},
		{Entry: Entry{ID: "chair"}, Acquired: "2026-03-01", Years: 13, Net: d("400"), Tax: d("76")},
	}
	vat := []VATMovement{{Entry: Entry{ID: "ust"}, Amount: d("-150")}, {Entry: Entry{ID: "back"}, Amount: d("20")}}
	r := BuildEUR(2026, false, income, vat, nil, expenses, assets)
	want := map[string]string{"112": "1000.00", "140": "190.00", "103": "500.00", "141": "20.00", "228": "100.00", "175": "70.00", "165": "30.00", "183": "10.00", "185": "456.00", "130": "150.00", "132": "400.00", "186": "150.00"}
	all := append(append([]Field{}, r.Income...), r.Expenses...)
	for kz, amount := range want {
		if f, ok := field(all, kz); !ok || f.Amount != amount {
			t.Errorf("Kz %s = %q, want %q", kz, f.Amount, amount)
		}
	}
	// The non-deductible share is typed into the form, so it can be copied.
	if f, _ := field(all, "165"); f.Info {
		t.Fatal("Kz 165 marked as check-only")
	}
	// Income 1710; expenses 100 + 70 + 10 + 456 + 150 + 400 + 150 (Bewirtung's 30 % excluded).
	if r.IncomeTotal != "1710.00" || r.ExpenseTotal != "1336.00" || r.Profit != "374.00" {
		t.Fatalf("totals %s %s %s", r.IncomeTotal, r.ExpenseTotal, r.Profit)
	}
	if len(r.Assets) != 2 || r.Assets[0].BookValue != "1650.00" || !r.Assets[1].GWG {
		t.Fatalf("assets %+v", r.Assets)
	}

	small := BuildEUR(2026, true, income, vat, nil, expenses[:1], nil)
	if f, _ := field(small.Income, "111"); f.Amount != "1690.00" {
		t.Fatalf("small business income %+v", small.Income)
	}
	if f, _ := field(small.Expenses, "228"); f.Amount != "119.00" {
		t.Fatalf("small business expense %+v", small.Expenses)
	}
	if _, ok := field(small.Expenses, "185"); ok {
		t.Fatal("small business deducts input tax")
	}
}

func TestEURCreditNoteRefundMarginAndSmallBusinessVAT(t *testing.T) {
	income := []Income{
		{Entry: Entry{ID: "paid"}, Category: "S", Net: d("200"), Tax: d("38")},
		{Entry: Entry{ID: "refund"}, Category: "S", Net: d("-50"), Tax: d("-9.50")},
		{Entry: Entry{ID: "used"}, Category: "E", Net: d("1190"), Margin: true},
	}
	expenses := []Expense{
		{Entry: Entry{ID: "bill"}, Category: "edv", Net: d("100"), Tax: d("19")},
		{Entry: Entry{ID: "supplier-refund"}, Category: "edv", Net: d("-20"), Tax: d("-3.80")},
	}
	r := BuildEUR(2026, false, income, nil, nil, expenses, nil)
	all := append(append([]Field{}, r.Income...), r.Expenses...)
	want := map[string]string{"112": "1340.00", "140": "28.50", "228": "80.00", "185": "15.20"}
	for kz, amount := range want {
		if f, ok := field(all, kz); !ok || f.Amount != amount {
			t.Errorf("Kz %s = %q, want %q", kz, f.Amount, amount)
		}
	}
	if _, ok := field(all, "103"); ok {
		t.Error("refund on a credit note booked as tax-free income")
	}
	if codes := issueCodes(r.Issues); codes["margin_scheme"] != 1 || len(codes) != 1 {
		t.Fatalf("issues %+v", r.Issues)
	}

	// A small business that paid § 13b tax deducts the payment.
	vat := []VATMovement{{Entry: Entry{ID: "paid-13b"}, Amount: d("-190")}}
	small := BuildEUR(2026, true, nil, vat, nil, []Expense{{Entry: Entry{ID: "ads"}, Category: "werbung", Net: d("1000")}}, nil)
	if f, ok := field(small.Expenses, "186"); !ok || f.Amount != "190.00" || small.ExpenseTotal != "1190.00" {
		t.Fatalf("small business VAT payment %+v total %s", small.Expenses, small.ExpenseTotal)
	}
}

func TestZMReportAndPeriods(t *testing.T) {
	r := BuildZM([]ZMSale{
		{Entry: Entry{ID: "a"}, VATID: "at u123 456 78", Partner: "Wien KG", Net: d("1000.90")},
		{Entry: Entry{ID: "b"}, VATID: "ATU12345678", Partner: "Wien KG", Net: d("-100.40")},
		{Entry: Entry{ID: "c"}, VATID: "ATU12345678", Partner: "Wien KG", Services: true, Net: d("300")},
		{Entry: Entry{ID: "d"}, VATID: "", Partner: "Ohne ID", Net: d("50")},
	})
	if len(r.Lines) != 2 || r.Lines[0].Amount != "900" || r.Lines[0].Services || r.Lines[1].Amount != "300" || r.Total != "1200" || r.Lines[0].Country != "AT" {
		t.Fatalf("lines %+v total %s", r.Lines, r.Total)
	}
	if len(r.Issues) != 1 || r.Issues[0].Code != "zm_missing_vat_id" {
		t.Fatalf("issues %+v", r.Issues)
	}
	p := ZMPeriods(2026, map[int]bool{3: true})
	if len(p) != 6 || p[0].ID != "2026-Q1" || p[2].ID != "2026-07" || p[4].ID != "2026-09" || p[5].ID != "2026-Q4" {
		t.Fatalf("periods %+v", p)
	}
	// 25 October 2026 is a Sunday; a Dauerfristverlängerung does not apply.
	if p[4].Due != "2026-10-26" || p[5].Due != "2027-01-25" {
		t.Fatalf("due %s %s", p[4].Due, p[5].Due)
	}
	if QuarterOf("2026-08-05") != 3 || QuarterOf("x") != 0 {
		t.Fatal("quarter")
	}
	if AdvisorDeadline(2024) != "2026-04-30" || AdvisorDeadline(2025) != "2027-03-01" || AdvisorDeadline(2026) != "2028-02-29" {
		t.Fatal(AdvisorDeadline(2025), AdvisorDeadline(2026))
	}
}

func TestEURSharedLinesAndSmallBusinessBreakdown(t *testing.T) {
	expenses := []Expense{
		{Entry: Entry{ID: "train"}, Category: "fahrtkosten", Net: d("80")},
		{Entry: Entry{ID: "fuel"}, Category: "fahrzeug", Net: d("50")},
		{Entry: Entry{ID: "fees"}, Category: "bank", Net: d("9")},
	}
	r := BuildEUR(2026, false, nil, nil, nil, expenses, nil)
	f, ok := field(r.Expenses, "146")
	if !ok || f.Amount != "130.00" || f.Zeile != "70" || f.Label != "Sonstige Fahrtkosten (ohne AfA und Zinsen)" {
		t.Fatalf("Zeile 70 %+v", f)
	}
	if f, ok = field(r.Expenses, "183"); !ok || f.Amount != "9.00" {
		t.Fatalf("bank fees %+v", r.Expenses)
	}
	income := []Income{{Entry: Entry{ID: "de"}, Category: "E", Net: d("1000")}, {Entry: Entry{ID: "us"}, Category: "O", Net: d("400")}}
	small := BuildEUR(2026, true, income, nil, nil, nil, nil)
	if f, ok = field(small.Income, "119"); !ok || f.Amount != "400.00" || f.Info || small.IncomeTotal != "1400.00" {
		t.Fatalf("Zeile 13 %+v total %s", small.Income, small.IncomeTotal)
	}
}

func TestDepreciationSchedule(t *testing.T) {
	acquired := time.Date(2026, time.July, 20, 0, 0, 0, 0, time.UTC)
	sum := decimal.Zero
	for y := 2026; y <= 2030; y++ {
		this, rest := Depreciation(d("3000"), acquired, 3, y)
		sum = sum.Add(this)
		if y == 2026 && this.String() != "500" {
			t.Fatalf("first year %s", this)
		}
		if y == 2029 && (this.String() != "500" || !rest.IsZero()) {
			t.Fatalf("last year %s rest %s", this, rest)
		}
	}
	if !sum.Equal(d("3000")) {
		t.Fatalf("total %s", sum)
	}
	// Computers with a one-year life are deducted in full in the year of purchase.
	late := time.Date(2026, time.November, 3, 0, 0, 0, 0, time.UTC)
	if this, rest := Depreciation(d("1200"), late, 1, 2026); !this.Equal(d("1200")) || !rest.IsZero() {
		t.Fatalf("one-year life: %s %s", this, rest)
	}
	if this, _ := Depreciation(d("1200"), late, 1, 2027); !this.IsZero() {
		t.Fatal("one-year life depreciated twice")
	}
}

func TestFormatGerman(t *testing.T) {
	if FormatGerman("1234567.5", true) != "1.234.567,50" || FormatGerman("-12", false) != "-12" || FormatGerman("999", false) != "999" {
		t.Fatal(FormatGerman("1234567.5", true), FormatGerman("-12", false))
	}
}

func TestEURTradeTaxIsNotDeductible(t *testing.T) {
	expenses := []Expense{{Entry: Entry{ID: "x"}, Category: "edv", Net: d("100"), Tax: d("19")}}
	trade := []TradeTaxMovement{{Entry: Entry{ID: "q1"}, Amount: d("-500")}, {Entry: Entry{ID: "q2"}, Amount: d("-500")}, {Entry: Entry{ID: "back"}, Amount: d("120")}}
	r := BuildEUR(2026, false, nil, nil, trade, expenses, nil)
	f, ok := field(r.Expenses, "168")
	if !ok || f.Zeile != "67" || f.Amount != "880.00" || f.Info || len(f.Entries) != 3 {
		t.Fatalf("Zeile 67 nicht abziehbar: %+v", f)
	}
	// Only the software counts: 100 net plus 19 input tax.
	if r.ExpenseTotal != "119.00" || r.Profit != "-119.00" {
		t.Fatalf("Gewerbesteuer reduced the profit: %s %s", r.ExpenseTotal, r.Profit)
	}
	if _, ok := field(r.Income, "141"); ok {
		t.Fatal("refund counted as income")
	}

	// More refunded than paid: the line stays empty and the refund is named.
	r = BuildEUR(2026, true, nil, nil, []TradeTaxMovement{{Entry: Entry{ID: "back"}, Amount: d("300")}}, nil, nil)
	if f, _ := field(r.Expenses, "168"); f.Amount != "0.00" || r.Profit != "0.00" || issueCodes(r.Issues)["trade_tax_refund"] != 1 {
		t.Fatalf("refund only: %+v %+v", f, r.Issues)
	}
}
