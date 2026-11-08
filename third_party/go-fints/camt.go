package fints

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"strings"
	"time"
)

func (c *Client) transactionsCAMT(ctx context.Context, d *dialog, account Account, from, to time.Time) ([]Transaction, error) {
	if !supportsOperation(account, "HKCAZ") {
		return nil, errors.New("account does not support HKCAZ")
	}
	maxDays := 0
	if capability := c.capabilities["HKCAZ"]; capability != nil {
		maxDays = capability.MaxDays
	}
	result := make([]Transaction, 0)
	for _, dateRange := range splitDateRange(from, to, maxDays) {
		transactions, err := c.transactionsCAMTRange(ctx, d, account, dateRange.from, dateRange.to)
		if err != nil {
			return nil, err
		}
		result = append(result, transactions...)
	}
	return result, nil
}

func (c *Client) transactionsCAMTRange(
	ctx context.Context,
	d *dialog,
	account Account,
	from, to time.Time,
) ([]Transaction, error) {
	capability := c.capabilities["HKCAZ"]
	if capability == nil || len(capability.Versions) == 0 {
		return nil, errors.New("bank advertises neither a usable HKKAZ nor HKCAZ transaction operation")
	}
	if !containsInt(capability.Versions, 1) {
		return nil, errors.New("bank advertises no supported HKCAZ version")
	}
	if account.IBAN == "" {
		return nil, errors.New("HKCAZ requires an IBAN")
	}
	formats := capability.Formats
	if len(formats) == 0 {
		formats = []string{"urn:iso:std:iso:20022:tech:xsd:camt.052.001.02"}
	}

	touchdown := ""
	seen := map[string]bool{}
	var transactions []Transaction
	for page := 0; page < 100; page++ {
		command := buildHKCAZ(account, formats, from, to, touchdown)
		response, err := c.executeCommand(ctx, d, command, "HKCAZ")
		if err != nil {
			return nil, err
		}
		if err := checkCAMTResponseAccounts(response, account); err != nil {
			return nil, err
		}
		documents := camtDocuments(response)
		if len(documents) == 0 && !hasResponseCode(response, "3010") {
			return nil, errors.New("bank returned no HICAZ CAMT transaction data")
		}
		for _, document := range documents {
			parsed, err := parseCAMTForAccount(document, account)
			if err != nil {
				return nil, fmt.Errorf("parse CAMT transaction data: %w", err)
			}
			transactions = append(transactions, parsed...)
			c.statementBalance = laterBalance(c.statementBalance, camtClosingBalance(document))
		}
		next := touchdownPoint(response)
		if next == "" {
			return transactions, nil
		}
		if seen[next] {
			return nil, errors.New("FinTS CAMT pagination cursor repeated")
		}
		seen[next] = true
		touchdown = next
	}
	return nil, errors.New("FinTS CAMT pagination exceeded 100 pages")
}

func buildHKCAZ(account Account, formats []string, from, to time.Time, touchdown string) string {
	dateFrom, dateTo := "", ""
	if !from.IsZero() {
		dateFrom = from.Format("20060102")
	}
	if !to.IsZero() {
		dateTo = to.Format("20060102")
	}
	escapedFormats := make([]string, 0, len(formats))
	for _, format := range formats {
		escapedFormats = append(escapedFormats, escape(format))
	}
	return fmt.Sprintf(
		"HKCAZ:{n}:1+%s+%s+N+%s+%s++%s'",
		buildKTI(account),
		strings.Join(escapedFormats, ":"),
		dateFrom,
		dateTo,
		escape(touchdown),
	)
}

func buildKTI(account Account) string {
	country := account.CountryCode
	if country == "" {
		country = ibanCountryToFinTS(account.IBAN[:min(2, len(account.IBAN))])
	}
	return strings.Join([]string{
		escape(account.IBAN),
		escape(account.BIC),
		escape(account.AccountNumber),
		escape(account.Subaccount),
		escape(country),
		escape(account.BankCode),
	}, ":")
}

func camtDocuments(response []segment) [][]byte {
	var documents [][]byte
	for _, s := range findSegments(response, "HICAZ") {
		if len(s.Fields) < 3 {
			continue
		}
		field := s.Fields[2]
		if field.binary {
			documents = append(documents, append([]byte(nil), field.raw...))
			continue
		}
		components, err := splitWire(field.raw, ':')
		if err != nil {
			continue
		}
		for _, value := range components {
			if content, ok := binaryContent(value); ok {
				documents = append(documents, append([]byte(nil), content...))
			}
		}
	}
	return documents
}

type camtDocument struct {
	XMLName    xml.Name        `xml:"Document"`
	Statements []camtStatement `xml:"BkToCstmrStmt>Stmt"`
	Reports    []camtStatement `xml:"BkToCstmrAcctRpt>Rpt"`
}

