package fints

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// StatusEntry is one historical bank processing response, not a current error.
// Dialog references and response text can contain private customer information.
type StatusEntry struct {
	DialogID      string          `json:"dialog_id"`
	MessageNumber int             `json:"message_number"`
	SegmentNumber int             `json:"segment_number,omitempty"`
	Date          string          `json:"date"`
	Time          string          `json:"time"`
	Response      ResponseMessage `json:"response"`
}

// StatusProtocol reads the bank's historical order-processing journal.
// Zero date boundaries leave the range open. This does not submit any orders.
func (c *Client) StatusProtocol(ctx context.Context, from, to time.Time) ([]StatusEntry, error) {
	if !from.IsZero() && !to.IsZero() && from.After(to) {
		return nil, errors.New("status start date must not be after end date")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	d, err := c.openDialog(ctx)
	if err != nil {
		return nil, err
	}
	defer c.endDialog(ctx, d)
	version, err := c.readOperationVersion("HKPRO", 3, 4)
	if err != nil {
		return nil, err
	}
	start, end := "", ""
	if !from.IsZero() {
		start = from.Format("20060102")
	}
	if !to.IsZero() {
		end = to.Format("20060102")
	}
	response, err := c.readPages(ctx, d, "HKPRO", func(cursor string) string {
		return fmt.Sprintf("HKPRO:{n}:%d+%s+%s++%s'", version, start, end, escape(cursor))
	})
	if err != nil {
		return nil, err
	}
	segments := findSegments(response, "HIPRO")
	if len(segments) == 0 && !hasResponseCode(response, "3010") {
		return nil, errors.New("bank returned no status journal")
	}
	result := make([]StatusEntry, 0, len(segments))
	for _, s := range segments {
		if s.Header.Version != 3 && s.Header.Version != 4 {
			return nil, errors.New("unsupported HIPRO response version")
		}
		if len(s.Fields) < 5 || len(s.Fields[4].components) < 3 {
			return nil, errors.New("incomplete status journal response")
		}
		message, err := optionalPositiveInt(component(s, 0, 1))
		if err != nil || message == 0 {
			return nil, errors.New("invalid status message reference")
		}
		segmentNumber, err := optionalPositiveInt(component(s, 1, 0))
		if err != nil {
			return nil, err
		}
		date, err := protocolDate(component(s, 2, 0))
		if err != nil {
			return nil, err
		}
		code := component(s, 4, 0)
		if len(code) != 4 {
			return nil, errors.New("invalid status response code")
		}
		for _, r := range code {
			if r < '0' || r > '9' {
				return nil, errors.New("invalid status response code")
			}
		}
		result = append(result, StatusEntry{DialogID: component(s, 0, 0), MessageNumber: message, SegmentNumber: segmentNumber, Date: date, Time: component(s, 3, 0), Response: ResponseMessage{Code: code, Message: component(s, 4, 2), ReferenceElement: component(s, 4, 1), Segment: segmentNumber, Params: append([]string(nil), s.Fields[4].components[3:]...)}})
	}
	return result, nil
}

// CommunicationEndpoint is a bank-advertised endpoint, preserved as data only.
// The library never automatically follows it or transmits credentials to it.
// ServiceType is the protocol code: 1 T-Online, 2 TCP/IP, 3 HTTPS.
type CommunicationEndpoint struct {
	ServiceType    string `json:"service_type"`
	Address        string `json:"address"`
	AddressAdjunct string `json:"address_adjunct,omitempty"`
	FilterFunction string `json:"filter_function,omitempty"`
	FilterVersion  string `json:"filter_version,omitempty"`
}
type BankCommunication struct {
	CountryCode     string                  `json:"country_code"`
	BankCode        string                  `json:"bank_code"`
	DefaultLanguage string                  `json:"default_language"`
	Endpoints       []CommunicationEndpoint `json:"endpoints"`
}

// CommunicationEndpoints retrieves advertised bank connection information.
// Application code must explicitly validate and select a trusted HTTPS endpoint
// before using it in a new Config; this operation does not change Config.Endpoint.
func (c *Client) CommunicationEndpoints(ctx context.Context) ([]BankCommunication, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	d, err := c.openDialog(ctx)
	if err != nil {
		return nil, err
	}
	defer c.endDialog(ctx, d)
	if _, err := c.readOperationVersion("HKKOM", 4); err != nil {
		return nil, err
	}
	response, err := c.readPages(ctx, d, "HKKOM", func(cursor string) string { return fmt.Sprintf("HKKOM:{n}:4++++%s'", escape(cursor)) })
	if err != nil {
		return nil, err
	}
	segments := findSegments(response, "HIKOM")
	if len(segments) == 0 && !hasResponseCode(response, "3010") {
		return nil, errors.New("bank returned no communication endpoints")
	}
	result := make([]BankCommunication, 0, len(segments))
	for _, s := range segments {
		if s.Header.Version != 4 {
			return nil, errors.New("unsupported HIKOM response version")
		}
		if len(s.Fields) < 3 || component(s, 0, 0) == "" || component(s, 0, 1) == "" {
			return nil, errors.New("incomplete bank communication response")
		}
		bank := BankCommunication{CountryCode: component(s, 0, 0), BankCode: component(s, 0, 1), DefaultLanguage: component(s, 1, 0)}
		for index := 2; index < len(s.Fields); index++ {
			if component(s, index, 0) == "" || component(s, index, 1) == "" {
				return nil, errors.New("incomplete communication endpoint")
			}
			bank.Endpoints = append(bank.Endpoints, CommunicationEndpoint{ServiceType: component(s, index, 0), Address: component(s, index, 1), AddressAdjunct: component(s, index, 2), FilterFunction: component(s, index, 3), FilterVersion: component(s, index, 4)})
		}
		result = append(result, bank)
	}
	return result, nil
}
