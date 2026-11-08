package fints

import (
	"strings"
	"testing"
)

func camtTestDocument(body string) []byte {
	return []byte(`<Document xmlns="urn:iso:std:iso:20022:tech:xsd:camt.053.001.08"><BkToCstmrStmt><Stmt>` + body + `</Stmt></BkToCstmrStmt></Document>`)
}
func TestCAMTBatchIsSplitOnlyWhenAmountsReconcile(t *testing.T) {
	entry := `<Ntry><Amt Ccy="EUR">30.00</Amt><CdtDbtInd>CRDT</CdtDbtInd><BookgDt><Dt>2026-09-01</Dt></BookgDt><NtryDtls>
 <TxDtls><Amt Ccy="EUR">10.00</Amt><Refs><EndToEndId>one</EndToEndId></Refs><RltdPties><Dbtr><Pty><Nm>One</Nm></Pty></Dbtr></RltdPties><RmtInf><Strd><CdtrRefInf><Ref>invoice-one</Ref></CdtrRefInf></Strd></RmtInf></TxDtls>
 <TxDtls><AmtDtls><TxAmt><Amt Ccy="EUR">20.00</Amt></TxAmt></AmtDtls><Refs><EndToEndId>two</EndToEndId></Refs><RltdPties><Dbtr><Nm>Two</Nm></Dbtr></RltdPties></TxDtls>
 </NtryDtls></Ntry>`
	transactions, err := parseCAMT(camtTestDocument(entry))
	if err != nil {
		t.Fatal(err)
	}
	if len(transactions) != 2 || transactions[0].Amount != 1000 || transactions[1].Amount != 2000 || transactions[0].CounterpartyName != "One" || transactions[0].Description != "invoice-one" {
		t.Fatalf("batch=%#v", transactions)
	}
	for _, altered := range []string{strings.Replace(entry, "20.00", "21.00", 1), strings.Replace(entry, `<Amt Ccy="EUR">10.00</Amt>`, "", 1), strings.Replace(entry, `<Amt Ccy="EUR">10.00</Amt>`, `<Amt Ccy="USD">10.00</Amt>`, 1)} {
		transactions, err := parseCAMT(camtTestDocument(altered))
		if err != nil {
			t.Fatal(err)
		}
		if len(transactions) != 1 || transactions[0].Amount != 3000 || transactions[0].CounterpartyName != "" || transactions[0].CustomerReference != "" {
			t.Fatalf("ambiguous aggregate falsely assigned=%#v", transactions)
		}
	}
}
func TestCAMTRejectsUnsupportedXMLAndAmbiguousMoney(t *testing.T) {
	for _, document := range [][]byte{
		[]byte(`<invoice><amount>1</amount></invoice>`), []byte(`<Document><Unknown/></Document>`),
		[]byte(`<Document xmlns="urn:unknown"><BkToCstmrStmt><Stmt/></BkToCstmrStmt></Document>`),
		camtTestDocument(`<Ntry><Amt Ccy="EUR">1.00</Amt><CdtDbtInd>CRDT</CdtDbtInd><BookgDt><Dt>2026-02-30</Dt></BookgDt></Ntry>`),
		camtTestDocument(`<Ntry><Amt Ccy="EUR">1.00</Amt><CdtDbtInd>CRDT</CdtDbtInd><Sts>UNKNOWN</Sts></Ntry>`),
		camtTestDocument(`<Ntry><Amt Ccy="EUR">1.23</Amt><CdtDbtInd>INVALID</CdtDbtInd></Ntry>`),
		camtTestDocument(`<Ntry><Amt Ccy="EUR">1.234</Amt><CdtDbtInd>CRDT</CdtDbtInd></Ntry>`),
		camtTestDocument(`<Ntry><Amt Ccy="EUR">-1.00</Amt><CdtDbtInd>CRDT</CdtDbtInd></Ntry>`),
		camtTestDocument(`<Ntry><Amt Ccy="EUR">92233720368547758.08</Amt><CdtDbtInd>CRDT</CdtDbtInd></Ntry>`),
	} {
		if _, err := parseCAMT(document); err == nil {
			t.Fatalf("invalid document accepted: %s", document)
		}
	}
}
func TestCAMTCurrencyPrecisionAndPendingFilter(t *testing.T) {
	transactions, err := parseCAMT(camtTestDocument(`<Ntry><Amt Ccy="JPY">123</Amt><CdtDbtInd>CRDT</CdtDbtInd><Sts><Cd>BOOK</Cd></Sts></Ntry><Ntry><Amt Ccy="KWD">1.234</Amt><CdtDbtInd>DBIT</CdtDbtInd></Ntry><Ntry><Amt Ccy="EUR">10.00</Amt><CdtDbtInd>DBIT</CdtDbtInd><Sts>PDNG</Sts></Ntry>`))
	if err != nil {
		t.Fatal(err)
	}
	if len(transactions) != 2 || transactions[0].Amount != 123 || transactions[1].Amount != -1234 {
		t.Fatalf("amounts=%#v", transactions)
	}
}
func TestExactMinorUnitsRejectTruncationAndOverflow(t *testing.T) {
	for _, value := range []string{"1,001", "-1,00", "1,2,3", "1.2,00", "92233720368547758,08"} {
		if _, err := parseMinorUnits(value); err == nil {
			t.Fatalf("invalid amount accepted: %s", value)
		}
	}
	for value, want := range map[string]int64{"1,2300": 123, "92233720368547758,07": 9223372036854775807, "0,": 0} {
		got, err := parseMinorUnits(value)
		if err != nil || got != want {
			t.Fatalf("%s = %d, %v", value, got, err)
		}
	}
}

func TestFormatMinorUnitsHandlesMinimumInt64(t *testing.T) {
	if got := formatMinorUnits(-9223372036854775808); got != "-92233720368547758.08" {
		t.Fatalf("format=%q", got)
	}
}

func TestMT940DecodesLatin1AndNonEuroMinorUnits(t *testing.T) {
	data := []byte(":60F:C260901JPY1000,\r\n:61:2609010901C123,NTRFref//bank\r\n:86:835?20B\xfcro?32M\xfcller\r\n")
	transactions, err := parseMT940(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(transactions) != 1 || transactions[0].Amount != 123 || transactions[0].Description != "Büro" || transactions[0].CounterpartyName != "Müller" {
		t.Fatalf("transaction=%#v", transactions)
	}
}
