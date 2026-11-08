package fints

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Money is an exact amount in the ISO currency's minor units.
type Money struct {
	Amount   int64  `json:"amount"`
	Currency string `json:"currency"`
}

// DatedBalance records a signed balance at the bank's stated date/time.
type DatedBalance struct {
	Money
	Date string `json:"date"`
	Time string `json:"time,omitempty"`
}

// AccountBalance distinguishes booked funds from pending and available funds.
type AccountBalance struct {
	Account     Account       `json:"account"`
	ProductName string        `json:"product_name,omitempty"`
	Booked      DatedBalance  `json:"booked"`
	Pending     *DatedBalance `json:"pending,omitempty"`
	CreditLimit *Money        `json:"credit_limit,omitempty"`
	Available   *Money        `json:"available,omitempty"`
	Used        *Money        `json:"used,omitempty"`
	Overdraft   *Money        `json:"overdraft,omitempty"`
	BookingDate string        `json:"booking_date,omitempty"`
	BookingTime string        `json:"booking_time,omitempty"`
	DueDate     string        `json:"due_date,omitempty"`
}

// Balance retrieves the booked balance and optional pending/available amounts.
// Only versions advertised by the bank and implemented here are requested.
func (c *Client) Balance(ctx context.Context, account Account) (*AccountBalance, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	d, account, err := c.openReadDialog(ctx, account)
	if err != nil {
		return nil, err
	}
	defer c.endDialog(ctx, d)
	if !supportsOperation(account, "HKSAL") {
		return nil, errors.New("account does not support HKSAL")
	}
	version, err := c.readOperationVersion("HKSAL", 5, 6, 7)
	if err != nil {
		return nil, err
	}
	af, err := readAccountField(account, version == 7)
	if err != nil {
		return nil, err
	}
	response, err := c.executeCommand(ctx, d, fmt.Sprintf("HKSAL:{n}:%d+%s+N'", version, af), "HKSAL")
	if err != nil {
		return nil, err
	}
	balances := findSegments(response, "HISAL")
	if len(balances) != 1 {
		return nil, errors.New("bank returned an ambiguous or missing balance")
	}
	return parseBalance(balances[0], account)
}

func (c *Client) openReadDialog(ctx context.Context, account Account) (*dialog, Account, error) {
	if account.IBAN == "" && account.AccountNumber == "" {
		return nil, account, errors.New("account requires an IBAN or account number")
	}
	account = normalizeAccount(account)
	if account.CountryCode == "" {
		account.CountryCode = c.config.CountryCode
	}
	if account.BankCode == "" {
		account.BankCode = c.config.BankCode
	}
	d, err := c.openDialog(ctx)
	if err != nil {
		return nil, account, err
	}
	return d, resolveAccount(account, d.accounts), nil
}

func (c *Client) readOperationVersion(operation string, supported ...int) (int, error) {
	capability := c.capabilities[operation]
	version := 0
	if capability != nil {
		for _, candidate := range supported {
			if candidate > version && containsInt(capability.Versions, candidate) {
				version = candidate
			}
		}
	}
	if version == 0 {
		return 0, fmt.Errorf("bank advertises no supported %s version", operation)
	}
	return version, nil
}

func readAccountField(account Account, international bool) (string, error) {
	if international {
		if account.IBAN == "" {
			return "", errors.New("operation requires an IBAN")
		}
		return buildKTI(account), nil
	}
	if account.AccountNumber == "" || account.CountryCode == "" || account.BankCode == "" {
		return "", errors.New("operation requires national account details")
	}
	return strings.Join([]string{escape(account.AccountNumber), escape(account.Subaccount), escape(account.CountryCode), escape(account.BankCode)}, ":"), nil
}

func parseBalance(s segment, account Account) (*AccountBalance, error) {
	if s.Header.Version < 5 || s.Header.Version > 7 {
		return nil, errors.New("unsupported HISAL response version")
	}
	if len(s.Fields) < 4 {
		return nil, errors.New("incomplete HISAL balance response")
	}
	// Verify the response belongs to the requested account before attaching it.
	if s.Header.Version == 7 {
		if iban := component(s, 0, 0); iban != "" && account.IBAN != "" && normalizeIBAN(iban) != account.IBAN {
			return nil, errors.New("balance response is for a different account")
		}
	} else if number := component(s, 0, 0); number != "" && account.AccountNumber != "" && strings.TrimLeft(number, "0") != strings.TrimLeft(account.AccountNumber, "0") {
		return nil, errors.New("balance response is for a different account")
	}
	booked, err := parseDatedBalance(s, 3)
	if err != nil {
		return nil, err
	}
	result := &AccountBalance{Account: account, ProductName: component(s, 1, 0), Booked: *booked}
	if component(s, 4, 0) != "" {
		result.Pending, err = parseDatedBalance(s, 4)
		if err != nil {
			return nil, err
		}
	}
	targets := []**Money{&result.CreditLimit, &result.Available, &result.Used}
	if s.Header.Version >= 6 {
		targets = append(targets, &result.Overdraft)
	}
	for i, target := range targets {
		if component(s, 5+i, 0) == "" {
			continue
		}
		m, err := parseBalanceMoney(component(s, 5+i, 0), component(s, 5+i, 1))
		if err != nil {
			return nil, err
		}
		*target = &m
	}
	if s.Header.Version == 5 {
		result.BookingDate, err = protocolDate(component(s, 8, 0))
		result.BookingTime = component(s, 9, 0)
	} else {
		result.BookingDate, err = protocolDate(component(s, 9, 0))
		result.BookingTime = component(s, 9, 1)
	}
	if err != nil {
		return nil, err
	}
	result.DueDate, err = protocolDate(component(s, 10, 0))
	if err != nil {
		return nil, err
	}
	return result, nil
}

func parseBalanceMoney(amount, currency string) (Money, error) {
	value, err := parseCurrencyAmount(amount, currency, ',')
	if err != nil {
		return Money{}, fmt.Errorf("invalid balance amount: %w", err)
	}
	return Money{Amount: value, Currency: currency}, nil
}
func parseDatedBalance(s segment, index int) (*DatedBalance, error) {
	mark := component(s, index, 0)
	if mark != "C" && mark != "D" {
		return nil, errors.New("invalid balance credit/debit indicator")
	}
	money, err := parseBalanceMoney(component(s, index, 1), component(s, index, 2))
	if err != nil {
		return nil, err
	}
	if mark == "D" {
		money.Amount = -money.Amount
	}
	date, err := protocolDate(component(s, index, 3))
	if err != nil {
		return nil, err
	}
	return &DatedBalance{Money: money, Date: date, Time: component(s, index, 4)}, nil
}
func protocolDate(value string) (string, error) {
	if value == "" {
		return "", nil
	}
	date, err := time.Parse("20060102", value)
	if err != nil {
		return "", errors.New("invalid FinTS calendar date")
	}
	return date.Format(time.DateOnly), nil
}
