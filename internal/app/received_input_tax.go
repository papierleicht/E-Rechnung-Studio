package app

import (
	"errors"
	"sort"
	"strings"

	"github.com/shopspring/decimal"
	"papierleicht.de/e-rechnung-studio/internal/model"
	"papierleicht.de/e-rechnung-studio/internal/store"
	"papierleicht.de/e-rechnung-studio/internal/taxforms"
	validator "papierleicht.de/e-rechnung-studio/internal/validation"
)

// prepaidMessage is raised for a final invoice whose tax the app counts in part.
// Abschnitt 15.3 Abs. 5 UStAE: from a final invoice only the tax on the remaining
// payment is deductible, the rest stands in the down-payment invoices.
const prepaidMessage = "Eine Endrechnung weist die Steuer der ganzen Leistung aus. Gab es Anzahlungsrechnungen, entfällt auf die Endrechnung in der Regel nur die Steuer der Restzahlung (Abschnitt 15.3 Abs. 5 UStAE). Die App rechnet deshalb nur diesen Anteil ein. Gab es keine Anzahlungsrechnungen, entscheide im Beleg anders."

// onPaymentMessage explains a document whose tax the app counts with its payments.
// § 15 Abs. 1 Satz 1 Nr. 1 UStG and Abschnitt 15.3 UStAE: tax on a payment ahead of
// the supply counts once invoice and payment are there, and only for the part paid.
const onPaymentMessage = "Laut Datei ist das eine Anzahlungs-, Abschlags- oder Teilrechnung. Wird vor der Leistung gezahlt, zählt die Steuer in der Regel erst mit der Zahlung und nur für den bezahlten Anteil (Abschnitt 15.3 UStAE). Ob die Leistung schon ausgeführt ist, zeigt die Datei nicht. Die App rechnet deshalb nur erfasste Zahlungen ein. Ist die Leistung ausgeführt, entscheide im Beleg anders."

// unpaidMessage is raised for documents and receipts that wait for a payment.
const unpaidMessage = "Diese Anzahlungen sind laut App noch nicht voll bezahlt. Ihre Steuer zählt erst mit der Zahlung. Hast du schon gezahlt, erfasse die Zahlung. Kommt keine Zahlung mehr, entscheide im Beleg."

// receivedInputTax says how a received document enters the Vorsteuer: as the
// user decided or, without a decision, as the app suggests.
type receivedInputTax struct {
	// Review is the reason a document stays out until the user decides.
	Review string
	// Tax is the amount that goes into the figures, always positive.
	Tax decimal.Decimal
	// Share of the document's tax that counts, 1 for all of it.
	Share decimal.Decimal
	// Fixed marks an amount the user set, which fits no share of the document.
	Fixed bool
	// Excluded marks a document the user keeps out.
	Excluded bool
	// Remainder marks a final invoice of which the app counts only the
	// remaining payment.
	Remainder bool
	// OnPayment marks tax that counts with the payments on the document, not on
	// its date. Tax is then what counts once everything is paid.
	OnPayment bool
	// Note describes the entry.
	Note string
}

// remainingShare is the part of a final invoice that is still to pay. It is zero
// when the document deducts no payments or nothing remains to pay, as on an
// invoice settled before it was written: then no down-payment invoices are in sight.
func remainingShare(v model.ReceivedInvoice) decimal.Decimal {
	totals := v.Inspection.Totals
	prepaid, err := decimal.NewFromString(totals.Prepaid)
	if err != nil || !prepaid.IsPositive() {
		return decimal.Zero
	}
	gross, err := decimal.NewFromString(totals.Gross)
	if err != nil || !gross.IsPositive() {
		return decimal.Zero
	}
	due, err := decimal.NewFromString(totals.Due)
	if err != nil || !due.IsPositive() || due.GreaterThanOrEqual(gross) {
		return decimal.Zero
	}
	return due.Div(gross)
}

// billedAhead reports a document type that usually bills a payment ahead of the
// supply: 386 prepayment invoice, 326 partial invoice, 875 partial construction
// invoice. The list of XRechnung has no 386 (BR-DE-17), so a prepayment can arrive
// as 326. Whether a part of the supply is already done, the file does not show.
func billedAhead(documentType int) bool {
	return documentType == 386 || documentType == 326 || documentType == 875
}

// followsPayments reports whether the tax of a document can follow its payments.
// That takes a gross total to divide and something left to pay: a document that
// deducts its whole amount was paid before it was written, so invoice and payment
// are both there on its date.
func followsPayments(v model.ReceivedInvoice) bool {
	totals := v.Inspection.Totals
	gross, err := decimal.NewFromString(totals.Gross)
	if err != nil || !gross.IsPositive() {
		return false
	}
	prepaid, err := decimal.NewFromString(totals.Prepaid)
	if err != nil || !prepaid.IsPositive() {
		return true
	}
	due, err := decimal.NewFromString(totals.Due)
	return err != nil || due.IsPositive()
}

