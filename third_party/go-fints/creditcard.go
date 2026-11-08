package fints

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// CreditCardField preserves one field of a bank-specific DIKKU response.
// Binary fields contain decoded bytes. Non-binary fields contain FinTS wire
// bytes in Data and decoded components in Components. Data may contain personal
// financial information and must not be written to diagnostic logs.
type CreditCardField struct {
	Data       []byte   `json:"data"`
	Binary     bool     `json:"binary"`
	Components []string `json:"components,omitempty"`
}

// CreditCardResponse is a raw DIKKU response, not a normalized booked
// Transaction. DKKKU is a proprietary extension; its response layout and
// distinction between pending and booked entries depend on the bank.
type CreditCardResponse struct {
	Version int               `json:"version"`
	Fields  []CreditCardField `json:"fields"`
}

// CreditCardTransactions reads a bank's advertised DKKKU version 2 extension.
// It preserves raw response fields because a portable transaction schema is not
// established. Never import these responses as booked ledger entries without a
// bank-specific decoder. A zero date leaves that date boundary open.
func (c *Client) CreditCardTransactions(ctx context.Context, account Account, cardNumber string, from, to time.Time) ([]CreditCardResponse, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	cardNumber = strings.TrimSpace(cardNumber)
	if cardNumber == "" || len(cardNumber) > 64 || strings.ContainsAny(cardNumber, "\r\n\x00") {
		return nil, errors.New("a valid credit-card identifier is required")
	}
	if account.IBAN == "" && account.AccountNumber == "" {
		return nil, errors.New("credit-card retrieval requires an account")
	}
	if !from.IsZero() && !to.IsZero() && from.After(to) {
		return nil, errors.New("credit-card start date must not be after end date")
	}
	d, err := c.openDialog(ctx)
	if err != nil {
		return nil, err
	}
	defer c.endDialog(context.WithoutCancel(ctx), d)
	if _, err := c.readOperationVersion("DKKKU", 2); err != nil {
		return nil, err
	}
	account = resolveAccount(normalizeAccount(account), d.accounts)
	if !supportsOperation(account, "DKKKU") {
		return nil, errors.New("account does not advertise credit-card retrieval")
	}
	if account.CountryCode == "" {
		account.CountryCode = c.config.CountryCode
	}
	if account.BankCode == "" {
		account.BankCode = c.config.BankCode
	}
	accountField, err := readAccountField(account, false)
	if err != nil {
		return nil, err
	}
	touchdown := ""
	seen := map[string]bool{}
	result := make([]CreditCardResponse, 0)
	for page := 0; page < 100; page++ {
		command := buildCreditCardRequest(accountField, cardNumber, from, to, touchdown)
		response, err := c.executeCommand(ctx, d, command, "DKKKU")
		if err != nil {
			return nil, err
		}
		rows, err := creditCardResponses(response)
		if err != nil {
			return nil, err
		}
		result = append(result, rows...)
		next := touchdownPoint(response)
		if next == "" {
			return result, nil
		}
		if seen[next] {
			return nil, errors.New("credit-card pagination repeated a continuation token")
		}
		seen[next] = true
		touchdown = next
	}
	return nil, errors.New("credit-card pagination exceeded 100 pages")
}

func buildCreditCardRequest(accountField, cardNumber string, from, to time.Time, touchdown string) string {
	start, end := "", ""
	if !from.IsZero() {
		start = from.Format("20060102")
	}
	if !to.IsZero() {
		end = to.Format("20060102")
	}
	return fmt.Sprintf("DKKKU:{n}:2+%s+%s++%s+%s++%s'", accountField, escape(cardNumber), start, end, escape(touchdown))
}

func creditCardResponses(response []segment) ([]CreditCardResponse, error) {
	segments := findSegments(response, "DIKKU")
	if len(segments) == 0 {
		// FinTS explicitly distinguishes an empty result from a missing response.
		if hasResponseCode(response, "3010") {
			return []CreditCardResponse{}, nil
		}
		return nil, errors.New("bank returned no DIKKU credit-card data")
	}
	result := make([]CreditCardResponse, 0, len(segments))
	for _, s := range segments {
		if s.Header.Version != 2 {
			return nil, fmt.Errorf("unsupported DIKKU response version %d", s.Header.Version)
		}
		if len(s.Fields) == 0 {
			return nil, errors.New("bank returned an empty DIKKU segment")
		}
		row := CreditCardResponse{Version: s.Header.Version, Fields: make([]CreditCardField, 0, len(s.Fields))}
		for _, field := range s.Fields {
			row.Fields = append(row.Fields, CreditCardField{
				Data: append([]byte(nil), field.raw...), Binary: field.binary,
				Components: append([]string(nil), field.components...),
			})
		}
		result = append(result, row)
	}
	return result, nil
}
