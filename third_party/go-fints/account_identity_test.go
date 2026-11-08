package fints

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestMT940ExplicitAccountMismatchAbortsWholeImport(t *testing.T) {
	account := Account{IBAN: "DE00123456781234567890", BankCode: "12345678", AccountNumber: "1234567890"}
	base := ":25:DE00123456781234567890\r\n:60F:C260901EUR0,\r\n:61:2609010901C1,NTRFref//bank\r\n"
	for _, identifier := range []string{"DE00123456781234567891", "12345678/1234567891", "87654321/1234567890", "1234567891"} {
		transactions, err := parseMT940ForAccount([]byte(base+":25:"+identifier+"\r\n:61:2609010901C2,NTRFsecond//bank2\r\n"), account)
		if err == nil || transactions != nil {
			t.Fatalf("mismatching identifier %q returned partial data %#v / %v", identifier, transactions, err)
		}
		if strings.Contains(err.Error(), identifier) {
			t.Fatal("account identifier leaked into error")
		}
	}
	for _, identifier := range []string{"DE00123456781234567890", "12345678/001234567890", "/1234567890", "1234567890", ""} {
		transactions, err := parseMT940ForAccount([]byte(":25:"+identifier+"\r\n:60F:C260901EUR0,\r\n:61:2609010901C1,NTRFref\r\n"), account)
		if err != nil || len(transactions) != 1 {
			t.Fatalf("compatible/absent identity %q: %#v, %v", identifier, transactions, err)
		}
	}
}
func TestCAMTChecksEveryReportAccountBeforeReturningAnyData(t *testing.T) {
	account := Account{IBAN: "DE00123456781234567890", BankCode: "12345678", AccountNumber: "1234567890"}
	entry := `<Ntry><Amt Ccy="EUR">1.00</Amt><CdtDbtInd>CRDT</CdtDbtInd></Ntry>`
	for _, accountXML := range []string{
		`<Id><IBAN>DE00123456781234567891</IBAN></Id>`,
		`<Id><Othr><Id>1234567891</Id></Othr></Id>`,
	} {
		document := []byte(`<Document><BkToCstmrStmt><Stmt><Acct><Id><IBAN>DE00123456781234567890</IBAN></Id></Acct>` + entry + `</Stmt><Stmt><Acct>` + accountXML + `</Acct>` + entry + `</Stmt></BkToCstmrStmt></Document>`)
		transactions, err := parseCAMTForAccount(document, account)
		if err == nil || transactions != nil {
			t.Fatalf("mismatching second account returned partial data %#v / %v", transactions, err)
		}
	}
	for _, accountXML := range []string{"", `<Acct><Id><IBAN>DE00123456781234567890</IBAN></Id></Acct>`, `<Acct><Id><Othr><Id>001234567890</Id></Othr></Id></Acct>`} {
		transactions, err := parseCAMTForAccount(camtTestDocument(accountXML+entry), account)
		if err != nil || len(transactions) != 1 {
			t.Fatalf("compatible/absent identity: %#v, %v", transactions, err)
		}
	}
}
func TestTransactionsRejectWrongAccountOnLaterMT940Page(t *testing.T) {
	calls := 0
	client := readTestClient(t, "", func(request []segment) string {
		calls++
		if len(findSegments(request, "HKKAZ")) != 1 {
			t.Fatal("expected HKKAZ")
		}
		identifier := readTestAccount().IBAN
		if calls == 2 {
			identifier = "DE0012345678001234567891"
		}
		payload := ":25:" + identifier + "\r\n:60F:C260901EUR0,\r\n:61:2609010901C1,NTRFref\r\n"
		response := "HIKAZ:4:6:3+" + binary([]byte(payload)) + "'"
		if calls == 1 {
			response += "HIRMS:5:2:3+3040::More:next'"
		}
		return response
	})
	client.config.Compatibility.SkipTransactionSEPAAccountDiscovery = true
	transactions, err := client.Transactions(context.Background(), readTestAccount(), time.Time{}, time.Time{})
	if err == nil || transactions != nil || calls != 2 {
		t.Fatalf("cross-account page accepted: calls=%d %#v / %v", calls, transactions, err)
	}
}
func TestCAMTRejectsEnvelopeAndReportContradictionsAtomically(t *testing.T) {
	for _, wrong := range []string{"envelope", "report"} {
		t.Run(wrong, func(t *testing.T) {
			calls := 0
			client := readTestClient(t, "HICAZS:20:1+1+1+0'", func(request []segment) string {
				calls++
				if len(findSegments(request, "HKCAZ")) != 1 {
					t.Fatal("expected HKCAZ")
				}
				envelopeID, reportID := readTestAccount().IBAN, readTestAccount().IBAN
				if calls == 2 {
					if wrong == "envelope" {
						envelopeID = "DE0012345678001234567891"
					} else {
						reportID = "DE0012345678001234567891"
					}
				}
				doc := camtTestDocument(`<Acct><Id><IBAN>` + reportID + `</IBAN></Id></Acct><Ntry><Amt Ccy="EUR">1.00</Amt><CdtDbtInd>CRDT</CdtDbtInd></Ntry>`)
				response := "HICAZ:4:1:3+" + envelopeID + "+urn?:iso?:std?:iso?:20022?:tech?:xsd?:camt.053.001.08+" + binary(doc) + "'"
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
			transactions, err := client.transactionsCAMT(ctx, d, account, time.Time{}, time.Time{})
			if err == nil || transactions != nil || calls != 2 {
				t.Fatalf("cross-account page accepted: calls=%d %#v / %v", calls, transactions, err)
			}
		})
	}
}
