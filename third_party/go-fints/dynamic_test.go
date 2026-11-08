package fints

import (
	"context"
	"encoding/base64"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestNormalizeGermanIBANDerivesDomesticAccount(t *testing.T) {
	account := normalizeAccount(Account{IBAN: "DE89 3704 0044 0532 0130 00"})
	if account.IBAN != "DE89370400440532013000" ||
		account.BankCode != "37040044" ||
		account.AccountNumber != "0532013000" ||
		account.CountryCode != "280" {
		t.Fatalf("normalizeAccount() = %#v", account)
	}
}

func TestResolveAccountPrefersUPDNationalNumberForMatchingIBAN(t *testing.T) {
	requested := normalizeAccount(Account{IBAN: "DE89370400440532013000"})
	if requested.AccountNumber != "0532013000" {
		t.Fatalf("derived account number = %q", requested.AccountNumber)
	}
	resolved := resolveAccount(requested, []Account{{
		IBAN:          requested.IBAN,
		AccountNumber: "532013000",
		Subaccount:    "EUR",
		CountryCode:   "280",
		BankCode:      "37040044",
	}})
	if resolved.AccountNumber != "532013000" || resolved.Subaccount != "EUR" {
		t.Fatalf("resolveAccount() = %#v", resolved)
	}
}

func TestHKKAZ6CanBeBuiltDirectlyFromGermanIBAN(t *testing.T) {
	account := normalizeAccount(Account{IBAN: "DE89370400440532013000"})
	command := buildHKKAZ(6, account, time.Time{}, time.Time{}, "")
	segments, err := parseSegments([]byte(strings.Replace(command, "{n}", "1", 1)))
	if err != nil {
		t.Fatal(err)
	}
	if len(segments) != 1 ||
		component(segments[0], 0, 0) != "0532013000" ||
		component(segments[0], 0, 2) != "280" ||
		component(segments[0], 0, 3) != "37040044" {
		t.Fatalf("HKKAZ6 = %#v", segments)
	}
}

func TestConfiguredTransactionDefaultsPopulateRequiredAccountComponents(t *testing.T) {
	client := &Client{config: Config{Compatibility: CompatibilityOptions{
		TransactionAccountDefaults: Account{
			BIC:        "PBNKDEFF",
			Subaccount: "EUR",
			Currency:   "EUR",
		},
	}}}
	account := client.transactionAccountDefaults(Account{
		IBAN:          "DE00100100100012345678",
		AccountNumber: "0012345678",
		Subaccount:    "00",
		CountryCode:   "280",
		BankCode:      "10010010",
	})
	if account.BIC != "PBNKDEFF" ||
		account.Subaccount != "EUR" ||
		account.Currency != "EUR" {
		t.Fatalf("transactionAccountDefaults() = %#v", account)
	}

	command := buildHKCAZ(
		account,
		[]string{"urn:iso:std:iso:20022:tech:xsd:camt.052.001.08"},
		time.Time{},
		time.Time{},
		"",
	)
	segments, err := parseSegments([]byte(strings.Replace(command, "{n}", "1", 1)))
	if err != nil {
		t.Fatal(err)
	}
	if len(segments) != 1 ||
		component(segments[0], 0, 1) != "PBNKDEFF" ||
		component(segments[0], 0, 3) != "EUR" {
		t.Fatalf("HKCAZ account = %#v", segments)
	}
}

func TestSplitDateRangeUsesBankMaximum(t *testing.T) {
	from := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, time.April, 10, 0, 0, 0, 0, time.UTC)
	ranges := splitDateRange(from, to, 90)
	if len(ranges) != 2 ||
		ranges[0].to.Format(time.DateOnly) != "2026-03-31" ||
		ranges[1].from.Format(time.DateOnly) != "2026-04-01" ||
		!ranges[1].to.Equal(to) {
		t.Fatalf("splitDateRange() = %#v", ranges)
	}
}

func TestCapabilitiesAreNegotiatedFromBPD(t *testing.T) {
	response, err := parseSegments([]byte(
		"HIKAZS:1:7+1+1+0+90:N:N'" +
			"HICAZS:2:1+1+1+0+180:N:N:urn?:iso?:std?:iso?:20022?:tech?:xsd?:camt.052.001.08'" +
			"HICCSS:3:1+1+1+0+urn?:iso?:std?:iso?:20022?:tech?:xsd?:pain.001.001.09'",
	))
	if err != nil {
		t.Fatal(err)
	}
	client := &Client{
		capabilities:    make(map[string]*Capability),
		tanRequirements: map[string]bool{"HKCCS": true},
	}
	client.updateCapabilities(response)

	if got := client.capabilities["HKKAZ"]; got == nil ||
		!containsInt(got.Versions, 7) || got.MaxDays != 90 {
		t.Fatalf("HKKAZ capability = %#v", got)
	}
	if got := client.capabilities["HKCAZ"]; got == nil ||
		len(got.Formats) != 1 || got.MaxDays != 180 {
		t.Fatalf("HKCAZ capability = %#v", got)
	}
	if got := client.capabilities["HKCCS"]; got == nil ||
		!got.TANRequired || !strings.Contains(got.Formats[0], "pain.001") {
		t.Fatalf("HKCCS capability = %#v", got)
	}
}

