package app

import (
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"papierleicht.de/e-rechnung-studio/internal/model"
	"papierleicht.de/e-rechnung-studio/internal/store"
	"papierleicht.de/e-rechnung-studio/internal/taxforms"

	"github.com/shopspring/decimal"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// Self-filing: receipts without e-invoice data, expense categories, and the
// values a business without a tax advisor types into ELSTER (Umsatzsteuer-
// Voranmeldung and Anlage EÜR). The app prepares; the user transmits.

func validExpenseCategory(id string) bool {
	_, ok := taxforms.CategoryByID(id)
	return ok
}

// ExpenseCategories are the Anlage EÜR groups offered for receipts.
func (s *InvoiceService) ExpenseCategories() []taxforms.Category { return taxforms.Categories }

func (s *InvoiceService) Expenses() ([]store.Expense, error) { return s.store.Expenses() }

// ExpenseUpload carries an attached file base64-encoded from the UI.
type ExpenseUpload struct {
	Expense store.Expense `json:"expense"`
	// File is base64; empty keeps the stored attachment.
	File string `json:"file"`
}

func (s *InvoiceService) SaveExpense(u ExpenseUpload) (store.Expense, error) {
	e := u.Expense
	e.Attachment = nil
	if u.File != "" {
		if len(u.File) > base64.StdEncoding.EncodedLen(store.MaxExpenseAttachment) {
			return e, errors.New("Die Datei ist größer als 10 MB.")
		}
		data, err := base64.StdEncoding.DecodeString(u.File)
		if err != nil {
			return e, errors.New("Die Datei lässt sich nicht lesen.")
		}
		e.Attachment = data
	}
	saved, err := s.store.SaveExpense(e, validExpenseCategory, taxforms.AssetCategory)
	saved.Attachment = nil
	return saved, err
}

// CancelExpense takes a receipt out of all figures; it stays visible with its reason.
func (s *InvoiceService) CancelExpense(id, reason string) (store.Expense, error) {
	return s.store.CancelExpense(id, reason)
}

// OpenExpenseAttachment shows the stored receipt file in the system viewer.
func (s *InvoiceService) OpenExpenseAttachment(id string) error {
	e, err := s.store.ExpenseAttachment(id)
	if err != nil {
		return err
	}
	if len(e.Attachment) == 0 {
		return errors.New("Für diesen Beleg ist keine Datei gespeichert.")
	}
	app := application.Get()
	if app == nil {
		return errors.New("Dateien kannst du nur in der App öffnen.")
	}
	ext := strings.ToLower(filepath.Ext(e.AttachmentName))
	if ext == "" {
		ext = ".pdf"
	}
	file, err := os.CreateTemp("", "erechnungstudio-beleg-*"+ext)
	if err != nil {
		return err
	}
	_, err = file.Write(e.Attachment)
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		os.Remove(file.Name())
		return err
	}
	return app.Browser.OpenFile(file.Name())
}

func (s *InvoiceService) ReceivedCategories() (map[string]store.ReceivedCategory, error) {
	return s.store.ReceivedCategories()
}

func (s *InvoiceService) SaveReceivedCategory(c store.ReceivedCategory) error {
	return s.store.SaveReceivedCategory(c, validExpenseCategory, taxforms.AssetCategory)
}

// VATPeriodStatus is one Voranmeldungszeitraum with its filing state.
type VATPeriodStatus struct {
	Period  taxforms.Period  `json:"period"`
	Filing  *store.VATFiling `json:"filing,omitempty"`
	Overdue bool             `json:"overdue"`
	// Started is false for periods that have not begun yet.
	Started bool `json:"started"`
	// Changed marks a transmitted period whose result differs from the
	// transmitted one, e.g. after a late receipt or a correction.
	Changed bool `json:"changed,omitempty"`
}

// TaxSetup tells the UI which forms apply.
type TaxSetup struct {
	SmallBusiness     bool   `json:"smallBusiness"`
	VATPeriod         string `json:"vatPeriod"`
	VATExtension      bool   `json:"vatExtension"`
	SpecialPrepayment string `json:"specialPrepayment"`
	TaxBasis          string `json:"taxBasis"`
	// Filing is "self" (ELSTER by the user) or "advisor" (a tax advisor files
	// and receives the DATEV export).
	Filing string `json:"filing"`
	// Activity is "trade", "freelance" or empty while not chosen.
	Activity string `json:"activity"`
}

func (s *InvoiceService) TaxSetup() (TaxSetup, error) {
	settings, err := s.store.FinanceSettings()
	if err != nil {
		return TaxSetup{}, err
	}
	company, err := s.store.Company()
	if err != nil {
		return TaxSetup{}, err
	}
	return taxSetup(settings, company.SmallBusiness), nil
}

func taxSetup(settings store.FinanceSettings, smallBusiness bool) TaxSetup {
	basis := settings.TaxBasis
	if basis != "ist" {
		basis = "soll"
	}
	filing := settings.Filing
	if filing != "advisor" {
		filing = "self"
	}
	return TaxSetup{SmallBusiness: smallBusiness, VATPeriod: effectiveVATPeriod(settings, smallBusiness), VATExtension: settings.VATExtension,
		SpecialPrepayment: settings.SpecialPrepayment, TaxBasis: basis, Filing: filing, Activity: settings.Activity}
}

// VATPeriods lists the year's Voranmeldungszeiträume and marks transmitted
// ones whose result has changed since.
func (s *InvoiceService) VATPeriods(year int) ([]VATPeriodStatus, error) {
	periods, err := s.vatPeriods(year, time.Now().Format(time.DateOnly))
	if err != nil {
		return periods, err
	}
	filed := false
	for _, p := range periods {
		filed = filed || p.Filing != nil
	}
	if !filed {
		return periods, nil
	}
	setup, err := s.TaxSetup()
	if err != nil {
		return nil, err
	}
	in, err := s.loadFinanceInput(fmt.Sprintf("%d-01-01", year), fmt.Sprintf("%d-12-31", year))
	if err != nil {
		return nil, err
	}
	for i, p := range periods {
		if p.Filing != nil {
			periods[i].Changed = payableChanged(*p.Filing, buildVATReturn(in, p.Period, setup).Payable)
		}
	}
	return periods, nil
}

func (s *InvoiceService) vatPeriods(year int, today string) ([]VATPeriodStatus, error) {
	setup, err := s.TaxSetup()
	if err != nil {
		return []VATPeriodStatus{}, err
	}
	var owed []string
	if setup.SmallBusiness {
		if owed, err = s.smallBusinessTaxDates(); err != nil {
			return nil, err
		}
	}
	filings, err := s.store.VATFilings()
	if err != nil {
		return nil, err
	}
	out := []VATPeriodStatus{}
	for _, p := range taxforms.Periods(year, setup.VATPeriod, setup.VATExtension) {
		f, filed := filings[p.ID]
		// A small business files only for periods in which it owes tax.
		if setup.SmallBusiness && !filed && !anyInPeriod(owed, p.From, p.To) {
			continue
		}
		st := VATPeriodStatus{Period: p, Started: p.From <= today}
		if filed {
			st.Filing = &f
		} else {
			st.Overdue = p.Due < today
		}
		out = append(out, st)
	}
	return out, nil
}

