package app

import (
	"sort"

	"github.com/shopspring/decimal"
	"papierleicht.de/e-rechnung-studio/internal/model"
	"papierleicht.de/e-rechnung-studio/internal/store"
	"papierleicht.de/e-rechnung-studio/internal/taxforms"
)

// Anzahlungsrechnungen under Soll-Versteuerung: the Umsatzsteuer arises with the
// money, not with the supply (§ 13 Abs. 1 Nr. 1 Buchst. a Satz 4 UStG). The user
// marks such an invoice; the app never derives it from a date. The remaining
// invoice stays an ordinary one.

const (
	advanceOpenMessage     = "Diese Anzahlungsrechnungen sind laut App noch nicht voll bezahlt. Ihre Umsatzsteuer zählt hier erst mit dem Zahlungseingang. Ist schon Geld gekommen, erfasse die Zahlung."
	advanceOverpaidMessage = "Auf diese Anzahlungsrechnungen ist mehr eingegangen, als sie ausweisen. Die App rechnet nur den Rechnungsbetrag ein. Was mit dem Rest gilt, klärst du am besten mit deiner Steuerberatung."
	advanceLateMessage     = "Diese Zahlungen auf Anzahlungsrechnungen wurden nach der Festschreibung erfasst oder aufgehoben. Die Steuer folgt dem Zahlungsdatum, das Ergebnis des festgeschriebenen Zeitraums ändert sich also. Ist die Voranmeldung schon abgegeben, prüfe eine Berichtigung."
)

// advanceDraft reports a document marked as an advance. A credit note counts only
// as the correction of one: it then inherits the mark and follows its payouts.
func advanceDraft(d model.InvoiceDraft) bool {
	return d.AdvancePayment && (d.Kind != "credit-note" || d.CorrectsInvoiceID != "")
}

// advanceDoc is what the walk through the payments needs from an invoice.
type advanceDoc struct {
	ID, CorrectsID, IssueDate string
	Credit                    bool
	// Gross is the invoice amount, positive on credit notes too: money paid out
	// on a credit note is a positive payment.
	Gross decimal.Decimal
}

func (v issuedTax) advance() advanceDoc {
	return advanceDoc{ID: v.ID, CorrectsID: v.CorrectsID, IssueDate: v.IssueDate, Credit: v.Credit, Gross: v.Gross.Abs()}
}

func (doc datevDoc) advance() advanceDoc {
	gross := decimal.Zero
	for _, t := range doc.Taxes {
		gross = gross.Add(t.Basis).Add(t.Amount)
	}
	return advanceDoc{ID: doc.ID, CorrectsID: doc.CorrectsID, IssueDate: doc.Date, Credit: doc.Credit, Gross: gross.Abs()}
}

// advancePart is what one payment adds to the paid share of an invoice. A
// returned payment takes its part back in the period of the refund.
type advancePart struct {
	Payment       store.Payment
	Date          string
	Before, After decimal.Decimal
	Overpaid      bool
}

// of cuts a part out of an amount of the invoice. Taking the difference of the
// rounded totals keeps the parts adding up to the amount.
func (p advancePart) of(amount decimal.Decimal) decimal.Decimal {
	return amount.Mul(p.After).Round(2).Sub(amount.Mul(p.Before).Round(2))
}

func wholeAmount(amount decimal.Decimal) decimal.Decimal { return amount.Round(2) }

func shareOf(share decimal.Decimal) func(decimal.Decimal) decimal.Decimal {
	return func(amount decimal.Decimal) decimal.Decimal { return amount.Mul(share).Round(2) }
}

// walkAdvance goes through the live payments on an invoice by date and hands
// each one the paid amount before and after it, held between zero and the
// invoice amount. So an overpayment adds nothing, and refunding it takes nothing
// back.
func walkAdvance(doc advanceDoc, payments []store.Payment, visit func(p store.Payment, before, after, total decimal.Decimal)) {
	own := []store.Payment{}
	for _, p := range payments {
		if p.Direction == "outgoing" && p.InvoiceID == doc.ID && p.RevokedAt == "" {
			own = append(own, p)
		}
	}
	sort.SliceStable(own, func(i, j int) bool {
		if own[i].Date != own[j].Date {
			return own[i].Date < own[j].Date
		}
		return own[i].CreatedAt < own[j].CreatedAt
	})
	total, paid := decimal.Zero, decimal.Zero
	for _, p := range own {
		amount, err := decimal.NewFromString(p.Amount)
		if err != nil {
			continue
		}
		total = total.Add(amount)
		after := decimal.Max(decimal.Zero, decimal.Min(total, doc.Gross))
		visit(p, paid, after, total)
		paid = after
	}
}

func advanceParts(doc advanceDoc, payments []store.Payment) []advancePart {
	parts := []advancePart{}
	if !doc.Gross.IsPositive() {
		return parts
	}
	walkAdvance(doc, payments, func(p store.Payment, before, after, total decimal.Decimal) {
		if !after.Equal(before) {
			parts = append(parts, advancePart{Payment: p, Date: p.Date, Before: before.Div(doc.Gross), After: after.Div(doc.Gross), Overpaid: total.GreaterThan(doc.Gross)})
		}
	})
	return parts
}

