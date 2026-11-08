package app

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/shopspring/decimal"
	"github.com/wailsapp/wails/v3/pkg/application"
	"papierleicht.de/e-rechnung-studio/internal/datev"
	"papierleicht.de/e-rechnung-studio/internal/invoicexml"
	"papierleicht.de/e-rechnung-studio/internal/store"
	"papierleicht.de/e-rechnung-studio/internal/taxforms"
)

// The DATEV export hands the tax advisor a Buchungsstapel of the period's
// invoices, credit notes, checked received invoices and, optionally,
// payments. Like the finance report it never guesses: anything it cannot
// book is listed in the checklist instead of being dropped silently.

// DatevIssue is one checklist item. Severity "blocking" prevents the export,
// "excluded" lists records left out of the file, "review" lists records that
// are included but need the tax advisor's attention.
type DatevIssue struct {
	Code     string         `json:"code"`
	Severity string         `json:"severity"`
	Message  string         `json:"message"`
	Entries  []FinanceEntry `json:"entries"`
}

type DatevPreview struct {
	From            string              `json:"from"`
	To              string              `json:"to"`
	FiscalYearStart string              `json:"fiscalYearStart"`
	Ready           bool                `json:"ready"`
	Bookings        int                 `json:"bookings"`
	Invoices        int                 `json:"invoices"`
	CreditNotes     int                 `json:"creditNotes"`
	Received        int                 `json:"received"`
	Expenses        int                 `json:"expenses"`
	Payments        int                 `json:"payments"`
	Documents       int                 `json:"documents"`
	NewAccounts     int                 `json:"newAccounts"`
	Issues          []DatevIssue        `json:"issues"`
	NotCovered      []string            `json:"notCovered"`
	Settings        store.DatevSettings `json:"settings"`
}

type DatevExportResult struct {
	Preview DatevPreview `json:"preview"`
	CSVPath string       `json:"csvPath"`
	ZIPPath string       `json:"zipPath"`
}

const msgMissingAccount = "Für diese Fälle ist noch kein Konto eingetragen. Die Belege fehlen deshalb im Export."

const (
	severityBlocking = "blocking"
	severityExcluded = "excluded"
	severityReview   = "review"
)

type datevTax struct {
	Category, ReasonCode, Reason string
	Percent, Basis, Amount       decimal.Decimal
}

// datevDoc is an issued or received invoice or a receipt reduced to what the export needs.
type datevDoc struct {
	Kind                                  string // issued, received or expense
	ID, Number, Date, Partner, PartnerKey string
	VATID, Country, Currency, Treatment   string
	Profile, Filename, ReceivedID         string
	Credit                                bool
	Taxes                                 []datevTax
	Problem                               string
	// Advance and CorrectsID: see issuedTax.
	Advance    bool
	CorrectsID string
}

type datevInput struct {
	From, To      string
	Created       time.Time
	Settings      store.DatevSettings
	SmallBusiness bool
	// TaxBasis is "ist" or "soll", as in the finance settings.
	TaxBasis string
	Issued   []datevDoc
	Received []datevDoc
	Expenses []store.Expense
	// ReceivedCategories holds the expense category chosen on a received invoice.
	ReceivedCategories map[string]store.ReceivedCategory
	Payments           []store.Payment
	Persons            []store.DatevPersonAccount
}

type datevBooking struct {
	datev.Booking
	DocKind, DocID string
}

type datevBuild struct {
	Preview    DatevPreview
	Header     datev.Header
	Bookings   []datevBooking
	NewPersons []store.DatevPersonAccount
}

func (s *InvoiceService) DatevSettings() (store.DatevSettings, error) {
	return s.store.DatevSettings()
}

func (s *InvoiceService) SaveDatevSettings(v store.DatevSettings) (store.DatevSettings, error) {
	return s.store.SaveDatevSettings(v)
}

// DatevDefaultAccounts suggests standard accounts for a chart and account length.
func (s *InvoiceService) DatevDefaultAccounts(chart string, length int) (datev.Accounts, error) {
	return datev.DefaultAccounts(chart, length)
}

func (s *InvoiceService) DatevExportPreview(from, to string) (DatevPreview, error) {
	in, err := s.loadDatevInput(from, to)
	if err != nil {
		return DatevPreview{}, err
	}
	return buildDatevExport(in).Preview, nil
}

// ExportDatev writes the Buchungsstapel and, if enabled, a ZIP of the
// original documents into the DATEV folder below the export directory.
// Existing files are never overwritten.
func (s *InvoiceService) ExportDatev(from, to string) (DatevExportResult, error) {
	s.exportMu.Lock()
	defer s.exportMu.Unlock()
	in, err := s.loadDatevInput(from, to)
	if err != nil {
		return DatevExportResult{}, err
	}
	b := buildDatevExport(in)
	result := DatevExportResult{Preview: b.Preview}
	if !b.Preview.Ready {
		for _, issue := range b.Preview.Issues {
			if issue.Severity == severityBlocking {
				return result, errors.New(issue.Message)
			}
		}
		return result, errors.New("Der Export ist noch nicht möglich. Prüfe die Hinweise.")
	}
	csv, err := writeDatev(b)
	if err != nil {
		return result, err
	}
	settings, err := s.GetAppSettings()
	if err != nil {
		return result, err
	}
	dir := filepath.Join(settings.ExportDirectory, "DATEV")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return result, fmt.Errorf("Der DATEV-Ordner konnte nicht angelegt werden: %w", err)
	}
	// Person accounts are reserved before writing, so a retried export never
	// hands the same number to someone else.
	if err := s.store.AddDatevPersonAccounts(b.NewPersons); err != nil {
		return result, err
	}
	period := strings.ReplaceAll(from, "-", "") + "_" + strings.ReplaceAll(to, "-", "")
	if result.CSVPath, err = exportArtifact(dir, "EXTF_Buchungsstapel_"+period, ".csv", "", csv); err != nil {
		return result, err
	}
	if in.Settings.IncludeDocuments && b.Preview.Documents > 0 {
		archive, missing, err := s.datevDocuments(in, b)
		if err != nil {
			return result, err
		}
		if len(missing) > 0 {
			result.Preview.Issues = append(result.Preview.Issues, DatevIssue{Code: "document_missing", Severity: severityReview, Message: "Für diese Buchungen fehlt die Belegdatei im ZIP. Gib den Beleg deiner Kanzlei separat weiter.", Entries: missing})
		}
		if result.ZIPPath, err = exportArtifact(dir, "Belege_"+period, ".zip", "", archive); err != nil {
			return result, err
		}
	}
	return result, nil
}

