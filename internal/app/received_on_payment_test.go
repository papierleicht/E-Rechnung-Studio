package app

import (
	"encoding/base64"
	"strings"
	"testing"

	"papierleicht.de/e-rechnung-studio/internal/model"
	"papierleicht.de/e-rechnung-studio/internal/store"
	"papierleicht.de/e-rechnung-studio/internal/taxforms"
)

// An advance of 1190.00 with 190.00 tax, dated 2 October.
var advanceTotals = model.Totals{Net: "1000.00", Tax: "190.00", Gross: "1190.00", Due: "1190.00"}

func advance(id string, documentType int, totals model.Totals) model.ReceivedInvoice {
	v := receivedFor(id, valid(), totals)
	v.Inspection.DocumentType = documentType
	return v
}

// counted is the Vorsteuer of a period in the report and in the Voranmeldung,
// which must agree.
func counted(t *testing.T, in financeInput) string {
	t.Helper()
	r := buildFinanceReport(in)
	assertTraceable(t, r)
	_, purchases, _ := vatRecords(in)
	inReturn := d("0")
	if got := kz(taxforms.BuildVATReturn(nil, purchases, d("0")).Fields, "66"); got != "" {
		inReturn = d(got)
	}
	if !inReturn.Equal(d(r.InputTax.Amount)) {
		t.Fatalf("report says %s, Voranmeldung %s", r.InputTax.Amount, inReturn)
	}
	return r.InputTax.Amount
}

func returnIssue(in financeInput, code string) (taxforms.Entry, bool) {
	_, _, issues := vatRecords(in)
	for _, issue := range issues {
		if issue.Code == code && len(issue.Entries) > 0 {
			return issue.Entries[0], true
		}
	}
	return taxforms.Entry{}, false
}

func inMonth(in financeInput, from, to string) financeInput {
	in.From, in.To = from, to
	return in
}

// § 15 Abs. 1 Satz 1 Nr. 1 UStG and Abschnitt 15.3 Abs. 1 and 4 UStAE: tax on a
// payment ahead of the supply counts once invoice and payment are there, and only
// for the part paid.
func TestAnAdvanceCountsWithItsPayments(t *testing.T) {
	for name, documentType := range map[string]int{"prepayment invoice": 386, "partial invoice": 326, "partial construction invoice": 875} {
		t.Run(name, func(t *testing.T) {
			in := baseInput()
			in.Received = []model.ReceivedInvoice{advance("a", documentType, advanceTotals)}
			if got := counted(t, in); got != "0.00" {
				t.Fatalf("nothing is paid yet, got %s", got)
			}
			if note, listed := entryNote(buildFinanceReport(in).Issues, "advance_unpaid", "a"); !listed || note != "offen 1.190,00 €" {
				t.Fatalf("the report must name the open advance: %q %v", note, listed)
			}
			if e, listed := returnIssue(in, "advance_unpaid"); !listed || e.Note != "offen 1.190,00 €" {
				t.Fatalf("the Voranmeldung must name the open advance: %+v %v", e, listed)
			}

			in.Overview.Payments = []store.Payment{pay("p1", "a", "incoming", "595.00", "2026-10-20", "")}
			if got := counted(t, in); got != "95.00" {
				t.Fatalf("half is paid, got %s", got)
			}
			r := buildFinanceReport(in)
			e := inputEntry(t, r, "p1")
			if e.Kind != "payment" || e.Date != "2026-10-20" || e.Note != "Mit der Zahlung, anteilig 50 %" || !strings.HasPrefix(e.Label, "Zahlung zu a") {
				t.Fatalf("%+v", e)
			}
			if note, _ := entryNote(r.Issues, "advance_unpaid", "a"); note != "offen 595,00 €" {
				t.Fatalf("got %q", note)
			}

			in.Overview.Payments = append(in.Overview.Payments, pay("p2", "a", "incoming", "595.00", "2026-11-03", ""))
			if hasIssue(buildFinanceReport(in), "advance_unpaid") {
				t.Fatal("a paid advance is no open point")
			}
			if _, listed := returnIssue(in, "advance_unpaid"); listed {
				t.Fatal("a paid advance is no open point in the Voranmeldung")
			}
			if got := counted(t, inMonth(in, "2026-11-01", "2026-11-30")); got != "95.00" {
				t.Fatalf("the rest counts in November, got %s", got)
			}
			if got := counted(t, inMonth(in, "2026-09-01", "2026-09-30")); got != "0.00" {
				t.Fatalf("got %s", got)
			}
		})
	}
}

