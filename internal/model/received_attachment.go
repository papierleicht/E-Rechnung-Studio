package model

// ReceivedAttachment is one supporting document (BG-24) of a received e-invoice:
// a file embedded in the XML, a link to an external location or only a
// reference. The bytes stay in the stored original.
type ReceivedAttachment struct {
	// Index is the position in the invoice, the key for opening and saving.
	Index int `json:"index"`
	// Kind is "file" (BT-125), "link" (BT-124) or "reference" (BT-122 only).
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Reference string `json:"reference"`
	// Filename is safe to offer in a save dialog.
	Filename string `json:"filename"`
	MimeType string `json:"mimeType"`
	Size     int    `json:"size"`
	Link     string `json:"link"`
	// Openable says the app displays the file with another program.
	Openable bool `json:"openable"`
}
