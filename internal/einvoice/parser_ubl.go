// Adapted from github.com/speedata/einvoice under BSD-3-Clause; see LICENSE.

package einvoice

import (
	"encoding/base64"
	"fmt"
	"time"
)

// UBL 2.1 namespace URNs for Invoice and CreditNote documents
const (
	nsUBLInvoice    = "urn:oasis:names:specification:ubl:schema:xsd:Invoice-2"
	nsUBLCreditNote = "urn:oasis:names:specification:ubl:schema:xsd:CreditNote-2"
	nsUBLCAC        = "urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2"
	nsUBLCBC        = "urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2"
)

// parseTimeUBL parses ISO 8601 date format (YYYY-MM-DD) used in UBL documents.
func parseTimeUBL(ctx *xmlNode, path string) (time.Time, error) {
	timestring := ctx.value(path)
	if timestring == "" {
		return time.Time{}, nil
	}

	parsedDate, err := time.Parse("2006-01-02", timestring)
	if err != nil {
		return parsedDate, fmt.Errorf("invalid date %q at %s: %w", timestring, path, err)
	}

	return parsedDate, nil
}

// parseUBL parses a UBL 2.1 Invoice or CreditNote document into an Invoice struct.
// Both document types are mapped to the same Invoice struct, differentiated by InvoiceTypeCode.
func parseUBL(ctx *xmlNode) (*Invoice, error) {
	inv := &Invoice{SchemaType: UBL}

	root := ctx

	// Determine document type (Invoice vs CreditNote)
	localName := root.name.Local

	// Set namespace prefix based on document type
	prefix := "inv:"
	if localName == "CreditNote" {
		prefix = "cn:"
	}

	// Parse all components
	if err := parseUBLHeader(root, inv, prefix); err != nil {
		return nil, fmt.Errorf("parse UBL header: %w", err)
	}

	if err := parseUBLParties(root, inv, prefix); err != nil {
		return nil, fmt.Errorf("parse UBL parties: %w", err)
	}

	if err := parseUBLAllowanceCharge(root, inv, prefix); err != nil {
		return nil, fmt.Errorf("parse UBL allowances/charges: %w", err)
	}

	if err := parseUBLTaxTotal(root, inv, prefix); err != nil {
		return nil, fmt.Errorf("parse UBL tax total: %w", err)
	}

	if err := parseUBLMonetarySummation(root, inv, prefix); err != nil {
		return nil, fmt.Errorf("parse UBL monetary summation: %w", err)
	}

	if err := parseUBLPaymentMeans(root, inv, prefix); err != nil {
		return nil, fmt.Errorf("parse UBL payment means: %w", err)
	}

	if err := parseUBLPaymentTerms(root, inv, prefix); err != nil {
		return nil, fmt.Errorf("parse UBL payment terms: %w", err)
	}

	if err := parseUBLLines(root, inv, prefix); err != nil {
		return nil, fmt.Errorf("parse UBL lines: %w", err)
	}

	return inv, nil
}