// inputTaxFor applies the user's decision or, without one, the app's suggestion.
func inputTaxFor(v model.ReceivedInvoice, choice *model.InputTaxChoice) receivedInputTax {
	one := decimal.NewFromInt(1)
	if choice != nil {
		switch choice.Mode {
		case model.InputTaxNone:
			return receivedInputTax{Excluded: true, Note: "Von dir nicht eingerechnet"}
		case model.InputTaxAmount:
			if amount, err := decimal.NewFromString(choice.Amount); err == nil {
				return receivedInputTax{Tax: amount, Fixed: true, Note: "Betrag von dir festgelegt"}
			}
		}
	}
	tax, err := decimal.NewFromString(v.Inspection.Totals.Tax)
	if err != nil {
		return receivedInputTax{Review: "Steuerbetrag fehlt"}
	}
	if v.Inspection.Currency != "" && v.Inspection.Currency != "EUR" {
		return receivedInputTax{Review: "Fremdwährung"}
	}
	if choice != nil {
		if choice.Mode == model.InputTaxOnPayment && followsPayments(v) {
			return receivedInputTax{Tax: tax, Share: one, OnPayment: true, Note: "Mit der Zahlung"}
		}
		return receivedInputTax{Tax: tax, Share: one, Note: "Von dir eingerechnet"}
	}
	// Only findings that can concern a Pflichtangabe keep a document out
	// (BMF letter of 15.10.2026, Rn. 22 and 23).
	if status := validator.ReceivedInputTax(v.Inspection.Validation); !status.Allowed {
		return receivedInputTax{Review: status.Short}
	}
	share := remainingShare(v)
	if billedAhead(v.Inspection.DocumentType) && followsPayments(v) {
		if !share.IsPositive() {
			share = one
		}
		return receivedInputTax{Tax: tax.Mul(share).Round(2), Share: share, OnPayment: true, Note: "Mit der Zahlung"}
	}
	if share.IsPositive() {
		return receivedInputTax{Tax: tax.Mul(share).Round(2), Share: share, Remainder: true, Note: "Endrechnung: nur Steuer der Restzahlung eingerechnet"}
	}
	return receivedInputTax{Tax: tax, Share: one}
}

// paidPart is what one payment settles of a document: the share of its gross
// total paid before and after the payment.
type paidPart struct {
	Payment       string
	Date          string
	Before, After decimal.Decimal
}

// of takes the payment's part of an amount of the document. Rounding the running
// total keeps the parts adding up to the amount.
func (p paidPart) of(amount decimal.Decimal) decimal.Decimal {
	return amount.Mul(p.After).Round(2).Sub(amount.Mul(p.Before).Round(2))
}

// paidParts walks the payments on a document by date. Each counts on the day
// invoice and payment are both there (Abschnitt 15.3 Abs. 1 UStAE). What exceeds
// the amount to pay adds nothing, and a returned payment takes its part back. It
// also returns what is left to pay.
func paidParts(v model.ReceivedInvoice, payments []store.Payment) ([]paidPart, decimal.Decimal) {
	gross, _ := decimal.NewFromString(v.Inspection.Totals.Gross)
	payable := gross
	if due, err := decimal.NewFromString(v.Inspection.Totals.Due); err == nil && due.IsPositive() && due.LessThan(gross) {
		payable = due
	}
	own := []store.Payment{}
	for _, p := range payments {
		if p.Direction == "incoming" && p.InvoiceID == v.ID && p.RevokedAt == "" {
			own = append(own, p)
		}
	}
	sort.SliceStable(own, func(i, j int) bool { return own[i].Date < own[j].Date })
	parts := []paidPart{}
	total, paid := decimal.Zero, decimal.Zero
	for _, p := range own {
		amount, err := decimal.NewFromString(p.Amount)
		if err != nil {
			continue
		}
		total = total.Add(amount)
		after := decimal.Max(decimal.Zero, decimal.Min(total, payable))
		if after.Equal(paid) {
			continue
		}
		parts = append(parts, paidPart{Payment: p.ID, Date: max(p.Date, v.Inspection.IssueDate), Before: paid.Div(gross), After: after.Div(gross)})
		paid = after
	}
	return parts, payable.Sub(paid)
}

// paidInputTax is what a document adds to the Voranmeldung with one payment.
type paidInputTax struct {
	Entry     taxforms.Entry
	Purchases []taxforms.Purchase
}