func TestAPaymentBeforeTheInvoiceCountsOnTheInvoiceDate(t *testing.T) {
	in := baseInput()
	in.Received = []model.ReceivedInvoice{advance("a", 386, advanceTotals)}
	in.Overview.Payments = []store.Payment{pay("p1", "a", "incoming", "1190.00", "2026-09-20", "")}
	if got := counted(t, inMonth(in, "2026-09-01", "2026-09-30")); got != "0.00" {
		t.Fatalf("the invoice is not there yet, got %s", got)
	}
	if got := counted(t, in); got != "190.00" {
		t.Fatalf("got %s", got)
	}
	if e := inputEntry(t, buildFinanceReport(in), "p1"); e.Date != "2026-10-02" {
		t.Fatalf("%+v", e)
	}
}

func TestInstalmentsAddUpToTheTaxOfTheDocument(t *testing.T) {
	v := advance("a", 386, model.Totals{Net: "133.34", Tax: "21.33", Gross: "154.67", Due: "154.67"})
	in := baseInput()
	in.Received = []model.ReceivedInvoice{v}
	in.ReceivedTax = map[string]receivedTax{"a": {Rates: []taxRate{
		{Category: "S", Percent: d("19"), Basis: d("100.01"), Amount: d("19.00")},
		{Category: "S", Percent: d("7"), Basis: d("33.33"), Amount: d("2.33")},
	}}}
	in.Overview.Payments = []store.Payment{
		pay("p1", "a", "incoming", "51.56", "2026-10-05", ""),
		pay("p2", "a", "incoming", "51.56", "2026-10-06", ""),
		pay("p3", "a", "incoming", "51.55", "2026-10-07", ""),
	}
	if got := counted(t, in); got != "21.33" {
		t.Fatalf("got %s", got)
	}
	seen := map[string]bool{}
	for _, e := range buildFinanceReport(in).InputTax.Entries {
		key := e.Kind + e.ID
		if seen[key] {
			t.Fatalf("the report keys its list by kind and ID: %+v", e)
		}
		seen[key] = true
	}
	if len(seen) != 3 {
		t.Fatalf("one entry per payment, got %d", len(seen))
	}
}

func TestOnlyPaymentsUpToTheAmountDueCount(t *testing.T) {
	revoked := pay("p0", "a", "incoming", "1190.00", "2026-10-03", "")
	revoked.RevokedAt = "2026-10-04T10:00:00Z"
	in := baseInput()
	in.Received = []model.ReceivedInvoice{advance("a", 386, advanceTotals)}
	in.Overview.Payments = []store.Payment{
		revoked,
		pay("p1", "a", "incoming", "1000.00", "2026-10-10", ""),
		pay("p2", "a", "incoming", "1000.00", "2026-10-20", ""),
		pay("p3", "a", "incoming", "50.00", "2026-10-25", ""),
		pay("other", "b", "incoming", "1190.00", "2026-10-25", ""),
		pay("sale", "a", "outgoing", "1190.00", "2026-10-25", ""),
	}
	if got := counted(t, in); got != "190.00" {
		t.Fatalf("got %s", got)
	}
	for _, e := range buildFinanceReport(in).InputTax.Entries {
		if e.ID != "p1" && e.ID != "p2" {
			t.Fatalf("this payment adds nothing: %+v", e)
		}
	}
}