// smallBusinessTaxDates lists the dates of purchases on which a small business
// owes the Umsatzsteuer itself (§ 13b Abs. 8, § 18 Abs. 4a UStG): received
// e-invoices with reverse charge or an intra-community acquisition, and
// receipts marked as reverse charge.
func (s *InvoiceService) smallBusinessTaxDates() ([]string, error) {
	received, err := s.store.ListReceived()
	if err != nil {
		return nil, err
	}
	expenses, err := s.store.ActiveExpenses()
	if err != nil {
		return nil, err
	}
	overview, err := s.store.PaymentOverview()
	if err != nil {
		return nil, err
	}
	taxes := s.receivedTaxes(received)
	var dates []string
	for _, v := range received {
		if v.Status == "misimport" || !taxes[v.ID].owesTax() {
			continue
		}
		if !inputTaxFor(v, nil).OnPayment {
			dates = append(dates, v.Inspection.IssueDate)
			continue
		}
		parts, _ := paidParts(v, overview.Payments)
		for _, part := range parts {
			dates = append(dates, part.Date)
		}
	}
	for _, x := range expenses {
		if store.ReverseChargeRate(x.TaxRate) {
			dates = append(dates, expenseTaxDate(x))
		}
	}
	return dates, nil
}

func anyInPeriod(dates []string, from, to string) bool {
	for _, d := range dates {
		if inPeriod(d, from, to) {
			return true
		}
	}
	return false
}

// payableChanged compares a transmitted result with today's.
func payableChanged(f store.VATFiling, payable string) bool {
	filed, err1 := decimal.NewFromString(f.Payable)
	now, err2 := decimal.NewFromString(payable)
	return err1 == nil && err2 == nil && !filed.Equal(now)
}

// VATReturnView is the Voranmeldung of one period ready for ELSTER.
type VATReturnView struct {
	Period taxforms.Period    `json:"period"`
	Return taxforms.VATReturn `json:"return"`
	Filing *store.VATFiling   `json:"filing,omitempty"`
}

func (s *InvoiceService) VATReturn(periodID string) (VATReturnView, error) {
	setup, err := s.TaxSetup()
	if err != nil {
		return VATReturnView{}, err
	}
	p, ok := taxforms.PeriodByID(periodID, setup.VATPeriod, setup.VATExtension)
	if !ok {
		return VATReturnView{}, errors.New("Lege zuerst fest, wie oft du die Voranmeldung abgibst.")
	}
	in, err := s.loadFinanceInput(p.From, p.To)
	if err != nil {
		return VATReturnView{}, err
	}
	view := VATReturnView{Period: p, Return: buildVATReturn(in, p, setup)}
	if filings, err := s.store.VATFilings(); err == nil {
		if f, ok := filings[p.ID]; ok {
			view.Filing = &f
			if payableChanged(f, view.Return.Payable) {
				changed := taxforms.Issue{Code: "changed_after_filing", Entries: []taxforms.Entry{}, Message: fmt.Sprintf(
					"Seit der Übermittlung am %s hat sich das Ergebnis geändert: übermittelt %s €, jetzt %s €. Prüfe die geänderten Belege und gib bei Bedarf eine berichtigte Voranmeldung ab.",
					germanDate(f.SubmittedOn), taxforms.FormatGerman(f.Payable, true), taxforms.FormatGerman(view.Return.Payable, true))}
				view.Return.Issues = append([]taxforms.Issue{changed}, view.Return.Issues...)
			}
		}
	}
	return view, nil
}

// VATYear adds up a year from the same records as the Voranmeldungen, as the
// starting point for the annual return.
func (s *InvoiceService) VATYear(year int) (VATReturnView, error) {
	setup, err := s.TaxSetup()
	if err != nil {
		return VATReturnView{}, err
	}
	p := taxforms.Period{ID: fmt.Sprint(year), Label: fmt.Sprint(year), From: fmt.Sprintf("%d-01-01", year), To: fmt.Sprintf("%d-12-31", year)}
	in, err := s.loadFinanceInput(p.From, p.To)
	if err != nil {
		return VATReturnView{}, err
	}
	return VATReturnView{Period: p, Return: buildVATReturn(in, p, setup)}, nil
}

// buildVATReturn prepares the Voranmeldung of period p from loaded records.
func buildVATReturn(in financeInput, p taxforms.Period, setup TaxSetup) taxforms.VATReturn {
	in.From, in.To = p.From, p.To
	sales, purchases, issues := vatRecords(in)
	special := decimal.Zero
	if p.Last && setup.VATExtension {
		special, _ = decimal.NewFromString(setup.SpecialPrepayment)
	}
	r := taxforms.BuildVATReturn(sales, purchases, special)
	r.Issues = append(issues, r.Issues...)
	return r
}

func (s *InvoiceService) MarkVATFiled(periodID, submittedOn, payable string) error {
	return s.store.SaveVATFiling(store.VATFiling{PeriodID: periodID, SubmittedOn: submittedOn, Payable: payable})
}

func (s *InvoiceService) UnmarkVATFiled(periodID string) error {
	return s.store.DeleteVATFiling(periodID)
}

func toTaxEntry(e FinanceEntry) taxforms.Entry {
	return taxforms.Entry{Kind: e.Kind, ID: e.ID, Date: e.Date, Label: e.Label, Amount: e.Amount, Note: e.Note}
}

type issueSet struct {
	order []string
	byKey map[string]*taxforms.Issue
}

func (s *issueSet) add(code, message string, e taxforms.Entry) {
	if s.byKey == nil {
		s.byKey = map[string]*taxforms.Issue{}
	}
	if s.byKey[code] == nil {
		s.byKey[code] = &taxforms.Issue{Code: code, Message: message, Entries: []taxforms.Entry{}}
		s.order = append(s.order, code)
	}
	if e.ID != "" {
		s.byKey[code].Entries = append(s.byKey[code].Entries, e)
	}
}

func (s *issueSet) list() []taxforms.Issue {
	out := []taxforms.Issue{}
	for _, code := range s.order {
		out = append(out, *s.byKey[code])
	}
	return out
}