type camtStatement struct {
	IBAN           string      `xml:"Acct>Id>IBAN"`
	OtherAccountID string      `xml:"Acct>Id>Othr>Id"`
	Entries        []camtEntry `xml:"Ntry"`
}

type camtEntry struct {
	Amount      camtAmount   `xml:"Amt"`
	CreditDebit string       `xml:"CdtDbtInd"`
	Status      camtStatus   `xml:"Sts"`
	BookingDate camtDate     `xml:"BookgDt"`
	ValueDate   camtDate     `xml:"ValDt"`
	EntryRef    string       `xml:"NtryRef"`
	AccountRef  string       `xml:"AcctSvcrRef"`
	Additional  string       `xml:"AddtlNtryInf"`
	BankCode    camtBankCode `xml:"BkTxCd"`
	Details     []camtTx     `xml:"NtryDtls>TxDtls"`
}

type camtStatus struct {
	Value string `xml:",chardata"`
	Code  string `xml:"Cd"`
}

type camtAmount struct {
	Currency string `xml:"Ccy,attr"`
	Value    string `xml:",chardata"`
}

type camtDate struct {
	Date     string `xml:"Dt"`
	DateTime string `xml:"DtTm"`
}

type camtBankCode struct {
	Domain string `xml:"Domn>Cd"`
	Family string `xml:"Domn>Fmly>Cd"`
	Sub    string `xml:"Domn>Fmly>SubFmlyCd"`
	Custom string `xml:"Prtry>Cd"`
}

type camtTx struct {
	Amount              camtAmount `xml:"AmtDtls>TxAmt>Amt"`
	DirectAmount        camtAmount `xml:"Amt"`
	CreditDebit         string     `xml:"CdtDbtInd"`
	CreditorPartyName   string     `xml:"RltdPties>Cdtr>Pty>Nm"`
	DebtorPartyName     string     `xml:"RltdPties>Dbtr>Pty>Nm"`
	StructuredReference []string   `xml:"RmtInf>Strd>CdtrRefInf>Ref"`
	EndToEnd            string     `xml:"Refs>EndToEndId"`
	Transaction         string     `xml:"Refs>TxId"`
	AccountRef          string     `xml:"Refs>AcctSvcrRef"`
	CreditorName        string     `xml:"RltdPties>Cdtr>Nm"`
	DebtorName          string     `xml:"RltdPties>Dbtr>Nm"`
	CreditorIBAN        string     `xml:"RltdPties>CdtrAcct>Id>IBAN"`
	DebtorIBAN          string     `xml:"RltdPties>DbtrAcct>Id>IBAN"`
	CreditorBIC         string     `xml:"RltdAgts>CdtrAgt>FinInstnId>BIC"`
	CreditorBICFI       string     `xml:"RltdAgts>CdtrAgt>FinInstnId>BICFI"`
	DebtorBIC           string     `xml:"RltdAgts>DbtrAgt>FinInstnId>BIC"`
	DebtorBICFI         string     `xml:"RltdAgts>DbtrAgt>FinInstnId>BICFI"`
	Purpose             []string   `xml:"RmtInf>Ustrd"`
}

func parseCAMT(data []byte) ([]Transaction, error) {
	return parseCAMTForAccount(data, Account{})
}

func parseCAMTForAccount(data []byte, account Account) ([]Transaction, error) {
	var document camtDocument
	if err := xml.Unmarshal(data, &document); err != nil {
		return nil, err
	}
	if namespace := document.XMLName.Space; namespace != "" && !strings.HasPrefix(namespace, "urn:iso:std:iso:20022:tech:xsd:camt.052.") && !strings.HasPrefix(namespace, "urn:iso:std:iso:20022:tech:xsd:camt.053.") {
		return nil, errors.New("unsupported CAMT XML namespace")
	}
	if len(document.Statements) == 0 && len(document.Reports) == 0 {
		return nil, errors.New("CAMT document contains no supported statement or report")
	}
	statements := append(document.Statements, document.Reports...)
	var result []Transaction
	for _, statement := range statements {
		if err := checkResponseAccount(account, Account{IBAN: statement.IBAN}); err != nil {
			return nil, err
		}
		if err := checkMT940AccountIdentifier(statement.OtherAccountID, account); err != nil {
			return nil, err
		}
		for _, entry := range statement.Entries {
			status := strings.TrimSpace(firstNonEmpty(entry.Status.Code, entry.Status.Value))
			if status == "PDNG" || status == "INFO" {
				continue
			}
			if status != "" && status != "BOOK" {
				return nil, errors.New("unsupported CAMT entry status")
			}
			transactions, err := camtEntryTransactions(entry)
			if err != nil {
				return nil, err
			}
			result = append(result, transactions...)
		}
	}
	return result, nil
}

