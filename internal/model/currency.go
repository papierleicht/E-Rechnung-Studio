package model

import "golang.org/x/text/currency"

// CurrencyScale returns the ISO currency's number of fractional digits.
func CurrencyScale(code string) (int32, error) {
	unit, err := currency.ParseISO(code)
	if err != nil {
		return 0, err
	}
	scale, _ := currency.Standard.Rounding(unit)
	return int32(scale), nil
}
