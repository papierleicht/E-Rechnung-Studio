package fints

import "testing"

func TestParseMT940(t *testing.T) {
	data := []byte(":20:START\n" +
		":25:DE00123456780000000000\n" +
		":28C:1/1\n" +
		":60F:C260101EUR1000,00\n" +
		":61:2601020102D12,34NTRFNONREF//BANKREF1\n" +
		":86:835?00SEPA TRANSFER?20Invoice 42?30GENODEF1XXX?31DE009999?32Example GmbH\n" +
		":61:2601030103C100,00NTRFEREF42//BANKREF2\n" +
		":86:835?20Refund?32Other Person\n" +
		":62F:C260103EUR1087,66\n-")

	transactions, err := parseMT940(data)
	if err != nil {
		t.Fatalf("parseMT940() error = %v", err)
	}
	if len(transactions) != 2 {
		t.Fatalf("got %d transactions, want 2", len(transactions))
	}
	first := transactions[0]
	if first.Amount != -1234 || first.Currency != "EUR" {
		t.Fatalf("first amount/currency = %d/%s, want -1234/EUR", first.Amount, first.Currency)
	}
	if first.BookingDate != "2026-01-02" || first.ValueDate != "2026-01-02" {
		t.Fatalf("first dates = %s/%s", first.BookingDate, first.ValueDate)
	}
	if first.Description != "Invoice 42" || first.CounterpartyName != "Example GmbH" {
		t.Fatalf("first description/name = %q/%q", first.Description, first.CounterpartyName)
	}
	if first.CounterpartyIBAN != "DE009999" || first.CounterpartyBankID != "GENODEF1XXX" {
		t.Fatalf("first counterparty identifiers = %q/%q", first.CounterpartyIBAN, first.CounterpartyBankID)
	}
	if transactions[1].Amount != 10000 {
		t.Fatalf("second amount = %d, want 10000", transactions[1].Amount)
	}
}

func TestParseMT940RejectsMalformedTransaction(t *testing.T) {
	_, err := parseMT940([]byte(":60F:C260101EUR0,00\n:61:not-a-transaction\n"))
	if err == nil {
		t.Fatal("parseMT940() error = nil, want error")
	}
}

func TestParseMT940FundsCodeAndReversalMarks(t *testing.T) {
	data := []byte(":60F:C240616EUR100,00\n" +
		":61:2406170617DR36,00N037NONREF\n" +
		":61:2406180618RD10,00NTRFREVERSAL-DEBIT\n" +
		":61:2406190619RC5,00NTRFREVERSAL-CREDIT\n")

	transactions, err := parseMT940(data)
	if err != nil {
		t.Fatalf("parseMT940() error = %v", err)
	}
	if len(transactions) != 3 {
		t.Fatalf("got %d transactions, want 3", len(transactions))
	}
	if got := transactions[0]; got.Amount != -3600 ||
		got.Type != "N037" ||
		got.CustomerReference != "NONREF" {
		t.Fatalf("funds-code transaction = %#v", got)
	}
	if transactions[1].Amount != 1000 {
		t.Fatalf("RD amount = %d, want 1000", transactions[1].Amount)
	}
	if transactions[2].Amount != -500 {
		t.Fatalf("RC amount = %d, want -500", transactions[2].Amount)
	}
}

func TestParseMinorUnits(t *testing.T) {
	tests := map[string]int64{
		"0,01":     1,
		"12,3":     1230,
		"999":      99900,
		"1.234,56": 123456,
	}
	for input, want := range tests {
		got, err := parseMinorUnits(input)
		if err != nil {
			t.Fatalf("parseMinorUnits(%q) error = %v", input, err)
		}
		if got != want {
			t.Fatalf("parseMinorUnits(%q) = %d, want %d", input, got, want)
		}
	}
}

func TestMT940NonemptyMalformedPayloadCannotBecomeSuccessfulEmptyImport(t *testing.T) {
	for _, payload := range []string{
		"random non-statement payload",
		"   \r\n",
		":99:unknown\r\n:ABC:also unknown\r\n",
		":20:reference\r\n:25:1234567890\r\n",
		":60F:C\r\n",
		":60F:C260901EUR\r\n",
		":60F:C260230EUR1,00\r\n:62F:C260901EUR1,00\r\n",
		":60F:C260901EURnot-amount\r\n:62F:C260901EUR1,00\r\n",
		":60F:C260901EUR1,001\r\n:62F:C260901EUR1,00\r\n",
		":60F:C260901EUR1,00\r\n",
		":60F:C260901EUR1,00\r\n:62F:C260901EUR1,00\r\n:60F:C260902EUR1,00\r\n",
		":62F:C260901EUR1,00\r\n",
		":60F:C260901EUR1,00\r\n:62F:truncated\r\n",
		":60F:C260901EUR1,00\r\n:62F:C260901USD1,00\r\n",
	} {
		transactions, err := parseMT940ForAccount([]byte(payload), Account{})
		if err == nil || transactions != nil {
			t.Fatalf("malformed payload accepted: %q => %#v / %v", payload, transactions, err)
		}
	}
}
func TestMT940ExplicitEmptyAndValidEmptyStatement(t *testing.T) {
	for _, payload := range [][]byte{nil, {}, []byte(":20:EMPTY\r\n:25:1234567890\r\n:60F:C260901EUR10,00\r\n:62F:C260901EUR10,00\r\n-")} {
		transactions, err := parseMT940ForAccount(payload, Account{AccountNumber: "1234567890"})
		if err != nil || len(transactions) != 0 {
			t.Fatalf("empty result rejected: %#v / %v", transactions, err)
		}
	}
}
