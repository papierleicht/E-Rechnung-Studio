package app

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/shopspring/decimal"
	"papierleicht.de/e-rechnung-studio/internal/invoicexml"
	"papierleicht.de/e-rechnung-studio/internal/model"
	"papierleicht.de/e-rechnung-studio/internal/store"
	"papierleicht.de/e-rechnung-studio/internal/taxforms"
)

// The finance report prepares figures for the user and their tax advisor. It
// is no tax return: every figure lists the records it consists of, and gaps
// are reported as issues instead of being estimated.

// FinanceEntry is one record behind a figure.
type FinanceEntry struct {
	Kind   string `json:"kind"` // payment, invoice, received, booking, vat, balance
	ID     string `json:"id"`
	Date   string `json:"date"`
	Label  string `json:"label"`
	Amount string `json:"amount"`
	Note   string `json:"note,omitempty"`
}

type FinanceFigure struct {
	Label       string         `json:"label"`
	Amount      string         `json:"amount"`
	Explanation string         `json:"explanation"`
	Area        string         `json:"area"`
	Available   bool           `json:"available"`
	Incomplete  bool           `json:"incomplete"`
	Entries     []FinanceEntry `json:"entries"`
}

// FinanceIssue marks data that is missing or excluded. Area names the report
// section whose figures it makes incomplete.
type FinanceIssue struct {
	Code    string         `json:"code"`
	Area    string         `json:"area"`
	Message string         `json:"message"`
	Entries []FinanceEntry `json:"entries"`
}

type TaxRateLine struct {
	Category string `json:"category"`
	Percent  string `json:"percent"`
	Net      string `json:"net"`
	Tax      string `json:"tax"`
}

type FinanceReport struct {
	From          string `json:"from"`
	To            string `json:"to"`
	Today         string `json:"today"`
	TaxBasis      string `json:"taxBasis"`
	SmallBusiness bool   `json:"smallBusiness"`
	Complete      bool   `json:"complete"`

	// Payments: cash actually recorded in the period, open items as of today.
	Inflows     FinanceFigure `json:"inflows"`
	Outflows    FinanceFigure `json:"outflows"`
	Other       FinanceFigure `json:"other"`
	Receivables FinanceFigure `json:"receivables"`
	Payables    FinanceFigure `json:"payables"`

	// EÜR preparation (cash basis).
	Income   FinanceFigure `json:"income"`
	Expenses FinanceFigure `json:"expenses"`
	Result   FinanceFigure `json:"result"`

	// VAT overview.
	OutputTax      FinanceFigure `json:"outputTax"`
	OutputTaxRates []TaxRateLine `json:"outputTaxRates"`
	InputTax       FinanceFigure `json:"inputTax"`
	VATPayable     FinanceFigure `json:"vatPayable"`

	// Liquidity and reserves.
	BankBalance      FinanceFigure `json:"bankBalance"`
	VATReserve       FinanceFigure `json:"vatReserve"`
	IncomeTaxReserve FinanceFigure `json:"incomeTaxReserve"`
	Available        FinanceFigure `json:"available"`
	// PendingTax is what the open invoices add to the tax pot once paid.
	PendingTax FinanceFigure `json:"pendingTax"`

	// NotCovered lists what the report deliberately leaves out.
	NotCovered []string       `json:"notCovered"`
	Issues     []FinanceIssue `json:"issues"`
}

const (
	areaPayments  = "zahlungen"
	areaEUR       = "euer"
	areaVAT       = "ust"
	areaLiquidity = "liquiditaet"
)

type taxRate struct {
	Category      string
	Percent       decimal.Decimal
	Basis, Amount decimal.Decimal
	ReasonCode    string // BT-121, e.g. VATEX-EU-F for the Differenzbesteuerung
}

// issuedTax is an issued invoice reduced to the values the report needs.
// Credit notes carry negative amounts. TaxDate is when the Umsatzsteuer arises
// under Soll-Versteuerung; for corrections see applyCorrectionDates.
type issuedTax struct {
	ID, Number, Partner, Currency, TaxDate string
	BuyerCountry, BuyerVATID               string
	Credit                                 bool
	Gross                                  decimal.Decimal
	Rates                                  []taxRate
	Problem                                string
	Treatment, IssueDate                   string
	// Correction marks a credit note or corrected invoice; CorrectsID names
	// the corrected invoice and Reason is a model.Correction… constant.
	Correction         bool
	CorrectsID, Reason string
	// Advance marks an invoice the user declared an Anzahlungsrechnung. Under
	// Soll-Versteuerung its tax follows the payments, see advance_tax.go.
	Advance bool
}

// marginCodes mark the Differenzbesteuerung (§ 25a UStG) in a tax group.
var marginCodes = map[string]bool{"VATEX-EU-F": true, "VATEX-EU-I": true, "VATEX-EU-J": true}

// margin reports a tax group under the Differenzbesteuerung.
func (v issuedTax) margin(t taxRate) bool {
	return t.Category == "E" && (marginCodes[t.ReasonCode] || strings.HasPrefix(v.Treatment, "margin-"))
}

// documentDated reports a tax group whose Voranmeldungszeitraum follows the
// document even under Ist-Versteuerung: intra-community deliveries (§ 13
// Abs. 1 Nr. 6 UStG) and services for businesses in other member states
// (§ 18b Satz 1 Nr. 2 UStG). They are also the rows of the ZM.
func (v issuedTax) documentDated(t taxRate) bool {
	return documentDatedTax(t.Category, v.BuyerCountry)
}

func documentDatedTax(category, buyerCountry string) bool {
	return category == "K" || (category == "AE" && taxforms.OtherEU(buyerCountry))
}

func (v issuedTax) hasMargin() bool {
	for _, t := range v.Rates {
		if v.margin(t) {
			return true
		}
	}
	return false
}

const marginNotice = "Bei Differenzbesteuerung fehlt die Umsatzsteuer auf deine Marge, weil die App deine Einkaufspreise nicht kennt."

// receivedTax is a stored e-invoice reduced to its tax breakdown. Credit notes
// (type code 381) carry negative amounts, like issued ones.
type receivedTax struct {
	Credit  bool
	Country string // seller country
	Rates   []taxRate
}

// owesTax reports a purchase on which the buyer owes the Umsatzsteuer:
// reverse charge (§ 13b UStG) or an intra-community acquisition.
func (t receivedTax) owesTax() bool {
	for _, r := range t.Rates {
		if r.Category == "AE" || r.Category == "K" {
			return true
		}
	}
	return false
}

