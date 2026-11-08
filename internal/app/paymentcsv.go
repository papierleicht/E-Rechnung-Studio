package app

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
	"unicode"

	"github.com/shopspring/decimal"
	"papierleicht.de/e-rechnung-studio/internal/model"
)

// csvRow is one booking as a bank file states it, before it gets its identity.
type csvRow struct {
	date, currency, partner, iban, purpose, reference string
	amount                                            decimal.Decimal
}

// The column names banks use, compared without case, umlauts and punctuation
// (see csvHeaderKey). Each list is ordered by preference; exact names keep
// columns such as "Lastschrift Ursprungsbetrag" or "IBAN Auftragskonto" out.
var (
	csvDateNames      = []string{"buchungstag", "buchungsdatum", "buchung", "datum", "gebuchtam", "bookingdate", "date", "valutadatum", "valuta", "wertstellung", "wertstellungvaluta", "wert"}
	csvAmountNames    = []string{"betrag", "betrageur", "betragineur", "umsatz", "umsatzineur", "umsatzeur", "amount"}
	csvDebitNames     = []string{"soll", "solleur", "sollbetrag", "belastung", "ausgang"}
	csvCreditNames    = []string{"haben", "habeneur", "habenbetrag", "gutschrift", "eingang"}
	csvSignNames      = []string{"sollhaben", "sh"}
	csvCurrencyNames  = []string{"waehrung", "whrung", "wkz", "currency"}
	csvPartnerNames   = []string{"partner", "beguenstigterzahlungspflichtiger", "zahlungspflichtigerbeguenstigter", "auftraggeberempfaenger", "empfaengerauftraggeber", "auftraggeberbeguenstigter", "beguenstigterauftraggeber", "namezahlungsbeteiligter", "zahlungsbeteiligter", "name", "gegenkonto"}
	csvPayerNames     = []string{"zahlungspflichtiger", "zahlungspflichtige", "auftraggeber"}
	csvPayeeNames     = []string{"zahlungsempfaengerin", "zahlungsempfaenger", "empfaenger", "beguenstigter"}
	csvIBANNames      = []string{"ibanzahlungsbeteiligter", "kontonummeriban", "ibankontonummer", "iban", "gegeniban", "kontonummer"}
	csvPurposeNames   = []string{"verwendungszweck", "vorgangverwendungszweck", "umsatzinformation", "purpose", "beschreibung"}
	csvFallbackText   = []string{"buchungstext", "vorgang", "umsatzart"}
	csvReferenceNames = []string{"referenz", "kundenreferenz", "kundenreferenzendtoend", "endtoendreferenz", "reference"}
	csvStatusNames    = []string{"status"}
)

type csvColumns struct {
	date, amount, debit, credit, sign, currency, partner, payer, payee, iban, purpose, fallback, reference, status int
}

func csvHeaderKey(s string) string {
	s = strings.NewReplacer("ä", "ae", "ö", "oe", "ü", "ue", "ß", "ss", "€", "").Replace(strings.ToLower(s))
	return strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return r
		}
		return -1
	}, s)
}

func csvFind(index map[string][]int, names []string) int {
	for _, name := range names {
		if at := index[name]; len(at) > 0 {
			return at[0]
		}
	}
	return -1
}

// csvHeader recognizes the header line, or reports that this line is none.
func csvHeader(record []string) (csvColumns, bool) {
	index := map[string][]int{}
	for i, cell := range record {
		key := csvHeaderKey(cell)
		index[key] = append(index[key], i)
	}
	c := csvColumns{date: csvFind(index, csvDateNames), amount: csvFind(index, csvAmountNames), debit: csvFind(index, csvDebitNames), credit: csvFind(index, csvCreditNames),
		sign: csvFind(index, csvSignNames), currency: csvFind(index, csvCurrencyNames), partner: csvFind(index, csvPartnerNames), payer: csvFind(index, csvPayerNames),
		payee: csvFind(index, csvPayeeNames), iban: csvFind(index, csvIBANNames), purpose: csvFind(index, csvPurposeNames), fallback: csvFind(index, csvFallbackText),
		reference: csvFind(index, csvReferenceNames), status: csvFind(index, csvStatusNames)}
	// Some banks name two currency columns: one for the balance, one for the amount.
	for _, name := range csvCurrencyNames {
		for _, at := range index[name] {
			if c.amount >= 0 && at > c.amount {
				c.currency = at
			}
		}
	}
	return c, c.date >= 0 && (c.amount >= 0 || c.debit >= 0 || c.credit >= 0)
}

