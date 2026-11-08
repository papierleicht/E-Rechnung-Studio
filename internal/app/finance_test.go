package app

import (
	"strings"
	"testing"

	fints "github.com/BergerAPI/go-fints"
	"github.com/shopspring/decimal"
	"papierleicht.de/e-rechnung-studio/internal/model"
	"papierleicht.de/e-rechnung-studio/internal/store"
)

func d(s string) decimal.Decimal { return decimal.RequireFromString(s) }

// invoice19 is a 1,000 € net invoice at 19 % (gross 1,190 €).
func invoice19(id, number, taxDate string) issuedTax {
	return issuedTax{ID: id, Number: number, Partner: "Kunde", Currency: "EUR", TaxDate: taxDate, Gross: d("1190"), Rates: []taxRate{{Category: "S", Percent: d("19"), Basis: d("1000"), Amount: d("190")}}}
}

func pay(id, invoice, direction, amount, date, booking string) store.Payment {
	return store.Payment{ID: id, InvoiceID: invoice, Direction: direction, Amount: amount, Currency: "EUR", Date: date, Source: "bank", BookingID: booking}
}

func booking(id, date string, cents int64) PaymentBooking {
	return PaymentBooking{ID: id, AccountID: "DE02120300000000202051", Source: "bank", Data: fints.Transaction{BookingDate: date, Amount: cents, Currency: "EUR", CounterpartyName: "Partner " + id}}
}

func baseInput() financeInput {
	return financeInput{From: "2026-10-01", To: "2026-10-31", Today: "2026-11-02", Settings: store.FinanceSettings{TaxBasis: "soll"}, Treatments: map[string]store.BookingTreatment{}}
}

func hasIssue(r FinanceReport, code string) bool {
	for _, i := range r.Issues {
		if i.Code == code {
			return true
		}
	}
	return false
}

// Every listed figure must equal the sum of the records it shows.
func assertTraceable(t *testing.T, r FinanceReport) {
	t.Helper()
	for _, f := range []FinanceFigure{r.Inflows, r.Outflows, r.Other, r.Receivables, r.Payables, r.Income, r.Expenses, r.Result, r.OutputTax, r.InputTax, r.VATPayable, r.BankBalance, r.Available} {
		if !f.Available {
			continue
		}
		sum := decimal.Zero
		for _, e := range f.Entries {
			v, err := decimal.NewFromString(e.Amount)
			if err != nil {
				t.Fatalf("%s: entry %q has no amount", f.Label, e.Label)
			}
			sum = sum.Add(v)
		}
		if !sum.Equal(d(f.Amount)) {
			t.Errorf("%s = %s, but its entries add up to %s", f.Label, f.Amount, sum.StringFixed(2))
		}
	}
}

func TestPartialPaymentAndCollectivePayment(t *testing.T) {
	in := baseInput()
	in.Issued = []issuedTax{invoice19("a", "RE-1", "2026-10-05"), invoice19("b", "RE-2", "2026-10-06")}
	in.Bookings = []PaymentBooking{booking("bk1", "2026-10-20", 159_000)}
	// One bank transfer pays RE-1 in full and RE-2 partly (Sammelzahlung).
	in.Overview = store.PaymentOverview{
		Payments: []store.Payment{pay("p1", "a", "outgoing", "1190.00", "2026-10-20", "bk1"), pay("p2", "b", "outgoing", "400.00", "2026-10-20", "bk1")},
		Invoices: []store.PaymentBalance{
			{ID: "a", Direction: "outgoing", Number: "RE-1", Currency: "EUR", Remaining: "0.00"},
			{ID: "b", Direction: "outgoing", Number: "RE-2", Currency: "EUR", Remaining: "790.00", DueDate: "2026-10-30"},
		},
	}
	r := buildFinanceReport(in)
	assertTraceable(t, r)
	if r.Inflows.Amount != "1590.00" || len(r.Inflows.Entries) != 2 {
		t.Fatalf("inflows %+v", r.Inflows)
	}
	if r.Receivables.Amount != "790.00" || r.Receivables.Entries[0].Note != "überfällig" {
		t.Fatalf("receivables %+v", r.Receivables)
	}
	if hasIssue(r, "unassigned_bookings") {
		t.Fatal("fully allocated collective payment reported as unassigned")
	}
	// Soll: both invoices count by date of supply, regardless of payment.
	if r.OutputTax.Amount != "380.00" || r.OutputTaxRates[0].Net != "2000.00" {
		t.Fatalf("output tax %+v %+v", r.OutputTax, r.OutputTaxRates)
	}
}

