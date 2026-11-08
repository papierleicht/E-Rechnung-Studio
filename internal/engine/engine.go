package engine

import (
	"bytes"
	"errors"
	"fmt"
	"html/template"
	"strings"
	"time"

	"github.com/beevik/etree"
	"github.com/shopspring/decimal"
	"papierleicht.de/e-rechnung-studio/internal/einvoice"
	"papierleicht.de/e-rechnung-studio/internal/model"
	"papierleicht.de/e-rechnung-studio/internal/profile"
)

type Engine struct{}

func New() *Engine { return &Engine{} }

func parseDate(field, value string, required bool) (time.Time, error) {
	if value == "" && !required {
		return time.Time{}, nil
	}
	v, err := time.Parse("2006-01-02", value)
	if err != nil {
		return time.Time{}, fmt.Errorf("Gib für „%s“ ein gültiges Datum ein.", field)
	}
	return v, nil
}

func parseAmount(field, value string, positive bool) (decimal.Decimal, error) {
	if strings.TrimSpace(value) == "" {
		return decimal.Zero, fmt.Errorf("Fülle „%s“ aus.", field)
	}
	v, err := decimal.NewFromString(strings.ReplaceAll(strings.TrimSpace(value), ",", "."))
	if err != nil {
		return decimal.Zero, fmt.Errorf("Gib für „%s“ eine gültige Zahl ein.", field)
	}
	if v.IsNegative() {
		return decimal.Zero, fmt.Errorf("Gib für „%s“ einen Wert größer oder gleich 0 ein.", field)
	}
	if positive && !v.GreaterThan(decimal.Zero) {
		return decimal.Zero, fmt.Errorf("Gib für „%s“ einen Wert größer als 0 ein.", field)
	}
	return v, nil
}

func party(name, contact, email, vat, endpoint, scheme string, a model.Address) einvoice.Party {
	p := einvoice.Party{Name: name, VATaxRegistration: strings.TrimSpace(vat), URIUniversalCommunication: strings.TrimSpace(endpoint), URIUniversalCommunicationScheme: scheme, PostalAddress: &einvoice.PostalAddress{Line1: a.Street, Line2: a.Additional, PostcodeCode: a.Postcode, City: a.City, CountryID: strings.ToUpper(a.Country)}}
	if contact != "" || email != "" {
		p.DefinedTradeContact = []einvoice.DefinedTradeContact{{PersonName: contact, EMail: email}}
	}
	return p
}