// parseUBLHeader parses the document header elements (BT-1 to BT-24, BG-1, BG-3, BG-14, BG-24).
func parseUBLHeader(root *xmlNode, inv *Invoice, prefix string) error {
	// BT-24: CustomizationID (Specification identifier)
	inv.GuidelineSpecifiedDocumentContextParameter = root.value("cbc:CustomizationID")

	// BT-23: ProfileID (Business process type)
	inv.BPSpecifiedDocumentContextParameter = root.value("cbc:ProfileID")

	// BT-1: Invoice number
	inv.InvoiceNumber = root.value("cbc:ID")

	// BT-3: Invoice type code
	inv.InvoiceTypeCode = CodeDocument(root.integer("cbc:InvoiceTypeCode"))
	if inv.InvoiceTypeCode == 0 {
		// Try CreditNoteTypeCode for credit notes
		inv.InvoiceTypeCode = CodeDocument(root.integer("cbc:CreditNoteTypeCode"))
	}
	// If still 0 and this is a CreditNote document, default to 381
	if inv.InvoiceTypeCode == 0 && prefix == "cn:" {
		inv.InvoiceTypeCode = 381
	}

	// BT-2: Invoice date
	var err error
	inv.InvoiceDate, err = parseTimeUBL(root, "cbc:IssueDate")
	if err != nil {
		return err
	}

	// BT-72: Actual delivery date (optional, in cac:Delivery)
	inv.OccurrenceDateTime, err = parseTimeUBL(root, "cac:Delivery/cbc:ActualDeliveryDate")
	if err != nil {
		return fmt.Errorf("invalid occurrence date time: %w", err)
	}

	// BT-5: Invoice currency
	inv.InvoiceCurrencyCode = root.value("cbc:DocumentCurrencyCode")

	// BT-6: Tax currency (optional)
	inv.TaxCurrencyCode = root.value("cbc:TaxCurrencyCode")

	// BT-10: Buyer reference (optional)
	inv.BuyerReference = root.value("cbc:BuyerReference")

	// BT-19: Accounting cost (Buyer accounting reference)
	inv.ReceivableSpecifiedTradeAccountingAccount = root.value("cbc:AccountingCost")

	// BT-13: Purchase order reference
	inv.BuyerOrderReferencedDocument = root.value("cac:OrderReference/cbc:ID")

	// BT-14: Sales order reference
	inv.SellerOrderReferencedDocument = root.value("cac:OrderReference/cbc:SalesOrderID")

	// BT-12: Contract document reference
	inv.ContractReferencedDocument = root.value("cac:ContractDocumentReference/cbc:ID")

	// BT-11: Project reference
	inv.SpecifiedProcuringProjectID = root.value("cac:ProjectReference/cbc:ID")

	// BT-16: Despatch advice reference
	inv.DespatchAdviceReferencedDocument = root.value("cac:DespatchDocumentReference/cbc:ID")

	// BT-15: Receiving advice reference
	inv.ReceivingAdviceReferencedDocument = root.value("cac:ReceiptDocumentReference/cbc:ID")

	// BG-1: Process notes
	noteCount := root.count("cbc:Note")
	if noteCount > 0 {
		inv.Notes = make([]Note, 0, noteCount)
		for _, note := range root.nodes("cbc:Note") {
			inv.Notes = append(inv.Notes, Note{
				Text: note.value(""),
				// UBL doesn't typically have subject codes in Note elements
			})
		}
	}

	// BG-3: Preceding invoice references
	refCount := root.count("cac:BillingReference/cac:InvoiceDocumentReference")
	if refCount > 0 {
		inv.InvoiceReferencedDocument = make([]ReferencedDocument, 0, refCount)
		for _, ref := range root.nodes("cac:BillingReference/cac:InvoiceDocumentReference") {
			refDoc := ReferencedDocument{
				ID: ref.value("cbc:ID"),
			}

			refDoc.Date, err = parseTimeUBL(ref, "cbc:IssueDate")
			if err != nil {
				return fmt.Errorf("invalid referenced document date: %w", err)
			}

			inv.InvoiceReferencedDocument = append(inv.InvoiceReferencedDocument, refDoc)
		}
	}

	// BG-14: Invoice period (document level)
	// BR-CO-19: Track BG-14 (INVOICING PERIOD) presence to validate later
	if root.exists("cac:InvoicePeriod") {
		inv.hasBillingPeriodInXML = true
		inv.BillingSpecifiedPeriodStart, err = parseTimeUBL(root, "cac:InvoicePeriod/cbc:StartDate")
		if err != nil {
			return fmt.Errorf("invalid billing period start date: %w", err)
		}
		inv.BillingSpecifiedPeriodEnd, err = parseTimeUBL(root, "cac:InvoicePeriod/cbc:EndDate")
		if err != nil {
			return fmt.Errorf("invalid billing period end date: %w", err)
		}
	}

	// BG-24: Additional supporting documents
	docCount := root.count("cac:AdditionalDocumentReference")
	if docCount > 0 {
		inv.AdditionalReferencedDocument = make([]Document, 0, docCount)
		for _, doc := range root.nodes("cac:AdditionalDocumentReference") {
			addDoc := Document{
				IssuerAssignedID: doc.value("cbc:ID"),
				TypeCode:         doc.value("cbc:DocumentTypeCode"),
				Name:             doc.value("cbc:DocumentDescription"),
				URIID:            doc.value("cac:Attachment/cac:ExternalReference/cbc:URI"),
			}

			// Handle embedded binary object
			binaryData := doc.value("cac:Attachment/cbc:EmbeddedDocumentBinaryObject")
			if binaryData != "" {
				addDoc.AttachmentMimeCode = doc.node("cac:Attachment/cbc:EmbeddedDocumentBinaryObject").attribute("mimeCode")
				addDoc.AttachmentFilename = doc.node("cac:Attachment/cbc:EmbeddedDocumentBinaryObject").attribute("filename")

				// Decode base64-encoded attachment data
				data, err := base64.StdEncoding.DecodeString(binaryData)
				if err != nil {
					return fmt.Errorf("cannot decode attachment: %w", err)
				}
				addDoc.AttachmentBinaryObject = data
			}

			inv.AdditionalReferencedDocument = append(inv.AdditionalReferencedDocument, addDoc)
		}
	}

	return nil
}