// vatRecords collects the period's sales and purchases for the Voranmeldung.
// A small business reports only the tax it owes as a recipient: reverse-charge
// purchases and intra-community acquisitions, without input tax (§ 13b
// Abs. 8, § 18 Abs. 4a UStG). Its own sales are exempt and stay out.
func vatRecords(in financeInput) ([]taxforms.Sale, []taxforms.Purchase, []taxforms.Issue) {
	var issues issueSet
	var sales []taxforms.Sale
	issued := map[string]issuedTax{}
	for _, v := range in.Issued {
		issued[v.ID] = v
	}
	// Under Ist-Versteuerung payments carry the tax groups that follow the
	// payment, the invoices the others (documentDated).
	ist := in.Settings.TaxBasis == "ist"
	addSale := func(v issuedTax, e taxforms.Entry, part func(decimal.Decimal) decimal.Decimal, fromPayment bool) {
		for _, t := range v.Rates {
			if (ist || v.Advance) && fromPayment == v.documentDated(t) {
				continue
			}
			sales = append(sales, taxforms.Sale{Entry: e, Category: t.Category, Percent: t.Percent,
				Net: part(t.Basis), Tax: part(t.Amount), BuyerCountry: v.BuyerCountry, Margin: v.margin(t)})
		}
	}
	addInvoices := func() {
		for _, v := range in.Issued {
			if !inPeriod(v.TaxDate, in.From, in.To) {
				continue
			}
			e := taxforms.Entry{Kind: "invoice", ID: v.ID, Date: v.TaxDate, Label: invoiceLabel(v.Number, v.Partner)}
			if v.Problem != "" || v.Currency != "EUR" {
				if !ist { // under Ist-Versteuerung the payment reports it
					issues.add("issued_without_tax", "Kundenrechnungen ohne auswertbare Steuerdaten oder in Fremdwährung sind nicht enthalten.", e)
				}
				continue
			}
			addSale(v, e, wholeAmount, false)
		}
	}
	switch {
	case in.SmallBusiness:
	case in.Settings.TaxBasis == "ist":
		for _, p := range in.Overview.Payments {
			if p.RevokedAt != "" || p.Direction != "outgoing" || !inPeriod(p.Date, in.From, in.To) {
				continue
			}
			v := issued[p.InvoiceID]
			amount, err := decimal.NewFromString(p.Amount)
			e := taxforms.Entry{Kind: "payment", ID: p.ID, Date: p.Date, Label: "Zahlung zu " + invoiceLabel(v.Number, v.Partner)}
			if err != nil || v.Problem != "" || v.Gross.IsZero() || p.Currency != "EUR" {
				issues.add("issued_without_tax", "Zahlungen zu Rechnungen ohne auswertbare Steuerdaten oder in Fremdwährung sind nicht enthalten.", e)
				continue
			}
			if v.Credit {
				amount = amount.Neg()
			}
			addSale(v, e, shareOf(amount.Div(v.Gross)), true)
		}
		addInvoices()
	default:
		addInvoices()
		advances := issuedAdvances(in)
		for _, item := range advances.Paid {
			addSale(item.Invoice, toTaxEntry(item.Entry), item.Part.of, true)
		}
		for _, e := range advances.Open {
			issues.add("issued_advance_open", advanceOpenMessage, toTaxEntry(e))
		}
		for _, e := range advances.Overpaid {
			issues.add("issued_advance_overpaid", advanceOverpaidMessage, toTaxEntry(e))
		}
		for _, e := range advances.Late {
			issues.add("issued_advance_late", advanceLateMessage, toTaxEntry(e))
		}
	}

	var purchases []taxforms.Purchase
	linked := map[string]bool{}
	for _, x := range in.Expenses {
		if x.ReceivedID != "" {
			linked[x.ReceivedID] = true
		}
		tax, _ := decimal.NewFromString(x.Tax)
		net, _ := decimal.NewFromString(x.Net)
		date := expenseTaxDate(x)
		e := taxforms.Entry{Kind: "expense", ID: x.ID, Date: date, Label: x.Supplier}
		reverse := store.ReverseChargeRate(x.TaxRate)
		if !reverse && (!tax.IsPositive() || in.SmallBusiness) {
			continue
		}
		if date == "" && x.Date <= in.To {
			e.Date = x.Date
			issues.add("advance_unpaid", unpaidMessage, e)
		}
		if !inPeriod(date, in.From, in.To) {
			continue
		}
		if reverse {
			purchases = append(purchases, taxforms.Purchase{Entry: e, Category: "AE", Net: net, EU: x.TaxRate == store.RateReverseChargeEU, NoInputTax: in.SmallBusiness})
		} else {
			purchases = append(purchases, taxforms.Purchase{Entry: e, Category: "S", Net: net, Tax: tax})
		}
	}
	originals := receivedOriginals(in.Received)
	for _, v := range in.Received {
		if v.Status == "misimport" || linked[v.ID] || v.Inspection.IssueDate == "" || v.Inspection.IssueDate > in.To {
			continue
		}
		t, parsed := in.ReceivedTax[v.ID]
		if in.SmallBusiness && !t.owesTax() {
			continue
		}
		e := taxforms.Entry{Kind: "received", ID: v.ID, Date: v.Inspection.IssueDate, Label: invoiceLabel(v.Inspection.InvoiceNumber, v.Inspection.SellerName)}
		if t.Credit {
			e.Label = "Gutschrift " + e.Label
		}
		// Small businesses deduct nothing, so there is nothing for them to decide.
		var choice *model.InputTaxChoice
		if !in.SmallBusiness {
			choice = choiceOf(in.InputTaxChoices, v.ID)
		}
		counted := inputTaxFor(v, choice)
		if counted.OnPayment {
			paid, open := onPaymentInputTax(in, v, counted, choice, e)
			for _, item := range paid {
				purchases = append(purchases, item.Purchases...)
			}
			if open.IsPositive() {
				e.Note = openNote(open)
				issues.add("advance_unpaid", unpaidMessage, e)
			}
			continue
		}
		if !inPeriod(v.Inspection.IssueDate, in.From, in.To) {
			continue
		}
		if counted.Review != "" {
			message := "Eingangsrechnungen ohne geprüfte Rechnungsdaten oder in Fremdwährung sind in der Vorsteuer nicht eingerechnet. Entscheide im Beleg, ob und wie viel davon zählt."
			if in.SmallBusiness {
				message = "Eingangsrechnungen mit Steuerschuld bei dir, die nicht geprüft oder nicht in Euro sind, fehlen. Rechne sie in Euro um und trage sie selbst ein."
			}
			issues.add("received_needs_review", message, e)
			continue
		}
		if counted.Excluded {
			continue
		}
		if counted.Remainder {
			issues.add("received_prepaid_review", prepaidMessage, e)
		}
		if original, open := correctionOpen(v.ID, originals, in.InputTaxChoices); open {
			correction := e
			correction.Note = "Berichtigung zu " + original.Inspection.InvoiceNumber + ", Steuer beider Rechnungen prüfen"
			issues.add("received_correction_review", correctionMessage, correction)
		}
		purchases = append(purchases, countedPurchases(v, t, parsed, counted, choice, in.SmallBusiness, e)...)
	}
	for _, v := range in.Received {
		if !in.SmallBusiness && v.Status != "misimport" && !linked[v.ID] && v.Inspection.IssueDate == "" && v.Kind == "pdf" {
			issues.add("receipt_without_amounts", "PDF-Belege ohne Rechnungsdaten zählen erst, wenn du ihre Beträge erfasst.", taxforms.Entry{Kind: "received", ID: v.ID, Label: firstNonEmpty(v.Inspection.SellerName, v.Filename)})
		}
	}
	return sales, purchases, issues.list()
}

// countedPurchases is what a received document adds to the Voranmeldung. A small
// business reports only the tax it owes itself.
func countedPurchases(v model.ReceivedInvoice, t receivedTax, parsed bool, counted receivedInputTax, choice *model.InputTaxChoice, smallBusiness bool, e taxforms.Entry) []taxforms.Purchase {
	if counted.Fixed || (choice != nil && parsed && !taxGroupsMatch(t, counted.Tax)) {
		tax := counted.Tax
		if t.Credit {
			tax = tax.Neg()
		}
		return []taxforms.Purchase{{Entry: e, Category: "S", Tax: tax}}
	}
	out := []taxforms.Purchase{}
	for _, p := range receivedPurchases(v, t, parsed, e) {
		if smallBusiness {
			if p.Category != "AE" && p.Category != "K" {
				continue
			}
			p.NoInputTax = true
		}
		if counted.Remainder {
			p.Net, p.Tax = p.Net.Mul(counted.Share).Round(2), p.Tax.Mul(counted.Share).Round(2)
		}
		out = append(out, p)
	}
	return out
}

