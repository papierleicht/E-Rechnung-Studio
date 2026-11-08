package engine

import (
	"strings"
	"testing"
)

func TestInputErrorsAreGerman(t *testing.T) {
	for _, input := range []string{"", "abc", "-1", "0"} {
		_, err := parseAmount("Menge", input, true)
		if err == nil || (!strings.HasPrefix(err.Error(), "Gib ") && !strings.HasPrefix(err.Error(), "Fülle ")) || !strings.Contains(err.Error(), "Menge") {
			t.Fatalf("missing German guidance: %v", err)
		}
	}
	_, err := parseDate("Rechnungsdatum", "invalid", true)
	if err == nil || err.Error() != "Gib für „Rechnungsdatum“ ein gültiges Datum ein." {
		t.Fatalf("missing German date guidance: %v", err)
	}
}