func (e *Engine) Build(d model.InvoiceDraft, c model.Company) (*einvoice.Invoice, error) {
	c = model.NormalizeCompany(c)
	d.Customer = model.NormalizeCustomer(d.Customer)
	if !model.SupportedTaxTreatment(d.TaxTreatment) {
		return nil, fmt.Errorf("Wähle einen unterstützten Steuerfall.")
	}
	issue, err := parseDate("Rechnungsdatum", d.IssueDate, true)
	if err != nil {
		return nil, err
	}
	delivery, err := parseDate("Leistungsdatum", d.DeliveryDate, false)
	if err != nil {
		return nil, err
	}
	due, err := parseDate("Fälligkeitsdatum", d.DueDate, false)
	if err != nil {
		return nil, err
	}
	periodStart, err := parseDate("Beginn des Leistungszeitraums", d.PeriodStart, false)
	if err != nil {
		return nil, err
	}
	periodEnd, err := parseDate("Ende des Leistungszeitraums", d.PeriodEnd, false)
	if err != nil {
		return nil, err
	}
	if !periodStart.IsZero() && !periodEnd.IsZero() && periodEnd.Before(periodStart) {
		return nil, errors.New("Das Ende des Leistungszeitraums darf nicht vor dessen Beginn liegen.")
	}
	if d.Currency == "" {
		d.Currency = "EUR"
	}
	f, err := profile.Get(d.Profile)
	if err != nil {
		return nil, err
	}
	if d.Syntax != f.Syntax {
		return nil, fmt.Errorf("Die XML-Syntax passt nicht zum gewählten E-Rechnungsformat.")
	}
	inv := &einvoice.Invoice{SchemaType: einvoice.CII, GuidelineSpecifiedDocumentContextParameter: f.Specification, InvoiceNumber: d.Number, InvoiceTypeCode: 380, InvoiceDate: issue, OccurrenceDateTime: delivery, InvoiceCurrencyCode: d.Currency, BuyerReference: d.BuyerReference, PaymentReference: d.PaymentReference, Seller: party(c.Name, c.ContactName, c.Email, c.VATID, c.Endpoint, c.EndpointScheme, c.Address), Buyer: party(d.Customer.Name, d.Customer.ContactName, d.Customer.Email, d.Customer.VATID, d.Customer.Endpoint, d.Customer.EndpointScheme, d.Customer.Address)}
	if f.Syntax == "ubl" {
		inv.SchemaType = einvoice.UBL
		inv.Seller.SpecifiedLegalOrganization = &einvoice.SpecifiedLegalOrganization{TradingBusinessName: c.Name}
		inv.Buyer.SpecifiedLegalOrganization = &einvoice.SpecifiedLegalOrganization{TradingBusinessName: d.Customer.Name}
	}
	if f.XRechnung {
		inv.BPSpecifiedDocumentContextParameter = strings.TrimSpace(d.BusinessProcess)
		if inv.BPSpecifiedDocumentContextParameter == "" {
			inv.BPSpecifiedDocumentContextParameter = profile.DefaultBusinessProcess
		}
	}
	if c.Phone != "" {
		if len(inv.Seller.DefinedTradeContact) == 0 {
			inv.Seller.DefinedTradeContact = []einvoice.DefinedTradeContact{{}}
		}
		inv.Seller.DefinedTradeContact[0].PhoneNumber = c.Phone
	}
	if d.Kind == "credit-note" {
		inv.InvoiceTypeCode = 381
	} else if d.Kind == "corrected-invoice" {
		inv.InvoiceTypeCode = 384
	} else if d.Kind != "invoice" {
		return nil, fmt.Errorf("Wähle eine unterstützte Belegart.")
	}
	if d.Kind == "invoice" && d.AdvancePayment {
		// The code list of XRechnung leaves out 386: BR-DE-17 warns about it, and a
		// receiving system may refuse it. So 380 stays and a note says it there.
		if f.XRechnung {
			inv.Notes = append(inv.Notes, einvoice.Note{Text: "Anzahlungsrechnung"})
		} else {
			inv.InvoiceTypeCode = 386
		}
	}
	if d.PrecedingInvoiceNumber != "" {
		date, err := parseDate("Ursprüngliches Rechnungsdatum", d.PrecedingInvoiceDate, false)
		if err != nil {
			return nil, err
		}
		inv.InvoiceReferencedDocument = []einvoice.ReferencedDocument{{ID: d.PrecedingInvoiceNumber, Date: date}}
	}
	inv.BillingSpecifiedPeriodStart, inv.BillingSpecifiedPeriodEnd = periodStart, periodEnd
	if model.HasTaxCategory(d, "K") {
		country := strings.ToUpper(strings.TrimSpace(d.DeliveryCountry))
		if country == "" {
			country = d.Customer.Address.Country
		}
		inv.ShipTo = &einvoice.Party{PostalAddress: &einvoice.PostalAddress{CountryID: country}}
		if f.XRechnung {
			a := model.DeliveryLocation(d)
			inv.ShipTo.PostalAddress.City, inv.ShipTo.PostalAddress.PostcodeCode = a.City, a.Postcode
		}
	}
	inv.Seller.FCTaxRegistration = strings.TrimSpace(c.TaxNumber)
	// Category O excludes VAT IDs in the XML. Keep the company master data intact.
	if model.HasTaxCategory(d, "O") {
		inv.Seller.VATaxRegistration, inv.Buyer.VATaxRegistration = "", ""
		if c.TaxNumber == "" && c.VATID != "" {
			inv.Seller.ID = []string{c.VATID}
		}
	}
	// BT-32 alone does not satisfy BR-CO-26. Reuse the supplied tax number
	// as the unqualified seller identifier (BT-29), never as a VAT ID.
	if inv.Seller.VATaxRegistration == "" && inv.Seller.FCTaxRegistration != "" {
		inv.Seller.ID = []string{inv.Seller.FCTaxRegistration}
	}
	if d.Notes != "" {
		inv.Notes = append(inv.Notes, einvoice.Note{Text: d.Notes})
	}
	if strings.TrimSpace(d.PaymentTerms) != "" || !due.IsZero() {
		inv.SpecifiedTradePaymentTerms = []einvoice.SpecifiedTradePaymentTerms{{Description: strings.TrimSpace(d.PaymentTerms), DueDate: due}}
	}
	if err := setPayment(inv, d, c); err != nil {
		return nil, err
	}
	exempt := map[string]string{}
	exemptionCodes := map[string]string{}
	for i, l := range d.Lines {
		q, err := quantity(l.Quantity, d.Kind == "corrected-invoice")
		if err != nil {
			return nil, err
		}
		price, err := parseAmount(fmt.Sprintf("Position %d: Einzelpreis", i+1), l.UnitPrice, false)
		if err != nil {
			return nil, err
		}
		tax, err := model.LineTax(d, l)
		if err != nil {
			return nil, err
		}
		rate, err := parseAmount(fmt.Sprintf("Position %d: Steuersatz", i+1), tax.Rate, false)
		if err != nil {
			return nil, err
		}
		mergeExemption(exempt, exemptionCodes, tax)
		unit := l.Unit
		if unit == "" {
			unit = "C62"
		}
		total := q.Mul(price).Round(2)
		lineNote := ""
		if d.TaxTreatment == "mixed" || tax.Code == "VATEX-EU-F" || tax.Code == "VATEX-EU-I" || tax.Code == "VATEX-EU-J" {
			lineNote = tax.Reason
		}
		inv.InvoiceLines = append(inv.InvoiceLines, einvoice.InvoiceLine{LineID: fmt.Sprint(i + 1), ArticleNumber: l.ArticleNumber, ItemName: l.Name, Description: l.Description, Note: lineNote, BilledQuantity: q, BilledQuantityUnit: unit, NetPrice: price, TaxTypeCode: "VAT", TaxCategoryCode: tax.Category, TaxRateApplicablePercent: rate, Total: total})
	}
	if len(inv.InvoiceLines) == 0 {
		return nil, errors.New("Füge mindestens eine Rechnungsposition hinzu.")
	}
	if err := setAdjustments(inv, d); err != nil {
		return nil, err
	}
	inv.UpdateApplicableTradeTax(exempt)
	for i := range inv.TradeTaxes {
		inv.TradeTaxes[i].ExemptionReasonCode = exemptionCodes[inv.TradeTaxes[i].CategoryCode]
	}
	if err := applySourceTaxes(inv, d.SourceTaxes); err != nil {
		return nil, err
	}
	if d.PrepaidAmount != "" {
		prepaid, err := parseAmount("Bereits bezahlter Betrag", d.PrepaidAmount, false)
		if err != nil {
			return nil, err
		}
		if !prepaid.Equal(prepaid.Round(2)) {
			return nil, errors.New("Der bereits bezahlte Betrag darf höchstens zwei Nachkommastellen haben.")
		}
		inv.TotalPrepaid = prepaid
	}
	inv.UpdateTotals()
	if inv.TotalPrepaid.IsPositive() && inv.DuePayableAmount.IsNegative() {
		return nil, errors.New("Der bereits bezahlte Betrag übersteigt den Rechnungsbetrag. Prüf die Zahlung im Shop.")
	}
	if f.Conformance == "BASIC" {
		// BASIC has no DefinedTradeContact or product Description/SellerAssignedID.
		// Preserve description and article ID in its supported line note (BT-127).
		// Contacts remain in the immutable PDF presentation snapshot.
		inv.Seller.DefinedTradeContact, inv.Buyer.DefinedTradeContact = nil, nil
		for i := range inv.PaymentMeans {
			inv.PaymentMeans[i].PayeeSpecifiedCreditorFinancialInstitutionBIC = ""
			inv.PaymentMeans[i].PayeePartyCreditorFinancialAccountName = ""
		}
		for i := range inv.InvoiceLines {
			line := &inv.InvoiceLines[i]
			line.Note = strings.TrimSpace(line.Description + "\n" + line.Note)
			if line.ArticleNumber != "" {
				if line.Note != "" {
					line.Note += "\n"
				}
				line.Note += "Art.-Nr.: " + line.ArticleNumber
			}
			line.Description = ""
		}
	}
	return inv, nil
}