// receivedPurchases splits a checked e-invoice by tax group; without readable
// invoice data its totals count as ordinary input tax.
func receivedPurchases(v model.ReceivedInvoice, t receivedTax, parsed bool, e taxforms.Entry) []taxforms.Purchase {
	if !parsed {
		net, _ := decimal.NewFromString(v.Inspection.Totals.Net)
		tax, _ := decimal.NewFromString(v.Inspection.Totals.Tax)
		return []taxforms.Purchase{{Entry: e, Category: "S", Net: net, Tax: tax}}
	}
	out := []taxforms.Purchase{}
	for _, r := range t.Rates {
		out = append(out, taxforms.Purchase{Entry: e, Category: r.Category, Percent: r.Percent, Net: r.Basis, Tax: r.Amount, SellerCountry: t.Country})
	}
	return out
}

// taxGroupsMatch says whether a document's tax groups add up to the tax it
// states. On a broken document they can differ, and the user decides on the
// stated amount.
func taxGroupsMatch(t receivedTax, tax decimal.Decimal) bool {
	sum := decimal.Zero
	for _, r := range t.Rates {
		sum = sum.Add(r.Amount)
	}
	return sum.Abs().Equal(tax.Abs())
}

// EURView is the prepared Anlage EÜR of a calendar year.
type EURView struct {
	EUR      taxforms.EUR `json:"eur"`
	Deadline string       `json:"deadline"`
	// Questions name what the records cannot show; the user adds what applies.
	Questions []EURQuestion `json:"questions"`
	Small     bool          `json:"small"`
	// Filing notes that the annual returns were transmitted; Changed marks a
	// profit that differs from the transmitted one.
	Filing  *store.VATFiling `json:"filing,omitempty"`
	Changed bool             `json:"changed,omitempty"`
}

// EURQuestion is something the EÜR cannot know from the records. Answer says
// what to add when it applies.
type EURQuestion struct {
	ID       string `json:"id"`
	Question string `json:"question"`
	Answer   string `json:"answer"`
}

// eurQuestions leaves out the Gewerbesteuer for a freelance business, which
// does not pay it; while the activity is not chosen, it asks.
func eurQuestions(year int, smallBusiness bool, activity string) []EURQuestion {
	commute := "0,38 € je Entfernungskilometer"
	if year < 2026 {
		commute = "0,30 € je Entfernungskilometer, ab dem 21. Kilometer 0,38 €"
	}
	income := "Zeile 15 (netto) und Zeile 17 (Umsatzsteuer)"
	if smallBusiness {
		income = "Zeile 12"
	}
	questions := []EURQuestion{
		{ID: "cash", Question: "Hattest du Einnahmen ohne Rechnung in der App, etwa Barverkäufe oder Auszahlungen von Plattformen?", Answer: "Ergänze sie in " + income + "."},
		{ID: "home", Question: "Hast du an manchen Tagen zu Hause gearbeitet?", Answer: "Tagespauschale: 6 € je Tag, höchstens 1.260 € im Jahr (Zeile 66). Ist ein Arbeitszimmer der Mittelpunkt deiner Arbeit, gilt stattdessen Zeile 65."},
		{ID: "travel", Question: "Warst du beruflich mehr als 8 Stunden unterwegs?", Answer: "Verpflegungspauschale: 14 € ab 8 Stunden Abwesenheit, 28 € für volle Tage (Zeile 64). Fahrten mit dem Privatauto: 0,30 € je gefahrenem Kilometer (Zeile 71)."},
		{ID: "commute", Question: "Fährst du regelmäßig zu einem eigenen Büro, Laden oder einer Werkstatt?", Answer: "Fahrten zwischen Wohnung und Betriebsstätte: " + commute + " (Zeilen 72 und 73)."},
		{ID: "car", Question: "Nutzt du ein Fahrzeug aus dem Betriebsvermögen auch privat?", Answer: "Trage die private Nutzung als Einnahme in Zeile 20 ein: 1 % des Bruttolistenpreises je Monat (für Elektroautos weniger) oder nach Fahrtenbuch."},
		{ID: "withdrawals", Question: "Hast du Waren oder Leistungen deines Betriebs privat genutzt?", Answer: "Trage ihren Wert in Zeile 21 ein. Geld, das du vom Konto privat nimmst, gehört nicht in die EÜR."},
		{ID: "tradetax", Question: "Hast du Gewerbesteuer gezahlt, die nicht als Bankumsatz „Gewerbesteuer“ in der App steht?", Answer: "Trage sie in Zeile 67 in der Spalte „nicht abziehbar“ ein. Sie mindert den Gewinn nicht (§ 4 Abs. 5b EStG). Was du als Bankumsatz „Gewerbesteuer“ einordnest, steht dort schon."},
		{ID: "investments", Question: "Hast du einen Investitionsabzugsbetrag oder Sonderabschreibungen (§ 7g EStG) genutzt?", Answer: "Beides erfasst die App nicht. Trage es in der Anlage EÜR und der Anlage AVEÜR selbst ein."},
	}
	if activity == "freelance" {
		return slices.DeleteFunc(questions, func(q EURQuestion) bool { return q.ID == "tradetax" })
	}
	return questions
}

func (s *InvoiceService) EUR(year int) (EURView, error) {
	if year < 2000 || year > 2100 {
		return EURView{}, errors.New("Wähle ein gültiges Jahr.")
	}
	from, to := fmt.Sprintf("%d-01-01", year), fmt.Sprintf("%d-12-31", year)
	// The 10-day rule reaches into the next January.
	in, err := s.loadFinanceInput(fmt.Sprintf("%d-12-22", year-1), fmt.Sprintf("%d-01-10", year+1))
	if err != nil {
		return EURView{}, err
	}
	in.From, in.To = from, to
	income, vat, expenses, assets, issues, err := s.eurRecords(in, year)
	if err != nil {
		return EURView{}, err
	}
	r := taxforms.BuildEUR(year, in.SmallBusiness, income, vat, tradeTaxRecords(in, year), expenses, assets)
	r.Issues = append(issues, r.Issues...)
	view := EURView{EUR: r, Deadline: annualDeadline(year, taxSetup(in.Settings, in.SmallBusiness)), Small: in.SmallBusiness, Questions: eurQuestions(year, in.SmallBusiness, in.Settings.Activity)}
	if f, ok := in.AnnualFilings[strconv.Itoa(year)]; ok {
		view.Filing = &f
		view.Changed = payableChanged(f, r.Profit)
		if view.Changed {
			view.EUR.Issues = append([]taxforms.Issue{{Code: "changed_after_filing", Entries: []taxforms.Entry{}, Message: fmt.Sprintf(
				"Seit der Übermittlung am %s hat sich der Gewinn geändert: übermittelt %s €, jetzt %s €. Prüfe die geänderten Belege. War die übermittelte Erklärung falsch, meldest du das dem Finanzamt.",
				germanDate(f.SubmittedOn), taxforms.FormatGerman(f.Payable, true), taxforms.FormatGerman(r.Profit, true))}}, view.EUR.Issues...)
		}
	}
	return view, nil
}

// MarkAnnualFiled notes that the returns of a year (EÜR, Einkommensteuer and,
// where due, Umsatzsteuer) were transmitted, with the profit at that time.
func (s *InvoiceService) MarkAnnualFiled(year int, submittedOn, profit string) error {
	return s.store.SaveFiling(store.FilingAnnual, store.VATFiling{PeriodID: strconv.Itoa(year), SubmittedOn: submittedOn, Payable: profit})
}

