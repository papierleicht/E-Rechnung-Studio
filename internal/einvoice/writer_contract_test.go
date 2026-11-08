package einvoice

import (
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

func contractInvoice(schema CodeSchemaType, code CodeDocument) Invoice {
	return Invoice{
		SchemaType: schema,
		GuidelineSpecifiedDocumentContextParameter: SpecEN16931,
		InvoiceNumber:       `INV<&"2026`,
		InvoiceTypeCode:     code,
		InvoiceDate:         time.Date(2026, 9, 22, 0, 0, 0, 0, time.UTC),
		InvoiceCurrencyCode: "EUR",
		Seller:              Party{Name: "Müller & Söhne <Berlin>"},
		Buyer:               Party{Name: "Käufer & Co."},
	}
}

func TestWriteSchemaSelectionAndEscaping(t *testing.T) {
	for _, tc := range []struct {
		name   string
		schema CodeSchemaType
		code   CodeDocument
		root   xml.Name
	}{
		{"default CII", SchemaTypeUnknown, 380, xml.Name{Space: "urn:un:unece:uncefact:data:standard:CrossIndustryInvoice:100", Local: "CrossIndustryInvoice"}},
		{"explicit CII", CII, 381, xml.Name{Space: "urn:un:unece:uncefact:data:standard:CrossIndustryInvoice:100", Local: "CrossIndustryInvoice"}},
		{"UBL invoice", UBL, 380, xml.Name{Space: "urn:oasis:names:specification:ubl:schema:xsd:Invoice-2", Local: "Invoice"}},
		{"UBL credit note", UBL, 381, xml.Name{Space: "urn:oasis:names:specification:ubl:schema:xsd:CreditNote-2", Local: "CreditNote"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			inv := contractInvoice(tc.schema, tc.code)
			var output bytes.Buffer
			if err := inv.Write(&output); err != nil {
				t.Fatal(err)
			}
			// Independently decode the entire output so neither escaping nor root
			// namespace assertions depend on this package's own parser.
			decoder := xml.NewDecoder(bytes.NewReader(output.Bytes()))
			var root xml.Name
			var foundNumber, foundSeller bool
			for {
				token, err := decoder.Token()
				if err == io.EOF {
					break
				}
				if err != nil {
					t.Fatalf("output is not well-formed XML: %v", err)
				}
				switch node := token.(type) {
				case xml.StartElement:
					if root.Local == "" {
						root = node.Name
					}
				case xml.CharData:
					foundNumber = foundNumber || string(node) == inv.InvoiceNumber
					foundSeller = foundSeller || string(node) == inv.Seller.Name
				}
			}
			if root != tc.root {
				t.Errorf("root = %v, want %v", root, tc.root)
			}
			if !foundNumber || !foundSeller {
				t.Errorf("text changed on XML serialization: number found=%t, seller found=%t", foundNumber, foundSeller)
			}
		})
	}
}

type failingInvoiceWriter struct{ err error }

func (w failingInvoiceWriter) Write([]byte) (int, error) { return 0, w.err }

func TestWritePropagatesOutputErrors(t *testing.T) {
	want := errors.New("output unavailable")
	for _, schema := range []CodeSchemaType{SchemaTypeUnknown, CII, UBL} {
		t.Run(schema.String(), func(t *testing.T) {
			inv := contractInvoice(schema, 380)
			if err := inv.Write(failingInvoiceWriter{want}); !errors.Is(err, want) {
				t.Fatalf("Write error = %v, want wrapped output error", err)
			}
		})
	}
	inv := contractInvoice(CodeSchemaType(-1), 380)
	var output bytes.Buffer
	if err := inv.Write(&output); !errors.Is(err, ErrUnsupportedSchema) {
		t.Fatalf("unsupported schema error = %v", err)
	}
	if output.Len() != 0 {
		t.Fatal("unsupported schema wrote partial output")
	}
}

func TestTotalsPreserveProfilesWithoutLineCalculations(t *testing.T) {
	for _, profile := range []string{"", "urn:example:unknown", SpecFacturXMinimum, SpecFacturXBasicWL, SpecZUGFeRDMinimum} {
		t.Run(profile, func(t *testing.T) {
			inv := Invoice{
				GuidelineSpecifiedDocumentContextParameter: profile,
				LineTotal:        decimal.NewFromInt(100),
				TaxBasisTotal:    decimal.NewFromInt(95),
				TaxTotal:         decimal.RequireFromString("18.05"),
				GrandTotal:       decimal.RequireFromString("113.05"),
				DuePayableAmount: decimal.RequireFromString("63.05"),
				AllowanceTotal:   decimal.NewFromInt(5),
				InvoiceLines:     []InvoiceLine{{Total: decimal.NewFromInt(999)}},
			}
			before := inv
			inv.UpdateTotals()
			for name, pair := range map[string][2]decimal.Decimal{
				"line total":      {inv.LineTotal, before.LineTotal},
				"tax basis":       {inv.TaxBasisTotal, before.TaxBasisTotal},
				"tax total":       {inv.TaxTotal, before.TaxTotal},
				"grand total":     {inv.GrandTotal, before.GrandTotal},
				"payable amount":  {inv.DuePayableAmount, before.DuePayableAmount},
				"allowance total": {inv.AllowanceTotal, before.AllowanceTotal},
			} {
				if !pair[0].Equal(pair[1]) {
					t.Errorf("%s changed from %s to %s", name, pair[1], pair[0])
				}
			}
		})
	}
}

func TestTotalsExcludeGroupingAndInformationLines(t *testing.T) {
	inv := Invoice{GuidelineSpecifiedDocumentContextParameter: SpecFacturXExtended}
	for _, line := range []struct {
		kind   string
		amount int64
	}{
		{"", 40}, {"DETAIL", 60}, {"GROUP", 100}, {"INFORMATION", 100},
	} {
		inv.InvoiceLines = append(inv.InvoiceLines, InvoiceLine{
			LineStatusReasonCode:     line.kind,
			Total:                    decimal.NewFromInt(line.amount),
			TaxCategoryCode:          "S",
			TaxRateApplicablePercent: decimal.NewFromInt(19),
		})
	}
	inv.UpdateApplicableTradeTax(nil)
	inv.UpdateTotals()
	if len(inv.TradeTaxes) != 1 {
		t.Fatalf("VAT breakdown has %d entries, want 1", len(inv.TradeTaxes))
	}
	for name, pair := range map[string][2]decimal.Decimal{
		"line total":  {inv.LineTotal, decimal.NewFromInt(100)},
		"tax basis":   {inv.TradeTaxes[0].BasisAmount, decimal.NewFromInt(100)},
		"VAT":         {inv.TaxTotal, decimal.NewFromInt(19)},
		"grand total": {inv.GrandTotal, decimal.NewFromInt(119)},
	} {
		if !pair[0].Equal(pair[1]) {
			t.Errorf("%s = %s, want %s", name, pair[0], pair[1])
		}
	}
}