// OpenDatevExportFolder shows the folder of a file written by ExportDatev.
func (s *InvoiceService) OpenDatevExportFolder(path string) error {
	settings, err := s.GetAppSettings()
	if err != nil {
		return err
	}
	dir := filepath.Clean(filepath.Join(settings.ExportDirectory, "DATEV"))
	if filepath.Dir(filepath.Clean(path)) != dir {
		return errors.New("Dieser Ordner gehört nicht zum DATEV-Export.")
	}
	app := application.Get()
	if app == nil {
		return errors.New("Den Ordner kannst du nur in der App öffnen.")
	}
	return app.Browser.OpenFile(dir)
}

func writeDatev(b datevBuild) ([]byte, error) {
	rows := make([]datev.Booking, len(b.Bookings))
	for i, v := range b.Bookings {
		rows[i] = v.Booking
	}
	return datev.Write(b.Header, rows)
}

func (s *InvoiceService) loadDatevInput(from, to string) (datevInput, error) {
	in := datevInput{From: from, To: to, Created: time.Now()}
	var err error
	if in.Settings, err = s.store.DatevSettings(); err != nil {
		return in, err
	}
	company, err := s.store.Company()
	if err != nil {
		return in, err
	}
	in.SmallBusiness = company.SmallBusiness
	finance, err := s.store.FinanceSettings()
	if err != nil {
		return in, err
	}
	in.TaxBasis = finance.TaxBasis
	docs, err := s.store.IssuedDocuments()
	if err != nil {
		return in, err
	}
	for _, d := range docs {
		in.Issued = append(in.Issued, issuedDatevDoc(d))
	}
	received, err := s.store.ListReceived()
	if err != nil {
		return in, err
	}
	if in.Expenses, err = s.store.ActiveExpenses(); err != nil {
		return in, err
	}
	if in.ReceivedCategories, err = s.store.ReceivedCategories(); err != nil {
		return in, err
	}
	// A PDF whose amounts were captured by hand is booked as that expense.
	captured := map[string]bool{}
	for _, x := range in.Expenses {
		if x.ReceivedID != "" {
			captured[x.ReceivedID] = true
		}
	}
	choices, err := s.store.InputTaxChoices()
	if err != nil {
		return in, err
	}
	for _, v := range received {
		if v.Status == "misimport" || captured[v.ID] {
			continue
		}
		doc := datevDoc{Kind: "received", ID: v.ID, Number: v.Inspection.InvoiceNumber, Date: v.Inspection.IssueDate, Partner: firstNonEmpty(v.Inspection.SellerName, v.Filename), Currency: firstNonEmpty(v.Inspection.Currency, "EUR"), Filename: v.Filename}
		doc.PartnerKey = "name:" + partnerKey(doc.Partner)
		switch {
		case v.Inspection.Validation.State == "unsupported":
			doc.Problem = "Keine strukturierten Rechnungsdaten"
		case inputTaxProblem(v, choiceOf(choices, v.ID)) != "":
			doc.Problem = inputTaxProblem(v, choiceOf(choices, v.ID))
		case doc.Date == "":
			doc.Problem = "Rechnungsdatum fehlt"
		}
		// Only invoices that can be booked in this period need their XML.
		if doc.Problem == "" && inPeriod(doc.Date, from, to) {
			_, original, err := s.store.GetReceived(v.ID)
			if err != nil {
				return in, err
			}
			readReceivedTaxes(&doc, original, v.Kind)
		}
		in.Received = append(in.Received, doc)
	}
	overview, err := s.store.PaymentOverview()
	if err != nil {
		return in, err
	}
	in.Payments = overview.Payments
	in.Persons, err = s.store.DatevPersonAccounts()
	return in, err
}

func expenseDatevDoc(x store.Expense) datevDoc {
	return datevDoc{Kind: "expense", ID: x.ID, Number: firstNonEmpty(x.DocumentNumber, x.Number), Date: x.Date, Partner: x.Supplier, Currency: "EUR",
		PartnerKey: "name:" + partnerKey(x.Supplier), Filename: x.AttachmentName, ReceivedID: x.ReceivedID}
}

// expenseFile reads the file behind a receipt; a receipt may have none.
func (s *InvoiceService) expenseFile(doc datevDoc) (string, []byte, error) {
	if doc.ReceivedID != "" {
		record, original, err := s.store.GetReceived(doc.ReceivedID)
		return record.Filename, original, err
	}
	if doc.Filename == "" {
		return "", nil, nil
	}
	x, err := s.store.ExpenseAttachment(doc.ID)
	return x.AttachmentName, x.Attachment, err
}

