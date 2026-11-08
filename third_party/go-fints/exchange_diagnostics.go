package fints

import (
	"strconv"
	"strings"
)

// ExchangeDiagnostic records protocol structure, never message contents. It
// includes successful responses preceding a failure so authentication state
// can be investigated without recording any account or session identifiers.
type ExchangeDiagnostic struct {
	Sequence          int                     `json:"sequence"`
	RequestMessage    int                     `json:"request_message,omitempty"`
	ResponseMessage   int                     `json:"response_message,omitempty"`
	ResponseReference int                     `json:"response_reference,omitempty"`
	ResponseProtocol  int                     `json:"response_protocol,omitempty"`
	RequestDialog     string                  `json:"request_dialog"`
	ResponseDialog    string                  `json:"response_dialog"`
	TANMethod         string                  `json:"tan_method,omitempty"`
	RequestSegments   []SegmentDiagnostic     `json:"request_segments,omitempty"`
	ResponseSegments  []SegmentDiagnostic     `json:"response_segments,omitempty"`
	Responses         []ResponseCodeReference `json:"responses,omitempty"`
	TAN               []TANResponseDiagnostic `json:"tan,omitempty"`
	FailureReason     string                  `json:"failure_reason,omitempty"`
}

type SegmentDiagnostic struct {
	Type       string `json:"type"`
	Number     int    `json:"number,omitempty"`
	Version    int    `json:"version,omitempty"`
	Reference  int    `json:"reference,omitempty"`
	TANProcess string `json:"tan_process,omitempty"`
}

type ExchangeTrace struct {
	Exchanges []ExchangeDiagnostic `json:"exchanges,omitempty"`
	Dropped   int                  `json:"dropped,omitempty"`
}

// DiagnosticTrace returns a copy of the bounded metadata for this Client.
// Application code must not call it from a TAN callback (Client is locked).
func (c *Client) DiagnosticTrace() ExchangeTrace {
	c.mu.Lock()
	defer c.mu.Unlock()
	trace := ExchangeTrace{Dropped: c.traceDropped, Exchanges: append([]ExchangeDiagnostic(nil), c.trace...)}
	for i := range trace.Exchanges {
		e := &trace.Exchanges[i]
		e.RequestSegments = append([]SegmentDiagnostic(nil), e.RequestSegments...)
		e.ResponseSegments = append([]SegmentDiagnostic(nil), e.ResponseSegments...)
		e.Responses = append([]ResponseCodeReference(nil), e.Responses...)
		e.TAN = append([]TANResponseDiagnostic(nil), e.TAN...)
	}
	return trace
}

func (c *Client) recordExchange(d *dialog, requestDialog string, message int, commands []string, response []segment, err error) {
	safe := responseDiagnostic(commands, response)
	e := ExchangeDiagnostic{Sequence: len(c.trace) + c.traceDropped + 1, RequestMessage: safeNumber(message), RequestDialog: "existing", ResponseDialog: "missing", Responses: safe.Responses, TAN: safe.TAN}
	if requestDialog == "" || requestDialog == "0" {
		e.RequestDialog = "new"
	}
	if id := responseDialogID(response); id != "" && id != "0" {
		if id == requestDialog {
			e.ResponseDialog = "same"
		} else {
			e.ResponseDialog = "assigned_or_changed"
		}
	}
	if len(d.security) == 3 && strings.Trim(d.security, "0123456789") == "" {
		e.TANMethod = d.security
	}
	if err != nil {
		e.FailureReason = failureReason(err)
	}
	for i, command := range commands {
		if len(e.RequestSegments) == 16 {
			break
		}
		header, _, _ := strings.Cut(command, "+")
		parts := strings.Split(strings.TrimSuffix(header, "'"), ":")
		if len(parts) < 3 || !diagnosticSegmentType(parts[0]) {
			continue
		}
		version, _ := strconv.Atoi(parts[2])
		s := SegmentDiagnostic{Type: parts[0], Number: safeNumber(3 + i), Version: safeNumber(version)}
		if parts[0] == "HKTAN" {
			_, fields, _ := strings.Cut(command, "+")
			process, _, _ := strings.Cut(fields, "+")
			s.TANProcess = safeTANProcess(strings.TrimSuffix(process, "'"))
		}
		e.RequestSegments = append(e.RequestSegments, s)
	}
	for _, s := range response {
		if s.Header.Type == "HNHBK" {
			e.ResponseProtocol = diagnosticInt(component(s, 1, 0))
			e.ResponseMessage = diagnosticInt(component(s, 3, 0))
			e.ResponseReference = diagnosticInt(component(s, 4, 1))
		}
		if len(e.ResponseSegments) < 64 && diagnosticSegmentType(s.Header.Type) {
			e.ResponseSegments = append(e.ResponseSegments, SegmentDiagnostic{Type: s.Header.Type, Number: safeNumber(s.Header.Number), Version: safeNumber(s.Header.Version), Reference: safeNumber(s.Header.Reference)})
		}
	}
	// Retain initialization plus recent exchanges even after long polling.
	if len(c.trace) == 64 {
		copy(c.trace[16:], c.trace[17:])
		c.trace = c.trace[:63]
		c.traceDropped++
	}
	c.trace = append(c.trace, e)
}

func diagnosticSegmentType(value string) bool {
	if len(value) < 5 || len(value) > 6 {
		return false
	}
	for _, c := range value {
		if c < 'A' || c > 'Z' {
			return false
		}
	}
	return true
}

func safeTANProcess(value string) string {
	switch value {
	case "1", "2", "3", "4", "S":
		return value
	}
	return ""
}

func safeNumber(value int) int {
	if value >= 1 && value <= 999 {
		return value
	}
	return 0
}
func diagnosticInt(value string) int { v, _ := strconv.Atoi(value); return safeNumber(v) }
