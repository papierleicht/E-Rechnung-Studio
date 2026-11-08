package datev

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

var update = flag.Bool("update", false, "rewrite golden files")

func day(s string) time.Time {
	t, err := time.Parse(time.DateOnly, s)
	if err != nil {
		panic(err)
	}
	return t
}

func header() Header {
	return Header{
		Created:          time.Date(2026, 4, 2, 14, 4, 40, 439_000_000, time.UTC),
		ConsultantNumber: 29098, ClientNumber: 55003, FiscalYearStart: day("2026-01-01"), AccountLength: 4,
		From: day("2026-01-01"), To: day("2026-03-31"), Label: Label(day("2026-01-01"), day("2026-03-31")), Chart: "03",
	}
}

func golden(t *testing.T, name string, got []byte) {
	t.Helper()
	path := filepath.Join("testdata", name)
	if *update {
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("%s differs\n got: %q\nwant: %q", name, got, want)
	}
}

// The header matches DATEV's published sample field by field except for the
// values that differ by design (period, label, empty Diktatkürzel, no
// Festschreibung).
func TestHeaderGolden(t *testing.T) {
	h := header()
	got := h.line()
	want := `"EXTF";700;21;"Buchungsstapel";13;20260402140440439;;"RE";"";"";29098;55003;20260101;4;20260101;20260331;"Rechnungen 01/2026 bis 03/2026";"";1;0;0;"EUR";;"";;;"03";;;"";""`
	if got != want {
		t.Fatalf("header\n got %s\nwant %s", got, want)
	}
	if n := len(strings.Split(got, ";")); n != 31 {
		t.Fatalf("header has %d fields, DATEV defines 31", n)
	}
	if n := len(Columns); n != 125 {
		t.Fatalf("%d columns, format version 13 defines 125", n)
	}
}

func TestBookingsGolden(t *testing.T) {
	bookings := []Booking{
		// Invoice 19 % on an automatic revenue account, umlauts and € in the text.
		{Amount: decimal.RequireFromString("1190"), Side: "S", Account: "10000", CounterAccount: "8400", Date: day("2026-01-05"), Document: "RE-2026-001", Text: "RE-2026-001 Müller & Söhne GmbH – Büro €"},
		// Credit note: same accounts, Haben.
		{Amount: decimal.RequireFromString("119.5"), Side: "H", Account: "10000", CounterAccount: "8400", Date: day("2026-02-28"), Document: "GS-1", Text: `Gutschrift "Rabatt"`},
		// Received invoice with input tax 7 %.
		{Amount: decimal.RequireFromString("1234567.89"), Side: "H", Account: "70000", CounterAccount: "4980", TaxKey: "8", Date: day("2026-03-31"), Document: "ER/77", Text: strings.Repeat("x", 70)},
		// Intra-community supply with VAT ID.
		{Amount: decimal.RequireFromString("0.01"), Side: "S", Account: "10000", CounterAccount: "8125", Date: day("2026-03-01"), Document: "RE-2026-002", Text: "Kunde\nmit Umbruch", EUVATID: "ATU12345678"},
	}
	got, err := Write(header(), bookings)
	if err != nil {
		t.Fatal(err)
	}
	golden(t, "buchungsstapel.csv", got)
	lines := bytes.Split(bytes.TrimSuffix(got, []byte("\r\n")), []byte("\r\n"))
	if len(lines) != 6 {
		t.Fatalf("%d lines", len(lines))
	}
	for i, line := range lines[1:] {
		if n := bytes.Count(line, []byte(";")) + 1; n != 125 {
			t.Fatalf("line %d has %d fields", i+2, n)
		}
	}
	// Windows-1252: ü = 0xFC, € = 0x80, – = 0x96; no UTF-8 sequences.
	for _, b := range []byte{0xFC, 0x80, 0x96} {
		if !bytes.Contains(got, []byte{b}) {
			t.Fatalf("missing Windows-1252 byte %#x", b)
		}
	}
	if bytes.Contains(got, []byte("ü")) {
		t.Fatal("UTF-8 found in output")
	}
	row := strings.Split(string(lines[2]), ";")
	if row[0] != "1190,00" || row[1] != `"S"` || row[9] != "0501" || row[10] != `"RE-2026-001"` || row[8] != `""` {
		t.Fatalf("first booking %q", row[:11])
	}
	if text := strings.Split(string(lines[4]), ";")[13]; text != `"`+strings.Repeat("x", 60)+`"` {
		t.Fatalf("text not cut to 60 characters: %s", text)
	}
}

