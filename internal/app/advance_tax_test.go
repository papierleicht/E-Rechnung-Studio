package app

import (
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"papierleicht.de/e-rechnung-studio/internal/catalog"
	"papierleicht.de/e-rechnung-studio/internal/invoicexml"
	"papierleicht.de/e-rechnung-studio/internal/store"
	"papierleicht.de/e-rechnung-studio/internal/taxforms"
)

// advanceInvoice is the example from the bug report: 3,000 € net plus 570 € tax
// on an invoice of 1 March for a supply in June.
func advanceInvoice() issuedTax {
	return issuedTax{ID: "a", Number: "AN-1", Partner: "Kunde", Currency: "EUR", TaxDate: "2026-06-15", IssueDate: "2026-03-01", Advance: true, Gross: d("3570"),
		Rates: []taxRate{{Category: "S", Percent: d("19"), Basis: d("3000"), Amount: d("570")}}}
}

// restInvoice is the remaining invoice of the same example.
func restInvoice() issuedTax {
	return issuedTax{ID: "r", Number: "RE-2", Partner: "Kunde", Currency: "EUR", TaxDate: "2026-06-15", IssueDate: "2026-06-20", Gross: d("8330"),
		Rates: []taxRate{{Category: "S", Percent: d("19"), Basis: d("7000"), Amount: d("1330")}}}
}

func advanceInput(payments ...store.Payment) financeInput {
	in := baseInput()
	in.Issued = []issuedTax{advanceInvoice(), restInvoice()}
	in.Overview = store.PaymentOverview{Payments: payments}
	return in
}

func month(id string) taxforms.Period {
	from, to, _, _ := store.FilingPeriod(id)
	return taxforms.Period{ID: id, From: from, To: to}
}

// assertTax compares Finanzübersicht and Voranmeldung of one month.
func assertTax(t *testing.T, in financeInput, id, want string) {
	t.Helper()
	p := month(id)
	in.From, in.To = p.From, p.To
	r := buildFinanceReport(in)
	assertTraceable(t, r)
	if !d(r.OutputTax.Amount).Equal(d(want)) {
		t.Errorf("Finanzübersicht %s: %s, erwartet %s", id, r.OutputTax.Amount, want)
	}
	v := buildVATReturn(in, p, taxSetup(in.Settings, in.SmallBusiness))
	if !d(v.Payable).Equal(d(want)) {
		t.Errorf("Voranmeldung %s: %s, erwartet %s", id, v.Payable, want)
	}
}

func openIssue(in financeInput, id string) *taxforms.Issue {
	p := month(id)
	in.From, in.To = p.From, p.To
	for _, i := range buildVATReturn(in, p, taxSetup(in.Settings, in.SmallBusiness)).Issues {
		if i.Code == "issued_advance_open" {
			return &i
		}
	}
	return nil
}

func TestAdvanceTaxFollowsThePayment(t *testing.T) {
	in := advanceInput(pay("p", "a", "outgoing", "3570.00", "2026-03-10", ""))
	assertTax(t, in, "2026-03", "570")
	// The remaining invoice stays in the period of the supply, alone.
	assertTax(t, in, "2026-06", "1330")
}

func TestAdvanceIgnoresTheInvoiceDate(t *testing.T) {
	in := advanceInput(pay("p", "a", "outgoing", "3570.00", "2026-04-02", ""))
	assertTax(t, in, "2026-03", "0")
	assertTax(t, in, "2026-04", "570")
}

func TestAdvancePartialPaymentsCountTheirShare(t *testing.T) {
	in := advanceInput(pay("p1", "a", "outgoing", "1190.00", "2026-03-10", ""), pay("p2", "a", "outgoing", "2380.00", "2026-05-10", ""))
	assertTax(t, in, "2026-03", "190")
	assertTax(t, in, "2026-05", "380")
	if issue := openIssue(in, "2026-05"); issue != nil {
		t.Fatalf("fully paid invoice still open: %+v", issue)
	}
	in = advanceInput(pay("p1", "a", "outgoing", "1190.00", "2026-03-10", ""))
	if issue := openIssue(in, "2026-03"); issue == nil || !strings.Contains(issue.Entries[0].Note, "2.380,00") {
		t.Fatalf("open part after the first payment: %+v", issue)
	}
}