type financeInput struct {
	From, To, Today string
	Settings        store.FinanceSettings
	SmallBusiness   bool
	Issued          []issuedTax
	Received        []model.ReceivedInvoice
	// ReceivedTax holds the tax breakdown of received documents with readable
	// invoice data, by ID.
	ReceivedTax map[string]receivedTax
	// InputTaxChoices holds the user's decisions on the Vorsteuer, by document ID.
	InputTaxChoices map[string]model.InputTaxChoice
	// VATFilings and AnnualFilings are the transmission notes by period ID.
	VATFilings    map[string]store.VATFiling
	AnnualFilings map[string]store.VATFiling
	Overview      store.PaymentOverview
	Bookings      []PaymentBooking
	Treatments    map[string]store.BookingTreatment
	// AccountNames maps bank-internal account IDs to their IBAN.
	AccountNames map[string]string
	BankBalances []bankBalance
	// Expenses are receipts captured by hand.
	Expenses []store.Expense
	// Locks are the periods closed by hand.
	Locks []store.PeriodLock
}

// bankBalance is a booked balance as reported by the bank via FinTS.
type bankBalance struct {
	Account, Currency, Date string
	Amount                  decimal.Decimal
}

func (s *InvoiceService) FinanceSettings() (store.FinanceSettings, error) {
	return s.store.FinanceSettings()
}

func (s *InvoiceService) SaveFinanceSettings(f store.FinanceSettings) (store.FinanceSettings, error) {
	return s.store.SaveFinanceSettings(f)
}

func (s *InvoiceService) FinanceReport(from, to string) (FinanceReport, error) {
	start, e1 := time.Parse(time.DateOnly, from)
	end, e2 := time.Parse(time.DateOnly, to)
	if e1 != nil || e2 != nil || end.Before(start) || end.Sub(start) > 6*366*24*time.Hour {
		return FinanceReport{}, errors.New("Wähle einen gültigen Zeitraum.")
	}
	in, err := s.loadFinanceInput(from, to)
	if err != nil {
		return FinanceReport{}, err
	}
	return buildFinanceReport(in), nil
}

// loadFinanceInput gathers everything a finance report reads, valued today.
func (s *InvoiceService) loadFinanceInput(from, to string) (financeInput, error) {
	in := financeInput{From: from, To: to, Today: time.Now().Format(time.DateOnly), Treatments: map[string]store.BookingTreatment{}, AccountNames: map[string]string{}}
	var err error
	if in.Settings, err = s.store.FinanceSettings(); err != nil {
		return in, err
	}
	company, err := s.store.Company()
	if err != nil {
		return in, err
	}
	in.SmallBusiness = company.SmallBusiness
	docs, err := s.store.IssuedDocuments()
	if err != nil {
		return in, err
	}
	for _, d := range docs {
		in.Issued = append(in.Issued, issuedFromDocument(d))
	}
	applyCorrectionDates(in.Issued)
	applyIntraCommunityDates(in.Issued)
	if in.Received, err = s.store.ListReceived(); err != nil {
		return in, err
	}
	in.ReceivedTax = s.receivedTaxes(in.Received)
	if in.InputTaxChoices, err = s.store.InputTaxChoices(); err != nil {
		return in, err
	}
	if in.VATFilings, err = s.store.Filings(store.FilingVAT); err != nil {
		return in, err
	}
	if in.AnnualFilings, err = s.store.Filings(store.FilingAnnual); err != nil {
		return in, err
	}
	if in.Expenses, err = s.store.ActiveExpenses(); err != nil {
		return in, err
	}
	if in.Overview, err = s.store.PaymentOverview(); err != nil {
		return in, err
	}
	if in.Locks, err = s.store.PeriodLocks(); err != nil {
		return in, err
	}
	s.paymentMu.Lock()
	in.Bookings, err = s.paymentBookings()
	s.paymentMu.Unlock()
	if err != nil {
		return in, err
	}
	if s.bank != nil {
		overview, err := s.bank.Overview()
		if err != nil {
			return in, err
		}
		for _, a := range overview.Accounts {
			name := firstNonEmpty(a.Data.IBAN, a.Data.AccountNumber, a.ID)
			in.AccountNames[a.ID] = name
			if a.Balance == nil {
				continue
			}
			scale, err := model.CurrencyScale(a.Balance.Booked.Currency)
			if err != nil {
				continue
			}
			in.BankBalances = append(in.BankBalances, bankBalance{Account: name, Currency: a.Balance.Booked.Currency, Date: a.Balance.Booked.Date, Amount: decimal.NewFromInt(a.Balance.Booked.Amount).Shift(-int32(scale))})
		}
	}
	treatments, err := s.store.BookingTreatments()
	if err != nil {
		return in, err
	}
	for _, t := range treatments {
		in.Treatments[t.BookingID] = t
	}
	return in, nil
}

func issuedFromDocument(d store.IssuedDocument) issuedTax {
	v := issuedTax{ID: d.Draft.ID, Number: d.Draft.Number, Partner: d.Draft.Customer.Name, Currency: d.Draft.Currency, Credit: d.Draft.Kind == "credit-note", BuyerCountry: strings.ToUpper(d.Draft.Customer.Address.Country), BuyerVATID: d.Draft.Customer.VATID,
		Treatment: d.Draft.TaxTreatment, IssueDate: d.Draft.IssueDate, Correction: d.Draft.Kind == "credit-note" || d.Draft.Kind == "corrected-invoice",
		CorrectsID: d.Draft.CorrectsInvoiceID, Reason: d.Draft.CorrectionReason, Advance: advanceDraft(d.Draft)}
	// VAT arises with the supply; the invoice date only stands in when the
	// invoice names neither a delivery date nor a service period.
	v.TaxDate = firstNonEmpty(d.Draft.DeliveryDate, d.Draft.PeriodEnd, d.Draft.IssueDate)
	if v.Currency == "" {
		v.Currency = "EUR"
	}
	if len(d.XML) == 0 {
		v.Problem = "Keine gespeicherte Rechnungsdatei"
		return v
	}
	inv, err := invoicexml.Parse(d.XML)
	if err != nil {
		v.Problem = "Rechnungsdatei nicht auswertbar"
		return v
	}
	sign := decimal.NewFromInt(1)
	if v.Credit {
		sign = sign.Neg()
	}
	v.Gross = inv.GrandTotal.Mul(sign)
	for _, t := range inv.TradeTaxes {
		v.Rates = append(v.Rates, taxRate{Category: t.CategoryCode, Percent: t.Percent, Basis: t.BasisAmount.Mul(sign), Amount: t.CalculatedAmount.Mul(sign), ReasonCode: t.ExemptionReasonCode})
	}
	return v
}