func TestParseCAMT052Transaction(t *testing.T) {
	document := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<Document xmlns="urn:iso:std:iso:20022:tech:xsd:camt.052.001.08">
  <BkToCstmrAcctRpt><Rpt><Ntry>
    <Amt Ccy="EUR">12.34</Amt><CdtDbtInd>DBIT</CdtDbtInd>
    <BookgDt><Dt>2026-07-22</Dt></BookgDt><ValDt><Dt>2026-07-23</Dt></ValDt>
    <NtryRef>ENTRY-1</NtryRef><AcctSvcrRef>BANK-1</AcctSvcrRef>
    <BkTxCd><Prtry><Cd>SEPA</Cd></Prtry></BkTxCd>
    <NtryDtls><TxDtls><Refs><EndToEndId>E2E-1</EndToEndId></Refs>
      <RltdPties><Cdtr><Nm>Example Shop</Nm></Cdtr>
      <CdtrAcct><Id><IBAN>DE89370400440532013000</IBAN></Id></CdtrAcct></RltdPties>
      <RmtInf><Ustrd>Invoice 42</Ustrd></RmtInf>
    </TxDtls></NtryDtls>
  </Ntry></Rpt></BkToCstmrAcctRpt>
</Document>`)
	transactions, err := parseCAMT(document)
	if err != nil {
		t.Fatal(err)
	}
	if len(transactions) != 1 || transactions[0].Amount != -1234 ||
		transactions[0].CounterpartyName != "Example Shop" ||
		transactions[0].Description != "Invoice 42" {
		t.Fatalf("parseCAMT() = %#v", transactions)
	}
}

func TestHICAZExtractsRepeatedBinaryCAMTDocuments(t *testing.T) {
	first := []byte(`<?xml version="1.0"?><Document><A>one:1</A></Document>`)
	second := []byte(`<?xml version="1.0"?><Document><A>two?2</A></Document>`)
	wire := "HICAZ:1:1+DE89370400440532013000::::280:37040044" +
		"+urn?:iso?:std?:iso?:20022?:tech?:xsd?:camt.052.001.08+" +
		binary(first) + ":" + binary(second) + "'"
	segments, err := parseSegments([]byte(wire))
	if err != nil {
		t.Fatal(err)
	}
	documents := camtDocuments(segments)
	if len(documents) != 2 || string(documents[0]) != string(first) ||
		string(documents[1]) != string(second) {
		t.Fatalf("camtDocuments() = %q", documents)
	}
}

type sepaMockBank struct {
	t        *testing.T
	mu       sync.Mutex
	requests [][]segment
}

func (m *sepaMockBank) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	body, err := io.ReadAll(request.Body)
	if err != nil {
		m.t.Fatal(err)
	}
	raw, err := base64.StdEncoding.DecodeString(string(body))
	if err != nil {
		m.t.Fatal(err)
	}
	segments, err := parseSegments(raw)
	if err != nil {
		m.t.Fatal(err)
	}
	m.mu.Lock()
	m.requests = append(m.requests, segments)
	m.mu.Unlock()

	var inner string
	switch {
	case len(findSegments(segments, "HKEND")) > 0:
		inner = "HIRMG:2:2+0100::Dialog ended'"
	case len(findSegments(segments, "HKSPA")) > 0:
		inner = "HIRMG:2:2+0010::Accepted'" +
			"HISPA:3:1:3+J:DE89370400440532013000:COBADEFFXXX:0532013000:00:280:37040044'"
	case len(findSegments(segments, "HKCCS")) > 0:
		inner = "HIRMG:2:2+0010::Accepted'HIRMS:3:2:3+0020::Transfer accepted:ORDER-1'"
	case len(findSegments(segments, "HKIDN")) > 0:
		inner = "HIRMG:2:2+0010::Accepted'" +
			"HIBPA:3:3:2+1+280:37040044+Dynamic Bank+0+1+300'" +
			"HICCSS:4:1:2+1+1+0+urn?:iso?:std?:iso?:20022?:tech?:xsd?:pain.001.001.09'"
	default:
		m.t.Fatalf("unexpected request: %#v", segments)
	}
	payload := responseMessage("sepa-dialog", inner)
	_, _ = writer.Write([]byte(base64.StdEncoding.EncodeToString([]byte(payload))))
}

func TestSEPACreditTransferNegotiatesPainAndBuildsHKCCS(t *testing.T) {
	bank := &sepaMockBank{t: t}
	config := testConfig(mockHTTPClient(bank))
	config.BankCode = "37040044"
	config.SystemID = "system-123"
	config.SecurityFunction = "999"
	client, err := NewClient(config)
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.SEPACreditTransfer(context.Background(), SEPACreditTransfer{
		Debtor:       Account{IBAN: "DE89370400440532013000", BIC: "COBADEFFXXX"},
		DebtorName:   "Ada Example",
		CreditorName: "Grace Example",
		CreditorIBAN: "DE12500105170648489890",
		CreditorBIC:  "INGDDEFFXXX",
		Amount:       1234,
		Purpose:      "Invoice 42",
	})
	if err != nil {
		t.Fatalf("SEPACreditTransfer() error = %v", err)
	}
	if !result.Accepted || !strings.HasSuffix(result.PainDescriptor, ".09") {
		t.Fatalf("transfer result = %#v", result)
	}

	for _, request := range bank.requests {
		matches := findSegments(request, "HKCCS")
		if len(matches) == 0 {
			continue
		}
		command := matches[0]
		if component(command, 0, 0) != "DE89370400440532013000" ||
			component(command, 0, 4) != "280" ||
			!command.Fields[2].binary {
			t.Fatalf("HKCCS = %#v", command)
		}
		xmlText := string(command.Fields[2].raw)
		for _, expected := range []string{
			`pain.001.001.09`,
			`<InstdAmt Ccy="EUR">12.34</InstdAmt>`,
			`<Cdtr><Nm>Grace Example</Nm></Cdtr>`,
			`<ReqdExctnDt><Dt>`,
		} {
			if !strings.Contains(xmlText, expected) {
				t.Fatalf("pain.001 XML lacks %q:\n%s", expected, xmlText)
			}
		}
		return
	}
	t.Fatal("bank received no HKCCS request")
}