// A batch entry is split only if every detail has an explicit amount in the
// entry currency and the signed total reconciles. Otherwise retain the bank's
// aggregate entry without falsely assigning its full amount to the first payee.
func camtEntryTransactions(entry camtEntry) ([]Transaction, error) {
	aggregate, err := camtEntryTransaction(entry)
	if err != nil {
		return nil, err
	}
	if len(entry.Details) <= 1 {
		return []Transaction{aggregate}, nil
	}
	var result []Transaction
	var sum int64
	for _, detail := range entry.Details {
		amount := detail.DirectAmount
		if amount.Value == "" {
			amount = detail.Amount
		}
		if amount.Value == "" || amount.Currency != entry.Amount.Currency {
			return []Transaction{aggregate}, nil
		}
		individual := entry
		individual.Amount = amount
		individual.Details = []camtTx{detail}
		if detail.CreditDebit != "" {
			individual.CreditDebit = detail.CreditDebit
		}
		tx, err := camtEntryTransaction(individual)
		if err != nil {
			return nil, err
		}
		next := sum + tx.Amount
		if (tx.Amount > 0 && next < sum) || (tx.Amount < 0 && next > sum) {
			return nil, errors.New("CAMT batch total overflow")
		}
		sum = next
		result = append(result, tx)
	}
	if sum != aggregate.Amount {
		return []Transaction{aggregate}, nil
	}
	return result, nil
}

func camtEntryTransaction(entry camtEntry) (Transaction, error) {
	if entry.CreditDebit != "DBIT" && entry.CreditDebit != "CRDT" {
		return Transaction{}, errors.New("invalid CAMT credit/debit indicator")
	}
	amountText := strings.TrimSpace(entry.Amount.Value)
	amount, err := parseCurrencyAmount(amountText, entry.Amount.Currency, '.')
	if err != nil {
		return Transaction{}, fmt.Errorf("invalid CAMT amount: %w", err)
	}
	if entry.CreditDebit == "DBIT" {
		amount = -amount
	}
	for _, date := range []string{entry.BookingDate.value(), entry.ValueDate.value()} {
		if date != "" {
			if _, err := time.Parse(time.DateOnly, date); err != nil {
				return Transaction{}, errors.New("invalid CAMT date")
			}
		}
	}
	tx := Transaction{
		BookingDate:       entry.BookingDate.value(),
		ValueDate:         entry.ValueDate.value(),
		Amount:            amount,
		Currency:          entry.Amount.Currency,
		CustomerReference: entry.EntryRef,
		BankReference:     entry.AccountRef,
		Description:       strings.TrimSpace(entry.Additional),
		Type:              entry.BankCode.value(),
	}
	if len(entry.Details) != 1 {
		return tx, nil
	}
	detail := entry.Details[0]
	if detail.EndToEnd != "" && detail.EndToEnd != "NOTPROVIDED" {
		tx.CustomerReference = detail.EndToEnd
	} else if detail.Transaction != "" {
		tx.CustomerReference = detail.Transaction
	}
	if detail.AccountRef != "" {
		tx.BankReference = detail.AccountRef
	}
	if entry.CreditDebit == "DBIT" {
		tx.CounterpartyName = firstNonEmpty(detail.CreditorName, detail.CreditorPartyName)
		tx.CounterpartyIBAN = detail.CreditorIBAN
		tx.CounterpartyBankID = firstNonEmpty(detail.CreditorBICFI, detail.CreditorBIC)
	} else {
		tx.CounterpartyName = firstNonEmpty(detail.DebtorName, detail.DebtorPartyName)
		tx.CounterpartyIBAN = detail.DebtorIBAN
		tx.CounterpartyBankID = firstNonEmpty(detail.DebtorBICFI, detail.DebtorBIC)
	}
	if len(detail.Purpose) > 0 {
		tx.Description = strings.Join(detail.Purpose, " ")
	} else if len(detail.StructuredReference) > 0 {
		tx.Description = strings.Join(detail.StructuredReference, " ")
	}
	return tx, nil
}

func (date camtDate) value() string {
	value := firstNonEmpty(date.Date, date.DateTime)
	if len(value) >= len(time.DateOnly) {
		return value[:len(time.DateOnly)]
	}
	return value
}

func (code camtBankCode) value() string {
	if code.Custom != "" {
		return code.Custom
	}
	var values []string
	for _, value := range []string{code.Domain, code.Family, code.Sub} {
		if value != "" {
			values = append(values, value)
		}
	}
	return strings.Join(values, "/")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
