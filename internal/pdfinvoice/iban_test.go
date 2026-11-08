package pdfinvoice

import "testing"

func TestDisplayIBAN(t *testing.T) {
	for _, input := range []string{"DE89370400440532013000", "de89 3704\u00a00044 0532 0130 00"} {
		if got := displayIBAN(input); got != "DE89 3704 0044 0532 0130 00" {
			t.Fatalf("unexpected display: %q", got)
		}
	}
	if displayIBAN("") != "" {
		t.Fatal("empty account changed")
	}
}
