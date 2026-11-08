package app

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"papierleicht.de/e-rechnung-studio/internal/model"
	"papierleicht.de/e-rechnung-studio/internal/store"
)

// TestSeedTestData fills a new data folder with a fictitious design studio
// for trying the app by hand: customers, a year of invoices up to today,
// receipts, supplier e-invoices, a bank account from CSV, Umsatzsteuer
// filings and tax payments. It runs only when E_RECHNUNG_STUDIO_SEED_DIR names
// an empty or missing folder; `make testdata` sets it.
//
// Everything goes through the service like a click in the app would, so the
// data follows the same rules and protocols as real data. Dates are relative
// to today, so the overview always shows a current year.
func TestSeedTestData(t *testing.T) {
	dir := os.Getenv("E_RECHNUNG_STUDIO_SEED_DIR")
	if dir == "" {
		t.Skip("E_RECHNUNG_STUDIO_SEED_DIR not set")
	}
	if entries, err := os.ReadDir(dir); err == nil && len(entries) > 0 {
		t.Fatalf("%s is not empty; remove it first", dir)
	}
	s, err := NewInvoiceService(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	seed := testSeed{t: t, s: s, today: time.Now()}
	seed.run()
	t.Logf("Testdaten in %s", dir)
}

type testSeed struct {
	t     *testing.T
	s     *InvoiceService
	today time.Time
	// The bank statement grows while invoices and receipts are created.
	csv []string
}

const seedAccount = "DE02120300000000202051"

var seedCompany = model.Company{
	Name:        "Lindenblatt Gestaltung",
	ContactName: "Mara Lindner",
	Email:       "hallo@lindenblatt.example",
	Phone:       "0341 1234567",
	TaxNumber:   "231/123/45678",
	VATID:       "DE123456789",
	IBAN:        seedAccount,
	BIC:         "BYLADEM1001",
	BankName:    "Testbank Leipzig",
	Address:     model.Address{Street: "Lindenauer Markt 7", Postcode: "04177", City: "Leipzig", Country: "DE"},
}

var seedCustomers = []model.Customer{
	{Name: "Kaffeerösterei Bohnenwerk GmbH", Email: "buchhaltung@bohnenwerk.example", Address: model.Address{Street: "Karl-Liebknecht-Str. 12", Postcode: "04107", City: "Leipzig", Country: "DE"}},
	{Name: "Praxis Dr. Sommer", Email: "praxis@sommer.example", Address: model.Address{Street: "Gohliser Str. 4", Postcode: "04155", City: "Leipzig", Country: "DE"}},
	{Name: "Verlag Elbsand KG", Email: "rechnung@elbsand.example", Address: model.Address{Street: "Königsbrücker Str. 30", Postcode: "01099", City: "Dresden", Country: "DE"}},
	{Name: "Radhaus Weber", Email: "info@radhaus-weber.example", Address: model.Address{Street: "Zschochersche Str. 50", Postcode: "04229", City: "Leipzig", Country: "DE"}},
	{Name: "Stadtwerke Muldental GmbH", Email: "eingangsrechnung@sw-muldental.example", Address: model.Address{Street: "Am Werk 1", Postcode: "04668", City: "Grimma", Country: "DE"}},
	{Name: "Weingut Saalehang", Email: "kontor@saalehang.example", Address: model.Address{Street: "Weinbergweg 3", Postcode: "06632", City: "Freyburg", Country: "DE"}},
}

// issue issues an invoice and names the validation findings when it fails.
func (x *testSeed) issue(s *InvoiceService, d model.InvoiceDraft) model.InvoiceView {
	x.t.Helper()
	view, err := s.IssueInvoice(d)
	if err != nil {
		checked, _ := s.ValidateInvoice(d)
		x.t.Fatalf("%v: %+v", err, checked.Validation)
	}
	return view
}

func (x *testSeed) day(offset int) time.Time { return x.today.AddDate(0, 0, offset) }

func iso(d time.Time) string { return d.Format(time.DateOnly) }

func (x *testSeed) must(err error) {
	x.t.Helper()
	if err != nil {
		x.t.Fatal(err)
	}
}

// booking adds a line to the bank statement; the purpose identifies it later.
func (x *testSeed) booking(date time.Time, amount decimal.Decimal, partner, iban, purpose string) {
	x.csv = append(x.csv, strings.Join([]string{
		date.Format("02.01.2006"),
		strings.ReplaceAll(amount.StringFixed(2), ".", ","),
		"EUR", partner, iban, purpose,
	}, ";"))
}

func (x *testSeed) importStatement() {
	x.t.Helper()
	if len(x.csv) == 0 {
		return
	}
	text := "Buchungstag;Betrag;Währung;Name;IBAN;Verwendungszweck\n" + strings.Join(x.csv, "\n") + "\n"
	_, err := x.s.ImportPaymentCSV(seedAccount, text)
	x.must(err)
	x.csv = nil
}

// bookingID finds an imported booking by a unique part of its purpose.
func (x *testSeed) bookingID(purpose string) string {
	x.t.Helper()
	rows, err := x.s.PaymentBookings()
	x.must(err)
	for _, b := range rows {
		if strings.Contains(b.Data.Description, purpose) {
			return b.ID
		}
	}
	x.t.Fatalf("no booking with %q", purpose)
	return ""
}

type seedInvoice struct {
	customer int
	// issued and paid are days before today; paid 0 means open.
	issued, term, paid int
	lines              []model.InvoiceLine
	// part pays only this gross amount.
	part string
}

func line(name, quantity, unit, price string) model.InvoiceLine {
	return model.InvoiceLine{Name: name, Quantity: quantity, Unit: unit, UnitPrice: price, TaxRate: "19", TaxCategory: "S"}
}

func (x *testSeed) run() {
	company := seedCompany
	_, err := x.s.CompleteSetup(company)
	x.must(err)
	_, err = x.s.SaveFinanceSettings(store.FinanceSettings{
		TaxBasis:       "soll",
		ReservePercent: "25",
		VATPeriod:      "quarterly",
		Filing:         "self",
		Activity:       "trade",
		// The statement starts on January 1; the balance before it is entered once.
		Balances: []store.AccountBalance{{AccountID: seedAccount, Amount: "6200,00", Date: fmt.Sprintf("%d-12-31", x.today.Year()-1)}},
	})
	x.must(err)

	customers := make([]model.Customer, len(seedCustomers))
	for i, c := range seedCustomers {
		customers[i], err = x.s.SaveCustomer(c)
		x.must(err)
	}

	// Invoices over the year so far: most paid, the recent ones open, two overdue.
	yearStart := time.Date(x.today.Year(), 1, 1, 0, 0, 0, 0, time.Local)
	daysIntoYear := int(x.today.Sub(yearStart).Hours() / 24)
	at := func(fraction float64) int { return daysIntoYear - int(float64(daysIntoYear)*fraction) }
	invoices := []seedInvoice{
		{customer: 0, issued: at(0.04), term: 14, paid: at(0.08), lines: []model.InvoiceLine{line("Website-Relaunch, Gestaltung und Umsetzung", "1", "C62", "4000.00")}},
		{customer: 1, issued: at(0.12), term: 14, paid: at(0.15), lines: []model.InvoiceLine{line("Logo und Praxisschild", "1", "C62", "1200.00")}},
		{customer: 2, issued: at(0.2), term: 30, paid: at(0.27), lines: []model.InvoiceLine{line("Buchumschlag „Flussläufe“", "1", "C62", "1800.00"), line("Satz Innenteil", "14", "HUR", "50.00")}},
		{customer: 0, issued: at(0.3), term: 14, paid: at(0.34), lines: []model.InvoiceLine{line("Website-Wartung Quartal", "8", "HUR", "75.00")}},
		{customer: 3, issued: at(0.4), term: 14, paid: at(0.45), lines: []model.InvoiceLine{line("Messestand-Grafik", "1", "C62", "2600.00"), line("Druckdatenaufbereitung", "8", "HUR", "75.00")}},
		{customer: 1, issued: at(0.5), term: 14, paid: at(0.54), lines: []model.InvoiceLine{line("Social-Media-Paket, 12 Motive", "12", "C62", "150.00")}},
		{customer: 4, issued: at(0.58), term: 30, paid: at(0.66), lines: []model.InvoiceLine{line("Geschäftsbericht, Gestaltung", "1", "C62", "4600.00"), line("Infografiken", "4", "C62", "200.00")}},
		{customer: 2, issued: at(0.7), term: 14, paid: at(0.74), lines: []model.InvoiceLine{line("Verlagsprospekt Herbst", "12", "HUR", "75.00")}},
		{customer: 0, issued: at(0.8), term: 14, paid: at(0.83), lines: []model.InvoiceLine{line("Website-Wartung Quartal", "8", "HUR", "75.00")}},
		// Open: overdue for weeks, partly paid and overdue, and two not yet due.
		{customer: 3, issued: 28, term: 14, lines: []model.InvoiceLine{line("Katalog Frühjahr, 32 Seiten", "1", "C62", "4600.00")}},
		{customer: 5, issued: 16, term: 14, paid: 6, part: "1000.00", lines: []model.InvoiceLine{line("Etiketten-Design, 3 Weine", "3", "C62", "800.00"), line("Reinzeichnung", "6", "HUR", "75.00")}},
		{customer: 1, issued: 6, term: 14, lines: []model.InvoiceLine{line("Newsletter-Vorlagen", "10", "HUR", "75.00")}},
		{customer: 4, issued: 1, term: 30, lines: []model.InvoiceLine{line("Kampagnenplanung Folgejahr", "20", "HUR", "75.00")}},
	}
	payments := map[string]string{} // purpose tag → invoice id
	for _, inv := range invoices {
		d := model.NewDraft()
		issued := x.day(-inv.issued)
		d.IssueDate = iso(issued)
		d.DeliveryDate = iso(issued)
		d.DueDate = iso(issued.AddDate(0, 0, inv.term))
		d.PaymentTerms = fmt.Sprintf("Zahlbar innerhalb von %d Tagen ohne Abzug.", inv.term)
		d.Customer = customers[inv.customer]
		d.Lines = nil
		for i, l := range inv.lines {
			l.ID = fmt.Sprint(i + 1)
			d.Lines = append(d.Lines, l)
		}
		view := x.issue(x.s, d)
		if inv.paid == 0 {
			continue
		}
		amount := decimal.RequireFromString(view.Totals.Due)
		if inv.part != "" {
			amount = decimal.RequireFromString(inv.part)
		}
		purpose := "Rechnung " + view.Draft.Number
		x.booking(x.day(-inv.paid), amount, customers[inv.customer].Name, "", purpose)
		payments[purpose] = view.Draft.ID
	}
	// A draft that was started but not issued.
	draft := model.NewDraft()
	draft.Customer = customers[2]
	draft.Lines = []model.InvoiceLine{line("Lesezeichen zum Buch", "500", "C62", "0.40")}
	draft.Lines[0].ID = "1"
	_, err = x.s.SaveInvoice(draft)
	x.must(err)

	// Monthly and occasional costs, paid from the account.
	type cost struct {
		supplier, iban, category, rate, gross, note string
	}
	monthly := []cost{
		{"Atelierhaus Plagwitz eG", "DE75512108001245126199", "miete", "none", "650.00", "Miete Atelier"},
		{"Grafiksoftware Abo", "DE02100100100006820101", "edv", "19", "71.39", "Software-Abo"},
		{"Netzanbieter Mitte", "DE02100100100006820101", "telekommunikation", "19", "39.95", "Internet und Telefon"},
	}
	type expense struct {
		cost
		date time.Time
		tag  string
	}
	var expenses []expense
	for m := 1; m <= int(x.today.Month()); m++ {
		date := time.Date(x.today.Year(), time.Month(m), 3, 0, 0, 0, 0, time.Local)
		if date.After(x.today) {
			break
		}
		for _, c := range monthly {
			expenses = append(expenses, expense{c, date, fmt.Sprintf("%s %02d/%d", c.note, m, x.today.Year())})
		}
	}
	expenses = append(expenses, []expense{
		{cost{"Haftpflicht Kreativ VVaG", "DE75512108001245126199", "versicherung", "none", "240.00", "Betriebshaftpflicht"}, x.day(-at(0.05) * 1), "Betriebshaftpflicht Jahresbeitrag"},
		{cost{"Steuerkanzlei Rosental", "DE89370400440532013000", "beratung", "19", "297.50", "Jahresabschluss"}, x.day(-at(0.25)), "Jahresabschluss Vorjahr"},
		{cost{"Typo-Akademie", "DE02100100100006820101", "fortbildung", "19", "450.00", "Workshop Schriftgestaltung"}, x.day(-at(0.6)), "Workshop Schriftgestaltung"},
		{cost{"Bürobedarf Leipzig", "", "arbeitsmittel", "19", "86.70", "Papier und Druckerpatronen"}, x.day(-at(0.7)), "Bueromaterial Bestellung 4711"},
	}...)
	for _, e := range expenses {
		x.booking(e.date, decimal.RequireFromString(e.gross).Neg(), e.supplier, e.iban, e.tag)
	}

	// Income tax prepayments and private withdrawals.
	for q := 1; q <= 4; q++ {
		date := time.Date(x.today.Year(), time.Month(3*q), 10, 0, 0, 0, 0, time.Local)
		if date.Before(x.today) {
			x.booking(date, decimal.RequireFromString("-600.00"), "Finanzamt Leipzig II", "DE45860000000086001520", fmt.Sprintf("ESt-Vorauszahlung %d/%d", q, x.today.Year()))
		}
	}
	for m := 1; m <= int(x.today.Month()); m++ {
		date := time.Date(x.today.Year(), time.Month(m), 28, 0, 0, 0, 0, time.Local)
		if date.Before(x.today) {
			x.booking(date, decimal.RequireFromString("-1000.00"), "Mara Lindner", "DE02100100100006820101", fmt.Sprintf("Privatentnahme %02d/%d", m, x.today.Year()))
		}
	}
	// Two bookings left for the user to assign.
	x.booking(x.day(-4), decimal.RequireFromString("-34.99"), "Online-Versand", "", "Bestellung 302-118 Kabel")
	x.booking(x.day(-2), decimal.RequireFromString("450.00"), "Weingut Saalehang", "", "Abschlag Etiketten")
	x.importStatement()

	for purpose, id := range payments {
		_, err := x.s.RecordPayment(store.Payment{InvoiceID: id, Direction: "outgoing", BookingID: x.bookingID(purpose), Source: "bank", Reason: "Bankumsatz"})
		x.must(err)
	}
	for _, e := range expenses {
		// Linked to its bank booking, as „Als Ausgabe erfassen“ does.
		_, err := x.s.SaveExpense(ExpenseUpload{Expense: store.Expense{
			Date: iso(e.date), PaidDate: iso(e.date), Supplier: e.supplier, Note: e.note,
			Category: e.category, Gross: e.gross, TaxRate: e.rate, PaidWith: "bank",
			BookingID: x.bookingID(e.tag),
		}})
		x.must(err)
	}
	rows, err := x.s.PaymentBookings()
	x.must(err)
	for _, b := range rows {
		switch {
		case strings.HasPrefix(b.Data.Description, "ESt-Vorauszahlung"):
			x.must(x.s.SetBookingTreatment(b.ID, "tax", "Einkommensteuer-Vorauszahlung"))
		case strings.HasPrefix(b.Data.Description, "Privatentnahme"):
			x.must(x.s.SetBookingTreatment(b.ID, "private", "Privatentnahme"))
		}
	}

	// Supplier e-invoices: one paid, two still to pay.
	x.supplierInvoices()

	// Umsatzsteuer: finished quarters are filed and paid ten days after their end.
	for q := 1; q <= 4; q++ {
		end := time.Date(x.today.Year(), time.Month(3*q)+1, 0, 0, 0, 0, 0, time.Local)
		due := end.AddDate(0, 0, 10)
		if !due.Before(x.today) {
			break
		}
		period := fmt.Sprintf("%d-Q%d", x.today.Year(), q)
		ret, err := x.s.VATReturn(period)
		x.must(err)
		payable := decimal.RequireFromString(ret.Return.Payable)
		x.must(x.s.MarkVATFiled(period, iso(due.AddDate(0, 0, -2)), payable.StringFixed(2)))
		x.booking(due, payable.Neg(), "Finanzamt Leipzig II", "DE45860000000086001520", "USt-VA "+period)
	}
	x.importStatement()
	rows, err = x.s.PaymentBookings()
	x.must(err)
	for _, b := range rows {
		if strings.HasPrefix(b.Data.Description, "USt-VA ") {
			x.must(x.s.SetBookingTreatment(b.ID, "vat", "Umsatzsteuer-Vorauszahlung"))
		}
	}
	_, err = x.s.ReconcilePayments()
	x.must(err)

	r, err := x.s.FinanceReport(fmt.Sprintf("%d-01-01", x.today.Year()), iso(x.today))
	x.must(err)
	x.t.Logf("Kontostand %s, offene Forderungen %s, anstehende Zahlungen %s, offene USt %s, Rücklage %s, Ergebnis %s",
		r.BankBalance.Amount, r.Receivables.Amount, r.Payables.Amount, r.VATReserve.Amount, r.IncomeTaxReserve.Amount, r.Result.Amount)
	for _, issue := range r.Issues {
		x.t.Logf("Offener Punkt: %s (%d)", issue.Message, len(issue.Entries))
	}
	list, err := x.s.ListInvoices()
	x.must(err)
	for _, inv := range list {
		x.t.Logf("%s %s %s %s", inv.Number, inv.CustomerName, inv.Status, inv.Gross)
	}
}

// supplierInvoices issues e-invoices in a second, temporary service as the
// supplier and imports them here, as if they had arrived by mail.
func (x *testSeed) supplierInvoices() {
	dir, err := os.MkdirTemp("", "seed-supplier-*")
	x.must(err)
	defer os.RemoveAll(dir)
	type supplier struct {
		company model.Company
		issued  int
		term    int
		lines   []model.InvoiceLine
		paid    bool
	}
	buyer := model.Customer{Name: seedCompany.Name, Email: seedCompany.Email, VATID: seedCompany.VATID, Address: seedCompany.Address}
	suppliers := []supplier{
		{model.Company{Name: "Druckerei Lößnig GmbH", ContactName: "Jens Kraus", Email: "rechnung@druckerei-loessnig.example", TaxNumber: "231/987/65432", VATID: "DE987654321", IBAN: "DE89370400440532013000", BIC: "COBADEFFXXX", BankName: "Testbank", Address: model.Address{Street: "Bornaische Str. 90", Postcode: "04279", City: "Leipzig", Country: "DE"}},
			120, 14, []model.InvoiceLine{line("Druck Broschüre, 500 Stück", "500", "C62", "2.00")}, true},
		{model.Company{Name: "Druckerei Lößnig GmbH", ContactName: "Jens Kraus", Email: "rechnung@druckerei-loessnig.example", TaxNumber: "231/987/65432", VATID: "DE987654321", IBAN: "DE89370400440532013000", BIC: "COBADEFFXXX", BankName: "Testbank", Address: model.Address{Street: "Bornaische Str. 90", Postcode: "04279", City: "Leipzig", Country: "DE"}},
			9, 14, []model.InvoiceLine{line("Druck Katalog, 800 Stück", "800", "C62", "1.50")}, false},
		{model.Company{Name: "Fotostudio Westlicht", ContactName: "Ina Berg", Email: "ina@westlicht.example", TaxNumber: "231/555/12345", VATID: "DE555123456", IBAN: "DE75512108001245126199", BIC: "SOGEDEFFXXX", BankName: "Testbank", Address: model.Address{Street: "Karl-Heine-Str. 22", Postcode: "04229", City: "Leipzig", Country: "DE"}},
			4, 21, []model.InvoiceLine{line("Produktfotos Weinflaschen", "1", "DAY", "800.00")}, false},
	}
	for i, sup := range suppliers {
		other, err := NewInvoiceService(filepath.Join(dir, fmt.Sprint(i)))
		x.must(err)
		_, err = other.CompleteSetup(sup.company)
		x.must(err)
		d := model.NewDraft()
		issued := x.day(-sup.issued)
		d.IssueDate = iso(issued)
		d.DeliveryDate = iso(issued)
		d.DueDate = iso(issued.AddDate(0, 0, sup.term))
		d.PaymentTerms = fmt.Sprintf("Zahlbar innerhalb von %d Tagen.", sup.term)
		d.Customer = buyer
		d.Lines = nil
		for j, l := range sup.lines {
			l.ID = fmt.Sprint(j + 1)
			d.Lines = append(d.Lines, l)
		}
		view := x.issue(other, d)
		xml := view.XML
		if xml == "" {
			path, err := other.ExportInvoiceXML(view.Draft.ID, filepath.Join(dir, fmt.Sprintf("%d.xml", i)))
			x.must(err)
			data, err := os.ReadFile(path)
			x.must(err)
			xml = string(data)
		}
		other.Close()
		name := fmt.Sprintf("%s.xml", view.Draft.Number)
		imported, err := x.s.ImportReceivedInvoice(name, base64.StdEncoding.EncodeToString([]byte(xml)))
		x.must(err)
		if !sup.paid {
			continue
		}
		// Supplier numbers can repeat ours; the name keeps the purpose unique.
		purpose := "Ihre Rechnung " + view.Draft.Number + " Kd. Lindenblatt"
		x.booking(issued.AddDate(0, 0, sup.term-2), decimal.RequireFromString(view.Totals.Due).Neg(), sup.company.Name, sup.company.IBAN, purpose)
		x.importStatement()
		_, err = x.s.RecordPayment(store.Payment{InvoiceID: imported.Invoice.ID, Direction: "incoming", BookingID: x.bookingID(purpose), Source: "bank", Reason: "Bankumsatz"})
		x.must(err)
	}
}
