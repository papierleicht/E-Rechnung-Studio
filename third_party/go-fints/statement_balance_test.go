package fints

import "testing"

func TestMT940ClosingBalanceUsesLatestFinalBalance(t *testing.T) {
	data := []byte(":20:STARTUMS\n:25:12030000/1234567890\n:28C:0\n" +
		":60F:C261020EUR1000,00\n:61:2610201020CR87,66NMSCNONREF\n:62M:C261020EUR1087,66\n" +
		":60M:C261020EUR1087,66\n:61:2610211021DR1200,00NMSCNONREF\n:62F:D261021EUR112,34\n-\n" +
		":20:STARTUMS\n:60F:D261021EUR112,34\n:62F:D261019EUR5,00\n-")
	b := mt940ClosingBalance(data)
	if b == nil || b.Amount != -11234 || b.Currency != "EUR" || b.Date != "2026-10-21" {
		t.Fatalf("closing balance %+v", b)
	}
	if mt940ClosingBalance([]byte(":60F:C261020EUR1,00\n:62M:C261020EUR1,00\n")) != nil {
		t.Fatal("an intermediate balance is no closing balance")
	}
}

func TestCAMTClosingBalance(t *testing.T) {
	data := []byte(`<Document xmlns="urn:iso:std:iso:20022:tech:xsd:camt.052.001.02"><BkToCstmrAcctRpt><Rpt>
<Bal><Tp><CdOrPrtry><Cd>PRCD</Cd></CdOrPrtry></Tp><Amt Ccy="EUR">10.00</Amt><CdtDbtInd>CRDT</CdtDbtInd><Dt><Dt>2026-10-01</Dt></Dt></Bal>
<Bal><Tp><CdOrPrtry><Cd>CLBD</Cd></CdOrPrtry></Tp><Amt Ccy="EUR">8420.17</Amt><CdtDbtInd>CRDT</CdtDbtInd><Dt><Dt>2026-10-26</Dt></Dt></Bal>
</Rpt></BkToCstmrAcctRpt></Document>`)
	b := camtClosingBalance(data)
	if b == nil || b.Amount != 842017 || b.Date != "2026-10-26" {
		t.Fatalf("closing balance %+v", b)
	}
	debit := []byte(`<Document><BkToCstmrStmt><Stmt><Bal><Tp><CdOrPrtry><Cd>ITBD</Cd></CdOrPrtry></Tp><Amt Ccy="EUR">5.5</Amt><CdtDbtInd>DBIT</CdtDbtInd><Dt><DtTm>2026-10-26T10:00:00</DtTm></Dt></Bal></Stmt></BkToCstmrStmt></Document>`)
	if b := camtClosingBalance(debit); b == nil || b.Amount != -550 || b.Date != "2026-10-26" {
		t.Fatalf("debit balance %+v", b)
	}
}

func TestStatementBalanceIsACopy(t *testing.T) {
	c := &Client{statementBalance: &DatedBalance{Money: Money{Amount: 1, Currency: "EUR"}, Date: "2026-10-26"}}
	b := c.StatementBalance()
	b.Amount = 99
	if c.StatementBalance().Amount != 1 {
		t.Fatal("caller modified the client's balance")
	}
}