// parseUBLParties parses all party elements (BG-4, BG-7, BG-10, BG-11, BG-13).
func parseUBLParties(root *xmlNode, inv *Invoice, prefix string) error {
	// BG-4: Seller (AccountingSupplierParty)
	inv.Seller = parseUBLParty(root.node("cac:AccountingSupplierParty/cac:Party"))

	// BG-7: Buyer (AccountingCustomerParty)
	inv.Buyer = parseUBLParty(root.node("cac:AccountingCustomerParty/cac:Party"))

	// BG-10: Payee (optional)
	if root.exists("cac:PayeeParty") {
		payee := parseUBLParty(root.node("cac:PayeeParty"))
		inv.PayeeTradeParty = &payee
	}

	// BG-11: Seller tax representative (optional)
	if root.exists("cac:TaxRepresentativeParty") {
		taxRep := parseUBLParty(root.node("cac:TaxRepresentativeParty"))
		inv.SellerTaxRepresentativeTradeParty = &taxRep
	}

	// BG-13: Delivery party and delivery address may be supplied separately.
	if deliveryCtx := root.node("cac:Delivery"); deliveryCtx != nil {
		if deliveryPartyCtx := deliveryCtx.node("cac:DeliveryParty"); deliveryPartyCtx != nil {
			shipTo := parseUBLParty(deliveryPartyCtx)
			inv.ShipTo = &shipTo
		}
		if location := deliveryCtx.node("cac:DeliveryLocation"); location != nil {
			if address := location.node("cac:Address"); address != nil {
				if inv.ShipTo == nil {
					inv.ShipTo = &Party{}
				}
				inv.ShipTo.PostalAddress = parseUBLPostalAddress(address)
			}
		}
	}

	return nil
}

func parseUBLPostalAddress(address *xmlNode) *PostalAddress {
	return &PostalAddress{
		Line1:                  address.value("cbc:StreetName"),
		Line2:                  address.value("cbc:AdditionalStreetName"),
		Line3:                  address.value("cac:AddressLine/cbc:Line"),
		City:                   address.value("cbc:CityName"),
		PostcodeCode:           address.value("cbc:PostalZone"),
		CountrySubDivisionName: address.value("cbc:CountrySubentity"),
		CountryID:              address.value("cac:Country/cbc:IdentificationCode"),
	}
}

// parseUBLParty parses a single party (reusable for Seller, Buyer, Payee, etc.).
// Takes a context already positioned at the party element.
func parseUBLParty(partyCtx *xmlNode) Party {
	party := Party{}

	// Electronic address (BT-34, BT-49, BT-98)
	party.URIUniversalCommunication = partyCtx.value("cbc:EndpointID")
	party.URIUniversalCommunicationScheme = partyCtx.node("cbc:EndpointID").attribute("schemeID")

	// Party identification (BT-29, BT-46, BT-60, BT-71)
	idCount := partyCtx.count("cac:PartyIdentification")
	if idCount > 0 {
		party.GlobalID = make([]GlobalID, 0, idCount)
		party.ID = make([]string, 0, idCount)
		for _, id := range partyCtx.nodes("cac:PartyIdentification") {
			idValue := id.value("cbc:ID")
			idScheme := id.node("cbc:ID").attribute("schemeID")

			if idScheme != "" {
				party.GlobalID = append(party.GlobalID, GlobalID{
					ID:     idValue,
					Scheme: idScheme,
				})
			} else {
				party.ID = append(party.ID, idValue)
			}
		}
	}

	// Party name (BT-27, BT-44, BT-59, BT-70)
	party.Name = partyCtx.value("cac:PartyName/cbc:Name")
	if party.Name == "" {
		// Fallback to PartyLegalEntity/RegistrationName
		party.Name = partyCtx.value("cac:PartyLegalEntity/cbc:RegistrationName")
	}

	// Postal address (BG-5, BG-8, BG-12, BG-15)
	if partyCtx.exists("cac:PostalAddress") {
		postalAddr := &PostalAddress{
			Line1:                  partyCtx.value("cac:PostalAddress/cbc:StreetName"),
			Line2:                  partyCtx.value("cac:PostalAddress/cbc:AdditionalStreetName"),
			Line3:                  partyCtx.value("cac:PostalAddress/cac:AddressLine/cbc:Line"),
			City:                   partyCtx.value("cac:PostalAddress/cbc:CityName"),
			PostcodeCode:           partyCtx.value("cac:PostalAddress/cbc:PostalZone"),
			CountrySubDivisionName: partyCtx.value("cac:PostalAddress/cbc:CountrySubentity"),
			CountryID:              partyCtx.value("cac:PostalAddress/cac:Country/cbc:IdentificationCode"),
		}
		party.PostalAddress = postalAddr
	}

	// Legal organization (BT-30, BT-47, BT-61)
	if partyCtx.exists("cac:PartyLegalEntity") {
		legalOrg := &SpecifiedLegalOrganization{
			ID:                  partyCtx.value("cac:PartyLegalEntity/cbc:CompanyID"),
			Scheme:              partyCtx.node("cac:PartyLegalEntity/cbc:CompanyID").attribute("schemeID"),
			TradingBusinessName: partyCtx.value("cac:PartyLegalEntity/cbc:RegistrationName"),
		}
		party.SpecifiedLegalOrganization = legalOrg
	}

	// Tax registration (BT-31, BT-32, BT-48, BT-63)
	for _, taxScheme := range partyCtx.nodes("cac:PartyTaxScheme") {
		taxID := taxScheme.value("cbc:CompanyID")
		scheme := taxScheme.value("cac:TaxScheme/cbc:ID")

		switch scheme {
		case "VAT":
			party.VATaxRegistration = taxID
		case "FC":
			party.FCTaxRegistration = taxID
		}
	}

	// Contact (BG-6, BG-9)
	contactCount := partyCtx.count("cac:Contact")
	if contactCount > 0 {
		party.DefinedTradeContact = make([]DefinedTradeContact, 0, contactCount)
		for _, contact := range partyCtx.nodes("cac:Contact") {
			dtc := DefinedTradeContact{
				PersonName:     contact.value("cbc:Name"),
				DepartmentName: contact.value("cbc:Department"),
				PhoneNumber:    contact.value("cbc:Telephone"),
				EMail:          contact.value("cbc:ElectronicMail"),
			}
			party.DefinedTradeContact = append(party.DefinedTradeContact, dtc)
		}
	}

	return party
}

