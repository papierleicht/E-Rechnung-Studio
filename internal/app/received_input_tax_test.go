package app

import (
	"encoding/base64"
	"strings"
	"testing"

	"papierleicht.de/e-rechnung-studio/internal/model"
	"papierleicht.de/e-rechnung-studio/internal/taxforms"
)

func valid() model.ValidationResult { return model.ValidationResult{State: "valid"} }

func withRule(id string) model.ValidationResult {
	return model.ValidationResult{State: "invalid", Issues: []model.ValidationIssue{{Stage: "rules", RuleID: id, Severity: "error"}}}
}

var ordinaryTotals = model.Totals{Net: "100.00", Tax: "19.00", Gross: "119.00"}

// A final invoice over 1190.00 with 600.00 paid before; 590.00 are left to pay.
var finalTotals = model.Totals{Net: "1000.00", Tax: "190.00", Gross: "1190.00", Due: "590.00", Prepaid: "600.00"}

func inputEntry(t *testing.T, r FinanceReport, id string) FinanceEntry {
	t.Helper()
	for _, e := range r.InputTax.Entries {
		if e.ID == id {
			return e
		}
	}
	t.Fatalf("%s is not in the Vorsteuer: %+v", id, r.InputTax.Entries)
	return FinanceEntry{}
}

// § 2 StBerG: whether a document entitles to the deduction needs a look at the
// individual case. The app suggests, the user decides and the figures follow.
func TestTheUsersDecisionOverrulesTheSuggestion(t *testing.T) {
	cases := []struct {
		name   string
		doc    model.ReceivedInvoice
		choice model.InputTaxChoice
		want   string
		note   string
	}{
		{"a document the check keeps out", receivedFor("a", withRule("BR-CO-15"), ordinaryTotals), model.InputTaxChoice{Mode: model.InputTaxFull}, "19.00", "Von dir eingerechnet"},
		{"a document the suggestion counts", receivedFor("a", valid(), ordinaryTotals), model.InputTaxChoice{Mode: model.InputTaxNone}, "0.00", "Von dir nicht eingerechnet"},
		{"part of a document", receivedFor("a", valid(), ordinaryTotals), model.InputTaxChoice{Mode: model.InputTaxAmount, Amount: "9.50"}, "9.50", "Betrag von dir festgelegt"},
		{"a final invoice in full", receivedFor("a", valid(), finalTotals), model.InputTaxChoice{Mode: model.InputTaxFull}, "190.00", "Von dir eingerechnet"},
		{"a document the check keeps out, in part", receivedFor("a", withRule("BR-CO-15"), ordinaryTotals), model.InputTaxChoice{Mode: model.InputTaxAmount, Amount: "5.00"}, "5.00", "Betrag von dir festgelegt"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := baseInput()
			in.Received = []model.ReceivedInvoice{tc.doc}
			tc.choice.ReceivedID = tc.doc.ID
			in.InputTaxChoices = map[string]model.InputTaxChoice{tc.doc.ID: tc.choice}
			r := buildFinanceReport(in)
			assertTraceable(t, r)
			if r.InputTax.Amount != tc.want {
				t.Fatalf("got %s, want %s: %+v", r.InputTax.Amount, tc.want, r.InputTax)
			}
			if e := inputEntry(t, r, tc.doc.ID); e.Note != tc.note {
				t.Fatalf("got note %q", e.Note)
			}
			for _, code := range []string{"received_needs_review", "received_prepaid_review", "received_correction_review"} {
				if hasIssue(r, code) {
					t.Errorf("%s: a decided document needs no further hint: %+v", code, r.Issues)
				}
			}
			if got := buildDashboard(in, r).ReceivedToReview; got != 0 {
				t.Errorf("a decided document needs no review, got %d", got)
			}
		})
	}
}

