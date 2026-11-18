package umzug

import (
	"bytes"
	"encoding/xml"
	"io"
	"strings"

	"golang.org/x/text/encoding/charmap"
)

// A DATEV document package ("DATEV XML-Schnittstelle online", Belegbilder-
// und Rechnungsdatenservice) holds one document.xml that lists every receipt
// with its files and, for invoices, a data file with the invoice data
// ("LedgerImport"). Programs export it for the tax adviser as a ZIP.

// packageDocument is one receipt of document.xml.
type packageDocument struct {
	GUID     string
	Files    []string
	DataFile string
	// Direction from the invoice type property, if the package states it.
	Direction string
}

func charsetReader(label string, input io.Reader) (io.Reader, error) {
	switch strings.ToLower(label) {
	case "iso-8859-1", "latin1", "iso8859-1":
		return charmap.ISO8859_1.NewDecoder().Reader(input), nil
	case "windows-1252", "cp1252":
		return charmap.Windows1252.NewDecoder().Reader(input), nil
	case "iso-8859-15":
		return charmap.ISO8859_15.NewDecoder().Reader(input), nil
	}
	return input, nil
}

func xmlDecoder(data []byte) *xml.Decoder {
	d := xml.NewDecoder(bytes.NewReader(data))
	d.CharsetReader = charsetReader
	d.Strict = false
	return d
}

func attr(se xml.StartElement, local string) string {
	for _, a := range se.Attr {
		if a.Name.Local == local {
			return a.Value
		}
	}
	return ""
}

// readDocumentXML lists the receipts of a document.xml. It reads by element
// names only, so the versions of the format differ only in what they add.
func readDocumentXML(data []byte) []packageDocument {
	d := xmlDecoder(data)
	var out []packageDocument
	var current *packageDocument
	for {
		tok, err := d.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "document":
				out = append(out, packageDocument{GUID: strings.ToLower(attr(t, "guid"))})
				current = &out[len(out)-1]
			case "extension":
				if current == nil {
					continue
				}
				kind := strings.ToLower(attr(t, "type"))
				switch {
				case strings.HasSuffix(kind, "file") && attr(t, "name") != "":
					current.Files = append(current.Files, attr(t, "name"))
				case attr(t, "datafile") != "":
					current.DataFile = attr(t, "datafile")
				}
			case "property":
				if current != nil && strings.EqualFold(attr(t, "key"), "InvoiceType") {
					switch strings.ToLower(attr(t, "value")) {
					case "outgoing":
						current.Direction = "outgoing"
					case "incoming":
						current.Direction = "incoming"
					}
				}
			}
		case xml.EndElement:
			if t.Name.Local == "document" {
				current = nil
			}
		}
	}
	return out
}

// readLedger takes the invoice data of a data file: the first receivable
// (outgoing) or payable (incoming) entry.
func readLedger(data []byte) (Booking, bool) {
	d := xmlDecoder(data)
	var b Booking
	inside, found := false, false
	var field string
	for {
		tok, err := d.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "accountsReceivableLedger", "accountsPayableLedger":
				if found {
					return b, true
				}
				inside, found = true, true
				b.Direction = map[bool]string{true: "outgoing", false: "incoming"}[t.Name.Local == "accountsReceivableLedger"]
			default:
				if inside {
					field = t.Name.Local
				}
			}
		case xml.CharData:
			if !inside || field == "" {
				continue
			}
			value := strings.TrimSpace(string(t))
			switch field {
			case "invoiceId":
				b.Number = value
			case "date":
				if len(value) >= 10 {
					b.Date = value[:10]
				}
			case "amount":
				if v, ok := Amount(value); ok {
					b.Gross = v.Abs().StringFixed(2)
				}
			case "customerName", "supplierName":
				b.Text = value
			case "bookingText":
				if b.Text == "" {
					b.Text = value
				}
			}
		case xml.EndElement:
			switch t.Name.Local {
			case "accountsReceivableLedger", "accountsPayableLedger":
				return b, true
			}
			field = ""
		}
	}
	return b, found
}
