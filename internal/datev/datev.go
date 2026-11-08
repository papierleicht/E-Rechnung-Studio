// Package datev writes the DATEV-Format "Buchungsstapel" (EXTF, header version
// 700, format category 21, format version 13) as published on
// developer.datev.de. It only formats and checks fields; which invoice is
// booked to which account is decided by the caller.
package datev

import (
	"bytes"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/shopspring/decimal"
)

const (
	HeaderVersion = 700
	Category      = 21
	FormatName    = "Buchungsstapel"
	FormatVersion = 13
	// MaxBookings is DATEV's limit per Buchungsstapel file.
	MaxBookings = 99999
)

// Header holds the values of the first line. Dates are calendar days.
type Header struct {
	Created          time.Time
	ConsultantNumber int // Beraternummer, 1001–9999999
	ClientNumber     int // Mandantennummer, 1–99999
	FiscalYearStart  time.Time
	AccountLength    int // Sachkontenlänge, 4–8
	From, To         time.Time
	Label            string // Bezeichnung, at most 30 characters
	Chart            string // "03" or "04"
}

// Booking is one row. Amount is positive; Side refers to Account.
type Booking struct {
	Amount         decimal.Decimal
	Side           string // "S" or "H"
	Account        string
	CounterAccount string
	TaxKey         string // BU-Schlüssel, empty for automatic accounts
	Date           time.Time
	Document       string // Belegfeld 1, already passed through DocumentField
	Text           string // Buchungstext, at most 60 characters
	EUVATID        string // EU-Land u. UStID (Bestimmung)
}

var (
	digits      = regexp.MustCompile(`^\d+$`)
	documentRe  = regexp.MustCompile(`^[A-Za-z0-9_$&%*+\-/]{0,36}$`)
	labelRe     = regexp.MustCompile(`^[A-Za-z0-9_./ ]{0,30}$`)
	taxKeyRe    = regexp.MustCompile(`^\d{0,4}$`)
	vatIDRe     = regexp.MustCompile(`^([A-Z]{2}[A-Z0-9]{2,13})?$`)
	maxAmount   = decimal.RequireFromString("9999999999.99")
	errNoAmount = errors.New("Der Betrag muss größer als 0,00 sein.")
)

// Write renders a complete file: header, heading row and bookings, separated
// by CR/LF and encoded in Windows-1252 as DATEV expects by default.
func Write(h Header, bookings []Booking) ([]byte, error) {
	if err := h.validate(); err != nil {
		return nil, err
	}
	if len(bookings) > MaxBookings {
		return nil, fmt.Errorf("Ein Buchungsstapel darf höchstens %d Buchungen enthalten.", MaxBookings)
	}
	var out bytes.Buffer
	out.WriteString(h.line())
	out.WriteString("\r\n")
	out.WriteString(strings.Join(Columns[:], ";"))
	out.WriteString("\r\n")
	for i, b := range bookings {
		line, err := b.line(h)
		if err != nil {
			return nil, fmt.Errorf("Buchung %d (%s): %w", i+1, b.Document, err)
		}
		out.WriteString(line)
		out.WriteString("\r\n")
	}
	return Encode1252(out.String()), nil
}

func (h Header) validate() error {
	switch {
	case h.ConsultantNumber < 1001 || h.ConsultantNumber > 9999999:
		return errors.New("Die Beraternummer muss zwischen 1001 und 9999999 liegen.")
	case h.ClientNumber < 1 || h.ClientNumber > 99999:
		return errors.New("Die Mandantennummer muss zwischen 1 und 99999 liegen.")
	case h.AccountLength < 4 || h.AccountLength > 8:
		return errors.New("Die Sachkontenlänge muss zwischen 4 und 8 liegen.")
	case h.Chart != "03" && h.Chart != "04":
		return errors.New("Wähle SKR03 oder SKR04.")
	case h.To.Before(h.From) || h.From.Before(h.FiscalYearStart) || !h.To.Before(h.FiscalYearStart.AddDate(1, 0, 0)):
		return errors.New("Der Zeitraum muss innerhalb eines Wirtschaftsjahres liegen.")
	case !labelRe.MatchString(h.Label):
		return errors.New("Die Stapelbezeichnung enthält unzulässige Zeichen.")
	}
	return nil
}

