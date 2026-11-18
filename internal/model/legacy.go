package model

// LegacyDocument is a document taken over from another program during a move
// ("Umzug"). It is archived exactly as delivered and never changes; it does not
// count in the figures of this app (sales tax, results, open items).
type LegacyDocument struct {
	ID string `json:"id"`
	// Source is the program it came from, e.g. "invoiz" or "andere".
	Source string `json:"source"`
	// SourceID identifies the document within the source; for files it is the
	// SHA-256 of the main file.
	SourceID string `json:"sourceId"`
	// Direction is "outgoing" (own invoice), "incoming" (received) or "" when
	// the file does not say.
	Direction string `json:"direction"`
	// Kind is "invoice", "credit-note", "document" for a file without
	// structured invoice data, or "export" for a whole DATEV or GDPdU export.
	Kind     string `json:"kind"`
	Number   string `json:"number"`
	Date     string `json:"date"`
	Partner  string `json:"partner"`
	Currency string `json:"currency"`
	Net      string `json:"net"`
	Tax      string `json:"tax"`
	Gross    string `json:"gross"`
	// EInvoice says that the data above come from an embedded or separate
	// ZUGFeRD/XRechnung XML, not from a guess.
	EInvoice bool `json:"eInvoice"`
	// Basis says where number, date and amount come from: "einvoice" (the
	// XML of the file), "datev" (the export's record of the receipt) or ""
	// when the file says nothing the app can read.
	Basis string `json:"basis,omitempty"`
	// Origin is where the file was found, e.g. "export.zip/2024/RE-1.pdf".
	Origin     string       `json:"origin"`
	ImportedAt string       `json:"importedAt"`
	Files      []LegacyFile `json:"files"`
}

// LegacyFile is one original file of a legacy document. Data is only filled
// when the bytes are asked for.
type LegacyFile struct {
	Name   string `json:"name"`
	MIME   string `json:"mime"`
	SHA256 string `json:"sha256"`
	Size   int    `json:"size"`
	Data   []byte `json:"-"`
}

// LegacyYear sums up the archive for one calendar year and direction, so the
// customer can compare it with the reports of the old program.
type LegacyYear struct {
	Year      string `json:"year"`
	Direction string `json:"direction"`
	Count     int    `json:"count"`
	// Structured counts the documents with e-invoice data; only they have amounts.
	Structured int    `json:"structured"`
	Net        string `json:"net"`
	Tax        string `json:"tax"`
	Gross      string `json:"gross"`
}