func TestVATReturnFollowsTheUsersDecision(t *testing.T) {
	kz66 := func(t *testing.T, in financeInput) string {
		t.Helper()
		_, purchases, _ := vatRecords(in)
		return kz(taxforms.BuildVATReturn(nil, purchases, d("0")).Fields, "66")
	}
	blocked := receivedFor("a", withRule("BR-CO-15"), ordinaryTotals)
	counted := receivedFor("b", valid(), ordinaryTotals)
	in := baseInput()
	in.Received = []model.ReceivedInvoice{blocked, counted}
	if got := kz66(t, in); got != "19.00" && got != "19" {
		t.Fatalf("the suggestion counts one document, got %q", got)
	}
	in.InputTaxChoices = map[string]model.InputTaxChoice{
		"a": {ReceivedID: "a", Mode: model.InputTaxFull},
		"b": {ReceivedID: "b", Mode: model.InputTaxNone},
	}
	if got := kz66(t, in); got != "19.00" && got != "19" {
		t.Fatalf("the document kept out counts now, the other one not, got %q", got)
	}
	in.InputTaxChoices["a"] = model.InputTaxChoice{ReceivedID: "a", Mode: model.InputTaxAmount, Amount: "7.25"}
	if got := kz66(t, in); got != "7.25" {
		t.Fatalf("got %q", got)
	}
	// A small business deducts nothing, whatever a stored decision says.
	in.SmallBusiness = true
	if _, purchases, _ := vatRecords(in); len(purchases) != 0 {
		t.Fatalf("got %+v", purchases)
	}
}

// The user decides on the tax the document states. On a broken document its tax
// groups can say something else, and the return must count what the user chose.
func TestAFullDecisionCountsTheTaxTheDocumentStates(t *testing.T) {
	in := baseInput()
	in.Received = []model.ReceivedInvoice{receivedFor("a", withRule("BR-CO-14"), ordinaryTotals)}
	in.ReceivedTax = map[string]receivedTax{"a": {Rates: []taxRate{{Category: "S", Percent: d("19"), Basis: d("100"), Amount: d("20")}}}}
	in.InputTaxChoices = map[string]model.InputTaxChoice{"a": {ReceivedID: "a", Mode: model.InputTaxFull}}
	if _, purchases, _ := vatRecords(in); len(purchases) != 1 || !purchases[0].Tax.Equal(d("19")) {
		t.Fatalf("got %+v", purchases)
	}
}

// Abschnitt 15.3 Abs. 5 UStAE: from a final invoice only the tax on the remaining
// payment is deductible. The rest stands in the down-payment invoices.
func TestAFinalInvoiceCountsTheTaxOfTheRemainingPayment(t *testing.T) {
	in := baseInput()
	in.Received = []model.ReceivedInvoice{receivedFor("final", valid(), finalTotals)}
	r := buildFinanceReport(in)
	assertTraceable(t, r)
	if r.InputTax.Amount != "94.20" {
		t.Fatalf("190.00 × 590 / 1190 is 94.20: %+v", r.InputTax)
	}
	if note, ok := entryNote(r.Issues, "received_prepaid_review", "final"); !ok || !strings.Contains(note, "Restzahlung") {
		t.Fatalf("note %q %v", note, ok)
	}
	_, purchases, issues := vatRecords(in)
	if len(purchases) != 1 || !purchases[0].Tax.Equal(d("94.2")) || !purchases[0].Net.Equal(d("495.8")) {
		t.Fatalf("got %+v", purchases)
	}
	found := false
	for _, issue := range issues {
		found = found || issue.Code == "received_prepaid_review"
	}
	if !found {
		t.Fatalf("no hint in %+v", issues)
	}
}

// An invoice paid before it was written deducts the whole amount in BT-113. That
// is a shop invoice, not a final invoice after down payments: its tax counts.
func TestAnInvoicePaidInFullKeepsItsTax(t *testing.T) {
	in := baseInput()
	in.Received = []model.ReceivedInvoice{receivedFor("shop", valid(), model.Totals{Net: "100.00", Tax: "19.00", Gross: "119.00", Due: "0.00", Prepaid: "119.00"})}
	r := buildFinanceReport(in)
	if r.InputTax.Amount != "19.00" || hasIssue(r, "received_prepaid_review") {
		t.Fatalf("%+v %+v", r.InputTax, r.Issues)
	}
}