func csvCell(record []string, at int) string {
	if at < 0 || at >= len(record) {
		return ""
	}
	return strings.TrimSpace(strings.Trim(strings.TrimSpace(record[at]), " "))
}

func csvDate(s string) (string, bool) {
	for _, layout := range []string{time.DateOnly, "02.01.2006", "02.01.06", "2.1.2006", "2.1.06"} {
		if d, err := time.Parse(layout, s); err == nil {
			return d.Format(time.DateOnly), true
		}
	}
	return "", false
}

// amount reads the booking's signed amount from one column, a sign column, or
// separate debit and credit columns.
func (c csvColumns) rowAmount(record []string) (decimal.Decimal, bool) {
	if text := csvCell(record, c.amount); text != "" {
		v, err := model.ParseAmount(text)
		if err != nil {
			return v, false
		}
		if strings.EqualFold(csvCell(record, c.sign), "S") {
			v = v.Abs().Neg()
		}
		return v, true
	}
	if text := csvCell(record, c.credit); text != "" {
		v, err := model.ParseAmount(text)
		if err == nil && !v.IsZero() {
			return v.Abs(), true
		}
		if err != nil {
			return v, false
		}
	}
	if text := csvCell(record, c.debit); text != "" {
		v, err := model.ParseAmount(text)
		return v.Abs().Neg(), err == nil
	}
	return decimal.Zero, false
}

func parseCSVWith(text string, comma rune) ([]csvRow, bool, error) {
	r := csv.NewReader(strings.NewReader(text))
	r.Comma, r.FieldsPerRecord, r.LazyQuotes = comma, -1, true
	var columns csvColumns
	found := false
	out := []csvRow{}
	for records := 0; ; records++ {
		record, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, found, err
		}
		if !found {
			if columns, found = csvHeader(record); !found && records > 40 {
				return nil, false, nil
			}
			continue
		}
		line, _ := r.FieldPos(0)
		dateText := csvCell(record, columns.date)
		amount, amountOK := columns.rowAmount(record)
		date, dateOK := csvDate(dateText)
		status := strings.ToLower(csvCell(record, columns.status))
		switch {
		case !dateOK && !amountOK, strings.EqualFold(dateText, "offen"), status == "vorgemerkt", status == "pending":
			// Blank lines, closing balances and bookings that are not final yet.
			continue
		case !dateOK:
			return nil, true, fmt.Errorf("Die Zeile %d hat kein lesbares Datum („%s“).", line, dateText)
		case !amountOK:
			return nil, true, fmt.Errorf("Die Zeile %d hat keinen lesbaren Betrag.", line)
		case amount.IsZero():
			continue
		}
		row := csvRow{date: date, amount: amount, currency: strings.ToUpper(csvCell(record, columns.currency)), iban: csvCell(record, columns.iban), reference: csvCell(record, columns.reference)}
		if row.currency == "" {
			row.currency = "EUR"
		}
		row.partner = csvCell(record, columns.partner)
		if row.partner == "" {
			side := columns.payee
			if amount.IsPositive() {
				side = columns.payer
			}
			row.partner = csvCell(record, side)
		}
		row.purpose = firstNonEmpty(csvCell(record, columns.purpose), csvCell(record, columns.fallback))
		out = append(out, row)
	}
	return out, found, nil
}

// parseBankCSV reads the statement export of a bank. The header line may follow
// a few lines about the account, and banks differ in separator, column names,
// date and decimal format. The file never decides silently: a line that has a
// date or an amount but cannot be read stops the import.
func parseBankCSV(text string) ([]csvRow, error) {
	text = strings.TrimPrefix(strings.ReplaceAll(text, "\r\n", "\n"), "\ufeff")
	for _, comma := range []rune{';', ',', '\t', '|'} {
		rows, found, err := parseCSVWith(text, comma)
		if !found {
			continue
		}
		if err != nil {
			return nil, err
		}
		if len(rows) == 0 {
			return nil, errors.New("Die Datei enthält keine gebuchten Umsätze.")
		}
		return rows, nil
	}
	return nil, errors.New("Die Datei enthält keine erkennbaren Spalten für Datum und Betrag. Lade die Umsätze im Online-Banking als CSV herunter und wähle diese Datei aus.")
}