// overpaidParts lists payments that add nothing because the invoice is already
// fully paid.
func overpaidParts(doc advanceDoc, payments []store.Payment) []advancePart {
	parts := []advancePart{}
	walkAdvance(doc, payments, func(p store.Payment, before, after, total decimal.Decimal) {
		amount, _ := decimal.NewFromString(p.Amount)
		if after.Equal(before) && amount.IsPositive() && total.GreaterThan(doc.Gross) {
			parts = append(parts, advancePart{Payment: p, Date: p.Date, Overpaid: true})
		}
	})
	return parts
}

// allDates is later than any payment date.
const allDates = "9999-12-31"

// advancePaid is the amount paid on an invoice up to and including a day.
func advancePaid(doc advanceDoc, payments []store.Payment, until string) decimal.Decimal {
	paid := decimal.Zero
	walkAdvance(doc, payments, func(p store.Payment, _, after, _ decimal.Decimal) {
		if p.Date <= until {
			paid = after
		}
	})
	return paid
}

// advanceOpen is what is still to come on an invoice as things stand now, not
// as they stood when a period ended: a past period needs no action once a later
// payment has arrived. A credit note closes an advance nobody paid. On a credit
// note itself only money the customer has paid can still go back.
func advanceOpen(doc advanceDoc, all []advanceDoc, payments []store.Payment) decimal.Decimal {
	paid := advancePaid(doc, payments, allDates)
	if doc.Credit {
		for _, original := range all {
			if original.ID == doc.CorrectsID {
				refundable := decimal.Min(doc.Gross, advancePaid(original, payments, allDates))
				return decimal.Max(refundable.Sub(paid), decimal.Zero)
			}
		}
		return decimal.Zero
	}
	open := doc.Gross.Sub(paid)
	for _, credit := range all {
		if credit.Credit && credit.CorrectsID == doc.ID {
			open = open.Sub(credit.Gross)
		}
	}
	return decimal.Max(open, decimal.Zero)
}

func advanceOpenNote(open decimal.Decimal) string {
	return "offen " + taxforms.FormatGerman(open.StringFixed(2), true) + " €"
}

// paidAdvance is what one payment adds to the Umsatzsteuer of a period.
type paidAdvance struct {
	Invoice issuedTax
	Part    advancePart
	Entry   FinanceEntry
}

type issuedAdvanceList struct {
	Paid                 []paidAdvance
	Open, Overpaid, Late []FinanceEntry
}

// issuedAdvances lists what the marked invoices add to the period in in, for
// the finance report and the Voranmeldung alike. Invoices the report cannot
// evaluate stay out here and are reported with their date of supply.
func issuedAdvances(in financeInput) issuedAdvanceList {
	var out issuedAdvanceList
	all := []advanceDoc{}
	for _, v := range in.Issued {
		if v.Advance {
			all = append(all, v.advance())
		}
	}
	for _, v := range in.Issued {
		if !v.Advance || v.Problem != "" || v.Currency != "EUR" || v.Gross.IsZero() {
			continue
		}
		doc := v.advance()
		label := invoiceLabel(v.Number, v.Partner)
		for _, part := range advanceParts(doc, in.Overview.Payments) {
			if !inPeriod(part.Date, in.From, in.To) {
				continue
			}
			e := FinanceEntry{Kind: "payment", ID: part.Payment.ID, Date: part.Date, Label: "Zahlung zu Anzahlungsrechnung " + label}
			e.Note = "anteilig " + part.After.Sub(part.Before).Mul(decimal.NewFromInt(100)).Round(1).String() + " %"
			out.Paid = append(out.Paid, paidAdvance{Invoice: v, Part: part, Entry: e})
			if part.Overpaid {
				out.Overpaid = append(out.Overpaid, FinanceEntry{Kind: "payment", ID: part.Payment.ID, Date: part.Date, Label: label})
			}
		}
		for _, part := range overpaidParts(doc, in.Overview.Payments) {
			if inPeriod(part.Date, in.From, in.To) {
				out.Overpaid = append(out.Overpaid, FinanceEntry{Kind: "payment", ID: part.Payment.ID, Date: part.Date, Label: label})
			}
		}
		if open := advanceOpen(doc, all, in.Overview.Payments); open.IsPositive() && v.IssueDate <= in.To {
			out.Open = append(out.Open, FinanceEntry{Kind: "invoice", ID: v.ID, Date: v.IssueDate, Label: label, Note: advanceOpenNote(open)})
		}
		for _, p := range in.Overview.Payments {
			if p.Direction == "outgoing" && p.InvoiceID == v.ID && inPeriod(p.Date, in.From, in.To) && changedAfterLock(p, in.Locks) {
				out.Late = append(out.Late, FinanceEntry{Kind: "payment", ID: p.ID, Date: p.Date, Label: label})
			}
		}
	}
	return out
}

// changedAfterLock reports a payment recorded or revoked after the period of its
// date was closed by hand. Payments stay append-only, so a late one is possible.
func changedAfterLock(p store.Payment, locks []store.PeriodLock) bool {
	for _, l := range locks {
		if p.Date > l.To || (l.From != "" && p.Date < l.From) {
			continue
		}
		if p.CreatedAt > l.At || p.RevokedAt > l.At {
			return true
		}
	}
	return false
}
