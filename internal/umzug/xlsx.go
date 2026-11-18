package umzug

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"math"
	"path"
	"strconv"
	"strings"
)

// isXLSX tells whether the bytes are an Excel workbook (Office Open XML).
func isXLSX(data []byte) bool {
	if !bytes.HasPrefix(data, []byte("PK\x03\x04")) {
		return false
	}
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return false
	}
	for _, f := range r.File {
		if f.Name == "xl/workbook.xml" {
			return true
		}
	}
	return false
}

func zipFile(r *zip.Reader, name string) ([]byte, error) {
	for _, f := range r.File {
		if f.Name == name {
			rc, err := f.Open()
			if err != nil {
				return nil, err
			}
			defer rc.Close()
			return io.ReadAll(io.LimitReader(rc, MaxTableSize*4))
		}
	}
	return nil, errors.New("missing " + name)
}

// ReadXLSX reads the first sheet of an Excel workbook as a table: the first
// non-empty row names the columns. Numbers are written with a decimal comma,
// as German lists do, so amounts are read the same way as from a CSV file.
func ReadXLSX(data []byte) (Table, error) {
	t := Table{Encoding: "Excel", Delimiter: "Tabelle"}
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return t, errors.New("Die Excel-Datei ist beschädigt.")
	}
	sheet, err := firstSheet(r)
	if err != nil {
		return t, errors.New("Die Excel-Datei enthält keine Tabelle.")
	}
	shared, _ := sharedStrings(r)
	raw, err := zipFile(r, sheet)
	if err != nil {
		return t, errors.New("Die Excel-Datei enthält keine Tabelle.")
	}
	rows, err := sheetRows(raw, shared)
	if err != nil {
		return t, errors.New("Die Excel-Tabelle ließ sich nicht lesen.")
	}
	var records [][]string
	for _, row := range rows {
		if !blank(row) {
			records = append(records, row)
		}
	}
	if len(records) == 0 {
		return t, errors.New("Die Liste enthält keine Zeilen.")
	}
	width := 0
	for _, rec := range records {
		width = max(width, len(rec))
	}
	t.Columns = make([]string, width)
	copy(t.Columns, records[0])
	for _, rec := range records[1:] {
		row := make([]string, width)
		copy(row, rec)
		t.Rows = append(t.Rows, row)
	}
	if len(t.Rows) == 0 {
		return t, errors.New("Die Liste enthält nur Spaltennamen.")
	}
	return t, nil
}

func firstSheet(r *zip.Reader) (string, error) {
	wb, err := zipFile(r, "xl/workbook.xml")
	if err != nil {
		return "", err
	}
	var book struct {
		Sheets []struct {
			ID string `xml:"http://schemas.openxmlformats.org/officeDocument/2006/relationships id,attr"`
		} `xml:"sheets>sheet"`
	}
	if err = xml.Unmarshal(wb, &book); err != nil || len(book.Sheets) == 0 {
		return "", errors.New("no sheet")
	}
	rels, err := zipFile(r, "xl/_rels/workbook.xml.rels")
	if err != nil {
		return "xl/worksheets/sheet1.xml", nil
	}
	var list struct {
		Rels []struct {
			ID     string `xml:"Id,attr"`
			Target string `xml:"Target,attr"`
		} `xml:"Relationship"`
	}
	if err = xml.Unmarshal(rels, &list); err != nil {
		return "xl/worksheets/sheet1.xml", nil
	}
	for _, rel := range list.Rels {
		if rel.ID == book.Sheets[0].ID {
			target := strings.TrimPrefix(rel.Target, "/")
			if !strings.HasPrefix(target, "xl/") {
				target = path.Join("xl", target)
			}
			return target, nil
		}
	}
	return "xl/worksheets/sheet1.xml", nil
}

func sharedStrings(r *zip.Reader) ([]string, error) {
	raw, err := zipFile(r, "xl/sharedStrings.xml")
	if err != nil {
		return nil, err
	}
	var sst struct {
		Items []struct {
			T  string `xml:"t"`
			Rs []struct {
				T string `xml:"t"`
			} `xml:"r"`
		} `xml:"si"`
	}
	if err = xml.Unmarshal(raw, &sst); err != nil {
		return nil, err
	}
	out := make([]string, len(sst.Items))
	for i, si := range sst.Items {
		if len(si.Rs) == 0 {
			out[i] = si.T
			continue
		}
		var b strings.Builder
		for _, run := range si.Rs {
			b.WriteString(run.T)
		}
		out[i] = b.String()
	}
	return out, nil
}

// column turns "AB12" into the zero-based column 27.
func column(ref string) int {
	n := 0
	for _, c := range ref {
		if c < 'A' || c > 'Z' {
			break
		}
		n = n*26 + int(c-'A'+1)
	}
	return n - 1
}

func sheetRows(raw []byte, shared []string) ([][]string, error) {
	var ws struct {
		Rows []struct {
			Cells []struct {
				Ref    string `xml:"r,attr"`
				Type   string `xml:"t,attr"`
				Value  string `xml:"v"`
				Inline struct {
					T string `xml:"t"`
				} `xml:"is"`
			} `xml:"c"`
		} `xml:"sheetData>row"`
	}
	if err := xml.Unmarshal(raw, &ws); err != nil {
		return nil, err
	}
	out := make([][]string, 0, len(ws.Rows))
	for _, row := range ws.Rows {
		var cells []string
		for i, c := range row.Cells {
			at := i
			if c.Ref != "" {
				at = column(c.Ref)
			}
			if at < 0 || at > 1000 {
				continue
			}
			for len(cells) <= at {
				cells = append(cells, "")
			}
			value := c.Value
			switch c.Type {
			case "s":
				if n, err := strconv.Atoi(c.Value); err == nil && n >= 0 && n < len(shared) {
					value = shared[n]
				}
			case "inlineStr":
				value = c.Inline.T
			case "b":
				value = map[string]string{"1": "ja", "0": "nein"}[c.Value]
			case "str", "e":
			default:
				value = germanNumber(c.Value)
			}
			cells[at] = strings.TrimSpace(value)
		}
		out = append(out, cells)
	}
	return out, nil
}

// germanNumber writes a stored number with a decimal comma and without the
// noise of binary fractions (24.899999999999999 becomes 24,9).
func germanNumber(v string) string {
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return v
	}
	f = math.Round(f*1e9) / 1e9
	return strings.Replace(strconv.FormatFloat(f, 'f', -1, 64), ".", ",", 1)
}