// parseUBLAllowanceCharge parses document-level allowances and charges (BG-20, BG-21).
func parseUBLAllowanceCharge(root *xmlNode, inv *Invoice, prefix string) error {
	acCount := root.count("cac:AllowanceCharge")
	if acCount > 0 {
		inv.SpecifiedTradeAllowanceCharge = make([]AllowanceCharge, 0, acCount)
		for _, ac := range root.nodes("cac:AllowanceCharge") {
			chargeIndicator := ac.value("cbc:ChargeIndicator") == "true"

			basisAmount, err := getDecimal(ac, "cbc:BaseAmount")
			if err != nil {
				return err
			}

			actualAmount, err := getDecimal(ac, "cbc:Amount")
			if err != nil {
				return err
			}

			calculationPercent, err := getDecimal(ac, "cbc:MultiplierFactorNumeric")
			if err != nil {
				return err
			}

			categoryTaxRate, err := getDecimal(ac, "cac:TaxCategory/cbc:Percent")
			if err != nil {
				return err
			}

			allowanceCharge := AllowanceCharge{
				ChargeIndicator:                       chargeIndicator,
				BasisAmount:                           basisAmount,
				ActualAmount:                          actualAmount,
				CalculationPercent:                    calculationPercent,
				ReasonCode:                            ac.value("cbc:AllowanceChargeReasonCode"),
				Reason:                                ac.value("cbc:AllowanceChargeReason"),
				CategoryTradeTaxType:                  ac.value("cac:TaxCategory/cac:TaxScheme/cbc:ID"),
				CategoryTradeTaxCategoryCode:          ac.value("cac:TaxCategory/cbc:ID"),
				CategoryTradeTaxRateApplicablePercent: categoryTaxRate,
			}

			inv.SpecifiedTradeAllowanceCharge = append(inv.SpecifiedTradeAllowanceCharge, allowanceCharge)
		}
	}

	return nil
}

// parseUBLTaxTotal parses the tax breakdown (BG-23).
func parseUBLTaxTotal(root *xmlNode, inv *Invoice, prefix string) error {
	var err error

	// BT-110 and BT-111: Parse TaxTotal by matching currencyID (not position)
	// EN 16931 specifies which currency each total must be in, regardless of XML order
	for _, taxTotal := range root.nodes("cac:TaxTotal") {
		currency := taxTotal.node("cbc:TaxAmount").attribute("currencyID")
		if currency == "" {
			currency = inv.InvoiceCurrencyCode // Default if missing
		}

		amount, err := getDecimal(taxTotal, "cbc:TaxAmount")
		if err != nil {
			return fmt.Errorf("invalid TaxAmount with currency %s: %w", currency, err)
		}

		// BT-110: Tax total in invoice currency (must match BT-5)
		switch {
		case currency == inv.InvoiceCurrencyCode:
			inv.TaxTotalCurrency = currency
			inv.TaxTotal = amount
		case inv.TaxCurrencyCode != "" && currency == inv.TaxCurrencyCode:
			// BT-111: Tax total in accounting currency (must match BT-6)
			inv.TaxTotalAccountingCurrency = currency
			inv.TaxTotalAccounting = amount
		default:
			// Track unexpected TaxTotal currencies for validation
			inv.unexpectedTaxCurrencies = append(inv.unexpectedTaxCurrencies, currency)
		}
	}

	// BG-23: VAT breakdown (TaxSubtotal elements)
	taxSubtotalCount := root.count("cac:TaxTotal/cac:TaxSubtotal")
	if taxSubtotalCount > 0 {
		inv.TradeTaxes = make([]TradeTax, 0, taxSubtotalCount)
		for _, subtotal := range root.nodes("cac:TaxTotal/cac:TaxSubtotal") {
			tradeTax := TradeTax{}

			tradeTax.BasisAmount, err = getDecimal(subtotal, "cbc:TaxableAmount")
			if err != nil {
				return err
			}

			tradeTax.CalculatedAmount, err = getDecimal(subtotal, "cbc:TaxAmount")
			if err != nil {
				return err
			}

			tradeTax.TypeCode = subtotal.value("cac:TaxCategory/cac:TaxScheme/cbc:ID")
			if tradeTax.TypeCode == "" {
				tradeTax.TypeCode = "VAT" // Default to VAT
			}

			tradeTax.CategoryCode = subtotal.value("cac:TaxCategory/cbc:ID")

			tradeTax.Percent, err = getDecimal(subtotal, "cac:TaxCategory/cbc:Percent")
			if err != nil {
				return err
			}

			tradeTax.ExemptionReason = subtotal.value("cac:TaxCategory/cbc:TaxExemptionReason")
			tradeTax.ExemptionReasonCode = subtotal.value("cac:TaxCategory/cbc:TaxExemptionReasonCode")

			inv.TradeTaxes = append(inv.TradeTaxes, tradeTax)
		}
	}

	return nil
}