func TestIstTaxFollowsPayments(t *testing.T) {
	in := baseInput()
	in.Settings.TaxBasis = "ist"
	in.Issued = []issuedTax{invoice19("a", "RE-1", "2026-09-15")}
	in.Overview = store.PaymentOverview{Payments: []store.Payment{pay("p1", "a", "outgoing", "476.00", "2026-10-10", "")}}
	r := buildFinanceReport(in)
	assertTraceable(t, r)
	// 476 of 1,190 € is 40 %, so 40 % of the 190 € tax arises in October.
	if r.OutputTax.Amount != "76.00" || r.OutputTaxRates[0].Net != "400.00" || !strings.Contains(r.OutputTax.Entries[0].Note, "40") {
		t.Fatalf("ist tax %+v %+v", r.OutputTax, r.OutputTaxRates)
	}
	in.Settings.TaxBasis = "soll"
	if got := buildFinanceReport(in).OutputTax.Amount; got != "0.00" {
		t.Fatalf("soll tax for a September supply in October = %s", got)
	}
}

func TestOverpaymentAndRefundViaCreditNote(t *testing.T) {
	in := baseInput()
	credit := invoice19("c", "GS-1", "2026-10-12")
	credit.Credit = true
	credit.Gross = d("-119")
	credit.Rates = []taxRate{{Category: "S", Percent: d("19"), Basis: d("-100"), Amount: d("-19")}}
	in.Issued = []issuedTax{invoice19("a", "RE-1", "2026-10-05"), credit}
	in.Overview = store.PaymentOverview{
		Payments: []store.Payment{
			pay("p1", "a", "outgoing", "1200.00", "2026-10-08", ""), // 10 € overpaid
			pay("p2", "c", "outgoing", "119.00", "2026-10-15", ""),  // refund to the customer
		},
		Invoices: []store.PaymentBalance{{ID: "a", Direction: "outgoing", Currency: "EUR", Remaining: "0.00", Overpaid: "10.00"}},
	}
	r := buildFinanceReport(in)
	assertTraceable(t, r)
	if r.Income.Amount != "1081.00" {
		t.Fatalf("income %+v", r.Income)
	}
	if r.OutputTax.Amount != "171.00" {
		t.Fatalf("credit note must reduce output tax: %+v", r.OutputTax)
	}
	if r.Receivables.Amount != "0.00" {
		t.Fatalf("overpaid invoice still open: %+v", r.Receivables)
	}
}

func TestReceivedCreditNoteReducesInputTaxAndIsRefunded(t *testing.T) {
	in := baseInput()
	valid := model.ValidationResult{State: "valid"}
	totals := model.Totals{Net: "100.00", Tax: "19.00", Gross: "119.00"}
	in.Received = []model.ReceivedInvoice{
		{ID: "bill", Status: "active", Inspection: model.ImportInspection{InvoiceNumber: "L-1", SellerName: "Lieferant", IssueDate: "2026-10-02", Currency: "EUR", Totals: totals, Validation: valid}},
		{ID: "credit", Status: "active", Inspection: model.ImportInspection{InvoiceNumber: "G-1", SellerName: "Lieferant", IssueDate: "2026-10-20", Currency: "EUR", Totals: totals, Validation: valid}},
	}
	in.ReceivedTax = map[string]receivedTax{"credit": {Credit: true, Country: "DE", Rates: []taxRate{{Category: "S", Percent: d("19"), Basis: d("-100"), Amount: d("-19")}}}}
	in.Overview = store.PaymentOverview{
		Payments: []store.Payment{pay("p1", "bill", "incoming", "119.00", "2026-10-05", ""), pay("p2", "credit", "incoming", "119.00", "2026-10-25", "")},
		Invoices: []store.PaymentBalance{{ID: "open-credit", Direction: "incoming", Currency: "EUR", Remaining: "50.00"}},
	}
	in.ReceivedTax["open-credit"] = receivedTax{Credit: true, Rates: []taxRate{}}
	r := buildFinanceReport(in)
	assertTraceable(t, r)
	if r.InputTax.Amount != "0.00" || r.Outflows.Amount != "0.00" {
		t.Fatalf("input tax %s outflows %s", r.InputTax.Amount, r.Outflows.Amount)
	}
	if r.Receivables.Amount != "50.00" || r.Payables.Amount != "0.00" {
		t.Fatalf("a supplier's credit note is money owed to the business: %+v %+v", r.Receivables, r.Payables)
	}
}

