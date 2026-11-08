package fints

import (
	"context"
	"errors"
	"fmt"
	"strconv"
)

// StatementInfo is one available electronic statement from HKKAU.
type StatementInfo struct {
	Number       int    `json:"number"`
	Year         int    `json:"year,omitempty"`
	Confirmation string `json:"confirmation"`
	Available    bool   `json:"available"`
	CreatedDate  string `json:"created_date,omitempty"`
	CreatedTime  string `json:"created_time,omitempty"`
	CreationType string `json:"creation_type,omitempty"`
}

const (
	StatementFormatMT940   = "1"
	StatementFormatISO8583 = "2"
	StatementFormatPDF     = "3"
)

// StatementRequest identifies a statement. Format may be empty for bank default.
type StatementRequest struct {
	Number int    `json:"number"`
	Year   int    `json:"year"`
	Format string `json:"format,omitempty"`
}

// ElectronicStatement preserves the bank's original document and receipt token.
// Fetching never acknowledges receipt automatically; ConfirmationCode is retained
// so a separate, explicitly implemented receipt workflow can be added later.
type ElectronicStatement struct {
	Format           string `json:"format"`
	PeriodStart      string `json:"period_start,omitempty"`
	PeriodEnd        string `json:"period_end,omitempty"`
	CreatedDate      string `json:"created_date,omitempty"`
	Year             int    `json:"year,omitempty"`
	Number           int    `json:"number,omitempty"`
	Data             []byte `json:"data"`
	StatementInfo    string `json:"statement_info,omitempty"`
	CustomerInfo     string `json:"customer_info,omitempty"`
	AdvertisingText  string `json:"advertising_text,omitempty"`
	IBAN             string `json:"iban,omitempty"`
	BIC              string `json:"bic,omitempty"`
	Name             string `json:"name,omitempty"`
	ConfirmationCode []byte `json:"confirmation_code,omitempty"`
}

func (c *Client) StatementList(ctx context.Context, account Account) ([]StatementInfo, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	d, account, err := c.openReadDialog(ctx, account)
	if err != nil {
		return nil, err
	}
	defer c.endDialog(ctx, d)
	if !supportsOperation(account, "HKKAU") {
		return nil, errors.New("account does not support HKKAU")
	}
	version, err := c.readOperationVersion("HKKAU", 1, 2)
	if err != nil {
		return nil, err
	}
	af, err := readAccountField(account, version == 2)
	if err != nil {
		return nil, err
	}
	response, err := c.readPages(ctx, d, "HKKAU", func(cursor string) string { return fmt.Sprintf("HKKAU:{n}:%d+%s++%s'", version, af, escape(cursor)) })
	if err != nil {
		return nil, err
	}
	var result []StatementInfo
	segments := findSegments(response, "HIKAU")
	if len(segments) == 0 && !hasResponseCode(response, "3010") {
		return nil, errors.New("bank returned no statement list")
	}
	for _, s := range segments {
		number, err := strconv.Atoi(component(s, 0, 0))
		if err != nil || number < 1 {
			return nil, errors.New("invalid statement number")
		}
		date, err := protocolDate(component(s, 4, 0))
		if err != nil {
			return nil, err
		}
		year, err := optionalPositiveInt(component(s, 3, 0))
		if err != nil {
			return nil, err
		}
		available := component(s, 2, 0)
		if available != "J" && available != "N" {
			return nil, errors.New("invalid statement availability")
		}
		result = append(result, StatementInfo{Number: number, Year: year, Confirmation: component(s, 1, 0), Available: available == "J", CreatedDate: date, CreatedTime: component(s, 5, 0), CreationType: component(s, 6, 0)})
	}
	return result, nil
}
func (c *Client) Statement(ctx context.Context, account Account, request StatementRequest) ([]ElectronicStatement, error) {
	if request.Number < 1 || request.Number > 99999 || request.Year < 1000 || request.Year > 9999 {
		return nil, errors.New("statement requires a valid number and four-digit year")
	}
	if request.Format != "" && request.Format != StatementFormatMT940 && request.Format != StatementFormatISO8583 && request.Format != StatementFormatPDF {
		return nil, errors.New("unsupported statement format")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	d, account, err := c.openReadDialog(ctx, account)
	if err != nil {
		return nil, err
	}
	defer c.endDialog(ctx, d)
	if !supportsOperation(account, "HKEKA") {
		return nil, errors.New("account does not support HKEKA")
	}
	version, err := c.readOperationVersion("HKEKA", 3, 4, 5)
	if err != nil {
		return nil, err
	}
	af, err := readAccountField(account, version >= 4)
	if err != nil {
		return nil, err
	}
	response, err := c.readPages(ctx, d, "HKEKA", func(cursor string) string {
		return fmt.Sprintf("HKEKA:{n}:%d+%s+%s+%d+%d++%s'", version, af, request.Format, request.Number, request.Year, escape(cursor))
	})
	if err != nil {
		return nil, err
	}
	var result []ElectronicStatement
	for _, s := range findSegments(response, "HIEKA") {
		statement, err := parseElectronicStatement(s)
		if err != nil {
			return nil, err
		}
		result = append(result, statement)
	}
	if len(result) == 0 {
		return nil, errors.New("bank returned no electronic statement")
	}
	return result, nil
}
func parseElectronicStatement(s segment) (ElectronicStatement, error) {
	result := ElectronicStatement{Format: component(s, 0, 0)}
	var err error
	result.PeriodStart, err = protocolDate(component(s, 1, 0))
	if err != nil {
		return result, err
	}
	result.PeriodEnd, err = protocolDate(component(s, 1, 1))
	if err != nil {
		return result, err
	}
	dataIndex := 2
	switch s.Header.Version {
	case 3, 4:
	case 5:
		dataIndex = 5
		result.CreatedDate, err = protocolDate(component(s, 2, 0))
		if err != nil {
			return result, err
		}
		result.Year, err = optionalPositiveInt(component(s, 3, 0))
		if err != nil {
			return result, err
		}
		result.Number, err = optionalPositiveInt(component(s, 4, 0))
		if err != nil {
			return result, err
		}
	default:
		return result, errors.New("unsupported HIEKA response version")
	}
	if len(s.Fields) <= dataIndex || !s.Fields[dataIndex].binary {
		return result, errors.New("statement document is not binary data")
	}
	result.Data = append([]byte(nil), s.Fields[dataIndex].raw...)
	result.StatementInfo = component(s, dataIndex+1, 0)
	result.CustomerInfo = component(s, dataIndex+2, 0)
	result.AdvertisingText = component(s, dataIndex+3, 0)
	result.IBAN = component(s, dataIndex+4, 0)
	result.BIC = component(s, dataIndex+5, 0)
	for i := 6; i <= 8; i++ {
		if value := component(s, dataIndex+i, 0); value != "" {
			if result.Name != "" {
				result.Name += " "
			}
			result.Name += value
		}
	}
	if len(s.Fields) > dataIndex+9 && s.Fields[dataIndex+9].binary {
		result.ConfirmationCode = append([]byte(nil), s.Fields[dataIndex+9].raw...)
	}
	return result, nil
}
func optionalPositiveInt(value string) (int, error) {
	if value == "" {
		return 0, nil
	}
	number, err := strconv.Atoi(value)
	if err != nil || number < 0 {
		return 0, errors.New("invalid numeric FinTS field")
	}
	return number, nil
}