// applyCorrectionDates moves corrections to the Voranmeldungszeitraum in
// which the amount changes (§ 17 Abs. 1 Satz 8 UStG): the issue date of the
// credit note or corrected invoice. A credit note that cancels a faulty
// invoice for a new one keeps the period of the supply, so it offsets the
// original there. Without a recorded reason a credit note for the full amount
// of its invoice counts as such a replacement, as before the reason existed.
func applyCorrectionDates(issued []issuedTax) {
	gross := map[string]decimal.Decimal{}
	for _, v := range issued {
		gross[v.ID] = v.Gross
	}
	for i := range issued {
		v := &issued[i]
		if !v.Correction || v.Reason == model.CorrectionReplace || v.IssueDate == "" {
			continue
		}
		original, ok := gross[v.CorrectsID]
		if v.Reason == "" && ok && v.Credit && v.Gross.Neg().Equal(original) {
			continue
		}
		v.TaxDate = v.IssueDate
	}
}

// applyIntraCommunityDates places an intra-community delivery in the period
// of its invoice, at the latest the month after the delivery (§ 13 Abs. 1
// Nr. 6 UStG); the Zusammenfassende Meldung uses the same date (§ 18a Abs. 8
// UStG). Other tax groups of the same invoice move with it.
func applyIntraCommunityDates(issued []issuedTax) {
	for i := range issued {
		v := &issued[i]
		if v.Correction || v.IssueDate == "" || !v.hasCategory("K") {
			continue
		}
		supplied, err := time.Parse(time.DateOnly, v.TaxDate)
		if err != nil {
			continue
		}
		latest := time.Date(supplied.Year(), supplied.Month()+2, 0, 0, 0, 0, 0, time.UTC).Format(time.DateOnly)
		v.TaxDate = min(v.IssueDate, latest)
	}
}

func (v issuedTax) hasCategory(category string) bool {
	for _, t := range v.Rates {
		if t.Category == category {
			return true
		}
	}
	return false
}

// receivedTaxes returns the tax breakdown of the received documents with
// readable invoice data; for the others the inspected totals stand in.
func (s *InvoiceService) receivedTaxes(list []model.ReceivedInvoice) map[string]receivedTax {
	s.receivedMu.Lock()
	defer s.receivedMu.Unlock()
	if s.receivedTax == nil {
		s.receivedTax = map[string]receivedTax{}
	}
	out := map[string]receivedTax{}
	for _, v := range list {
		// Plain PDF receipts have no totals and no invoice data to read.
		if v.Status == "misimport" || v.Inspection.Totals.Gross == "" {
			continue
		}
		t, ok := s.receivedTax[v.ID]
		if !ok {
			if t, ok = s.readReceivedTax(v.ID); !ok {
				continue // not cached: the store may only have been busy
			}
			s.receivedTax[v.ID] = t
		}
		if t.Rates != nil {
			out[v.ID] = t
		}
	}
	return out
}

// readReceivedTax parses a stored document. ok is false if it could not be
// loaded; a document without readable invoice data has nil Rates.
func (s *InvoiceService) readReceivedTax(id string) (receivedTax, bool) {
	item, data, err := s.store.GetReceived(id)
	if err != nil {
		return receivedTax{}, false
	}
	xml, notice, err := receivedInvoiceXML(data, item.Kind)
	if err != nil || notice != nil || len(xml) == 0 {
		return receivedTax{}, true
	}
	inv, err := invoicexml.Parse(xml)
	if err != nil || len(inv.TradeTaxes) == 0 {
		return receivedTax{}, true
	}
	t := receivedTax{Credit: inv.InvoiceTypeCode == 381, Rates: []taxRate{}}
	if inv.Seller.PostalAddress != nil {
		t.Country = strings.ToUpper(inv.Seller.PostalAddress.CountryID)
	}
	sign := decimal.NewFromInt(1)
	if t.Credit {
		sign = sign.Neg()
	}
	for _, x := range inv.TradeTaxes {
		t.Rates = append(t.Rates, taxRate{Category: x.CategoryCode, Percent: x.Percent, Basis: x.BasisAmount.Mul(sign), Amount: x.CalculatedAmount.Mul(sign), ReasonCode: x.ExemptionReasonCode})
	}
	return t, true
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func money(d decimal.Decimal) string { return d.StringFixed(2) }

func inPeriod(date, from, to string) bool { return date != "" && date >= from && date <= to }

func figure(label, area, explanation string, entries []FinanceEntry) FinanceFigure {
	sum := decimal.Zero
	for _, e := range entries {
		v, _ := decimal.NewFromString(e.Amount)
		sum = sum.Add(v)
	}
	if entries == nil {
		entries = []FinanceEntry{}
	}
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].Date < entries[j].Date })
	return FinanceFigure{Label: label, Area: area, Explanation: explanation, Amount: money(sum), Available: true, Entries: entries}
}

func amountOf(f FinanceFigure) decimal.Decimal {
	v, _ := decimal.NewFromString(f.Amount)
	return v
}

func invoiceLabel(number, partner string) string {
	switch {
	case number != "" && partner != "":
		return number + " · " + partner
	case number != "":
		return number
	case partner != "":
		return partner
	}
	return "Ohne Nummer"
}

var expensePaidNotes = map[string]string{"bank": "per Bank", "cash": "bar", "private": "privat bezahlt"}

var treatmentLabels = map[string]string{
	"reserve": "Rücklage", "transfer": "Umbuchung", "private": "Privat",
	"tax": "Private Steuer", "vat": "Umsatzsteuer (Finanzamt)", "tradetax": "Gewerbesteuer", "other": "Sonstiges",
}

