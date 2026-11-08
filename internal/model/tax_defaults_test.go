package model

import "testing"

func TestInvoiceTaxPresetCompatibility(t *testing.T) {
	empty, special := "", "reverse-charge"
	tests := []struct {
		name string
		c    Company
		want string
	}{
		{"legacy normal", Company{}, "standard"},
		{"legacy small", Company{SmallBusiness: true, DefaultTaxTreatment: "reverse-charge"}, "small-business"},
		{"legacy special", Company{DefaultTaxTreatment: "exempt"}, "exempt"},
		{"explicit reset", Company{DefaultTaxTreatment: "exempt", InvoiceTaxPreset: &empty}, "standard"},
		{"small reset", Company{SmallBusiness: true, DefaultTaxTreatment: "exempt", InvoiceTaxPreset: &empty}, "small-business"},
		{"independent preset", Company{SmallBusiness: true, InvoiceTaxPreset: &special}, "reverse-charge"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.c.InvoiceTaxTreatment(); got != tt.want {
				t.Fatalf("got %s want %s", got, tt.want)
			}
		})
	}
}
