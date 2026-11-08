package app

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"papierleicht.de/e-rechnung-studio/internal/datev"
	"papierleicht.de/e-rechnung-studio/internal/store"
)

var updateDatev = flag.Bool("update-datev", false, "rewrite DATEV golden files")

func datevSettings(chart string) store.DatevSettings {
	s := store.DefaultDatevSettings()
	s.Chart = chart
	s.Accounts, _ = datev.DefaultAccounts(chart, 4)
	s.ConsultantNumber, s.ClientNumber, s.UpdatedAt = "29098", "55003", "2026-04-01T00:00:00Z"
	s.IncludePayments = true
	return s
}

func tax(category, percent, basis, amount string) datevTax {
	return datevTax{Category: category, Percent: d(percent), Basis: d(basis), Amount: d(amount)}
}

func issuedDoc(id, number, date string, taxes ...datevTax) datevDoc {
	return datevDoc{Kind: "issued", ID: id, Number: number, Date: date, Partner: "Müller GmbH", PartnerKey: "customer:c1", Currency: "EUR", Country: "DE", Taxes: taxes}
}

// datevCases covers every supported tax case, a credit note, a received
// invoice with 19 % and 7 % input tax, a payment and the records that must
// appear in the checklist instead of the file.
func datevCases(chart string) datevInput {
	ic := issuedDoc("ic", "RE-5", "2026-02-10", tax("K", "0", "2000", "0"))
	ic.Country, ic.VATID, ic.Partner, ic.PartnerKey = "AT", "ATU 123.456.78", "Wien KG", "customer:c2"
	rcEU := issuedDoc("rceu", "RE-6", "2026-02-11", tax("AE", "0", "300", "0"))
	rcEU.Country, rcEU.Partner, rcEU.PartnerKey = "FR", "Paris SARL", "customer:c3"
	margin := issuedDoc("mg", "RE-7", "2026-02-12", datevTax{Category: "E", ReasonCode: "VATEX-EU-F", Percent: d("0"), Basis: d("500"), Amount: d("0")})
	small := issuedDoc("kl", "RE-8", "2026-02-13", datevTax{Category: "E", Reason: "Umsatzsteuerbefreit nach § 19 UStG", Percent: d("0"), Basis: d("80"), Amount: d("0")})
	credit := issuedDoc("gs", "GS-1", "2026-03-02", tax("S", "19", "100", "19"))
	credit.Credit = true
	foreign := issuedDoc("usd", "RE-9", "2026-03-03", tax("S", "19", "100", "19"))
	foreign.Currency = "USD"
	historic := issuedDoc("h16", "RE-10", "2026-03-04", tax("S", "16", "100", "16"))
	received := datevDoc{Kind: "received", ID: "er1", Number: "ER 2026.01", Date: "2026-01-20", Partner: "Bürobedarf Süd", PartnerKey: "vat:DE123456789", Currency: "EUR", Taxes: []datevTax{tax("S", "19", "100", "19"), tax("S", "7", "50", "3.50")}}
	unchecked := datevDoc{Kind: "received", ID: "er2", Number: "X1", Date: "2026-01-21", Partner: "Scan GmbH", Currency: "EUR", Problem: "Keine strukturierten Rechnungsdaten"}
	return datevInput{
		From: "2026-01-01", To: "2026-03-31", Created: time.Date(2026, 4, 2, 9, 30, 0, 0, time.UTC), Settings: datevSettings(chart),
		Issued: []datevDoc{
			issuedDoc("a", "RE-1", "2026-01-05", tax("S", "19", "1000", "190"), tax("S", "7", "100", "7")),
			issuedDoc("z", "RE 2", "2026-01-06", tax("Z", "0", "5000", "0")),
			issuedDoc("ex", "RE-3", "2026-01-07", tax("E", "0", "400", "0")),
			issuedDoc("g", "RE-4", "2026-01-08", tax("G", "0", "700", "0")),
			ic, rcEU, margin, small, credit, foreign, historic,
			issuedDoc("old", "RE-0", "2025-12-31", tax("S", "19", "100", "19")),
		},
		Received: []datevDoc{received, unchecked},
		Payments: []store.Payment{
			pay("p1", "a", "outgoing", "1297.00", "2026-01-20", ""),
			pay("p2", "er1", "incoming", "172.50", "2026-01-25", ""),
		},
	}
}

func datevGolden(t *testing.T, name string, got []byte) {
	t.Helper()
	path := filepath.Join("testdata", "datev", name)
	if *updateDatev {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("%s differs from golden file; run go test -run Datev -update-datev and review the diff", name)
	}
}

func datevIssue(p DatevPreview, code string) *DatevIssue {
	for i := range p.Issues {
		if p.Issues[i].Code == code {
			return &p.Issues[i]
		}
	}
	return nil
}

