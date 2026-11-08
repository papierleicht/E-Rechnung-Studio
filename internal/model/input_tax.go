package model

// InputTaxStatus is the app's suggestion for a received document: whether it
// goes into the Vorsteuer of the app's figures, and why not. Whether a document
// entitles to the deduction depends on the individual case, which the app does
// not judge. It keeps out documents whose check found something that can matter,
// and the user has the last word (see InputTaxChoice).
type InputTaxStatus struct {
	Allowed bool `json:"allowed"`
	// Blocker is the class of finding that keeps the document out: "format",
	// "business", "hint" or "unchecked". Empty when Allowed.
	Blocker string `json:"blocker,omitempty"`
	// Reason explains the suggestion in a sentence or two for the document view.
	Reason string `json:"reason,omitempty"`
	// Short is a few words for lists and notes.
	Short string `json:"short,omitempty"`
	// Tax is the Vorsteuer the app suggests in the document's currency. It can
	// be less than the tax of the document; empty when the app suggests none.
	Tax string `json:"tax,omitempty"`
	// OnPayment marks a suggestion that counts Tax with the document's payments
	// instead of on its date.
	OnPayment bool `json:"onPayment,omitempty"`
	// Choice is the user's decision. It replaces the suggestion.
	Choice *InputTaxChoice `json:"choice,omitempty"`
}

// Modes of an InputTaxChoice.
const (
	// InputTaxFull counts the whole tax of the document.
	InputTaxFull = "full"
	// InputTaxNone keeps the document out of the Vorsteuer.
	InputTaxNone = "none"
	// InputTaxAmount counts the amount the user gives.
	InputTaxAmount = "amount"
	// InputTaxOnPayment counts the tax with the payments on the document.
	InputTaxOnPayment = "payment"
)

// InputTaxChoice is the user's own decision on how much Vorsteuer of a received
// document goes into the figures.
type InputTaxChoice struct {
	ReceivedID string `json:"receivedId"`
	// Mode is InputTaxFull, InputTaxNone, InputTaxAmount or InputTaxOnPayment.
	Mode string `json:"mode"`
	// Amount is the Vorsteuer in euros for InputTaxAmount.
	Amount string `json:"amount,omitempty"`
	// Note is the user's own reason.
	Note      string `json:"note,omitempty"`
	DecidedAt string `json:"decidedAt,omitempty"`
}
