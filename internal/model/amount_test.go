package model

import "testing"

func TestParseAmount(t *testing.T) {
	for in, want := range map[string]string{
		"11.240,00": "11240", "11240,00": "11240", "1,234.56": "1234.56", "1234.56": "1234.56",
		"-20.00": "-20", "12,50-": "-12.5", "+1.234,56 €": "1234.56", "1.234.567,89": "1234567.89",
		"1.234": "1234", "1,234": "1.234", "1,234,567": "1234567", "0,5": "0.5", "−9,90": "-9.9", "1 234,50": "1234.5", "400": "400",
	} {
		got, err := ParseAmount(in)
		if err != nil || got.String() != want {
			t.Errorf("ParseAmount(%q) = %s, %v; want %s", in, got, err, want)
		}
	}
	for _, in := range []string{"", "abc", "1,2,3", "1.23.4", "12.34,5.6", "1e5", "€", "1,23.45"} {
		if got, err := ParseAmount(in); err == nil {
			t.Errorf("ParseAmount(%q) = %s, want an error", in, got)
		}
	}
}
