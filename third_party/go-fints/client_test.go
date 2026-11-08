package fints

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

type mockBank struct {
	t        *testing.T
	mu       sync.Mutex
	requests [][]segment
	page     int
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

func (m *mockBank) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	body, err := io.ReadAll(request.Body)
	if err != nil {
		m.t.Errorf("read request: %v", err)
		writer.WriteHeader(http.StatusInternalServerError)
		return
	}
	raw, err := base64.StdEncoding.DecodeString(string(body))
	if err != nil {
		m.t.Errorf("decode request: %v", err)
		writer.WriteHeader(http.StatusBadRequest)
		return
	}
	segments, err := parseSegments(raw)
	if err != nil {
		m.t.Errorf("parse request: %v", err)
		writer.WriteHeader(http.StatusBadRequest)
		return
	}
	m.mu.Lock()
	m.requests = append(m.requests, segments)
	m.mu.Unlock()

	var payload string
	switch {
	case len(findSegments(segments, "HKSYN")) > 0:
		payload = m.syncResponse()
	case len(findSegments(segments, "HKIDN")) > 0:
		payload = m.initResponse()
	case len(findSegments(segments, "HKSPA")) > 0:
		payload = m.accountsResponse()
	case len(findSegments(segments, "HKKAZ")) > 0:
		payload = m.transactionsResponse()
	case len(findSegments(segments, "HKEND")) > 0:
		payload = responseMessage("dialog-1", "HIRMG:2:2+0100::Dialog ended'")
	default:
		m.t.Errorf("unexpected request segments: %#v", segments)
		writer.WriteHeader(http.StatusBadRequest)
		return
	}

	writer.Header().Set("Content-Type", "text/plain")
	_, _ = writer.Write([]byte(base64.StdEncoding.EncodeToString([]byte(payload))))
}

func (m *mockBank) syncResponse() string {
	inner := "HIRMG:2:2+0010::Message accepted'" +
		"HIRMS:3:2:5+0020::Synchronization successful'" +
		"HISYN:4:4:5+system-123+1'" +
		"HIBPA:5:3:5+1+280:12345678+Mock Bank+0+1+300'" +
		"HIKAZS:6:6:5+1+1+100:N:N'" +
		"HIUPA:7:4:5+customer+1+0'"
	return responseMessage("sync-dialog", inner)
}

func (m *mockBank) initResponse() string {
	inner := "HIRMG:2:2+0010::Message accepted'" +
		"HIRMS:3:2:2+0020::Dialog initialized'" +
		"HIKAZS:4:6:2+1+1+100:N:N'" +
		"HIUPD:5:6:2+1234567890:00:280:12345678+DE0012345678001234567890+customer+1+EUR+Ada Lovelace++Checking'"
	return responseMessage("dialog-1", inner)
}

func (m *mockBank) accountsResponse() string {
	inner := "HIRMG:2:2+0010::Message accepted'" +
		"HIRMS:3:2:3+0020::Accounts returned'" +
		"HISPA:4:1:3+J:DE0012345678001234567890:MOCKDEFFXXX:1234567890:00:280:12345678'"
	return responseMessage("dialog-1", inner)
}

func (m *mockBank) transactionsResponse() string {
	m.mu.Lock()
	m.page++
	page := m.page
	m.mu.Unlock()

	statement := ""
	status := "HIRMS:3:2:3+0020::Transactions returned'"
	if page == 1 {
		statement = ":20:START\r\n:25:DE0012345678001234567890\r\n:60F:C260101EUR100,00\r\n" +
			":61:2601020102D10,00NTRFREF1//BANK1\r\n:86:835?20Coffee?32Cafe\r\n"
		status = "HIRMS:3:2:3+3040::More entries:next-page'"
	} else {
		statement = ":61:2601030103C25,50NTRFREF2//BANK2\r\n:86:835?20Refund?32Shop\r\n" +
			":62F:C260103EUR115,50\r\n-"
	}
	inner := "HIRMG:2:2+0010::Message accepted'" + status +
		"HIKAZ:4:6:3+" + binary([]byte(statement)) + "'"
	return responseMessage("dialog-1", inner)
}

func responseMessage(dialogID, inner string) string {
	message := fmt.Sprintf("HNHBK:1:3+000000000000+300+%s+1'%sHNHBS:99:1+1'", dialogID, inner)
	return strings.Replace(message, "000000000000", fmt.Sprintf("%012d", len(message)), 1)
}

