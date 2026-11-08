package einvoice

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/shopspring/decimal"
)

// getDecimal reads an XML Schema decimal without converting through float64.
// Missing or empty optional elements keep their zero value. Exponents, NaN and
// infinity are not XML Schema decimals and are rejected before conversion.
func getDecimal(node *xmlNode, path string) (decimal.Decimal, error) {
	value := strings.Trim(node.value(path), " \t\r\n")
	if value == "" {
		return decimal.Zero, nil
	}
	digits, dots := 0, 0
	for i, ch := range value {
		switch {
		case ch >= '0' && ch <= '9':
			digits++
		case ch == '.':
			dots++
		case i == 0 && (ch == '+' || ch == '-'):
		default:
			return decimal.Zero, fmt.Errorf("invalid decimal value %q at %s", value, path)
		}
	}
	if digits == 0 || dots > 1 {
		return decimal.Zero, fmt.Errorf("invalid decimal value %q at %s", value, path)
	}
	result, err := decimal.NewFromString(value)
	if err != nil {
		return decimal.Zero, fmt.Errorf("invalid decimal value %q at %s: %w", value, path, err)
	}
	return result, nil
}

// ParseReader reads a CII CrossIndustryInvoice, UBL Invoice or UBL CreditNote.
// Both the root namespace URI and local name must identify a supported format.
// This is a syntax mapper; business-rule validation is performed by the caller.
func ParseReader(r io.Reader) (*Invoice, error) {
	root, err := readInvoiceXML(r)
	if err != nil {
		return nil, fmt.Errorf("cannot read invoice XML: %w", err)
	}
	var inv *Invoice
	switch {
	case root.name.Space == nsCIIRootInvoice && root.name.Local == "CrossIndustryInvoice":
		inv, err = parseCII(root)
	case root.name.Space == nsUBLInvoice && root.name.Local == "Invoice":
		inv, err = parseUBL(root)
	case root.name.Space == nsUBLCreditNote && root.name.Local == "CreditNote":
		inv, err = parseUBL(root)
	default:
		return nil, fmt.Errorf("unsupported invoice root {%s}%s", root.name.Space, root.name.Local)
	}
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", root.name.Local, err)
	}
	inv.isParsed = true
	return inv, nil
}

// ParseXMLFile reads an invoice XML file.
func ParseXMLFile(filename string) (*Invoice, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("einvoice: cannot open file: %w", err)
	}
	defer file.Close()
	return ParseReader(file)
}
