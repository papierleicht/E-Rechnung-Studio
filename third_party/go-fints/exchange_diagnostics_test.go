package fints

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestTraceRetainsInitializationBeforeUnexpectedAccountRequest(t *testing.T) {
	const private = "PRIVATE-DIALOG-REFERENCE-CHALLENGE"
	config := testConfig(mockHTTPClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		raw, _ := base64.StdEncoding.DecodeString(string(body))
		request, err := parseSegments(raw)
		if err != nil {
			t.Fatal(err)
		}
		var reply string
		switch {
		case len(findSegments(request, "HKIDN")) > 0:
			reply = "HIRMG:2:2+0010::" + private + "'HIRMS:3:2:3+3076::" + private + "'HITAN:4:7:5+4++noref+nochallenge'"
		case len(findSegments(request, "HKSPA")) > 0:
			reply = "HIRMG:2:2+9800::" + private + "+9120::" + private + "'"
		case len(findSegments(request, "HKEND")) > 0:
			reply = "HIRMG:2:2+0100::" + private + "'"
		default:
			t.Fatal("unexpected request")
		}
		_, _ = w.Write([]byte(base64.StdEncoding.EncodeToString([]byte(responseMessage(private, reply)))))
	})))
	config.SecurityFunction, config.SystemID = "901", "PRIVATE-SYSTEM"
	config.PIN, config.UserID = "PRIVATE-PIN", "PRIVATE-USER"
	client, err := NewClient(config)
	if err != nil {
		t.Fatal(err)
	}
	client.parametersReady = true
	_, err = client.Accounts(context.Background())
	var bankErr *BankError
	if !errors.As(err, &bankErr) || !bankErrorHasCode(err, "9120") {
		t.Fatalf("expected original 9120: %v", err)
	}
	trace := client.DiagnosticTrace()
	if len(trace.Exchanges) != 3 {
		t.Fatalf("exchange count: %d", len(trace.Exchanges))
	}
	initial, failed := trace.Exchanges[0], trace.Exchanges[1]
	if initial.RequestMessage != 1 || initial.ResponseProtocol != 300 || initial.RequestDialog != "new" || initial.ResponseDialog != "assigned_or_changed" || initial.TANMethod != "901" {
		t.Fatalf("missing initialization metadata: %#v", initial)
	}
	if len(initial.RequestSegments) != 3 || initial.RequestSegments[2].Type != "HKTAN" || initial.RequestSegments[2].Number != 5 || initial.RequestSegments[2].TANProcess != "4" || len(initial.TAN) != 1 || initial.TAN[0].TaskReferenceState != "placeholder" {
		t.Fatalf("missing TAN context: %#v", initial)
	}
	if failed.RequestMessage != 2 || failed.ResponseDialog != "same" || failed.RequestSegments[0].Type != "HKSPA" || failed.FailureReason != "bank_rejected" || len(failed.Responses) != 2 || failed.Responses[1].Code != "9120" {
		t.Fatalf("missing failed-request context: %#v", failed)
	}
	encoded, _ := json.Marshal(trace)
	if strings.Contains(string(encoded), "PRIVATE") || strings.Contains(string(encoded), "noref") || strings.Contains(string(encoded), "nochallenge") {
		t.Fatal("raw bank values leaked")
	}
	trace.Exchanges[0].RequestSegments[0].Type = private
	trace.Exchanges[0].ResponseSegments[0].Type = private
	trace.Exchanges[0].TAN[0].ChallengeState = private
	trace.Exchanges[0].Responses[0].Code = private
	encoded, _ = json.Marshal(client.DiagnosticTrace())
	if strings.Contains(string(encoded), private) {
		t.Fatal("snapshot mutated internal trace")
	}
}

func TestTraceRetainsStartAndEndWithinMemoryLimit(t *testing.T) {
	client, err := NewClient(testConfig(&http.Client{}))
	if err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 80; i++ {
		client.recordExchange(&dialog{}, "0", i, []string{"HKTAN:{n}:7+S++++PRIVATE-TASK+N'"}, nil, nil)
	}
	trace := client.DiagnosticTrace()
	if len(trace.Exchanges) != 64 || trace.Dropped != 16 || trace.Exchanges[15].Sequence != 16 || trace.Exchanges[16].Sequence != 33 || trace.Exchanges[63].Sequence != 80 {
		t.Fatalf("trace retention wrong: len=%d dropped=%d", len(trace.Exchanges), trace.Dropped)
	}
	encoded, _ := json.Marshal(trace)
	if strings.Contains(string(encoded), "PRIVATE") {
		t.Fatal("task reference leaked")
	}
}