func (s *InvoiceService) UnmarkAnnualFiled(year int) error {
	return s.store.DeleteFiling(store.FilingAnnual, strconv.Itoa(year))
}

// SmallBusinessStatus compares the turnover of a small business with the
// limits of § 19 Abs. 1 UStG: at most 25,000 € in the previous year and
// 100,000 € in the current one, 25,000 € in the year of founding. Turnover
// counts when it is received; non-taxable sales such as services for
// businesses abroad do not count.
type SmallBusinessStatus struct {
	Year     int    `json:"year"`
	Turnover string `json:"turnover"`
	// PreviousTurnover is empty when the app has no records of that year.
	PreviousTurnover string `json:"previousTurnover"`
	// State is "ok", "near" (over 20,000 €), "next_year" (over 25,000 €: the
	// regular rules apply from 1 January), "exceeded" (over 100,000 €) or
	// "previous_exceeded" (the previous year was over 25,000 €).
	State string `json:"state"`
	// Crossed is the payment with which the year exceeded 100,000 €; from that
	// sale on, Umsatzsteuer is due.
	Crossed *taxforms.Entry `json:"crossed,omitempty"`
}

var (
	smallBusinessPrevious = decimal.NewFromInt(25000)
	smallBusinessCurrent  = decimal.NewFromInt(100000)
	smallBusinessNear     = decimal.NewFromInt(20000)
)

func (s *InvoiceService) SmallBusinessStatus(year int) (SmallBusinessStatus, error) {
	in, err := s.loadFinanceInput(fmt.Sprintf("%d-01-01", year-1), fmt.Sprintf("%d-12-31", year))
	if err != nil {
		return SmallBusinessStatus{}, err
	}
	return smallBusinessStatus(in, year), nil
}

func smallBusinessStatus(in financeInput, year int) SmallBusinessStatus {
	st := SmallBusinessStatus{Year: year, State: "ok"}
	current, crossed, _ := turnover(in, year)
	st.Turnover = money(current)
	previous, _, known := turnover(in, year-1)
	if known {
		st.PreviousTurnover = money(previous)
	}
	switch {
	case previous.GreaterThan(smallBusinessPrevious):
		st.State = "previous_exceeded"
	case current.GreaterThan(smallBusinessCurrent):
		st.State, st.Crossed = "exceeded", crossed
	case current.GreaterThan(smallBusinessPrevious):
		st.State = "next_year"
	case current.GreaterThan(smallBusinessNear):
		st.State = "near"
	}
	return st
}

// turnover sums the payments received in a year for taxable sales. known
// tells whether the app holds any invoice or payment of that year; crossed is
// the payment that took the sum over 100,000 €.
func turnover(in financeInput, year int) (decimal.Decimal, *taxforms.Entry, bool) {
	prefix := strconv.Itoa(year) + "-"
	issued := map[string]issuedTax{}
	known := false
	for _, v := range in.Issued {
		issued[v.ID] = v
		known = known || strings.HasPrefix(v.IssueDate, prefix)
	}
	payments := append([]store.Payment(nil), in.Overview.Payments...)
	sort.SliceStable(payments, func(i, j int) bool { return payments[i].Date < payments[j].Date })
	sum := decimal.Zero
	var crossed *taxforms.Entry
	for _, p := range payments {
		if p.RevokedAt != "" || p.Direction != "outgoing" || p.Currency != "EUR" || !strings.HasPrefix(p.Date, prefix) {
			continue
		}
		known = true
		amount, err := decimal.NewFromString(p.Amount)
		if err != nil {
			continue
		}
		v := issued[p.InvoiceID]
		if v.Credit {
			amount = amount.Neg()
		}
		// Without readable tax data the whole payment counts, to warn early.
		if !v.Gross.IsZero() {
			countable := decimal.Zero
			for _, t := range v.Rates {
				if t.Category != "O" && (t.Category != "AE" || taxforms.Domestic(v.BuyerCountry)) {
					countable = countable.Add(t.Basis).Add(t.Amount)
				}
			}
			amount = amount.Mul(countable).Div(v.Gross).Round(2)
		}
		before := sum
		sum = sum.Add(amount)
		if crossed == nil && !before.GreaterThan(smallBusinessCurrent) && sum.GreaterThan(smallBusinessCurrent) {
			crossed = &taxforms.Entry{Kind: "payment", ID: p.ID, Date: p.Date, Label: "Zahlung zu " + invoiceLabel(v.Number, v.Partner), Amount: money(amount)}
		}
	}
	return sum, crossed, known
}

// ZMReturnView is the Zusammenfassende Meldung of one Meldezeitraum.
type ZMReturnView struct {
	Period taxforms.Period   `json:"period"`
	Report taxforms.ZMReport `json:"report"`
	Filing *store.VATFiling  `json:"filing,omitempty"`
}

// ZMPeriods lists the Meldezeiträume of a year that contain deliveries or
// services for businesses in other member states, or were transmitted.
// Small businesses file no Zusammenfassende Meldung (§ 18a Abs. 4 UStG).
func (s *InvoiceService) ZMPeriods(year int) ([]VATPeriodStatus, error) {
	return s.zmPeriods(year, time.Now().Format(time.DateOnly))
}

func (s *InvoiceService) zmPeriods(year int, today string) ([]VATPeriodStatus, error) {
	in, err := s.loadFinanceInput(fmt.Sprintf("%d-01-01", year), fmt.Sprintf("%d-12-31", year))
	if err != nil {
		return []VATPeriodStatus{}, err
	}
	filings, err := s.store.Filings(store.FilingZM)
	if err != nil {
		return nil, err
	}
	return zmPeriodsFrom(in, year, today, filings), nil
}

func zmPeriodsFrom(in financeInput, year int, today string, filings map[string]store.VATFiling) []VATPeriodStatus {
	out := []VATPeriodStatus{}
	if in.SmallBusiness {
		return out
	}
	sales := zmSales(in)
	for _, p := range taxforms.ZMPeriods(year, zmMonthly(sales, year)) {
		f, filed := filings[p.ID]
		report := taxforms.BuildZM(zmSalesIn(sales, p))
		if !filed && len(report.Lines) == 0 && len(report.Issues) == 0 {
			continue
		}
		st := VATPeriodStatus{Period: p, Started: p.From <= today}
		if filed {
			st.Filing = &f
			st.Changed = payableChanged(f, report.Total)
		} else {
			st.Overdue = p.Due < today
		}
		out = append(out, st)
	}
	return out
}

// TaxOverview is what the tax page lists for a year: its Voranmeldungen,
// Zusammenfassende Meldungen and annual returns, together with those of the
// previous year, which are often still due. One load serves all of them.
type TaxOverview struct {
	Setup  TaxSetup          `json:"setup"`
	Year   int               `json:"year"`
	VAT    []VATPeriodStatus `json:"vat"`
	ZM     []VATPeriodStatus `json:"zm"`
	Annual []AnnualStatus    `json:"annual"`
	// SmallBusiness compares the turnover with the limits of § 19 UStG.
	SmallBusiness *SmallBusinessStatus `json:"smallBusiness,omitempty"`
	// Since is the date of the first record in the app, or today without one.
	// Earlier periods were handled elsewhere, so the page does not list them.
	Since string `json:"since"`
	// Taxes sums up the year by kind of tax.
	Taxes TaxSummary `json:"taxes"`
}