func buildFinanceReport(in financeInput) FinanceReport {
	r := FinanceReport{From: in.From, To: in.To, Today: in.Today, TaxBasis: in.Settings.TaxBasis, SmallBusiness: in.SmallBusiness}
	if r.TaxBasis != "ist" {
		r.TaxBasis = "soll"
	}
	issues := map[string]*FinanceIssue{}
	issue := func(code, area, message string, e FinanceEntry) {
		if issues[code] == nil {
			issues[code] = &FinanceIssue{Code: code, Area: area, Message: message, Entries: []FinanceEntry{}}
		}
		if e.ID != "" {
			issues[code].Entries = append(issues[code].Entries, e)
		}
	}

	issued := map[string]issuedTax{}
	for _, v := range in.Issued {
		issued[v.ID] = v
	}
	received := map[string]model.ReceivedInvoice{}
	for _, v := range in.Received {
		received[v.ID] = v
	}
	// Receipts captured by hand replace the linked booking or PDF document.
	expenseOfBooking, expenseOfReceived := map[string]bool{}, map[string]bool{}
	for _, x := range in.Expenses {
		if x.BookingID != "" {
			expenseOfBooking[x.BookingID] = true
		}
		if x.ReceivedID != "" {
			expenseOfReceived[x.ReceivedID] = true
		}
	}

	// Payments by payment date. Refunds on credit notes reduce income.
	var inflows, outflows []FinanceEntry
	type paidShare struct {
		payment store.Payment
		amount  decimal.Decimal
	}
	var outgoingPaid []paidShare
	for _, p := range in.Overview.Payments {
		if p.RevokedAt != "" || !inPeriod(p.Date, in.From, in.To) {
			continue
		}
		amount, err := decimal.NewFromString(p.Amount)
		if err != nil {
			continue
		}
		var label string
		if p.Direction == "outgoing" {
			inv := issued[p.InvoiceID]
			label = "Zahlung zu " + invoiceLabel(inv.Number, inv.Partner)
			if inv.Credit {
				amount = amount.Neg()
				label = "Auszahlung zu Gutschrift " + invoiceLabel(inv.Number, inv.Partner)
			}
		} else {
			rec := received[p.InvoiceID]
			label = "Zahlung an " + invoiceLabel(rec.Inspection.InvoiceNumber, rec.Inspection.SellerName)
			if in.ReceivedTax[p.InvoiceID].Credit {
				amount = amount.Neg()
				label = "Erstattung zu Gutschrift " + invoiceLabel(rec.Inspection.InvoiceNumber, rec.Inspection.SellerName)
			}
		}
		e := FinanceEntry{Kind: "payment", ID: p.ID, Date: p.Date, Label: label, Amount: money(amount), Note: sourceNote(p.Source)}
		if p.Currency != "EUR" {
			e.Amount = p.Amount + " " + p.Currency
			issue("foreign_currency", areaPayments, "Zahlungen in Fremdwährung sind in keiner Summe enthalten.", e)
			continue
		}
		if p.Direction == "outgoing" {
			inflows = append(inflows, e)
			outgoingPaid = append(outgoingPaid, paidShare{p, amount})
		} else {
			outflows = append(outflows, e)
		}
	}
	var assetOutflows []FinanceEntry
	for _, x := range in.Expenses {
		if x.PaidDate == "" || !inPeriod(x.PaidDate, in.From, in.To) {
			continue
		}
		gross, _ := decimal.NewFromString(x.Gross)
		e := FinanceEntry{Kind: "expense", ID: x.ID, Date: x.PaidDate, Label: "Beleg " + x.Supplier, Amount: money(gross), Note: expensePaidNotes[x.PaidWith]}
		outflows = append(outflows, e)
		if x.Category == "asset" {
			assetOutflows = append(assetOutflows, e)
		}
	}
	r.Inflows = figure("Zahlungseingänge", areaPayments, "Zahlungen auf deine Kundenrechnungen, nach Zahlungsdatum. Auszahlungen zu Gutschriften sind abgezogen.", inflows)
	r.Outflows = figure("Zahlungsausgänge", areaPayments, "Zahlungen auf deine Eingangsrechnungen, nach Zahlungsdatum. Erstattungen zu Gutschriften sind abgezogen.", outflows)

	// Bank movements: everything that is neither allocated nor classified is
	// a gap, because it may be a payment without a document.
	var other, vatMovements, taxPayments []FinanceEntry
	for _, b := range in.Bookings {
		if !inPeriod(b.Data.BookingDate, in.From, in.To) {
			continue
		}
		scale, err := model.CurrencyScale(b.Data.Currency)
		if err != nil {
			continue
		}
		e := FinanceEntry{Kind: "booking", ID: b.ID, Date: b.Data.BookingDate, Label: firstNonEmpty(b.Data.CounterpartyName, b.Data.Description, "Bankumsatz"), Amount: money(decimal.NewFromInt(b.Data.Amount).Shift(-int32(scale)))}
		if expenseOfBooking[b.ID] {
			continue // counted with its receipt
		}
		t := in.Treatments[b.ID]
		if t.Kind != "" {
			e.Note = treatmentLabels[t.Kind]
			if b.Data.Currency != "EUR" {
				issue("foreign_currency", areaPayments, "Zahlungen in Fremdwährung sind in keiner Summe enthalten.", e)
				continue
			}
			other = append(other, e)
			switch t.Kind {
			case "vat":
				vatMovements = append(vatMovements, e)
			case "tax":
				taxPayments = append(taxPayments, e)
			}
			continue
		}
		left, err := bookingUnallocated(b, in.Overview.Payments)
		if err == nil && !left.IsZero() {
			e.Amount = money(left)
			e.Note = "nicht zugeordnet"
			issue("unassigned_bookings", areaPayments, "Bankumsätze ohne Rechnung oder Einordnung. Sie fehlen in den Zahlungen und in der EÜR.", e)
		}
	}
	r.Other = figure("Sonstige Geldbewegungen", areaPayments, "Eingeordnete Bankumsätze ohne Rechnung, z. B. Privatentnahmen, Umbuchungen oder Steuerzahlungen. Sie zählen nicht zu Einnahmen oder Ausgaben.", other)

	// Open items are valued today, independent of the chosen period.
	var receivables, payables []FinanceEntry
	for _, b := range in.Overview.Invoices {
		remaining := b.Collectable()
		if !remaining.IsPositive() {
			continue
		}
		e := FinanceEntry{Kind: "invoice", ID: b.ID, Date: b.DueDate, Label: invoiceLabel(b.Number, b.Partner), Amount: money(remaining)}
		if b.Direction == "incoming" {
			e.Kind = "received"
		}
		if b.DueDate != "" && b.DueDate < in.Today {
			e.Note = "überfällig"
		}
		if b.Currency != "EUR" {
			e.Amount = b.Remaining + " " + b.Currency
			issue("foreign_currency", areaPayments, "Zahlungen in Fremdwährung sind in keiner Summe enthalten.", e)
			continue
		}
		credit := issued[b.ID].Credit
		if b.Direction == "incoming" {
			credit = in.ReceivedTax[b.ID].Credit
		}
		if credit {
			e.Note = firstNonEmpty(e.Note, "Gutschrift")
		}
		// Credit notes turn the direction around: issued ones are paid out,
		// received ones are refunded to the business.
		if (b.Direction == "outgoing") != credit {
			receivables = append(receivables, e)
		} else {
			payables = append(payables, e)
		}
	}
	r.Receivables = figure("Offene Forderungen", areaPayments, "Restbeträge deiner Kundenrechnungen und ausstehende Erstattungen aus Gutschriften deiner Lieferanten, Stand heute. Teilzahlungen sind abgezogen.", receivables)
	r.Payables = figure("Anstehende Zahlungen", areaPayments, "Offene Restbeträge deiner Eingangsrechnungen und auszuzahlende Gutschriften, Stand heute.", payables)

	// EÜR: cash basis, gross amounts including VAT.
	incomeEntries := append([]FinanceEntry(nil), inflows...)
	var expenseEntries []FinanceEntry
	assetIDs := map[string]bool{}
	for _, e := range assetOutflows {
		assetIDs[e.ID] = true
	}
	for _, e := range outflows {
		if !assetIDs[e.ID] {
			expenseEntries = append(expenseEntries, e)
		}
	}
	// Payments to and refunds from the tax office keep their own kind, so the
	// overview can show the Umsatzsteuer share of the expenses.
	for _, e := range vatMovements {
		e.Kind = "vat"
		amount, _ := decimal.NewFromString(e.Amount)
		if amount.IsPositive() {
			incomeEntries = append(incomeEntries, e)
		} else {
			e.Amount = money(amount.Neg())
			expenseEntries = append(expenseEntries, e)
		}
	}
	for _, e := range assetOutflows {
		issue("assets_depreciated", areaEUR, "Anschaffungen für mehrere Jahre werden abgeschrieben und fehlen deshalb hier. Die Abschreibung steht unter „Steuern erledigen“ in der EÜR.", e)
	}
	r.Income = figure("Betriebseinnahmen", areaEUR, "Zahlungseingänge im Zeitraum (Zuflussprinzip), brutto inklusive Umsatzsteuer, dazu Erstattungen des Finanzamts. Ob jeder Eingang steuerlich eine Betriebseinnahme ist, prüft die App nicht.", incomeEntries)
	r.Expenses = figure("Betriebsausgaben", areaEUR, "Bezahlte Eingangsrechnungen und erfasste Belege im Zeitraum (Abflussprinzip), brutto inklusive Vorsteuer, dazu Umsatzsteuer-Zahlungen ans Finanzamt. Ob und in welcher Höhe sie abzugsfähig sind, prüft die App nicht.", expenseEntries)
	result := amountOf(r.Income).Sub(amountOf(r.Expenses))
	r.Result = FinanceFigure{Label: "Ergebnis", Area: areaEUR, Available: true, Amount: money(result), Entries: []FinanceEntry{
		{Kind: "figure", ID: "income", Label: "Betriebseinnahmen", Amount: r.Income.Amount},
		{Kind: "figure", ID: "expenses", Label: "Betriebsausgaben", Amount: money(amountOf(r.Expenses).Neg())},
	}, Explanation: "Einnahmen minus Ausgaben im Zeitraum, rein rechnerisch. Das ist kein Geld auf dem Konto und noch kein steuerlicher Gewinn."}

	// VAT.
	var outputEntries []FinanceEntry
	rates := map[string]*TaxRateLine{}
	addRate := func(t taxRate, part func(decimal.Decimal) decimal.Decimal) decimal.Decimal {
		key := t.Category + "|" + t.Percent.String()
		if rates[key] == nil {
			rates[key] = &TaxRateLine{Category: t.Category, Percent: t.Percent.String(), Net: "0.00", Tax: "0.00"}
		}
		net, _ := decimal.NewFromString(rates[key].Net)
		tax, _ := decimal.NewFromString(rates[key].Tax)
		basis, amount := part(t.Basis), part(t.Amount)
		rates[key].Net, rates[key].Tax = money(net.Add(basis)), money(tax.Add(amount))
		return amount
	}
	if !in.SmallBusiness {
		if r.TaxBasis == "soll" {
			for _, v := range in.Issued {
				if !inPeriod(v.TaxDate, in.From, in.To) {
					continue
				}
				e := FinanceEntry{Kind: "invoice", ID: v.ID, Date: v.TaxDate, Label: invoiceLabel(v.Number, v.Partner)}
				if v.Problem != "" || v.Currency != "EUR" {
					e.Note = firstNonEmpty(v.Problem, "Fremdwährung")
					issue("issued_without_tax", areaVAT, "Kundenrechnungen ohne auswertbare Steuerdaten sind in der Umsatzsteuer nicht enthalten.", e)
					continue
				}
				tax, counted := decimal.Zero, false
				for _, t := range v.Rates {
					if v.Advance && !v.documentDated(t) {
						continue
					}
					tax, counted = tax.Add(addRate(t, wholeAmount)), true
				}
				if v.Advance && !counted {
					continue // all of it follows the payments
				}
				if v.hasMargin() {
					issue("margin_scheme", areaVAT, marginNotice, e)
				}
				e.Amount = money(tax)
				outputEntries = append(outputEntries, e)
			}
			advances := issuedAdvances(in)
			for _, item := range advances.Paid {
				tax := decimal.Zero
				for _, t := range item.Invoice.Rates {
					if !item.Invoice.documentDated(t) {
						tax = tax.Add(addRate(t, item.Part.of))
					}
				}
				e := item.Entry
				if item.Invoice.hasMargin() {
					issue("margin_scheme", areaVAT, marginNotice, e)
				}
				e.Amount = money(tax)
				outputEntries = append(outputEntries, e)
			}
			for _, e := range advances.Open {
				issue("issued_advance_open", areaVAT, advanceOpenMessage, e)
			}
			for _, e := range advances.Overpaid {
				issue("issued_advance_overpaid", areaVAT, advanceOverpaidMessage, e)
			}
			for _, e := range advances.Late {
				issue("issued_advance_late", areaVAT, advanceLateMessage, e)
			}
		} else {
			for _, p := range outgoingPaid {
				v := issued[p.payment.InvoiceID]
				e := FinanceEntry{Kind: "payment", ID: p.payment.ID, Date: p.payment.Date, Label: "Zahlung zu " + invoiceLabel(v.Number, v.Partner)}
				if v.Problem != "" || v.Gross.IsZero() {
					e.Note = firstNonEmpty(v.Problem, "Rechnungsbetrag fehlt")
					issue("issued_without_tax", areaVAT, "Kundenrechnungen ohne auswertbare Steuerdaten sind in der Umsatzsteuer nicht enthalten.", e)
					continue
				}
				// Ist-Versteuerung: tax arises with each payment, in proportion
				// to the invoice's rate breakdown.
				share := p.amount.Div(v.Gross)
				tax := decimal.Zero
				for _, t := range v.Rates {
					tax = tax.Add(addRate(t, shareOf(share)))
				}
				if v.hasMargin() {
					issue("margin_scheme", areaVAT, marginNotice, e)
				}
				e.Amount = money(tax)
				e.Note = "anteilig " + share.Mul(decimal.NewFromInt(100)).Round(1).String() + " %"
				outputEntries = append(outputEntries, e)
			}
		}
	} else {
		// Small businesses still owe the tax on reverse-charge purchases and on
		// intra-community acquisitions with their VAT ID (§ 13b Abs. 8 UStG).
		_, purchases, _ := vatRecords(in)
		for _, p := range purchases {
			note := "Steuerschuld bei dir (Reverse Charge)"
			if p.Category == "K" {
				note = "Einkauf aus einem anderen EU-Land"
			}
			outputEntries = append(outputEntries, FinanceEntry{Kind: p.Entry.Kind, ID: p.Entry.ID, Date: p.Entry.Date, Label: p.Entry.Label, Amount: money(taxforms.OwedTax(p)), Note: note})
		}
	}
	outputExplanation := "Umsatzsteuer aus deinen Kundenrechnungen nach Leistungsdatum (Soll-Versteuerung). Anzahlungsrechnungen zählen stattdessen mit dem Zahlungseingang, nur für den bezahlten Anteil."
	if r.TaxBasis == "ist" {
		outputExplanation = "Umsatzsteuer aus Zahlungseingängen im Zeitraum (Ist-Versteuerung)."
	}
	var inputEntries []FinanceEntry
	if !in.SmallBusiness {
		for _, x := range in.Expenses {
			tax, _ := decimal.NewFromString(x.Tax)
			if !tax.IsPositive() {
				continue
			}
			date := expenseTaxDate(x)
			if date == "" && x.Date <= in.To {
				issue("advance_unpaid", areaVAT, unpaidMessage, FinanceEntry{Kind: "expense", ID: x.ID, Date: x.Date, Label: "Beleg " + x.Supplier})
			}
			if !inPeriod(date, in.From, in.To) {
				continue
			}
			inputEntries = append(inputEntries, FinanceEntry{Kind: "expense", ID: x.ID, Date: date, Label: "Beleg " + x.Supplier, Amount: money(tax)})
		}
		originals := receivedOriginals(in.Received)
		for _, v := range in.Received {
			if v.Status == "misimport" || expenseOfReceived[v.ID] || v.Inspection.IssueDate == "" || v.Inspection.IssueDate > in.To {
				continue
			}
			e := FinanceEntry{Kind: "received", ID: v.ID, Date: v.Inspection.IssueDate, Label: invoiceLabel(v.Inspection.InvoiceNumber, v.Inspection.SellerName)}
			choice := choiceOf(in.InputTaxChoices, v.ID)
			counted := inputTaxFor(v, choice)
			if counted.OnPayment {
				paid, open := onPaymentInputTax(in, v, counted, choice, toTaxEntry(e))
				for _, item := range paid {
					tax := decimal.Zero
					for _, p := range item.Purchases {
						tax = tax.Add(p.Tax)
					}
					inputEntries = append(inputEntries, FinanceEntry{Kind: item.Entry.Kind, ID: item.Entry.ID, Date: item.Entry.Date, Label: item.Entry.Label, Amount: money(tax), Note: item.Entry.Note})
				}
				if open.IsPositive() {
					e.Note = openNote(open)
					issue("advance_unpaid", areaVAT, unpaidMessage, e)
				}
				continue
			}
			if !inPeriod(v.Inspection.IssueDate, in.From, in.To) {
				continue
			}
			if counted.Review != "" {
				e.Note = counted.Review
				issue("received_needs_review", areaVAT, "Eingangsrechnungen mit fehlenden oder ungeprüften Angaben sind in der Vorsteuer nicht eingerechnet. Entscheide im Beleg, ob und wie viel davon zählt.", e)
				continue
			}
			tax := counted.Tax
			if in.ReceivedTax[v.ID].Credit {
				tax, e.Note = tax.Neg(), "Gutschrift"
			}
			if counted.Note != "" {
				e.Note = counted.Note
			}
			e.Amount = money(tax)
			if counted.Remainder {
				issue("received_prepaid_review", areaVAT, prepaidMessage, e)
			}
			// A correction and its invoice both count until the user says which applies
			// (BMF letter of 15.10.2026, Rn. 20 and 21).
			if original, open := correctionOpen(v.ID, originals, in.InputTaxChoices); open {
				correction := e
				correction.Note = "Berichtigung zu " + original.Inspection.InvoiceNumber + ", Steuer beider Rechnungen prüfen"
				issue("received_correction_review", areaVAT, correctionMessage, correction)
				if e.Note == "" {
					e.Note = "Berichtigung zu " + original.Inspection.InvoiceNumber
				}
			}
			inputEntries = append(inputEntries, e)
		}
	}
	// Received invoices without any issue date cannot be placed in a period.
	for _, v := range in.Received {
		if !in.SmallBusiness && v.Status != "misimport" && !expenseOfReceived[v.ID] && v.Inspection.IssueDate == "" {
			issue("received_needs_review", areaVAT, "Eingangsrechnungen mit fehlenden oder ungeprüften Angaben sind in der Vorsteuer nicht eingerechnet. Entscheide im Beleg, ob und wie viel davon zählt.", FinanceEntry{Kind: "received", ID: v.ID, Label: firstNonEmpty(v.Inspection.SellerName, v.Filename), Note: "Rechnungsdatum fehlt"})
		}
	}
	r.OutputTax = figure("Umsatzsteuer", areaVAT, outputExplanation, outputEntries)
	r.InputTax = figure("Vorsteuer", areaVAT, "Steuer deiner Eingangsrechnungen und erfassten Belege nach Rechnungsdatum, Anzahlungen nach Zahlung, wie die App sie vorschlägt oder du sie im Beleg festgelegt hast. Ob du sie abziehen darfst, prüft die App nicht.", inputEntries)
	if in.SmallBusiness {
		r.OutputTax.Explanation = "Als Kleinunternehmer (§ 19 UStG) weist du keine Umsatzsteuer aus. Hier steht nur die Steuer aus Einkäufen, die du mit „Steuerschuld bei dir“ erfasst hast, etwa Werbung oder Software von Anbietern im Ausland."
		r.InputTax.Explanation = "Als Kleinunternehmer (§ 19 UStG) ziehst du keine Vorsteuer ab."
	}
	r.OutputTaxRates = []TaxRateLine{}
	for _, line := range rates {
		r.OutputTaxRates = append(r.OutputTaxRates, *line)
	}
	sort.Slice(r.OutputTaxRates, func(i, j int) bool {
		a, _ := decimal.NewFromString(r.OutputTaxRates[i].Percent)
		b, _ := decimal.NewFromString(r.OutputTaxRates[j].Percent)
		return a.GreaterThan(b) || (a.Equal(b) && r.OutputTaxRates[i].Category < r.OutputTaxRates[j].Category)
	})
	payable := amountOf(r.OutputTax).Sub(amountOf(r.InputTax))
	r.VATPayable = FinanceFigure{Label: "Zahllast", Area: areaVAT, Available: true, Amount: money(payable), Entries: []FinanceEntry{
		{Kind: "figure", ID: "outputTax", Label: "Umsatzsteuer", Amount: r.OutputTax.Amount},
		{Kind: "figure", ID: "inputTax", Label: "Vorsteuer", Amount: money(amountOf(r.InputTax).Neg())},
	}, Explanation: "Umsatzsteuer minus ausgewiesene Vorsteuer, rein rechnerisch. Ein negativer Betrag ist ein möglicher Erstattungsanspruch. Bereits geleistete Vorauszahlungen sind nicht abgezogen."}

	// Liquidity: per account the newer of the bank's balance and the one the
	// user entered counts, so a manual value remains the fallback.
	type dated struct {
		entry  FinanceEntry
		manual bool
	}
	latest := map[string]dated{}
	var order []string
	consider := func(e FinanceEntry, manual bool) {
		key := accountKey(e.ID)
		prev, seen := latest[key]
		if !seen {
			order = append(order, key)
		}
		if !seen || e.Date > prev.entry.Date || (e.Date == prev.entry.Date && !manual) {
			latest[key] = dated{e, manual}
		}
	}
	for _, b := range in.BankBalances {
		e := FinanceEntry{Kind: "balance", ID: b.Account, Date: b.Date, Label: b.Account, Amount: money(b.Amount), Note: "von der Bank abgerufen"}
		if b.Currency != "EUR" {
			e.Amount = money(b.Amount) + " " + b.Currency
			issue("foreign_currency", areaPayments, "Zahlungen in Fremdwährung sind in keiner Summe enthalten.", e)
			continue
		}
		consider(e, false)
	}
	for _, b := range in.Settings.Balances {
		consider(FinanceEntry{Kind: "balance", ID: b.AccountID, Date: b.Date, Label: b.AccountID, Amount: b.Amount, Note: "manuell erfasst"}, true)
	}
	// Roll each balance forward with bookings imported for a later date, e.g.
	// CSV statements after a manually entered balance.
	type rolled struct {
		sum   decimal.Decimal
		count int
		last  string
	}
	forward := map[string]*rolled{}
	for _, b := range in.Bookings {
		key := accountKey(firstNonEmpty(in.AccountNames[b.AccountID], b.AccountID))
		base, ok := latest[key]
		if !ok || b.Data.Currency != "EUR" || b.Data.BookingDate <= base.entry.Date {
			continue
		}
		if forward[key] == nil {
			forward[key] = &rolled{sum: decimal.Zero}
		}
		f := forward[key]
		f.sum = f.sum.Add(decimal.NewFromInt(b.Data.Amount).Shift(-2))
		f.count++
		if b.Data.BookingDate > f.last {
			f.last = b.Data.BookingDate
		}
	}
	var balanceEntries []FinanceEntry
	for _, key := range order {
		e := latest[key].entry
		if f := forward[key]; f != nil {
			amount, _ := decimal.NewFromString(e.Amount)
			e.Amount = money(amount.Add(f.sum))
			e.Note += fmt.Sprintf(", fortgerechnet mit %d späteren Umsätzen (%s €)", f.count, money(f.sum))
			e.Date = f.last
		}
		if days := daysBetween(e.Date, in.Today); days > 7 {
			e.Note += fmt.Sprintf(", Stand vor %d Tagen", days)
			issue("stale_balance", areaLiquidity, "Einige Kontostände sind älter als eine Woche. Ein neuer Bankabruf oder Eintrag aktualisiert sie.", e)
		}
		balanceEntries = append(balanceEntries, e)
	}
	for _, b := range in.Bookings {
		label := firstNonEmpty(in.AccountNames[b.AccountID], b.AccountID)
		if key := accountKey(label); latest[key].entry.ID == "" && key != "" {
			latest[key] = dated{entry: FinanceEntry{ID: label}}
			issue("missing_balance", areaLiquidity, "Für diese Konten ist kein Kontostand vorhanden. Ein Bankabruf holt ihn automatisch, sonst trage ihn manuell ein.", FinanceEntry{Kind: "balance", ID: label, Label: label})
		}
	}
	r.BankBalance = figure("Kontostand", areaLiquidity, "Je Konto der neueste Stand: von der Bank abgerufen oder von dir eingetragen. Das ist Geld auf dem Konto, nicht dein Ergebnis.", balanceEntries)
	if len(balanceEntries) == 0 {
		r.BankBalance.Available = false
		issue("missing_balance", areaLiquidity, "Für diese Konten ist kein Kontostand vorhanden. Ein Bankabruf holt ihn automatisch, sonst trage ihn manuell ein.", FinanceEntry{})
	}
	// The Umsatzsteuer reserve is what is still owed today, period by period,
	// so payments already made are not set aside a second time.
	r.VATReserve = figure("Offene Umsatzsteuer", areaLiquidity, "Umsatzsteuer, die noch ans Finanzamt geht: laufende und noch nicht fällige Zeiträume sowie fällige ohne Übermittlungsvermerk. Stand heute, unabhängig vom gewählten Zeitraum.", openVAT(in))
	vatReserve := amountOf(r.VATReserve)
	r.IncomeTaxReserve = FinanceFigure{Label: "Rücklage Einkommensteuer", Area: areaLiquidity, Entries: []FinanceEntry{}, Amount: "0.00"}
	if in.Settings.ReservePercent == "" {
		r.IncomeTaxReserve.Explanation = "Lege einen Prozentsatz fest, den du vom Ergebnis zurücklegen möchtest."
		issue("missing_reserve_rate", areaLiquidity, "Für die Einkommensteuer-Rücklage ist noch kein Prozentsatz festgelegt.", FinanceEntry{})
	} else {
		pct, _ := decimal.NewFromString(in.Settings.ReservePercent)
		reserve := decimal.Max(result, decimal.Zero).Mul(pct).Div(decimal.NewFromInt(100)).Round(2)
		paid := decimal.Zero
		for _, e := range taxPayments {
			amount, _ := decimal.NewFromString(e.Amount)
			paid = paid.Sub(amount) // payments are negative
		}
		paid = decimal.Min(decimal.Max(paid, decimal.Zero), reserve)
		r.IncomeTaxReserve.Available = true
		r.IncomeTaxReserve.Amount = money(reserve.Sub(paid))
		r.IncomeTaxReserve.Explanation = "Dein selbst gewählter Satz von " + pct.String() + " % auf ein positives Ergebnis im Zeitraum, abzüglich bereits gezahlter Einkommensteuer (als „Private Steuer“ eingeordnet). Das ist keine Steuerberechnung."
		r.IncomeTaxReserve.Entries = []FinanceEntry{{Kind: "figure", ID: "result", Label: pct.String() + " % vom Ergebnis", Amount: money(reserve), Note: "Ergebnis " + r.Result.Amount + " €"}}
		if paid.IsPositive() {
			r.IncomeTaxReserve.Entries = append(r.IncomeTaxReserve.Entries, FinanceEntry{Kind: "figure", ID: "taxPaid", Label: "Bereits gezahlte Einkommensteuer", Amount: money(paid.Neg())})
		}
	}
	taxPaid := decimal.Zero
	for _, e := range taxPayments {
		amount, _ := decimal.NewFromString(e.Amount)
		taxPaid = taxPaid.Sub(amount) // payments are negative
	}
	r.PendingTax = pendingTax(in, issued, receivables, payables, result, taxPaid)
	r.Available = FinanceFigure{Label: "Frei verfügbar", Area: areaLiquidity, Entries: []FinanceEntry{}, Amount: "0.00", Explanation: "Kontostand abzüglich anstehender Zahlungen, offener Umsatzsteuer und Rücklage. Offene Forderungen sind nicht eingerechnet, weil das Geld noch nicht da ist."}
	if r.BankBalance.Available {
		available := amountOf(r.BankBalance).Sub(amountOf(r.Payables)).Sub(vatReserve).Sub(amountOf(r.IncomeTaxReserve))
		r.Available.Available = true
		r.Available.Amount = money(available)
		r.Available.Entries = []FinanceEntry{
			{Kind: "figure", ID: "bankBalance", Label: "Kontostand", Amount: r.BankBalance.Amount},
			{Kind: "figure", ID: "payables", Label: "Anstehende Zahlungen", Amount: money(amountOf(r.Payables).Neg())},
			{Kind: "figure", ID: "vatReserve", Label: "Offene Umsatzsteuer", Amount: money(vatReserve.Neg())},
			{Kind: "figure", ID: "incomeTaxReserve", Label: "Rücklage Einkommensteuer", Amount: money(amountOf(r.IncomeTaxReserve).Neg())},
		}
	}

	r.NotCovered = []string{
		"Abschreibungen und die 10-Tage-Regel: Beides berücksichtigt die EÜR unter „Steuern“",
		"Einkommensteuer-Prognose: Dafür fehlen ein geprüftes Modell und deine persönlichen Angaben",
	}
	r.Issues = []FinanceIssue{}
	incomplete := map[string]bool{}
	for _, code := range []string{"unassigned_bookings", "received_needs_review", "received_prepaid_review", "received_correction_review", "issued_without_tax", "issued_advance_overpaid", "issued_advance_late", "margin_scheme", "assets_depreciated", "foreign_currency", "missing_balance", "stale_balance", "missing_reserve_rate"} {
		if v := issues[code]; v != nil {
			r.Issues = append(r.Issues, *v)
			incomplete[v.Area] = true
		}
	}
	// An open advance leaves no gap in the figures: its tax does not count yet.
	for _, key := range []string{"advance_unpaid", "issued_advance_open"} {
		if v := issues[key]; v != nil {
			r.Issues = append(r.Issues, *v)
		}
	}
	// Unassigned bank movements may be undocumented income or expenses.
	if incomplete[areaPayments] {
		incomplete[areaEUR] = true
	}
	if incomplete[areaEUR] || incomplete[areaVAT] {
		incomplete[areaLiquidity] = true
	}
	for _, f := range []*FinanceFigure{&r.Inflows, &r.Outflows, &r.Other, &r.Receivables, &r.Payables, &r.Income, &r.Expenses, &r.Result, &r.OutputTax, &r.InputTax, &r.VATPayable, &r.BankBalance, &r.VATReserve, &r.IncomeTaxReserve, &r.Available} {
		f.Incomplete = incomplete[f.Area]
	}
	r.Complete = len(r.Issues) == 0
	return r
}

