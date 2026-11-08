package fints

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
)

func TestTANFailureRetainsOnlyInitialCodesAndPreservesErrorChain(t *testing.T) {
	response, err := parseSegments([]byte("HIRMS:2:2:5+3955::PRIVATE-TEXT:PRIVATE-PARAM+3076::PRIVATE-TEXT'HITAN:3:7:5+4++PRIVATE-REF+PRIVATE-CHALLENGE'"))
	if err != nil {
		t.Fatal(err)
	}
	original := &BankError{Code: "9800"}
	wrapped := fmt.Errorf("poll attempt 1: %w", original)
	got := tanErrorWithResponse(wrapped, response)
	var bankErr *BankError
	if !errors.As(got, &bankErr) || !errors.Is(got, original) || got.Error() != wrapped.Error() {
		t.Fatal("error chain or context lost")
	}
	want := []ResponseCodeReference{{Code: "3955", Segment: 5}, {Code: "3076", Segment: 5}}
	if !reflect.DeepEqual(bankErr.TANResponses, want) || original.TANResponses != nil {
		t.Fatalf("unexpected response metadata: %#v", bankErr.TANResponses)
	}
	data, _ := json.Marshal(bankErr.TANResponses)
	if strings.Contains(string(data), "PRIVATE") {
		t.Fatal("private response fields leaked")
	}
	if tanErrorWithResponse(nil, response) != nil {
		t.Fatal("nil error changed")
	}
}

func TestBankErrorCarriesOnlySentSegmentNames(t *testing.T) {
	const pin = "synthetic-pin-value"
	const tan = "synthetic-tan-value"
	const userID = "synthetic-user-value"
	const customerID = "synthetic-customer-value"
	const systemID = "synthetic-system-value"
	const medium = "synthetic-medium-value"
	client, err := NewClient(testConfig(&http.Client{
		Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			// This transport never contacts a bank. Echo synthetic credentials to
			// also verify that adding metadata preserves the existing redaction.
			body := responseMessage("synthetic-dialog", "HIRMG:2:2+9800::Dialog aborted'"+
				"HIRMS:3:2:3+9340::Invalid signature "+pin+":"+tan+"'")
			return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header),
				Body: io.NopCloser(strings.NewReader(base64.StdEncoding.EncodeToString([]byte(body))))}, nil
		}),
	}))
	if err != nil {
		t.Fatal(err)
	}
	client.config.PIN = pin
	client.config.UserID = userID
	client.config.CustomerID = customerID
	commands := []string{
		"HKIDN:{n}:2+280:12345678+" + customerID + "+" + systemID + "+1'",
		"HKVVB:{n}:3+0+0+0+synthetic-product+1.0'",
		"HKTAN:{n}:7+4+HKIDN++++++" + medium + "'",
	}
	_, err = client.sendCommandsWithTAN(context.Background(), &dialog{
		id: "0", nextMessage: 1, systemID: systemID, security: "901",
	}, commands, tan)
	var bankErr *BankError
	if !errors.As(err, &bankErr) {
		t.Fatalf("expected typed bank error, got %T", err)
	}
	if bankErr.Code != "9800" || len(bankErr.Details) != 2 || bankErr.Details[1].Code != "9340" || bankErr.Details[1].Segment != 3 {
		t.Fatalf("bank response details were not preserved: %#v", bankErr)
	}
	if !reflect.DeepEqual(bankErr.RequestSegments, []string{"HKIDN", "HKVVB", "HKTAN"}) {
		t.Fatalf("unexpected request segments: %v", bankErr.RequestSegments)
	}
	metadata, err := json.Marshal(bankErr.RequestSegments)
	if err != nil {
		t.Fatal(err)
	}
	for _, private := range []string{pin, tan, userID, customerID, systemID, medium, "12345678", "synthetic-product", "{n}", ":"} {
		if strings.Contains(string(metadata), private) {
			t.Fatalf("request metadata contains command data %q", private)
		}
	}
	if strings.Contains(bankErr.Error(), pin) || strings.Contains(bankErr.Error(), tan) {
		t.Fatal("existing PIN/TAN redaction was lost")
	}
	wrapped := fmt.Errorf("authenticate FinTS dialog: %w", bankErr)
	var recovered *BankError
	if !errors.As(wrapped, &recovered) || recovered != bankErr || !errors.Is(wrapped, bankErr) {
		t.Fatal("bank error is no longer accessible through normal Go error wrapping")
	}
}

func TestBankRequestSegmentMetadataIsFilteredBoundedAndIndependent(t *testing.T) {
	original := &BankError{
		Code: "9800", Message: "Original bank text",
		Details:         []BankResponseError{{Code: "9340", Message: "Original detail"}},
		RequestSegments: []string{"OLDXX"},
	}
	commands := []string{
		"HKIDN:{n}:2+private-value'", "HKIDN:4:2+another-value'",
		"HKKAZ:{n}:7+private-value'", "ABCDEF:{n}:1+private-value'",
		"HKIDN", "HKID:{n}:1'", "HKIDNXX:{n}:1'", "hkidn:{n}:1'",
		"HK1DN:{n}:1'", "HKÄDN:{n}:1'", "HK\nDN:{n}:1'", " HKIDN:{n}:1'",
	}
	got := bankErrorWithRequestSegments(original, commands)
	if got == original || !reflect.DeepEqual(got.RequestSegments, []string{"HKIDN", "HKKAZ", "ABCDEF"}) {
		t.Fatalf("unexpected copied metadata: %#v", got)
	}
	if got.Code != original.Code || got.Message != original.Message || !reflect.DeepEqual(got.Details, original.Details) {
		t.Fatal("adding request metadata changed bank response details")
	}
	got.RequestSegments[0] = "OTHER"
	if !reflect.DeepEqual(original.RequestSegments, []string{"OLDXX"}) {
		t.Fatal("adding or modifying metadata mutated the shared original error")
	}
	var many []string
	for index := 0; index < 22; index++ {
		many = append(many, fmt.Sprintf("HKAA%c:{n}:1+private-value'", 'A'+index))
	}
	bounded := bankErrorWithRequestSegments(original, many)
	if len(bounded.RequestSegments) != 16 || bounded.RequestSegments[0] != "HKAAA" || bounded.RequestSegments[15] != "HKAAP" {
		t.Fatalf("request metadata limit was not applied: %v", bounded.RequestSegments)
	}
	if got := bankErrorWithRequestSegments(original, []string{"no-header"}); got.RequestSegments != nil {
		t.Fatalf("invalid commands retained old metadata: %v", got.RequestSegments)
	}
}
