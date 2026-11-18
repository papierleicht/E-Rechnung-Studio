package umzug

import (
	"crypto/sha256"
	"encoding/hex"
	"path"
	"sort"
	"strings"
	"unicode"

	erechnung "github.com/papierleicht/go-e-rechnung-validator"
	"papierleicht.de/e-rechnung-studio/internal/einvoice"
	"papierleicht.de/e-rechnung-studio/internal/invoicexml"
	"papierleicht.de/e-rechnung-studio/internal/model"
)

// Own identifies the customer's company, so the app can tell own invoices
// from received ones.
type Own struct {
	Name      string
	VATID     string
	TaxNumber string
}

// Group is the files of one document: a PDF and an XML with the same name in
// the same folder belong together (XRechnung exports often deliver both).
type Group struct {
	Entries []Entry
}

// Groups puts the entries of a collection into documents.
func Groups(c *Collection) []Group {
	index := map[string]int{}
	var groups []Group
	for _, e := range c.Entries {
		kind := fileKind(e.Name)
		key := ""
		if kind == "pdf" || kind == "xml" {
			key = strings.ToLower(strings.TrimSuffix(e.Origin, path.Ext(e.Origin)))
		}
		if at, ok := index[key]; ok && key != "" && len(groups[at].Entries) == 1 && fileKind(groups[at].Entries[0].Name) != kind {
			groups[at].Entries = append(groups[at].Entries, e)
			continue
		}
		if key != "" {
			index[key] = len(groups)
		}
		groups = append(groups, Group{Entries: []Entry{e}})
	}
	for i := range groups {
		// The PDF is the main file: it is what people open.
		sort.SliceStable(groups[i].Entries, func(a, b int) bool {
			return fileKind(groups[i].Entries[a].Name) == "pdf" && fileKind(groups[i].Entries[b].Name) != "pdf"
		})
	}
	return groups
}

// Read turns a group into a legacy document with its file contents. The data
// shown in the archive come only from e-invoice XML; a plain PDF or a photo is
// archived as it is, without guessing number, date or amount.
func Read(g Group, source string, own Own) (model.LegacyDocument, error) {
	d := model.LegacyDocument{Source: source, Kind: "document", Origin: g.Entries[0].Origin}
	var xml []byte
	for _, e := range g.Entries {
		data, err := e.Open()
		if err != nil {
			return d, err
		}
		d.Files = append(d.Files, model.LegacyFile{Name: e.Name, MIME: mimeOf(e.Name), Data: data, Size: len(data)})
		if xml != nil {
			continue
		}
		switch fileKind(e.Name) {
		case "xml":
			xml = data
		case "pdf":
			if embedded, err := erechnung.ExtractXML(data); err == nil {
				xml = embedded
			}
		}
	}
	sum := sha256.Sum256(d.Files[0].Data)
	d.SourceID = hex.EncodeToString(sum[:])
	if xml == nil {
		return d, nil
	}
	inv, err := invoicexml.Parse(xml)
	if err != nil || inv == nil || strings.TrimSpace(inv.InvoiceNumber) == "" {
		// An XML that is no invoice is kept as a plain file.
		return d, nil
	}
	d.EInvoice = true
	d.Kind = "invoice"
	if inv.InvoiceTypeCode == 381 {
		d.Kind = "credit-note"
	}
	d.Number = strings.TrimSpace(inv.InvoiceNumber)
	if !inv.InvoiceDate.IsZero() {
		d.Date = inv.InvoiceDate.Format("2006-01-02")
	}
	d.Currency = inv.InvoiceCurrencyCode
	d.Net, d.Tax, d.Gross = inv.TaxBasisTotal.StringFixed(2), inv.TaxTotal.StringFixed(2), inv.GrandTotal.StringFixed(2)
	d.Direction = direction(inv, own)
	switch d.Direction {
	case "outgoing":
		d.Partner = inv.Buyer.Name
	default:
		d.Partner = inv.Seller.Name
	}
	return d, nil
}

// direction compares seller and buyer with the own company. Without a clear
// match it stays open rather than guessing.
func direction(inv *einvoice.Invoice, own Own) string {
	seller, buyer := sameParty(inv.Seller, own), sameParty(inv.Buyer, own)
	switch {
	case seller && !buyer:
		return "outgoing"
	case buyer && !seller:
		return "incoming"
	}
	return ""
}

func sameParty(p einvoice.Party, own Own) bool {
	if v := model.CompactIdentifier(own.VATID); v != "" && strings.EqualFold(model.CompactIdentifier(p.VATaxRegistration), v) {
		return true
	}
	if v := digits(own.TaxNumber); v != "" && digits(p.FCTaxRegistration) == v {
		return true
	}
	if v := nameKey(own.Name); v != "" && nameKey(p.Name) == v {
		return true
	}
	return false
}

func digits(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsDigit(r) {
			return r
		}
		return -1
	}, s)
}

func nameKey(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return unicode.ToLower(r)
		}
		return -1
	}, s)
}