func TestAdvanceCentsAddUpOverPartialPayments(t *testing.T) {
	in := advanceInput(pay("p1", "a", "outgoing", "1000.01", "2026-03-10", ""), pay("p2", "a", "outgoing", "1000.01", "2026-04-10", ""), pay("p3", "a", "outgoing", "1569.98", "2026-05-10", ""))
	sum := decimal.Zero
	for _, id := range []string{"2026-03", "2026-04", "2026-05"} {
		p := month(id)
		in.From, in.To = p.From, p.To
		sum = sum.Add(d(buildFinanceReport(in).OutputTax.Amount))
	}
	if !sum.Equal(d("570")) {
		t.Fatalf("partial tax adds up to %s", sum)
	}
}

func TestUnpaidAdvanceReportsNoTaxButStaysVisible(t *testing.T) {
	in := advanceInput()
	assertTax(t, in, "2026-03", "0")
	assertTax(t, in, "2026-06", "1330")
	issue := openIssue(in, "2026-03")
	if issue == nil || len(issue.Entries) != 1 || !strings.Contains(issue.Entries[0].Note, "3.570,00") {
		t.Fatalf("open advance: %+v", issue)
	}
	in.From, in.To = "2026-03-01", "2026-03-31"
	r := buildFinanceReport(in)
	if !hasIssue(r, "issued_advance_open") {
		t.Fatal("open advance missing from the finance report")
	}
	if r.OutputTax.Incomplete {
		t.Fatal("an open advance leaves no gap in the figures")
	}
	in.From, in.To = "2026-02-01", "2026-02-28"
	if hasIssue(buildFinanceReport(in), "issued_advance_open") {
		t.Fatal("advance reported before it was issued")
	}
}

func TestLaterPaymentClearsTheOpenPointOfThePastPeriod(t *testing.T) {
	in := advanceInput(pay("p", "a", "outgoing", "1785.00", "2026-04-10", ""))
	assertTax(t, in, "2026-03", "0")
	issue := openIssue(in, "2026-03")
	if issue == nil || !strings.Contains(issue.Entries[0].Note, "1.785,00") {
		t.Fatalf("a part payment leaves the rest open: %+v", issue)
	}
	in = advanceInput(pay("p", "a", "outgoing", "3570.00", "2026-04-10", ""))
	assertTax(t, in, "2026-03", "0")
	if issue := openIssue(in, "2026-03"); issue != nil {
		t.Fatalf("paid in April, still open in March: %+v", issue)
	}
	in.From, in.To = "2026-03-01", "2026-03-31"
	if hasIssue(buildFinanceReport(in), "issued_advance_open") {
		t.Fatal("paid advance still open in the finance report")
	}
}

func TestAdvanceOverpaymentCountsOnlyTheInvoice(t *testing.T) {
	in := advanceInput(pay("p", "a", "outgoing", "4000.00", "2026-03-10", ""))
	assertTax(t, in, "2026-03", "570")
	in.From, in.To = "2026-03-01", "2026-03-31"
	if r := buildFinanceReport(in); !hasIssue(r, "issued_advance_overpaid") {
		t.Fatalf("overpayment not reported: %+v", r.Issues)
	}
	// Sending the excess back takes no tax back.
	in = advanceInput(pay("p", "a", "outgoing", "4000.00", "2026-03-10", ""), pay("b", "a", "outgoing", "-430.00", "2026-04-10", ""))
	assertTax(t, in, "2026-03", "570")
	assertTax(t, in, "2026-04", "0")
}

func TestAdvanceRefundReducesTheTaxWhenRepaid(t *testing.T) {
	in := advanceInput(pay("p", "a", "outgoing", "3570.00", "2026-03-10", ""), pay("b", "a", "outgoing", "-1190.00", "2026-05-20", ""))
	assertTax(t, in, "2026-03", "570")
	assertTax(t, in, "2026-05", "-190")
}