func (e *Engine) XML(inv *einvoice.Invoice) ([]byte, error) {
	var out bytes.Buffer
	if err := inv.Write(&out); err != nil {
		return nil, err
	}
	if inv.SchemaType == einvoice.UBL {
		// Correct BT-9 placement in the current UBL writer. Invoice uses the
		// root DueDate; CreditNote uses PaymentMeans/PaymentDueDate.
		doc := etree.NewDocument()
		if err := doc.ReadFromBytes(out.Bytes()); err != nil {
			return nil, err
		}
		root := doc.Root()
		normalizeUBLSettlement(root)
		// UBL BT-75..80 belong to DeliveryLocation/Address, not DeliveryParty.
		for _, delivery := range root.FindElements("cac:Delivery") {
			if party := delivery.FindElement("cac:DeliveryParty"); party != nil {
				if address := party.FindElement("cac:PostalAddress"); address != nil {
					party.RemoveChild(address)
					address.Tag = "Address"
					location := etree.NewElement("cac:DeliveryLocation")
					location.AddChild(address)
					delivery.InsertChildAt(party.Index(), location)
					if len(party.ChildElements()) == 0 {
						delivery.RemoveChild(party)
					}
				}
			}
		}
		for _, terms := range root.FindElements("cac:PaymentTerms") {
			if due := terms.FindElement("cbc:PaymentDueDate"); due != nil {
				terms.RemoveChild(due)
				if inv.InvoiceTypeCode == 381 {
					means := root.FindElement("cac:PaymentMeans")
					if means == nil {
						return nil, errors.New("Die Zahlungsanweisung für die Gutschrift fehlt.")
					}
					code := means.FindElement("cbc:PaymentMeansCode")
					means.InsertChildAt(code.Index()+1, due)
				} else {
					due.Tag = "DueDate"
					date := root.FindElement("cbc:IssueDate")
					root.InsertChildAt(date.Index()+1, due)
				}
			}
			if len(terms.ChildElements()) == 0 {
				root.RemoveChild(terms)
			}
		}
		return doc.WriteToBytes()
	}
	// The current writer emits empty BT-23 for EXTENDED even though its XSD
	// makes this group optional. Remove only that known empty optional group.
	if inv.GuidelineSpecifiedDocumentContextParameter == einvoice.SpecFacturXExtended {
		doc := etree.NewDocument()
		if err := doc.ReadFromBytes(out.Bytes()); err != nil {
			return nil, err
		}
		if node := doc.FindElement("rsm:CrossIndustryInvoice/rsm:ExchangedDocumentContext/ram:BusinessProcessSpecifiedDocumentContextParameter"); node != nil && inv.BPSpecifiedDocumentContextParameter == "" {
			node.Parent().RemoveChild(node)
		}
		// EXTENDED groups exempt lines by reason as well as tax category.
		// The current writer only supplies the reason on the VAT breakdown.
		taxes := doc.FindElements("//ram:SpecifiedLineTradeSettlement/ram:ApplicableTradeTax")
		taxes = append(taxes, doc.FindElements("//ram:SpecifiedTradeAllowanceCharge/ram:CategoryTradeTax")...)
		for _, tax := range taxes {
			category := tax.FindElement("ram:CategoryCode").Text()
			for _, headerTax := range inv.TradeTaxes {
				if headerTax.CategoryCode == category && headerTax.ExemptionReason != "" {
					node := etree.NewElement("ram:ExemptionReason")
					node.SetText(headerTax.ExemptionReason)
					tax.InsertChildAt(tax.FindElement("ram:TypeCode").Index()+1, node)
					if headerTax.ExemptionReasonCode != "" {
						code := etree.NewElement("ram:ExemptionReasonCode")
						code.SetText(headerTax.ExemptionReasonCode)
						tax.InsertChildAt(tax.FindElement("ram:CategoryCode").Index()+1, code)
					}
					break
				}
			}
		}
		return doc.WriteToBytes()
	}
	return out.Bytes(), nil
}

