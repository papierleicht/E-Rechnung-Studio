package fints

import (
	"context"
	"encoding/base64"
	"errors"
	"io"
	"net/http"
	"testing"
	"time"
)

func TestAccountsPreservesBankErrorWithoutUPDFallback(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		raw, err := base64.StdEncoding.DecodeString(string(body))
		if err != nil {
			t.Fatal(err)
		}
		segments, err := parseSegments(raw)
		if err != nil {
			t.Fatal(err)
		}
		inner := ""
		switch {
		case len(findSegments(segments, "HKEND")) > 0:
			inner = "HIRMG:2:2+0100::Ended'"
		case len(findSegments(segments, "HKIDN")) > 0:
			inner = "HIRMG:2:2+0010::Accepted'HIRMS:3:2:3+3076::No SCA'HITAN:4:7:5+4++noref+nochallenge'"
		case len(findSegments(segments, "HKSPA")) > 0:
			inner = "HIRMG:2:2+0010::Accepted'HIRMS:3:2:4+3955::Approval required'HITAN:4:7:4+4++test-task+Approve'"
		case hktanProcess(segments) == "S":
			inner = "HIRMG:2:2+9800::Aborted'HIRMS:3:2:3+9010::Cannot process'"
		default:
			t.Fatalf("unexpected request")
		}
		payload := responseMessage("dialog-accounts-test", inner)
		_, _ = w.Write([]byte(base64.StdEncoding.EncodeToString([]byte(payload))))
	})
	config := testConfig(mockHTTPClient(handler))
	config.SystemID = "test-system"
	config.SecurityFunction = "901"
	config.HKTANVersion = 7
	config.AutoPoll = true
	config.PollInterval = time.Millisecond
	client, err := NewClient(config)
	if err != nil {
		t.Fatal(err)
	}
	client.parametersReady = true
	client.tanRequirements["HKSPA"] = true
	_, err = client.Accounts(context.Background())
	var bankErr *BankError
	if !errors.As(err, &bankErr) {
		t.Fatalf("Accounts error = %v, want original BankError", err)
	}
	if bankErr.Code != "9800" || len(bankErr.Details) != 2 || bankErr.Details[1].Code != "9010" {
		t.Fatalf("bank response lost: %#v", bankErr)
	}
	if len(bankErr.RequestSegments) != 1 || bankErr.RequestSegments[0] != "HKTAN" {
		t.Fatalf("request context lost: %#v", bankErr.RequestSegments)
	}
	found := false
	for _, response := range bankErr.TANResponses {
		if response.Code == "3955" && response.Segment == 4 {
			found = true
		}
	}
	if !found {
		t.Fatalf("TAN context lost: %#v", bankErr.TANResponses)
	}
}
