package banking

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	fints "github.com/BergerAPI/go-fints"
)

type diagnosticHTTPFailure struct{}

func (diagnosticHTTPFailure) RoundTrip(*http.Request) (*http.Response, error) {
	return &http.Response{StatusCode: 503, Status: "PRIVATE-STATUS", Header: make(http.Header), Body: io.NopCloser(strings.NewReader("PRIVATE-BODY"))}, nil
}

func TestLocalProtocolFailurePersistsConcreteReasonWithoutHTTPData(t *testing.T) {
	s, _ := testService(t)
	s.factory = func(c fints.Config) (readClient, error) {
		c.HTTPClient = &http.Client{Transport: diagnosticHTTPFailure{}}
		return fints.NewClient(c)
	}
	op, err := s.Connect(testConnection(), "PRIVATE-PIN", false)
	if err != nil {
		t.Fatal(err)
	}
	result := awaitTerminal(t, s, op.ID)
	if result.Status != "error" {
		t.Fatalf("unexpected status: %s", result.Status)
	}
	data, err := os.ReadFile(s.diagnosticPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "PRIVATE") || strings.Contains(result.Message, "PRIVATE") {
		t.Fatal("private HTTP or credential data leaked")
	}
	var d bankDiagnostic
	if err := json.Unmarshal(data, &d); err != nil {
		t.Fatal(err)
	}
	if d.SchemaVersion != 3 || d.Kind != "client_error" || d.Stage != "accounts_and_login" {
		t.Fatalf("missing diagnostic version or category: %#v", d)
	}
	foundHTTP, foundRequest := false, false
	for _, frame := range d.Failures {
		if frame.Reason == "http_status" && frame.HTTPStatus == 503 {
			foundHTTP = true
		}
		if strings.Join(frame.RequestSegments, ",") == "HKIDN,HKVVB,HKSYN" {
			foundRequest = true
		}
	}
	if !foundHTTP || !foundRequest {
		t.Fatalf("missing HTTP/request context: %#v", d.Failures)
	}
	if d.Trace == nil || len(d.Trace.Exchanges) != 1 || d.Trace.Exchanges[0].RequestMessage != 1 || d.Trace.Exchanges[0].RequestDialog != "new" {
		t.Fatalf("missing safe exchange trace: %#v", d.Trace)
	}
}

func TestBankFailureRetainsCodesWithoutPrivateBankText(t *testing.T) {
	s, _ := testService(t)
	const privateText = "PRIVATE-PIN-TAN-USER-ACCOUNT-DIALOG"
	bankErr := &fints.BankError{
		Code: "9800", Message: privateText,
		Details: []fints.BankResponseError{
			{Code: "9800", Message: privateText},
			{Code: "9340", Segment: 4, Message: privateText, ReferenceElement: privateText, Params: []string{privateText}},
			{Code: "9340", Segment: 4},
			{Code: privateText, Segment: -1},
		},
		RequestSegments: []string{"HKIDN", "HKVVB", "HKTAN", "HKTAN", privateText},
		TANResponses:    []fints.ResponseCodeReference{{Code: "3955", Segment: 5}, {Code: privateText}, {Code: "3955", Segment: 5}},
	}
	s.factory = func(c fints.Config) (readClient, error) {
		return &fakeClient{cfg: c, err: fmt.Errorf("private wrapper %s: %w", privateText, bankErr)}, nil
	}
	op, err := s.Connect(testConnection(), privateText, false)
	if err != nil {
		t.Fatal(err)
	}
	result := awaitTerminal(t, s, op.ID)
	if result.Status != "error" || !strings.Contains(result.Message, "Codes 9800, 9340") {
		t.Fatalf("missing secondary bank code: %#v", result)
	}
	data, err := os.ReadFile(s.diagnosticPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), privateText) || strings.Contains(result.Message, privateText) {
		t.Fatal("diagnostic exposed bank text or credentials")
	}
	var diagnostic bankDiagnostic
	if err := json.Unmarshal(data, &diagnostic); err != nil {
		t.Fatal(err)
	}
	if diagnostic.Stage != "accounts_and_login" || diagnostic.Kind != "bank_rejected" ||
		len(diagnostic.Responses) != 2 || diagnostic.Responses[1].Code != "9340" || diagnostic.Responses[1].Segment != 4 ||
		strings.Join(diagnostic.RequestSegments, ",") != "HKIDN,HKVVB,HKTAN" {
		t.Fatalf("unexpected diagnostic: %#v", diagnostic)
	}
	if len(diagnostic.TANResponses) != 1 || diagnostic.TANResponses[0].Code != "3955" || diagnostic.TANResponses[0].Segment != 5 {
		t.Fatalf("unexpected TAN response metadata: %#v", diagnostic.TANResponses)
	}
	if _, err := time.Parse(time.RFC3339Nano, diagnostic.Time); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(s.diagnosticPath); err != nil {
		t.Fatal(err)
	} else if runtime.GOOS != "windows" && info.Mode().Perm() != 0600 {
		t.Fatalf("diagnostic permissions: %o", info.Mode().Perm())
	}

	// A later failure replaces the snapshot; it never accumulates a history.
	s.factory = func(c fints.Config) (readClient, error) {
		return nil, errors.New(privateText)
	}
	op, err = s.Connect(testConnection(), privateText, false)
	if err != nil {
		t.Fatal(err)
	}
	awaitTerminal(t, s, op.ID)
	data, err = os.ReadFile(s.diagnosticPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "9800") || strings.Contains(string(data), privateText) {
		t.Fatal("last-error snapshot retained previous response or raw error")
	}
	leftovers, err := filepath.Glob(filepath.Join(filepath.Dir(s.diagnosticPath), ".banking-diagnostic-*"))
	if err != nil || len(leftovers) != 0 {
		t.Fatalf("temporary diagnostics remain: %v, %v", leftovers, err)
	}
}

func TestBankDiagnosticIsBoundedAndUsesOnlyAllowedMetadata(t *testing.T) {
	bankErr := &fints.BankError{Code: "9800"}
	for i := 0; i < 1000; i++ {
		bankErr.Details = append(bankErr.Details, fints.BankResponseError{Code: fmt.Sprintf("9%03d", i), Segment: 10000})
	}
	diagnostic := makeBankDiagnostic("private-stage", "private-tan-value", bankErr)
	if diagnostic.Stage != "unknown" || diagnostic.TANMethod != "" || len(diagnostic.Responses) != 64 || !diagnostic.Truncated {
		t.Fatalf("diagnostic exceeded allowlist or bound: %#v", diagnostic)
	}
	for _, response := range diagnostic.Responses {
		if response.Segment != 0 {
			t.Fatal("invalid segment reference persisted")
		}
	}
}

func TestDiagnosticWriteFailureKeepsBankFailureVisible(t *testing.T) {
	s, _ := testService(t)
	s.diagnosticPath = filepath.Join(t.TempDir(), "missing", "diagnostic.json")
	s.factory = func(c fints.Config) (readClient, error) {
		return &fakeClient{cfg: c, err: &fints.BankError{Code: "9800"}}, nil
	}
	op, err := s.Connect(testConnection(), "synthetic-pin", false)
	if err != nil {
		t.Fatal(err)
	}
	result := awaitTerminal(t, s, op.ID)
	if result.Status != "error" || !strings.Contains(result.Message, "9800") ||
		!strings.Contains(result.Message, "Fehlerdiagnose konnte nicht gespeichert") {
		t.Fatalf("diagnostic write failure hid bank error: %#v", result)
	}
}
