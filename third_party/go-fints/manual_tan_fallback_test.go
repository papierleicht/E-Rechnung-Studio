package fints

import (
	"context"
	"encoding/base64"
	"io"
	"net/http"
	"testing"
	"time"
)

func TestBankResponseSelectsManualOrDecoupledTAN(t *testing.T) {
	for _, code := range []string{"0030", "3955"} {
		t.Run(code, func(t *testing.T) {
			decoupled := code == "3955"
			calls := 0
			config := testConfig(mockHTTPClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				body, _ := io.ReadAll(r.Body)
				raw, _ := base64.StdEncoding.DecodeString(string(body))
				request, err := parseSegments(raw)
				if err != nil {
					t.Fatal(err)
				}
				wantProcess, wantTAN := "2", "test-tan"
				if decoupled {
					wantProcess, wantTAN = "S", ""
				}
				if hktanProcess(request) != wantProcess {
					t.Fatalf("process=%s, want %s", hktanProcess(request), wantProcess)
				}
				sig := findSegments(request, "HNSHA")
				if len(sig) != 1 || component(sig[0], 2, 1) != wantTAN {
					t.Fatal("unexpected TAN signature")
				}
				_, _ = w.Write([]byte(base64.StdEncoding.EncodeToString([]byte(responseMessage("test-dialog", "HIRMG:2:2+0010::Accepted'HIRMS:3:2:3+0020::Done'")))))
			})))
			config.SecurityFunction = "901"
			config.SystemID = "test-system"
			config.PollInterval = time.Millisecond
			config.TANHandler = func(_ context.Context, challenge TANChallenge) (string, error) {
				if challenge.Decoupled != decoupled {
					t.Fatalf("Decoupled=%v, want %v", challenge.Decoupled, decoupled)
				}
				return "test-tan", nil
			}
			client, err := NewClient(config)
			if err != nil {
				t.Fatal(err)
			}
			client.tanMethods["901"] = TANMethod{SecurityFunction: "901", HKTANVersion: 7, Decoupled: true}
			response, err := parseSegments([]byte("HIRMS:2:2:5+" + code + "::Approval'HITAN:3:7:3+4++test-task+Approve'"))
			if err != nil {
				t.Fatal(err)
			}
			_, err = client.completeTAN(context.Background(), &dialog{id: "test-dialog", security: "901", systemID: "test-system", nextMessage: 2, lastTANRequest: 5}, response)
			if err != nil {
				t.Fatal(err)
			}
			if calls != 1 {
				t.Fatalf("requests=%d", calls)
			}
		})
	}
}

func TestTANChallengeUsesSubmittedHKTANReference(t *testing.T) {
	tests := []struct {
		name, wire, reference string
		required              bool
	}{
		{"sole HITAN with mismatched reference", "HIRMS:2:2:5+3955::Approve'HITAN:3:7:3+4++task+Approve'", "task", true},
		{"sole HITAN without reference", "HIRMS:2:2:5+3955::Approve'HITAN:3:7+4++task+Approve'", "task", true},
		{"prefer matching HITAN", "HIRMS:2:2:5+3955::Approve'HITAN:3:7:3+4++wrong+Approve'HITAN:4:7:5+4++right+Approve'", "right", true},
		{"ambiguous HITANs fail closed", "HIRMS:2:2:5+3955::Approve'HITAN:3:7:3+4++one+Approve'HITAN:4:7:4+4++two+Approve'", "", true},
		{"unresolved challenge fails closed", "HIRMS:2:2:3+0030::Approve'HITAN:3:7:5+4++task+Approve'", "", true},
		{"exemption placeholders remain ignored", "HIRMS:2:2:3+3076::No SCA'HIRMS:3:2:5+0030::Accepted'HITAN:4:7:5+4++noref+nochallenge'", "", false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response, err := parseSegments([]byte(test.wire))
			if err != nil {
				t.Fatal(err)
			}
			challenge, _, required := tanChallenge(response, "901", "", 5)
			if required != test.required || challenge.TaskReference != test.reference {
				t.Fatalf("required=%v challenge=%#v", required, challenge)
			}
		})
	}
}
