package fints

import (
	"context"
	"encoding/base64"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestCreditCardRequestEscapesFieldsAndPositions(t *testing.T) {
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	to := from.AddDate(0, 1, -1)
	wire := buildCreditCardRequest("123:00:280:12345678", "card+alias", from, to, "page:2")
	segments, err := parseSegments([]byte(strings.ReplaceAll(wire, "{n}", "3")))
	if err != nil {
		t.Fatal(err)
	}
	s := segments[0]
	if s.Header.Type != "DKKKU" || s.Header.Version != 2 || component(s, 1, 0) != "card+alias" || component(s, 2, 0) != "" || component(s, 3, 0) != "20260101" || component(s, 4, 0) != "20260131" || component(s, 6, 0) != "page:2" {
		t.Fatalf("incorrect credit-card request: %#v", s)
	}
}

func TestCreditCardResponsesPreserveOpaqueFields(t *testing.T) {
	payload := []byte("opaque+statement:\x00data")
	segments, err := parseSegments([]byte("DIKKU:4:2:3+issuer?:value+" + binary(payload) + "'"))
	if err != nil {
		t.Fatal(err)
	}
	rows, err := creditCardResponses(segments)
	if err != nil || len(rows) != 1 || rows[0].Fields[0].Components[0] != "issuer:value" || !rows[0].Fields[1].Binary || string(rows[0].Fields[1].Data) != string(payload) {
		t.Fatalf("rows=%#v err=%v", rows, err)
	}
	segments[0].Fields[1].raw[0] = 'X'
	if rows[0].Fields[1].Data[0] == 'X' {
		t.Fatal("returned response aliases mutable parser data")
	}
}

func TestCreditCardResponseRejectsMissingOrUnsupportedData(t *testing.T) {
	for _, wire := range []string{"HIRMG:1:2+0010::OK'", "DIKKU:1:9+data'", "DIKKU:1:2'"} {
		segments, err := parseSegments([]byte(wire))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := creditCardResponses(segments); err == nil {
			t.Errorf("accepted malformed response %s", wire)
		}
	}
	segments, _ := parseSegments([]byte("HIRMS:2:2:3+3010::No data'"))
	if rows, err := creditCardResponses(segments); err != nil || len(rows) != 0 {
		t.Fatalf("explicit no-data result: %#v %v", rows, err)
	}
}

func TestCreditCardTransactionsNegotiationAndPagination(t *testing.T) {
	for _, tc := range []struct {
		name                              string
		advertised, denyAccount, repeated bool
		wantError                         bool
	}{
		{"supported", true, false, false, false},
		{"not_advertised", false, false, false, true},
		{"account_denied", true, true, false, true},
		{"repeated_token", true, false, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			client, err := NewClient(Config{
				Endpoint: "https://bank.example/fints", BankCode: "12345678", UserID: "test-user",
				PIN: "test-pin", ProductID: "test-product", SystemID: "system-test",
				HTTPClient: &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
					body, err := io.ReadAll(request.Body)
					if err != nil {
						t.Fatal(err)
					}
					raw, err := base64.StdEncoding.DecodeString(string(body))
					if err != nil {
						t.Fatal(err)
					}
					segments, err := parseSegments(raw)
					if err != nil {
						t.Fatal(err)
					}
					payload := "HIRMG:2:2+0010::OK'"
					switch {
					case len(findSegments(segments, "HKIDN")) > 0:
						if tc.advertised {
							payload += "DIKKUS:3:2+1+1+0+90:N:J'"
						}
					case len(findSegments(segments, "DKKKU")) > 0:
						calls++
						command := findSegments(segments, "DKKKU")[0]
						if calls == 2 && component(command, 6, 0) != "NEXT" {
							t.Fatal("missing continuation token")
						}
						payload += "DIKKU:3:2+row'"
						if calls == 1 || tc.repeated {
							payload += "HIRMS:4:2:3+3040::More:NEXT'"
						}
					case len(findSegments(segments, "HKEND")) > 0:
					default:
						t.Fatalf("unexpected request: %#v", segments)
					}
					response := base64.StdEncoding.EncodeToString([]byte(responseMessage("card-dialog", payload)))
					return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(response))}, nil
				})},
			})
			if err != nil {
				t.Fatal(err)
			}
			client.parametersReady = true
			account := Account{AccountNumber: "123456", CountryCode: "280", BankCode: "12345678"}
			if tc.denyAccount {
				account.SupportedOperations = map[string]bool{"HKKAZ": true}
			}
			rows, err := client.CreditCardTransactions(context.Background(), account, "test-card", time.Time{}, time.Time{})
			if (err != nil) != tc.wantError {
				t.Fatalf("rows=%#v err=%v", rows, err)
			}
			if !tc.wantError && (len(rows) != 2 || calls != 2) {
				t.Fatalf("rows=%d calls=%d", len(rows), calls)
			}
			if (!tc.advertised || tc.denyAccount) && calls != 0 {
				t.Fatal("sent unadvertised operation")
			}
		})
	}
}

func TestCreditCardInputRejectedBeforeNetwork(t *testing.T) {
	client, err := NewClient(Config{Endpoint: "https://bank.example/fints", BankCode: "12345678", UserID: "user", PIN: "pin", ProductID: "product"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.CreditCardTransactions(context.Background(), Account{AccountNumber: "1"}, "", time.Time{}, time.Time{}); err == nil {
		t.Fatal("accepted empty card identifier")
	}
	if _, err := client.CreditCardTransactions(context.Background(), Account{}, "card", time.Time{}, time.Time{}); err == nil {
		t.Fatal("accepted missing account")
	}
	if _, err := client.CreditCardTransactions(context.Background(), Account{AccountNumber: "1"}, "card", time.Now(), time.Now().Add(-time.Hour)); err == nil {
		t.Fatal("accepted reversed dates")
	}
}
