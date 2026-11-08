package model

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Invoice number series (Nummernkreise). § 14 Abs. 4 Nr. 4 UStG requires a
// consecutive number that identifies the invoice once. Abschn. 14.5 Abs. 10
// UStAE allows several separate series, e.g. per customer or period, as long
// as every number is unique and clearly belongs to one series.
//
// A series is a template with one counter token {N…} (the number of N is the
// minimum width) and optional date tokens {JJJJ}, {JJ} and {MM}.

// DefaultNumberSeries is the series of every installation that never chose one.
const DefaultNumberSeries = "RE-{JJJJ}-{NNNN}"

// MaxInvoiceNumberLength is the length DATEV accepts in Belegfeld 1.
const MaxInvoiceNumberLength = 36

// Draft.NumberSeries values besides an explicit template. The empty value
// follows the customer's own series, if any, and otherwise the standard one.
const (
	SeriesAuto     = ""
	SeriesStandard = "standard"
)

// SeriesParts is a template resolved for one date: number = Prefix + counter
// (zero-padded to Width) + Suffix.
type SeriesParts struct {
	Prefix string
	Suffix string
	Width  int
}

func (p SeriesParts) Format(n int64) string {
	return p.Prefix + fmt.Sprintf("%0*d", p.Width, n) + p.Suffix
}

// Counter returns the counter of a number in this series. Letters compare
// case-insensitively, like the uniqueness check.
func (p SeriesParts) Counter(number string) (int64, bool) {
	if len(number) <= len(p.Prefix)+len(p.Suffix) ||
		!strings.EqualFold(number[:len(p.Prefix)], p.Prefix) ||
		!strings.EqualFold(number[len(number)-len(p.Suffix):], p.Suffix) {
		return 0, false
	}
	digits := number[len(p.Prefix) : len(number)-len(p.Suffix)]
	if strings.Trim(digits, "0123456789") != "" {
		return 0, false
	}
	n, err := strconv.ParseInt(digits, 10, 64)
	return n, err == nil
}

type seriesToken struct {
	text    string
	counter int // width of a counter token
	date    string
}

func scanSeries(template string) ([]seriesToken, error) {
	var out []seriesToken
	for rest := template; rest != ""; {
		open := strings.IndexByte(rest, '{')
		if open < 0 {
			out = append(out, seriesToken{text: rest})
			break
		}
		if open > 0 {
			out = append(out, seriesToken{text: rest[:open]})
		}
		end := strings.IndexByte(rest[open:], '}')
		if end < 0 {
			return nil, errors.New("Eine geschweifte Klammer ist nicht geschlossen.")
		}
		name := strings.ToUpper(rest[open+1 : open+end])
		switch {
		case name == "JJJJ" || name == "JJ" || name == "MM":
			out = append(out, seriesToken{date: name})
		case name != "" && strings.Trim(name, "N") == "":
			out = append(out, seriesToken{counter: len(name)})
		default:
			return nil, fmt.Errorf("Den Platzhalter {%s} gibt es nicht. Erlaubt sind {JJJJ}, {JJ}, {MM} und {NNN}.", rest[open+1:open+end])
		}
		rest = rest[open+end+1:]
	}
	return out, nil
}

// Letters without umlauts, digits and -/_.:+ pass unchanged through payment
// references (SEPA), file names and, mostly, DATEV.
func allowedNumberRune(r rune) bool {
	return r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || strings.ContainsRune(numberSymbols, r)
}

const numberSymbols = "-/_.:+"

const allowedNumberText = "Verwende Buchstaben ohne Umlaute, Ziffern und „-“, „/“, „_“, „.“, „:“ oder „+“."

// ValidateSeries checks a template: exactly one counter, known date tokens,
// allowed characters, and the counter separated from other digits so that
// every number belongs to exactly one counter value.
func ValidateSeries(template string) error {
	if strings.TrimSpace(template) == "" {
		return errors.New("Gib ein Format für die Rechnungsnummer ein, z. B. RE-{JJJJ}-{NNNN}.")
	}
	tokens, err := scanSeries(template)
	if err != nil {
		return err
	}
	counters := 0
	for i, t := range tokens {
		for _, r := range t.text {
			if !allowedNumberRune(r) {
				return fmt.Errorf("„%c“ ist im Format nicht erlaubt. %s", r, allowedNumberText)
			}
		}
		if t.counter == 0 {
			continue
		}
		counters++
		if t.counter > 9 {
			return errors.New("Die laufende Nummer darf höchstens neun Stellen vorgeben.")
		}
		digitNear := func(j int, last bool) bool {
			if j < 0 || j >= len(tokens) {
				return false
			}
			if tokens[j].date != "" || tokens[j].counter > 0 {
				return true
			}
			s := tokens[j].text
			if last {
				return s[len(s)-1] >= '0' && s[len(s)-1] <= '9'
			}
			return s[0] >= '0' && s[0] <= '9'
		}
		if digitNear(i-1, true) || digitNear(i+1, false) {
			return errors.New("Trenne die laufende Nummer mit einem Zeichen wie „-“ oder „/“ von Jahr, Monat und anderen Ziffern.")
		}
	}
	if counters != 1 {
		return errors.New("Das Format braucht genau eine laufende Nummer, z. B. {NNNN}.")
	}
	example, _ := ResolveSeries(template, time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC))
	if len(example.Format(1)) > MaxInvoiceNumberLength {
		return fmt.Errorf("Rechnungsnummern aus diesem Format wären länger als %d Zeichen.", MaxInvoiceNumberLength)
	}
	return nil
}

