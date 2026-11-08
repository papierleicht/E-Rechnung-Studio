package model

import (
	"testing"
	"time"
)

func TestSeriesFromNumber(t *testing.T) {
	date := time.Date(2026, 10, 29, 0, 0, 0, 0, time.UTC)
	for number, want := range map[string]string{
		"RE-2026-0042": "RE-{JJJJ}-{NNNN}",
		"2026/118":     "{JJJJ}/{NNN}",
		"MUE-0007":     "MUE-{NNNN}",
		"RE-0007-2026": "RE-{NNNN}-{JJJJ}",
		"RE-26-007":    "RE-{JJ}-{NNN}",
		"K26-007":      "K26-{NNN}",
		"2026-2026":    "{JJJJ}-{NNNN}",
		"26117":        "{NNNNN}",
		"A7B":          "A{N}B",
		"RE-2025-0003": "RE-2025-{NNNN}",
		"RE/2026:42":   "RE/{JJJJ}:{NN}",
	} {
		got, ok := SeriesFromNumber(number, date)
		if !ok || got != want {
			t.Errorf("SeriesFromNumber(%q) = %q, %v; want %q", number, got, ok, want)
		}
	}
	for _, number := range []string{"", "RE", "RE 1", "Rechnung#1", "1234567890123"} {
		if got, ok := SeriesFromNumber(number, date); ok {
			t.Errorf("SeriesFromNumber(%q) = %q, want none", number, got)
		}
	}
}

func TestValidateSeries(t *testing.T) {
	for _, valid := range []string{DefaultNumberSeries, "{JJJJ}/{NNN}", "K12-{N}", "{NNNN}", "RE-{JJ}{MM}-{NNN}", "re_{nnn}.a"} {
		if err := ValidateSeries(valid); err != nil {
			t.Errorf("ValidateSeries(%q): %v", valid, err)
		}
	}
	for _, invalid := range []string{"", "RE-2026", "{NNN}-{NN}", "{JJJJ}{NNN}", "RE2{NNN}", "{NNN}3", "RE {NNN}", "RE#{NNN}", "RE-{XX}-{NNN}", "RE-{NNN", "{NNNNNNNNNN}", "ABCDEFGHIJKLMNOPQRSTUVWXYZ-ABCDEFGHIJ-{NNN}"} {
		if err := ValidateSeries(invalid); err == nil {
			t.Errorf("ValidateSeries(%q) accepted", invalid)
		}
	}
}

func TestResolveSeries(t *testing.T) {
	p, err := ResolveSeries("RE-{JJ}{MM}-{NNN}/X", time.Date(2027, 3, 5, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if got := p.Format(7); got != "RE-2703-007/X" {
		t.Fatalf("Format = %q", got)
	}
	if got := p.Format(1234); got != "RE-2703-1234/X" {
		t.Fatalf("wide Format = %q", got)
	}
	if n, ok := p.Counter("re-2703-0042/x"); !ok || n != 42 {
		t.Fatalf("Counter = %d, %v", n, ok)
	}
	for _, other := range []string{"RE-2703-/X", "RE-2703-4a/X", "RE-2704-001/X", "RE-2703-001"} {
		if _, ok := p.Counter(other); ok {
			t.Errorf("Counter(%q) matched", other)
		}
	}
}

func TestValidateInvoiceNumber(t *testing.T) {
	// Letter sequences alone are allowed (Abschn. 14.5 Abs. 10 UStAE).
	for _, valid := range []string{"RE-2026-0001", "2026/118", "a.b_c-1", "RE/2026:42", "BASIC-DETAILS"} {
		if err := ValidateInvoiceNumber(valid); err != nil {
			t.Errorf("%q: %v", valid, err)
		}
	}
	for _, invalid := range []string{"", "RE 1", "RÉ-1", "RE#1", "RE,1", "1234567890123456789012345678901234567"} {
		if err := ValidateInvoiceNumber(invalid); err == nil {
			t.Errorf("%q accepted", invalid)
		}
	}
}
