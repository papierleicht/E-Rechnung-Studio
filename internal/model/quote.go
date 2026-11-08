package model

// Quote is an offer (Angebot) to a customer. Document reuses the invoice draft
// for parties, positions, tax treatment, adjustments and notes, so quotes share
// the invoice calculation. A quote is never an invoice and has no e-invoice XML.
//
// Status is draft, sent, accepted, rejected or converted. "expired" is derived
// from ValidUntil for sent quotes and never stored.
type Quote struct {
	AutomaticNumber bool         `json:"automaticNumber,omitempty"`
	ID              string       `json:"id"`
	Number          string       `json:"number"`
	Status          string       `json:"status"`
	ValidUntil      string       `json:"validUntil"`
	InvoiceID       string       `json:"invoiceId,omitempty"`
	SentAt          string       `json:"sentAt,omitempty"`
	DecidedAt       string       `json:"decidedAt,omitempty"`
	ConvertedAt     string       `json:"convertedAt,omitempty"`
	Document        InvoiceDraft `json:"document"`
	CreatedAt       string       `json:"createdAt"`
	UpdatedAt       string       `json:"updatedAt"`
}

type QuoteSummary struct {
	ID            string `json:"id"`
	Number        string `json:"number"`
	CustomerID    string `json:"customerId"`
	CustomerName  string `json:"customerName"`
	IssueDate     string `json:"issueDate"`
	ValidUntil    string `json:"validUntil"`
	Status        string `json:"status"`
	Gross         string `json:"gross"`
	InvoiceID     string `json:"invoiceId"`
	InvoiceNumber string `json:"invoiceNumber"`
	UpdatedAt     string `json:"updatedAt"`
	// MailedAt is the latest successful e-mail delivery (empty if none).
	MailedAt string `json:"mailedAt,omitempty"`
}

type QuoteView struct {
	Quote      Quote            `json:"quote"`
	Company    Company          `json:"company"`
	Totals     Totals           `json:"totals"`
	LineTotals []string         `json:"lineTotals"`
	Validation ValidationResult `json:"validation"`
	// InvoiceNumber names the linked invoice (or its draft number) for display.
	InvoiceNumber string `json:"invoiceNumber,omitempty"`
}

// EffectiveQuoteStatus reports a sent quote past its validity as expired.
// Dates use the ISO format, so string comparison orders them correctly.
func EffectiveQuoteStatus(status, validUntil, today string) string {
	if status == "sent" && validUntil != "" && validUntil < today {
		return "expired"
	}
	return status
}