func TestSmallBusinessOwesReverseChargeTax(t *testing.T) {
	in := baseInput()
	in.SmallBusiness = true
	in.Expenses = []store.Expense{
		{ID: "ads", Date: "2026-10-03", PaidDate: "2026-10-03", Supplier: "Werbeplattform", Category: "werbung", Gross: "1000.00", Net: "1000.00", Tax: "0.00", TaxRate: store.RateReverseChargeEU, PaidWith: "bank"},
		{ID: "office", Date: "2026-10-04", PaidDate: "2026-10-04", Supplier: "Bürohandel", Category: "arbeitsmittel", Gross: "119.00", Net: "100.00", Tax: "19.00", TaxRate: "19", PaidWith: "bank"},
	}
	r := buildFinanceReport(in)
	assertTraceable(t, r)
	if r.OutputTax.Amount != "190.00" || r.InputTax.Amount != "0.00" || r.VATReserve.Amount != "190.00" {
		t.Fatalf("output %s input %s reserve %s", r.OutputTax.Amount, r.InputTax.Amount, r.VATReserve.Amount)
	}
}

func TestMarginSchemeMakesVATIncomplete(t *testing.T) {
	in := baseInput()
	used := issuedTax{ID: "m", Number: "RE-7", Partner: "Kunde", Currency: "EUR", TaxDate: "2026-10-05", Gross: d("1190"), Rates: []taxRate{{Category: "E", Percent: d("0"), Basis: d("1190"), Amount: d("0"), ReasonCode: "VATEX-EU-F"}}}
	in.Issued = []issuedTax{used}
	r := buildFinanceReport(in)
	if !hasIssue(r, "margin_scheme") || !r.VATPayable.Incomplete {
		t.Fatalf("margin sale must flag the VAT figures: %+v", r.Issues)
	}
}

func TestCorrectionDatesFollowReason(t *testing.T) {
	original := invoice19("orig", "RE-1", "2026-09-09")
	correction := func(id, reason string, gross string, credit bool) issuedTax {
		v := issuedTax{ID: id, TaxDate: "2026-09-09", IssueDate: "2026-10-20", Correction: true, CorrectsID: "orig", Reason: reason, Credit: credit, Gross: d(gross)}
		return v
	}
	standalone := correction("alone", "", "-119", true)
	standalone.CorrectsID = ""
	issued := []issuedTax{
		original,
		correction("full", "", "-1190", true),   // replaced invoice, as before
		correction("partial", "", "-119", true), // price reduction
		correction("cancel", model.CorrectionChange, "-1190", true), // supply cancelled
		correction("reissue", model.CorrectionReplace, "-119", true),
		correction("delta", "", "-119", false), // corrected invoice
		standalone,
	}
	applyCorrectionDates(issued)
	want := map[string]string{"orig": "2026-09-09", "full": "2026-09-09", "partial": "2026-10-20", "cancel": "2026-10-20", "reissue": "2026-09-09", "delta": "2026-10-20", "alone": "2026-10-20"}
	for _, v := range issued {
		if v.TaxDate != want[v.ID] {
			t.Errorf("%s: tax date %s, want %s", v.ID, v.TaxDate, want[v.ID])
		}
	}
}

func TestRevokedForeignAndRepeatedRecordsDoNotCount(t *testing.T) {
	in := baseInput()
	in.Issued = []issuedTax{invoice19("a", "RE-1", "2026-10-05")}
	revoked := pay("p1", "a", "outgoing", "1190.00", "2026-10-08", "")
	revoked.RevokedAt = "2026-10-09T10:00:00Z"
	usd := pay("p2", "a", "outgoing", "50.00", "2026-10-08", "")
	usd.Currency = "USD"
	in.Overview = store.PaymentOverview{Payments: []store.Payment{revoked, usd}}
	// A repeated import yields the same booking ID once; the report lists it once.
	in.Bookings = []PaymentBooking{booking("bk1", "2026-10-03", -5_000)}
	r := buildFinanceReport(in)
	if r.Inflows.Amount != "0.00" || !hasIssue(r, "foreign_currency") {
		t.Fatalf("revoked or foreign payment counted: %+v", r.Inflows)
	}
	if !hasIssue(r, "unassigned_bookings") || !r.Income.Incomplete || !r.Available.Incomplete {
		t.Fatal("unassigned booking must mark payments, EÜR and liquidity incomplete")
	}
	if r.Complete {
		t.Fatal("report with issues marked complete")
	}
}

