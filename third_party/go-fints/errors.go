package fints

import (
	"errors"
	"fmt"
	"strings"
)

var (
	// ErrTANRequired is returned when a bank requests a second factor but no
	// TAN handler or automatic decoupled polling was configured.
	ErrTANRequired = errors.New("TAN authentication requires user interaction")
	// ErrUnsupportedTAN is kept as a compatibility alias.
	ErrUnsupportedTAN = ErrTANRequired
)

// BankError is a 9xxx FinTS response returned by the bank.
type BankError struct {
	Code    string              `json:"code"`
	Message string              `json:"message"`
	Details []BankResponseError `json:"details,omitempty"`
	// RequestSegments contains only local command names, never command data.
	RequestSegments []string `json:"request_segments,omitempty"`
	// TANResponses contains only codes and segment references preceding a failed
	// TAN continuation, never challenge text or task references.
	TANResponses []ResponseCodeReference `json:"tan_responses,omitempty"`
}

type ResponseCodeReference struct {
	Code    string `json:"code"`
	Segment int    `json:"segment,omitempty"`
}

func (e *BankError) Error() string {
	primary := BankResponseError{Code: e.Code, Message: e.Message}
	for _, detail := range e.Details {
		if detail.Code == e.Code && detail.Message == e.Message {
			primary = detail
			break
		}
	}
	message := "FinTS bank error " + formatBankResponse(primary)
	var details []string
	for _, detail := range e.Details {
		if detail.Code == e.Code && detail.Message == e.Message {
			continue
		}
		details = append(details, formatBankResponse(detail))
	}
	if len(details) > 0 {
		message += "; " + strings.Join(details, "; ")
	}
	return message
}

func formatBankResponse(detail BankResponseError) string {
	value := detail.Code + ": " + detail.Message
	var location []string
	if detail.Segment != 0 {
		location = append(location, fmt.Sprintf("segment %d", detail.Segment))
	}
	if detail.ReferenceElement != "" {
		location = append(location, "element "+detail.ReferenceElement)
	}
	if len(location) > 0 {
		value += " (" + strings.Join(location, ", ") + ")"
	}
	if len(detail.Params) > 0 {
		value += " [" + strings.Join(detail.Params, ", ") + "]"
	}
	return value
}

// BankResponseError is one 9xxx response from HIRMG or HIRMS. Segment is the
// request segment referenced by HIRMS, when supplied by the bank.
type BankResponseError struct {
	Code             string   `json:"code"`
	Message          string   `json:"message"`
	Segment          int      `json:"segment,omitempty"`
	ReferenceElement string   `json:"reference_element,omitempty"`
	Params           []string `json:"params,omitempty"`
}

// TANRequiredError records the challenge information returned by the bank.
type TANRequiredError struct {
	// Details is presentation metadata, not a resumable dialog token. Configure a
	// TANHandler to answer a challenge without abandoning the current dialog.
	Details   *TANChallenge `json:"details,omitempty"`
	Code      string        `json:"code"`
	Challenge string        `json:"challenge,omitempty"`
}

func (e *TANRequiredError) Error() string {
	parts := []string{ErrTANRequired.Error()}
	if e.Code != "" {
		parts = append(parts, "response "+e.Code)
	}
	if e.Challenge != "" {
		parts = append(parts, e.Challenge)
	}
	return strings.Join(parts, ": ")
}

func (e *TANRequiredError) Unwrap() error { return ErrTANRequired }

// TANMediumRequiredError reports the active media from which the caller must
// choose before retrying.
type TANMediumRequiredError struct {
	Media []TANMedium
}

func (e *TANMediumRequiredError) Error() string {
	var names []string
	for _, medium := range e.Media {
		if medium.Name != "" {
			names = append(names, medium.Name)
		}
	}
	if len(names) > 0 {
		return "a TAN medium must be selected from: " + strings.Join(names, ", ")
	}
	return "a TAN medium must be selected"
}
