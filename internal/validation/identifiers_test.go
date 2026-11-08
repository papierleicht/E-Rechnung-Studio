package validation

import (
	"papierleicht.de/e-rechnung-studio/internal/model"
	"testing"
)

func TestIBANFormatAndChecksum(t *testing.T) {
	for _, tc := range []struct {
		value string
		valid bool
	}{
		{"DE89370400440532013000", true}, {"de89 3704\t0044\u00a00532 0130 00", true},
		{"GB82WEST12345698765432", true}, {"FR1420041010050500013M02606", true},
		{"NL91ABNA0417164300", true}, {"NO9386011117947", true}, {"XK051212012345678906", true},
		{"DE88370400440532013000", false}, {"DE8937040044053201300", false},
		{"DE00", false}, {"not-an-iban", false}, {"ZZ89370400440532013000", false},
		{"DE89-370400440532013000", false}, {"", false},
	} {
		t.Run(tc.value, func(t *testing.T) {
			if got := validIBAN(tc.value); got != tc.valid {
				t.Fatalf("validIBAN=%v want %v", got, tc.valid)
			}
		})
	}
}

func TestTaxIdentifierFormats(t *testing.T) {
	for _, value := range []string{"DE123456789", "de 123 456 789", "FR40303265045", "ATU12345678", "EL123456789"} {
		if !validVAT(value) {
			t.Errorf("valid VAT rejected: %s", value)
		}
	}
	for _, value := range []string{"DEabc", "DE12345678", "DE1234567890", "123456789", "DE12345678A"} {
		if validVAT(value) {
			t.Errorf("invalid VAT accepted: %s", value)
		}
	}
	for _, value := range []string{"12/345/67890", "123/345/67890", "123/4567/8901", "01234567890", "1234567890", "1234567890123"} {
		if !germanTax.MatchString(value) {
			t.Errorf("valid tax format rejected: %s", value)
		}
	}
	for _, value := range []string{"123", "DE123456789", "123456789012", "12//345/67890", "00000abc00000"} {
		if germanTax.MatchString(value) {
			t.Errorf("invalid tax format accepted: %s", value)
		}
	}
	// National tax numbers outside DE must not be forced into a German pattern.
	if issues := CompanyIssues(model.Company{TaxNumber: "123-ABC", Address: model.Address{Country: "FR"}}); len(issues) > 0 {
		t.Fatalf("foreign tax format rejected: %+v", issues)
	}
}