func TestOtherMovementsInputTaxAndReserves(t *testing.T) {
	in := baseInput()
	in.Settings.ReservePercent = "30"
	// Typed with spaces; must still match the bank account of the bookings.
	in.Settings.Balances = []store.AccountBalance{{AccountID: "de02 1203 0000 0000 2020 51", Amount: "5000.00", Date: "2026-11-01"}}
	in.Issued = []issuedTax{invoice19("a", "RE-1", "2026-10-05")}
	valid := model.ValidationResult{State: "valid"}
	in.Received = []model.ReceivedInvoice{
		{ID: "r1", Status: "active", Inspection: model.ImportInspection{InvoiceNumber: "L-1", SellerName: "Lieferant", IssueDate: "2026-10-02", Currency: "EUR", Totals: model.Totals{Net: "100.00", Tax: "19.00", Gross: "119.00"}, Validation: valid}},
		{ID: "r2", Status: "active", Inspection: model.ImportInspection{SellerName: "Ungeprüft", IssueDate: "2026-10-03", Currency: "EUR", Totals: model.Totals{Tax: "5.00"}, Validation: model.ValidationResult{State: "invalid"}}},
		{ID: "r3", Status: "misimport", Inspection: model.ImportInspection{IssueDate: "2026-10-03", Totals: model.Totals{Tax: "999.00"}, Validation: valid}},
	}
	in.Bookings = []PaymentBooking{booking("priv", "2026-10-04", -20_000), booking("tax", "2026-10-10", -30_000), booking("ust", "2026-10-11", -5_000)}
	in.Treatments = map[string]store.BookingTreatment{"priv": {BookingID: "priv", Kind: "private"}, "tax": {BookingID: "tax", Kind: "tax"}, "ust": {BookingID: "ust", Kind: "vat"}}
	in.Overview = store.PaymentOverview{
		Payments: []store.Payment{pay("p1", "a", "outgoing", "1190.00", "2026-10-08", ""), pay("p2", "r1", "incoming", "119.00", "2026-10-09", "")},
		Invoices: []store.PaymentBalance{{ID: "r9", Direction: "incoming", Currency: "EUR", Remaining: "300.00"}},
	}
	r := buildFinanceReport(in)
	assertTraceable(t, r)
	// Private movements stay out of the EÜR; the Umsatzsteuer payment is an expense.
	if r.Other.Amount != "-550.00" || r.Expenses.Amount != "169.00" {
		t.Fatalf("other %s, expenses %s", r.Other.Amount, r.Expenses.Amount)
	}
	// The overview marks the tax office's share of the expenses by its kind.
	vat := 0
	for _, e := range r.Expenses.Entries {
		if e.Kind == "vat" {
			vat++
			if e.ID != "ust" || e.Amount != "50.00" {
				t.Fatalf("vat entry %+v", e)
			}
		}
	}
	if vat != 1 {
		t.Fatalf("vat entries %+v", r.Expenses.Entries)
	}
	if r.InputTax.Amount != "19.00" || !hasIssue(r, "received_needs_review") {
		t.Fatalf("input tax %+v issues %+v", r.InputTax, r.Issues)
	}
	// Result 1,021 €; open VAT 190 − 19 = 171 € for the running year; reserve
	// 30 % of 1,021 € less the 300 € income tax already paid.
	if r.Result.Amount != "1021.00" || r.VATReserve.Amount != "171.00" || r.IncomeTaxReserve.Amount != "6.30" {
		t.Fatalf("result %s vat %s reserve %s", r.Result.Amount, r.VATReserve.Amount, r.IncomeTaxReserve.Amount)
	}
	// 5,000 − 300 payable − 171 − 6.30.
	if hasIssue(r, "missing_balance") || hasIssue(r, "stale_balance") {
		t.Fatalf("balance not matched to its account: %+v", r.Issues)
	}
	if !r.Available.Available || r.Available.Amount != "4522.70" {
		t.Fatalf("available %+v", r.Available)
	}
}

