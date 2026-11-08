package fints

import (
	"strconv"
	"strings"
	"unicode"
)

func normalizeAccount(account Account) Account {
	account.IBAN = normalizeIBAN(account.IBAN)
	account.BIC = strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(account.BIC), " ", ""))
	account.BankCode = strings.TrimSpace(account.BankCode)
	account.AccountNumber = strings.TrimSpace(account.AccountNumber)
	account.Subaccount = strings.TrimSpace(account.Subaccount)

	if len(account.IBAN) == 22 && strings.HasPrefix(account.IBAN, "DE") && validIBAN(account.IBAN) {
		if account.CountryCode == "" {
			account.CountryCode = "280"
		}
		if account.BankCode == "" {
			account.BankCode = account.IBAN[4:12]
		}
		if account.AccountNumber == "" {
			// The German BBAN always carries the ten-digit national account
			// number. Preserve leading zeroes; several banks require them.
			account.AccountNumber = account.IBAN[12:22]
		}
	}
	if account.CountryCode == "" && len(account.IBAN) >= 2 {
		account.CountryCode = ibanCountryToFinTS(account.IBAN[:2])
	}
	return account
}

func normalizeIBAN(value string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return unicode.ToUpper(r)
	}, strings.TrimSpace(value))
}

func validIBAN(value string) bool {
	value = normalizeIBAN(value)
	if len(value) < 15 || len(value) > 34 {
		return false
	}
	rearranged := value[4:] + value[:4]
	remainder := 0
	for _, r := range rearranged {
		switch {
		case r >= '0' && r <= '9':
			remainder = (remainder*10 + int(r-'0')) % 97
		case r >= 'A' && r <= 'Z':
			number := int(r-'A') + 10
			remainder = (remainder*100 + number) % 97
		default:
			return false
		}
	}
	return remainder == 1
}

func ibanCountryToFinTS(country string) string {
	// Numeric ISO-3166 identifiers used by FinTS for the countries in which
	// FinTS/SEPA account contacts are commonly encountered.
	codes := map[string]string{
		"AT": "040", "BE": "056", "CH": "756", "CZ": "203", "DE": "280",
		"DK": "208", "ES": "724", "FI": "246", "FR": "250", "GB": "826",
		"GR": "300", "HR": "191", "HU": "348", "IE": "372", "IS": "352",
		"IT": "380", "LI": "438", "LU": "442", "NL": "528", "NO": "578",
		"PL": "616", "PT": "620", "RO": "642", "SE": "752", "SI": "705",
		"SK": "703",
	}
	return codes[strings.ToUpper(country)]
}

func formatMinorUnits(value int64) string {
	// Unsigned magnitude also handles math.MinInt64 without negation overflow.
	magnitude := uint64(value)
	if value < 0 {
		magnitude = uint64(-(value + 1)) + 1
	}
	text := strconv.FormatUint(magnitude/100, 10) + "." + strconv.FormatUint(magnitude%100+100, 10)[1:]
	if value < 0 {
		text = "-" + text
	}
	return text
}
