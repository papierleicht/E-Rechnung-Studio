// Package gdpdu writes data for the tax office in the Beschreibungsstandard:
// CSV files plus an index.xml that describes them, so that the audit software
// reads them without any further step (GoBD Rz. 167, 176 and Anlage).
package gdpdu

import (
	"bytes"
	_ "embed"
	"encoding/xml"
	"fmt"
	"slices"
	"strings"
)

// DTDName is the file the index refers to. It must lie next to index.xml and
// must not be changed.
const DTDName = "gdpdu-01-09-2004.dtd"

//go:embed gdpdu-01-09-2004.dtd
var DTD []byte

// Kinds of columns.
const (
	Text    = "text"
	Number  = "number"
	Date    = "date"
	dateFmt = "DD.MM.YYYY"
)

// Column describes one field of a table.
type Column struct {
	Name        string
	Description string
	Kind        string
	// Accuracy is the number of decimal places of a Number.
	Accuracy int
}

// ForeignKey links a table to another one of the same package.
type ForeignKey struct {
	Column, Table, Target string
}

// Table is one CSV file. Key columns come first, as the standard requires.
type Table struct {
	File, Name, Description string
	From, To                string
	Keys                    int
	Columns                 []Column
	Foreign                 []ForeignKey
	Rows                    [][]string
}

// Clean makes a text safe for the file. The standard forbids the text delimiter
// inside a field, and a record must stay on one line.
func Clean(s string) string {
	return strings.NewReplacer(`"`, "'", "\r\n", " ", "\n", " ", "\r", " ", "\t", " ").Replace(s)
}

// Day writes an ISO date in the format of the index.
func Day(iso string) string {
	if len(iso) < 10 || iso[4] != '-' || iso[7] != '-' {
		return ""
	}
	return iso[8:10] + "." + iso[5:7] + "." + iso[:4]
}

// Amount writes a decimal with a comma and no grouping.
func Amount(s string) string { return strings.ReplaceAll(s, ".", ",") }

// padded writes a number with the declared decimal places, which the index
// promises for the whole column.
func padded(value string, accuracy int) string {
	if value == "" || accuracy == 0 {
		return value
	}
	whole, fraction, _ := strings.Cut(value, ",")
	if len(fraction) >= accuracy {
		return value
	}
	return whole + "," + fraction + strings.Repeat("0", accuracy-len(fraction))
}

// Filled gives the rows as the file writes them, so a view shows the same numbers.
func (t Table) Filled() [][]string {
	out := make([][]string, len(t.Rows))
	for i, row := range t.Rows {
		out[i] = slices.Clone(row)
		for j, c := range t.Columns {
			if j < len(row) && c.Kind == Number {
				out[i][j] = padded(row[j], c.Accuracy)
			}
		}
	}
	return out
}

// CSV writes the file: a header line, semicolons, CRLF, quoted text.
func (t Table) CSV() []byte {
	var b bytes.Buffer
	for i, c := range t.Columns {
		if i > 0 {
			b.WriteByte(';')
		}
		fmt.Fprintf(&b, `"%s"`, Clean(c.Name))
	}
	b.WriteString("\r\n")
	for _, row := range t.Rows {
		for i, c := range t.Columns {
			if i > 0 {
				b.WriteByte(';')
			}
			value := ""
			if i < len(row) {
				value = row[i]
			}
			switch c.Kind {
			case Text:
				fmt.Fprintf(&b, `"%s"`, Clean(value))
			case Number:
				b.WriteString(padded(value, c.Accuracy))
			default:
				b.WriteString(value)
			}
		}
		b.WriteString("\r\n")
	}
	return b.Bytes()
}

type xmlRange struct {
	From string `xml:"From"`
	To   string `xml:"To,omitempty"`
}

type xmlValidity struct {
	Range  xmlRange `xml:"Range"`
	Format string   `xml:"Format"`
}

type xmlNumeric struct {
	Accuracy int `xml:"Accuracy"`
}

type xmlDate struct {
	Format string `xml:"Format"`
}

type xmlField struct {
	XMLName      xml.Name
	Name         string      `xml:"Name"`
	Description  string      `xml:"Description,omitempty"`
	Numeric      *xmlNumeric `xml:"Numeric,omitempty"`
	AlphaNumeric *struct{}   `xml:"AlphaNumeric,omitempty"`
	Date         *xmlDate    `xml:"Date,omitempty"`
}