func TestAdvanceCreditNoteFollowsThePayout(t *testing.T) {
	credit := advanceInvoice()
	credit.ID, credit.Number, credit.Credit, credit.Correction, credit.CorrectsID, credit.IssueDate = "c", "GS-1", true, true, "a", "2026-05-02"
	credit.Gross = d("-3570")
	credit.Rates = []taxRate{{Category: "S", Percent: d("19"), Basis: d("-3000"), Amount: d("-570")}}

	// Nobody paid: the credit note closes the advance and changes no tax.
	in := advanceInput()
	in.Issued = append(in.Issued, credit)
	applyCorrectionDates(in.Issued)
	assertTax(t, in, "2026-05", "0")
	if issue := openIssue(in, "2026-05"); issue != nil {
		t.Fatalf("cancelled advance still open: %+v", issue)
	}

	// Paid: the tax goes back in the month of the payout, not of the credit note.
	in.Overview.Payments = []store.Payment{pay("p", "a", "outgoing", "3570.00", "2026-03-10", ""), pay("o", "c", "outgoing", "3570.00", "2026-06-05", "")}
	assertTax(t, in, "2026-03", "570")
	assertTax(t, in, "2026-05", "0")
	assertTax(t, in, "2026-06", "760")
	if issue := openIssue(in, "2026-05"); issue != nil {
		t.Fatalf("payout made, still outstanding: %+v", issue)
	}
	in.Overview.Payments = in.Overview.Payments[:1]
	if issue := openIssue(in, "2026-05"); issue == nil || issue.Entries[0].ID != "c" {
		t.Fatalf("outstanding payout: %+v", issue)
	}
}

func TestRevokedAdvancePaymentCountsNoMore(t *testing.T) {
	revoked := pay("p", "a", "outgoing", "3570.00", "2026-03-10", "")
	revoked.RevokedAt = "2026-04-01T10:00:00Z"
	in := advanceInput(revoked)
	assertTax(t, in, "2026-03", "0")
	if openIssue(in, "2026-03") == nil {
		t.Fatal("advance without a live payment must be open again")
	}
}

func TestAdvanceChangesAfterClosingAreReported(t *testing.T) {
	late := pay("p", "a", "outgoing", "3570.00", "2026-03-10", "")
	late.CreatedAt = "2026-05-01T10:00:00Z"
	in := advanceInput(late)
	in.Locks = []store.PeriodLock{{ID: "l", From: "2026-03-01", To: "2026-03-31", At: "2026-04-15T10:00:00Z"}}
	in.From, in.To = "2026-03-01", "2026-03-31"
	if r := buildFinanceReport(in); !hasIssue(r, "issued_advance_late") {
		t.Fatalf("late payment not reported: %+v", r.Issues)
	}
	// The tax stays with the date of the payment.
	assertTax(t, in, "2026-03", "570")

	before := late
	before.CreatedAt = "2026-04-01T10:00:00Z"
	in.Overview.Payments = []store.Payment{before}
	if r := buildFinanceReport(in); hasIssue(r, "issued_advance_late") {
		t.Fatal("payment recorded before the closing reported as late")
	}
	before.RevokedAt = "2026-05-02T10:00:00Z"
	in.Overview.Payments = []store.Payment{before}
	if r := buildFinanceReport(in); !hasIssue(r, "issued_advance_late") {
		t.Fatal("revocation after the closing not reported")
	}
}

func TestAdvancePaymentChangesATransmittedPeriod(t *testing.T) {
	in := advanceInput(pay("p", "a", "outgoing", "3570.00", "2026-03-10", ""))
	filing := store.VATFiling{PeriodID: "2026-03", Payable: "0.00"}
	if !payableChanged(filing, buildVATReturn(in, month("2026-03"), TaxSetup{}).Payable) {
		t.Fatal("a payment in a transmitted period must show as a change")
	}
}

func TestAdvanceChangesNothingUnderIstAndForSmallBusinesses(t *testing.T) {
	marked := advanceInput(pay("p", "a", "outgoing", "1190.00", "2026-03-10", ""))
	marked.Settings.TaxBasis = "ist"
	plain := marked
	plain.Issued = []issuedTax{advanceInvoice(), restInvoice()}
	plain.Issued[0].Advance = false
	for _, id := range []string{"2026-03", "2026-06"} {
		p := month(id)
		marked.From, marked.To, plain.From, plain.To = p.From, p.To, p.From, p.To
		if a, b := buildFinanceReport(marked).OutputTax.Amount, buildFinanceReport(plain).OutputTax.Amount; a != b {
			t.Errorf("Ist %s: %s, ohne Kennzeichnung %s", id, a, b)
		}
		if a, b := buildVATReturn(marked, p, TaxSetup{}).Payable, buildVATReturn(plain, p, TaxSetup{}).Payable; a != b {
			t.Errorf("Ist-Voranmeldung %s: %s, ohne Kennzeichnung %s", id, a, b)
		}
	}
	assertTax(t, marked, "2026-03", "190")
	if openIssue(marked, "2026-03") != nil {
		t.Fatal("Ist-Versteuerung has no open advances")
	}

	small := advanceInput(pay("p", "a", "outgoing", "3570.00", "2026-03-10", ""))
	small.SmallBusiness = true
	assertTax(t, small, "2026-03", "0")
	if openIssue(advanceSmall(), "2026-03") != nil {
		t.Fatal("small businesses report no advances")
	}
}