// onPaymentInputTax lists what a document that counts with its payments adds in
// the period, one item per payment, and what is left to pay.
func onPaymentInputTax(in financeInput, v model.ReceivedInvoice, counted receivedInputTax, choice *model.InputTaxChoice, e taxforms.Entry) ([]paidInputTax, decimal.Decimal) {
	t, parsed := in.ReceivedTax[v.ID]
	parts, open := paidParts(v, in.Overview.Payments)
	out := []paidInputTax{}
	for _, part := range parts {
		if !inPeriod(part.Date, in.From, in.To) {
			continue
		}
		percent := part.After.Sub(part.Before).Mul(decimal.NewFromInt(100)).Round(1).String()
		item := paidInputTax{Entry: e}
		item.Entry.Kind, item.Entry.ID, item.Entry.Date = "payment", part.Payment, part.Date
		item.Entry.Label = "Zahlung zu " + e.Label
		item.Entry.Note = counted.Note + ", anteilig " + strings.ReplaceAll(percent, ".", ",") + " %"
		for _, p := range countedPurchases(v, t, parsed, counted, choice, in.SmallBusiness, item.Entry) {
			p.Net, p.Tax = part.of(p.Net), part.of(p.Tax)
			item.Purchases = append(item.Purchases, p)
		}
		out = append(out, item)
	}
	return out, open
}

// openNote marks an entry with what is left to pay.
func openNote(open decimal.Decimal) string {
	return "offen " + taxforms.FormatGerman(open.StringFixed(2), true) + " €"
}

// expenseTaxDate is the day a captured receipt counts for the Vorsteuer: its date
// or, for an advance, the day receipt and payment are both there. It stays empty
// while an advance is unpaid.
func expenseTaxDate(x store.Expense) string {
	if !x.Advance {
		return x.Date
	}
	if x.PaidDate == "" {
		return ""
	}
	return max(x.Date, x.PaidDate)
}

// inputTaxProblem names why an accounting export should leave a received document
// to be booked by hand: its Vorsteuer stays out, or it differs from the invoice.
func inputTaxProblem(v model.ReceivedInvoice, choice *model.InputTaxChoice) string {
	counted := inputTaxFor(v, choice)
	switch {
	case counted.Review != "":
		return "Vorsteuer offen: " + counted.Review
	case counted.Excluded:
		return "Von dir nicht in die Vorsteuer eingerechnet"
	case counted.Fixed, counted.Remainder:
		return "Vorsteuer weicht von der Rechnung ab, bitte von Hand buchen"
	case counted.OnPayment:
		return "Vorsteuer zählt mit der Zahlung, bitte von Hand buchen"
	}
	return ""
}

// describeInputTax adds what the document view shows: the check's status, the
// suggested amount and the user's decision.
func describeInputTax(v model.ReceivedInvoice, choice *model.InputTaxChoice) model.ImportInspection {
	inspection := validator.AnnotateReceived(v.Inspection)
	status := *inspection.InputTax
	suggested := inputTaxFor(v, nil)
	if suggested.Review == "" {
		status.Tax = money(suggested.Tax)
	}
	if suggested.Remainder {
		status.Reason = prepaidMessage
	}
	if suggested.OnPayment {
		status.OnPayment, status.Reason = true, onPaymentMessage
	}
	status.Choice = choice
	inspection.InputTax = &status
	return inspection
}

// presentReceived prepares stored documents for the user: their check result
// with the app's suggestion and the user's decision.
func (s *InvoiceService) presentReceived(items []model.ReceivedInvoice) error {
	choices, err := s.store.InputTaxChoices()
	if err != nil {
		return err
	}
	for i := range items {
		items[i].Inspection = describeInputTax(items[i], choiceOf(choices, items[i].ID))
	}
	return nil
}

func choiceOf(choices map[string]model.InputTaxChoice, id string) *model.InputTaxChoice {
	if c, ok := choices[id]; ok {
		return &c
	}
	return nil
}

func (s *InvoiceService) presentedInspection(id string) (model.ImportInspection, error) {
	item, _, err := s.store.GetReceived(id)
	if err != nil {
		return model.ImportInspection{}, err
	}
	items := []model.ReceivedInvoice{item}
	if err = s.presentReceived(items); err != nil {
		return model.ImportInspection{}, err
	}
	return items[0].Inspection, nil
}

// SaveInputTaxChoice stores the user's decision on the Vorsteuer of a received
// document. The app suggests; what goes into the figures is the user's call.
func (s *InvoiceService) SaveInputTaxChoice(c model.InputTaxChoice) (model.ImportInspection, error) {
	item, _, err := s.store.GetReceived(c.ReceivedID)
	if err != nil {
		return model.ImportInspection{}, err
	}
	if item.Status == "misimport" {
		return model.ImportInspection{}, errors.New("Für einen Fehlimport gibt es nichts einzurechnen.")
	}
	if _, err = s.store.SaveInputTaxChoice(c); err != nil {
		return model.ImportInspection{}, err
	}
	return s.presentedInspection(c.ReceivedID)
}

// ClearInputTaxChoice returns a received document to the app's suggestion.
func (s *InvoiceService) ClearInputTaxChoice(id string) (model.ImportInspection, error) {
	if err := s.store.ClearInputTaxChoice(id); err != nil {
		return model.ImportInspection{}, err
	}
	return s.presentedInspection(id)
}
