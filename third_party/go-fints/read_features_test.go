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

// readTestClient exchanges real FinTS envelopes with an in-memory synthetic bank.
func readTestClient(t *testing.T, parameters string, operation func([]segment) string) *Client {
	t.Helper()
	bank := &mockBank{t: t}
	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Fatal(err)
		}
		wire, err := base64.StdEncoding.DecodeString(string(body))
		if err != nil {
			t.Fatal(err)
		}
		segments, err := parseSegments(wire)
		if err != nil {
			t.Fatal(err)
		}
		payload := ""
		switch {
		case len(findSegments(segments, "HKSYN")) > 0:
			payload = bank.syncResponse()
		case len(findSegments(segments, "HKIDN")) > 0:
			payload = strings.Replace(bank.initResponse(), "HNHBS:", parameters+"HNHBS:", 1)
		case len(findSegments(segments, "HKEND")) > 0:
			payload = responseMessage("dialog-1", "HIRMG:2:2+0100::Ended'")
		default:
			payload = responseMessage("dialog-1", "HIRMG:2:2+0010::Accepted'"+operation(segments))
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(base64.StdEncoding.EncodeToString([]byte(payload))))}, nil
	})
	client, err := NewClient(testConfig(&http.Client{Transport: transport}))
	if err != nil {
		t.Fatal(err)
	}
	return client
}
func readTestAccount() Account {
	return Account{IBAN: "DE0012345678001234567890", AccountNumber: "1234567890", CountryCode: "280", BankCode: "12345678", Subaccount: "00"}
}

func TestBalanceNegotiatesAndPreservesSignedAvailableAmounts(t *testing.T) {
	client := readTestClient(t, "HISALS:20:6+1+1+0'HISALS:21:7+1+1+0'HISALS:22:9+1+1+0'", func(request []segment) string {
		segments := findSegments(request, "HKSAL")
		if len(segments) != 1 {
			t.Fatalf("missing HKSAL: %#v", request)
		}
		s := segments[0]
		if s.Header.Version != 7 || component(s, 0, 0) != readTestAccount().IBAN || component(s, 1, 0) != "N" {
			t.Fatalf("HKSAL = %#v", s)
		}
		return "HISAL:4:7:3+DE0012345678001234567890::1234567890:00:280:12345678+Checking+EUR+D:123,45:EUR:20260901:120000+C:5,00:EUR:20260901+500,00:EUR+376,55:EUR'"
	})
	balance, err := client.Balance(context.Background(), readTestAccount())
	if err != nil {
		t.Fatal(err)
	}
	if balance.Booked.Amount != -12345 || balance.Booked.Date != "2026-09-01" || balance.Pending.Amount != 500 || balance.Available.Amount != 37655 {
		t.Fatalf("balance = %#v", balance)
	}
}
func TestReadOperationRequiresAdvertisement(t *testing.T) {
	client := readTestClient(t, "", func(request []segment) string { t.Fatal("unsupported operation was sent"); return "" })
	if _, err := client.Balance(context.Background(), readTestAccount()); err == nil {
		t.Fatal("missing capability accepted")
	}
}
func TestStatementListFollowsCursorAndDecodesEscapes(t *testing.T) {
	pages := 0
	client := readTestClient(t, "HIKAUS:20:2+1+1+0'", func(request []segment) string {
		s := findSegments(request, "HKKAU")[0]
		pages++
		if s.Header.Version != 2 || component(s, 0, 0) != readTestAccount().IBAN {
			t.Fatalf("HKKAU = %#v", s)
		}
		if pages == 1 {
			return "HIKAU:4:2:3+8+0+J+2026+20260831+120000+Monthly'HIRMS:5:2:3+3040::More:cursor?+2'"
		}
		if component(s, 2, 0) != "cursor+2" {
			t.Fatalf("cursor %q", component(s, 2, 0))
		}
		return "HIKAU:4:2:3+9+2+J+2026+20260930+120000+Monthly'"
	})
	statements, err := client.StatementList(context.Background(), readTestAccount())
	if err != nil {
		t.Fatal(err)
	}
	if len(statements) != 2 || statements[0].Number != 8 || statements[1].CreatedDate != "2026-09-30" {
		t.Fatalf("statements=%#v", statements)
	}
}
func TestStatementPreservesBinaryAndDoesNotAcknowledge(t *testing.T) {
	content := []byte("%PDF-1.7\nsynthetic+'?@\x00\xff")
	client := readTestClient(t, "HIEKAS:20:5+1+1+0'", func(request []segment) string {
		s := findSegments(request, "HKEKA")[0]
		if s.Header.Version != 5 || component(s, 1, 0) != "3" || component(s, 2, 0) != "9" || component(s, 3, 0) != "2026" {
			t.Fatalf("HKEKA=%#v", s)
		}
		return "HIEKA:4:5:3+3+20260901:20260930+20261001+2026+9+" + binary(content) + "+Interest+Conditions+Advertisement+DE0012345678001234567890+MOCKDEFFXXX+Test+Holder++" + binary([]byte("receipt-code")) + "'"
	})
	statements, err := client.Statement(context.Background(), readTestAccount(), StatementRequest{Number: 9, Year: 2026, Format: StatementFormatPDF})
	if err != nil {
		t.Fatal(err)
	}
	if len(statements) != 1 || string(statements[0].Data) != string(content) || string(statements[0].ConfirmationCode) != "receipt-code" || statements[0].Name != "Test Holder" {
		t.Fatalf("statement=%#v", statements)
	}
}
func TestReadPaginationRejectsCycle(t *testing.T) {
	client := readTestClient(t, "HIWPDS:20:6+1+1+0'", func(request []segment) string {
		return "HIWPD:4:6:3+" + binary([]byte(":16R:GENL\r\n:16S:GENL")) + "'HIRMS:5:2:3+3040::More:loop'"
	})
	if _, err := client.Holdings(context.Background(), readTestAccount()); err == nil || !strings.Contains(err.Error(), "repeated") {
		t.Fatalf("error = %v", err)
	}
}
func TestHoldingsRequestsNationalAccountAndPreservesReport(t *testing.T) {
	content := []byte(":16R:GENL\r\n:16S:GENL")
	client := readTestClient(t, "HIWPDS:20:6+1+1+0'", func(request []segment) string {
		s := findSegments(request, "HKWPD")[0]
		if component(s, 0, 0) != "1234567890" || component(s, 0, 3) != "12345678" {
			t.Fatalf("HKWPD=%#v", s)
		}
		return "HIWPD:4:6:3+" + binary(content) + "'"
	})
	reports, err := client.Holdings(context.Background(), readTestAccount())
	if err != nil {
		t.Fatal(err)
	}
	if len(reports) != 1 || string(reports[0].Data) != string(content) {
		t.Fatalf("reports=%#v", reports)
	}
}
func TestBalanceRejectsInvalidOrOtherAccountResponse(t *testing.T) {
	for _, wire := range []string{
		"HISAL:4:7:3+OTHER+Checking+EUR+C:1,00:EUR:20260901'",
		"HISAL:4:7:3+DE0012345678001234567890+Checking+EUR+X:1,00:EUR:20260901'",
		"HISAL:4:7:3+DE0012345678001234567890+Checking+EUR+C:1,001:EUR:20260901'",
	} {
		segments, err := parseSegments([]byte(wire))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := parseBalance(segments[0], readTestAccount()); err == nil {
			t.Fatal("invalid balance accepted")
		}
	}
}