func advanceSmall() financeInput {
	in := advanceInput()
	in.SmallBusiness = true
	return in
}

func TestAdvanceServiceInOtherMemberStateKeepsTheDateOfTheZM(t *testing.T) {
	v := advanceInvoice()
	v.BuyerCountry, v.BuyerVATID = "AT", "ATU12345678"
	v.Rates = []taxRate{{Category: "AE", Basis: d("3000"), Amount: d("0")}}
	v.Gross = d("3000")
	in := advanceInput(pay("p", "a", "outgoing", "3000.00", "2026-03-10", ""))
	in.Issued = []issuedTax{v}
	sales := zmSales(in)
	if len(sales) != 1 || sales[0].Entry.Date != "2026-06-15" {
		t.Fatalf("ZM %+v", sales)
	}
	in.From, in.To = "2026-03-01", "2026-03-31"
	if records, _, _ := vatRecords(in); len(records) != 0 {
		t.Fatalf("EU service moved with the payment: %+v", records)
	}
	in.From, in.To = "2026-06-01", "2026-06-30"
	if records, _, _ := vatRecords(in); len(records) != 1 {
		t.Fatalf("EU service missing at the date of the invoice: %+v", records)
	}
}

func advanceDatevInput(payments ...store.Payment) datevInput {
	in := datevCases("SKR03")
	in.TaxBasis = "soll"
	advance := issuedDoc("a", "AN-1", "2026-03-01", tax("S", "19", "3000", "570"))
	advance.Advance = true
	in.Issued = []datevDoc{advance, issuedDoc("r", "RE-2", "2026-06-20", tax("S", "19", "7000", "1330"))}
	in.Received = nil
	in.Payments = payments
	return in
}

func datevGross(b datevBuild) decimal.Decimal {
	sum := decimal.Zero
	for _, row := range b.Bookings {
		if row.DocKind != "payment" {
			amount := row.Amount
			if row.Side == "H" {
				amount = amount.Neg()
			}
			sum = sum.Add(amount)
		}
	}
	return sum
}

func TestDatevBooksAdvancesWithThePaymentLikeTheVoranmeldung(t *testing.T) {
	cases := []struct {
		name     string
		payments []store.Payment
		months   map[string]string // net plus tax booked in each month
	}{
		{"voll im März", []store.Payment{pay("p", "a", "outgoing", "3570.00", "2026-03-10", "")}, map[string]string{"2026-03": "3570", "2026-04": "0"}},
		{"Teilzahlung", []store.Payment{pay("p", "a", "outgoing", "1190.00", "2026-03-10", "")}, map[string]string{"2026-03": "1190"}},
		{"Überzahlung", []store.Payment{pay("p", "a", "outgoing", "4000.00", "2026-03-10", "")}, map[string]string{"2026-03": "3570"}},
		{"Erstattung", []store.Payment{pay("p", "a", "outgoing", "3570.00", "2026-03-10", ""), pay("b", "a", "outgoing", "-1190.00", "2026-05-20", "")}, map[string]string{"2026-03": "3570", "2026-05": "-1190"}},
		{"unbezahlt", nil, map[string]string{"2026-03": "0"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			for id, want := range c.months {
				p := month(id)
				in := advanceDatevInput(c.payments...)
				in.From, in.To = p.From, p.To
				b := buildDatevExport(in)
				if got := datevGross(b); !got.Equal(d(want)) {
					t.Errorf("DATEV %s: %s, erwartet %s", id, got, want)
				}
				finance := advanceInput(c.payments...)
				tax := buildVATReturn(finance, p, TaxSetup{})
				if taxOf := d(want).Mul(d("19")).Div(d("119")).Round(2); !d(tax.Payable).Equal(taxOf) {
					t.Errorf("Voranmeldung %s: %s, DATEV enthält %s Steuer", id, tax.Payable, taxOf)
				}
			}
		})
	}
	in := advanceDatevInput(pay("p", "a", "outgoing", "1190.00", "2026-03-10", ""))
	in.From, in.To = "2026-03-01", "2026-03-31"
	b := buildDatevExport(in)
	if b.Bookings[0].DocKind != "issued" || b.Bookings[0].Date.Format(time.DateOnly) != "2026-03-10" {
		t.Fatalf("booking %+v", b.Bookings)
	}
	if datevIssue(b.Preview, "issued_advance_open") == nil || datevIssue(b.Preview, "advance_booked") == nil {
		t.Fatalf("missing checklist items: %+v", b.Preview.Issues)
	}
}