func TestOpenVATCountsUnpaidPeriodsOnly(t *testing.T) {
	in := baseInput() // today 2026-11-02
	in.Settings.VATPeriod, in.Settings.VATExtension = "quarterly", true
	in.Issued = []issuedTax{invoice19("q1", "RE-1", "2026-02-05"), invoice19("q2", "RE-2", "2026-05-05"), invoice19("q3", "RE-3", "2026-08-05"), invoice19("q4", "RE-4", "2026-10-05")}
	in.VATFilings = map[string]store.VATFiling{
		"2026-Q2": {PeriodID: "2026-Q2", SubmittedOn: "2026-08-03", Payable: "150.00"}, // due in August, 40 € more since
		"2026-Q3": {PeriodID: "2026-Q3", SubmittedOn: "2026-10-30", Payable: "190.00"}, // due 10 November
	}
	r := buildFinanceReport(in)
	// Q1 is overdue without a transmission, Q2 owes the increase, Q3 is not
	// yet due and Q4 still running.
	if r.VATReserve.Amount != "610.00" || len(r.VATReserve.Entries) != 4 {
		t.Fatalf("open VAT %+v", r.VATReserve)
	}
	if e := r.VATReserve.Entries[1]; e.ID != "vat:2026-Q2" || e.Amount != "40.00" {
		t.Fatalf("increase after transmission %+v", e)
	}
	if e := r.VATReserve.Entries[3]; e.ID != "vat:2026-Q4" || e.Note != "läuft noch, fällig 10.02.2027" {
		t.Fatalf("running period %+v", e)
	}
}

func TestSmallBusinessAndMissingSettings(t *testing.T) {
	in := baseInput()
	in.SmallBusiness = true
	in.Issued = []issuedTax{invoice19("a", "RE-1", "2026-10-05")}
	in.Received = []model.ReceivedInvoice{{ID: "r1", Status: "active", Inspection: model.ImportInspection{Totals: model.Totals{Tax: "19.00"}}}}
	r := buildFinanceReport(in)
	if r.OutputTax.Amount != "0.00" || r.InputTax.Amount != "0.00" || hasIssue(r, "received_needs_review") {
		t.Fatalf("small business must not show VAT: %+v", r)
	}
	if r.BankBalance.Available || r.Available.Available || !hasIssue(r, "missing_balance") || !hasIssue(r, "missing_reserve_rate") {
		t.Fatal("missing balance and reserve rate must be reported, not assumed")
	}
}

