package pdfinvoice

import (
	"time"

	"papierleicht.de/e-rechnung-studio/internal/model"
)

// QuoteDetails switch the shared invoice layout to an offer ("Angebot").
// The XML only transports positions, parties and totals into the renderer; it
// is never attached, and no payment request (QR code, due date) is printed.
type QuoteDetails struct {
	ValidUntil time.Time
}

// RenderQuote creates the quote PDF with the same typesetter and logo as invoices.
func RenderQuote(xml []byte, issuer *model.Company, buyer *model.Customer, quote QuoteDetails) ([]byte, error) {
	return renderWithPlan(xml, issuer, buyer, nil, PaymentQROptions{}, &quote)
}

// QuoteHTML is the preview counterpart of RenderQuote with the same page plan.
func QuoteHTML(xml []byte, issuer *model.Company, buyer *model.Customer, quote QuoteDetails) (string, error) {
	return documentHTML(xml, issuer, buyer, PaymentQROptions{}, &quote)
}
