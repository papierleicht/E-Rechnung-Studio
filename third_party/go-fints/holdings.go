package fints

import (
	"context"
	"errors"
	"fmt"
)

// HoldingsReport preserves an original MT535 securities report. Values are not
// coerced into money: securities quantities and prices have their own precision.
type HoldingsReport struct {
	Format string `json:"format"`
	Data   []byte `json:"data"`
}

// Holdings retrieves original depot reports, including all continuation pages.
// Parsing securities, corporate actions, prices and currencies is left to a
// dedicated MT535 parser; this method does not claim normalized position data.
func (c *Client) Holdings(ctx context.Context, account Account) ([]HoldingsReport, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	d, account, err := c.openReadDialog(ctx, account)
	if err != nil {
		return nil, err
	}
	defer c.endDialog(ctx, d)
	if !supportsOperation(account, "HKWPD") {
		return nil, errors.New("account does not support HKWPD")
	}
	version, err := c.readOperationVersion("HKWPD", 5, 6)
	if err != nil {
		return nil, err
	}
	af, err := readAccountField(account, false)
	if err != nil {
		return nil, err
	}
	response, err := c.readPages(ctx, d, "HKWPD", func(cursor string) string { return fmt.Sprintf("HKWPD:{n}:%d+%s++++%s'", version, af, escape(cursor)) })
	if err != nil {
		return nil, err
	}
	var reports []HoldingsReport
	segments := findSegments(response, "HIWPD")
	if len(segments) == 0 && !hasResponseCode(response, "3010") {
		return nil, errors.New("bank returned no holdings report")
	}
	for _, s := range segments {
		if len(s.Fields) == 0 || !s.Fields[0].binary {
			return nil, errors.New("missing binary holdings report")
		}
		reports = append(reports, HoldingsReport{Format: "MT535", Data: append([]byte(nil), s.Fields[0].raw...)})
	}
	return reports, nil
}

// readPages refuses repeated continuation tokens rather than returning an
// incomplete result as successful. All callers are holding the client mutex.
func (c *Client) readPages(ctx context.Context, d *dialog, operation string, command func(string) string) ([]segment, error) {
	cursor := ""
	seen := map[string]bool{}
	var all []segment
	for page := 0; page < 100; page++ {
		response, err := c.executeCommand(ctx, d, command(cursor), operation)
		if err != nil {
			return nil, err
		}
		all = append(all, response...)
		next := touchdownPoint(response)
		if next == "" {
			return all, nil
		}
		if seen[next] {
			return nil, errors.New("FinTS pagination cursor repeated")
		}
		seen[next] = true
		cursor = next
	}
	return nil, errors.New("FinTS pagination exceeded 100 pages")
}