func testConfig(httpClient *http.Client) Config {
	return Config{
		Endpoint:   "http://bank.test/fints",
		BankCode:   "12345678",
		UserID:     "user",
		CustomerID: "customer",
		PIN:        "12345",
		ProductID:  "PRODUCT-ID",
		AllowHTTP:  true,
		HTTPClient: httpClient,
	}
}

func TestNewClientValidatesProductMetadataBeforeIO(t *testing.T) {
	for _, test := range []struct {
		name    string
		id      string
		version string
		wantErr string
	}{
		{"default version", "test-product", "", ""},
		{"ASCII limits", strings.Repeat("A", 25), "0.1.1", ""},
		{"Latin1 limits", strings.Repeat("ä", 25), "äöüßé", ""},
		{"escaped characters", strings.Repeat("+", 25), "?+:'@", ""},
		{"long ID", strings.Repeat("A", 26), "0.1.1", "product ID must contain at most 25"},
		{"long Latin1 ID", strings.Repeat("ä", 26), "0.1.1", "product ID must contain at most 25"},
		{"long version", "test-product", "Papierleicht", "product version must contain at most 5"},
		{"long Latin1 version", "test-product", "äöüßéà", "product version must contain at most 5"},
		{"unencodable ID", "test-€", "0.1.1", "product ID must only contain ISO-8859-1"},
		{"unencodable version", "test-product", "1.0€", "product version must only contain ISO-8859-1"},
		{"invalid UTF8 ID", "test-\xff", "0.1.1", "product ID must only contain ISO-8859-1"},
		{"invalid UTF8 version", "test-product", "1.\xff", "product version must only contain ISO-8859-1"},
		{"blank version", "test-product", " ", "product version is required"},
	} {
		t.Run(test.name, func(t *testing.T) {
			config := testConfig(&http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				t.Fatal("client construction performed network I/O")
				return nil, nil
			})})
			config.ProductID, config.ProductVersion = test.id, test.version
			client, err := NewClient(config)
			if test.wantErr == "" {
				if err != nil {
					t.Fatal(err)
				}
				if client.config.ProductID != test.id {
					t.Fatal("product ID was modified")
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("NewClient error = %v, want %s", err, test.wantErr)
			}
			if strings.Contains(err.Error(), config.PIN) || strings.Contains(err.Error(), test.id) ||
				(test.version != " " && strings.Contains(err.Error(), test.version)) {
				t.Fatal("validation error exposes supplied configuration values")
			}
		})
	}
}

func TestNewClientValidatesAndNormalizesCompatibilityOptions(t *testing.T) {
	config := testConfig(&http.Client{})
	config.Compatibility = CompatibilityOptions{
		ForceTANForOperations:         []string{" hkkaz ", "HKKAZ", "hkcaz"},
		PreferredSecurityFunctions:    []string{"921", " 920 "},
		TANMediumRequiredForFunctions: []string{"921"},
		DecoupledPollProcess:          "s",
	}
	client, err := NewClient(config)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(
		client.config.Compatibility.ForceTANForOperations,
		",",
	); got != "HKKAZ,HKCAZ" {
		t.Fatalf("force TAN operations = %q", got)
	}
	if client.config.Compatibility.DecoupledPollProcess != "S" {
		t.Fatalf(
			"decoupled poll process = %q",
			client.config.Compatibility.DecoupledPollProcess,
		)
	}

	config.Compatibility.DecoupledPollProcess = "invalid"
	if _, err := NewClient(config); err == nil {
		t.Fatal("invalid decoupled poll process was accepted")
	}
}

func TestNewClientUsesAConfigurableHTTPTimeout(t *testing.T) {
	config := testConfig(nil)
	client, err := NewClient(config)
	if err != nil {
		t.Fatal(err)
	}
	if client.httpClient.Timeout != 90*time.Second {
		t.Fatalf("default HTTP timeout = %s, want 90s", client.httpClient.Timeout)
	}

	config.Timeout = 3 * time.Minute
	client, err = NewClient(config)
	if err != nil {
		t.Fatal(err)
	}
	if client.httpClient.Timeout != 3*time.Minute {
		t.Fatalf("configured HTTP timeout = %s, want 3m", client.httpClient.Timeout)
	}

	config.Timeout = -1
	if _, err := NewClient(config); err == nil {
		t.Fatal("negative HTTP timeout was accepted")
	}
}