func Totals(inv *einvoice.Invoice) model.Totals {
	t := model.Totals{Net: inv.TaxBasisTotal.StringFixed(2), Tax: inv.TaxTotal.StringFixed(2), Gross: inv.GrandTotal.StringFixed(2), Due: inv.DuePayableAmount.StringFixed(2)}
	// A received final invoice deducts earlier payments (BT-113). The app does not issue such documents.
	if !inv.TotalPrepaid.IsZero() {
		t.Prepaid = inv.TotalPrepaid.StringFixed(2)
	}
	return t
}

func LineTotals(inv *einvoice.Invoice) []string {
	out := make([]string, len(inv.InvoiceLines))
	for i, line := range inv.InvoiceLines {
		out[i] = line.Total.StringFixed(2)
	}
	return out
}

type previewData struct {
	Draft   model.InvoiceDraft
	Company model.Company
	Totals  model.Totals
	Rows    []previewRow
}
type previewRow struct{ Name, Description, Quantity, UnitPrice, TaxRate, Total string }

func documentTitle(d model.InvoiceDraft) string {
	switch {
	case d.Kind == "credit-note":
		return "Gutschrift"
	case d.Kind == "invoice" && d.AdvancePayment:
		return "Anzahlungsrechnung"
	}
	return "Rechnung"
}