func TestCAMTRespectsAdvertisedDateWindowsAndPagination(t *testing.T) {
	calls := 0
	client := readTestClient(t, "HICAZS:20:1+1+1+0+2:N:N:urn?:iso?:std?:iso?:20022?:tech?:xsd?:camt.052.001.08'", func(request []segment) string {
		s := findSegments(request, "HKCAZ")[0]
		calls++
		from, to := component(s, 3, 0), component(s, 4, 0)
		if calls <= 2 && (from != "20260901" || to != "20260902") {
			t.Fatalf("first date window %s..%s", from, to)
		}
		if calls == 3 && (from != "20260903" || to != "20260903") {
			t.Fatalf("second date window %s..%s", from, to)
		}
		if calls == 2 && component(s, 6, 0) != "next" {
			t.Fatal("pagination cursor missing")
		}
		if calls == 3 && component(s, 6, 0) != "" {
			t.Fatal("pagination cursor carried to new range")
		}
		document := camtTestDocument(`<Ntry><Amt Ccy="EUR">1.00</Amt><CdtDbtInd>CRDT</CdtDbtInd></Ntry>`)
		response := "HICAZ:4:1:3+DE0012345678001234567890+urn?:iso?:std?:iso?:20022?:tech?:xsd?:camt.053.001.08+" + binary(document) + "'"
		if calls == 1 {
			response += "HIRMS:5:2:3+3040::More:next'"
		}
		return response
	})
	ctx := context.Background()
	client.mu.Lock()
	defer client.mu.Unlock()
	d, account, err := client.openReadDialog(ctx, readTestAccount())
	if err != nil {
		t.Fatal(err)
	}
	defer client.endDialog(ctx, d)
	from, _ := time.Parse(time.DateOnly, "2026-09-01")
	to, _ := time.Parse(time.DateOnly, "2026-09-03")
	transactions, err := client.transactionsCAMT(ctx, d, account, from, to)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 3 || len(transactions) != 3 {
		t.Fatalf("calls=%d transactions=%d", calls, len(transactions))
	}
}
func TestCAMTExplicitEmptyDiffersFromMissingAndRepeatingResponse(t *testing.T) {
	for _, mode := range []string{"empty", "missing", "repeat"} {
		t.Run(mode, func(t *testing.T) {
			client := readTestClient(t, "HICAZS:20:1+1+1+0'", func(request []segment) string {
				if mode == "empty" {
					return "HIRMS:3:2:3+3010::No data'"
				}
				if mode == "missing" {
					return "HIRMS:3:2:3+0020::Accepted'"
				}
				return "HICAZ:4:1:3+DE0012345678001234567890+urn?:iso?:std?:iso?:20022?:tech?:xsd?:camt.053.001.08+" + binary(camtTestDocument("")) + "'HIRMS:5:2:3+3040::More:loop'"
			})
			ctx := context.Background()
			client.mu.Lock()
			defer client.mu.Unlock()
			d, account, err := client.openReadDialog(ctx, readTestAccount())
			if err != nil {
				t.Fatal(err)
			}
			defer client.endDialog(ctx, d)
			transactions, err := client.transactionsCAMT(ctx, d, account, time.Time{}, time.Time{})
			if mode == "empty" {
				if err != nil || len(transactions) != 0 {
					t.Fatalf("empty=%#v,%v", transactions, err)
				}
			} else if err == nil {
				t.Fatal("incomplete response accepted")
			}
		})
	}
}
