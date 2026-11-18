package umzug

import (
	"bytes"
	"encoding/csv"
	"errors"
	"io"
	"path"
	"regexp"
	"strconv"
	"strings"
)

// DATEV format (developer.datev.de, "DATEV-Format"): line 1 is a header with
// the kind of file, line 2 the column names, the data follow. Text is quoted,
// fields are separated by semicolons, the default code page is Windows-1252.

// Categories of DATEV files the move reads.
const (
	datevPartners = 16 // Debitoren/Kreditoren
	datevBookings = 21 // Buchungsstapel
)

// DatevFile is one EXTF or DTVF file.
type DatevFile struct {
	Category int
	Name     string
	// From and To are the period as YYYYMMDD.
	From, To string
	// AccountLength is the length of general ledger accounts; personal
	// accounts have one more digit.
	AccountLength int
	Table         Table
}

// isDatev tells by the first bytes whether a file is a DATEV-Format file.
func isDatev(data []byte) bool {
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})
	head := strings.TrimLeft(string(data[:min(len(data), 8)]), "\"")
	return strings.HasPrefix(head, "EXTF") || strings.HasPrefix(head, "DTVF")
}

// ReadDatev reads a DATEV-Format file.
func ReadDatev(data []byte) (DatevFile, error) {
	var f DatevFile
	text, encoding := decodeText(data)
	r := csv.NewReader(strings.NewReader(text))
	r.Comma, r.FieldsPerRecord, r.LazyQuotes = ';', -1, true
	var records [][]string
	for {
		rec, err := r.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return f, errors.New("Die DATEV-Datei ließ sich nicht lesen.")
		}
		if blank(rec) {
			continue
		}
		records = append(records, rec)
	}
	if len(records) < 2 || len(records[0]) < 5 {
		return f, errors.New("Das ist keine vollständige DATEV-Datei.")
	}
	h := records[0]
	field := func(i int) string {
		if i < len(h) {
			return strings.TrimSpace(h[i])
		}
		return ""
	}
	if kind := strings.Trim(field(0), "\uFEFF\""); kind != "EXTF" && kind != "DTVF" {
		return f, errors.New("Das ist keine DATEV-Datei.")
	}
	f.Category, _ = strconv.Atoi(field(2))
	f.Name = field(3)
	f.AccountLength, _ = strconv.Atoi(field(13))
	f.From, f.To = field(14), field(15)
	f.Table = Table{Encoding: encoding, Delimiter: "Semikolon", AccountLength: f.AccountLength}
	for _, c := range records[1] {
		f.Table.Columns = append(f.Table.Columns, strings.TrimSpace(c))
	}
	for _, rec := range records[2:] {
		row := make([]string, len(f.Table.Columns))
		for i := range row {
			if i < len(rec) {
				row[i] = strings.TrimSpace(rec[i])
			}
		}
		f.Table.Rows = append(f.Table.Rows, row)
	}
	return f, nil
}

// Label names the file for the archive, e.g. "DATEV-Buchungsstapel 01.01.2025 – 31.12.2025".
func (f DatevFile) Label() string {
	name := map[int]string{datevBookings: "Buchungsstapel", datevPartners: "Debitoren/Kreditoren", 20: "Kontenbeschriftungen", 46: "Zahlungsbedingungen", 48: "Diverse Adressen", 65: "Wiederkehrende Buchungen"}[f.Category]
	if name == "" {
		name = f.Name
	}
	label := "DATEV-" + name
	if from, to := isoDay(f.From), isoDay(f.To); from != "" && to != "" {
		label += " " + germanDay(from) + " – " + germanDay(to)
	}
	return label
}

func isoDay(yyyymmdd string) string {
	if len(yyyymmdd) < 8 {
		return ""
	}
	if _, err := strconv.Atoi(yyyymmdd[:8]); err != nil {
		return ""
	}
	return yyyymmdd[:4] + "-" + yyyymmdd[4:6] + "-" + yyyymmdd[6:8]
}

func germanDay(iso string) string { return iso[8:10] + "." + iso[5:7] + "." + iso[:4] }

