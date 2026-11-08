package app

import (
	"encoding/base64"
	"testing"
	"time"

	"papierleicht.de/e-rechnung-studio/internal/model"
	"papierleicht.de/e-rechnung-studio/internal/store"
	"papierleicht.de/e-rechnung-studio/internal/taxforms"
)

func selfFilingService(t *testing.T, period string) *InvoiceService {
	t.Helper()
	s, err := NewInvoiceService(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	if _, err = s.SaveCompany(testCompany()); err != nil {
		t.Fatal(err)
	}
	if _, err = s.SaveFinanceSettings(store.FinanceSettings{TaxBasis: "soll", VATPeriod: period}); err != nil {
		t.Fatal(err)
	}
	return s
}

func kz(fields []taxforms.Field, id string) string {
	for _, f := range fields {
		if f.Kz == id {
			return f.Amount
		}
	}
	return ""
}

func TestExpenseValidationAndAttachment(t *testing.T) {
	s := selfFilingService(t, "quarterly")
	base := store.Expense{Date: "2026-10-05", PaidDate: "2026-10-05", Supplier: "Bürohandel", Category: "arbeitsmittel", Gross: "119,00", TaxRate: "19", PaidWith: "cash"}
	for _, bad := range []store.Expense{
		{Date: "05.10.2026", Supplier: "x", Category: "arbeitsmittel", Gross: "1", TaxRate: "19", PaidWith: "cash"},
		{Date: "2026-10-05", Supplier: "x", Category: "unbekannt", Gross: "1", TaxRate: "19", PaidWith: "cash"},
		{Date: "2026-10-05", Supplier: "x", Category: "asset", Gross: "1000", TaxRate: "19", PaidWith: "bank"},
		{Date: "2026-10-05", Supplier: "x", Category: "edv", Gross: "-5", TaxRate: "19", PaidWith: "bank"},
	} {
		if _, err := s.SaveExpense(ExpenseUpload{Expense: bad}); err == nil {
			t.Fatalf("accepted %+v", bad)
		}
	}
	saved, err := s.SaveExpense(ExpenseUpload{Expense: base, File: base64.StdEncoding.EncodeToString([]byte("%PDF-1.4 test"))})
	if err == nil {
		t.Fatal("attachment without type accepted")
	}
	base.AttachmentName, base.AttachmentType = "beleg.pdf", "application/pdf"
	if saved, err = s.SaveExpense(ExpenseUpload{Expense: base, File: base64.StdEncoding.EncodeToString([]byte("%PDF-1.4 test"))}); err != nil {
		t.Fatal(err)
	}
	if saved.Net != "100.00" || saved.Tax != "19.00" || saved.Attachment != nil {
		t.Fatalf("saved %+v", saved)
	}
	reverse := base
	reverse.TaxRate, reverse.AttachmentName, reverse.AttachmentType = store.RateReverseChargeOther, "", ""
	if rc, err := s.SaveExpense(ExpenseUpload{Expense: reverse}); err != nil || rc.Net != "119.00" || rc.Tax != "0.00" {
		t.Fatalf("reverse charge receipt %+v %v", rc, err)
	}
	// A restaurant bill: 59.50 € drinks at 19 %, 42.80 € food at 7 %.
	mixed := reverse
	mixed.Category, mixed.Gross, mixed.TaxRate, mixed.ReducedGross = "bewirtung", "102,30", store.RateMixed, "42,80"
	if m, err := s.SaveExpense(ExpenseUpload{Expense: mixed}); err != nil || m.Net != "90.00" || m.Tax != "12.30" || m.ReducedGross != "42.80" {
		t.Fatalf("mixed receipt %+v %v", m, err)
	}
	mixed.ReducedGross = "102.30"
	if _, err := s.SaveExpense(ExpenseUpload{Expense: mixed}); err == nil {
		t.Fatal("7 % share equal to the whole amount accepted")
	}
	// Editing without a new file keeps the stored attachment.
	saved.Note = "Druckerpapier"
	if _, err = s.SaveExpense(ExpenseUpload{Expense: saved}); err != nil {
		t.Fatal(err)
	}
	if stored, _ := s.store.ExpenseAttachment(saved.ID); string(stored.Attachment) != "%PDF-1.4 test" || stored.Note != "Druckerpapier" {
		t.Fatalf("attachment lost: %+v", stored)
	}
	linked := base
	linked.BookingID, linked.AttachmentName, linked.AttachmentType = "bk-1", "", ""
	if _, err = s.SaveExpense(ExpenseUpload{Expense: linked}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.SaveExpense(ExpenseUpload{Expense: linked}); err == nil {
		t.Fatal("second receipt for one booking accepted")
	}
	backup, err := s.ExportBackup()
	if err != nil {
		t.Fatal(err)
	}
	restored := selfFilingService(t, "quarterly")
	if err = restored.RestoreBackup(backup); err != nil {
		t.Fatal(err)
	}
	if list, _ := restored.Expenses(); len(list) != 4 {
		t.Fatalf("backup lost receipts: %d", len(list))
	}
}

func TestVATReturnFromInvoicesAndReceipts(t *testing.T) {
	s := selfFilingService(t, "quarterly")
	d := testDraft() // 200 € net at 19 %, buyer in France, delivered 2026-09-09
	d.Customer.Address.Country = "DE"
	d.Number = "RE-UST-1"
	if _, err := s.IssueInvoice(d); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveExpense(ExpenseUpload{Expense: store.Expense{Date: "2026-08-01", PaidDate: "2026-08-01", Supplier: "Softwareanbieter", Category: "edv", Gross: "59.50", TaxRate: "19", PaidWith: "bank"}}); err != nil {
		t.Fatal(err)
	}
	v, err := s.VATReturn("2026-Q3")
	if err != nil {
		t.Fatal(err)
	}
	if kz(v.Return.Fields, "81") != "200" || kz(v.Return.Fields, "66") != "9.50" || v.Return.Payable != "28.50" || v.Period.Due != "2026-10-12" {
		t.Fatalf("return %+v due %s", v.Return, v.Period.Due)
	}
	if _, err = s.VATReturn("2026-05"); err == nil {
		t.Fatal("monthly period accepted for quarterly filer")
	}
	if err = s.MarkVATFiled("2026-Q3", "2026-10-08", v.Return.Payable); err != nil {
		t.Fatal(err)
	}
	periods, err := s.vatPeriods(2026, "2026-10-26")
	if err != nil || len(periods) != 4 || periods[2].Filing == nil || periods[2].Overdue || !periods[1].Overdue || periods[3].Overdue {
		t.Fatalf("periods %+v %v", periods, err)
	}
	deadlines, err := s.taxDeadlines(time.Date(2027, 1, 5, 0, 0, 0, 0, time.UTC))
	if err != nil || len(deadlines) != 1 || deadlines[0].PeriodID != "2026-Q4" || deadlines[0].Due != "2027-01-11" {
		t.Fatalf("deadlines %+v %v", deadlines, err)
	}
	// Q2 (due 10 July) is too old to remind about in late October.
	if late, _ := s.taxDeadlines(time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC)); len(late) != 0 {
		t.Fatalf("old periods listed: %+v", late)
	}
}

func TestEURWithPaymentsReceiptsAndAssets(t *testing.T) {
	s := selfFilingService(t, "monthly")
	d := testDraft()
	d.Number = "RE-EUR-1"
	issued, err := s.IssueInvoice(d)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.RecordPayment(store.Payment{InvoiceID: issued.Draft.ID, Direction: "outgoing", Amount: "238.00", Currency: "EUR", Date: "2026-09-30", Source: "manual", Reason: "Zahlung"}); err != nil {
		t.Fatal(err)
	}
	for _, x := range []store.Expense{
		{Date: "2026-09-01", PaidDate: "2026-09-01", Supplier: "Restaurant", Category: "bewirtung", Gross: "119.00", TaxRate: "19", PaidWith: "private"},
		{Date: "2026-07-10", PaidDate: "2026-07-10", Supplier: "Computerladen", Category: "asset", Years: 3, Gross: "2380.00", TaxRate: "19", PaidWith: "bank"},
		{Date: "2026-12-30", Supplier: "Unbezahlt", Category: "edv", Gross: "10.00", TaxRate: "none", PaidWith: "bank"},
	} {
		if _, err = s.SaveExpense(ExpenseUpload{Expense: x}); err != nil {
			t.Fatal(err)
		}
	}
	view, err := s.EUR(2026)
	if err != nil {
		t.Fatal(err)
	}
	all := append(append([]taxforms.Field{}, view.EUR.Income...), view.EUR.Expenses...)
	// Laptop 2,000 € net from July over 3 years: 6/36 = 333.33 €.
	want := map[string]string{"112": "200.00", "140": "38.00", "175": "70.00", "185": "399.00", "130": "333.33"}
	for id, amount := range want {
		if kz(all, id) != amount {
			t.Errorf("Kz %s = %q, want %q", id, kz(all, id), amount)
		}
	}
	if view.Deadline != "2027-08-02" || view.EUR.Profit != "-564.33" {
		t.Fatalf("deadline %s profit %s", view.Deadline, view.EUR.Profit)
	}
	unpaid := false
	for _, is := range view.EUR.Issues {
		unpaid = unpaid || is.Code == "expense_unpaid"
	}
	if !unpaid {
		t.Fatalf("unpaid receipt not reported: %+v", view.EUR.Issues)
	}
}

func hasTaxIssue(issues []taxforms.Issue, code string) bool {
	for _, i := range issues {
		if i.Code == code {
			return true
		}
	}
	return false
}

func TestCreditNotePeriodFollowsReasonAndFlagsFiledPeriod(t *testing.T) {
	s := selfFilingService(t, "quarterly")
	d := testDraft() // 200 € net at 19 %, delivered 2026-09-09
	d.Customer.Address.Country = "DE"
	d.Number = "RE-GS-1"
	issued, err := s.IssueInvoice(d)
	if err != nil {
		t.Fatal(err)
	}
	q3, err := s.VATReturn("2026-Q3")
	if err != nil || q3.Return.Payable != "38.00" {
		t.Fatalf("Q3 %+v %v", q3.Return, err)
	}
	if err = s.MarkVATFiled("2026-Q3", "2026-10-08", q3.Return.Payable); err != nil {
		t.Fatal(err)
	}
	// A price reduction in October counts in October (§ 17 Abs. 1 Satz 7 UStG).
	reduction, err := s.CreateCorrectionDraft(issued.Draft.ID)
	if err != nil {
		t.Fatal(err)
	}
	reduction.Draft.IssueDate = "2026-10-15"
	reduction.Draft.Lines[0].Quantity = "1"
	reduction.Draft.CorrectionReason = model.CorrectionChange
	if _, err = s.IssueInvoice(reduction.Draft); err != nil {
		t.Fatalf("%v %+v", err, reduction.Validation)
	}
	q3, _ = s.VATReturn("2026-Q3")
	q4, err := s.VATReturn("2026-Q4")
	if err != nil || kz(q3.Return.Fields, "81") != "200" || kz(q4.Return.Fields, "81") != "-100" || q4.Return.Payable != "-19.00" {
		t.Fatalf("Q3 %+v Q4 %+v %v", q3.Return.Fields, q4.Return.Fields, err)
	}
	if hasTaxIssue(q3.Return.Issues, "changed_after_filing") {
		t.Fatal("transmitted period flagged without a change")
	}
	// A cancelled faulty invoice keeps the period of the supply, so the
	// transmitted Q3 changes and must say so.
	replacement, err := s.CreateCorrectionDraft(issued.Draft.ID)
	if err != nil {
		t.Fatal(err)
	}
	replacement.Draft.IssueDate = "2026-10-16"
	replacement.Draft.CorrectionReason = model.CorrectionReplace
	if _, err = s.IssueInvoice(replacement.Draft); err != nil {
		t.Fatal(err)
	}
	q3, _ = s.VATReturn("2026-Q3")
	if kz(q3.Return.Fields, "81") != "0" || len(q3.Return.Issues) == 0 || q3.Return.Issues[0].Code != "changed_after_filing" {
		t.Fatalf("Q3 after replacement %+v", q3.Return)
	}
	periods, err := s.VATPeriods(2026)
	if err != nil || !periods[2].Changed || periods[1].Changed || periods[3].Changed {
		t.Fatalf("periods %+v %v", periods, err)
	}
}

func TestReceivedCreditNoteReducesInputTaxAndExpenses(t *testing.T) {
	seller := selfFilingService(t, "quarterly")
	d := testDraft()
	d.Customer.Address.Country = "DE"
	d.Number = "RE-L-1"
	issued, err := seller.IssueInvoice(d)
	if err != nil {
		t.Fatal(err)
	}
	correction, err := seller.CreateCorrectionDraft(issued.Draft.ID)
	if err != nil {
		t.Fatal(err)
	}
	correction.Draft.IssueDate = "2026-10-15"
	correction.Draft.CorrectionReason = model.CorrectionChange
	credit, err := seller.IssueInvoice(correction.Draft)
	if err != nil {
		t.Fatal(err)
	}
	buyer := selfFilingService(t, "quarterly")
	ids := map[string]string{}
	for name, id := range map[string]string{"rechnung.xml": issued.Draft.ID, "gutschrift.xml": credit.Draft.ID} {
		xml, err := seller.store.Artifact(id, "cii-xml")
		if err != nil {
			t.Fatal(err)
		}
		imported, err := buyer.ImportReceivedInvoice(name, base64.StdEncoding.EncodeToString(xml))
		if err != nil {
			t.Fatal(err)
		}
		if err = buyer.SaveReceivedCategory(store.ReceivedCategory{ReceivedID: imported.Invoice.ID, Category: "edv"}); err != nil {
			t.Fatal(err)
		}
		ids[name] = imported.Invoice.ID
	}
	for _, p := range []store.Payment{
		{InvoiceID: ids["rechnung.xml"], Direction: "incoming", Amount: "238.00", Currency: "EUR", Date: "2026-09-20", Source: "manual", Reason: "Zahlung"},
		{InvoiceID: ids["gutschrift.xml"], Direction: "incoming", Amount: "238.00", Currency: "EUR", Date: "2026-10-20", Source: "manual", Reason: "Erstattung"},
	} {
		if _, err = buyer.RecordPayment(p); err != nil {
			t.Fatal(err)
		}
	}
	q3, _ := buyer.VATReturn("2026-Q3")
	q4, err := buyer.VATReturn("2026-Q4")
	if err != nil || kz(q3.Return.Fields, "66") != "38.00" || kz(q4.Return.Fields, "66") != "-38.00" || q4.Return.Payable != "38.00" {
		t.Fatalf("input tax Q3 %+v Q4 %+v %v", q3.Return.Fields, q4.Return.Fields, err)
	}
	report, err := buyer.FinanceReport("2026-10-01", "2026-12-31")
	if err != nil || report.InputTax.Amount != "-38.00" || report.Outflows.Amount != "-238.00" {
		t.Fatalf("report input %s outflows %s %v", report.InputTax.Amount, report.Outflows.Amount, err)
	}
	eur, err := buyer.EUR(2026)
	if err != nil {
		t.Fatal(err)
	}
	if kz(eur.EUR.Expenses, "228") != "0.00" || kz(eur.EUR.Expenses, "185") != "0.00" {
		t.Fatalf("refund must cancel the expense: %+v", eur.EUR.Expenses)
	}
}

func TestSmallBusinessFilesReverseChargePeriods(t *testing.T) {
	s, err := NewInvoiceService(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	c := testCompany()
	c.SmallBusiness = true
	c.ExemptionReason = "Umsatzsteuerbefreit nach § 19 UStG"
	if _, err = s.SaveCompany(c); err != nil {
		t.Fatal(err)
	}
	if periods, err := s.VATPeriods(2026); err != nil || len(periods) != 0 {
		t.Fatalf("periods without reverse charge %+v %v", periods, err)
	}
	for _, x := range []store.Expense{
		{Date: "2026-08-05", PaidDate: "2026-08-05", Supplier: "Werbeplattform Irland", Category: "werbung", Gross: "500.00", TaxRate: store.RateReverseChargeEU, PaidWith: "bank"},
		{Date: "2026-08-06", PaidDate: "2026-08-06", Supplier: "Bürohandel", Category: "arbeitsmittel", Gross: "119.00", TaxRate: "19", PaidWith: "bank"},
	} {
		if _, err = s.SaveExpense(ExpenseUpload{Expense: x}); err != nil {
			t.Fatal(err)
		}
	}
	periods, err := s.VATPeriods(2026)
	if err != nil || len(periods) != 1 || periods[0].Period.ID != "2026-Q3" {
		t.Fatalf("periods %+v %v", periods, err)
	}
	v, err := s.VATReturn("2026-Q3")
	if err != nil || kz(v.Return.Fields, "46") != "500" || kz(v.Return.Fields, "47") != "95.00" || kz(v.Return.Fields, "67") != "" || kz(v.Return.Fields, "66") != "" || v.Return.Payable != "95.00" {
		t.Fatalf("return %+v %v", v.Return, err)
	}
	deadlines, err := s.taxDeadlines(time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC))
	if err != nil || len(deadlines) != 1 || deadlines[0].PeriodID != "2026-Q3" || deadlines[0].Due != "2026-10-12" {
		t.Fatalf("deadlines %+v %v", deadlines, err)
	}
	eur, err := s.EUR(2026)
	if err != nil || kz(eur.EUR.Expenses, "224") != "500.00" || kz(eur.EUR.Expenses, "229") != "119.00" {
		t.Fatalf("small business expenses %+v %v", eur.EUR.Expenses, err)
	}
}

func TestTenDayRule(t *testing.T) {
	if !inYearWithTenDays("2027-01-08", 2026, true) || inYearWithTenDays("2026-01-08", 2026, true) || !inYearWithTenDays("2026-06-01", 2026, true) || inYearWithTenDays("2027-01-11", 2026, true) {
		t.Fatal("10-day rule")
	}
	if inYearWithTenDays("2027-01-08", 2026, false) || !inYearWithTenDays("2026-01-08", 2026, false) {
		t.Fatal("without a return due on 10 January payments count by date")
	}
	// With Dauerfristverlängerung the fourth quarter is due in February
	// (BFH X R 2/21); monthly filers then pay November on 10 January.
	if tenDayRuleApplies("quarterly", true) || !tenDayRuleApplies("quarterly", false) || !tenDayRuleApplies("monthly", true) || tenDayRuleApplies("yearly", false) {
		t.Fatal("ten-day rule must follow the due date")
	}
}

func TestEURWarnsAboutGapsAndNotesTenDayRule(t *testing.T) {
	s := selfFilingService(t, "quarterly")
	d := testDraft()
	d.Customer.Address.Country = "DE"
	d.Number = "RE-OFFEN-1"
	if _, err := s.IssueInvoice(d); err != nil {
		t.Fatal(err)
	}
	in, err := s.loadFinanceInput("2026-01-01", "2026-12-31")
	if err != nil {
		t.Fatal(err)
	}
	in.Bookings = []PaymentBooking{booking("unknown", "2026-05-04", -4_200), booking("ust", "2027-01-08", -19_000), booking("ust-late", "2027-01-08", -5_000)}
	in.Treatments = map[string]store.BookingTreatment{"ust": {BookingID: "ust", Kind: "vat"}}
	_, vat, _, _, issues, err := s.eurRecords(in, 2026)
	if err != nil {
		t.Fatal(err)
	}
	codes := map[string]bool{}
	for _, i := range issues {
		codes[i.Code] = true
	}
	if !codes["issued_open"] || !codes["unassigned_bookings"] {
		t.Fatalf("open invoice and unassigned booking must be named: %+v", issues)
	}
	if len(vat) != 1 || vat[0].Entry.Note != "10-Tage-Regel: zählt für 2026" {
		t.Fatalf("payment in the ten days: %+v", vat)
	}
	// Quarterly with Dauerfristverlängerung: Q4 is due in February, so the
	// January payment counts for 2027.
	in.Settings.VATExtension = true
	if _, vat, _, _, _, _ = s.eurRecords(in, 2026); len(vat) != 0 {
		t.Fatalf("ten-day rule applied despite extension: %+v", vat)
	}
}

func TestSmallBusinessLimits(t *testing.T) {
	in := baseInput()
	in.SmallBusiness = true
	small := issuedTax{ID: "a", Number: "RE-1", Partner: "Kunde", Currency: "EUR", IssueDate: "2026-02-01", Gross: d("60000"), Rates: []taxRate{{Category: "E", Basis: d("60000"), Amount: d("0")}}}
	abroad := issuedTax{ID: "b", Number: "RE-2", Partner: "US Inc.", Currency: "EUR", IssueDate: "2026-03-01", BuyerCountry: "US", Gross: d("50000"), Rates: []taxRate{{Category: "O", Basis: d("50000"), Amount: d("0")}}}
	in.Issued = []issuedTax{small, abroad}
	in.Overview.Payments = []store.Payment{pay("p1", "a", "outgoing", "30000.00", "2026-03-10", ""), pay("p2", "b", "outgoing", "50000.00", "2026-03-11", "")}
	st := smallBusinessStatus(in, 2026)
	// Services abroad do not count: 30,000 € is over 25,000 € but under 100,000 €.
	if st.Turnover != "30000.00" || st.State != "next_year" || st.PreviousTurnover != "" {
		t.Fatalf("status %+v", st)
	}
	in.Overview.Payments = append(in.Overview.Payments, pay("p3", "a", "outgoing", "30000.00", "2026-06-01", ""))
	in.Issued = append(in.Issued, issuedTax{ID: "c", Number: "RE-3", Partner: "Kunde", Currency: "EUR", IssueDate: "2026-07-01", Gross: d("50000"), Rates: []taxRate{{Category: "E", Basis: d("50000"), Amount: d("0")}}})
	in.Overview.Payments = append(in.Overview.Payments, pay("p4", "c", "outgoing", "50000.00", "2026-07-15", ""))
	if st = smallBusinessStatus(in, 2026); st.State != "exceeded" || st.Crossed == nil || st.Crossed.ID != "p4" {
		t.Fatalf("crossing 100,000 € %+v", st)
	}
	if next := smallBusinessStatus(in, 2027); next.State != "previous_exceeded" || next.PreviousTurnover != "110000.00" {
		t.Fatalf("following year %+v", next)
	}
}

func TestIstVersteuerungKeepsIntraCommunityInInvoicePeriod(t *testing.T) {
	in := baseInput()
	in.Settings.TaxBasis = "ist"
	ic := issuedTax{ID: "ic", Number: "RE-EU", Partner: "Wien KG", Currency: "EUR", TaxDate: "2026-09-20", IssueDate: "2026-09-20", BuyerCountry: "AT", Gross: d("1000"), Rates: []taxRate{{Category: "K", Basis: d("1000"), Amount: d("0")}}}
	in.Issued = []issuedTax{ic, invoice19("de", "RE-DE", "2026-09-21")}
	in.Overview.Payments = []store.Payment{pay("p1", "ic", "outgoing", "1000.00", "2026-10-05", ""), pay("p2", "de", "outgoing", "1190.00", "2026-10-06", "")}
	in.From, in.To = "2026-10-01", "2026-12-31"
	sales, _, _ := vatRecords(in)
	r := taxforms.BuildVATReturn(sales, nil, d("0"))
	if kz(r.Fields, "81") != "1000" || kz(r.Fields, "41") != "" {
		t.Fatalf("Q4 under Ist: %+v", r.Fields)
	}
	in.From, in.To = "2026-07-01", "2026-09-30"
	sales, _, _ = vatRecords(in)
	if r = taxforms.BuildVATReturn(sales, nil, d("0")); kz(r.Fields, "41") != "1000" || kz(r.Fields, "81") != "" {
		t.Fatalf("Q3 under Ist: %+v", r.Fields)
	}
}

func TestZMFromIntraCommunityInvoices(t *testing.T) {
	s := selfFilingService(t, "quarterly")
	goods := testDraft() // French customer with VAT ID, delivered 2026-09-09
	goods.Number, goods.TaxTreatment = "RE-ZM-1", "intra-community"
	services := testDraft()
	services.Number, services.TaxTreatment = "RE-ZM-2", "reverse-charge"
	for _, d := range []model.InvoiceDraft{goods, services} {
		if v, err := s.IssueInvoice(d); err != nil {
			t.Fatalf("%v %+v", err, v.Validation)
		}
	}
	periods, err := s.ZMPeriods(2026)
	if err != nil || len(periods) != 1 || periods[0].Period.ID != "2026-Q3" || periods[0].Period.Due != "2026-10-26" {
		t.Fatalf("periods %+v %v", periods, err)
	}
	zm, err := s.ZMReturn("2026-Q3")
	if err != nil || len(zm.Report.Lines) != 2 || zm.Report.Total != "400" {
		t.Fatalf("report %+v %v", zm.Report, err)
	}
	if l := zm.Report.Lines[0]; l.VATID != "FR40303265045" || l.Services || l.Amount != "200" || !zm.Report.Lines[1].Services {
		t.Fatalf("lines %+v", zm.Report.Lines)
	}
	if err = s.MarkZMFiled("2026-Q3", "2026-10-20", "300"); err != nil {
		t.Fatal(err)
	}
	if periods, _ = s.ZMPeriods(2026); !periods[0].Changed {
		t.Fatalf("changed total not flagged: %+v", periods)
	}
	deadlines, err := s.taxDeadlines(time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	for _, dl := range deadlines {
		if dl.Kind == "zm" {
			t.Fatalf("transmitted ZM still listed: %+v", deadlines)
		}
	}
	if err = s.UnmarkZMFiled("2026-Q3"); err != nil {
		t.Fatal(err)
	}
	if deadlines, _ = s.taxDeadlines(time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC)); len(deadlines) != 2 || deadlines[1].Task != "zm:2026-Q3" {
		t.Fatalf("deadlines %+v", deadlines)
	}
}

func TestTaxOverviewBundlesTheYear(t *testing.T) {
	s := selfFilingService(t, "quarterly")
	d := testDraft() // French customer with VAT ID, delivered 2026-09-09
	d.Number, d.TaxTreatment = "RE-OV-1", "intra-community"
	if _, err := s.IssueInvoice(d); err != nil {
		t.Fatal(err)
	}
	if err := s.MarkVATFiled("2026-Q3", "2026-10-08", "0.00"); err != nil {
		t.Fatal(err)
	}
	o, err := s.taxOverview(2026, "2026-10-28")
	if err != nil {
		t.Fatal(err)
	}
	if len(o.VAT) != 8 || len(o.ZM) != 1 || len(o.Annual) != 2 || o.SmallBusiness != nil || o.Setup.Filing != "self" || o.Since != "2026-09-09" {
		t.Fatalf("overview %+v", o)
	}
	// Q3 holds the delivery in Kz 41 only, so the transmitted 0 € still matches.
	if q3 := o.VAT[6]; q3.Period.ID != "2026-Q3" || q3.Filing == nil || q3.Changed {
		t.Fatalf("Q3 %+v", q3)
	}
	if o.ZM[0].Period.ID != "2026-Q3" || !o.ZM[0].Overdue || o.Annual[1].Due != "2027-08-02" || o.Annual[0].Profit != "0.00" {
		t.Fatalf("zm %+v annual %+v", o.ZM, o.Annual)
	}
}

func TestAnnualFilingNotesAndAdvisorDeadline(t *testing.T) {
	s := selfFilingService(t, "quarterly")
	if err := s.MarkAnnualFiled(2025, "2026-07-20", "0.00"); err != nil {
		t.Fatal(err)
	}
	view, err := s.EUR(2025)
	if err != nil || view.Filing == nil || view.Changed || len(view.Questions) == 0 {
		t.Fatalf("view %+v %v", view, err)
	}
	if _, err = s.SaveFinanceSettings(store.FinanceSettings{TaxBasis: "soll", VATPeriod: "quarterly", Filing: "advisor"}); err != nil {
		t.Fatal(err)
	}
	if view, _ = s.EUR(2025); view.Deadline != "2027-03-01" {
		t.Fatalf("advisor deadline %s", view.Deadline)
	}
	if _, err = s.SaveFinanceSettings(store.FinanceSettings{TaxBasis: "soll", Filing: "jemand"}); err == nil {
		t.Fatal("unknown filing accepted")
	}
}

func TestActivityInTaxSetupAndQuestions(t *testing.T) {
	s := selfFilingService(t, "quarterly")
	settings, err := s.FinanceSettings()
	if err != nil {
		t.Fatal(err)
	}
	asked := func(activity string) bool {
		for _, q := range eurQuestions(2026, false, activity) {
			if q.ID == "tradetax" {
				return true
			}
		}
		return false
	}
	for _, activity := range []string{"", "trade", "freelance"} {
		settings.Activity = activity
		if _, err = s.SaveFinanceSettings(settings); err != nil {
			t.Fatal(err)
		}
		setup, err := s.TaxSetup()
		if err != nil || setup.Activity != activity {
			t.Fatalf("activity %q: %+v %v", activity, setup, err)
		}
		// A freelance business pays no Gewerbesteuer, so the EÜR does not ask.
		if asked(activity) != (activity != "freelance") {
			t.Fatalf("Gewerbesteuer question for %q", activity)
		}
	}
}
