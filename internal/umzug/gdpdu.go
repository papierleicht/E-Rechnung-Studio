package umzug

import (
	"bytes"
	"encoding/csv"
	"encoding/xml"
	"errors"
	"io"
	"strconv"
	"strings"
	"time"

	"golang.org/x/text/encoding/charmap"
)

// The Beschreibungsstandard for data handed to the tax office (GDPdU, Anlage
// to the GoBD) describes its tables in an index.xml: file, name, code page,
// separators, the first line with data and the columns. Programs write it for
// the Betriebsprüfung ("Z3-Export", "Daten Betriebsprüfung", "GoBD-Export").

// gdpduTable is one table of an index.xml.
type gdpduTable struct {
	URL         string
	Name        string
	Description string
	Encoding    string
	From        int
	Delimiter   string
	Quote       string
	Fixed       bool
	Columns     []string
}

type gdpduIndex struct {
	Media  string
	Tables []gdpduTable
	// Until is the last day the tables cover, as YYYY-MM-DD, from their
	// Validity ranges.
	Until string
}

// validityDay reads the end of a Validity range in the formats the
// description standard allows.
func validityDay(value, format string) string {
	layouts := []string{"02.01.2006", "2006-01-02", "20060102", "02.01.06"}
	switch strings.ToUpper(strings.TrimSpace(format)) {
	case "YYYY-MM-DD":
		layouts = []string{"2006-01-02"}
	case "YYYYMMDD":
		layouts = []string{"20060102"}
	case "DD.MM.YYYY":
		layouts = []string{"02.01.2006"}
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, strings.TrimSpace(value)); err == nil {
			return t.Format("2006-01-02")
		}
	}
	return ""
}

// readIndex reads an index.xml by element names, so DTD versions and
// programs that leave optional parts out are read alike.
func readIndex(data []byte) (gdpduIndex, error) {
	var idx gdpduIndex
	d := xmlDecoder(data)
	var stack []string
	var table *gdpduTable
	var text strings.Builder
	var until, untilFormat string
	parent := func(n int) string {
		if len(stack) > n {
			return stack[len(stack)-1-n]
		}
		return ""
	}
	for {
		tok, err := d.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return idx, errors.New("Die index.xml ließ sich nicht lesen.")
		}
		switch t := tok.(type) {
		case xml.StartElement:
			name := t.Name.Local
			stack = append(stack, name)
			text.Reset()
			switch name {
			case "Table":
				idx.Tables = append(idx.Tables, gdpduTable{Encoding: "ANSI", From: 1, Delimiter: ";", Quote: `"`})
				table = &idx.Tables[len(idx.Tables)-1]
			case "ANSI", "Macintosh", "OEM", "UTF8", "UTF16", "UTF7", "UTF32":
				if table != nil && parent(1) == "Table" {
					table.Encoding = name
				}
			case "FixedLength":
				if table != nil {
					table.Fixed = true
				}
			}
		case xml.CharData:
			text.Write(t)
		case xml.EndElement:
			value := strings.TrimSpace(text.String())
			name := t.Name.Local
			switch {
			case table == nil && name == "Name" && parent(1) == "Media":
				idx.Media = value
			case table == nil:
			case name == "Table":
				table = nil
			case name == "URL" && parent(1) == "Table":
				table.URL = value
			case name == "Name" && parent(1) == "Table":
				table.Name = value
			case name == "Description" && parent(1) == "Table":
				table.Description = value
			case name == "To" && parent(1) == "Range" && parent(2) == "Validity":
				until = value
			case name == "Format" && parent(1) == "Validity":
				untilFormat = value
			case name == "Validity":
				if day := validityDay(until, untilFormat); day > idx.Until {
					idx.Until = day
				}
				until, untilFormat = "", ""
			case name == "From" && parent(1) == "Range" && parent(2) == "Table":
				if n, err := strconv.Atoi(value); err == nil && n > 0 {
					table.From = n
				}
			case name == "ColumnDelimiter":
				table.Delimiter = value
			case name == "TextEncapsulator":
				table.Quote = value
			case name == "Name" && (parent(1) == "VariableColumn" || parent(1) == "VariablePrimaryKey" || parent(1) == "FixedColumn" || parent(1) == "FixedPrimaryKey"):
				table.Columns = append(table.Columns, value)
			}
			stack = stack[:len(stack)-1]
			text.Reset()
		}
	}
	if len(idx.Tables) == 0 {
		return idx, errors.New("Die index.xml beschreibt keine Tabellen.")
	}
	return idx, nil
}

