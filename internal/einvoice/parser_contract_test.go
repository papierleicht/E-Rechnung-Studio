package einvoice

import (
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/shopspring/decimal"
)

func readParserFixture(t testing.TB, name string) string {
	t.Helper()
	data, err := os.ReadFile("testdata/" + name + ".xml")
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func parseFixtureXML(t testing.TB, data string) *Invoice {
	t.Helper()
	inv, err := ParseReader(strings.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	return inv
}

func minimalUBL(body string) string {
	return `<Invoice xmlns="` + nsUBLInvoice + `" xmlns:cbc="` + nsUBLCBC + `" xmlns:cac="` + nsUBLCAC + `" xmlns:foreign="urn:foreign">` + body + `</Invoice>`
}

func TestParserNamespaceEquivalence(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"cii", "ubl-invoice", "ubl-creditnote"} {
		t.Run(name, func(t *testing.T) {
			data := readParserFixture(t, name)
			want := parseFixtureXML(t, data)
			if want.InvoiceNumber != "471102" || want.Seller.Name != "Beispiel Lieferant" || len(want.InvoiceLines) != 1 || want.InvoiceLines[0].BilledQuantity.String() != "2" || want.GrandTotal.String() != "119" {
				t.Fatalf("incorrect fixture mapping: %#v", want)
			}
			if want.Seller.FCTaxRegistration != "TEST-TAX" || want.Seller.VATaxRegistration != "DE123456789" {
				t.Fatalf("tax schemes were not selected independently: %#v", want.Seller)
			}
			if len(want.Notes) != 1 || want.Notes[0].Text != "Projektbeispiel & Test" {
				t.Fatalf("XML character data changed: %#v", want.Notes)
			}
			if len(want.SpecifiedTradePaymentTerms) != 1 || want.SpecifiedTradePaymentTerms[0].DueDate.Format("2006-01-02") != "2026-10-06" {
				t.Fatalf("payment date missing: %#v", want.SpecifiedTradePaymentTerms)
			}
			prefixed := strings.NewReplacer("xmlns:rsm=", "xmlns:document=", "rsm:", "document:", "xmlns:ram=", "xmlns:aggregate=", "ram:", "aggregate:", "xmlns:udt=", "xmlns:types=", "udt:", "types:", "xmlns:cbc=", "xmlns:basic=", "cbc:", "basic:", "xmlns:cac=", "xmlns:aggregate=", "cac:", "aggregate:").Replace(data)
			defaultNS := data
			if name == "cii" {
				defaultNS = strings.NewReplacer("xmlns:ram=", "xmlns=", "<ram:", "<", "</ram:", "</").Replace(data)
			} else {
				root := "Invoice"
				if name == "ubl-creditnote" {
					root = "CreditNote"
				}
				defaultNS = strings.NewReplacer("<"+root+" xmlns=", "<doc:"+root+" xmlns:doc=", "</"+root+">", "</doc:"+root+">", "xmlns:cbc=", "xmlns=", "<cbc:", "<", "</cbc:", "</").Replace(data)
			}
			for variant, input := range map[string]string{"renamed prefixes": prefixed, "default component namespace": defaultNS, "UTF-8 BOM": "\ufeff" + data} {
				t.Run(variant, func(t *testing.T) {
					if got := parseFixtureXML(t, input); !reflect.DeepEqual(got, want) {
						t.Fatalf("mapping depends on namespace spelling or BOM\ngot: %#v\nwant: %#v", got, want)
					}
				})
			}
		})
	}
}

func TestParserRejectsMalformedAndUnsupportedXML(t *testing.T) {
	t.Parallel()
	valid := minimalUBL("")
	cases := map[string]string{
		"empty":                        "",
		"missing root":                 `<?xml version="1.0"?>`,
		"missing namespace":            `<Invoice/>`,
		"unsupported namespace":        `<Invoice xmlns="urn:unsupported"/>`,
		"wrong root local name":        `<Other xmlns="` + nsUBLInvoice + `"/>`,
		"wrong credit note namespace":  `<CreditNote xmlns="` + nsUBLInvoice + `"/>`,
		"wrong invoice namespace":      `<Invoice xmlns="` + nsUBLCreditNote + `"/>`,
		"wrong CII root":               `<Invoice xmlns="` + nsCIIRootInvoice + `"/>`,
		"unclosed":                     strings.TrimSuffix(valid, "</Invoice>"),
		"mismatched tags":              minimalUBL(`<cbc:ID>test</cbc:Name>`),
		"multiple roots":               valid + valid,
		"outside text":                 "text" + valid,
		"trailing text":                valid + "text",
		"outside non-XML whitespace":   "\u00a0" + valid,
		"trailing BOM":                 valid + "\ufeff",
		"non-leading BOM":              " \ufeff" + valid,
		"duplicate BOM":                "\ufeff\ufeff" + valid,
		"undeclared prefix":            minimalUBL(`<missing:ID>test</missing:ID>`),
		"undeclared attribute prefix":  minimalUBL(`<cbc:ID missing:id="test">test</cbc:ID>`),
		"unbound namespace prefix":     minimalUBL(`<cbc:ID xmlns:cbc="">test</cbc:ID>`),
		"duplicate attribute":          minimalUBL(`<cbc:ID schemeID="1" schemeID="2"/>`),
		"duplicate expanded attribute": minimalUBL(`<cbc:ID xmlns:a="urn:test" xmlns:b="urn:test" a:id="1" b:id="2"/>`),
		"duplicate namespace":          minimalUBL(`<cbc:ID xmlns:a="urn:a" xmlns:a="urn:b"/>`),
		"reserved namespace prefix":    minimalUBL(`<cbc:ID xmlns:xml="urn:wrong"/>`),
		"reserved namespace URI":       minimalUBL(`<cbc:ID xmlns:a="http://www.w3.org/XML/1998/namespace"/>`),
		"invalid local NCName":         minimalUBL(`<cbc:1bad/>`),
		"invalid attribute NCName":     minimalUBL(`<cbc:ID cbc:1bad="yes"/>`),
		"invalid namespace NCName":     minimalUBL(`<cbc:ID xmlns:1bad="urn:test"/>`),
		"nested declaration":           minimalUBL(`<?xml version="1.0"?>`),
		"duplicate declaration":        `<?xml version="1.0"?><?xml version="1.0"?>` + valid,
		"late declaration":             "\n" + `<?xml version="1.0"?>` + valid,
		"reserved PI case":             `<?XML version="1.0"?>` + valid,
		"unknown entity":               minimalUBL(`<cbc:ID>&external;</cbc:ID>`),
		"external DTD":                 `<!DOCTYPE Invoice SYSTEM "file:///nonexistent-invoice.dtd">` + valid,
		"internal DTD":                 `<!DOCTYPE Invoice [<!ENTITY external "replaced">]>` + valid,
		"excessive depth":              minimalUBL(strings.Repeat(`<cac:Party>`, 256) + strings.Repeat(`</cac:Party>`, 256)),
	}
	for name, input := range cases {
		t.Run(name, func(t *testing.T) {
			if inv, err := ParseReader(strings.NewReader(input)); err == nil || inv != nil {
				t.Fatalf("expected error and nil invoice, got invoice=%v error=%v", inv, err)
			}
		})
	}
}

func TestParserIgnoresForeignNamespaceLookalikes(t *testing.T) {
	t.Parallel()
	data := minimalUBL(`<foreign:ID>wrong</foreign:ID><cbc:ID xmlns:cbc="urn:shadowed">wrong</cbc:ID><ID xmlns="">wrong</ID><cbc:ID>correct</cbc:ID>
<cac:AccountingSupplierParty xmlns:cac="urn:shadowed"><cac:Party><cac:PartyName><cbc:Name>wrong</cbc:Name></cac:PartyName></cac:Party></cac:AccountingSupplierParty>
<cac:LegalMonetaryTotal><foreign:PayableAmount>999</foreign:PayableAmount><cbc:PayableAmount>12.34</cbc:PayableAmount></cac:LegalMonetaryTotal>
<cac:InvoiceLine><cbc:ID>1</cbc:ID><foreign:InvoicedQuantity>999</foreign:InvoicedQuantity><cbc:InvoicedQuantity foreign:unitCode="WRONG">2</cbc:InvoicedQuantity></cac:InvoiceLine>`)
	inv := parseFixtureXML(t, data)
	if inv.InvoiceNumber != "correct" || inv.Seller.Name != "" || inv.DuePayableAmount.String() != "12.34" || len(inv.InvoiceLines) != 1 || inv.InvoiceLines[0].BilledQuantity.String() != "2" || inv.InvoiceLines[0].BilledQuantityUnit != "" {
		t.Fatalf("foreign namespace affected invoice fields: %#v", inv)
	}
	cii := readParserFixture(t, "cii")
	cii = strings.Replace(cii, `<ram:SellerTradeParty>`, `<ram:SellerTradeParty><fake:Name xmlns:fake="urn:foreign">wrong</fake:Name>`, 1)
	cii = strings.Replace(cii, `schemeID="VA"`, `xmlns:fake="urn:foreign" fake:schemeID="VA"`, 1)
	inv = parseFixtureXML(t, cii)
	if inv.Seller.Name != "Beispiel Lieferant" || inv.Seller.VATaxRegistration != "" || inv.Seller.FCTaxRegistration != "TEST-TAX" {
		t.Fatalf("foreign CII elements or attributes affected party: %#v", inv.Seller)
	}
}

func TestParserExactDecimalValues(t *testing.T) {
	t.Parallel()
	for _, value := range []string{"123456789012345678901234567890.123456789", "-0.125", "+.50", "  12.3400\n", "0"} {
		t.Run(value, func(t *testing.T) {
			inv := parseFixtureXML(t, minimalUBL(`<cac:LegalMonetaryTotal><cbc:PayableAmount>`+value+`</cbc:PayableAmount></cac:LegalMonetaryTotal>`))
			if !inv.DuePayableAmount.Equal(decimal.RequireFromString(strings.TrimSpace(value))) || !inv.hasDuePayableAmountInXML {
				t.Fatalf("decimal lost precision: %s", inv.DuePayableAmount)
			}
		})
	}
	for _, value := range []string{"NaN", "Inf", "1e3", "1,25", "--1", ".", "1.2.3", "12 34", "\u00a012.34", "١٢"} {
		t.Run("invalid "+value, func(t *testing.T) {
			for _, data := range []string{
				minimalUBL(`<cac:LegalMonetaryTotal><cbc:PayableAmount>` + value + `</cbc:PayableAmount></cac:LegalMonetaryTotal>`),
				strings.Replace(readParserFixture(t, "cii"), `<ram:DuePayableAmount>119.00</ram:DuePayableAmount>`, `<ram:DuePayableAmount>`+value+`</ram:DuePayableAmount>`, 1),
			} {
				if _, err := ParseReader(strings.NewReader(data)); err == nil || !strings.Contains(err.Error(), "invalid decimal value") {
					t.Fatalf("invalid decimal was not rejected: %v", err)
				}
			}
		})
	}
}

func TestParserDeliveryAddressAndCreditNoteDueDate(t *testing.T) {
	t.Parallel()
	for _, fixture := range []string{"ubl-invoice", "ubl-creditnote"} {
		inv := parseFixtureXML(t, readParserFixture(t, fixture))
		if inv.ShipTo == nil || inv.ShipTo.Name != "Warenannahme" || inv.ShipTo.PostalAddress == nil || inv.ShipTo.PostalAddress.Line1 != "Lieferweg 2" || inv.ShipTo.PostalAddress.City != "Bonn" {
			t.Fatalf("delivery party and location address were not combined: %#v", inv.ShipTo)
		}
	}
	inv := parseFixtureXML(t, minimalUBL(`<cac:Delivery><cac:DeliveryLocation><cbc:ID>location-only</cbc:ID></cac:DeliveryLocation></cac:Delivery>`))
	if inv.ShipTo != nil {
		t.Fatalf("location identifier alone created an empty party: %#v", inv.ShipTo)
	}
	creditNote := readParserFixture(t, "ubl-creditnote")
	for _, date := range []string{"", "2026-02-30", "06.10.2026"} {
		data := strings.Replace(creditNote, `<cbc:PaymentDueDate>2026-10-06</cbc:PaymentDueDate>`, `<cbc:PaymentDueDate>`+date+`</cbc:PaymentDueDate>`, 1)
		if _, err := ParseReader(strings.NewReader(data)); err == nil {
			t.Fatalf("invalid credit note due date %q was accepted", date)
		}
	}
	withoutTerms := strings.Replace(creditNote, `<cac:PaymentTerms><cbc:Note>14 Tage</cbc:Note></cac:PaymentTerms>`, "", 1)
	inv = parseFixtureXML(t, withoutTerms)
	if len(inv.SpecifiedTradePaymentTerms) != 1 || inv.SpecifiedTradePaymentTerms[0].DueDate.Format("2006-01-02") != "2026-10-06" {
		t.Fatalf("credit note due date without payment terms was lost: %#v", inv.SpecifiedTradePaymentTerms)
	}
}

func TestParserKeepsMixedTextOrderAndCDATA(t *testing.T) {
	t.Parallel()
	inv := parseFixtureXML(t, minimalUBL(`<cbc:Note>first<![CDATA[ & second]]><foreign:span> third</foreign:span> last</cbc:Note>`))
	if len(inv.Notes) != 1 || inv.Notes[0].Text != "first & second third last" {
		t.Fatalf("text content changed: %#v", inv.Notes)
	}
}