// AnnualStatus is the state of the returns of a year (EÜR, Einkommensteuer
// and, where due, the Umsatzsteuer-Jahreserklärung).
type AnnualStatus struct {
	Year    int              `json:"year"`
	Due     string           `json:"due"`
	Profit  string           `json:"profit"`
	Filing  *store.VATFiling `json:"filing,omitempty"`
	Changed bool             `json:"changed,omitempty"`
}

func (s *InvoiceService) TaxOverview(year int) (TaxOverview, error) {
	return s.taxOverview(year, time.Now().Format(time.DateOnly))
}

func (s *InvoiceService) taxOverview(year int, today string) (TaxOverview, error) {
	if year < 2001 || year > 2100 {
		return TaxOverview{}, errors.New("Wähle ein gültiges Jahr.")
	}
	setup, err := s.TaxSetup()
	if err != nil {
		return TaxOverview{}, err
	}
	in, err := s.loadFinanceInput(fmt.Sprintf("%d-01-01", year-1), fmt.Sprintf("%d-12-31", year))
	if err != nil {
		return TaxOverview{}, err
	}
	zmFilings, err := s.store.Filings(store.FilingZM)
	if err != nil {
		return TaxOverview{}, err
	}
	o := TaxOverview{Setup: setup, Year: year, VAT: []VATPeriodStatus{}, ZM: []VATPeriodStatus{}, Annual: []AnnualStatus{}}
	for _, y := range []int{year - 1, year} {
		periods, err := s.vatPeriods(y, today)
		if err != nil {
			return o, err
		}
		for i, p := range periods {
			if p.Filing != nil {
				periods[i].Changed = payableChanged(*p.Filing, buildVATReturn(in, p.Period, setup).Payable)
			}
		}
		o.VAT = append(o.VAT, periods...)
		o.ZM = append(o.ZM, zmPeriodsFrom(in, y, today, zmFilings)...)
		yearIn := in
		yearIn.From, yearIn.To = fmt.Sprintf("%d-01-01", y), fmt.Sprintf("%d-12-31", y)
		income, vat, expenses, assets, _, err := s.eurRecords(yearIn, y)
		if err != nil {
			return o, err
		}
		a := AnnualStatus{Year: y, Due: annualDeadline(y, setup), Profit: taxforms.BuildEUR(y, in.SmallBusiness, income, vat, tradeTaxRecords(yearIn, y), expenses, assets).Profit}
		if f, ok := in.AnnualFilings[strconv.Itoa(y)]; ok {
			a.Filing, a.Changed = &f, payableChanged(f, a.Profit)
		}
		o.Annual = append(o.Annual, a)
	}
	if setup.SmallBusiness {
		st := smallBusinessStatus(in, year)
		o.SmallBusiness = &st
	}
	o.Since = firstRecord(in, today)
	in.Today = today
	o.Taxes = taxSummary(in, year, o.Annual[len(o.Annual)-1].Profit)
	return o, nil
}

// firstRecord is the earliest date of an invoice, received document, receipt,
// payment or bank booking, or today when there is none.
func firstRecord(in financeInput, today string) string {
	first := today
	consider := func(date string) {
		if date != "" && date < first {
			first = date
		}
	}
	for _, v := range in.Issued {
		consider(v.IssueDate)
	}
	for _, v := range in.Received {
		if v.Status != "misimport" {
			consider(v.Inspection.IssueDate)
		}
	}
	for _, x := range in.Expenses {
		consider(x.Date)
	}
	for _, p := range in.Overview.Payments {
		consider(p.Date)
	}
	for _, b := range in.Bookings {
		consider(b.Data.BookingDate)
	}
	return first
}

func (s *InvoiceService) ZMReturn(periodID string) (ZMReturnView, error) {
	year := yearOf(periodID)
	if year == 0 {
		return ZMReturnView{}, errors.New("Wähle einen gültigen Meldezeitraum.")
	}
	in, err := s.loadFinanceInput(fmt.Sprintf("%d-01-01", year), fmt.Sprintf("%d-12-31", year))
	if err != nil {
		return ZMReturnView{}, err
	}
	sales := zmSales(in)
	for _, p := range taxforms.ZMPeriods(year, zmMonthly(sales, year)) {
		if p.ID != periodID {
			continue
		}
		view := ZMReturnView{Period: p, Report: taxforms.BuildZM(zmSalesIn(sales, p))}
		if filings, err := s.store.Filings(store.FilingZM); err == nil {
			if f, ok := filings[p.ID]; ok {
				view.Filing = &f
				if payableChanged(f, view.Report.Total) {
					view.Report.Issues = append([]taxforms.Issue{{Code: "changed_after_filing", Entries: []taxforms.Entry{}, Message: fmt.Sprintf(
						"Seit der Übermittlung am %s hat sich die Summe geändert: übermittelt %s €, jetzt %s €. Prüfe die Änderung. War die Meldung falsch, ist sie innerhalb eines Monats zu berichtigen (§ 18a Abs. 10 UStG).",
						germanDate(f.SubmittedOn), taxforms.FormatGerman(f.Payable, false), taxforms.FormatGerman(view.Report.Total, false))}}, view.Report.Issues...)
				}
			}
		}
		return view, nil
	}
	return ZMReturnView{}, errors.New("Für diesen Zeitraum gibt es keine Zusammenfassende Meldung.")
}

func (s *InvoiceService) MarkZMFiled(periodID, submittedOn, total string) error {
	return s.store.SaveFiling(store.FilingZM, store.VATFiling{PeriodID: periodID, SubmittedOn: submittedOn, Payable: total})
}

func (s *InvoiceService) UnmarkZMFiled(periodID string) error {
	return s.store.DeleteFiling(store.FilingZM, periodID)
}

// zmSales lists the tax groups of all issued documents that belong in a
// Zusammenfassende Meldung, dated like the Voranmeldung.
func zmSales(in financeInput) []taxforms.ZMSale {
	var out []taxforms.ZMSale
	for _, v := range in.Issued {
		if v.Problem != "" || v.Currency != "EUR" {
			continue
		}
		for _, t := range v.Rates {
			if !v.documentDated(t) {
				continue
			}
			out = append(out, taxforms.ZMSale{Entry: taxforms.Entry{Kind: "invoice", ID: v.ID, Date: v.TaxDate, Label: invoiceLabel(v.Number, v.Partner)},
				VATID: v.BuyerVATID, Country: v.BuyerCountry, Partner: v.Partner, Services: t.Category == "AE", Net: t.Basis})
		}
	}
	return out
}

func zmSalesIn(sales []taxforms.ZMSale, p taxforms.Period) []taxforms.ZMSale {
	var out []taxforms.ZMSale
	for _, s := range sales {
		if inPeriod(s.Entry.Date, p.From, p.To) {
			out = append(out, s)
		}
	}
	return out
}

// zmMonthly marks the quarters of a year whose deliveries of goods, or those
// of one of the four quarters before, exceed 50,000 €.
func zmMonthly(sales []taxforms.ZMSale, year int) map[int]bool {
	goods := map[int]decimal.Decimal{} // by year*4 + quarter
	for _, s := range sales {
		var y int
		if s.Services || len(s.Entry.Date) < 4 {
			continue
		}
		if _, err := fmt.Sscanf(s.Entry.Date, "%4d", &y); err != nil {
			continue
		}
		goods[y*4+taxforms.QuarterOf(s.Entry.Date)] = goods[y*4+taxforms.QuarterOf(s.Entry.Date)].Add(s.Net)
	}
	out := map[int]bool{}
	for q := 1; q <= 4; q++ {
		for back := 0; back <= 4; back++ {
			if goods[year*4+q-back].GreaterThan(taxforms.ZMGoodsLimit) {
				out[q] = true
			}
		}
	}
	return out
}