func decodeAs(data []byte, encoding string) string {
	switch encoding {
	case "UTF8":
		return string(bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF}))
	case "UTF16":
		if bytes.HasPrefix(data, []byte{0xFE, 0xFF}) {
			return decodeUTF16(data[2:], true)
		}
		return decodeUTF16(bytes.TrimPrefix(data, []byte{0xFF, 0xFE}), false)
	case "Macintosh":
		text, _ := charmap.Macintosh.NewDecoder().Bytes(data)
		return string(text)
	case "OEM":
		text, _ := charmap.CodePage850.NewDecoder().Bytes(data)
		return string(text)
	}
	text, _ := charmap.Windows1252.NewDecoder().Bytes(data)
	return string(text)
}

// readGdpduTable reads one described table. The columns come from the index;
// lines before "From" (often a heading) are skipped.
func readGdpduTable(t gdpduTable, data []byte) (Table, error) {
	out := Table{Encoding: map[string]string{"ANSI": "Windows-1252 (ANSI)", "UTF8": "UTF-8", "UTF16": "UTF-16", "Macintosh": "Mac Roman", "OEM": "DOS (OEM)"}[t.Encoding], Delimiter: delimiterName(t.Delimiter), Columns: t.Columns}
	if t.Fixed {
		return out, errors.New("Tabellen mit fester Spaltenbreite liest die App noch nicht.")
	}
	if len(data) > MaxTableSize {
		return out, errors.New("Die Tabelle ist größer als 20 MB.")
	}
	text := decodeAs(data, t.Encoding)
	lines := strings.SplitAfter(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	if t.From > 1 && t.From-1 < len(lines) {
		lines = lines[t.From-1:]
	} else if t.From-1 >= len(lines) {
		lines = nil
	}
	r := csv.NewReader(strings.NewReader(strings.Join(lines, "")))
	r.FieldsPerRecord, r.LazyQuotes = -1, true
	if d := []rune(t.Delimiter); len(d) == 1 {
		r.Comma = d[0]
	} else {
		r.Comma = ';'
	}
	for {
		rec, err := r.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return out, errors.New("Die Tabelle " + t.Name + " ließ sich nicht lesen.")
		}
		if blank(rec) {
			continue
		}
		row := make([]string, len(out.Columns))
		for i := range row {
			if i < len(rec) {
				row[i] = strings.TrimSpace(rec[i])
			}
		}
		out.Rows = append(out.Rows, row)
	}
	return out, nil
}

func delimiterName(d string) string {
	switch d {
	case ";":
		return "Semikolon"
	case ",":
		return "Komma"
	case "\t":
		return "Tabulator"
	case "|":
		return "senkrechter Strich"
	}
	return d
}

// customerTable tells whether a table looks like a list of customers: a name
// and at least two of address, e-mail or VAT ID.
func customerTable(columns []string) bool {
	m := GuessMapping(ListCustomers, columns)
	if _, ok := m["name"]; !ok {
		if _, ok = m["lastName"]; !ok {
			return false
		}
	}
	hits := 0
	for _, f := range []string{"street", "postcode", "city", "email", "vatId"} {
		if _, ok := m[f]; ok {
			hits++
		}
	}
	return hits >= 2
}
