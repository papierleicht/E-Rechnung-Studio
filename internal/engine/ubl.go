package engine

import "github.com/beevik/etree"

// The library emits these optional settlement groups out of UBL schema order.
// Reorder only the affected groups; their values and all other XML stay intact.
func normalizeUBLSettlement(root *etree.Element) {
	orderElements(root, "cac:PaymentMeans", "cac:PaymentTerms", "cac:AllowanceCharge")
	for _, totals := range root.FindElements("cac:LegalMonetaryTotal") {
		orderElements(totals, "cbc:LineExtensionAmount", "cbc:TaxExclusiveAmount", "cbc:TaxInclusiveAmount", "cbc:AllowanceTotalAmount", "cbc:ChargeTotalAmount", "cbc:PrepaidAmount", "cbc:PayableRoundingAmount", "cbc:PayableAmount")
	}
	for _, tax := range root.FindElements(".//cac:TaxCategory") {
		orderElements(tax, "cbc:TaxExemptionReasonCode", "cbc:TaxExemptionReason")
	}
}

func orderElements(parent *etree.Element, names ...string) {
	var elements []*etree.Element
	first := len(parent.Child)
	for _, name := range names {
		for _, element := range parent.FindElements(name) {
			first = min(first, element.Index())
			elements = append(elements, element)
		}
	}
	for _, element := range elements {
		parent.RemoveChild(element)
	}
	for i, element := range elements {
		parent.InsertChildAt(first+i, element)
	}
}
