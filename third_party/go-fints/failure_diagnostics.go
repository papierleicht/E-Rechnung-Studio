package fints

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
)

// FailureDiagnostic contains protocol structure only. It never includes wire
// data, error strings, identifiers, task references, or challenge contents.
type FailureDiagnostic struct {
	Step            string                  `json:"step"`
	Reason          string                  `json:"reason,omitempty"`
	HTTPStatus      int                     `json:"http_status,omitempty"`
	RequestSegments []string                `json:"request_segments,omitempty"`
	Responses       []ResponseCodeReference `json:"responses,omitempty"`
	TAN             []TANResponseDiagnostic `json:"tan,omitempty"`
}

type TANResponseDiagnostic struct {
	Version            int    `json:"version"`
	Reference          int    `json:"segment,omitempty"`
	Process            string `json:"process,omitempty"`
	TaskReferenceState string `json:"task_reference_state"`
	ChallengeState     string `json:"challenge_state"`
}

// The diagnostic wrapper is private: callers cannot inject arbitrary text
// into a snapshot by constructing an error with public metadata fields.
type failureError struct {
	cause      error
	diagnostic FailureDiagnostic
}

func (e *failureError) Error() string { return e.cause.Error() }
func (e *failureError) Unwrap() error { return e.cause }

// FailureDiagnostics returns up to 12 context frames, outermost first. Errors
// from other libraries are never converted to text for logging.
func FailureDiagnostics(err error) []FailureDiagnostic {
	var frames []FailureDiagnostic
	for len(frames) < 12 {
		var failure *failureError
		if !errors.As(err, &failure) {
			break
		}
		d := failure.diagnostic
		d.Responses = append([]ResponseCodeReference(nil), d.Responses...)
		d.RequestSegments = append([]string(nil), d.RequestSegments...)
		d.TAN = append([]TANResponseDiagnostic(nil), d.TAN...)
		frames = append(frames, d)
		err = failure.cause
	}
	return frames
}

func failureWithResponse(err error, step, reason string, commands []string, response []segment) error {
	if err == nil {
		return nil
	}
	if reason == "" {
		reason = failureReason(err)
	}
	d := responseDiagnostic(commands, response)
	d.Step, d.Reason = step, reason
	return &failureError{cause: err, diagnostic: d}
}

func responseDiagnostic(commands []string, response []segment) FailureDiagnostic {
	d := FailureDiagnostic{}
	d.RequestSegments = bankErrorWithRequestSegments(&BankError{}, commands).RequestSegments
	// Reuse the strict numeric allowlist from TAN diagnostics.
	var enriched *BankError
	errors.As(tanErrorWithResponse(&BankError{}, response), &enriched)
	d.Responses = enriched.TANResponses
	for _, s := range findSegments(response, "HITAN") {
		if len(d.TAN) == 8 {
			break
		}
		t := TANResponseDiagnostic{TaskReferenceState: "present", ChallengeState: "present"}
		if s.Header.Version >= 1 && s.Header.Version <= 99 {
			t.Version = s.Header.Version
		}
		if s.Header.Reference > 0 && s.Header.Reference <= 999 {
			t.Reference = s.Header.Reference
		}
		switch process := component(s, 0, 0); process {
		case "1", "2", "3", "4", "S":
			t.Process = process
		}
		switch component(s, 2, 0) {
		case "":
			t.TaskReferenceState = "missing"
		case "noref":
			t.TaskReferenceState = "placeholder"
		}
		switch component(s, 3, 0) {
		case "":
			t.ChallengeState = "missing"
		case "nochallenge":
			t.ChallengeState = "placeholder"
		}
		d.TAN = append(d.TAN, t)
	}
	return d
}

func failureReason(err error) string {
	var bank *BankError
	var medium *TANMediumRequiredError
	var dns *net.DNSError
	var cert *tls.CertificateVerificationError
	var network net.Error
	switch {
	case errors.Is(err, context.Canceled):
		return "cancelled"
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case errors.As(err, &bank):
		return "bank_rejected"
	case errors.As(err, &medium):
		return "tan_medium_required"
	case errors.Is(err, ErrTANRequired):
		return "tan_required"
	case errors.As(err, &dns):
		return "dns_error"
	case errors.As(err, &cert):
		return "tls_certificate_error"
	case errors.As(err, &network):
		if network.Timeout() {
			return "timeout"
		}
		return "network_error"
	default:
		return "local_error"
	}
}

func httpFailure(err error, status int) error {
	failure := &failureError{cause: err, diagnostic: FailureDiagnostic{Step: "http", Reason: "http_status"}}
	if status >= 100 && status <= 599 {
		failure.diagnostic.HTTPStatus = status
	}
	return failure
}