func TestAReturnedPaymentTakesItsPartBack(t *testing.T) {
	in := baseInput()
	in.Received = []model.ReceivedInvoice{advance("a", 386, advanceTotals)}
	in.Overview.Payments = []store.Payment{
		pay("p1", "a", "incoming", "1190.00", "2026-10-20", ""),
		pay("p2", "a", "incoming", "-595.00", "2026-11-03", ""),
	}
	if got := counted(t, in); got != "190.00" {
		t.Fatalf("got %s", got)
	}
	november := inMonth(in, "2026-11-01", "2026-11-30")
	if got := counted(t, november); got != "-95.00" {
		t.Fatalf("got %s", got)
	}
	if note, _ := entryNote(buildFinanceReport(november).Issues, "advance_unpaid", "a"); note != "offen 595,00 €" {
		t.Fatalf("got %q", note)
	}
}

// Construction invoices often repeat what was billed before and deduct it. Only
// the new part is to be paid, and only its tax counts.
func TestACumulativeInvoiceCountsTheNewPart(t *testing.T) {
	v := advance("a", 875, model.Totals{Net: "2000.00", Tax: "380.00", Gross: "2380.00", Due: "1190.00", Prepaid: "1190.00"})
	if status := describeInputTax(v, nil).InputTax; status.Tax != "190.00" || !status.OnPayment {
		t.Fatalf("%+v", status)
	}
	in := baseInput()
	in.Received = []model.ReceivedInvoice{v}
	in.Overview.Payments = []store.Payment{pay("p1", "a", "incoming", "2000.00", "2026-10-20", "")}
	if got := counted(t, in); got != "190.00" {
		t.Fatalf("got %s", got)
	}
}

func TestADocumentPaidBeforeItWasWrittenCountsOnItsDate(t *testing.T) {
	settled := model.Totals{Net: "1000.00", Tax: "190.00", Gross: "1190.00", Due: "0.00", Prepaid: "1190.00"}
	for name, choice := range map[string]*model.InputTaxChoice{
		"suggestion": nil,
		"decision":   {ReceivedID: "a", Mode: model.InputTaxOnPayment},
	} {
		t.Run(name, func(t *testing.T) {
			in := baseInput()
			in.Received = []model.ReceivedInvoice{advance("a", 386, settled)}
			if choice != nil {
				in.InputTaxChoices = map[string]model.InputTaxChoice{"a": *choice}
			}
			if got := counted(t, in); got != "190.00" {
				t.Fatalf("got %s", got)
			}
			if hasIssue(buildFinanceReport(in), "advance_unpaid") {
				t.Fatal("nothing is left to pay")
			}
		})
	}
}

func TestTheUserDecidesWhenADocumentCounts(t *testing.T) {
	cases := []struct {
		name   string
		doc    model.ReceivedInvoice
		mode   string
		want   string
		unpaid bool
	}{
		{"an ordinary invoice with the payment", receivedFor("a", valid(), ordinaryTotals), model.InputTaxOnPayment, "0.00", true},
		{"a blocked document with the payment", receivedFor("a", withRule("BR-CO-15"), ordinaryTotals), model.InputTaxOnPayment, "0.00", true},
		{"a partial invoice whose part is done", advance("a", 326, advanceTotals), model.InputTaxFull, "190.00", false},
		{"an advance that is never paid", advance("a", 386, advanceTotals), model.InputTaxNone, "0.00", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := baseInput()
			in.Received = []model.ReceivedInvoice{tc.doc}
			in.InputTaxChoices = map[string]model.InputTaxChoice{"a": {ReceivedID: "a", Mode: tc.mode}}
			if got := counted(t, in); got != tc.want {
				t.Fatalf("got %s, want %s", got, tc.want)
			}
			if got := hasIssue(buildFinanceReport(in), "advance_unpaid"); got != tc.unpaid {
				t.Fatalf("open point: got %v", got)
			}
		})
	}
}