// openVAT lists the Umsatzsteuer still to be paid as of today: periods that
// are running or not yet due, and due periods without a transmission note.
// Transmitted periods past their due date count as paid. Without
// Voranmeldungen the annual return stands in for the periods.
func openVAT(in financeInput) []FinanceEntry {
	year := yearOf(in.Today)
	if year == 0 {
		return nil
	}
	setup := taxSetup(in.Settings, in.SmallBusiness)
	periods, filings := []taxforms.Period{}, in.VATFilings
	switch setup.VATPeriod {
	case "monthly", "quarterly":
		periods = append(taxforms.Periods(year-1, setup.VATPeriod, setup.VATExtension), taxforms.Periods(year, setup.VATPeriod, setup.VATExtension)...)
	default:
		filings = in.AnnualFilings
		for _, y := range []int{year - 1, year} {
			periods = append(periods, taxforms.Period{ID: strconv.Itoa(y), Label: strconv.Itoa(y), From: fmt.Sprintf("%d-01-01", y), To: fmt.Sprintf("%d-12-31", y), Due: taxforms.AnnualDeadline(y)})
		}
	}
	firstDue := fmt.Sprintf("%d-01-01", year)
	var out []FinanceEntry
	for _, p := range periods {
		if p.From > in.Today || p.Due < firstDue {
			continue
		}
		payable, err := decimal.NewFromString(buildVATReturn(in, p, setup).Payable)
		if err != nil {
			continue
		}
		note := "fällig " + germanDate(p.Due)
		if p.To >= in.Today {
			note = "läuft noch, " + note
		}
		// A transmitted period counts as paid once due; only a later increase
		// is still owed, with a corrected return.
		if f, filed := filings[p.ID]; filed && p.Due < in.Today {
			paid, err := decimal.NewFromString(f.Payable)
			if err != nil {
				continue
			}
			payable, note = payable.Sub(paid), "Nachzahlung nach Änderung seit der Übermittlung"
		}
		if payable.IsPositive() {
			out = append(out, FinanceEntry{Kind: "figure", ID: "vat:" + p.ID, Date: p.Due, Label: "Umsatzsteuer " + p.Label, Amount: money(payable), Note: note})
		}
	}
	return out
}

// accountKey matches an IBAN typed with spaces or in lower case.
func accountKey(id string) string { return strings.ToUpper(strings.ReplaceAll(id, " ", "")) }

func sourceNote(source string) string {
	switch source {
	case "bank":
		return "Bankabruf"
	case "csv":
		return "CSV-Import"
	case "shop":
		return "im Shop bezahlt"
	}
	return "manuell erfasst"
}

func daysBetween(from, to string) int {
	a, e1 := time.Parse(time.DateOnly, from)
	b, e2 := time.Parse(time.DateOnly, to)
	if e1 != nil || e2 != nil {
		return 0
	}
	return int(b.Sub(a).Hours() / 24)
}