func TestDatevLeavesAdvancesAloneUnderIstAndForSmallBusinesses(t *testing.T) {
	in := advanceDatevInput()
	in.TaxBasis = "ist"
	in.From, in.To = "2026-03-01", "2026-03-31"
	if got := datevGross(buildDatevExport(in)); !got.Equal(d("3570")) {
		t.Fatalf("Ist: %s", got)
	}
	in.TaxBasis, in.SmallBusiness = "soll", true
	if got := datevGross(buildDatevExport(in)); !got.Equal(d("3570")) {
		t.Fatalf("small business: %s", got)
	}
}

func TestAdvanceInvoiceTypePassesTheChecks(t *testing.T) {
	s := receivedService(t)
	for _, format := range catalog.Formats() {
		profile := format
		draft := testDraft()
		draft.AdvancePayment = true
		draft.Notes = "Eigener Hinweis"
		draft.Profile, draft.Syntax = profile.ID, profile.Syntax
		draft.BuyerReference, draft.Customer.Endpoint, draft.Customer.EndpointScheme = "04011000-12345-34", "kunde@example.org", "EM"
		view, err := s.ValidateInvoice(draft)
		if err != nil {
			t.Fatalf("%s: %v", profile.ID, err)
		}
		if view.Validation.State != "valid" {
			t.Errorf("%s: %+v", profile.ID, view.Validation)
		}
		inv, err := invoicexml.Parse([]byte(view.XML))
		if err != nil {
			t.Fatalf("%s: %v", profile.ID, err)
		}
		want := 386
		if profile.XRechnung {
			want = 380
		}
		if int(inv.InvoiceTypeCode) != want {
			t.Errorf("%s: type %d, want %d", profile.ID, inv.InvoiceTypeCode, want)
		}
		if !strings.Contains(view.PreviewHTML, "Anzahlungsrechnung") {
			t.Errorf("%s: document does not say it is an advance", profile.ID)
		}
		if !strings.Contains(view.XML, "Eigener Hinweis") {
			t.Errorf("%s: the user's note is gone", profile.ID)
		}
		if profile.XRechnung && !strings.Contains(view.XML, "<ram:Content>Anzahlungsrechnung") && !strings.Contains(view.XML, "<cbc:Note>Anzahlungsrechnung") {
			t.Errorf("%s: the note on the advance is gone", profile.ID)
		}
	}
}

func TestOnlyTheUserMarksAnAdvance(t *testing.T) {
	draft := testDraft()
	draft.DeliveryDate = "2099-01-01"
	if issuedFromDocument(store.IssuedDocument{Draft: draft}).Advance {
		t.Fatal("a date in the future must not make an invoice an advance")
	}
}

func TestIssuedAdvanceKeepsItsMarkAndPassesItToTheCreditNote(t *testing.T) {
	s := receivedService(t)
	draft := testDraft()
	draft.AdvancePayment = true
	saved, err := s.SaveInvoice(draft)
	if err != nil {
		t.Fatal(err)
	}
	issued, err := s.IssueInvoice(saved.Draft)
	if err != nil {
		t.Fatal(err)
	}
	if !issued.Draft.AdvancePayment {
		t.Fatal("issuing lost the mark")
	}
	correction, err := s.CreateCorrectionDraft(issued.Draft.ID)
	if err != nil {
		t.Fatal(err)
	}
	docs, err := s.store.IssuedDocuments()
	if err != nil || len(docs) != 1 || !issuedFromDocument(docs[0]).Advance || !issuedDatevDoc(docs[0]).Advance {
		t.Fatalf("issued documents %v %+v", err, docs)
	}
	if !advanceDraft(correction.Draft) {
		t.Fatal("a credit note of an advance must follow its payouts")
	}
	correction.Draft.CorrectsInvoiceID = ""
	if advanceDraft(correction.Draft) {
		t.Fatal("a free credit note is no advance")
	}
}
