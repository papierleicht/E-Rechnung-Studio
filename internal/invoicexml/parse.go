// Package invoicexml exposes byte-oriented parsing for application callers.
package invoicexml

import (
	"bytes"

	"papierleicht.de/e-rechnung-studio/internal/einvoice"
)

// Parse maps CII or UBL XML to the shared invoice model. The owned parser handles
// delivery addresses and CreditNote payment dates directly, so no second XML
// parse or post-processing is needed here.
func Parse(data []byte) (*einvoice.Invoice, error) {
	return einvoice.ParseReader(bytes.NewReader(data))
}