// parseUBLMonetarySummation parses the monetary totals (BT-106 to BT-115).
func parseUBLMonetarySummation(root *xmlNode, inv *Invoice, prefix string) error {
	legalMonetaryTotal := root.node("cac:LegalMonetaryTotal")

	// Track XML element presence for BR-12 through BR-15 validation
	inv.hasLineTotalInXML = legalMonetaryTotal.exists("cbc:LineExtensionAmount")
	inv.hasTaxBasisTotalInXML = legalMonetaryTotal.exists("cbc:TaxExclusiveAmount")
	inv.hasGrandTotalInXML = legalMonetaryTotal.exists("cbc:TaxInclusiveAmount")
	inv.hasDuePayableAmountInXML = legalMonetaryTotal.exists("cbc:PayableAmount")

	var err error

	// BT-106: Sum of Invoice line net amount
	inv.LineTotal, err = getDecimal(legalMonetaryTotal, "cbc:LineExtensionAmount")
	if err != nil {
		return err
	}

	// BT-107: Sum of allowances on document level
	inv.AllowanceTotal, err = getDecimal(legalMonetaryTotal, "cbc:AllowanceTotalAmount")
	if err != nil {
		return err
	}

	// BT-108: Sum of charges on document level
	inv.ChargeTotal, err = getDecimal(legalMonetaryTotal, "cbc:ChargeTotalAmount")
	if err != nil {
		return err
	}

	// BT-109: Invoice total amount without VAT
	inv.TaxBasisTotal, err = getDecimal(legalMonetaryTotal, "cbc:TaxExclusiveAmount")
	if err != nil {
		return err
	}

	// BT-112: Invoice total amount with VAT
	inv.GrandTotal, err = getDecimal(legalMonetaryTotal, "cbc:TaxInclusiveAmount")
	if err != nil {
		return err
	}

	// BT-113: Paid amount
	inv.TotalPrepaid, err = getDecimal(legalMonetaryTotal, "cbc:PrepaidAmount")
	if err != nil {
		return err
	}

	// BT-114: Rounding amount
	inv.RoundingAmount, err = getDecimal(legalMonetaryTotal, "cbc:PayableRoundingAmount")
	if err != nil {
		return err
	}

	// BT-115: Amount due for payment
	inv.DuePayableAmount, err = getDecimal(legalMonetaryTotal, "cbc:PayableAmount")
	if err != nil {
		return err
	}

	return nil
}

