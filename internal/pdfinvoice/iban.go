package pdfinvoice

import (
	"papierleicht.de/e-rechnung-studio/internal/model"
	"strings"
)

// Group only the visible account number; never modify the invoice XML.
func displayIBAN(value string) string {
	value = strings.ToUpper(model.CompactIdentifier(value))
	groups := []string{}
	for len(value) > 4 {
		groups = append(groups, value[:4])
		value = value[4:]
	}
	if value != "" {
		groups = append(groups, value)
	}
	return strings.Join(groups, " ")
}
