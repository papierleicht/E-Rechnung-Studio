package model

// ReceivedInvoice keeps imported documents separate from issued invoices and counters.
type ReceivedInvoice struct {
	// ReceiptNumber is assigned once on import and never changes.
	ReceiptNumber string `json:"receiptNumber,omitempty"`
	// Locked explains why the document can no longer change; it is worked out
	// when listing and never stored.
	Locked     string           `json:"locked,omitempty"`
	Status     string           `json:"status"`
	History    []ReceivedEvent  `json:"history"`
	ID         string           `json:"id"`
	Filename   string           `json:"filename"`
	Kind       string           `json:"kind"`
	ImportedAt string           `json:"importedAt"`
	Size       int              `json:"size"`
	Inspection ImportInspection `json:"inspection"`
}

// History documents normal app actions; the change log is the tamper-evident record.
type ReceivedEvent struct {
	Action string `json:"action"`
	Reason string `json:"reason"`
	At     string `json:"at"`
}

type ReceivedPreview struct {
	Inspection   ImportInspection `json:"inspection"`
	PDF          string           `json:"pdf"`
	PreviewError string           `json:"previewError"`
	Duplicate    *ReceivedInvoice `json:"duplicate"`
}

type ReceivedImport struct {
	Invoice   ReceivedInvoice `json:"invoice"`
	Duplicate bool            `json:"duplicate"`
}
