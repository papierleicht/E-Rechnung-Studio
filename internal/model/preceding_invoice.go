package model

// PrecedingInvoice is an invoice that a received document refers to (BG-3), for
// example the invoice a correction replaces.
type PrecedingInvoice struct {
	// Number is the invoice number (BT-25).
	Number string `json:"number"`
	// Date is the issue date (BT-26) as YYYY-MM-DD, empty if the document names none.
	Date string `json:"date,omitempty"`
}