// parseUBLPaymentMeans parses payment means elements (BG-16, BG-17, BG-18, BG-19).
func parseUBLPaymentMeans(root *xmlNode, inv *Invoice, prefix string) error {
	pmCount := root.count("cac:PaymentMeans")
	if pmCount > 0 {
		inv.PaymentMeans = make([]PaymentMeans, 0, pmCount)
		for _, pm := range root.nodes("cac:PaymentMeans") {
			paymentMeans := PaymentMeans{
				TypeCode:    pm.integer("cbc:PaymentMeansCode"),
				Information: pm.value("cbc:InstructionNote"),
			}

			// BT-83: Remittance information
			inv.PaymentReference = pm.value("cbc:PaymentID")

			// BG-17: Credit transfer (IBAN/BIC)
			if pm.exists("cac:PayeeFinancialAccount") {
				paymentMeans.PayeePartyCreditorFinancialAccountIBAN = pm.value("cac:PayeeFinancialAccount/cbc:ID")
				paymentMeans.PayeePartyCreditorFinancialAccountName = pm.value("cac:PayeeFinancialAccount/cbc:Name")
				paymentMeans.PayeePartyCreditorFinancialAccountProprietaryID = pm.value("cac:PayeeFinancialAccount/cac:ID")
				paymentMeans.PayeeSpecifiedCreditorFinancialInstitutionBIC = pm.value("cac:PayeeFinancialAccount/cac:FinancialInstitutionBranch/cbc:ID")
				// BR-61: Track XML element presence to validate later.
				// Per EN 16931 schematron, BR-61 test is "(ram:IBANID) or (ram:ProprietaryID)"
				// which checks for element PRESENCE, not value. An empty element <cbc:ID/>
				// satisfies the test because the element exists.
				paymentMeans.hasPayeeAccountInXML = true
				paymentMeans.hasPayeeIBANInXML = pm.exists("cac:PayeeFinancialAccount/cbc:ID")
				paymentMeans.hasPayeeProprietaryIDInXML = pm.exists("cac:PayeeFinancialAccount/cac:ID")
			}

			// BG-18: Payment card information
			if pm.exists("cac:CardAccount") {
				paymentMeans.ApplicableTradeSettlementFinancialCardID = pm.value("cac:CardAccount/cbc:PrimaryAccountNumberID")
				paymentMeans.ApplicableTradeSettlementFinancialCardCardholderName = pm.value("cac:CardAccount/cbc:HolderName")
			}

			// BG-19: Direct debit
			if pm.exists("cac:PaymentMandate") {
				paymentMeans.PayerPartyDebtorFinancialAccountIBAN = pm.value("cac:PaymentMandate/cac:PayerFinancialAccount/cbc:ID")
			}

			inv.PaymentMeans = append(inv.PaymentMeans, paymentMeans)
		}
	}

	return nil
}

// parseUBLPaymentTerms parses payment terms (BT-20, BT-9, BT-89).
func parseUBLPaymentTerms(root *xmlNode, inv *Invoice, prefix string) error {
	// BT-9: Invoice due dates are normally supplied at the root level.
	rootDueDate, err := parseTimeUBL(root, "cbc:DueDate")
	if err != nil {
		return err
	}

	ptCount := root.count("cac:PaymentTerms")
	if ptCount > 0 {
		inv.SpecifiedTradePaymentTerms = make([]SpecifiedTradePaymentTerms, 0, ptCount)
		for _, pt := range root.nodes("cac:PaymentTerms") {
			paymentTerm := SpecifiedTradePaymentTerms{
				Description: pt.value("cbc:Note"),
			}

			// BT-9: Payment due date (prefer element-level DueDate if present)
			paymentTerm.DueDate, err = parseTimeUBL(pt, "cbc:PaymentDueDate")
			if err != nil {
				return err
			}
			// If not in PaymentTerms, use root-level DueDate
			if paymentTerm.DueDate.IsZero() && !rootDueDate.IsZero() {
				paymentTerm.DueDate = rootDueDate
			}

			// BT-89: Direct debit mandate identifier
			paymentTerm.DirectDebitMandateID = pt.value("cbc:PaymentMeansID")

			inv.SpecifiedTradePaymentTerms = append(inv.SpecifiedTradePaymentTerms, paymentTerm)
		}
	} else if !rootDueDate.IsZero() {
		// If there are no PaymentTerms elements but there's a root-level DueDate,
		// create a single PaymentTerms entry with just the DueDate
		inv.SpecifiedTradePaymentTerms = []SpecifiedTradePaymentTerms{
			{DueDate: rootDueDate},
		}
	}

	// CreditNote carries BT-9 in PaymentMeans/PaymentDueDate. Preserve the
	// application's existing preference for this value in the first terms entry.
	if prefix == "cn:" || inv.InvoiceTypeCode == 381 {
		for _, means := range root.nodes("cac:PaymentMeans") {
			for _, dueNode := range means.nodes("cbc:PaymentDueDate") {
				due, err := time.Parse("2006-01-02", dueNode.value(""))
				if err != nil {
					return fmt.Errorf("invalid CreditNote PaymentDueDate: %w", err)
				}
				if len(inv.SpecifiedTradePaymentTerms) == 0 {
					inv.SpecifiedTradePaymentTerms = []SpecifiedTradePaymentTerms{{}}
				}
				inv.SpecifiedTradePaymentTerms[0].DueDate = due
			}
		}
	}

	return nil
}

