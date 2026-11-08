package model

import "testing"

func TestStandardZeroRateUsesZeroRatedCategory(t *testing.T) {
	for _, mode := range []string{"standard", "mixed"} {
		for _, rate := range []string{"0", "0.00", " 0,0 "} {
			line := InvoiceLine{TaxTreatment: "standard", TaxCategory: "S", TaxRate: rate, ExemptionReason: "obsolete"}
			tax, err := LineTax(InvoiceDraft{TaxTreatment: mode, TaxExemptionReason: "obsolete"}, line)
			if err != nil || tax.Category != "Z" || tax.Reason != "" || tax.Code != "" {
				t.Fatalf("%s/%s: %+v, %v", mode, rate, tax, err)
			}
		}
	}
	for _, rate := range []string{"19", "7", "", "invalid", "-1"} {
		tax, _ := LineTax(InvoiceDraft{TaxTreatment: "standard"}, InvoiceLine{TaxRate: rate})
		if tax.Category != "S" || tax.Rate != rate {
			t.Fatalf("rate changed unexpectedly: %+v", tax)
		}
	}
	for _, kind := range []string{"small-business", "exempt", "margin-goods"} {
		tax, _ := LineTax(InvoiceDraft{TaxTreatment: kind}, InvoiceLine{TaxRate: "0"})
		if tax.Category != "E" {
			t.Fatalf("exemption reclassified: %+v", tax)
		}
	}
}