func TestAFullDecisionOnPaymentCountsTheTaxTheDocumentStates(t *testing.T) {
	in := baseInput()
	in.Received = []model.ReceivedInvoice{receivedFor("a", withRule("BR-CO-14"), ordinaryTotals)}
	in.ReceivedTax = map[string]receivedTax{"a": {Rates: []taxRate{{Category: "S", Percent: d("19"), Basis: d("100"), Amount: d("20")}}}}
	in.InputTaxChoices = map[string]model.InputTaxChoice{"a": {ReceivedID: "a", Mode: model.InputTaxOnPayment}}
	in.Overview.Payments = []store.Payment{pay("p1", "a", "incoming", "59.50", "2026-10-20", "")}
	if got := counted(t, in); got != "9.50" {
		t.Fatalf("got %s", got)
	}
}

func TestACreditNoteOnPaymentKeepsItsSign(t *testing.T) {
	in := baseInput()
	in.Received = []model.ReceivedInvoice{receivedFor("a", valid(), ordinaryTotals)}
	in.ReceivedTax = map[string]receivedTax{"a": {Credit: true, Rates: []taxRate{{Category: "S", Percent: d("19"), Basis: d("-100"), Amount: d("-19")}}}}
	in.InputTaxChoices = map[string]model.InputTaxChoice{"a": {ReceivedID: "a", Mode: model.InputTaxOnPayment}}
	in.Overview.Payments = []store.Payment{pay("p1", "a", "incoming", "119.00", "2026-10-20", "")}
	if got := counted(t, in); got != "-19.00" {
		t.Fatalf("got %s", got)
	}
}

// § 13b Abs. 4 Satz 2 UStG: on a payment ahead of the supply the tax the recipient
// owes arises with the payment, too.
func TestTaxOwedOnAnAdvanceFollowsThePayment(t *testing.T) {
	in := baseInput()
	in.Received = []model.ReceivedInvoice{advance("a", 386, model.Totals{Net: "1000.00", Tax: "0.00", Gross: "1000.00", Due: "1000.00"})}
	in.ReceivedTax = map[string]receivedTax{"a": {Country: "FR", Rates: []taxRate{{Category: "AE", Basis: d("1000"), Amount: d("0")}}}}
	in.Overview.Payments = []store.Payment{pay("p1", "a", "incoming", "500.00", "2026-11-03", "")}
	if _, purchases, _ := vatRecords(in); len(purchases) != 0 {
		t.Fatalf("nothing is paid in October: %+v", purchases)
	}
	november := inMonth(in, "2026-11-01", "2026-11-30")
	_, purchases, _ := vatRecords(november)
	r := taxforms.BuildVATReturn(nil, purchases, d("0"))
	if kz(r.Fields, "46") != "500" || kz(r.Fields, "47") != "95.00" || kz(r.Fields, "67") != "95.00" {
		t.Fatalf("%+v", r.Fields)
	}
	november.SmallBusiness = true
	_, purchases, _ = vatRecords(november)
	r = taxforms.BuildVATReturn(nil, purchases, d("0"))
	if kz(r.Fields, "47") != "95.00" || kz(r.Fields, "67") != "" {
		t.Fatalf("a small business owes the tax and deducts nothing: %+v", r.Fields)
	}
}

func TestSmallBusinessFilesForThePeriodOfThePayment(t *testing.T) {
	s := receivedService(t)
	doc := advance("a", 386, model.Totals{Net: "1000.00", Tax: "0.00", Gross: "1000.00", Due: "1000.00"})
	if _, err := s.store.SaveReceived(doc, []byte("original")); err != nil {
		t.Fatal(err)
	}
	s.receivedTax = map[string]receivedTax{"a": {Country: "FR", Rates: []taxRate{{Category: "AE", Basis: d("1000"), Amount: d("0")}}}}
	dates, err := s.smallBusinessTaxDates()
	if err != nil || len(dates) != 0 {
		t.Fatalf("nothing is paid yet: %v %v", dates, err)
	}
	if _, err = s.store.AddPayment(store.Payment{InvoiceID: "a", Direction: "incoming", Amount: "1000.00", Currency: "EUR", Date: "2026-11-03", Source: "manual", Reason: "Überweisung"}); err != nil {
		t.Fatal(err)
	}
	dates, err = s.smallBusinessTaxDates()
	if err != nil || len(dates) != 1 || dates[0] != "2026-11-03" {
		t.Fatalf("%v %v", dates, err)
	}
}