func TestPostReportsConfiguredTimeout(t *testing.T) {
	httpClient := &http.Client{
		Timeout: 2 * time.Minute,
		Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return nil, context.DeadlineExceeded
		}),
	}
	config := testConfig(httpClient)
	client, err := NewClient(config)
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.post(context.Background(), []byte("request"))
	if err == nil ||
		!strings.Contains(err.Error(), "timed out after 2m0s") ||
		!strings.Contains(err.Error(), "increase Config.Timeout") {
		t.Fatalf("timeout error = %v", err)
	}
}

func TestClientAccountsFullDialog(t *testing.T) {
	bank := &mockBank{t: t}

	client, err := NewClient(testConfig(mockHTTPClient(bank)))
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	accounts, err := client.Accounts(context.Background())
	if err != nil {
		t.Fatalf("Accounts() error = %v", err)
	}
	if len(accounts) != 1 {
		t.Fatalf("got %d accounts, want 1", len(accounts))
	}
	account := accounts[0]
	if account.IBAN != "DE0012345678001234567890" || account.BIC != "MOCKDEFFXXX" {
		t.Fatalf("account identifiers = %q/%q", account.IBAN, account.BIC)
	}
	if account.Currency != "EUR" || account.OwnerName != "Ada Lovelace" || account.ProductName != "Checking" {
		t.Fatalf("account metadata = %#v", account)
	}

	assertAllRequestsSigned(t, bank.requests, "12345")
}

func TestFirstTwoStepDialogRequestsFreshUPD(t *testing.T) {
	requestedUPDVersion := ""
	handler := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, _ := io.ReadAll(request.Body)
		raw, _ := base64.StdEncoding.DecodeString(string(body))
		segments, err := parseSegments(raw)
		if err != nil {
			t.Fatal(err)
		}

		var inner string
		switch {
		case len(findSegments(segments, "HKEND")) > 0:
			inner = "HIRMG:2:2+0100::Dialog ended'"
		case len(findSegments(segments, "HKIDN")) > 0:
			hkvvb := findSegments(segments, "HKVVB")
			if len(hkvvb) != 1 {
				t.Fatalf("initialization HKVVB = %#v", hkvvb)
			}
			requestedUPDVersion = component(hkvvb[0], 1, 0)
			inner = "HIRMG:2:2+0010::Dialog initialized'" +
				"HIUPA:3:4:2+customer+7+0'" +
				"HIUPD:4:6:2+1234567890:00:280:12345678+DE0012345678001234567890+customer+1+EUR+Ada Lovelace++Checking'"
		default:
			t.Fatalf("unexpected request: %#v", segments)
		}
		payload := responseMessage("dialog-fresh-upd", inner)
		_, _ = writer.Write([]byte(base64.StdEncoding.EncodeToString([]byte(payload))))
	})

	config := testConfig(mockHTTPClient(handler))
	config.SystemID = "system-123"
	config.SecurityFunction = "921"
	config.HKTANVersion = 7
	client, err := NewClient(config)
	if err != nil {
		t.Fatal(err)
	}
	client.parametersReady = true
	client.bpdVersion = 9
	client.updVersion = 7

	dialog, err := client.openDialog(context.Background())
	if err != nil {
		t.Fatalf("openDialog() error = %v", err)
	}
	client.endDialog(context.Background(), dialog)
	if requestedUPDVersion != "0" {
		t.Fatalf("requested UPD version = %q, want 0", requestedUPDVersion)
	}
	if !client.authenticatedParametersReady {
		t.Fatal("authenticated parameters were not marked ready")
	}
}