func issuedDatevDoc(d store.IssuedDocument) datevDoc {
	v := datevDoc{Kind: "issued", ID: d.Draft.ID, Number: d.Draft.Number, Date: d.Draft.IssueDate, Partner: d.Draft.Customer.Name,
		VATID: d.Draft.Customer.VATID, Country: strings.ToUpper(strings.TrimSpace(d.Draft.Customer.Address.Country)),
		Currency: firstNonEmpty(d.Draft.Currency, "EUR"), Treatment: d.Draft.TaxTreatment, Profile: d.Draft.Profile, Credit: d.Draft.Kind == "credit-note",
		Advance: advanceDraft(d.Draft), CorrectsID: d.Draft.CorrectsInvoiceID}
	if d.Draft.CustomerID != "" {
		v.PartnerKey = "customer:" + d.Draft.CustomerID
	} else {
		v.PartnerKey = "name:" + partnerKey(v.Partner)
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
	for _, t := range inv.TradeTaxes {
		v.Taxes = append(v.Taxes, datevTax{Category: t.CategoryCode, ReasonCode: t.ExemptionReasonCode, Reason: t.ExemptionReason, Percent: t.Percent, Basis: t.BasisAmount, Amount: t.CalculatedAmount})
	}
	return v
}

func readReceivedTaxes(doc *datevDoc, original []byte, kind string) {
	data, notice, err := receivedInvoiceXML(original, kind)
	if err != nil || notice != nil {
		doc.Problem = "Keine strukturierten Rechnungsdaten"
		return
	}
	inv, err := invoicexml.Parse(data)
	if err != nil || len(inv.TradeTaxes) == 0 {
		doc.Problem = "Rechnungsdaten nicht auswertbar"
		return
	}
	doc.Credit = inv.InvoiceTypeCode == 381
	if vat := normalizeVATID(inv.Seller.VATaxRegistration); vat != "" {
		doc.VATID = vat
		doc.PartnerKey = "vat:" + vat
	}
	for _, t := range inv.TradeTaxes {
		doc.Taxes = append(doc.Taxes, datevTax{Category: t.CategoryCode, ReasonCode: t.ExemptionReasonCode, Reason: t.ExemptionReason, Percent: t.Percent, Basis: t.BasisAmount, Amount: t.CalculatedAmount})
	}
}

func partnerKey(name string) string { return strings.ToLower(strings.Join(strings.Fields(name), " ")) }

func normalizeVATID(v string) string {
	return strings.ToUpper(strings.NewReplacer(" ", "", ".", "", "-", "").Replace(strings.TrimSpace(v)))
}

// EU member states by ISO code (Greece also as EL, Northern Ireland as XI).
var euCountries = map[string]bool{"AT": true, "BE": true, "BG": true, "CY": true, "CZ": true, "DE": true, "DK": true, "EE": true, "EL": true, "ES": true, "FI": true, "FR": true, "GR": true, "HR": true, "HU": true, "IE": true, "IT": true, "LT": true, "LU": true, "LV": true, "MT": true, "NL": true, "PL": true, "PT": true, "RO": true, "SE": true, "SI": true, "SK": true, "XI": true}

// revenueCase names the tax case of one tax group of an issued invoice and
// the account configured for it. ok is false for cases without a mapping.
func revenueCase(t datevTax, doc datevDoc, r datev.RevenueAccounts) (label, account string, ok bool) {
	switch t.Category {
	case "S":
		switch {
		case t.Percent.Equal(decimal.NewFromInt(19)):
			return "19 % USt", r.Standard19, true
		case t.Percent.Equal(decimal.NewFromInt(7)):
			return "7 % USt", r.Reduced7, true
		}
		return t.Percent.String() + " % USt", "", false
	case "Z":
		return "Nullsteuersatz", r.ZeroRated, true
	case "E":
		switch {
		case t.ReasonCode == "VATEX-EU-F" || t.ReasonCode == "VATEX-EU-I" || t.ReasonCode == "VATEX-EU-J" || strings.HasPrefix(doc.Treatment, "margin-"):
			return "Differenzbesteuerung", r.Margin, true
		case doc.Treatment == "small-business" || strings.Contains(t.Reason, "§ 19") || strings.Contains(t.Reason, "§19"):
			return "Kleinunternehmer § 19", r.SmallBusiness, true
		}
		return "steuerfrei", r.Exempt, true
	case "G":
		return "Ausfuhr", r.Export, true
	case "K":
		return "innergemeinschaftliche Lieferung", r.IntraCommunity, true
	case "AE":
		switch {
		case doc.Country == "" || doc.Country == "DE":
			return "§ 13b, Empfänger im Inland", r.ReverseCharge, true
		case euCountries[doc.Country]:
			return "§ 13b, Empfänger im übrigen EU-Gebiet", r.ReverseChargeEU, true
		}
		// A service for a business outside the EU is not taxable in Germany.
		return "nicht steuerbar, Empfänger außerhalb der EU", r.OutOfScope, true
	case "O":
		return "nicht steuerbar", r.OutOfScope, true
	}
	return "Steuerkategorie " + t.Category, "", false
}

// inputTaxKey is the BU-Schlüssel for a received invoice's tax group:
// 9 = Vorsteuer 19 %, 8 = Vorsteuer 7 %. Reverse charge is left to the tax
// advisor (review), other positive rates are not supported.
func inputTaxKey(t datevTax, smallBusiness bool) (key string, reverse, ok bool) {
	switch t.Category {
	case "S":
		switch {
		case smallBusiness && (t.Percent.Equal(decimal.NewFromInt(19)) || t.Percent.Equal(decimal.NewFromInt(7))):
			return "", false, true
		case t.Percent.Equal(decimal.NewFromInt(19)):
			return "9", false, true
		case t.Percent.Equal(decimal.NewFromInt(7)):
			return "8", false, true
		}
		return "", false, false
	case "Z", "E", "O", "G":
		return "", false, true
	case "AE", "K":
		return "", true, true
	}
	return "", false, false
}

func buildDatevExport(in datevInput) datevBuild {
	b := datevBuild{Preview: DatevPreview{From: in.From, To: in.To, Settings: in.Settings}}
	p := &b.Preview
	issues := map[string]*DatevIssue{}
	issue := func(code, severity, message string, e FinanceEntry) {
		if issues[code] == nil {
			issues[code] = &DatevIssue{Code: code, Severity: severity, Message: message, Entries: []FinanceEntry{}}
		}
		if e.ID != "" {
			issues[code].Entries = append(issues[code].Entries, e)
		}
	}
	set := in.Settings
	accounts := set.Accounts

	// Settings and period.
	if _, err := store.NormalizeDatevSettings(set); err != nil {
		issue("settings", severityBlocking, err.Error(), FinanceEntry{})
	}
	if set.UpdatedAt == "" {
		issue("settings_unsaved", severityBlocking, "Stimme die DATEV-Einstellungen mit deiner Steuerkanzlei ab und speichere sie einmal.", FinanceEntry{})
	}
	consultant, _ := strconv.Atoi(set.ConsultantNumber)
	client, _ := strconv.Atoi(set.ClientNumber)
	if consultant == 0 || client == 0 {
		issue("settings_numbers", severityBlocking, "Beraternummer und Mandantennummer fehlen. Du erhältst beide von deiner Steuerkanzlei.", FinanceEntry{})
	}
	if accounts.Debtor == "" || accounts.Creditor == "" {
		issue("settings_persons", severityBlocking, "Gib ein Debitoren- und ein Kreditorenkonto an.", FinanceEntry{})
	}
	from, e1 := time.Parse(time.DateOnly, in.From)
	to, e2 := time.Parse(time.DateOnly, in.To)
	var fyStart time.Time
	if e1 != nil || e2 != nil || to.Before(from) {
		issue("period", severityBlocking, "Wähle einen gültigen Zeitraum.", FinanceEntry{})
	} else {
		month := set.FiscalYearStartMonth
		if month < 1 || month > 12 {
			month = 1
		}
		fyStart = datev.FiscalYearStart(from, month)
		p.FiscalYearStart = fyStart.Format(time.DateOnly)
		if !to.Before(fyStart.AddDate(1, 0, 0)) {
			issue("period", severityBlocking, fmt.Sprintf("Ein Buchungsstapel darf nur ein Wirtschaftsjahr umfassen (ab %s). Teile den Zeitraum auf.", fyStart.Format("02.01.2006")), FinanceEntry{})
		}
	}
	chart := strings.TrimPrefix(set.Chart, "SKR")
	b.Header = datev.Header{Created: in.Created, ConsultantNumber: consultant, ClientNumber: client, FiscalYearStart: fyStart, AccountLength: set.AccountLength, From: from, To: to, Label: datev.Label(from, to), Chart: chart}

	// Person accounts: one collective account or consecutive numbers that
	// stay with the customer or supplier once assigned.
	known := map[string]string{}
	next := map[string]int{"debtor": 0, "creditor": 0}
	starts := map[string]string{"debtor": accounts.Debtor, "creditor": accounts.Creditor}
	for kind, start := range starts {
		n, _ := strconv.Atoi(start)
		next[kind] = n
	}
	for _, v := range in.Persons {
		known[v.Kind+"|"+v.Key] = v.Number
		if n, err := strconv.Atoi(v.Number); err == nil && n >= next[v.Kind] && len(v.Number) == len(starts[v.Kind]) {
			next[v.Kind] = n + 1
		}
	}
	person := func(kind string, doc datevDoc) string {
		if set.PersonAccounts != "individual" {
			return starts[kind]
		}
		if number := known[kind+"|"+doc.PartnerKey]; number != "" {
			return number
		}
		number := strconv.Itoa(next[kind])
		if len(number) != len(starts[kind]) || (kind == "debtor" && number >= accounts.Creditor && accounts.Creditor > accounts.Debtor) {
			issue("settings_persons", severityBlocking, "Im gewählten Nummernbereich sind keine Personenkonten mehr frei.", FinanceEntry{})
		}
		next[kind]++
		known[kind+"|"+doc.PartnerKey] = number
		b.NewPersons = append(b.NewPersons, store.DatevPersonAccount{Kind: kind, Key: doc.PartnerKey, Number: number, Name: doc.Partner})
		return number
	}

	entryOf := func(doc datevDoc) FinanceEntry {
		kind := "invoice"
		if doc.Kind == "received" || doc.Kind == "expense" {
			kind = doc.Kind
		}
		return FinanceEntry{Kind: kind, ID: doc.ID, Date: doc.Date, Label: invoiceLabel(doc.Number, doc.Partner)}
	}
	renamed := map[string]bool{}
	documentField := func(doc datevDoc) string {
		field, changed := datev.DocumentField(doc.Number)
		if changed && doc.Number != "" && !renamed[doc.Kind+"|"+doc.ID] {
			renamed[doc.Kind+"|"+doc.ID] = true
			e := entryOf(doc)
			e.Note = "Belegfeld 1: " + field
			issue("document_number_changed", severityReview, "DATEV erlaubt in Belegnummern keine Leerzeichen, Umlaute, Punkte oder Kommas. Wir haben diese Nummern angepasst.", e)
		}
		return field
	}
	booked := map[string]bool{}
	add := func(doc datevDoc, amount decimal.Decimal, side, account, counter, key, text, vat string, date time.Time, field string) {
		if amount.IsNegative() {
			amount = amount.Neg()
			side = map[string]string{"S": "H", "H": "S"}[side]
		}
		b.Bookings = append(b.Bookings, datevBooking{Booking: datev.Booking{Amount: amount, Side: side, Account: account, CounterAccount: counter, TaxKey: key, Date: date, Document: field, Text: text, EUVATID: vat}, DocKind: doc.Kind, DocID: doc.ID})
		booked[doc.Kind+"|"+doc.ID] = true
	}
	usable := func(doc datevDoc, without string) bool {
		e := entryOf(doc)
		switch {
		case doc.Problem != "":
			e.Note = doc.Problem
			issue(without, severityExcluded, map[string]string{
				"issued_without_tax": "Kundenrechnungen ohne auswertbare Steuerdaten fehlen im Export.",
				"received_unchecked": "Eingangsbelege ohne strukturierte, bestandene Rechnungsprüfung fehlen im Export. Gib sie deiner Kanzlei separat weiter.",
			}[without], e)
			return false
		case doc.Currency != "EUR":
			e.Note = doc.Currency
			issue("foreign_currency", severityExcluded, "Belege und Zahlungen in Fremdwährung fehlen im Export, weil kein Umrechnungskurs vorliegt.", e)
			return false
		}
		return true
	}

	// Issued invoices and credit notes by invoice date. Under Soll-Versteuerung
	// advance invoices are booked with their payments, like the Voranmeldung.
	onPayment := func(doc datevDoc) bool { return doc.Advance && in.TaxBasis != "ist" && !in.SmallBusiness }
	issued := map[string]datevDoc{}
	counted := map[string]bool{}
	for _, doc := range in.Issued {
		issued[doc.ID] = doc
	}
	bookIssued := func(doc datevDoc, taxes []datevTax, part func(decimal.Decimal) decimal.Decimal, date time.Time, prefix string) {
		type line struct {
			account, label string
			tax            datevTax
			gross          decimal.Decimal
		}
		var lines []line
		total := decimal.Zero
		excluded := false
		for _, t := range taxes {
			label, account, ok := revenueCase(t, doc, accounts.Revenue)
			e := entryOf(doc)
			e.Note = label
			if !ok {
				issue("unsupported_tax", severityExcluded, "Für diese Steuerfälle gibt es keine Standardkontierung. Die Rechnungen fehlen im Export. Kläre sie mit deiner Kanzlei.", e)
				excluded = true
				continue
			}
			if account == "" {
				issue("missing_account", severityExcluded, msgMissingAccount, e)
				excluded = true
				continue
			}
			gross := part(t.Basis).Add(part(t.Amount))
			total = total.Add(gross)
			lines = append(lines, line{account, label, t, gross})
		}
		if excluded {
			return
		}
		if total.IsZero() {
			issue("zero_amount", severityExcluded, "Belege über 0,00 € enthalten keine Buchung.", entryOf(doc))
			return
		}
		field := documentField(doc)
		debtor := person("debtor", doc)
		text := prefix + doc.Number + " " + doc.Partner
		side := "S"
		if doc.Credit {
			side, text = "H", "Gutschrift "+text
		}
		if !counted[doc.ID] {
			counted[doc.ID] = true
			if doc.Credit {
				p.CreditNotes++
			} else {
				p.Invoices++
			}
		}
		for _, l := range lines {
			if l.gross.IsZero() {
				continue
			}
			vat := ""
			if l.tax.Category == "K" || strings.Contains(l.label, "EU-Gebiet") {
				if vat = normalizeVATID(doc.VATID); vat == "" || len(vat) > 15 {
					vat = ""
					e := entryOf(doc)
					e.Note = l.label
					issue("missing_vat_id", severityReview, "Für EU-Umsätze fehlt die USt-IdNr. des Kunden. Du brauchst sie für die Zusammenfassende Meldung.", e)
				}
			}
			if l.label == "Differenzbesteuerung" {
				issue("margin_scheme", severityReview, "Differenzbesteuerung: Der Bruttobetrag ist ohne Steuer gebucht. Die Steuer auf die Marge ermittelt deine Kanzlei.", entryOf(doc))
			}
			add(doc, l.gross, side, debtor, l.account, "", text, vat, date, field)
		}
	}
	for _, doc := range in.Issued {
		if onPayment(doc) {
			continue
		}
		if inPeriod(doc.Date, in.From, in.To) && usable(doc, "issued_without_tax") {
			date, _ := time.Parse(time.DateOnly, doc.Date)
			bookIssued(doc, doc.Taxes, wholeAmount, date, "")
		}
	}
	var advanceDocs []advanceDoc
	for _, doc := range in.Issued {
		if onPayment(doc) {
			advanceDocs = append(advanceDocs, doc.advance())
		}
	}
	for _, doc := range in.Issued {
		if !onPayment(doc) {
			continue
		}
		var parts []advancePart
		var overpaid bool
		for _, part := range advanceParts(doc.advance(), in.Payments) {
			if inPeriod(part.Date, in.From, in.To) {
				parts = append(parts, part)
				overpaid = overpaid || part.Overpaid
			}
		}
		for _, part := range overpaidParts(doc.advance(), in.Payments) {
			overpaid = overpaid || inPeriod(part.Date, in.From, in.To)
		}
		if open := advanceOpen(doc.advance(), advanceDocs, in.Payments); open.IsPositive() && doc.Date <= in.To {
			e := entryOf(doc)
			e.Note = advanceOpenNote(open)
			issue("issued_advance_open", severityExcluded, "Anzahlungsrechnungen ohne vollständigen Zahlungseingang fehlen im Export, soweit nichts bezahlt ist. Sie stehen in der Voranmeldung als offener Punkt.", e)
		}
		dated, followPayment := []datevTax{}, []datevTax{}
		for _, t := range doc.Taxes {
			if documentDatedTax(t.Category, doc.Country) {
				dated = append(dated, t)
			} else {
				followPayment = append(followPayment, t)
			}
		}
		datedNow := inPeriod(doc.Date, in.From, in.To) && len(dated) > 0
		if (!datedNow && len(parts) == 0 && !overpaid) || !usable(doc, "issued_without_tax") {
			continue
		}
		if overpaid {
			issue("issued_advance_overpaid", severityReview, advanceOverpaidMessage, entryOf(doc))
		}
		if datedNow {
			date, _ := time.Parse(time.DateOnly, doc.Date)
			bookIssued(doc, dated, wholeAmount, date, "")
		}
		for _, part := range parts {
			date, _ := time.Parse(time.DateOnly, part.Date)
			bookIssued(doc, followPayment, part.of, date, "Anzahlung ")
			e := entryOf(doc)
			e.Date, e.Note = part.Date, "Zahlung vom "+germanDate(part.Date)
			issue("advance_booked", severityReview, "Diese Anzahlungsrechnungen sind mit dem Zahlungsdatum und dem bezahlten Anteil auf dem Erlöskonto gebucht, passend zur Voranmeldung. Ob ein Konto für erhaltene Anzahlungen besser passt, entscheidet deine Kanzlei.", e)
		}
	}

	// Received invoices by invoice date.
	received := map[string]datevDoc{}
	for _, doc := range in.Received {
		received[doc.ID] = doc
		if doc.Date == "" && doc.Problem != "" {
			usable(doc, "received_unchecked")
			continue
		}
		if !inPeriod(doc.Date, in.From, in.To) || !usable(doc, "received_unchecked") {
			continue
		}
		type line struct {
			key   string
			gross decimal.Decimal
		}
		var lines []line
		total := decimal.Zero
		excluded := false
		for _, t := range doc.Taxes {
			key, reverse, ok := inputTaxKey(t, in.SmallBusiness)
			e := entryOf(doc)
			e.Note = t.Category + " " + t.Percent.String() + " %"
			if !ok {
				issue("unsupported_tax", severityExcluded, "Für diese Steuerfälle gibt es keine Standardkontierung. Die Rechnungen fehlen im Export. Kläre sie mit deiner Kanzlei.", e)
				excluded = true
				continue
			}
			if reverse {
				issue("received_reverse_charge", severityReview, "Eingangsrechnungen mit Steuerschuld des Leistungsempfängers oder innergemeinschaftlichem Erwerb sind ohne Steuerschlüssel gebucht. Deine Kanzlei ergänzt Umsatz- und Vorsteuer.", entryOf(doc))
			}
			gross := t.Basis.Add(t.Amount)
			total = total.Add(gross)
			lines = append(lines, line{key, gross})
		}
		if accounts.Expense == "" {
			issue("missing_account", severityExcluded, msgMissingAccount, FinanceEntry{Kind: "received", ID: doc.ID, Date: doc.Date, Label: invoiceLabel(doc.Number, doc.Partner), Note: "Aufwandskonto"})
			excluded = true
		}
		if excluded {
			continue
		}
		if total.IsZero() {
			issue("zero_amount", severityExcluded, "Belege über 0,00 € enthalten keine Buchung.", entryOf(doc))
			continue
		}
		field := documentField(doc)
		date, _ := time.Parse(time.DateOnly, doc.Date)
		creditor := person("creditor", doc)
		expenseAccount := accounts.Expense
		if c := in.ReceivedCategories[doc.ID].Category; c != "" {
			expenseAccount = datev.CategoryAccount(accounts, set.Chart, set.AccountLength, c)
		}
		side, text := "H", doc.Partner+" "+doc.Number
		if doc.Credit {
			side, text = "S", "Gutschrift "+text
		}
		p.Received++
		for _, l := range lines {
			if !l.gross.IsZero() {
				add(doc, l.gross, side, creditor, expenseAccount, l.key, text, "", date, field)
			}
		}
	}

	// Receipts entered by hand, by receipt date; advance payments by payment date
	// like the Voranmeldung. Purchases for several years go to the asset account;
	// their depreciation stays with the tax advisor.
	for _, x := range in.Expenses {
		doc := expenseDatevDoc(x)
		day := x.Date
		if x.Advance {
			day = x.PaidDate
		}
		if !inPeriod(day, in.From, in.To) {
			continue
		}
		doc.Date = day
		gross, err := decimal.NewFromString(x.Gross)
		if err != nil {
			continue
		}
		type line struct {
			key   string
			gross decimal.Decimal
		}
		lines := []line{{"", gross}}
		switch {
		case in.SmallBusiness, x.TaxRate == "none", x.TaxRate == "0":
		case store.ReverseChargeRate(x.TaxRate):
			issue("received_reverse_charge", severityReview, "Eingangsrechnungen mit Steuerschuld des Leistungsempfängers oder innergemeinschaftlichem Erwerb sind ohne Steuerschlüssel gebucht. Deine Kanzlei ergänzt Umsatz- und Vorsteuer.", entryOf(doc))
		case x.TaxRate == "19":
			lines = []line{{"9", gross}}
		case x.TaxRate == "7":
			lines = []line{{"8", gross}}
		case x.TaxRate == store.RateMixed:
			reduced, err := decimal.NewFromString(x.ReducedGross)
			if err != nil {
				issue("unsupported_tax", severityExcluded, "Für diese Steuerfälle gibt es keine Standardkontierung. Die Rechnungen fehlen im Export. Kläre sie mit deiner Kanzlei.", entryOf(doc))
				continue
			}
			lines = []line{{"9", gross.Sub(reduced)}, {"8", reduced}}
		default:
			issue("unsupported_tax", severityExcluded, "Für diese Steuerfälle gibt es keine Standardkontierung. Die Rechnungen fehlen im Export. Kläre sie mit deiner Kanzlei.", entryOf(doc))
			continue
		}
		account := datev.CategoryAccount(accounts, set.Chart, set.AccountLength, x.Category)
		if account == "" {
			e := entryOf(doc)
			e.Note = "Aufwandskonto"
			issue("missing_account", severityExcluded, msgMissingAccount, e)
			continue
		}
		switch x.Category {
		case taxforms.AssetCategory:
			issue("expense_asset", severityReview, "Diese Anschaffungen sind auf das Konto für Betriebs- und Geschäftsausstattung gebucht. Die Abschreibung und das genaue Anlagekonto legt deine Kanzlei fest.", entryOf(doc))
		case "bewirtung":
			issue("expense_entertainment", severityReview, "Bewirtungskosten sind voll auf einem Konto gebucht. Die Aufteilung in abziehbar und nicht abziehbar übernimmt deine Kanzlei.", entryOf(doc))
		}
		if x.FileSum == "" && x.ReceivedID == "" {
			issue("expense_without_file", severityReview, "Zu diesen Ausgaben gibt es keine Belegdatei im ZIP. Gib deiner Kanzlei das Papier weiter.", entryOf(doc))
		}
		field := documentField(doc)
		date, _ := time.Parse(time.DateOnly, day)
		creditor := person("creditor", doc)
		text := doc.Partner + " " + doc.Number
		p.Expenses++
		for _, l := range lines {
			if !l.gross.IsZero() {
				add(doc, l.gross, "H", creditor, account, l.key, text, "", date, field)
			}
		}
	}

	// Optional payments: the bank account against the person account, with
	// the invoice number in Belegfeld 1 so DATEV can clear the open item.
	if set.IncludePayments {
		for _, x := range in.Expenses {
			if x.PaidDate == "" || !inPeriod(x.PaidDate, in.From, in.To) {
				continue
			}
			doc := expenseDatevDoc(x)
			gross, err := decimal.NewFromString(x.Gross)
			if err != nil {
				continue
			}
			e := FinanceEntry{Kind: "payment", ID: x.ID, Date: x.PaidDate, Label: "Zahlung zu " + invoiceLabel(doc.Number, doc.Partner), Amount: x.Gross}
			switch {
			case x.PaidWith != "bank":
				e.Note = map[string]string{"cash": "bar", "private": "privat"}[x.PaidWith]
				issue("expense_not_bank", severityReview, "Bar oder privat bezahlte Ausgaben stehen ohne Zahlungsbuchung im Export. Deine Kanzlei bucht sie auf Kasse oder Privat.", e)
				continue
			case accounts.Bank == "":
				e.Note = "Bankkonto"
				issue("missing_account", severityExcluded, msgMissingAccount, e)
				continue
			}
			date, _ := time.Parse(time.DateOnly, x.PaidDate)
			payment := doc
			payment.Kind, payment.ID = "payment", x.ID
			add(payment, gross, "H", accounts.Bank, person("creditor", doc), "", "Zahlung "+doc.Number+" "+doc.Partner, "", date, documentField(doc))
			p.Payments++
		}
		for _, pay := range in.Payments {
			if pay.RevokedAt != "" || !inPeriod(pay.Date, in.From, in.To) {
				continue
			}
			doc, ok := issued[pay.InvoiceID]
			kind := "debtor"
			if pay.Direction == "incoming" {
				doc, ok = received[pay.InvoiceID]
				kind = "creditor"
			}
			e := FinanceEntry{Kind: "payment", ID: pay.ID, Date: pay.Date, Label: "Zahlung zu " + invoiceLabel(doc.Number, doc.Partner), Amount: pay.Amount, Note: sourceNote(pay.Source)}
			amount, err := decimal.NewFromString(pay.Amount)
			switch {
			case !ok || err != nil:
				issue("payment_without_invoice", severityExcluded, "Zahlungen ohne auswertbare Rechnung fehlen im Export.", e)
				continue
			case pay.Currency != "EUR":
				e.Note = pay.Currency
				issue("foreign_currency", severityExcluded, "Belege und Zahlungen in Fremdwährung fehlen im Export, weil kein Umrechnungskurs vorliegt.", e)
				continue
			case accounts.Bank == "":
				e.Note = "Bankkonto"
				issue("missing_account", severityExcluded, msgMissingAccount, e)
				continue
			case amount.IsZero():
				continue
			}
			field := documentField(doc)
			date, _ := time.Parse(time.DateOnly, pay.Date)
			// Money received on an invoice is a debit on the bank account;
			// refunds on credit notes and payments to suppliers are credits.
			side := "S"
			if kind == "creditor" {
				side = "H"
			}
			if doc.Credit {
				side = map[string]string{"S": "H", "H": "S"}[side]
			}
			payment := doc
			payment.Kind, payment.ID = "payment", pay.ID
			add(payment, amount, side, accounts.Bank, person(kind, doc), "", "Zahlung "+doc.Number+" "+doc.Partner, "", date, field)
			p.Payments++
		}
	}

	sort.SliceStable(b.Bookings, func(i, j int) bool {
		a, c := b.Bookings[i], b.Bookings[j]
		if !a.Date.Equal(c.Date) {
			return a.Date.Before(c.Date)
		}
		return a.Document < c.Document
	})
	p.Bookings = len(b.Bookings)
	p.NewAccounts = len(b.NewPersons)
	if set.IncludeDocuments {
		for key := range booked {
			if !strings.HasPrefix(key, "payment|") {
				p.Documents++
			}
		}
	}
	if len(issues) == 0 || !hasBlocking(issues) {
		if len(b.Bookings) == 0 {
			issue("empty", severityBlocking, "Im Zeitraum gibt es keine Belege, die du exportieren kannst.", FinanceEntry{})
		} else if _, err := writeDatev(b); err != nil {
			issue("format", severityBlocking, "Die DATEV-Datei wäre ungültig: "+err.Error(), FinanceEntry{})
		}
	}

	p.Issues = []DatevIssue{}
	for _, code := range []string{"settings", "settings_unsaved", "settings_numbers", "settings_persons", "period", "empty", "format",
		"missing_account", "unsupported_tax", "issued_without_tax", "received_unchecked", "foreign_currency", "zero_amount", "payment_without_invoice",
		"issued_advance_open", "document_number_changed", "missing_vat_id", "margin_scheme", "received_reverse_charge", "expense_asset", "expense_entertainment", "expense_without_file", "expense_not_bank", "advance_booked", "issued_advance_overpaid"} {
		if v := issues[code]; v != nil {
			p.Issues = append(p.Issues, *v)
		}
	}
	p.Ready = !hasBlocking(issues)
	p.NotCovered = []string{
		"Bankumsätze ohne Rechnung, z. B. Gebühren, Privatentnahmen oder Steuerzahlungen",
		"Abschreibungen und andere Buchungen ohne Beleg",
		"Leistungsdatum und Fälligkeit: Die Steuerperiode ergibt sich aus dem Rechnungsdatum, bei Anzahlungsrechnungen unter Soll-Versteuerung aus dem Zahlungsdatum",
	}
	if !set.IncludePayments {
		p.NotCovered = append(p.NotCovered, "Zahlungen: Deine Kanzlei bucht sie meist direkt aus den Kontoauszügen")
	}
	if set.PersonAccounts == "individual" {
		p.NotCovered = append(p.NotCovered, "Stammdaten der Personenkonten: Namen stehen im Buchungstext, Adressen legt deine Kanzlei an")
	}
	if set.IncludeDocuments {
		p.NotCovered = append(p.NotCovered, "Verknüpfung der Belege in DATEV: Die Belege liegen als ZIP bei und werden von deiner Kanzlei zugeordnet")
	}
	return b
}

func hasBlocking(issues map[string]*DatevIssue) bool {
	for _, v := range issues {
		if v.Severity == severityBlocking {
			return true
		}
	}
	return false
}

// datevDocuments packs the originals of all booked invoices. File names start
// with the Belegfeld 1 value so the tax advisor can match them to bookings.
func (s *InvoiceService) datevDocuments(in datevInput, b datevBuild) ([]byte, []FinanceEntry, error) {
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	seen := map[string]bool{}
	names := map[string]bool{}
	var missing []FinanceEntry
	docs := map[string]datevDoc{}
	for _, d := range in.Issued {
		docs["issued|"+d.ID] = d
	}
	for _, d := range in.Received {
		docs["received|"+d.ID] = d
	}
	for _, x := range in.Expenses {
		docs["expense|"+x.ID] = expenseDatevDoc(x)
	}
	write := func(name string, data []byte) error {
		base, ext := strings.TrimSuffix(name, filepath.Ext(name)), filepath.Ext(name)
		for i := 2; names[name]; i++ {
			name = fmt.Sprintf("%s (%d)%s", base, i, ext)
		}
		names[name] = true
		f, err := w.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Deflate, Modified: in.Created})
		if err != nil {
			return err
		}
		_, err = f.Write(data)
		return err
	}
	for _, booking := range b.Bookings {
		key := booking.DocKind + "|" + booking.DocID
		doc, ok := docs[key]
		if !ok || seen[key] {
			continue
		}
		seen[key] = true
		entry := FinanceEntry{Kind: "invoice", ID: doc.ID, Date: doc.Date, Label: invoiceLabel(doc.Number, doc.Partner)}
		field := exportFilename(firstNonEmpty(booking.Document, doc.ID))
		if doc.Kind == "issued" {
			pdf, err := s.store.Artifact(doc.ID, "facturx-pdf")
			if err != nil || len(pdf) == 0 {
				missing = append(missing, entry)
				continue
			}
			if err := write("Ausgang/"+field+".pdf", pdf); err != nil {
				return nil, nil, err
			}
			if strings.HasPrefix(doc.Profile, "xrechnung-") {
				if xml, err := s.store.Artifact(doc.ID, "cii-xml"); err == nil && len(xml) > 0 {
					if err := write("Ausgang/"+field+".xml", xml); err != nil {
						return nil, nil, err
					}
				}
			}
			continue
		}
		if doc.Kind == "expense" {
			name, data, err := s.expenseFile(doc)
			if err != nil {
				missing = append(missing, entry)
				continue
			}
			if len(data) > 0 {
				if err := write("Eingang/"+field+" - "+exportFilename(name), data); err != nil {
					return nil, nil, err
				}
			}
			continue
		}
		entry.Kind = "received"
		_, original, err := s.store.GetReceived(doc.ID)
		if err != nil || len(original) == 0 {
			missing = append(missing, entry)
			continue
		}
		if err := write("Eingang/"+field+" - "+exportFilename(doc.Filename), original); err != nil {
			return nil, nil, err
		}
	}
	if err := w.Close(); err != nil {
		return nil, nil, err
	}
	return buf.Bytes(), missing, nil
}