// parseUBLLines parses all invoice line items (BG-25).
func parseUBLLines(root *xmlNode, inv *Invoice, prefix string) error {
	// Determine line element and quantity element names based on document type
	lineElementName := "cac:InvoiceLine"
	quantityElementName := "cbc:InvoicedQuantity"
	if prefix == "cn:" {
		lineElementName = "cac:CreditNoteLine"
		quantityElementName = "cbc:CreditedQuantity"
	}

	lineCount := root.count(lineElementName)
	if lineCount > 0 {
		inv.InvoiceLines = make([]InvoiceLine, 0, lineCount)
	}

	for _, lineItem := range root.nodes(lineElementName) {
		invoiceLine := InvoiceLine{}
		var err error

		// BT-126: Invoice line identifier
		invoiceLine.LineID = lineItem.value("cbc:ID")

		// BT-127: Invoice line note
		invoiceLine.Note = lineItem.value("cbc:Note")

		// BG-26: Invoice line period
		// BR-CO-20: Track BG-26 (INVOICE LINE PERIOD) presence to validate later
		if lineItem.exists("cac:InvoicePeriod") {
			invoiceLine.linePeriodPresent = true
			invoiceLine.BillingSpecifiedPeriodStart, err = parseTimeUBL(lineItem, "cac:InvoicePeriod/cbc:StartDate")
			if err != nil {
				return fmt.Errorf("invalid line billing period start date for line %s: %w", invoiceLine.LineID, err)
			}
			invoiceLine.BillingSpecifiedPeriodEnd, err = parseTimeUBL(lineItem, "cac:InvoicePeriod/cbc:EndDate")
			if err != nil {
				return fmt.Errorf("invalid line billing period end date for line %s: %w", invoiceLine.LineID, err)
			}
		}

		// BT-128: Invoice line object identifier
		invoiceLine.AdditionalReferencedDocumentID = lineItem.value("cac:DocumentReference/cbc:ID")
		invoiceLine.AdditionalReferencedDocumentTypeCode = lineItem.value("cac:DocumentReference/cbc:DocumentTypeCode")

		// BT-132: Referenced purchase order line
		invoiceLine.BuyerOrderReferencedDocument = lineItem.value("cac:OrderLineReference/cbc:LineID")

		// BT-133: Invoice line Buyer accounting reference
		invoiceLine.ReceivableSpecifiedTradeAccountingAccount = lineItem.value("cbc:AccountingCost")

		// BT-129: Invoiced quantity (or Credited quantity for credit notes)
		invoiceLine.BilledQuantity, err = getDecimal(lineItem, quantityElementName)
		if err != nil {
			return err
		}

		// BT-130: Invoiced quantity unit of measure
		invoiceLine.BilledQuantityUnit = lineItem.node(quantityElementName).attribute("unitCode")

		// BT-131: Invoice line net amount
		// Track XML element presence for BR-24 validation
		invoiceLine.hasLineTotalInXML = lineItem.exists("cbc:LineExtensionAmount")
		invoiceLine.Total, err = getDecimal(lineItem, "cbc:LineExtensionAmount")
		if err != nil {
			return err
		}

		// Parse item information
		if err := parseUBLLineItem(lineItem, &invoiceLine); err != nil {
			return err
		}

		// Parse price information
		if err := parseUBLLinePrice(lineItem, &invoiceLine); err != nil {
			return err
		}

		// BG-27: Line level allowances
		// BG-28: Line level charges
		lineACCount := lineItem.count("cac:AllowanceCharge")
		if lineACCount > 0 {
			// Pre-allocate both slices with full capacity since we don't know the split
			invoiceLine.InvoiceLineAllowances = make([]AllowanceCharge, 0, lineACCount)
			invoiceLine.InvoiceLineCharges = make([]AllowanceCharge, 0, lineACCount)
			for _, ac := range lineItem.nodes("cac:AllowanceCharge") {
				chargeIndicator := ac.value("cbc:ChargeIndicator") == "true"

				basisAmount, err := getDecimal(ac, "cbc:BaseAmount")
				if err != nil {
					return err
				}

				actualAmount, err := getDecimal(ac, "cbc:Amount")
				if err != nil {
					return err
				}

				calculationPercent, err := getDecimal(ac, "cbc:MultiplierFactorNumeric")
				if err != nil {
					return err
				}

				alc := AllowanceCharge{
					ChargeIndicator:    chargeIndicator,
					BasisAmount:        basisAmount,
					ActualAmount:       actualAmount,
					CalculationPercent: calculationPercent,
					ReasonCode:         ac.value("cbc:AllowanceChargeReasonCode"),
					Reason:             ac.value("cbc:AllowanceChargeReason"),
				}

				if chargeIndicator {
					invoiceLine.InvoiceLineCharges = append(invoiceLine.InvoiceLineCharges, alc)
				} else {
					invoiceLine.InvoiceLineAllowances = append(invoiceLine.InvoiceLineAllowances, alc)
				}
			}
		}

		// Parse line tax information
		taxInfo := lineItem.node("cac:Item/cac:ClassifiedTaxCategory")
		invoiceLine.TaxTypeCode = taxInfo.value("cac:TaxScheme/cbc:ID")
		if invoiceLine.TaxTypeCode == "" {
			invoiceLine.TaxTypeCode = "VAT" // Default to VAT
		}
		invoiceLine.TaxCategoryCode = taxInfo.value("cbc:ID")
		invoiceLine.TaxRateApplicablePercent, err = getDecimal(taxInfo, "cbc:Percent")
		if err != nil {
			return err
		}
		invoiceLine.hasTaxRateApplicablePercent = taxInfo.exists("cbc:Percent")

		inv.InvoiceLines = append(inv.InvoiceLines, invoiceLine)
	}

	return nil
}