// tenDayRuleApplies reports whether a Voranmeldung of the year falls due on
// 10 January. Only then do Umsatzsteuer payments and refunds up to that day
// count for the earlier year (§ 11 Abs. 2 Satz 2 EStG): the statutory due date
// counts even on a weekend (BFH X R 44/16), but with Dauerfristverlängerung a
// quarterly return for the fourth quarter is due in February (BFH X R 2/21).
// Monthly filers with extension pay November on 10 January.
func tenDayRuleApplies(frequency string, extension bool) bool {
	return frequency == "monthly" || (frequency == "quarterly" && !extension)
}

// inYearWithTenDays places an Umsatzsteuer payment or refund in a year; with
// the ten-day rule the first ten days of January count for the earlier year.
func inYearWithTenDays(date string, year int, tenDays bool) bool {
	if tenDays {
		switch {
		case date >= fmt.Sprintf("%d-01-01", year) && date <= fmt.Sprintf("%d-01-10", year):
			return false
		case date >= fmt.Sprintf("%d-01-01", year+1) && date <= fmt.Sprintf("%d-01-10", year+1):
			return true
		}
	}
	return strings.HasPrefix(date, strconv.Itoa(year)+"-")
}

// effectiveVATPeriod is how often the Voranmeldung is due. A small business
// files only for periods in which it owes tax as a recipient; the calendar
// quarter applies unless the tax office set months (§ 18 Abs. 2 and 4a UStG).
func effectiveVATPeriod(settings store.FinanceSettings, smallBusiness bool) string {
	if smallBusiness && settings.VATPeriod != "monthly" {
		return "quarterly"
	}
	return settings.VATPeriod
}

// tradeTaxRecords lists the Gewerbesteuer paid to or refunded by the
// municipality in year, from bank bookings the user classified as such.
func tradeTaxRecords(in financeInput, year int) []taxforms.TradeTaxMovement {
	var out []taxforms.TradeTaxMovement
	prefix := strconv.Itoa(year) + "-"
	for _, b := range in.Bookings {
		if in.Treatments[b.ID].Kind != "tradetax" || b.Data.Currency != "EUR" || !strings.HasPrefix(b.Data.BookingDate, prefix) {
			continue
		}
		e := taxforms.Entry{Kind: "booking", ID: b.ID, Date: b.Data.BookingDate, Label: firstNonEmpty(b.Data.CounterpartyName, b.Data.Description, "Gemeinde")}
		out = append(out, taxforms.TradeTaxMovement{Entry: e, Amount: decimal.NewFromInt(b.Data.Amount).Shift(-2)})
	}
	return out
}

func (s *InvoiceService) eurRecords(in financeInput, year int) ([]taxforms.Income, []taxforms.VATMovement, []taxforms.Expense, []taxforms.Asset, []taxforms.Issue, error) {
	var issues issueSet
	var income []taxforms.Income
	var expenses []taxforms.Expense
	var assets []taxforms.Asset
	var vat []taxforms.VATMovement
	issued := map[string]issuedTax{}
	for _, v := range in.Issued {
		issued[v.ID] = v
	}
	cats, err := s.store.ReceivedCategories()
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}
	received := map[string]int{}
	for i, v := range in.Received {
		received[v.ID] = i
	}
	for _, p := range in.Overview.Payments {
		if p.RevokedAt != "" || !inPeriod(p.Date, in.From, in.To) {
			continue
		}
		amount, err := decimal.NewFromString(p.Amount)
		if err != nil {
			continue
		}
		if p.Direction == "outgoing" {
			v := issued[p.InvoiceID]
			e := taxforms.Entry{Kind: "payment", ID: p.ID, Date: p.Date, Label: "Zahlung zu " + invoiceLabel(v.Number, v.Partner)}
			if v.Problem != "" || v.Gross.IsZero() || p.Currency != "EUR" {
				issues.add("income_without_tax", "Zahlungseingänge zu Rechnungen ohne auswertbare Steuerdaten oder in Fremdwährung sind nicht enthalten.", e)
				continue
			}
			if v.Credit {
				amount = amount.Neg()
			}
			share := amount.Div(v.Gross)
			for _, t := range v.Rates {
				income = append(income, taxforms.Income{Entry: e, Category: t.Category, Net: t.Basis.Mul(share).Round(2), Tax: t.Amount.Mul(share).Round(2), Margin: v.margin(t)})
			}
			continue
		}
		i, ok := received[p.InvoiceID]
		if !ok {
			continue
		}
		v := in.Received[i]
		credit := in.ReceivedTax[v.ID].Credit
		label := "Zahlung an "
		if credit {
			label = "Erstattung zu Gutschrift "
		}
		e := taxforms.Entry{Kind: "payment", ID: p.ID, Date: p.Date, Label: label + invoiceLabel(v.Inspection.InvoiceNumber, v.Inspection.SellerName)}
		gross, _ := decimal.NewFromString(v.Inspection.Totals.Gross)
		net, _ := decimal.NewFromString(v.Inspection.Totals.Net)
		tax, _ := decimal.NewFromString(v.Inspection.Totals.Tax)
		if !gross.IsPositive() || p.Currency != "EUR" {
			issues.add("expense_without_amounts", "Zahlungen auf Eingangsrechnungen ohne Gesamtbetrag oder in Fremdwährung sind nicht enthalten.", e)
			continue
		}
		c, ok := cats[v.ID]
		if !ok {
			issues.add("expense_without_category", "Für diese Eingangsrechnungen fehlt die Kategorie. Die App rechnet sie vorerst unter „Sonstige Betriebsausgaben“.", e)
			c.Category = "sonstige"
		}
		if c.Category == taxforms.AssetCategory {
			continue // depreciated from the invoice date below
		}
		share := amount.Div(gross)
		if credit {
			share = share.Neg() // a refund reduces the expense and the paid input tax
		}
		expenses = append(expenses, taxforms.Expense{Entry: e, Category: c.Category, Net: net.Mul(share).Round(2), Tax: tax.Mul(share).Round(2)})
	}
	for id, c := range cats {
		i, ok := received[id]
		if !ok || c.Category != taxforms.AssetCategory {
			continue
		}
		v := in.Received[i]
		e := taxforms.Entry{Kind: "received", ID: v.ID, Date: v.Inspection.IssueDate, Label: invoiceLabel(v.Inspection.InvoiceNumber, v.Inspection.SellerName)}
		if in.ReceivedTax[id].Credit {
			issues.add("asset_credit_note", "Gutschriften zu Anschaffungen mindern deren Anschaffungskosten. Passe die Abschreibung dafür selbst an.", e)
			continue
		}
		net, _ := decimal.NewFromString(v.Inspection.Totals.Net)
		tax, _ := decimal.NewFromString(v.Inspection.Totals.Tax)
		assets = append(assets, taxforms.Asset{Entry: e, Acquired: v.Inspection.IssueDate, Years: c.Years, Net: net, Tax: tax})
	}
	for _, x := range in.Expenses {
		e := taxforms.Entry{Kind: "expense", ID: x.ID, Date: firstNonEmpty(x.PaidDate, x.Date), Label: x.Supplier, Note: x.Note}
		net, _ := decimal.NewFromString(x.Net)
		tax, _ := decimal.NewFromString(x.Tax)
		if x.Category == taxforms.AssetCategory {
			assets = append(assets, taxforms.Asset{Entry: e, Acquired: x.Date, Years: x.Years, Net: net, Tax: tax})
			continue
		}
		if x.PaidDate == "" {
			if strings.HasPrefix(x.Date, strconv.Itoa(year)) {
				issues.add("expense_unpaid", "Diese Belege sind noch nicht bezahlt und zählen erst im Jahr der Zahlung.", e)
			}
			continue
		}
		if !inPeriod(x.PaidDate, in.From, in.To) {
			continue
		}
		expenses = append(expenses, taxforms.Expense{Entry: e, Category: x.Category, Net: net, Tax: tax})
	}
	tenDays := tenDayRuleApplies(effectiveVATPeriod(in.Settings, in.SmallBusiness), in.Settings.VATExtension)
	yearPrefix := strconv.Itoa(year) + "-"
	for _, b := range in.Bookings {
		date := b.Data.BookingDate
		if in.Treatments[b.ID].Kind != "vat" || b.Data.Currency != "EUR" || !inYearWithTenDays(date, year, tenDays) {
			continue
		}
		e := taxforms.Entry{Kind: "booking", ID: b.ID, Date: date, Label: firstNonEmpty(b.Data.CounterpartyName, b.Data.Description, "Finanzamt")}
		if !strings.HasPrefix(date, yearPrefix) {
			e.Note = "10-Tage-Regel: zählt für " + strconv.Itoa(year)
		}
		vat = append(vat, taxforms.VATMovement{Entry: e, Amount: decimal.NewFromInt(b.Data.Amount).Shift(-2)})
	}

	// A payment nobody recorded leaves no other trace in the EÜR: name open
	// documents of the year and bank movements without any assignment.
	for _, b := range in.Overview.Invoices {
		remaining := b.Collectable()
		if !remaining.IsPositive() || b.Currency != "EUR" {
			continue
		}
		e := taxforms.Entry{Kind: "invoice", ID: b.ID, Note: "offen " + taxforms.FormatGerman(remaining.StringFixed(2), true) + " €"}
		code, message := "issued_open", "Diese Kundenrechnungen und Gutschriften sind laut App noch offen. Ist die Zahlung schon erfolgt, erfasse sie – sonst fehlt sie in der EÜR."
		if b.Direction == "incoming" {
			i, ok := received[b.ID]
			if !ok {
				continue
			}
			v := in.Received[i]
			e.Kind, e.Date, e.Label = "received", v.Inspection.IssueDate, invoiceLabel(v.Inspection.InvoiceNumber, v.Inspection.SellerName)
			code, message = "received_open", "Diese Eingangsrechnungen sind laut App noch nicht bezahlt. Hast du sie schon bezahlt, erfasse die Zahlung – sonst fehlt die Ausgabe in der EÜR."
		} else {
			v := issued[b.ID]
			e.Date, e.Label = v.IssueDate, invoiceLabel(v.Number, v.Partner)
		}
		if strings.HasPrefix(e.Date, yearPrefix) {
			issues.add(code, message, e)
		}
	}
	captured := map[string]bool{}
	for _, x := range in.Expenses {
		captured[x.BookingID] = x.BookingID != ""
	}
	for _, b := range in.Bookings {
		if !strings.HasPrefix(b.Data.BookingDate, yearPrefix) || in.Treatments[b.ID].Kind != "" || captured[b.ID] {
			continue
		}
		if left, err := bookingUnallocated(b, in.Overview.Payments); err == nil && !left.IsZero() {
			issues.add("unassigned_bookings", "Bankumsätze ohne Rechnung, Beleg oder Einordnung. Sind es Einnahmen oder Ausgaben, fehlen sie in der EÜR.",
				taxforms.Entry{Kind: "booking", ID: b.ID, Date: b.Data.BookingDate, Label: firstNonEmpty(b.Data.CounterpartyName, b.Data.Description, "Bankumsatz"), Note: taxforms.FormatGerman(left.StringFixed(2), true) + " €"})
		}
	}
	return income, vat, expenses, assets, issues.list(), nil
}