func TestRemainingShareNeedsAPaymentAndARest(t *testing.T) {
	for name, totals := range map[string]model.Totals{
		"nothing paid":      ordinaryTotals,
		"no amount due":     {Gross: "119.00", Prepaid: "10.00"},
		"everything paid":   {Gross: "119.00", Prepaid: "119.00", Due: "0.00"},
		"unreadable amount": {Gross: "x", Prepaid: "10.00", Due: "109.00"},
		"due above gross":   {Gross: "119.00", Prepaid: "10.00", Due: "120.00"},
	} {
		if got := remainingShare(receivedFor("a", valid(), totals)); !got.IsZero() {
			t.Errorf("%s: got %s", name, got)
		}
	}
}

func TestForeignCurrencyStaysOutUnlessTheUserGivesAnAmount(t *testing.T) {
	doc := receivedFor("a", valid(), ordinaryTotals)
	doc.Inspection.Currency = "USD"
	in := baseInput()
	in.Received = []model.ReceivedInvoice{doc}
	if r := buildFinanceReport(in); r.InputTax.Amount != "0.00" || !hasIssue(r, "received_needs_review") {
		t.Fatalf("%+v", r.InputTax)
	}
	in.InputTaxChoices = map[string]model.InputTaxChoice{"a": {ReceivedID: "a", Mode: model.InputTaxFull}}
	if r := buildFinanceReport(in); r.InputTax.Amount != "0.00" || !hasIssue(r, "received_needs_review") {
		t.Fatalf("the whole tax is no amount in euros: %+v", r.InputTax)
	}
	in.InputTaxChoices = map[string]model.InputTaxChoice{"a": {ReceivedID: "a", Mode: model.InputTaxAmount, Amount: "17.40"}}
	if r := buildFinanceReport(in); r.InputTax.Amount != "17.40" {
		t.Fatalf("%+v", r.InputTax)
	}
}

func TestACreditNoteKeepsItsSignInADecision(t *testing.T) {
	in := baseInput()
	in.Received = []model.ReceivedInvoice{receivedFor("credit", valid(), ordinaryTotals)}
	in.ReceivedTax = map[string]receivedTax{"credit": {Credit: true, Rates: []taxRate{{Category: "S", Percent: d("19"), Basis: d("-100"), Amount: d("-19")}}}}
	in.InputTaxChoices = map[string]model.InputTaxChoice{"credit": {ReceivedID: "credit", Mode: model.InputTaxAmount, Amount: "9.50"}}
	if r := buildFinanceReport(in); r.InputTax.Amount != "-9.50" {
		t.Fatalf("%+v", r.InputTax)
	}
	if _, purchases, _ := vatRecords(in); len(purchases) != 1 || !purchases[0].Tax.Equal(d("-9.5")) {
		t.Fatalf("%+v", purchases)
	}
}