// parseUBLLineItem parses item-specific information within a line.
func parseUBLLineItem(lineItem *xmlNode, invoiceLine *InvoiceLine) error {
	item := lineItem.node("cac:Item")

	// BT-153: Item name
	invoiceLine.ItemName = item.value("cbc:Name")

	// BT-154: Item description
	invoiceLine.Description = item.value("cbc:Description")

	// BT-155: Item Seller's identifier
	invoiceLine.ArticleNumber = item.value("cac:SellersItemIdentification/cbc:ID")

	// BT-156: Item Buyer's identifier
	invoiceLine.ArticleNumberBuyer = item.value("cac:BuyersItemIdentification/cbc:ID")

	// BT-157: Item standard identifier
	invoiceLine.GlobalID = item.value("cac:StandardItemIdentification/cbc:ID")
	invoiceLine.GlobalIDType = item.node("cac:StandardItemIdentification/cbc:ID").attribute("schemeID")

	// BT-158: Item classification identifier
	classCount := item.count("cac:CommodityClassification")
	if classCount > 0 {
		invoiceLine.ProductClassification = make([]Classification, 0, classCount)
		for _, class := range item.nodes("cac:CommodityClassification") {
			classification := Classification{
				ClassCode:     class.value("cbc:ItemClassificationCode"),
				ListID:        class.node("cbc:ItemClassificationCode").attribute("listID"),
				ListVersionID: class.node("cbc:ItemClassificationCode").attribute("listVersionID"),
			}
			invoiceLine.ProductClassification = append(invoiceLine.ProductClassification, classification)
		}
	}

	// BT-159: Item country of origin
	invoiceLine.OriginTradeCountry = item.value("cac:OriginCountry/cbc:IdentificationCode")

	// BG-32: Item attributes
	attrCount := item.count("cac:AdditionalItemProperty")
	if attrCount > 0 {
		invoiceLine.Characteristics = make([]Characteristic, 0, attrCount)
		for _, attr := range item.nodes("cac:AdditionalItemProperty") {
			characteristic := Characteristic{
				Description: attr.value("cbc:Name"),
				Value:       attr.value("cbc:Value"),
			}
			invoiceLine.Characteristics = append(invoiceLine.Characteristics, characteristic)
		}
	}

	return nil
}

// parseUBLLinePrice parses price information within a line.
func parseUBLLinePrice(lineItem *xmlNode, invoiceLine *InvoiceLine) error {
	price := lineItem.node("cac:Price")

	var err error

	// BT-146: Item net price
	// Track XML element presence for BR-26 validation
	invoiceLine.hasNetPriceInXML = price.exists("cbc:PriceAmount")
	invoiceLine.NetPrice, err = getDecimal(price, "cbc:PriceAmount")
	if err != nil {
		return err
	}

	// BT-149: Item price base quantity with unit code
	invoiceLine.BasisQuantity, err = getDecimal(price, "cbc:BaseQuantity")
	if err != nil {
		return err
	}
	invoiceLine.BasisQuantityUnit = price.node("cbc:BaseQuantity").attribute("unitCode")

	// BT-148: Item gross price (price before allowances)
	// UBL doesn't have a direct gross price field, but may have allowances on price
	// For now, calculate from net price if allowances exist on price
	priceACCount := price.count("cac:AllowanceCharge")
	if priceACCount > 0 {
		invoiceLine.AppliedTradeAllowanceCharge = make([]AllowanceCharge, 0, priceACCount)
		for _, ac := range price.nodes("cac:AllowanceCharge") {
			chargeIndicator := ac.value("cbc:ChargeIndicator") == "true"

			basisAmount, err := getDecimal(ac, "cbc:BaseAmount")
			if err != nil {
				return err
			}

			actualAmount, err := getDecimal(ac, "cbc:Amount")
			if err != nil {
				return err
			}

			calculationPercent, err := getDecimal(ac, "cbc:MultiplierFactorNumeric")
			if err != nil {
				return err
			}

			allowanceCharge := AllowanceCharge{
				ChargeIndicator:    chargeIndicator,
				BasisAmount:        basisAmount,
				ActualAmount:       actualAmount,
				CalculationPercent: calculationPercent,
				ReasonCode:         ac.value("cbc:AllowanceChargeReasonCode"),
				Reason:             ac.value("cbc:AllowanceChargeReason"),
			}

			invoiceLine.AppliedTradeAllowanceCharge = append(invoiceLine.AppliedTradeAllowanceCharge, allowanceCharge)

			// Calculate gross price if we have basis amount
			if !basisAmount.IsZero() && invoiceLine.GrossPrice.IsZero() {
				invoiceLine.GrossPrice = basisAmount
			}
		}
	}

	return nil
}
