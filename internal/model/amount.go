package model

import (
	"errors"
	"regexp"
	"strings"

	"github.com/shopspring/decimal"
)

var (
	amountDigits   = regexp.MustCompile(`^\d[\d.,]*$`)
	groupedByDot   = regexp.MustCompile(`^\d{1,3}(\.\d{3})+$`)
	groupedByComma = regexp.MustCompile(`^\d{1,3}(,\d{3})+$`)
)

// ParseAmount reads an amount the way people and banks write it: "1.234,56",
// "1234,56", "1,234.56" or "1234.56", with an optional sign, also a trailing
// one ("12,50-"), and an optional euro sign. "1.234" is 1234: the dot groups
// thousands in German. "1,234" keeps its comma as the decimal separator.
func ParseAmount(s string) (decimal.Decimal, error) {
	invalid := errors.New("invalid amount")
	s = strings.NewReplacer(" ", "", " ", "", " ", "", "'", "", "€", "", "EUR", "", "−", "-").Replace(strings.TrimSpace(s))
	negative := false
	switch {
	case strings.HasPrefix(s, "-"), strings.HasPrefix(s, "+"):
		negative = s[0] == '-'
		s = s[1:]
	case strings.HasSuffix(s, "-"), strings.HasSuffix(s, "+"):
		negative = strings.HasSuffix(s, "-")
		s = s[:len(s)-1]
	}
	if len(s) > 64 || !amountDigits.MatchString(s) {
		return decimal.Zero, invalid
	}
	dot, comma := strings.LastIndex(s, "."), strings.LastIndex(s, ",")
	switch {
	case dot >= 0 && comma >= 0 && comma > dot:
		if !groupedByDot.MatchString(s[:comma]) {
			return decimal.Zero, invalid
		}
		s = strings.ReplaceAll(s[:comma], ".", "") + "." + s[comma+1:]
	case dot >= 0 && comma >= 0:
		if !groupedByComma.MatchString(s[:dot]) {
			return decimal.Zero, invalid
		}
		s = strings.ReplaceAll(s[:dot], ",", "") + "." + s[dot+1:]
	case groupedByDot.MatchString(s):
		s = strings.ReplaceAll(s, ".", "")
	case strings.Count(s, ",") > 1 && groupedByComma.MatchString(s):
		s = strings.ReplaceAll(s, ",", "")
	case strings.Count(s, ",")+strings.Count(s, ".") > 1:
		return decimal.Zero, invalid
	default:
		s = strings.ReplaceAll(s, ",", ".")
	}
	amount, err := decimal.NewFromString(s)
	if err != nil {
		return decimal.Zero, invalid
	}
	if negative {
		amount = amount.Neg()
	}
	return amount, nil
}
