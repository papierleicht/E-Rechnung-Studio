package banking

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"time"

	fints "github.com/BergerAPI/go-fints"
)

var bankErrorCodePattern = regexp.MustCompile(`^9[0-9]{3}$`)
var diagnosticSegmentPattern = regexp.MustCompile(`^[A-Z]{5,6}$`)
var diagnosticTANPattern = regexp.MustCompile(`^[0-9]{3}$`)
var diagnosticResponseCodePattern = regexp.MustCompile(`^[0-9]{4}$`)

type diagnosticResponse struct {
	Code    string `json:"code"`
	Segment int    `json:"segment,omitempty"`
}

// This is a deliberately small allowlist, not a serialization of an error or
// connection. Bank text, parameters and error strings may contain credentials.
type bankDiagnostic struct {
	SchemaVersion   int                       `json:"schema_version"`
	Time            string                    `json:"time"`
	Stage           string                    `json:"stage"`
	Kind            string                    `json:"kind"`
	TANMethod       string                    `json:"tan_method,omitempty"`
	RequestSegments []string                  `json:"request_segments,omitempty"`
	Responses       []diagnosticResponse      `json:"responses,omitempty"`
	TANResponses    []diagnosticResponse      `json:"tan_responses,omitempty"`
	Truncated       bool                      `json:"responses_truncated,omitempty"`
	Failures        []fints.FailureDiagnostic `json:"failures,omitempty"`
	Trace           *fints.ExchangeTrace      `json:"trace,omitempty"`
}

func diagnosticResponses(bankErr *fints.BankError) ([]diagnosticResponse, bool) {
	var result []diagnosticResponse
	seen := make(map[diagnosticResponse]bool)
	truncated := false
	add := func(code string, segment int) {
		if !bankErrorCodePattern.MatchString(code) {
			return
		}
		if segment < 1 || segment > 999 {
			segment = 0
		}
		response := diagnosticResponse{Code: code, Segment: segment}
		if seen[response] {
			return
		}
		if len(result) == 64 {
			truncated = true
			return
		}
		seen[response] = true
		result = append(result, response)
	}
	add(bankErr.Code, 0)
	for _, detail := range bankErr.Details {
		add(detail.Code, detail.Segment)
	}
	return result, truncated
}

func diagnosticBankCodes(bankErr *fints.BankError) []string {
	responses, _ := diagnosticResponses(bankErr)
	var result []string
	seen := make(map[string]bool)
	for _, response := range responses {
		if !seen[response.Code] {
			result = append(result, response.Code)
			seen[response.Code] = true
		}
	}
	return result
}

func makeBankDiagnostic(stage, tanMethod string, err error) bankDiagnostic {
	d := bankDiagnostic{SchemaVersion: 3, Time: time.Now().UTC().Format(time.RFC3339Nano), Stage: "unknown", Kind: "other", Failures: fints.FailureDiagnostics(err)}
	switch stage {
	case "client_setup", "accounts_and_login", "save_accounts", "tan_methods", "transactions_and_login", "import_bookings", "save_connection":
		d.Stage = stage
	}
	if diagnosticTANPattern.MatchString(tanMethod) {
		d.TANMethod = tanMethod
	}
	var bankErr *fints.BankError
	switch {
	case errors.Is(err, context.Canceled):
		d.Kind = "cancelled"
	case errors.Is(err, context.DeadlineExceeded):
		d.Kind = "timeout"
	case errors.As(err, &bankErr):
		d.Kind = "bank_rejected"
		d.Responses, d.Truncated = diagnosticResponses(bankErr)
		seenResponses := make(map[diagnosticResponse]bool)
		for _, response := range bankErr.TANResponses {
			if !diagnosticResponseCodePattern.MatchString(response.Code) {
				continue
			}
			ref := response.Segment
			if ref < 1 || ref > 999 {
				ref = 0
			}
			item := diagnosticResponse{Code: response.Code, Segment: ref}
			if !seenResponses[item] && len(d.TANResponses) < 64 {
				d.TANResponses = append(d.TANResponses, item)
				seenResponses[item] = true
			}
		}
		seen := make(map[string]bool)
		for _, segment := range bankErr.RequestSegments {
			if diagnosticSegmentPattern.MatchString(segment) && !seen[segment] && len(d.RequestSegments) < 16 {
				d.RequestSegments = append(d.RequestSegments, segment)
				seen[segment] = true
			}
		}
	}
	if d.Kind == "other" && len(d.Failures) > 0 {
		d.Kind = "client_error"
	}
	return d
}

// Keep only the latest failure, replacing it atomically with a private file.
// Diagnostics must never grow into a history of customer banking activity.
func writeBankDiagnostic(path string, diagnostic bankDiagnostic) error {
	data, err := json.MarshalIndent(diagnostic, "", "  ")
	if err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".banking-diagnostic-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	_, writeErr := file.Write(append(data, '\n'))
	closeErr := file.Close()
	if writeErr != nil {
		return writeErr
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(file.Name(), path)
}
