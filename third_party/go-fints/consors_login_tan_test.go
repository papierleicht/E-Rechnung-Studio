package fints

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

// Reproduce the observed structure only; all references and credentials below
// are synthetic. Consors puts 0030 on HKIDN while HITAN refers to HKTAN.
func TestAccountsCompletesLoginTANReferencedToHKIDN(t *testing.T) {
	for _, method := range []string{"900", "901"} {
		t.Run(method, func(t *testing.T) {
			const fakeTAN = "123456789"
			approved, prompted, accountsRequested := false, false, false
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				request := consorsTestRequest(t, r)
				var reply string
				switch {
				case len(findSegments(request, "HKEND")) > 0:
					reply = "HIRMG:2:2+0100::Ended'"
				case len(findSegments(request, "HKIDN")) > 0:
					tan := findSegments(request, "HKTAN")
					if len(tan) != 1 || tan[0].Header.Number != 5 || component(tan[0], 1, 0) != "HKIDN" {
						t.Fatal("missing login HKTAN")
					}
					reply = "HIRMG:2:2+0010::Accepted'HIRMS:3:2:3+0030::TAN required'HIRMS:4:2:4+0020::Parameters accepted+3920::Methods:900:901'HITAN:5:7:5+4++synthetic-task+Enter TAN'"
				case hktanProcess(request) == "2":
					if !prompted || approved {
						t.Fatal("unexpected TAN continuation")
					}
					tan := findSegments(request, "HKTAN")
					sig := findSegments(request, "HNSHA")
					if len(tan) != 1 || component(tan[0], 4, 0) != "synthetic-task" || len(sig) != 1 || component(sig[0], 2, 1) != fakeTAN {
						t.Fatal("TAN or original task reference missing")
					}
					approved = true
					reply = "HIRMG:2:2+0010::Accepted'HIRMS:3:2:3+0020::Authenticated'"
				case len(findSegments(request, "HKSPA")) > 0:
					if !approved {
						t.Fatal("HKSPA sent before login TAN completion")
					}
					accountsRequested = true
					reply = "HIRMG:2:2+0010::Accepted'HISPA:3:1:3+J:DE00100100100012345678:MARKDEF1100:0012345678::280:10010010'"
				default:
					t.Fatal("unexpected request, especially no decoupled polling is allowed")
				}
				consorsTestReply(w, request, reply)
			})
			client := consorsTestClient(t, method, handler)
			client.config.TANHandler = func(_ context.Context, ch TANChallenge) (string, error) {
				if ch.Decoupled || ch.TaskReference != "synthetic-task" {
					t.Fatalf("wrong challenge: %#v", ch)
				}
				prompted = true
				return fakeTAN, nil
			}
			accounts, err := client.Accounts(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if !prompted || !approved || !accountsRequested || len(accounts) != 1 {
				t.Fatalf("incomplete login: prompt=%v approved=%v accounts=%v", prompted, approved, accountsRequested)
			}
		})
	}
}

func TestAccountsRejectsUnrelatedLoginChallengeCodes(t *testing.T) {
	for _, test := range []struct{ name, feedback string }{
		{"unrelated HKVVB", "HIRMS:3:2:4+0030::Unrelated TAN request'"},
	} {
		t.Run(test.name, func(t *testing.T) {
			client := consorsTestClient(t, "901", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				request := consorsTestRequest(t, r)
				reply := ""
				if len(findSegments(request, "HKIDN")) > 0 {
					reply = "HIRMG:2:2+0010::Accepted'" + test.feedback + "HITAN:5:7:5+4++synthetic-task+Approve'"
				} else if len(findSegments(request, "HKEND")) > 0 {
					reply = "HIRMG:2:2+0100::Ended'"
				} else {
					t.Fatal("ambiguous login must not continue or request accounts")
				}
				consorsTestReply(w, request, reply)
			}))
			client.config.TANHandler = func(context.Context, TANChallenge) (string, error) {
				t.Fatal("ambiguous login must not prompt")
				return "", nil
			}
			_, err := client.Accounts(context.Background())
			if err == nil {
				t.Fatal("ambiguous login reported success")
			}
		})
	}
}

func consorsTestClient(t *testing.T, method string, handler http.Handler) *Client {
	t.Helper()
	config := testConfig(mockHTTPClient(handler))
	config.SystemID = "synthetic-system"
	config.SecurityFunction = method
	config.HKTANVersion = 7
	client, err := NewClient(config)
	if err != nil {
		t.Fatal(err)
	}
	client.parametersReady = true
	client.tanMethods[method] = TANMethod{SecurityFunction: method, HKTANVersion: 7, Decoupled: method == "901"}
	return client
}

func consorsTestRequest(t *testing.T, r *http.Request) []segment {
	t.Helper()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := base64.StdEncoding.DecodeString(string(body))
	if err != nil {
		t.Fatal(err)
	}
	request, err := parseSegments(raw)
	if err != nil {
		t.Fatal(err)
	}
	return request
}

func consorsTestReply(w http.ResponseWriter, request []segment, inner string) {
	header := findSegments(request, "HNHBK")[0]
	number := component(header, 3, 0)
	payload := fmt.Sprintf("HNHBK:1:3+000000000000+300+synthetic-dialog+%s+synthetic-dialog:%s'%sHNHBS:99:1+%s'", number, number, inner, number)
	payload = strings.Replace(payload, "000000000000", fmt.Sprintf("%012d", len(payload)), 1)
	_, _ = w.Write([]byte(base64.StdEncoding.EncodeToString([]byte(payload))))
}

func TestAccountsPrefersHKTANDecoupledStatusOverOrder0030(t *testing.T) {
	approved, prompted, fetched := false, false, false
	client := consorsTestClient(t, "901", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		request := consorsTestRequest(t, r)
		reply := ""
		switch {
		case len(findSegments(request, "HKEND")) > 0:
			reply = "HIRMG:2:2+0100::Ended'"
		case len(findSegments(request, "HKIDN")) > 0:
			reply = "HIRMG:2:2+0010::Accepted'HIRMS:3:2:3+0030::Order needs approval'HIRMS:4:2:5+3955::Decoupled approval'HITAN:5:7:5+4++synthetic-task+Approve in app'"
		case hktanProcess(request) == "S":
			if !prompted || approved {
				t.Fatal("unexpected status poll")
			}
			tan := findSegments(request, "HKTAN")
			sig := findSegments(request, "HNSHA")
			if component(tan[0], 4, 0) != "synthetic-task" || len(sig) != 1 || component(sig[0], 2, 1) != "" {
				t.Fatal("incorrect decoupled continuation")
			}
			approved = true
			reply = "HIRMG:2:2+0010::Accepted'HIRMS:3:2:3+0020::Approved'"
		case len(findSegments(request, "HKSPA")) > 0:
			if !approved {
				t.Fatal("accounts before approval")
			}
			fetched = true
			reply = "HIRMG:2:2+0010::Accepted'HISPA:3:1:3+J:DE00100100100012345678:MARKDEF1100:0012345678::280:10010010'"
		default:
			t.Fatal("unexpected request")
		}
		consorsTestReply(w, request, reply)
	}))
	client.config.PollInterval = time.Millisecond
	client.config.TANHandler = func(_ context.Context, ch TANChallenge) (string, error) {
		if !ch.Decoupled {
			t.Fatal("3955 must take priority over order0030")
		}
		prompted = true
		return "", nil
	}
	accounts, err := client.Accounts(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !prompted || !approved || !fetched || len(accounts) != 1 {
		t.Fatal("incomplete decoupled flow")
	}
}