func TestClientTransactionsPaginatesAndParses(t *testing.T) {
	bank := &mockBank{t: t}

	client, err := NewClient(testConfig(mockHTTPClient(bank)))
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	from := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, time.January, 31, 0, 0, 0, 0, time.UTC)
	account := Account{AccountNumber: "1234567890", BankCode: "12345678"}
	transactions, err := client.Transactions(context.Background(), account, from, to)
	if err != nil {
		t.Fatalf("Transactions() error = %v", err)
	}
	if len(transactions) != 2 {
		t.Fatalf("got %d transactions, want 2", len(transactions))
	}
	if transactions[0].Amount != -1000 || transactions[1].Amount != 2550 {
		t.Fatalf("amounts = %d, %d", transactions[0].Amount, transactions[1].Amount)
	}
	// The final closing balance travels with the transactions; no HKSAL needed.
	if b := client.StatementBalance(); b == nil || b.Amount != 11550 || b.Date != "2026-01-03" {
		t.Fatalf("StatementBalance() = %+v", b)
	}

	bank.mu.Lock()
	requests := append([][]segment(nil), bank.requests...)
	bank.mu.Unlock()
	var transactionRequests []segment
	for _, request := range requests {
		if matches := findSegments(request, "HKKAZ"); len(matches) > 0 {
			transactionRequests = append(transactionRequests, matches[0])
		}
	}
	if len(transactionRequests) != 2 {
		t.Fatalf("got %d HKKAZ requests, want 2", len(transactionRequests))
	}
	if transactionRequests[0].Header.Version != 6 {
		t.Fatalf("HKKAZ version = %d, want 6", transactionRequests[0].Header.Version)
	}
	if got := component(transactionRequests[0], 2, 0); got != "20260101" {
		t.Fatalf("from date = %q, want 20260101", got)
	}
	if got := component(transactionRequests[1], 5, 0); got != "next-page" {
		t.Fatalf("touchdown = %q, want next-page", got)
	}
}

func TestClientResolvesIBANToNationalAccountForHKKAZ6(t *testing.T) {
	bank := &mockBank{t: t}
	client, err := NewClient(testConfig(mockHTTPClient(bank)))
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Transactions(
		context.Background(),
		Account{IBAN: "DE0012345678001234567890"},
		time.Time{},
		time.Time{},
	)
	if err != nil {
		t.Fatalf("Transactions() error = %v", err)
	}

	for _, request := range bank.requests {
		matches := findSegments(request, "HKKAZ")
		if len(matches) == 0 {
			continue
		}
		if matches[0].Header.Version != 6 ||
			component(matches[0], 0, 0) != "1234567890" ||
			component(matches[0], 0, 1) != "00" {
			t.Fatalf("HKKAZ account = %#v, want resolved national account", matches[0])
		}
		return
	}
	t.Fatal("bank received no HKKAZ request")
}

func TestTransactionsCanSkipSEPAAccountDiscovery(t *testing.T) {
	var requestKinds []string
	handler := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
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

		var inner string
		switch {
		case len(findSegments(segments, "HKEND")) > 0:
			requestKinds = append(requestKinds, "HKEND")
			inner = "HIRMG:2:2+0100::Dialog ended'"
		case len(findSegments(segments, "HKSPA")) > 0:
			t.Fatal("transaction dialog must not send disabled HKSPA probe")
		case len(findSegments(segments, "HKKAZ")) > 0:
			requestKinds = append(requestKinds, "HKKAZ")
			inner = "HIRMG:2:2+0010::Accepted'" +
				"HIRMS:3:2:2+0020::Transactions returned'HIKAZ:4:6:2+@0@'"
		case len(findSegments(segments, "HKIDN")) > 0:
			requestKinds = append(requestKinds, "HKIDN")
			inner = "HIRMG:2:2+0010::Accepted'" +
				"HIKAZS:4:6:1+1+1+100:N:N'" +
				"HIUPD:5:6:1+1234567890:EUR:280:10010010+DE00100100101234567890+customer+1+EUR+Ada Lovelace++Checking'"
		default:
			t.Fatalf("unexpected request: %#v", segments)
		}
		payload := responseMessage("dialog-fyrst", inner)
		_, _ = writer.Write([]byte(base64.StdEncoding.EncodeToString([]byte(payload))))
	})

	config := testConfig(mockHTTPClient(handler))
	config.BankCode = "10010010"
	config.SystemID = "system-123"
	config.SecurityFunction = defaultSecurity
	config.Compatibility.SkipTransactionSEPAAccountDiscovery = true
	client, err := NewClient(config)
	if err != nil {
		t.Fatal(err)
	}
	client.parametersReady = true

	transactions, err := client.Transactions(
		context.Background(),
		Account{IBAN: "DE00100100101234567890"},
		time.Time{},
		time.Time{},
	)
	if err != nil {
		t.Fatalf("Transactions() error = %v", err)
	}
	if len(transactions) != 0 {
		t.Fatalf("got %d transactions, want none", len(transactions))
	}
	if got, want := strings.Join(requestKinds, ","), "HKIDN,HKKAZ,HKEND"; got != want {
		t.Fatalf("request order = %q, want %q", got, want)
	}
}

