package model

import (
	"strings"
	"unicode"
)

// CompactIdentifier removes display whitespace without dropping meaningful zeros.
func CompactIdentifier(value string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, value)
}

func NormalizeCompany(c Company) Company {
	c.VATID = strings.ToUpper(CompactIdentifier(c.VATID))
	c.TaxNumber = CompactIdentifier(c.TaxNumber)
	c.IBAN = strings.ToUpper(CompactIdentifier(c.IBAN))
	c.BIC = strings.ToUpper(CompactIdentifier(c.BIC))
	c.AdditionalAccounts = append([]BankAccount(nil), c.AdditionalAccounts...)
	for i := range c.AdditionalAccounts {
		c.AdditionalAccounts[i].IBAN = strings.ToUpper(CompactIdentifier(c.AdditionalAccounts[i].IBAN))
		c.AdditionalAccounts[i].BIC = strings.ToUpper(CompactIdentifier(c.AdditionalAccounts[i].BIC))
		c.AdditionalAccounts[i].Name = strings.TrimSpace(c.AdditionalAccounts[i].Name)
	}
	c.Address.Country = strings.ToUpper(strings.TrimSpace(c.Address.Country))
	// The electronic sender address (BT-34) defaults to the contact e-mail unless
	// another address type such as the VAT ID was chosen.
	c.Endpoint = strings.TrimSpace(c.Endpoint)
	if email := strings.TrimSpace(c.Email); c.Endpoint == "" && email != "" && (c.EndpointScheme == "" || c.EndpointScheme == "EM") {
		c.Endpoint, c.EndpointScheme = email, "EM"
	}
	return c
}

func NormalizeCustomer(c Customer) Customer {
	c.VATID = strings.ToUpper(CompactIdentifier(c.VATID))
	c.Address.Country = strings.ToUpper(strings.TrimSpace(c.Address.Country))
	return c
}

// DeliveryLocation defaults to the buyer's location only within that same country.
func DeliveryLocation(d InvoiceDraft) Address {
	a := Address{Country: strings.ToUpper(strings.TrimSpace(d.DeliveryCountry)), City: strings.TrimSpace(d.DeliveryCity), Postcode: strings.TrimSpace(d.DeliveryPostcode)}
	if a.Country == "" {
		a.Country = strings.ToUpper(d.Customer.Address.Country)
	}
	if a.Country == strings.ToUpper(d.Customer.Address.Country) && a.City == "" && a.Postcode == "" {
		a.City, a.Postcode = d.Customer.Address.City, d.Customer.Address.Postcode
	}
	return a
}
