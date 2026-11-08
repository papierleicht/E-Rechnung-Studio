package fints

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Field :61: places the debit/credit mark before the optional funds code:
// value date, entry date, C/D/RC/RD, funds code, amount, transaction type.
var transactionLinePattern = regexp.MustCompile(`^(\d{6})(\d{4})?(R?[CD])([A-Z])?([0-9,]+)([A-Z][A-Z0-9]{3})(.*)$`)

type mt940Tag struct {
	name  string
	value string
}

func parseMT940(data []byte) ([]Transaction, error) {
	return parseMT940ForAccount(data, Account{})
}

func parseMT940ForAccount(data []byte, account Account) ([]Transaction, error) {
	if len(data) == 0 {
		return []Transaction{}, nil
	}
	// FinTS MT940 binary payloads use the protocol's ISO-8859-1 text encoding.
	tags := scanMT940Tags(decodeLatin1(data))
	currency := ""
	openingSeen, closingSeen := false, false
	entriesSinceOpening := 0
	var transactions []Transaction
	var current *Transaction

	for _, tag := range tags {
		switch tag.name {
		case "25":
			if err := checkMT940AccountIdentifier(tag.value, account); err != nil {
				return nil, err
			}
		case "60F", "60M":
			if openingSeen && entriesSinceOpening == 0 && !closingSeen {
				return nil, fmt.Errorf("empty MT940 statement is missing its closing balance")
			}
			balanceCurrency, err := validateMT940Balance(tag.value)
			if err != nil {
				return nil, err
			}
			currency = balanceCurrency
			openingSeen = true
			closingSeen = false
			entriesSinceOpening = 0
		case "62F", "62M":
			balanceCurrency, err := validateMT940Balance(tag.value)
			if err != nil {
				return nil, err
			}
			if !openingSeen || balanceCurrency != currency {
				return nil, fmt.Errorf("MT940 closing balance has no matching opening balance")
			}
			closingSeen = true
		case "61":
			transaction, err := parseTransactionLine(tag.value, currency)
			if err != nil {
				return nil, err
			}
			transactions = append(transactions, transaction)
			entriesSinceOpening++
			current = &transactions[len(transactions)-1]
		case "86":
			if current != nil {
				applyDescription(current, tag.value)
			}
		}
	}
	if !openingSeen {
		return nil, fmt.Errorf("nonempty MT940 response has no valid opening balance")
	}
	if entriesSinceOpening == 0 && !closingSeen {
		return nil, fmt.Errorf("empty MT940 statement is missing its closing balance")
	}
	return transactions, nil
}

// A nonempty payload must contain recognizable balance and transaction data.
// Older transaction fragments can omit the closing balance when actual entries
// exist, but cannot masquerade as a successfully parsed empty statement.
func validateMT940Balance(value string) (string, error) {
	value = strings.TrimSpace(value)
	if len(value) < 11 || (value[0] != 'C' && value[0] != 'D') {
		return "", fmt.Errorf("invalid MT940 balance format")
	}
	if _, err := time.Parse("060102", value[1:7]); err != nil {
		return "", fmt.Errorf("invalid MT940 balance date")
	}
	currency := value[7:10]
	if _, err := parseCurrencyAmount(value[10:], currency, ','); err != nil {
		return "", fmt.Errorf("invalid MT940 balance amount or currency")
	}
	return currency, nil
}

func scanMT940Tags(input string) []mt940Tag {
	input = strings.ReplaceAll(input, "\r\n", "\n")
	input = strings.ReplaceAll(input, "\r", "\n")
	var tags []mt940Tag
	for _, line := range strings.Split(input, "\n") {
		if strings.HasPrefix(line, ":") {
			if index := strings.Index(line[1:], ":"); index >= 0 {
				index++
				tags = append(tags, mt940Tag{name: line[1:index], value: line[index+1:]})
				continue
			}
		}
		if len(tags) > 0 && line != "-" {
			tags[len(tags)-1].value += "\n" + line
		}
	}
	return tags
}

func parseTransactionLine(line, currency string) (Transaction, error) {
	line = strings.ReplaceAll(line, "\n", "")
	match := transactionLinePattern.FindStringSubmatch(line)
	if match == nil {
		return Transaction{}, fmt.Errorf("unsupported MT940 :61: transaction format")
	}

	amount, err := parseCurrencyAmount(match[5], currency, ',')
	if err != nil {
		return Transaction{}, fmt.Errorf("parse MT940 amount: %w", err)
	}
	// D is a debit and RC reverses a credit, so both reduce the balance.
	// C and RD (reversal of a debit) increase it.
	if match[3] == "D" || match[3] == "RC" {
		amount = -amount
	}
	valueDate, err := time.Parse("060102", match[1])
	if err != nil {
		return Transaction{}, fmt.Errorf("parse MT940 value date: %w", err)
	}
	bookingDate := valueDate
	if match[2] != "" {
		month, _ := strconv.Atoi(match[2][:2])
		year := valueDate.Year()
		if valueDate.Month() == time.December && month == 1 {
			year++
		} else if valueDate.Month() == time.January && month == 12 {
			year--
		}
		bookingDate, err = time.Parse("20060102", fmt.Sprintf("%04d%s", year, match[2]))
		if err != nil {
			return Transaction{}, fmt.Errorf("parse MT940 booking date: %w", err)
		}
	}

	customerReference := match[7]
	bankReference := ""
	if before, after, ok := strings.Cut(customerReference, "//"); ok {
		customerReference = before
		bankReference = after
	}

	return Transaction{
		BookingDate:       bookingDate.Format(time.DateOnly),
		ValueDate:         valueDate.Format(time.DateOnly),
		Amount:            amount,
		Currency:          currency,
		Type:              match[6],
		CustomerReference: strings.TrimSpace(customerReference),
		BankReference:     strings.TrimSpace(bankReference),
	}, nil
}