func TestClientRetriesCurrencyForMissingNationalSubaccount(t *testing.T) {
	var hkkazSubaccounts []string
	handler := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
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

		var inner string
		switch {
		case len(findSegments(segments, "HKEND")) > 0:
			inner = "HIRMG:2:2+0100::Dialog ended'"
		case len(findSegments(segments, "HKSPA")) > 0:
			inner = "HIRMG:2:2+0010::Accepted'" +
				"HISPA:3:1:3+J:DE0012345678001234567890:MOCKDEFFXXX:1234567890:00:280:12345678'"
		case len(findSegments(segments, "HKKAZ")) > 0:
			requestAccount := findSegments(segments, "HKKAZ")[0]
			hkkazSubaccounts = append(hkkazSubaccounts, component(requestAccount, 0, 1))
			if len(hkkazSubaccounts) == 1 {
				inner = "HIRMG:2:2+9050::Teilweise fehlerhaft'" +
					"HIRMS:3:2:3+9160:2,2:Nicht belegt.'"
			} else {
				inner = "HIRMG:2:2+0010::Accepted'HIRMS:3:2:3+0020::Transactions returned'HIKAZ:4:6:3+@0@'"
			}
		case len(findSegments(segments, "HKIDN")) > 0:
			inner = "HIRMG:2:2+0010::Accepted'" +
				"HIKAZS:4:6:2+1+1+100:N:N'" +
				"HIUPD:5:6:2+1234567890:00:280:12345678+DE0012345678001234567890+customer+1+EUR+Ada Lovelace++Checking'"
		default:
			t.Fatalf("unexpected request: %#v", segments)
		}
		payload := responseMessage("dialog-currency", inner)
		_, _ = writer.Write([]byte(base64.StdEncoding.EncodeToString([]byte(payload))))
	})

	config := testConfig(mockHTTPClient(handler))
	config.SystemID = "system-123"
	config.SecurityFunction = defaultSecurity
	client, err := NewClient(config)
	if err != nil {
		t.Fatal(err)
	}
	// The test focuses on the authenticated dialog and account request.
	client.parametersReady = true

	transactions, err := client.Transactions(
		context.Background(),
		Account{IBAN: "DE0012345678001234567890"},
		time.Time{},
		time.Time{},
	)
	if err != nil {
		t.Fatalf("Transactions() error = %v", err)
	}
	if len(transactions) != 0 {
		t.Fatalf("Transactions() = %#v, want no entries", transactions)
	}
	if got, want := strings.Join(hkkazSubaccounts, ","), "00,EUR"; got != want {
		t.Fatalf("HKKAZ subaccounts = %q, want %q", got, want)
	}
}

func TestClientFallsBackFromHKKAZ7ToAdvertisedNationalVersion(t *testing.T) {
	var versions []int
	handler := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
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
		matches := findSegments(segments, "HKKAZ")
		if len(matches) != 1 {
			t.Fatalf("request has %d HKKAZ segments, want 1", len(matches))
		}
		versions = append(versions, matches[0].Header.Version)

		inner := "HIRMG:2:2+0010::Accepted'HIRMS:3:2:3+0020::Transactions returned'"
		if matches[0].Header.Version == 7 {
			inner = "HIRMG:2:2+9050::Teilweise fehlerhaft'" +
				"HIRMS:3:2:3+9160:2,2:Nicht belegt.'"
		}
		payload := responseMessage("dialog-v7", inner)
		_, _ = writer.Write([]byte(base64.StdEncoding.EncodeToString([]byte(payload))))
	})

	config := testConfig(mockHTTPClient(handler))
	config.SystemID = "system-123"
	config.SecurityFunction = defaultSecurity
	client, err := NewClient(config)
	if err != nil {
		t.Fatal(err)
	}
	dialog := &dialog{
		id:            "dialog-v7",
		nextMessage:   1,
		systemID:      "system-123",
		security:      defaultSecurity,
		hkkazVersions: []int{7, 6},
	}
	account := Account{
		IBAN:          "DE0012345678001234567890",
		AccountNumber: "1234567890",
		Subaccount:    "EUR",
		CountryCode:   "280",
		BankCode:      "12345678",
	}

	_, _, resolvedVersion, err := client.executeHKKAZ(
		context.Background(),
		dialog,
		7,
		account,
		time.Time{},
		time.Time{},
		"",
	)
	if err != nil {
		t.Fatalf("executeHKKAZ() error = %v", err)
	}
	if got, want := fmt.Sprint(versions), "[7 6]"; got != want {
		t.Fatalf("HKKAZ versions = %s, want %s", got, want)
	}
	if resolvedVersion != 6 {
		t.Fatalf("resolved HKKAZ version = %d, want 6", resolvedVersion)
	}
}