var previewTemplate = template.Must(template.New("invoice").Funcs(template.FuncMap{"position": func(i int) int { return i + 1 }, "title": documentTitle}).Parse(`
<article class="invoice-paper">
<header><div><span class="eyebrow">{{title .Draft}}</span><h1>{{.Company.Name}}</h1></div><div class="doc-meta"><strong>{{.Draft.Number}}</strong><span>{{.Draft.IssueDate}}</span></div></header>
<section class="address"><small>{{.Company.Name}} · {{.Company.Address.Street}} · {{.Company.Address.Postcode}} {{.Company.Address.City}}</small><h2>{{.Draft.Customer.Name}}</h2><p>{{.Draft.Customer.Address.Street}}<br>{{.Draft.Customer.Address.Postcode}} {{.Draft.Customer.Address.City}}<br>{{.Draft.Customer.Address.Country}}</p></section>
<section class="intro"><h2>{{title .Draft}} {{.Draft.Number}}</h2><p>{{if .Draft.DeliveryDate}}{{if and .Draft.AdvancePayment (eq .Draft.Kind "invoice")}}Voraussichtliches Leistungsdatum{{else}}Leistungsdatum{{end}}: {{.Draft.DeliveryDate}}{{end}}{{if or .Draft.PeriodStart .Draft.PeriodEnd}} · Leistungszeitraum: {{if .Draft.PeriodStart}}ab {{.Draft.PeriodStart}}{{end}}{{if .Draft.PeriodEnd}} bis {{.Draft.PeriodEnd}}{{end}}{{end}}{{if eq .Draft.TaxTreatment "intra-community"}} · Lieferland: {{if .Draft.DeliveryCountry}}{{.Draft.DeliveryCountry}}{{else}}{{.Draft.Customer.Address.Country}}{{end}}{{end}}{{if .Draft.DueDate}} · Fällig: {{.Draft.DueDate}}{{end}}</p></section>
<table><thead><tr><th>Pos.</th><th>Beschreibung</th><th class="num">Menge</th><th class="num">Einzelpreis</th><th class="num">USt.</th><th class="num">Gesamt</th></tr></thead><tbody>{{range $i,$r:=.Rows}}<tr><td>{{position $i}}</td><td><strong>{{$r.Name}}</strong><small>{{$r.Description}}</small></td><td class="num">{{$r.Quantity}}</td><td class="num">{{$r.UnitPrice}} €</td><td class="num">{{$r.TaxRate}} %</td><td class="num">{{$r.Total}} €</td></tr>{{end}}</tbody></table>
<section class="totals"><div><span>Netto</span><strong>{{.Totals.Net}} €</strong></div><div><span>Umsatzsteuer</span><strong>{{.Totals.Tax}} €</strong></div><div class="grand"><span>Gesamt</span><strong>{{.Totals.Gross}} €</strong></div>{{if .Draft.PrepaidAmount}}<div><span>Bereits bezahlt</span><strong>{{.Draft.PrepaidAmount}} €</strong></div><div><span>Zahlbetrag</span><strong>{{.Totals.Due}} €</strong></div>{{end}}</section>
{{if .Draft.TaxExemptionReason}}<p class="tax-note">{{.Draft.TaxExemptionReason}}</p>{{end}}{{if .Draft.Notes}}<p class="notes">{{.Draft.Notes}}</p>{{end}}
<footer>{{.Company.IBAN}} {{.Company.BIC}} · {{.Company.Email}} · {{if .Company.VATID}}USt-IdNr.: {{.Company.VATID}}{{else if .Company.TaxNumber}}Steuernummer: {{.Company.TaxNumber}}{{end}}</footer></article>`))

