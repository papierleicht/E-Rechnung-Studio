package validation

import (
	"strings"

	"papierleicht.de/e-rechnung-studio/internal/einvoice"
	"papierleicht.de/e-rechnung-studio/internal/model"
)

// identifierGaps says which tax identifiers the parties of the XML lack: the
// VAT ID of the seller (or of its tax representative) and of the buyer, the
// seller's VAT ID and tax number, and the buyer's VAT ID and registration ID.
func identifierGaps(inv *einvoice.Invoice) (sellerMissing, buyerMissing, sellerTaxMissing, buyerIdentifierMissing bool) {
	sellerMissing = strings.TrimSpace(inv.Seller.VATaxRegistration) == ""
	if rep := inv.SellerTaxRepresentativeTradeParty; rep != nil && strings.TrimSpace(rep.VATaxRegistration) != "" {
		sellerMissing = false
	}
	buyerMissing = strings.TrimSpace(inv.Buyer.VATaxRegistration) == ""
	sellerTaxMissing = sellerMissing && strings.TrimSpace(inv.Seller.FCTaxRegistration) == ""
	buyerIdentifierMissing = buyerMissing
	if legal := inv.Buyer.SpecifiedLegalOrganization; legal != nil && strings.TrimSpace(legal.ID) != "" {
		buyerIdentifierMissing = false
	}
	return
}

// Use the actual XML parties, not saved customer records, to explain official
// failures. Preserve the rule, severity and original diagnostic in Details.
func explainIntraCommunityIdentifiers(result model.ValidationResult, inv *einvoice.Invoice) model.ValidationResult {
	sellerMissing, buyerMissing, sellerTaxMissing, buyerIdentifierMissing := identifierGaps(inv)
	for i := range result.Issues {
		issue := &result.Issues[i]
		switch issue.RuleID {
		case "BR-AE-02", "BR-AE-03", "BR-AE-04":
			switch {
			case sellerTaxMissing && buyerIdentifierMissing:
				issue.Message = "Für diese Rechnung mit Steuerschuld beim Kunden fehlen steuerliche Kennungen auf beiden Seiten. Ergänze zuerst deine Steuernummer oder Umsatzsteuer-ID in den Unternehmensdaten und danach die Umsatzsteuer-ID deines Kunden."
				issue.Field = "company.taxNumber"
			case sellerTaxMissing:
				issue.Message = "Die Kennung deines Kunden ist vorhanden. Es fehlt noch deine Steuernummer oder Umsatzsteuer-ID. Ergänze eine dieser Angaben in den Unternehmensdaten."
				issue.Field = "company.taxNumber"
			case buyerIdentifierMissing:
				issue.Message = "Deine steuerliche Kennung ist vorhanden. Für diese Rechnung mit Steuerschuld beim Kunden fehlt noch dessen Umsatzsteuer-ID oder zulässige Registerkennung. Ergänze die Umsatzsteuer-ID in den Empfängerdaten der Rechnung."
				issue.Field = "customer.vatId"
			}
		case "BR-IC-02", "BR-IC-03", "BR-IC-04":
			switch {
			case sellerMissing && buyerMissing:
				issue.Message = "Für diese steuerfreie Lieferung in ein anderes EU-Land fehlen deine eigene Umsatzsteuer-ID und die deines Kunden. Ergänze zuerst deine ID in den Unternehmensdaten. Deine Steuernummer reicht dafür nicht aus."
				issue.Field = "company.vatId"
			case sellerMissing:
				issue.Message = "Die Umsatzsteuer-ID deines Kunden ist eingetragen. Es fehlt noch deine eigene Umsatzsteuer-ID. Ergänze sie in den Unternehmensdaten. Deine Steuernummer reicht für diese steuerfreie EU-Lieferung nicht aus."
				issue.Field = "company.vatId"
			case buyerMissing:
				issue.Message = "Auf deiner Seite ist die erforderliche Kennung vorhanden. Für diese steuerfreie Lieferung in ein anderes EU-Land fehlt noch die Umsatzsteuer-ID deines Kunden auf dieser Rechnung. Ergänze sie in den Empfängerdaten der Rechnung."
				issue.Field = "customer.vatId"
			}
		}
	}
	return result
}