func TestDatevExportGoldenSKR03AndSKR04(t *testing.T) {
	for _, chart := range []string{"SKR03", "SKR04"} {
		t.Run(chart, func(t *testing.T) {
			b := buildDatevExport(datevCases(chart))
			if !b.Preview.Ready {
				t.Fatalf("not ready: %+v", b.Preview.Issues)
			}
			csv, err := writeDatev(b)
			if err != nil {
				t.Fatal(err)
			}
			datevGolden(t, strings.ToLower(chart)+".csv", csv)
		})
	}
}

func TestDatevBookingsAndChecklist(t *testing.T) {
	b := buildDatevExport(datevCases("SKR03"))
	p := b.Preview
	// 9 bookable invoices (RE-1 has two rate groups), 1 received with 2 groups, 2 payments.
	if p.Bookings != 14 || p.Invoices != 8 || p.CreditNotes != 1 || p.Received != 1 || p.Payments != 2 {
		t.Fatalf("counts %+v", p)
	}
	find := func(document, counter string) datev.Booking {
		for _, v := range b.Bookings {
			if v.Document == document && v.CounterAccount == counter {
				return v.Booking
			}
		}
		t.Fatalf("no booking %s/%s", document, counter)
		return datev.Booking{}
	}
	if v := find("RE-1", "8400"); v.Side != "S" || v.Account != "10000" || v.Amount.String() != "1190" || v.TaxKey != "" {
		t.Fatalf("19 %%: %+v", v)
	}
	if v := find("RE-1", "8300"); v.Amount.String() != "107" {
		t.Fatalf("7 %%: %+v", v)
	}
	if v := find("GS-1", "8400"); v.Side != "H" || !strings.HasPrefix(v.Text, "Gutschrift") {
		t.Fatalf("credit note: %+v", v)
	}
	if v := find("RE-5", "8125"); v.EUVATID != "ATU12345678" {
		t.Fatalf("intra-community: %+v", v)
	}
	find("RE-6", "8336")
	find("RE-7", "8193")
	find("RE-8", "8195")
	find("RE-2", "8290")
	find("RE-3", "8100")
	find("RE-4", "8120")
	if v := find("ER-2026-01", "4980"); v.Side != "H" || v.Account != "70000" {
		t.Fatalf("received: %+v", v)
	}
	keys := map[string]bool{}
	for _, v := range b.Bookings {
		if v.Document == "ER-2026-01" && v.CounterAccount == "4980" {
			keys[v.TaxKey] = true
		}
	}
	if !keys["9"] || !keys["8"] {
		t.Fatalf("input tax keys %v", keys)
	}
	if v := find("RE-1", "10000"); v.Account != "1200" || v.Side != "S" {
		t.Fatalf("incoming money: %+v", v)
	}
	if v := find("ER-2026-01", "70000"); v.Account != "1200" || v.Side != "H" {
		t.Fatalf("outgoing money: %+v", v)
	}
	for code, entries := range map[string]int{"foreign_currency": 1, "unsupported_tax": 1, "received_unchecked": 1, "document_number_changed": 2, "margin_scheme": 1, "missing_vat_id": 1} {
		issue := datevIssue(p, code)
		if issue == nil || len(issue.Entries) != entries {
			t.Errorf("%s: %+v", code, issue)
		}
	}
	// The invoice from the previous year is neither booked nor reported.
	for _, v := range b.Bookings {
		if v.DocID == "old" {
			t.Fatal("invoice outside the period exported")
		}
	}
}

func TestDatevBlocksWithoutSettingsAndAcrossFiscalYears(t *testing.T) {
	in := datevCases("SKR03")
	in.Settings = store.DefaultDatevSettings()
	p := buildDatevExport(in).Preview
	if p.Ready || datevIssue(p, "settings_unsaved") == nil || datevIssue(p, "settings_numbers") == nil {
		t.Fatalf("unsaved settings must block: %+v", p.Issues)
	}
	in = datevCases("SKR03")
	in.To = "2027-01-31"
	if p := buildDatevExport(in).Preview; p.Ready || datevIssue(p, "period") == nil {
		t.Fatal("period across fiscal years must block")
	}
	in = datevCases("SKR03")
	in.Settings.FiscalYearStartMonth = 7
	in.From, in.To = "2025-07-01", "2026-06-30"
	p = buildDatevExport(in).Preview
	if !p.Ready || p.FiscalYearStart != "2025-07-01" {
		t.Fatalf("fiscal year from July: %+v", p)
	}
}

func TestDatevReverseChargeOutsideEUIsNotTaxable(t *testing.T) {
	accounts, err := datev.DefaultAccounts("SKR03", 4)
	if err != nil {
		t.Fatal(err)
	}
	doc := issuedDoc("us", "RE-11", "2026-02-14", tax("AE", "0", "900", "0"))
	for country, want := range map[string]string{"US": "8338", "CH": "8338", "FR": "8336", "DE": "8337", "": "8337"} {
		doc.Country = country
		if _, account, ok := revenueCase(doc.Taxes[0], doc, accounts.Revenue); !ok || account != want {
			t.Errorf("%q: account %s, want %s", country, account, want)
		}
	}
}