type xmlForeignKey struct {
	Name       string `xml:"Name"`
	References string `xml:"References"`
	Alias      *struct {
		From string `xml:"From"`
		To   string `xml:"To"`
	} `xml:"Alias,omitempty"`
}

type xmlVariable struct {
	ColumnDelimiter  string `xml:"ColumnDelimiter"`
	RecordDelimiter  string `xml:"RecordDelimiter"`
	TextEncapsulator string `xml:"TextEncapsulator"`
	Fields           []xmlField
	Foreign          []xmlForeignKey `xml:"ForeignKey"`
}

type xmlTable struct {
	URL         string       `xml:"URL"`
	Name        string       `xml:"Name"`
	Description string       `xml:"Description,omitempty"`
	Validity    *xmlValidity `xml:"Validity,omitempty"`
	UTF8        struct{}     `xml:"UTF8"`
	Decimal     string       `xml:"DecimalSymbol"`
	Grouping    string       `xml:"DigitGroupingSymbol"`
	Range       xmlRange     `xml:"Range"`
	Variable    xmlVariable  `xml:"VariableLength"`
}

type xmlSupplier struct {
	Name     string `xml:"Name"`
	Location string `xml:"Location"`
	Comment  string `xml:"Comment"`
}

type xmlMedia struct {
	Name   string     `xml:"Name"`
	Tables []xmlTable `xml:"Table"`
}

type xmlDataSet struct {
	XMLName  xml.Name    `xml:"DataSet"`
	Version  string      `xml:"Version"`
	Supplier xmlSupplier `xml:"DataSupplier"`
	Media    xmlMedia    `xml:"Media"`
}

func field(element string, c Column) xmlField {
	f := xmlField{XMLName: xml.Name{Local: element}, Name: c.Name, Description: c.Description}
	switch c.Kind {
	case Number:
		f.Numeric = &xmlNumeric{Accuracy: c.Accuracy}
	case Date:
		f.Date = &xmlDate{Format: dateFmt}
	default:
		f.AlphaNumeric = &struct{}{}
	}
	return f
}

// Supplier names the company that hands over the data.
type Supplier struct {
	Name, Location, Comment string
}

// Index writes index.xml for the tables.
func Index(supplier Supplier, media string, tables []Table) ([]byte, error) {
	set := xmlDataSet{Version: "1.0", Supplier: xmlSupplier(supplier), Media: xmlMedia{Name: media}}
	for _, t := range tables {
		out := xmlTable{URL: t.File, Name: t.Name, Description: t.Description, Decimal: ",", Grouping: ".", Range: xmlRange{From: "2"}}
		if t.From != "" && t.To != "" {
			out.Validity = &xmlValidity{Range: xmlRange{From: Day(t.From), To: Day(t.To)}, Format: dateFmt}
		}
		out.Variable = xmlVariable{ColumnDelimiter: ";", RecordDelimiter: "\r\n", TextEncapsulator: `"`}
		for i, c := range t.Columns {
			element := "VariableColumn"
			if i < t.Keys {
				element = "VariablePrimaryKey"
			}
			out.Variable.Fields = append(out.Variable.Fields, field(element, c))
		}
		for _, fk := range t.Foreign {
			key := xmlForeignKey{Name: fk.Column, References: fk.Table}
			if fk.Target != "" && fk.Target != fk.Column {
				key.Alias = &struct {
					From string `xml:"From"`
					To   string `xml:"To"`
				}{fk.Column, fk.Target}
			}
			out.Variable.Foreign = append(out.Variable.Foreign, key)
		}
		set.Media.Tables = append(set.Media.Tables, out)
	}
	var b bytes.Buffer
	b.WriteString(xml.Header)
	b.WriteString(`<!DOCTYPE DataSet SYSTEM "` + DTDName + `">` + "\n")
	enc := xml.NewEncoder(&b)
	enc.Indent("", "  ")
	if err := enc.Encode(set); err != nil {
		return nil, err
	}
	b.WriteByte('\n')
	return b.Bytes(), nil
}
