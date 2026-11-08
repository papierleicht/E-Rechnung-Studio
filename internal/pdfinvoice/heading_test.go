package pdfinvoice

import "testing"

func TestSplitSellerName(t *testing.T) {
	long := "Muster GmbH Internationale Handelsgesellschaft mit beschränkter Haftung & Co. KG"
	for name, want := range map[string]sellerHeading{
		"Muster GmbH":   {Title: "Muster GmbH"},
		" Muster GmbH ": {Title: "Muster GmbH"},
		// Two lines in the large heading are fine; only short names never change.
		"Bäckerei Sonnenkorn Handelsgesellschaft mbH":                                  {Title: "Bäckerei Sonnenkorn Handelsgesellschaft mbH"},
		"Atelier Lindner – Zweigniederlassung Nordrhein-Westfalen und Rheinland-Pfalz": {Title: "Atelier Lindner", Subtitle: "Zweigniederlassung Nordrhein-Westfalen und Rheinland-Pfalz"},
		"Atelier Lindner - Zweigniederlassung Nordrhein-Westfalen und Rheinland-Pfalz": {Title: "Atelier Lindner", Subtitle: "Zweigniederlassung Nordrhein-Westfalen und Rheinland-Pfalz"},
		// A dash inside a word is no break, and a head that is itself long is not split.
		"Nordrhein-Westfälische Handelsgesellschaft für Bürobedarf": {Title: "Nordrhein-Westfälische Handelsgesellschaft für Bürobedarf"},
		long + " – Zweigniederlassung Köln":                         {Title: long + " – Zweigniederlassung Köln", Compact: true},
		long:                                                        {Title: long, Compact: true},
	} {
		if got := splitSellerName(name); got != want {
			t.Errorf("%q: %+v, want %+v", name, got, want)
		}
	}
}