func TestAccountingExportLeavesDecidedDocumentsToTheUser(t *testing.T) {
	blocked := receivedFor("a", withRule("BR-CO-15"), ordinaryTotals)
	allowed := receivedFor("b", withRule("BR-DE-15"), ordinaryTotals)
	final := receivedFor("c", valid(), finalTotals)
	for name, tc := range map[string]struct {
		doc    model.ReceivedInvoice
		choice *model.InputTaxChoice
		want   string
	}{
		"kept out by the check":      {blocked, nil, "Vorsteuer offen: Regelverstoß prüfen"},
		"counted despite the check":  {blocked, &model.InputTaxChoice{Mode: model.InputTaxFull}, ""},
		"finding without tax effect": {allowed, nil, ""},
		"kept out by the user":       {allowed, &model.InputTaxChoice{Mode: model.InputTaxNone}, "Von dir nicht in die Vorsteuer eingerechnet"},
		"own amount":                 {allowed, &model.InputTaxChoice{Mode: model.InputTaxAmount, Amount: "5"}, "Vorsteuer weicht von der Rechnung ab, bitte von Hand buchen"},
		"remaining payment":          {final, nil, "Vorsteuer weicht von der Rechnung ab, bitte von Hand buchen"},
		"advance":                    {advance("d", 386, advanceTotals), nil, "Vorsteuer zählt mit der Zahlung, bitte von Hand buchen"},
		"with the payment":           {allowed, &model.InputTaxChoice{Mode: model.InputTaxOnPayment}, "Vorsteuer zählt mit der Zahlung, bitte von Hand buchen"},
	} {
		if got := inputTaxProblem(tc.doc, tc.choice); got != tc.want {
			t.Errorf("%s: got %q", name, got)
		}
	}
}

func TestStoredDecisionReachesTheDocumentAndTheReport(t *testing.T) {
	s := receivedService(t)
	xml := replaceOnce(t, issuedXML(t, s, "xrechnung-cii", "cii"), "<ram:CalculatedAmount>38.00</ram:CalculatedAmount>", "<ram:CalculatedAmount>39.00</ram:CalculatedAmount>")
	imported, err := s.ImportReceivedInvoice("lieferant.xml", base64.StdEncoding.EncodeToString([]byte(xml)))
	if err != nil {
		t.Fatal(err)
	}
	id := imported.Invoice.ID
	if status := imported.Invoice.Inspection.InputTax; status == nil || status.Allowed || status.Tax != "" || status.Choice != nil {
		t.Fatalf("the check keeps it out: %+v", status)
	}
	got, err := s.SaveInputTaxChoice(model.InputTaxChoice{ReceivedID: id, Mode: model.InputTaxFull, Note: "Betrag mit dem Lieferanten geklärt"})
	if err != nil {
		t.Fatal(err)
	}
	if got.InputTax == nil || got.InputTax.Choice == nil || got.InputTax.Choice.Note != "Betrag mit dem Lieferanten geklärt" {
		t.Fatalf("%+v", got.InputTax)
	}
	list, err := s.ListReceivedInvoices()
	if err != nil || len(list) != 1 || list[0].Inspection.InputTax.Choice == nil {
		t.Fatalf("%+v %v", list, err)
	}
	rechecked, err := s.RecheckReceivedInvoice(id)
	if err != nil || rechecked.InputTax.Choice == nil {
		t.Fatalf("a new check keeps the decision: %+v %v", rechecked.InputTax, err)
	}
	cleared, err := s.ClearInputTaxChoice(id)
	if err != nil || cleared.InputTax.Choice != nil {
		t.Fatalf("%+v %v", cleared.InputTax, err)
	}
	if _, err = s.SaveInputTaxChoice(model.InputTaxChoice{ReceivedID: "unbekannt", Mode: model.InputTaxNone}); err == nil {
		t.Fatal("an unknown document must be refused")
	}
	if _, err = s.ChangeReceivedStatus(id, "misimport", "falsche Datei"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.SaveInputTaxChoice(model.InputTaxChoice{ReceivedID: id, Mode: model.InputTaxNone}); err == nil {
		t.Fatal("a misimport counts for nothing")
	}
}

func TestSuggestionIsShownWithItsAmount(t *testing.T) {
	inspection := describeInputTax(receivedFor("final", valid(), finalTotals), nil)
	if inspection.InputTax.Tax != "94.20" || !strings.Contains(inspection.InputTax.Reason, "Restzahlung") {
		t.Fatalf("%+v", inspection.InputTax)
	}
	ordinary := describeInputTax(receivedFor("a", valid(), ordinaryTotals), nil)
	if ordinary.InputTax.Tax != "19.00" || ordinary.InputTax.Reason != "" {
		t.Fatalf("%+v", ordinary.InputTax)
	}
}
