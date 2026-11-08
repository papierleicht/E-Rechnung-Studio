package fints

import (
	"context"
	"sort"
	"strconv"
	"strings"
)

// Capabilities returns the operations and versions advertised in the current
// bank parameter data (BPD). No bank-specific operation table is hard-coded.
func (c *Client) Capabilities(ctx context.Context) ([]Capability, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := c.ensureAuthentication(ctx); err != nil {
		return nil, err
	}
	result := make([]Capability, 0, len(c.capabilities))
	for _, capability := range c.capabilities {
		copy := *capability
		copy.Versions = append([]int(nil), capability.Versions...)
		copy.Formats = append([]string(nil), capability.Formats...)
		copy.TANRequired = c.tanRequirements[copy.Operation]
		result = append(result, copy)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Operation < result[j].Operation
	})
	return result, nil
}

func (c *Client) updateCapabilities(response []segment) {
	for _, s := range response {
		kind := s.Header.Type
		if len(kind) < 4 || !(strings.HasPrefix(kind, "HI") || kind == "DIKKUS") || !strings.HasSuffix(kind, "S") {
			continue
		}
		// HIRMS is an order response, not a business-parameter segment.
		if kind == "HIPINS" || kind == "HIRMS" {
			continue
		}
		operation := "HK" + kind[2:len(kind)-1]
		if kind == "DIKKUS" {
			operation = "DKKKU"
		}
		capability := c.capabilities[operation]
		if capability == nil {
			capability = &Capability{Operation: operation}
			c.capabilities[operation] = capability
		}
		capability.Versions = appendUniqueInt(capability.Versions, s.Header.Version)
		for _, value := range flattenedComponents(s) {
			if strings.HasPrefix(value, "urn:") {
				capability.Formats = appendUniqueString(capability.Formats, value)
			}
		}
		if (operation == "HKKAZ" || operation == "HKCAZ") && capability.MaxDays == 0 {
			values := flattenedComponents(s)
			for index := 2; index < len(values); index++ {
				days, err := strconv.Atoi(values[index])
				if err != nil || days <= 0 {
					continue
				}
				// In newer parameter versions the security class precedes
				// the operation-specific maximum-day field.
				if index == 2 && days <= 4 && index+1 < len(values) {
					continue
				}
				capability.MaxDays = days
				break
			}
		}
	}

	for operation, capability := range c.capabilities {
		sort.Ints(capability.Versions)
		sort.Strings(capability.Formats)
		capability.TANRequired = c.tanRequirements[operation]
	}
	if capability := c.capabilities["HKCAZ"]; capability != nil {
		c.hkcazVers = append([]int(nil), capability.Versions...)
	}
}

func appendUniqueInt(values []int, value int) []int {
	for _, candidate := range values {
		if candidate == value {
			return values
		}
	}
	return append(values, value)
}

func appendUniqueString(values []string, value string) []string {
	for _, candidate := range values {
		if candidate == value {
			return values
		}
	}
	return append(values, value)
}

func supportsOperation(account Account, operation string) bool {
	if len(account.SupportedOperations) == 0 {
		return true
	}
	return account.SupportedOperations[operation]
}