// debitor tells whether a personal account is a customer (Debitor). With
// general ledger accounts of length L, customers use L+1 digits starting with
// 1 to 6, suppliers 7 to 9.
func debitor(account string, length int) (bool, bool) {
	account = strings.TrimSpace(account)
	if length < 4 || len(account) != length+1 {
		return false, false
	}
	switch account[0] {
	case '1', '2', '3', '4', '5', '6':
		return true, true
	case '7', '8', '9':
		return false, true
	}
	return false, false
}

// Booking is what a Buchungsstapel says about a receipt.
type Booking struct {
	Number    string
	Date      string
	Gross     string
	Text      string
	Direction string
	Account   string
	// Source names where the data come from for the archive: "" for a
	// DATEV export, "api" for the API of a program.
	Source string
}

var guidPattern = regexp.MustCompile(`[0-9A-Fa-f]{8}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{12}`)

// Bookings maps the GUIDs of "Beleglink" to the booking that refers to them.
func (f DatevFile) Bookings() map[string]Booking {
	out := map[string]Booking{}
	if f.Category != datevBookings {
		return out
	}
	col := map[string]int{}
	for i, c := range f.Table.Columns {
		col[headerKey(c)] = i
	}
	get := func(row []string, key string) string {
		if at, ok := col[key]; ok && at < len(row) {
			return row[at]
		}
		return ""
	}
	for _, row := range f.Table.Rows {
		guid := strings.ToLower(guidPattern.FindString(get(row, "beleglink")))
		if guid == "" {
			continue
		}
		b := Booking{Number: get(row, "belegfeld1"), Text: get(row, "buchungstext"), Date: f.bookingDate(get(row, "belegdatum"))}
		if v, ok := Amount(get(row, "umsatzohnesollhabenkz")); ok && !v.IsZero() {
			b.Gross = v.Abs().StringFixed(2)
		}
		for _, account := range []string{get(row, "konto"), get(row, "gegenkontoohnebuschluessel")} {
			if isDebitor, ok := debitor(account, f.AccountLength); ok {
				b.Account = account
				b.Direction = map[bool]string{true: "outgoing", false: "incoming"}[isDebitor]
				break
			}
		}
		if _, seen := out[guid]; !seen {
			out[guid] = b
		}
	}
	return out
}

// bookingDate turns the four digits TTMM of "Belegdatum" into a date in the
// period of the file; DATEV leaves the year out.
func (f DatevFile) bookingDate(ddmm string) string {
	ddmm = strings.TrimSpace(ddmm)
	if len(ddmm) == 3 {
		ddmm = "0" + ddmm
	}
	if len(ddmm) == 8 { // TTMMJJJJ
		return ddmm[4:] + "-" + ddmm[2:4] + "-" + ddmm[:2]
	}
	if len(ddmm) != 4 {
		return ""
	}
	from, to := isoDay(f.From), isoDay(f.To)
	for _, y := range []string{from, to} {
		if y == "" {
			continue
		}
		date := y[:4] + "-" + ddmm[2:] + "-" + ddmm[:2]
		if (from == "" || date >= from) && (to == "" || date <= to) {
			return date
		}
	}
	if from != "" {
		return from[:4] + "-" + ddmm[2:] + "-" + ddmm[:2]
	}
	return ""
}

// partnerNames maps personal accounts to their names in a Debitoren/Kreditoren file.
func (f DatevFile) partnerNames() map[string]string {
	out := map[string]string{}
	if f.Category != datevPartners {
		return out
	}
	m := GuessMapping(ListCustomers, f.Table.Columns)
	for _, p := range Customers(Table{Columns: f.Table.Columns, Rows: f.Table.Rows}, m, nil) {
		if account := cell(f.Table.Rows[p.Row-2], m, "account"); account != "" && p.Item.Name != "" {
			out[account] = p.Item.Name
		}
	}
	return out
}

// isDatevName is a cheap test before reading a CSV.
func isDatevName(name string) bool {
	base := strings.ToUpper(path.Base(name))
	return strings.HasPrefix(base, "EXTF_") || strings.HasPrefix(base, "DTVF_")
}