func ymd(t time.Time) string { return t.Format("20060102") }

func (h Header) line() string {
	f := []string{
		quote("EXTF"), strconv.Itoa(HeaderVersion), strconv.Itoa(Category), quote(FormatName),
		strconv.Itoa(FormatVersion), h.Created.Format("20060102150405") + fmt.Sprintf("%03d", h.Created.Nanosecond()/1e6),
		"",          // 7 Importiert
		quote("RE"), // 8 Herkunft
		quote(""),   // 9 Exportiert von
		quote(""),   // 10 Importiert von
		strconv.Itoa(h.ConsultantNumber), strconv.Itoa(h.ClientNumber),
		ymd(h.FiscalYearStart), strconv.Itoa(h.AccountLength), ymd(h.From), ymd(h.To),
		quote(h.Label),
		quote(""), // 18 Diktatkürzel
		"1",       // 19 Buchungstyp: Finanzbuchführung
		"0",       // 20 Rechnungslegungszweck: unabhängig
		"0",       // 21 Festschreibung: none, so the accountant can still correct
		quote("EUR"),
		"", quote(""), "", "", // 23–26 reserved / Derivatskennzeichen
		quote(h.Chart),
		"", "", quote(""), // 28 Branchenlösung, 29–30 reserved
		quote(""), // 31 Anwendungsinformation
	}
	return strings.Join(f, ";")
}

func (b Booking) line(h Header) (string, error) {
	if !b.Amount.IsPositive() {
		return "", errNoAmount
	}
	if b.Amount.GreaterThan(maxAmount) || !b.Amount.Equal(b.Amount.Round(2)) {
		return "", errors.New("Der Betrag ist zu groß oder hat mehr als zwei Nachkommastellen.")
	}
	if b.Side != "S" && b.Side != "H" {
		return "", errors.New("Soll/Haben-Kennzeichen fehlt.")
	}
	for _, account := range []string{b.Account, b.CounterAccount} {
		if !digits.MatchString(account) || len(account) > 9 || strings.Trim(account, "0") == "" {
			return "", fmt.Errorf("Konto %q ist ungültig.", account)
		}
	}
	if !taxKeyRe.MatchString(b.TaxKey) {
		return "", fmt.Errorf("BU-Schlüssel %q ist ungültig.", b.TaxKey)
	}
	if !documentRe.MatchString(b.Document) {
		return "", fmt.Errorf("Belegfeld 1 %q enthält unzulässige Zeichen.", b.Document)
	}
	if !vatIDRe.MatchString(b.EUVATID) {
		return "", fmt.Errorf("USt-IdNr. %q ist ungültig.", b.EUVATID)
	}
	if b.Date.Before(h.From) || b.Date.After(h.To) {
		return "", errors.New("Das Belegdatum liegt außerhalb des Stapelzeitraums.")
	}
	values := map[int]string{
		1:  FormatAmount(b.Amount),
		2:  b.Side,
		3:  "EUR",
		7:  b.Account,
		8:  b.CounterAccount,
		9:  b.TaxKey,
		10: b.Date.Format("0201"), // TTMM; DATEV takes the year from the header
		11: b.Document,
		14: Text(b.Text, 60),
		40: b.EUVATID,
	}
	f := make([]string, len(Columns))
	for i := range f {
		v := values[i+1]
		if textColumns[i+1] {
			v = quote(v)
		}
		f[i] = v
	}
	return strings.Join(f, ";"), nil
}

// FormatAmount writes a decimal comma without thousands separators.
func FormatAmount(d decimal.Decimal) string {
	return strings.Replace(d.StringFixed(2), ".", ",", 1)
}

