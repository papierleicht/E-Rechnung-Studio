package validation

import (
	"papierleicht.de/e-rechnung-studio/internal/einvoice"
	"papierleicht.de/e-rechnung-studio/internal/model"
	"strings"
)

// Supplemental checks are local application safeguards, separate from the
// unmodified bundled XSD/XSLT. Used for generated XML, imports and the CLI alike.
func SupplementalIssues(inv *einvoice.Invoice) []model.ValidationIssue {
	c := model.Company{VATID: inv.Seller.VATaxRegistration, TaxNumber: inv.Seller.FCTaxRegistration}
	if inv.Seller.PostalAddress != nil {
		c.Address.Country = inv.Seller.PostalAddress.CountryID
	}
	issues := append(CompanyIssues(c), CustomerIssues(model.Customer{VATID: inv.Buyer.VATaxRegistration})...)
	for _, payment := range inv.PaymentMeans {
		if iban := payment.PayeePartyCreditorFinancialAccountIBAN; iban != "" && !validIBAN(iban) {
			issues = append(issues, localIssue("company.iban", "APP-IBAN", "Prüfe die IBAN: Aufbau, Länge oder Prüfziffer stimmen nicht."))
		}
		if bic := payment.PayeeSpecifiedCreditorFinancialInstitutionBIC; bic != "" && !bicShape.MatchString(bic) {
			issues = append(issues, localIssue("company.bic", "APP-BIC", "Prüfe die BIC. Sie muss aus 8 oder 11 Zeichen im BIC-Format bestehen."))
		}
	}
	// BR-CO-25 is absent from the bundled CII rules. Do not present this
	// additional application requirement as an assertion from that ruleset.
	if inv.InvoiceTypeCode != 381 && inv.DuePayableAmount.IsPositive() {
		hasTerms := false
		for _, terms := range inv.SpecifiedTradePaymentTerms {
			hasTerms = hasTerms || !terms.DueDate.IsZero() || strings.TrimSpace(terms.Description) != ""
		}
		if !hasTerms {
			issues = append(issues, localIssue("paymentTerms", "APP-PAYMENT-TERMS", "Ergänze ein Fälligkeitsdatum oder Zahlungsbedingungen für den offenen Rechnungsbetrag."))
		}
	}
	return issues
}