func TestWriteRejectsInvalidValues(t *testing.T) {
	ok := Booking{Amount: decimal.NewFromInt(1), Side: "S", Account: "10000", CounterAccount: "8400", Date: day("2026-01-05"), Document: "RE-1"}
	for name, change := range map[string]func(*Booking){
		"zero":        func(b *Booking) { b.Amount = decimal.Zero },
		"negative":    func(b *Booking) { b.Amount = decimal.NewFromInt(-1) },
		"fraction":    func(b *Booking) { b.Amount = decimal.RequireFromString("1.234") },
		"side":        func(b *Booking) { b.Side = "" },
		"account":     func(b *Booking) { b.CounterAccount = "84a0" },
		"document":    func(b *Booking) { b.Document = "RE 1" },
		"outside":     func(b *Booking) { b.Date = day("2026-04-01") },
		"vat":         func(b *Booking) { b.EUVATID = "at 123" },
		"zeroAccount": func(b *Booking) { b.Account = "0000" },
	} {
		b := ok
		change(&b)
		if _, err := Write(header(), []Booking{b}); err == nil {
			t.Errorf("%s: invalid booking accepted", name)
		}
	}
	h := header()
	h.To = day("2027-01-01")
	if _, err := Write(h, []Booking{ok}); err == nil {
		t.Error("batch across two fiscal years accepted")
	}
	h = header()
	h.ConsultantNumber = 999
	if _, err := Write(h, []Booking{ok}); err == nil {
		t.Error("consultant number below 1001 accepted")
	}
}

func TestDocumentField(t *testing.T) {
	for in, want := range map[string]string{
		"RE-2026-001":           "RE-2026-001",
		"RE 2026.001":           "RE-2026-001",
		"Rechnung Nr.5":         "Rechnung-Nr-5",
		"Größe/1":               "Groesse/1",
		strings.Repeat("A", 40): strings.Repeat("A", 36),
	} {
		got, changed := DocumentField(in)
		if got != want || changed != (in != want) {
			t.Errorf("DocumentField(%q) = %q, %v", in, got, changed)
		}
	}
}

func TestDefaultAccounts(t *testing.T) {
	skr03, _ := DefaultAccounts("SKR03", 4)
	skr04, _ := DefaultAccounts("SKR04", 4)
	if skr03.Revenue.Standard19 != "8400" || skr04.Revenue.Standard19 != "4400" || skr03.Expense != "4980" || skr04.Expense != "6850" || skr03.Debtor != "10000" || skr04.Creditor != "70000" {
		t.Fatalf("defaults %+v %+v", skr03, skr04)
	}
	long, _ := DefaultAccounts("SKR04", 6)
	if long.Revenue.Reduced7 != "430000" || long.Debtor != "1000000" {
		t.Fatalf("padded %+v", long)
	}
	if CheckAccount("x", long.Debtor, 6, true) != nil || CheckAccount("x", "10000", 6, true) == nil || CheckAccount("x", "84000", 4, false) == nil {
		t.Fatal("account length check")
	}
	if _, err := DefaultAccounts("SKR07", 4); err == nil {
		t.Fatal("unknown chart accepted")
	}
}

func TestFiscalYearStart(t *testing.T) {
	if got := FiscalYearStart(day("2026-03-15"), 7); !got.Equal(day("2025-07-01")) {
		t.Fatal(got)
	}
	if got := FiscalYearStart(day("2026-07-01"), 7); !got.Equal(day("2026-07-01")) {
		t.Fatal(got)
	}
	if Label(day("2026-05-01"), day("2026-05-31")) != "Rechnungen 05/2026" {
		t.Fatal("month label")
	}
}
