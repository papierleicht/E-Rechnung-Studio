package fints

import "errors"

func tanErrorWithResponse(err error, response []segment) error {
	var bankErr *BankError
	if !errors.As(err, &bankErr) {
		return err
	}
	copyError := *bankErr
	copyError.TANResponses = nil
	seen := make(map[ResponseCodeReference]bool)
	for _, s := range response {
		if s.Header.Type != "HIRMG" && s.Header.Type != "HIRMS" {
			continue
		}
		for i := range s.Fields {
			code := component(s, i, 0)
			if len(code) != 4 {
				continue
			}
			valid := true
			for _, ch := range code {
				if ch < '0' || ch > '9' {
					valid = false
				}
			}
			if !valid {
				continue
			}
			ref := s.Header.Reference
			if ref < 1 || ref > 999 {
				ref = 0
			}
			item := ResponseCodeReference{Code: code, Segment: ref}
			if !seen[item] && len(copyError.TANResponses) < 64 {
				copyError.TANResponses = append(copyError.TANResponses, item)
				seen[item] = true
			}
		}
	}
	return &tanDiagnosticError{cause: err, bank: &copyError}
}

// Preserve the original error chain and contextual message while exposing the
// enriched bank error through errors.As.
type tanDiagnosticError struct {
	cause error
	bank  *BankError
}

func (e *tanDiagnosticError) Error() string { return e.cause.Error() }
func (e *tanDiagnosticError) Unwrap() error { return e.cause }
func (e *tanDiagnosticError) As(target any) bool {
	if out, ok := target.(**BankError); ok {
		*out = e.bank
		return true
	}
	return false
}
