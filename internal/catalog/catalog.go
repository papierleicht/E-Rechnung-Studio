// Package catalog holds the invoice metadata shared with the frontend.
package catalog

import (
	"embed"
	"encoding/json"
)

//go:embed invoice.json
var files embed.FS

type TaxTreatment struct {
	ID, Label, Category, Reason, Code string
}
type Format struct {
	ID, Syntax, Label, Description, Specification, Conformance string
	XRechnung                                                  bool
}

var data struct {
	TaxTreatments []TaxTreatment
	Formats       []Format
}

func init() {
	raw, err := files.ReadFile("invoice.json")
	if err != nil {
		panic(err)
	}
	if err := json.Unmarshal(raw, &data); err != nil {
		panic(err)
	}
}
func Tax(id string) (TaxTreatment, bool) {
	for _, treatment := range data.TaxTreatments {
		if treatment.ID == id {
			return treatment, true
		}
	}
	return TaxTreatment{}, false
}
func Formats() []Format { return append([]Format(nil), data.Formats...) }
