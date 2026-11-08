// Package pdfinvoice creates PDF/A-3b documents from immutable invoice XML.
package pdfinvoice

import (
	"bytes"
	_ "embed"
	"fmt"
	"html/template"
	"os"
	"papierleicht.de/e-rechnung-studio/internal/invoicexml"
	"papierleicht.de/e-rechnung-studio/internal/model"
	"papierleicht.de/e-rechnung-studio/internal/profile"
	"strings"
	"sync"
	"time"
	"unicode"

	pdf "github.com/boxesandglue/baseline-pdf"
	"github.com/boxesandglue/boxesandglue/backend/document"
	"github.com/boxesandglue/boxesandglue/frontend"
	"github.com/boxesandglue/htmlbag"
	"github.com/shopspring/decimal"
	"papierleicht.de/e-rechnung-studio/internal/einvoice"
)

// creatorName is the PDF metadata entry naming the software that made a document.
const creatorName = "papierleicht E-Rechnung-Studio"

// Serialize rendering: the font/layout dependencies have shared caches.
var renderMu sync.Mutex

//go:embed invoice.html
var invoiceHTML string

//go:embed invoice.css
var invoiceCSS string

// pdfCSS keeps the PDF layout that htmlbag 0.0.48 produced; the browser
// preview never reads it. htmlbag now defaults to CSS half-leading, which
// moves every line down, and now draws a paragraph's border and padding
// inside a table cell, which the grand total never showed in the PDF.
//
// htmlbag also hyphenates by the German patterns of <html lang="de">, which
// the browser preview never does. Document numbers, names, addresses, IBANs,
// e-mail addresses and tax numbers must keep their exact characters, so they
// break only where they already contain a hyphen or a space; descriptions and
// notes keep the hyphenation.
const pdfCSS = `
body { -bag-leading-model: trailing; }
.total-entry.grand { border-top: none; padding: 0; }
.eyebrow-gap { letter-spacing: 0; }
@page { @bottom-center { vertical-align: top; } }
.identity, .invoice-meta, .recipient, .payment, .payment-qr, .pagefooter { hyphenate-character: ""; hyphenate-limit-chars: auto 3 3; }
`

// isAdvance reports an Anzahlungsrechnung: type 386 or, in XRechnung, which has
// no such type, the note the engine writes instead.
func isAdvance(inv *einvoice.Invoice) bool {
	if inv.InvoiceTypeCode == 386 {
		return true
	}
	for _, n := range inv.Notes {
		if n.Text == "Anzahlungsrechnung" {
			return true
		}
	}
	return false
}

var invoiceTemplate = template.Must(template.New("pdf-invoice").Funcs(template.FuncMap{
	"advance":       isAdvance,
	"sellerHeading": splitSellerName,
	"iban":          displayIBAN,
	"price": func(v decimal.Decimal) string {
		return germanNumber(v, max(int32(2), -v.Exponent()))
	},
	"money":  germanMoney,
	"number": func(v decimal.Decimal) string { return strings.ReplaceAll(v.String(), ".", ",") },
	"date": func(t time.Time) string {
		if t.IsZero() {
			return ""
		}
		return t.Format("02.01.2006")
	},
	"unit": func(s string) string {
		if v, ok := map[string]string{"C62": "Stück", "HUR": "Stunden", "DAY": "Tage", "KGM": "kg", "MTR": "Meter", "LS": "Pauschale", "WEE": "Wochen", "MON": "Monate", "LTR": "Liter", "MTK": "m²", "MTQ": "m³", "KMT": "km", "TNE": "t"}[s]; ok {
			return v
		}
		return s
	},
}).Parse(invoiceHTML))

func Render(xml []byte) ([]byte, error) {
	return RenderWithSnapshot(xml, nil, nil)
}