func (e *Engine) Preview(d model.InvoiceDraft, c model.Company, inv *einvoice.Invoice) (string, error) {
	if d.TaxTreatment == "zero-rated" {
		d.TaxExemptionReason = ""
	}
	if d.TaxExemptionReason == "" && len(d.Lines) > 0 {
		tax, _ := model.LineTax(d, d.Lines[0])
		d.TaxExemptionReason = tax.Reason
	}
	rows := make([]previewRow, len(d.Lines))
	for i, l := range d.Lines {
		q, _ := decimal.NewFromString(strings.ReplaceAll(l.Quantity, ",", "."))
		p, _ := decimal.NewFromString(strings.ReplaceAll(l.UnitPrice, ",", "."))
		tax, _ := model.LineTax(d, l)
		rows[i] = previewRow{Name: l.Name, Description: l.Description, Quantity: l.Quantity, UnitPrice: p.StringFixed(2), TaxRate: tax.Rate, Total: q.Mul(p).Round(2).StringFixed(2)}
	}
	var b bytes.Buffer
	err := previewTemplate.Execute(&b, previewData{d, c, Totals(inv), rows})
	return b.String(), err
}

// applySourceTaxes takes a shop's VAT of a rate group in place of the per-rate
// calculation while the group still has the basis the shop computed it for.
// BR-S-09 accepts a difference below one currency unit; line rounding stays
// far below that.
func applySourceTaxes(inv *einvoice.Invoice, taxes []model.SourceTax) error {
	for _, source := range taxes {
		rate, rateErr := decimal.NewFromString(source.Rate)
		basis, basisErr := decimal.NewFromString(source.Basis)
		amount, amountErr := decimal.NewFromString(source.Amount)
		if rateErr != nil || basisErr != nil || amountErr != nil || !amount.Equal(amount.Round(2)) {
			return errors.New("Die Steuerbeträge aus dem Shop sind ungültig.")
		}
		for i := range inv.TradeTaxes {
			t := &inv.TradeTaxes[i]
			if t.CategoryCode == "S" && t.Percent.Equal(rate) && t.BasisAmount.Equal(basis) && t.CalculatedAmount.Sub(amount).Abs().LessThan(decimal.NewFromInt(1)) {
				t.CalculatedAmount = amount
			}
		}
	}
	return nil
}