func TestACapturedAdvanceCountsFromItsPaymentDate(t *testing.T) {
	x := store.Expense{ID: "x", Date: "2026-10-05", Supplier: "Küchenbauer", Gross: "119.00", Net: "100.00", Tax: "19.00", TaxRate: "19", Advance: true}
	in := baseInput()
	in.Expenses = []store.Expense{x}
	if got := counted(t, in); got != "0.00" {
		t.Fatalf("nothing is paid yet, got %s", got)
	}
	if _, listed := entryNote(buildFinanceReport(in).Issues, "advance_unpaid", "x"); !listed {
		t.Fatal("the report must name the open advance")
	}
	if e, listed := returnIssue(in, "advance_unpaid"); !listed || e.Date != "2026-10-05" {
		t.Fatalf("the Voranmeldung must name the open advance: %+v %v", e, listed)
	}

	in.Expenses[0].PaidDate = "2026-11-03"
	if got := counted(t, in); got != "0.00" {
		t.Fatalf("paid in November, got %s", got)
	}
	if hasIssue(buildFinanceReport(in), "advance_unpaid") {
		t.Fatal("a paid advance is no open point")
	}
	if got := counted(t, inMonth(in, "2026-11-01", "2026-11-30")); got != "19.00" {
		t.Fatalf("got %s", got)
	}

	in.Expenses[0].PaidDate = "2026-09-20"
	if got := counted(t, inMonth(in, "2026-09-01", "2026-09-30")); got != "0.00" {
		t.Fatalf("the receipt is not there yet, got %s", got)
	}
	if got := counted(t, in); got != "19.00" {
		t.Fatalf("got %s", got)
	}

	in.Expenses[0].Advance, in.Expenses[0].PaidDate = false, ""
	if got := counted(t, in); got != "19.00" {
		t.Fatalf("an ordinary receipt counts on its date, got %s", got)
	}
}

func TestAnImportedAdvanceFollowsItsRecordedPayments(t *testing.T) {
	s := receivedService(t)
	xml := replaceOnce(t, issuedXML(t, s, "xrechnung-cii", "cii"), "<ram:TypeCode>380</ram:TypeCode>", "<ram:TypeCode>386</ram:TypeCode>")
	imported, err := s.ImportReceivedInvoice("anzahlung.xml", base64.StdEncoding.EncodeToString([]byte(xml)))
	if err != nil {
		t.Fatal(err)
	}
	status := imported.Invoice.Inspection.InputTax
	if imported.Invoice.Inspection.DocumentType != 386 || status == nil || !status.Allowed || !status.OnPayment || !strings.Contains(status.Reason, "Anzahlungs-") {
		t.Fatalf("%+v", imported.Invoice.Inspection)
	}
	full, err := s.SaveInputTaxChoice(model.InputTaxChoice{ReceivedID: imported.Invoice.ID, Mode: model.InputTaxOnPayment})
	if err != nil || full.InputTax.Choice == nil || full.InputTax.Choice.Mode != model.InputTaxOnPayment {
		t.Fatalf("%+v %v", full.InputTax, err)
	}
}

func TestSuggestionNamesAnAdvance(t *testing.T) {
	status := describeInputTax(advance("a", 326, advanceTotals), nil).InputTax
	if status.Tax != "190.00" || !status.Allowed || !status.OnPayment || !strings.Contains(status.Reason, "bezahlten Anteil") {
		t.Fatalf("%+v", status)
	}
	if ordinary := describeInputTax(advance("a", 380, advanceTotals), nil).InputTax; ordinary.OnPayment || ordinary.Reason != "" {
		t.Fatalf("%+v", ordinary)
	}
	if final := describeInputTax(advance("a", 877, finalTotals), nil).InputTax; final.OnPayment || !strings.Contains(final.Reason, "Restzahlung") {
		t.Fatalf("%+v", final)
	}
}