// RenderWithSnapshot supplements BASIC's readable presentation with the contact
// and bank detail fields its XML schema does not support. Issued snapshots, never current
// settings, are passed here. Financial data always comes from the validated XML.
func RenderWithSnapshot(xml []byte, issuer *model.Company, buyer *model.Customer, paymentQR ...PaymentQROptions) ([]byte, error) {
	var options PaymentQROptions
	if len(paymentQR) > 0 {
		options = paymentQR[0]
	}
	return renderWithPlan(xml, issuer, buyer, nil, options, nil)
}

type PaymentQROptions struct {
	Enabled     bool
	HideDetails bool
}

type invoiceTemplateData struct {
	*einvoice.Invoice
	Rows                 []pdfRow
	BottomGroups         [][]pdfBottomRow
	MarginScheme         bool
	PaymentQR            template.HTMLAttr
	HidePaymentQRDetails bool
	Logo                 template.HTMLAttr
	Quote                *QuoteDetails
}

// A plan-only pass uses the same typesetter and shipout as export, but skips
// final font embedding and PDF serialization. No canvas or PDF parsing is needed.
func renderWithPlan(xml []byte, issuer *model.Company, buyer *model.Customer, plan *pagePlan, paymentQR PaymentQROptions, quote *QuoteDetails) ([]byte, error) {
	renderMu.Lock()
	defer renderMu.Unlock()
	inv, err := presentationInvoice(xml, issuer, buyer)
	if err != nil {
		return nil, err
	}
	format, err := profile.Detect(inv)
	if err != nil {
		return nil, err
	}
	var html bytes.Buffer
	var qrPath string
	if paymentQR.Enabled {
		qrPath, err = paymentQRImage(inv)
		if err != nil {
			return nil, err
		}
		if qrPath != "" {
			defer os.Remove(qrPath)
		}
	}
	logo, err := issuerLogo(issuer)
	if err != nil {
		return nil, err
	}
	logoPath, err := logoImageFile(logo)
	if err != nil {
		return nil, err
	}
	if logoPath != "" {
		defer os.Remove(logoPath)
	}
	marginScheme := isMarginSchemeInvoice(inv)
	if err = invoiceTemplate.Execute(&html, invoiceTemplateData{
		Invoice: inv, Rows: layoutRows(inv.InvoiceLines, marginScheme), BottomGroups: bottomGroups(inv, marginScheme),
		MarginScheme: marginScheme, PaymentQR: localImageSource(qrPath), HidePaymentQRDetails: paymentQR.HideDetails,
		Logo: logoAttributes(localImageSource(logoPath), logo), Quote: quote,
	}); err != nil {
		return nil, err
	}
	source, err := flowDocument(html.String(), nil)
	if err != nil {
		return nil, err
	}
	var out bytes.Buffer
	fe, err := frontend.NewForWriter(&out)
	if err != nil {
		return nil, err
	}
	var missingGlyph rune
	fe.MissingGlyphFunc = func(_ *pdf.Face, r rune) {
		if missingGlyph == 0 {
			missingGlyph = r
		}
	}
	fe.Doc.Format = document.FormatPDFA3b
	fe.Doc.Title = "Rechnung " + inv.InvoiceNumber
	fe.Doc.Author = inv.Seller.Name
	fe.Doc.Creator = creatorName
	if _, err = fe.Doc.LoadSRGBColorprofile(); err != nil {
		return nil, err
	}
	if quote != nil {
		// A quote is no invoice: it neither embeds nor announces e-invoice XML.
		fe.Doc.Title = "Angebot " + inv.InvoiceNumber
	} else {
		attachInvoiceXML(fe, inv, format, xml)
	}
	cb, err := htmlbag.New(fe, htmlbag.NewCSSParserWithDefaults())
	if err != nil {
		return nil, err
	}
	if err = cb.AddCSS(invoiceCSS + pdfCSS); err != nil {
		return nil, err
	}
	if err = cb.InitPage(); err != nil {
		return nil, err
	}
	text, err := cb.HTMLToText(source)
	if err != nil {
		return nil, err
	}
	if err = cb.OutputPagesFromText(text); err != nil {
		return nil, err
	}
	if missingGlyph != 0 {
		return nil, fmt.Errorf("Das Zeichen %q kann in der PDF-Schrift nicht dargestellt werden. Ersetze es und starte den PDF-Export noch einmal.", missingGlyph)
	}
	if plan != nil {
		plan.Pages = len(fe.Doc.Pages)
		plan.Anchors = make(map[string]int)
		for name, anchor := range fe.Doc.PDFWriter.NameDestinations {
			for i, page := range fe.Doc.Pages {
				if page.Objectnumber == anchor.PageObjectnumber {
					plan.Anchors[string(name)] = i + 1
					break
				}
			}
		}
		return nil, nil
	}
	if err = fe.Finish(); err != nil {
		return nil, err
	}
	if err = fe.Doc.PDFWriter.FinishAndClose(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// attachInvoiceXML embeds the validated XML and, for Factur-X, its XMP metadata.
func attachInvoiceXML(fe *frontend.Document, inv *einvoice.Invoice, format profile.Format, xml []byte) {
	attachmentName, description := "factur-x.xml", "Elektronische Rechnung ("+format.Conformance+")"
	if format.XRechnung {
		attachmentName, description = "xrechnung.xml", "XRechnung 3.0 · "+strings.ToUpper(format.Syntax)
	}
	fe.Doc.AttachFile(document.Attachment{Name: attachmentName, Description: description, MimeType: "text/xml", Data: xml, CreationDate: inv.InvoiceDate, ModDate: inv.InvoiceDate})
	// XRechnung is an XML format. Its accompanying PDF embeds that XML but
	// must not claim Factur-X conformance in the XMP metadata.
	if !format.XRechnung {
		x := document.XMPExtension{Schema: "Factur-X PDFA Extension Schema", NamespaceURI: "urn:factur-x:pdfa:CrossIndustryDocument:invoice:1p0#", Prefix: "fx", Values: map[string]string{"DocumentFileName": "factur-x.xml", "DocumentType": "INVOICE", "Version": "1.0", "ConformanceLevel": format.Conformance}}
		for _, n := range []string{"DocumentFileName", "DocumentType", "Version", "ConformanceLevel"} {
			x.Properties = append(x.Properties, document.XMPExtensionProperty{Name: n, ValueType: "Text", Category: "external", Description: n})
		}
		fe.Doc.AddXMPExtension(x)
	}
}

// htmlbag opens image sources as literal filesystem paths, not URLs. URL
// filtering rejects Windows drive letters and URL escaping changes spaces and
// backslashes. Only pass paths to images created by this renderer here; escape
// HTML attribute syntax while preserving the filename for the HTML parser.
func localImageSource(path string) template.HTMLAttr {
	if path == "" {
		return ""
	}
	return template.HTMLAttr(`src="` + template.HTMLEscapeString(path) + `"`)
}

// Bound each table row so descriptions can continue across page boundaries.
// No text is discarded; concatenating the chunks reproduces the input exactly.
func textChunks(text string, limit int) []string {
	runes := []rune(text)
	var chunks []string
	for len(runes) > 0 {
		split := min(limit, len(runes))
		newlines := 0
		for i, r := range runes[:split] {
			if r == '\n' {
				newlines++
			}
			if newlines == 12 {
				split = i + 1
				break
			}
		}
		if split < len(runes) && newlines < 12 {
			for i := split; i > split/2; i-- {
				if unicode.IsSpace(runes[i-1]) {
					split = i
					break
				}
			}
		}
		chunks = append(chunks, string(runes[:split]))
		runes = runes[split:]
	}
	return chunks
}

type pdfRow struct {
	einvoice.InvoiceLine
	Heading, Details string
	TaxLabel         string
	Continuation     bool
}

func layoutRows(lines []einvoice.InvoiceLine, marginInvoice ...bool) []pdfRow {
	var rows []pdfRow
	pureMargin := len(marginInvoice) > 0 && marginInvoice[0]
	for _, line := range lines {
		headings := textChunks(line.ItemName, 180)
		description := line.Description
		if line.Note != "" {
			if description != "" {
				description += "\n"
			}
			description += line.Note
		}
		marginLine := pureMargin || (line.TaxCategoryCode == "E" && strings.Contains(line.Note, "/Sonderregelung"))
		if marginLine && !strings.Contains(description, "/Sonderregelung") {
			// Older uniform-margin invoices carry their statutory reason only
			// in the document's tax breakdown. Keep the position identifiable.
			description = strings.TrimSpace(description + "\nDifferenzbesteuerung (siehe Steuerhinweis)")
		}
		details := textChunks(description, 300)
		if len(headings) == 0 {
			headings = []string{""}
		}
		taxLabel := numberPercent(line.TaxRateApplicablePercent) + " %"
		if marginLine {
			taxLabel = "—"
		}
		for i, heading := range headings {
			rows = append(rows, pdfRow{InvoiceLine: line, Heading: heading, TaxLabel: taxLabel, Continuation: i > 0})
		}
		for i, detail := range details {
			if i == 0 {
				rows[len(rows)-1].Details = detail
			} else {
				rows = append(rows, pdfRow{InvoiceLine: line, Details: detail, Continuation: true})
			}
		}
	}
	return rows
}

func numberPercent(value decimal.Decimal) string {
	return strings.ReplaceAll(value.String(), ".", ",")
}

func isMarginSchemeInvoice(inv *einvoice.Invoice) bool {
	if len(inv.TradeTaxes) == 0 {
		return false
	}
	for _, tax := range inv.TradeTaxes {
		if tax.ExemptionReasonCode != "VATEX-EU-F" && tax.ExemptionReasonCode != "VATEX-EU-I" && tax.ExemptionReasonCode != "VATEX-EU-J" {
			return false
		}
	}
	return true
}

// Keep the short total block together, then allow lengthy notes to continue
// in bounded rows. Nested tables are deliberately avoided in the PDF layout.
type pdfBottomRow struct {
	Caption, Text, Label string
	Amount               decimal.Decimal
	HasAmount, Grand     bool
}

func bottomGroups(inv *einvoice.Invoice, marginInvoice ...bool) [][]pdfBottomRow {
	marginScheme := len(marginInvoice) > 0 && marginInvoice[0]
	var notes []pdfBottomRow
	add := func(caption, text string) {
		for i, chunk := range textChunks(text, 240) {
			row := pdfBottomRow{Text: chunk}
			if i == 0 {
				row.Caption = caption
			}
			notes = append(notes, row)
		}
	}
	for _, tax := range inv.TradeTaxes {
		add("Steuerlicher Hinweis", tax.ExemptionReason)
	}
	for _, terms := range inv.SpecifiedTradePaymentTerms {
		add("Zahlungsbedingungen", terms.Description)
	}
	for index, payment := range inv.PaymentMeans {
		label := payment.Information
		if payment.TypeCode == 10 {
			label = "Barzahlung"
		} else if payment.TypeCode == 97 {
			label = "Zahlung nach Vereinbarung"
		}
		if label != "" {
			add("Zahlungsart", label)
		}
		if index >= 2 && payment.PayeePartyCreditorFinancialAccountIBAN != "" {
			text := payment.PayeePartyCreditorFinancialAccountName + "\nIBAN: " + displayIBAN(payment.PayeePartyCreditorFinancialAccountIBAN)
			if payment.PayeeSpecifiedCreditorFinancialInstitutionBIC != "" {
				text += "\nBIC: " + payment.PayeeSpecifiedCreditorFinancialInstitutionBIC
			}
			add("Bankverbindung", strings.TrimSpace(text))
		}
	}
	add("Zahlungsreferenz", inv.PaymentReference)
	for _, note := range inv.Notes {
		add("Hinweis", note.Text)
	}
	var totals []pdfBottomRow
	if len(inv.SpecifiedTradeAllowanceCharge) > 0 {
		totals = append(totals, pdfBottomRow{Label: "Positionssumme", Amount: inv.LineTotal, HasAmount: true})
		if !inv.AllowanceTotal.IsZero() {
			totals = append(totals, pdfBottomRow{Label: "Nachlässe", Amount: inv.AllowanceTotal.Neg(), HasAmount: true})
		}
		if !inv.ChargeTotal.IsZero() {
			totals = append(totals, pdfBottomRow{Label: "Zuschläge", Amount: inv.ChargeTotal, HasAmount: true})
		}
	}
	if !marginScheme {
		totals = append(totals, pdfBottomRow{Label: "Netto", Amount: inv.TaxBasisTotal, HasAmount: true})
		for _, tax := range inv.TradeTaxes {
			totals = append(totals, pdfBottomRow{Label: "Umsatzsteuer " + strings.ReplaceAll(tax.Percent.String(), ".", ",") + " %", Amount: tax.CalculatedAmount, HasAmount: true})
		}
	}
	totals = append(totals, pdfBottomRow{Label: "Gesamt", Amount: inv.GrandTotal, HasAmount: true, Grand: true})
	if inv.TotalPrepaid.IsPositive() {
		totals = append(totals, pdfBottomRow{Label: "Bereits bezahlt", Amount: inv.TotalPrepaid.Neg(), HasAmount: true})
	}
	if !inv.DuePayableAmount.Equal(inv.GrandTotal) {
		totals = append(totals, pdfBottomRow{Label: "Zahlbetrag", Amount: inv.DuePayableAmount, HasAmount: true})
	}
	first := make([]pdfBottomRow, len(totals))
	for i, total := range totals {
		first[i] = total
		if i == 0 && len(notes) > 0 {
			first[i].Caption = notes[0].Caption
			first[i].Text = notes[0].Text
		}
	}
	groups := [][]pdfBottomRow{first}
	if len(notes) > 1 {
		groups = append(groups, notes[1:])
	}
	return groups
}

func presentationInvoice(xml []byte, issuer *model.Company, buyer *model.Customer) (*einvoice.Invoice, error) {
	inv, err := invoicexml.Parse(xml)
	if err != nil {
		return nil, err
	}
	format, err := profile.Detect(inv)
	if err != nil {
		return nil, err
	}
	if format.Conformance == "BASIC" {
		if issuer != nil {
			for i := range inv.PaymentMeans {
				payment := &inv.PaymentMeans[i]
				for _, account := range issuer.BankAccounts() {
					if payment.PayeePartyCreditorFinancialAccountIBAN == model.CompactIdentifier(account.IBAN) {
						payment.PayeeSpecifiedCreditorFinancialInstitutionBIC = account.BIC
						payment.PayeePartyCreditorFinancialAccountName = account.Name
					}
				}
			}
		}
		if issuer != nil && (issuer.ContactName != "" || issuer.Email != "" || issuer.Phone != "") {
			inv.Seller.DefinedTradeContact = []einvoice.DefinedTradeContact{{PersonName: issuer.ContactName, EMail: issuer.Email, PhoneNumber: issuer.Phone}}
		}
		if buyer != nil && (buyer.ContactName != "" || buyer.Email != "") {
			inv.Buyer.DefinedTradeContact = []einvoice.DefinedTradeContact{{PersonName: buyer.ContactName, EMail: buyer.Email}}
		}
	}
	if inv.Seller.PostalAddress == nil || inv.Buyer.PostalAddress == nil {
		return nil, fmt.Errorf("Rechnungsadressen fehlen")
	}
	return inv, nil
}

// germanMoney writes an amount as printed on a German invoice: 1.234,50.
func germanMoney(v decimal.Decimal) string { return germanNumber(v, 2) }

func germanNumber(v decimal.Decimal, places int32) string {
	whole, cents, _ := strings.Cut(v.Abs().StringFixed(places), ".")
	for i := len(whole) - 3; i > 0; i -= 3 {
		whole = whole[:i] + "." + whole[i:]
	}
	sign := ""
	if v.IsNegative() {
		sign = "-"
	}
	return sign + whole + "," + cents
}
