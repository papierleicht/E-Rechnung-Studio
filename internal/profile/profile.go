// Package profile is the shared registry for generation, validation and PDF metadata.
package profile

import (
	"fmt"
	"papierleicht.de/e-rechnung-studio/internal/catalog"

	"papierleicht.de/e-rechnung-studio/internal/einvoice"
)

const DefaultBusinessProcess = "urn:fdc:peppol.eu:2017:poacc:billing:01:1.0"

type Format = catalog.Format

var formats = catalog.Formats()

func Get(id string) (Format, error) {
	for _, f := range formats {
		if f.ID == id {
			return f, nil
		}
	}
	return Format{}, fmt.Errorf("Das E-Rechnungsformat %q wird nicht unterstützt. Wähle ein Format im erweiterten Menü.", id)
}

func Detect(inv *einvoice.Invoice) (Format, error) {
	for _, f := range formats {
		if inv.GuidelineSpecifiedDocumentContextParameter == f.Specification &&
			((f.Syntax == "cii" && inv.SchemaType == einvoice.CII) || (f.Syntax == "ubl" && inv.SchemaType == einvoice.UBL)) {
			return f, nil
		}
	}
	return Format{}, fmt.Errorf("Für diese Kombination aus Profil und XML-Syntax ist kein Prüfregelwerk eingebunden.")
}