// parseMinorUnits parses an unsigned FinTS amount exactly. The sign belongs
// to the surrounding credit/debit indicator, never to this field.
func parseMinorUnits(value string) (int64, error) {
	if strings.Contains(value, ".") {
		whole, _, _ := strings.Cut(value, ",")
		groups := strings.Split(whole, ".")
		if len(groups[0]) < 1 || len(groups[0]) > 3 {
			return 0, fmt.Errorf("invalid amount grouping")
		}
		for _, group := range groups[1:] {
			if len(group) != 3 {
				return 0, fmt.Errorf("invalid amount grouping")
			}
		}
		value = strings.ReplaceAll(value, ".", "")
	}
	return parseDecimalAmount(value, ',', 2)
}

// currencyDecimals covers ISO 4217 exceptions to the usual two minor digits.
// Precious-metal and non-currency units are rejected rather than rounded.
func currencyDecimals(currency string) (int, error) {
	switch currency {
	case "BIF", "CLP", "DJF", "GNF", "ISK", "JPY", "KMF", "KRW", "PYG", "RWF", "UGX", "UYI", "VND", "VUV", "XAF", "XOF", "XPF":
		return 0, nil
	case "BHD", "IQD", "JOD", "KWD", "LYD", "OMR", "TND":
		return 3, nil
	case "CLF", "UYW":
		return 4, nil
	case "XAU", "XAG", "XPD", "XPT", "XXX", "XTS", "":
		return 0, fmt.Errorf("unsupported monetary currency")
	}
	if len(currency) != 3 {
		return 0, fmt.Errorf("invalid currency code")
	}
	for _, r := range currency {
		if r < 'A' || r > 'Z' {
			return 0, fmt.Errorf("invalid currency code")
		}
	}
	return 2, nil
}

func parseCurrencyAmount(value, currency string, separator byte) (int64, error) {
	digits, err := currencyDecimals(currency)
	if err != nil {
		return 0, err
	}
	return parseDecimalAmount(value, separator, digits)
}

func parseDecimalAmount(value string, separator byte, digits int) (int64, error) {
	value = strings.TrimSpace(value)
	parts := strings.Split(value, string(separator))
	if len(parts) > 2 || parts[0] == "" {
		return 0, fmt.Errorf("invalid monetary amount")
	}
	fraction := ""
	if len(parts) == 2 {
		fraction = parts[1]
	}
	for _, part := range parts {
		for _, r := range part {
			if r < '0' || r > '9' {
				return 0, fmt.Errorf("invalid monetary amount")
			}
		}
	}
	if len(fraction) > digits {
		for _, r := range fraction[digits:] {
			if r != '0' {
				return 0, fmt.Errorf("monetary amount exceeds currency precision")
			}
		}
		fraction = fraction[:digits]
	}
	fraction += strings.Repeat("0", digits-len(fraction))
	amount, err := strconv.ParseInt(parts[0]+fraction, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("monetary amount is out of range")
	}
	return amount, nil
}

func applyDescription(transaction *Transaction, value string) {
	value = strings.ReplaceAll(value, "\n", "")
	firstQuestion := strings.IndexByte(value, '?')
	if firstQuestion < 0 {
		transaction.Description = strings.TrimSpace(value)
		return
	}

	structured := value[firstQuestion:]
	parts := strings.Split(structured, "?")
	var purpose, names []string
	for _, part := range parts {
		if len(part) < 2 {
			continue
		}
		code, text := part[:2], strings.TrimSpace(part[2:])
		switch code {
		case "00":
			if text != "" && transaction.Type == "" {
				transaction.Type = text
			}
		case "20", "21", "22", "23", "24", "25", "26", "27", "28", "29", "60", "61", "62", "63":
			if text != "" {
				purpose = append(purpose, text)
			}
		case "30":
			transaction.CounterpartyBankID = text
		case "31":
			transaction.CounterpartyIBAN = text
		case "32", "33":
			if text != "" {
				names = append(names, text)
			}
		}
	}
	transaction.Description = strings.Join(purpose, " ")
	transaction.CounterpartyName = strings.Join(names, " ")
	if transaction.Description == "" && firstQuestion > 0 {
		transaction.Description = strings.TrimSpace(value[:firstQuestion])
	}
}