// TaxDeadline is an open filing shown on the overview.
type TaxDeadline struct {
	Kind     string `json:"kind"` // vat, zm or annual
	PeriodID string `json:"periodId"`
	// Task opens the filing on the tax page, e.g. "zm:2026-Q3".
	Task    string `json:"task"`
	Label   string `json:"label"`
	Due     string `json:"due"`
	Overdue bool   `json:"overdue"`
}

// TaxDeadlines lists unfiled Voranmeldungen, Zusammenfassende Meldungen and
// annual returns due within the last six or the next three weeks, so older
// periods from before the app was used do not crowd the overview.
func (s *InvoiceService) TaxDeadlines() ([]TaxDeadline, error) {
	return s.taxDeadlines(time.Now())
}

func (s *InvoiceService) taxDeadlines(now time.Time) ([]TaxDeadline, error) {
	today := now.Format(time.DateOnly)
	soon := now.AddDate(0, 0, 21).Format(time.DateOnly)
	recent := now.AddDate(0, 0, -42).Format(time.DateOnly)
	out := []TaxDeadline{}
	add := func(kind, prefix string, periods []VATPeriodStatus) {
		for _, p := range periods {
			if p.Filing != nil || p.Period.To >= today || p.Period.Due > soon || p.Period.Due < recent {
				continue
			}
			out = append(out, TaxDeadline{Kind: kind, PeriodID: p.Period.ID, Task: kind + ":" + p.Period.ID, Label: prefix + p.Period.Label, Due: p.Period.Due, Overdue: p.Overdue})
		}
	}
	for _, year := range []int{now.Year() - 1, now.Year()} {
		periods, err := s.vatPeriods(year, today)
		if err != nil {
			return out, err
		}
		add("vat", "Umsatzsteuer-Voranmeldung ", periods)
		if periods, err = s.zmPeriods(year, today); err != nil {
			return out, err
		}
		add("zm", "Zusammenfassende Meldung ", periods)
	}
	setup, err := s.TaxSetup()
	if err != nil {
		return out, err
	}
	filings, err := s.store.Filings(store.FilingAnnual)
	if err != nil {
		return out, err
	}
	year := now.Year() - 1
	due := annualDeadline(year, setup)
	if _, done := filings[strconv.Itoa(year)]; !done && due <= soon && due >= recent {
		out = append(out, TaxDeadline{Kind: "annual", PeriodID: strconv.Itoa(year), Task: "annual:" + strconv.Itoa(year), Label: fmt.Sprintf("Steuererklärungen %d", year), Due: due, Overdue: due < today})
	}
	return out, nil
}

// annualDeadline is when the returns of a year are due: 31 July of the next
// year, or later when a tax advisor prepares them.
func annualDeadline(year int, setup TaxSetup) string {
	if setup.Filing == "advisor" {
		return taxforms.AdvisorDeadline(year)
	}
	return taxforms.AnnualDeadline(year)
}

// yearOf returns the year of a date or period ID such as 2026-Q3, or 0.
func yearOf(s string) int {
	if len(s) < 4 {
		return 0
	}
	y, err := strconv.Atoi(s[:4])
	if err != nil {
		return 0
	}
	return y
}
