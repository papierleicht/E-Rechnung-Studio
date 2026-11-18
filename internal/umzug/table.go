package umzug

import (
	"bytes"
	"encoding/csv"
	"errors"
	"io"
	"strings"
	"unicode"
	"unicode/utf16"
	"unicode/utf8"

	"golang.org/x/text/encoding/charmap"
)

// MaxTableSize limits a customer or article list.
const MaxTableSize = 20 << 20

// Table is a list read from a CSV or text export.
type Table struct {
	// Encoding and Delimiter are what the app recognized, shown to the customer.
	Encoding  string     `json:"encoding"`
	Delimiter string     `json:"delimiter"`
	Columns   []string   `json:"columns"`
	Rows      [][]string `json:"rows"`
	// AccountLength is set for DATEV files: customers (Debitoren) are then
	// told from suppliers by their account number.
	AccountLength int `json:"-"`
}

// ReadList reads a list a customer chose: an Excel workbook (.xlsx), a
// DATEV-Format file with its own header line, or any other CSV or text export.
func ReadList(data []byte) (Table, error) {
	if isXLSX(data) {
		return ReadXLSX(data)
	}
	if isDatev(data) {
		f, err := ReadDatev(data)
		return f.Table, err
	}
	return ReadTable(data)
}

// ReadTable reads a CSV or text export as the programs write them: UTF-8 with
// or without BOM, UTF-16 (Excel "Unicode-Text") or Windows-1252 (ANSI), with
// semicolons, commas, tabs or pipes, and optionally Excel's "sep=" line.
func ReadTable(data []byte) (Table, error) {
	var t Table
	if len(data) == 0 {
		return t, errors.New("Die Datei ist leer.")
	}
	if len(data) > MaxTableSize {
		return t, errors.New("Die Liste ist größer als 20 MB.")
	}
	text, encoding := decodeText(data)
	t.Encoding = encoding
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	lines := strings.SplitN(text, "\n", 2)
	first := strings.TrimSpace(lines[0])
	delimiter := rune(0)
	if strings.HasPrefix(strings.ToLower(first), "sep=") && len([]rune(first)) == 5 {
		delimiter = []rune(first)[4]
		if len(lines) > 1 {
			text = lines[1]
		} else {
			text = ""
		}
	}
	if delimiter == 0 {
		delimiter = guessDelimiter(text)
	}
	t.Delimiter = map[rune]string{';': "Semikolon", ',': "Komma", '\t': "Tabulator", '|': "senkrechter Strich"}[delimiter]
	r := csv.NewReader(strings.NewReader(text))
	r.Comma, r.FieldsPerRecord, r.LazyQuotes, r.TrimLeadingSpace = delimiter, -1, true, true
	var records [][]string
	for {
		rec, err := r.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return t, errors.New("Die Liste ließ sich nicht als CSV lesen. Prüfe, ob sie aus dem Programm als CSV oder Text exportiert wurde.")
		}
		if blank(rec) {
			continue
		}
		records = append(records, rec)
	}
	if len(records) == 0 {
		return t, errors.New("Die Liste enthält keine Zeilen.")
	}
	t.Columns = make([]string, len(records[0]))
	for i, c := range records[0] {
		t.Columns[i] = strings.TrimSpace(c)
	}
	for _, rec := range records[1:] {
		row := make([]string, len(t.Columns))
		for i := range row {
			if i < len(rec) {
				row[i] = unguard(strings.TrimSpace(rec[i]))
			}
		}
		t.Rows = append(t.Rows, row)
	}
	if len(t.Rows) == 0 {
		return t, errors.New("Die Liste enthält nur Spaltennamen.")
	}
	return t, nil
}

// unguard removes the apostrophe that programs put before a cell starting
// with =, +, - or @, so a spreadsheet does not run it as a formula
// (Haben does this, for example "'-5 % Rabatt" or "'+49 30 …").
func unguard(cell string) string {
	if len(cell) > 1 && cell[0] == '\'' && strings.ContainsRune("=+-@", rune(cell[1])) {
		return cell[1:]
	}
	return cell
}

func blank(rec []string) bool {
	for _, c := range rec {
		if strings.TrimSpace(c) != "" {
			return false
		}
	}
	return true
}

func decodeText(data []byte) (string, string) {
	switch {
	case bytes.HasPrefix(data, []byte{0xEF, 0xBB, 0xBF}):
		return string(data[3:]), "UTF-8"
	case bytes.HasPrefix(data, []byte{0xFF, 0xFE}):
		return decodeUTF16(data[2:], false), "UTF-16"
	case bytes.HasPrefix(data, []byte{0xFE, 0xFF}):
		return decodeUTF16(data[2:], true), "UTF-16"
	case utf8.Valid(data):
		return string(data), "UTF-8"
	}
	text, err := charmap.Windows1252.NewDecoder().Bytes(data)
	if err != nil {
		return string(data), "unbekannt"
	}
	return string(text), "Windows-1252 (ANSI)"
}

func decodeUTF16(data []byte, bigEndian bool) string {
	units := make([]uint16, 0, len(data)/2)
	for i := 0; i+1 < len(data); i += 2 {
		if bigEndian {
			units = append(units, uint16(data[i])<<8|uint16(data[i+1]))
		} else {
			units = append(units, uint16(data[i+1])<<8|uint16(data[i]))
		}
	}
	return string(utf16.Decode(units))
}

// guessDelimiter counts candidates outside quotes in the first lines and takes
// the one that appears most and equally often per line.
func guessDelimiter(text string) rune {
	candidates := []rune{';', '\t', ',', '|'}
	lines := strings.Split(text, "\n")
	best, bestScore := ';', -1
	for _, c := range candidates {
		counts := []int{}
		for _, line := range lines {
			if strings.TrimSpace(line) == "" {
				continue
			}
			n, quoted := 0, false
			for _, r := range line {
				if r == '"' {
					quoted = !quoted
				} else if r == c && !quoted {
					n++
				}
			}
			counts = append(counts, n)
			if len(counts) == 10 {
				break
			}
		}
		if len(counts) == 0 || counts[0] == 0 {
			continue
		}
		score := counts[0] * 10
		for _, n := range counts[1:] {
			if n != counts[0] {
				score -= 5
			}
		}
		if score > bestScore {
			best, bestScore = c, score
		}
	}
	return best
}

// headerKey compares column names without case, umlauts and punctuation.
func headerKey(s string) string {
	s = strings.NewReplacer("ä", "ae", "ö", "oe", "ü", "ue", "ß", "ss", "Ä", "ae", "Ö", "oe", "Ü", "ue").Replace(strings.ToLower(s))
	return strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return r
		}
		return -1
	}, s)
}
