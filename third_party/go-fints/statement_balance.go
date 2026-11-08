package fints

import (
	"encoding/xml"
	"strings"
	"time"
)

// StatementBalance returns the latest booked closing balance contained in the
// transaction data of the most recent Transactions call (MT940 :62F: or CAMT
// CLBD/ITBD), or nil if the bank sent none. Unlike Balance it needs no
// additional request and therefore no additional authorisation.
func (c *Client) StatementBalance() *DatedBalance {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.statementBalance == nil {
		return nil
	}
	b := *c.statementBalance
	return &b
}

// laterBalance keeps the balance with the later date; on equal dates the one
// seen later in the bank's response wins.
func laterBalance(current, next *DatedBalance) *DatedBalance {
	if next == nil {
		return current
	}
	if current == nil || next.Date >= current.Date {
		return next
	}
	return current
}

// mt940ClosingBalance reads final closing balances (:62F:). Intermediate
// balances (:62M:) only close a page of a longer statement.
func mt940ClosingBalance(data []byte) *DatedBalance {
	var result *DatedBalance
	for _, tag := range scanMT940Tags(decodeLatin1(data)) {
		if tag.name != "62F" {
			continue
		}
		value := strings.TrimSpace(tag.value)
		if _, err := validateMT940Balance(value); err != nil {
			continue
		}
		date, _ := time.Parse("060102", value[1:7])
		currency := value[7:10]
		amount, err := parseCurrencyAmount(value[10:], currency, ',')
		if err != nil {
			continue
		}
		if value[0] == 'D' {
			amount = -amount
		}
		result = laterBalance(result, &DatedBalance{Money: Money{Amount: amount, Currency: currency}, Date: date.Format(time.DateOnly)})
	}
	return result
}

type camtBalanceDocument struct {
	Statements []camtBalanceStatement `xml:"BkToCstmrStmt>Stmt"`
	Reports    []camtBalanceStatement `xml:"BkToCstmrAcctRpt>Rpt"`
}

type camtBalanceStatement struct {
	Balances []struct {
		Type        string     `xml:"Tp>CdOrPrtry>Cd"`
		Amount      camtAmount `xml:"Amt"`
		CreditDebit string     `xml:"CdtDbtInd"`
		Date        camtDate   `xml:"Dt"`
	} `xml:"Bal"`
}

// camtClosingBalance reads closing (CLBD) or interim (ITBD) booked balances.
func camtClosingBalance(data []byte) *DatedBalance {
	var document camtBalanceDocument
	if xml.Unmarshal(data, &document) != nil {
		return nil
	}
	var result *DatedBalance
	for _, statement := range append(document.Statements, document.Reports...) {
		for _, b := range statement.Balances {
			if b.Type != "CLBD" && b.Type != "ITBD" {
				continue
			}
			currency := strings.TrimSpace(b.Amount.Currency)
			amount, err := parseCurrencyAmount(strings.TrimSpace(b.Amount.Value), currency, '.')
			if err != nil {
				continue
			}
			if b.CreditDebit == "DBIT" {
				amount = -amount
			}
			date := strings.TrimSpace(b.Date.Date)
			if date == "" && len(b.Date.DateTime) >= 10 {
				date = b.Date.DateTime[:10]
			}
			if _, err := time.Parse(time.DateOnly, date); err != nil {
				continue
			}
			result = laterBalance(result, &DatedBalance{Money: Money{Amount: amount, Currency: currency}, Date: date})
		}
	}
	return result
}