func TestFinanceReportReadsIssuedInvoiceXML(t *testing.T) {
	s, err := NewInvoiceService(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	t.Setenv("E_RECHNUNG_STUDIO_EXPORT_DIR", t.TempDir())
	if _, err = s.SaveCompany(testCompany()); err != nil {
		t.Fatal(err)
	}
	draft := testDraft()
	draft.Number = ""
	draft.Lines = append(draft.Lines, model.InvoiceLine{ID: "2", Name: "Fachbuch", Quantity: "1", Unit: "C62", UnitPrice: "100.00", TaxRate: "7", TaxCategory: "S"})
	issued, err := s.IssueInvoice(draft)
	if err != nil {
		t.Fatal(err)
	}
	// 345 € gross: 200 € at 19 % and 100 € at 7 %. Pay 40 % in October.
	if _, err = s.RecordPayment(store.Payment{InvoiceID: issued.Draft.ID, Direction: "outgoing", Amount: "138.00", Currency: "EUR", Date: "2026-10-05", Source: "manual", Reason: "Teilzahlung"}); err != nil {
		t.Fatal(err)
	}
	soll, err := s.FinanceReport("2026-09-01", "2026-09-30")
	if err != nil {
		t.Fatal(err)
	}
	if soll.OutputTax.Amount != "45.00" || len(soll.OutputTaxRates) != 2 || soll.OutputTaxRates[0].Tax != "38.00" || soll.OutputTaxRates[1].Tax != "7.00" {
		t.Fatalf("soll %+v %+v", soll.OutputTax, soll.OutputTaxRates)
	}
	if soll.Receivables.Amount != "207.00" {
		t.Fatalf("receivables %+v", soll.Receivables)
	}
	if _, err = s.SaveFinanceSettings(store.FinanceSettings{TaxBasis: "ist", ReservePercent: "25,5"}); err != nil {
		t.Fatal(err)
	}
	ist, err := s.FinanceReport("2026-10-01", "2026-10-31")
	if err != nil {
		t.Fatal(err)
	}
	if ist.TaxBasis != "ist" || ist.Inflows.Amount != "138.00" || ist.OutputTax.Amount != "18.00" {
		t.Fatalf("ist %s inflows %s tax %s", ist.TaxBasis, ist.Inflows.Amount, ist.OutputTax.Amount)
	}
	if _, err = s.FinanceReport("2026-10-31", "2026-10-01"); err == nil {
		t.Fatal("reversed period accepted")
	}

	// A full credit note cancels the invoice's output tax.
	if _, err = s.SaveFinanceSettings(store.FinanceSettings{TaxBasis: "soll"}); err != nil {
		t.Fatal(err)
	}
	correction, err := s.CreateCorrectionDraft(issued.Draft.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.IssueInvoice(correction.Draft); err != nil {
		t.Fatal(err)
	}
	year, err := s.FinanceReport("2026-01-01", "2026-12-31")
	if err != nil {
		t.Fatal(err)
	}
	if year.OutputTax.Amount != "0.00" || len(year.OutputTax.Entries) != 2 || year.OutputTax.Entries[1].Amount != "-45.00" {
		t.Fatalf("credit note %+v", year.OutputTax)
	}
}

func TestFinanceSettingsValidationHistoryAndBackup(t *testing.T) {
	s, err := NewInvoiceService(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for _, bad := range []store.FinanceSettings{
		{TaxBasis: "monat"},
		{TaxBasis: "soll", ReservePercent: "120"},
		{TaxBasis: "soll", Balances: []store.AccountBalance{{AccountID: "A", Amount: "1,234", Date: "2026-10-01"}}},
		{TaxBasis: "soll", Balances: []store.AccountBalance{{AccountID: "A", Amount: "1", Date: "2026-10-01"}, {AccountID: "A", Amount: "2", Date: "2026-10-01"}}},
		{TaxBasis: "soll", Balances: []store.AccountBalance{{AccountID: "A", Amount: "1", Date: ""}}},
		{TaxBasis: "soll", Activity: "gmbh"},
	} {
		if _, err = s.SaveFinanceSettings(bad); err == nil {
			t.Fatalf("accepted %+v", bad)
		}
	}
	saved, err := s.SaveFinanceSettings(store.FinanceSettings{TaxBasis: "ist", ReservePercent: "30", Balances: []store.AccountBalance{{AccountID: "DE1", Amount: "-12,5", Date: "2026-10-01"}}})
	if err != nil || saved.Balances[0].Amount != "-12.50" {
		t.Fatal(saved, err)
	}
	backup, err := s.ExportBackup()
	if err != nil || !strings.Contains(backup, "finance-settings-history:") {
		t.Fatal("finance settings and their history must be part of the backup", err)
	}
	if _, err = s.SaveFinanceSettings(store.FinanceSettings{TaxBasis: "soll"}); err != nil {
		t.Fatal(err)
	}
	if err = s.RestoreBackup(backup); err != nil {
		t.Fatal(err)
	}
	restored, err := s.FinanceSettings()
	if err != nil || restored.TaxBasis != "ist" || len(restored.Balances) != 1 {
		t.Fatal(restored, err)
	}
}

func TestBankBalanceWithManualFallback(t *testing.T) {
	in := baseInput()
	in.Settings.ReservePercent = "0"
	in.AccountNames = map[string]string{"hash-1": "DE02120300000000202051", "hash-2": "DE89370400440532013000"}
	in.Bookings = []PaymentBooking{booking("b1", "2026-10-01", 0), booking("b2", "2026-10-01", 0), booking("b3", "2026-10-01", 0)}
	in.Bookings[0].AccountID, in.Bookings[1].AccountID, in.Bookings[2].AccountID = "hash-1", "hash-2", "hash-3"
	in.BankBalances = []bankBalance{
		{Account: "DE02120300000000202051", Currency: "EUR", Date: "2026-11-01", Amount: d("1000")},
		{Account: "DE89370400440532013000", Currency: "EUR", Date: "2026-10-01", Amount: d("50")},
	}
	in.Settings.Balances = []store.AccountBalance{
		// Older than the bank's value: the bank wins.
		{AccountID: "DE02 1203 0000 0000 2020 51", Amount: "900.00", Date: "2026-10-30"},
		// Newer than the bank's value: the manual entry wins.
		{AccountID: "DE89370400440532013000", Amount: "70.00", Date: "2026-11-02"},
	}
	r := buildFinanceReport(in)
	assertTraceable(t, r)
	if r.BankBalance.Amount != "1070.00" || len(r.BankBalance.Entries) != 2 {
		t.Fatalf("balance %+v", r.BankBalance)
	}
	notes := r.BankBalance.Entries[0].Note + "|" + r.BankBalance.Entries[1].Note
	if !strings.Contains(notes, "von der Bank abgerufen") || !strings.Contains(notes, "manuell erfasst") {
		t.Fatalf("source not shown: %s", notes)
	}
	// Only the account without any balance is missing, named by its ID.
	for _, i := range r.Issues {
		if i.Code == "missing_balance" && (len(i.Entries) != 1 || i.Entries[0].ID != "hash-3") {
			t.Fatalf("missing balances %+v", i.Entries)
		}
	}
}

func TestManualBalanceRollsForwardWithLaterBookings(t *testing.T) {
	in := baseInput()
	in.Today = "2026-10-08"
	in.Settings.ReservePercent = "0"
	in.Settings.Balances = []store.AccountBalance{{AccountID: "DE999", Amount: "1000.00", Date: "2026-10-01"}}
	in.Bookings = []PaymentBooking{
		booking("same-day", "2026-10-01", 99_00), // already part of the entered balance
		booking("in", "2026-10-05", 200_00),
		booking("out", "2026-10-07", -50_00),
	}
	for i := range in.Bookings {
		in.Bookings[i].AccountID = "DE999"
		in.Bookings[i].Source = "csv"
	}
	r := buildFinanceReport(in)
	assertTraceable(t, r)
	e := r.BankBalance.Entries[0]
	if r.BankBalance.Amount != "1150.00" || e.Date != "2026-10-07" || !strings.Contains(e.Note, "fortgerechnet mit 2") {
		t.Fatalf("rolled balance %+v", r.BankBalance)
	}
	if hasIssue(r, "stale_balance") {
		t.Fatal("a balance rolled forward to recent bookings is not stale")
	}
}

func TestTradeTaxIsNoIncomeTaxPayment(t *testing.T) {
	in := baseInput()
	in.Settings.ReservePercent = "30"
	in.Issued = []issuedTax{invoice19("a", "RE-1", "2026-10-05")}
	in.Bookings = []PaymentBooking{booking("gewst", "2026-10-15", -40_000), booking("est", "2026-10-10", -10_000)}
	in.Treatments = map[string]store.BookingTreatment{"gewst": {BookingID: "gewst", Kind: "tradetax"}, "est": {BookingID: "est", Kind: "tax"}}
	in.Overview = store.PaymentOverview{Payments: []store.Payment{pay("p1", "a", "outgoing", "1190.00", "2026-10-08", "")}}
	r := buildFinanceReport(in)
	assertTraceable(t, r)
	// Both stay outside income and expenses; only the Einkommensteuer reduces
	// the reserve: 30 % of 1,190 € less 100 €.
	if r.Other.Amount != "-500.00" || r.Expenses.Amount != "0.00" || r.IncomeTaxReserve.Amount != "257.00" {
		t.Fatalf("other %s expenses %s reserve %s", r.Other.Amount, r.Expenses.Amount, r.IncomeTaxReserve.Amount)
	}
	if hasIssue(r, "unassigned_bookings") {
		t.Fatal("classified booking reported as unassigned")
	}
	for _, e := range r.Other.Entries {
		if e.ID == "gewst" && e.Note != "Gewerbesteuer" {
			t.Fatalf("note %q", e.Note)
		}
	}
}

func TestTradeTaxRecordsOfTheYear(t *testing.T) {
	in := baseInput()
	in.Bookings = []PaymentBooking{booking("q4", "2025-11-15", -50_000), booking("q1", "2026-02-15", -50_000), booking("back", "2026-07-01", 12_000), booking("ust", "2026-02-10", -9_000)}
	in.Treatments = map[string]store.BookingTreatment{"q4": {Kind: "tradetax"}, "q1": {Kind: "tradetax"}, "back": {Kind: "tradetax"}, "ust": {Kind: "vat"}}
	got := tradeTaxRecords(in, 2026)
	if len(got) != 2 || got[0].Entry.ID != "q1" || got[0].Amount.String() != "-500" || got[1].Amount.String() != "120" {
		t.Fatalf("trade tax of 2026: %+v", got)
	}
}