func TestDatevMissingAccountExcludesInvoice(t *testing.T) {
	in := datevCases("SKR04")
	in.Settings.Accounts.Revenue.Margin = ""
	in.Settings.Accounts.Bank = ""
	b := buildDatevExport(in)
	issue := datevIssue(b.Preview, "missing_account")
	if issue == nil || len(issue.Entries) != 3 { // margin invoice and both payments
		t.Fatalf("missing account: %+v", issue)
	}
	for _, v := range b.Bookings {
		if v.DocID == "mg" || v.DocKind == "payment" {
			t.Fatalf("excluded record booked: %+v", v)
		}
	}
}

func TestDatevIndividualPersonAccountsStayAssigned(t *testing.T) {
	in := datevCases("SKR03")
	in.Settings.PersonAccounts = "individual"
	in.Persons = []store.DatevPersonAccount{{Kind: "debtor", Key: "customer:c2", Number: "10000"}}
	b := buildDatevExport(in)
	numbers := map[string]string{}
	for _, p := range b.NewPersons {
		numbers[p.Key] = p.Number
	}
	// c2 keeps 10000; c1 and c3 get the next free numbers in order of the bookings.
	if numbers["customer:c1"] != "10001" || numbers["customer:c3"] != "10002" || numbers["vat:DE123456789"] != "70000" || numbers["customer:c2"] != "" {
		t.Fatalf("new person accounts %v", numbers)
	}
	for _, v := range b.Bookings {
		if v.Document == "RE-5" && v.Account != "10000" {
			t.Fatalf("assigned account changed: %+v", v)
		}
	}
}

func TestDatevSmallBusinessBooksNoInputTax(t *testing.T) {
	in := datevCases("SKR03")
	in.SmallBusiness = true
	for _, v := range buildDatevExport(in).Bookings {
		if v.TaxKey != "" {
			t.Fatalf("small business booking with BU-Schlüssel: %+v", v)
		}
	}
}

func TestDatevBooksRecordedExpensesByCategory(t *testing.T) {
	in := datevInput{From: "2026-10-01", To: "2026-12-31", Created: time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC), Settings: datevSettings("SKR03"), TaxBasis: "soll"}
	in.Expenses = []store.Expense{
		{ID: "a", Number: "B-1", Supplier: "Bürobedarf Hansen", Date: "2026-10-12", Category: "arbeitsmittel", Gross: "59.90", TaxRate: "19", PaidDate: "2026-10-12", PaidWith: "cash"},
		{ID: "b", Number: "B-2", Supplier: "Restaurant", Date: "2026-10-13", Category: "bewirtung", Gross: "102.30", ReducedGross: "42.80", TaxRate: store.RateMixed},
		{ID: "c", Number: "B-3", Supplier: "Bank", Date: "2026-10-14", Category: "bank", Gross: "9.90", TaxRate: "none", PaidDate: "2026-10-14", PaidWith: "bank"},
		{ID: "d", Number: "B-4", Supplier: "Apple", Date: "2026-10-15", Category: "asset", Gross: "2379.00", TaxRate: "19", Years: 1},
		{ID: "e", Number: "B-5", Supplier: "Alt", Date: "2026-09-30", Category: "werbung", Gross: "10.00", TaxRate: "19"},
	}
	b := buildDatevExport(in)
	p := b.Preview
	if p.Expenses != 4 || p.Bookings != 6 || p.Payments != 1 {
		t.Fatalf("counts %+v", p)
	}
	key := func(counter, tax string) bool {
		for _, v := range b.Bookings {
			if v.CounterAccount == counter && v.TaxKey == tax && v.Side == "H" && v.Account == in.Settings.Accounts.Creditor {
				return true
			}
		}
		return false
	}
	if !key("4930", "9") || !key("4650", "9") || !key("4650", "8") || !key("4970", "") || !key("0490", "9") {
		t.Fatalf("bookings %+v", b.Bookings)
	}
	for _, code := range []string{"expense_asset", "expense_entertainment", "expense_without_file", "expense_not_bank"} {
		if datevIssue(p, code) == nil {
			t.Errorf("missing issue %s", code)
		}
	}
}

func TestDatevUsesTheCategoryOfAReceivedInvoice(t *testing.T) {
	in := datevCases("SKR04")
	in.ReceivedCategories = map[string]store.ReceivedCategory{}
	for _, doc := range in.Received {
		in.ReceivedCategories[doc.ID] = store.ReceivedCategory{Category: "edv"}
	}
	found := false
	for _, v := range buildDatevExport(in).Bookings {
		if v.DocKind == "received" {
			found = found || v.CounterAccount == "6495"
			if v.CounterAccount == "6850" {
				t.Fatal("category ignored")
			}
		}
	}
	if !found {
		t.Fatal("no booking on the category account")
	}
}