func TestClientFallsBackFromRejectedHKKAZToCAMT(t *testing.T) {
	camt := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<Document xmlns="urn:iso:std:iso:20022:tech:xsd:camt.052.001.08">
  <BkToCstmrAcctRpt><Rpt><Ntry>
    <Amt Ccy="EUR">4.20</Amt><CdtDbtInd>DBIT</CdtDbtInd>
    <BookgDt><Dt>2026-07-23</Dt></BookgDt>
    <NtryRef>CAMT-1</NtryRef><AddtlNtryInf>Fallback transaction</AddtlNtryInf>
  </Ntry></Rpt></BkToCstmrAcctRpt>
</Document>`)
	var sawHKKAZ, sawHKCAZ bool
	handler := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
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

		var inner string
		switch {
		case len(findSegments(segments, "HKEND")) > 0:
			inner = "HIRMG:2:2+0100::Dialog ended'"
		case len(findSegments(segments, "HKSPA")) > 0:
			inner = "HIRMG:2:2+0010::Accepted'" +
				"HISPA:3:1:3+J:DE0012345678001234567890:MOCKDEFFXXX:1234567890:EUR:280:12345678'"
		case len(findSegments(segments, "HKKAZ")) > 0:
			sawHKKAZ = true
			inner = "HIRMG:2:2+9050::Teilweise fehlerhaft'" +
				"HIRMS:3:2:3+9160:2,2:Nicht belegt.'"
		case len(findSegments(segments, "HKCAZ")) > 0:
			sawHKCAZ = true
			inner = "HIRMG:2:2+0010::Accepted'" +
				"HICAZ:4:1:3+DE0012345678001234567890:MOCKDEFFXXX:1234567890:EUR:280:12345678" +
				"+urn?:iso?:std?:iso?:20022?:tech?:xsd?:camt.052.001.08+" +
				binary(camt) + "'"
		case len(findSegments(segments, "HKIDN")) > 0:
			inner = "HIRMG:2:2+0010::Accepted'" +
				"HIKAZS:4:6:2+1+1+100:N:N'" +
				"HICAZS:5:1:2+1+1+0+180:N:N:urn?:iso?:std?:iso?:20022?:tech?:xsd?:camt.052.001.08'" +
				"HIUPD:6:6:2+1234567890:EUR:280:12345678+DE0012345678001234567890+customer+1+EUR+Ada Lovelace++Checking'"
		default:
			t.Fatalf("unexpected request: %#v", segments)
		}
		payload := responseMessage("dialog-camt", inner)
		_, _ = writer.Write([]byte(base64.StdEncoding.EncodeToString([]byte(payload))))
	})

	config := testConfig(mockHTTPClient(handler))
	config.SystemID = "system-123"
	config.SecurityFunction = defaultSecurity
	client, err := NewClient(config)
	if err != nil {
		t.Fatal(err)
	}
	client.parametersReady = true

	transactions, err := client.Transactions(
		context.Background(),
		Account{IBAN: "DE0012345678001234567890"},
		time.Time{},
		time.Time{},
	)
	if err != nil {
		t.Fatalf("Transactions() error = %v", err)
	}
	if !sawHKKAZ || !sawHKCAZ {
		t.Fatalf("fallback requests: HKKAZ=%v HKCAZ=%v", sawHKKAZ, sawHKCAZ)
	}
	if len(transactions) != 1 || transactions[0].Amount != -420 ||
		transactions[0].Description != "Fallback transaction" {
		t.Fatalf("Transactions() = %#v", transactions)
	}
}

func TestClientReturnsTANRequired(t *testing.T) {
	handler := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		payload := responseMessage("dialog-1",
			"HIRMG:2:2+0010::Message accepted'HIRMS:3:2:3+0030::TAN required'HITAN:4:6:3+4++order-ref+Approve in app'")
		_, _ = writer.Write([]byte(base64.StdEncoding.EncodeToString([]byte(payload))))
	})

	config := testConfig(mockHTTPClient(handler))
	config.SystemID = "system-123"
	client, err := NewClient(config)
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	_, err = client.Accounts(context.Background())
	if err == nil || !strings.Contains(err.Error(), ErrUnsupportedTAN.Error()) {
		t.Fatalf("Accounts() error = %v, want unsupported TAN", err)
	}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (function roundTripperFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

func mockHTTPClient(handler http.Handler) *http.Client {
	return &http.Client{
		Transport: roundTripperFunc(func(request *http.Request) (*http.Response, error) {
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, request)
			return recorder.Result(), nil
		}),
	}
}

func TestNewClientValidation(t *testing.T) {
	_, err := NewClient(Config{Endpoint: "http://bank.example", BankCode: "1", UserID: "u", PIN: "p", ProductID: "x"})
	if err == nil || !strings.Contains(err.Error(), "HTTPS") {
		t.Fatalf("NewClient() error = %v, want HTTPS validation error", err)
	}
}

func TestFirstBankErrorPrefersSegmentDetailOver9050(t *testing.T) {
	response, err := parseSegments([]byte(
		"HIRMG:2:2+9050::Teilweise fehlerhaft'" +
			"HIRMS:3:2:6+9210::Wert widerspricht Bankvorgaben:KUmsZeit6'",
	))
	if err != nil {
		t.Fatal(err)
	}
	got, ok := firstBankError(response).(*BankError)
	if !ok {
		t.Fatalf("firstBankError() = %#v", firstBankError(response))
	}
	if got.Code != "9210" || got.Details[1].Segment != 6 ||
		!strings.Contains(got.Error(), "9050") {
		t.Fatalf("BankError = %#v, text %q", got, got.Error())
	}
}

func TestCAMTFallbackIsLimitedToAccountAddressingErrors(t *testing.T) {
	accountResponse, err := parseSegments([]byte(
		"HIRMG:2:2+9050::Teilweise fehlerhaft'" +
			"HIRMS:3:2:3+9160:2,3:Nicht belegt.'",
	))
	if err != nil {
		t.Fatal(err)
	}
	authResponse, err := parseSegments([]byte(
		"HIRMG:2:2+9050::Teilweise fehlerhaft'" +
			"HIRMS:3:2:4+9940::Kein aktives TAN-Gerät verfügbar.'",
	))
	if err != nil {
		t.Fatal(err)
	}
	if !shouldFallbackToCAMT(firstBankError(accountResponse)) {
		t.Fatal("9160 should permit CAMT fallback")
	}
	if shouldFallbackToCAMT(firstBankError(authResponse)) {
		t.Fatal("9940 must not permit CAMT fallback")
	}
}

func TestDialogMessageNumberFollowsBankEnvelope(t *testing.T) {
	response, err := parseSegments([]byte(
		"HNHBK:1:3+000000000047+300+dialog-1+7'HNHBS:2:1+7'",
	))
	if err != nil {
		t.Fatal(err)
	}
	dialog := &dialog{nextMessage: 2}
	synchronizeDialogMessageNumber(dialog, response, 1)
	if dialog.nextMessage != 8 {
		t.Fatalf("next message = %d, want 8", dialog.nextMessage)
	}
}

func assertAllRequestsSigned(t *testing.T, requests [][]segment, pin string) {
	t.Helper()
	if len(requests) == 0 {
		t.Fatal("mock bank received no requests")
	}
	for index, request := range requests {
		signatures := findSegments(request, "HNSHA")
		if len(signatures) != 1 {
			t.Fatalf("request %d has %d HNSHA segments, want 1", index, len(signatures))
		}
		if got := component(signatures[0], 2, 0); got != pin {
			t.Fatalf("request %d PIN = %q, want %q", index, got, pin)
		}
	}
}
