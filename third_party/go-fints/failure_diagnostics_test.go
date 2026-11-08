package fints

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestMissingTANReferenceKeepsSafeDiagnosticWithoutCallingBank(t *testing.T) {
	const private = "PRIVATE-ACCOUNT-PIN-TAN"
	response, err := parseSegments([]byte("HIRMS:2:2:5+3955::" + private + ":" + private + "'HITAN:3:7:5+4++noref+" + private + "'"))
	if err != nil {
		t.Fatal(err)
	}
	client, err := NewClient(testConfig(&http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("malformed challenge must not send a continuation")
		return nil, nil
	})}))
	if err != nil {
		t.Fatal(err)
	}
	client.config.TANHandler = func(context.Context, TANChallenge) (string, error) {
		t.Fatal("malformed challenge must not prompt")
		return "", nil
	}
	_, err = client.completeTAN(context.Background(), &dialog{}, response)
	frames := FailureDiagnostics(fmt.Errorf("private wrapper %s: %w", private, err))
	if len(frames) != 2 || frames[1].Reason != "missing_tan_reference" || len(frames[1].Responses) != 1 || frames[1].Responses[0].Code != "3955" || len(frames[1].TAN) != 1 || frames[1].TAN[0].TaskReferenceState != "placeholder" {
		t.Fatalf("missing challenge diagnostics: %#v", frames)
	}
	data, _ := json.Marshal(frames)
	if strings.Contains(string(data), private) || strings.Contains(string(data), "noref") {
		t.Fatal("raw bank data leaked")
	}
	frames[0].TAN[0].TaskReferenceState = private
	frames[0].Responses[0].Code = private
	data, _ = json.Marshal(FailureDiagnostics(err))
	if strings.Contains(string(data), private) {
		t.Fatal("diagnostic snapshot aliases internal error")
	}
}

func TestTransportFailureDiagnostics(t *testing.T) {
	for _, test := range []struct {
		name   string
		status int
		body   string
		reason string
	}{
		{"http", 503, "PRIVATE-HTML", "http_status"},
		{"encoding", 200, "<html>PRIVATE</html>", "invalid_base64"},
		{"protocol", 200, "SE5IQks6SU5WQUxJRA==", "invalid_protocol_data"},
	} {
		t.Run(test.name, func(t *testing.T) {
			client, err := NewClient(testConfig(&http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: test.status, Status: "PRIVATE-STATUS", Header: make(http.Header), Body: io.NopCloser(strings.NewReader(test.body))}, nil
			})}))
			if err != nil {
				t.Fatal(err)
			}
			_, err = client.Accounts(context.Background())
			frames := FailureDiagnostics(err)
			found := false
			for _, f := range frames {
				if f.Reason == test.reason {
					found = true
					if test.reason == "http_status" && f.HTTPStatus != test.status {
						t.Fatal("HTTP status lost")
					}
				}
			}
			if !found {
				t.Fatalf("missing %s in %#v", test.reason, frames)
			}
			data, _ := json.Marshal(frames)
			if strings.Contains(string(data), "PRIVATE") {
				t.Fatal("private HTTP data leaked")
			}
		})
	}
}

func TestFailureDiagnosticsPreserveErrorIdentityAndAreBounded(t *testing.T) {
	original := errors.New("PRIVATE")
	err := original
	for i := 0; i < 20; i++ {
		err = failureWithResponse(err, "complete_tan", "local_error", nil, nil)
	}
	if !errors.Is(err, original) || err.Error() != original.Error() {
		t.Fatal("error identity changed")
	}
	if len(FailureDiagnostics(err)) != 12 {
		t.Fatal("unbounded diagnostic contexts")
	}
	if len(FailureDiagnostics(original)) != 0 {
		t.Fatal("unknown error text was captured")
	}
}