func quote(s string) string { return `"` + strings.ReplaceAll(s, `"`, `""`) + `"` }

// Text prepares free text: control characters and line breaks become spaces,
// runs of spaces collapse, characters outside Windows-1252 become "?", and the
// result is cut to max characters.
func Text(s string, max int) string {
	var b strings.Builder
	space := false
	for _, r := range s {
		if unicode.IsControl(r) || unicode.IsSpace(r) {
			space = b.Len() > 0
			continue
		}
		if _, ok := cp1252(r); !ok {
			r = '?'
		}
		if space {
			b.WriteRune(' ')
			space = false
		}
		b.WriteRune(r)
	}
	runes := []rune(b.String())
	if len(runes) > max {
		runes = []rune(strings.TrimRight(string(runes[:max]), " "))
	}
	return string(runes)
}

var transliteration = strings.NewReplacer("ä", "ae", "ö", "oe", "ü", "ue", "Ä", "Ae", "Ö", "Oe", "Ü", "Ue", "ß", "ss")

// DocumentField turns an invoice number into a valid Belegfeld 1: DATEV only
// allows letters, digits and $ & % * + - / _ (no spaces, umlauts, dots or
// commas) and at most 36 characters. Other characters become "-". Changed
// reports whether the number differs from the original, so the caller can
// list it for review.
func DocumentField(number string) (field string, changed bool) {
	s := transliteration.Replace(strings.TrimSpace(number))
	var b strings.Builder
	for _, r := range s {
		if r < 128 && (unicode.IsLetter(r) || unicode.IsDigit(r) || strings.ContainsRune("$&%*+-/_", r)) {
			b.WriteRune(r)
		} else {
			b.WriteRune('-')
		}
	}
	field = b.String()
	if len(field) > 36 {
		field = field[:36]
	}
	return field, field != strings.TrimSpace(number)
}

// Encode1252 converts UTF-8 text to Windows-1252. Text passed through Text
// only contains representable characters; anything else becomes "?".
func Encode1252(s string) []byte {
	out := make([]byte, 0, len(s))
	for _, r := range s {
		c, ok := cp1252(r)
		if !ok {
			c = '?'
		}
		out = append(out, c)
	}
	return out
}

// Windows-1252 differs from ISO-8859-1 only in 0x80–0x9F.
var cp1252High = map[rune]byte{
	'€': 0x80, '‚': 0x82, 'ƒ': 0x83, '„': 0x84, '…': 0x85, '†': 0x86, '‡': 0x87, 'ˆ': 0x88,
	'‰': 0x89, 'Š': 0x8A, '‹': 0x8B, 'Œ': 0x8C, 'Ž': 0x8E, '‘': 0x91, '’': 0x92, '“': 0x93,
	'”': 0x94, '•': 0x95, '–': 0x96, '—': 0x97, '˜': 0x98, '™': 0x99, 'š': 0x9A, '›': 0x9B,
	'œ': 0x9C, 'ž': 0x9E, 'Ÿ': 0x9F,
}

func cp1252(r rune) (byte, bool) {
	switch {
	case r < 0x80 || (r >= 0xA0 && r <= 0xFF):
		return byte(r), true
	}
	c, ok := cp1252High[r]
	return c, ok
}

// Label names a batch "Rechnungen MM/JJJJ bis MM/JJJJ" (30 characters, the
// maximum DATEV allows for field 17).
func Label(from, to time.Time) string {
	if from.Year() == to.Year() && from.Month() == to.Month() {
		return "Rechnungen " + from.Format("01/2006")
	}
	return "Rechnungen " + from.Format("01/2006") + " bis " + to.Format("01/2006")
}

// FiscalYearStart returns the first day of the fiscal year containing day.
func FiscalYearStart(day time.Time, startMonth int) time.Time {
	start := time.Date(day.Year(), time.Month(startMonth), 1, 0, 0, 0, 0, time.UTC)
	if day.Before(start) {
		start = start.AddDate(-1, 0, 0)
	}
	return start
}