// ResolveSeries fills the date tokens for the invoice date.
func ResolveSeries(template string, date time.Time) (SeriesParts, error) {
	tokens, err := scanSeries(template)
	if err != nil {
		return SeriesParts{}, err
	}
	var p SeriesParts
	counter := false
	for _, t := range tokens {
		text := t.text
		switch t.date {
		case "JJJJ":
			text = fmt.Sprintf("%04d", date.Year())
		case "JJ":
			text = fmt.Sprintf("%02d", date.Year()%100)
		case "MM":
			text = fmt.Sprintf("%02d", int(date.Month()))
		}
		switch {
		case t.counter > 0:
			counter, p.Width = true, t.counter
		case counter:
			p.Suffix += text
		default:
			p.Prefix += text
		}
	}
	if !counter {
		return p, errors.New("Das Format braucht genau eine laufende Nummer, z. B. {NNNN}.")
	}
	return p, nil
}

// SeriesPattern shows a template the way the UI explains it: RE-JJJJ-NNNN.
func SeriesPattern(template string) string {
	return strings.NewReplacer("{", "", "}", "").Replace(template)
}

// ValidateInvoiceNumber checks a number the user entered. Abschn. 14.5
// Abs. 10 UStAE allows digit and letter sequences; the character set keeps
// the number intact in payment references, file names and DATEV.
func ValidateInvoiceNumber(number string) error {
	if number == "" {
		return errors.New("Gib eine Rechnungsnummer ein.")
	}
	if len(number) > MaxInvoiceNumberLength {
		return fmt.Errorf("Die Rechnungsnummer darf höchstens %d Zeichen lang sein.", MaxInvoiceNumberLength)
	}
	for _, r := range number {
		if !allowedNumberRune(r) {
			if r == ' ' {
				return errors.New("Die Rechnungsnummer darf keine Leerzeichen enthalten.")
			}
			return fmt.Errorf("„%c“ ist in der Rechnungsnummer nicht erlaubt. %s", r, allowedNumberText)
		}
	}
	return nil
}

// SeriesFromNumber derives the series a number continues: its last digit run
// is the counter, a run equal to the invoice year becomes {JJJJ} or {JJ}.
// A year at the end ("7-2026") leaves the counter in front of it.
func SeriesFromNumber(number string, date time.Time) (string, bool) {
	if ValidateInvoiceNumber(number) != nil {
		return "", false
	}
	var runs []string
	for i := 0; i < len(number); {
		j := i
		digits := number[i] >= '0' && number[i] <= '9'
		for j < len(number) && (number[j] >= '0' && number[j] <= '9') == digits {
			j++
		}
		runs = append(runs, number[i:j])
		i = j
	}
	isDigits := func(s string) bool { return s[0] >= '0' && s[0] <= '9' }
	year, short := fmt.Sprintf("%04d", date.Year()), fmt.Sprintf("%02d", date.Year()%100)
	counter := -1
	for i := len(runs) - 1; i >= 0; i-- {
		if !isDigits(runs[i]) {
			continue
		}
		if counter < 0 {
			counter = i
			continue
		}
		// The last digits are the year and an earlier run counts: RE-0007-2026.
		if runs[counter] == year && runs[i] != year {
			counter = i
		}
		break
	}
	if counter < 0 {
		return "", false
	}
	yearDone := false
	for i, run := range runs {
		if i != counter && run == year {
			runs[i], yearDone = "{JJJJ}", true
		}
	}
	for i, run := range runs {
		// Two digits after a letter are rather a code (K26-007) than a year.
		separated := i == 0 || strings.ContainsRune(numberSymbols, rune(runs[i-1][len(runs[i-1])-1]))
		if !yearDone && i != counter && run == short && separated {
			runs[i] = "{JJ}"
		}
	}
	runs[counter] = "{" + strings.Repeat("N", len(runs[counter])) + "}"
	template := strings.Join(runs, "")
	if ValidateSeries(template) != nil {
		return "", false
	}
	return template, true
}
