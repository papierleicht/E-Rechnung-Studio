// Adapted from github.com/speedata/einvoice under BSD-3-Clause; see LICENSE.

package einvoice

import (
	"encoding/base64"
	"fmt"
	"time"
)

// CII (ZUGFeRD/Factur-X) namespace URN for root element
const nsCIIRootInvoice = "urn:un:unece:uncefact:data:standard:CrossIndustryInvoice:100"

// parseCIITime parses CII format dates (YYYYMMDD) into time.Time.
func parseCIITime(ctx *xmlNode, path string) (time.Time, error) {
	timestring := ctx.value(path)
	if timestring == "" {
		return time.Time{}, nil
	}

	parsedDate, err := time.Parse("20060102", timestring)
	if err != nil {
		return parsedDate, fmt.Errorf("%w", err)
	}

	return parsedDate, nil
}

// parseCIIParty parses a party (buyer, seller, payee, etc.) from CII format.
// Uses fixed CII child paths resolved by namespace URI.
func parseCIIParty(tradeParty *xmlNode) Party {
	adr := Party{}
	for _, id := range tradeParty.nodes("ram:ID") {
		adr.ID = append(adr.ID, id.value(""))
	}

	for _, gid := range tradeParty.nodes("ram:GlobalID") {
		scheme := GlobalID{
			Scheme: gid.attribute("schemeID"),
			ID:     gid.value(""),
		}
		adr.GlobalID = append(adr.GlobalID, scheme)
	}

	adr.Name = tradeParty.value("ram:Name")
	// BT-33: Seller additional legal information (or buyer/payee/etc. description)
	adr.Description = tradeParty.value("ram:Description")

	// BT-34, BT-49: Electronic address with scheme
	adr.URIUniversalCommunication = tradeParty.value("ram:URIUniversalCommunication/ram:URIID")
	adr.URIUniversalCommunicationScheme = tradeParty.node("ram:URIUniversalCommunication/ram:URIID").attribute("schemeID")

	if tradeParty.exists("ram:SpecifiedLegalOrganization") {
		slo := SpecifiedLegalOrganization{}
		slo.ID = tradeParty.value("ram:SpecifiedLegalOrganization/ram:ID")
		slo.Scheme = tradeParty.node("ram:SpecifiedLegalOrganization/ram:ID").attribute("schemeID")
		slo.TradingBusinessName = tradeParty.value("ram:SpecifiedLegalOrganization/ram:TradingBusinessName")
		adr.SpecifiedLegalOrganization = &slo
	}

	for _, dtc := range tradeParty.nodes("ram:DefinedTradeContact") {
		contact := DefinedTradeContact{}
		contact.PersonName = dtc.value("ram:PersonName")
		contact.DepartmentName = dtc.value("ram:DepartmentName")
		contact.PhoneNumber = dtc.value("ram:TelephoneUniversalCommunication/ram:CompleteNumber")
		contact.EMail = dtc.value("ram:EmailURIUniversalCommunication/ram:URIID")
		adr.DefinedTradeContact = append(adr.DefinedTradeContact, contact)
	}

	if tradeParty.exists("ram:PostalTradeAddress") {
		postalAddress := &PostalAddress{
			PostcodeCode:           tradeParty.value("ram:PostalTradeAddress/ram:PostcodeCode"),
			Line1:                  tradeParty.value("ram:PostalTradeAddress/ram:LineOne"),
			Line2:                  tradeParty.value("ram:PostalTradeAddress/ram:LineTwo"),
			Line3:                  tradeParty.value("ram:PostalTradeAddress/ram:LineThree"),
			City:                   tradeParty.value("ram:PostalTradeAddress/ram:CityName"),
			CountryID:              tradeParty.value("ram:PostalTradeAddress/ram:CountryID"),
			CountrySubDivisionName: tradeParty.value("ram:PostalTradeAddress/ram:CountrySubDivisionName"),
		}
		adr.PostalAddress = postalAddress
	}

	adr.FCTaxRegistration = tradeParty.taxRegistration("FC")
	adr.VATaxRegistration = tradeParty.taxRegistration("VA")

	return adr
}

// parseCII interprets the XML file as a ZUGFeRD or Factur-X cross industry invoice.
// It maps the CII document structure to the application invoice model.
func parseCII(ctx *xmlNode) (*Invoice, error) {
	root := ctx

	inv := &Invoice{SchemaType: CII}

	var err error
	if err = parseCIIExchangedDocumentContext(root.node("rsm:ExchangedDocumentContext"), inv); err != nil {
		return nil, err
	}

	if err = parseCIIExchangedDocument(root.node("rsm:ExchangedDocument"), inv); err != nil {
		return nil, err
	}

	if err = parseCIISupplyChainTradeTransaction(root.node("rsm:SupplyChainTradeTransaction"), inv); err != nil {
		return nil, err
	}

	return inv, nil
}

func parseCIIExchangedDocumentContext(ctx *xmlNode, inv *Invoice) error {
	// Store the raw URN value (BT-24 - Specification identifier)
	nc := ctx.node("ram:GuidelineSpecifiedDocumentContextParameter").node("ram:ID")
	inv.GuidelineSpecifiedDocumentContextParameter = nc.value("")

	// Store the business process identifier (BT-23)
	inv.BPSpecifiedDocumentContextParameter = ctx.value("ram:BusinessProcessSpecifiedDocumentContextParameter/ram:ID")

	return nil
}

func parseCIIExchangedDocument(exchangedDocument *xmlNode, inv *Invoice) error {
	inv.InvoiceNumber = exchangedDocument.value("ram:ID")
	inv.InvoiceTypeCode = CodeDocument(exchangedDocument.integer("ram:TypeCode"))

	invoiceDate, err := parseCIITime(exchangedDocument, "ram:IssueDateTime/udt:DateTimeString")
	if err != nil {
		return err
	}

	inv.InvoiceDate = invoiceDate

	for _, note := range exchangedDocument.nodes("ram:IncludedNote") {
		n := Note{}
		n.SubjectCode = note.value("ram:SubjectCode")
		n.Text = note.value("ram:Content")
		inv.Notes = append(inv.Notes, n)
	}

	return nil
}

func parseCIISupplyChainTradeTransaction(supplyChainTradeTransaction *xmlNode, inv *Invoice) error {
	var err error
	// BG-25
	for _, lineItem := range supplyChainTradeTransaction.nodes("ram:IncludedSupplyChainTradeLineItem") {
		invoiceLine := InvoiceLine{}
		invoiceLine.LineID = lineItem.value("ram:AssociatedDocumentLineDocument/ram:LineID")
		invoiceLine.ParentLineID = lineItem.value("ram:AssociatedDocumentLineDocument/ram:ParentLineID")
		invoiceLine.LineStatusCode = lineItem.value("ram:AssociatedDocumentLineDocument/ram:LineStatusCode")
		invoiceLine.LineStatusReasonCode = lineItem.value("ram:AssociatedDocumentLineDocument/ram:LineStatusReasonCode")
		invoiceLine.Note = lineItem.value("ram:AssociatedDocumentLineDocument/ram:IncludedNote/ram:Content")

		parseSpecifiedTradeProduct(lineItem.node("ram:SpecifiedTradeProduct"), &invoiceLine)
		specifiedLineTradeAgreement := lineItem.node("ram:SpecifiedLineTradeAgreement")
		if err = parseSpecifiedLineTradeAgreement(specifiedLineTradeAgreement, &invoiceLine); err != nil {
			return err
		}

		invoiceLine.BilledQuantity, err = getDecimal(lineItem, "ram:SpecifiedLineTradeDelivery/ram:BilledQuantity")
		if err != nil {
			return err
		}
		invoiceLine.BilledQuantityUnit = lineItem.node("ram:SpecifiedLineTradeDelivery/ram:BilledQuantity").attribute("unitCode")
		// BR-24: Track XML element presence to validate later
		invoiceLine.hasLineTotalInXML = lineItem.exists("ram:SpecifiedLineTradeSettlement/ram:SpecifiedTradeSettlementLineMonetarySummation/ram:LineTotalAmount")
		invoiceLine.Total, err = getDecimal(lineItem, "ram:SpecifiedLineTradeSettlement/ram:SpecifiedTradeSettlementLineMonetarySummation/ram:LineTotalAmount")
		if err != nil {
			return err
		}

		for _, allowanceCharge := range lineItem.nodes("ram:SpecifiedLineTradeSettlement/ram:SpecifiedTradeAllowanceCharge") {
			basisAmount, err := getDecimal(allowanceCharge, "ram:BasisAmount")
			if err != nil {
				return err
			}
			actualAmount, err := getDecimal(allowanceCharge, "ram:ActualAmount")
			if err != nil {
				return err
			}
			calculationPercent, err := getDecimal(allowanceCharge, "ram:CalculationPercent")
			if err != nil {
				return err
			}
			categoryTaxRate, err := getDecimal(allowanceCharge, "ram:CategoryTradeTax/ram:RateApplicablePercent")
			if err != nil {
				return err
			}

			alc := AllowanceCharge{
				ChargeIndicator:                       allowanceCharge.value("ram:ChargeIndicator/udt:Indicator") == "true",
				BasisAmount:                           basisAmount,
				ActualAmount:                          actualAmount,
				CalculationPercent:                    calculationPercent,
				ReasonCode:                            allowanceCharge.value("ram:ReasonCode"),
				Reason:                                allowanceCharge.value("ram:Reason"),
				CategoryTradeTaxType:                  allowanceCharge.value("ram:CategoryTradeTax/ram:TypeCode"),
				CategoryTradeTaxCategoryCode:          allowanceCharge.value("ram:CategoryTradeTax/ram:CategoryCode"),
				CategoryTradeTaxRateApplicablePercent: categoryTaxRate,
			}
			// Im Fall eines Abschlags (BG-27) ist der Wert des ChargeIndicators auf "false" zu setzen.
			// Im Fall eines Zuschlags (BG-28) ist der Wert des ChargeIndicators auf "true" zu setzen.
			if alc.ChargeIndicator {
				invoiceLine.InvoiceLineCharges = append(invoiceLine.InvoiceLineCharges, alc)
			} else {
				invoiceLine.InvoiceLineAllowances = append(invoiceLine.InvoiceLineAllowances, alc)
			}
		}

		taxInfo := lineItem.node("ram:SpecifiedLineTradeSettlement/ram:ApplicableTradeTax")
		// BG-27, BG-28
		invoiceLine.TaxTypeCode = taxInfo.value("ram:TypeCode")
		invoiceLine.TaxCategoryCode = taxInfo.value("ram:CategoryCode")
		invoiceLine.TaxRateApplicablePercent, err = getDecimal(taxInfo, "ram:RateApplicablePercent")
		if err != nil {
			return err
		}
		invoiceLine.hasTaxRateApplicablePercent = taxInfo.exists("ram:RateApplicablePercent")
		// BR-CO-20: Track BG-26 (INVOICE LINE PERIOD) presence to validate later
		invoiceLine.linePeriodPresent = lineItem.exists("ram:SpecifiedLineTradeSettlement/ram:BillingSpecifiedPeriod")
		invoiceLine.BillingSpecifiedPeriodStart, err = parseCIITime(lineItem, "ram:SpecifiedLineTradeSettlement/ram:BillingSpecifiedPeriod/ram:StartDateTime/udt:DateTimeString")
		if err != nil {
			return fmt.Errorf("invalid line billing period start date for line %s: %w", invoiceLine.LineID, err)
		}
		invoiceLine.BillingSpecifiedPeriodEnd, err = parseCIITime(lineItem, "ram:SpecifiedLineTradeSettlement/ram:BillingSpecifiedPeriod/ram:EndDateTime/udt:DateTimeString")
		if err != nil {
			return fmt.Errorf("invalid line billing period end date for line %s: %w", invoiceLine.LineID, err)
		}

		// BT-128: Referenced document (line level)
		invoiceLine.AdditionalReferencedDocumentID = lineItem.value("ram:SpecifiedLineTradeSettlement/ram:AdditionalReferencedDocument/ram:IssuerAssignedID")

		inv.InvoiceLines = append(inv.InvoiceLines, invoiceLine)
	}
	if err = parseCIIApplicableHeaderTradeAgreement(supplyChainTradeTransaction.node("ram:ApplicableHeaderTradeAgreement"), inv); err != nil {
		return err
	}

	if err = parseCIIApplicableHeaderTradeDelivery(supplyChainTradeTransaction.node("ram:ApplicableHeaderTradeDelivery"), inv); err != nil {
		return err
	}

	if err = parseCIIApplicableHeaderTradeSettlement(supplyChainTradeTransaction.node("ram:ApplicableHeaderTradeSettlement"), inv); err != nil {
		return err
	}

	return nil
}

func parseCIIApplicableHeaderTradeAgreement(applicableHeaderTradeAgreement *xmlNode, inv *Invoice) error {
	inv.BuyerReference = applicableHeaderTradeAgreement.value("ram:BuyerReference")
	// BT-13
	inv.BuyerOrderReferencedDocument = applicableHeaderTradeAgreement.value("ram:BuyerOrderReferencedDocument/ram:IssuerAssignedID") // BT-13
	// BT-12
	inv.ContractReferencedDocument = applicableHeaderTradeAgreement.value("ram:ContractReferencedDocument/ram:IssuerAssignedID") // BT-13
	inv.Buyer = parseCIIParty(applicableHeaderTradeAgreement.node("ram:BuyerTradeParty"))
	inv.Seller = parseCIIParty(applicableHeaderTradeAgreement.node("ram:SellerTradeParty"))

	if applicableHeaderTradeAgreement.exists("ram:SellerTaxRepresentativeTradeParty") {
		trp := parseCIIParty(applicableHeaderTradeAgreement.node("ram:SellerTaxRepresentativeTradeParty"))
		inv.SellerTaxRepresentativeTradeParty = &trp
	}

	// BT-14: Seller order reference
	inv.SellerOrderReferencedDocument = applicableHeaderTradeAgreement.value("ram:SellerOrderReferencedDocument/ram:IssuerAssignedID")

	for _, additionalDocument := range applicableHeaderTradeAgreement.nodes("ram:AdditionalReferencedDocument") {
		doc := Document{}
		doc.IssuerAssignedID = additionalDocument.value("ram:IssuerAssignedID")
		doc.URIID = additionalDocument.value("ram:URIID") // BT-124
		encoded := additionalDocument.value("ram:AttachmentBinaryObject")

		if encoded != "" {
			data, err := base64.StdEncoding.DecodeString(encoded)
			if err != nil {
				return fmt.Errorf("cannot decode attachment %w", err)
			}

			doc.AttachmentBinaryObject = data
		}

		doc.AttachmentFilename = additionalDocument.node("ram:AttachmentBinaryObject").attribute("filename")
		doc.AttachmentMimeCode = additionalDocument.node("ram:AttachmentBinaryObject").attribute("mimeCode")
		doc.Name = additionalDocument.value("ram:Name")
		doc.TypeCode = additionalDocument.value("ram:TypeCode")
		doc.ReferenceTypeCode = additionalDocument.value("ram:ReferenceTypeCode")
		inv.AdditionalReferencedDocument = append(inv.AdditionalReferencedDocument, doc)
	}

	// BT-11: Project reference (ID and Name)
	inv.SpecifiedProcuringProjectID = applicableHeaderTradeAgreement.value("ram:SpecifiedProcuringProject/ram:ID")
	inv.SpecifiedProcuringProjectName = applicableHeaderTradeAgreement.value("ram:SpecifiedProcuringProject/ram:Name")

	return nil
}

func parseCIIApplicableHeaderTradeDelivery(applicableHeaderTradeDelivery *xmlNode, inv *Invoice) error {
	// BT-16: Despatch advice reference
	inv.DespatchAdviceReferencedDocument = applicableHeaderTradeDelivery.value("ram:DespatchAdviceReferencedDocument/ram:IssuerAssignedID")
	// BT-15: Receiving advice reference
	inv.ReceivingAdviceReferencedDocument = applicableHeaderTradeDelivery.value("ram:ReceivingAdviceReferencedDocument/ram:IssuerAssignedID")
	// BT-72
	var err error
	inv.OccurrenceDateTime, err = parseCIITime(applicableHeaderTradeDelivery, "ram:ActualDeliverySupplyChainEvent/ram:OccurrenceDateTime/udt:DateTimeString")
	if err != nil {
		return fmt.Errorf("invalid occurrence date time: %w", err)
	}

	if applicableHeaderTradeDelivery.exists("ram:ShipToTradeParty") {
		st := parseCIIParty(applicableHeaderTradeDelivery.node("ram:ShipToTradeParty"))
		inv.ShipTo = &st
	}
	return nil
}

func parseCIIApplicableHeaderTradeSettlement(applicableHeaderTradeSettlement *xmlNode, inv *Invoice) error {
	var err error

	inv.InvoiceCurrencyCode = applicableHeaderTradeSettlement.value("ram:InvoiceCurrencyCode")
	// BT-6: Tax currency code (accounting currency, if different from invoice currency)
	inv.TaxCurrencyCode = applicableHeaderTradeSettlement.value("ram:TaxCurrencyCode")
	// BT-90: Creditor reference ID
	inv.CreditorReferenceID = applicableHeaderTradeSettlement.value("ram:CreditorReferenceID")
	// BT-83: Payment reference (remittance information)
	inv.PaymentReference = applicableHeaderTradeSettlement.value("ram:PaymentReference")
	// BG-10
	if applicableHeaderTradeSettlement.exists("ram:PayeeTradeParty") {
		ptp := parseCIIParty(applicableHeaderTradeSettlement.node("ram:PayeeTradeParty"))
		inv.PayeeTradeParty = &ptp
	}

	for _, paymentMeans := range applicableHeaderTradeSettlement.nodes("ram:SpecifiedTradeSettlementPaymentMeans") {
		// BG-16
		thisPaymentMeans := PaymentMeans{
			TypeCode:                                             paymentMeans.integer("ram:TypeCode"),
			Information:                                          paymentMeans.value("ram:Information"),
			PayeePartyCreditorFinancialAccountIBAN:               paymentMeans.value("ram:PayeePartyCreditorFinancialAccount/ram:IBANID"),
			PayeePartyCreditorFinancialAccountName:               paymentMeans.value("ram:PayeePartyCreditorFinancialAccount/ram:AccountName"),
			PayeePartyCreditorFinancialAccountProprietaryID:      paymentMeans.value("ram:PayeePartyCreditorFinancialAccount/ram:ProprietaryID"),
			PayeeSpecifiedCreditorFinancialInstitutionBIC:        paymentMeans.value("ram:PayeeSpecifiedCreditorFinancialInstitution/ram:BICID"),
			PayerPartyDebtorFinancialAccountIBAN:                 paymentMeans.value("ram:PayerPartyDebtorFinancialAccount/ram:IBANID"),
			ApplicableTradeSettlementFinancialCardID:             paymentMeans.value("ram:ApplicableTradeSettlementFinancialCard/ram:ID"),
			ApplicableTradeSettlementFinancialCardCardholderName: paymentMeans.value("ram:ApplicableTradeSettlementFinancialCard/ram:CardholderName"),
			// BR-61: Track XML element presence to validate later.
			// Per EN 16931 schematron, BR-61 test is "(ram:IBANID) or (ram:ProprietaryID)"
			// which checks for element PRESENCE, not value. An empty element <ram:IBANID/>
			// satisfies the test because the element exists.
			hasPayeeAccountInXML:       paymentMeans.exists("ram:PayeePartyCreditorFinancialAccount"),
			hasPayeeIBANInXML:          paymentMeans.exists("ram:PayeePartyCreditorFinancialAccount/ram:IBANID"),
			hasPayeeProprietaryIDInXML: paymentMeans.exists("ram:PayeePartyCreditorFinancialAccount/ram:ProprietaryID"),
		}
		inv.PaymentMeans = append(inv.PaymentMeans, thisPaymentMeans)
	}

	for _, allowanceCharge := range applicableHeaderTradeSettlement.nodes("ram:SpecifiedTradeAllowanceCharge") {
		basisAmount, err := getDecimal(allowanceCharge, "ram:BasisAmount")
		if err != nil {
			return err
		}
		actualAmount, err := getDecimal(allowanceCharge, "ram:ActualAmount")
		if err != nil {
			return err
		}
		calculationPercent, err := getDecimal(allowanceCharge, "ram:CalculationPercent")
		if err != nil {
			return err
		}
		categoryTaxRate, err := getDecimal(allowanceCharge, "ram:CategoryTradeTax/ram:RateApplicablePercent")
		if err != nil {
			return err
		}

		allowanceCharge := AllowanceCharge{
			ChargeIndicator:                       allowanceCharge.value("ram:ChargeIndicator/udt:Indicator") == "true",
			BasisAmount:                           basisAmount,
			ActualAmount:                          actualAmount,
			CalculationPercent:                    calculationPercent,
			ReasonCode:                            allowanceCharge.value("ram:ReasonCode"),
			Reason:                                allowanceCharge.value("ram:Reason"),
			CategoryTradeTaxType:                  allowanceCharge.value("ram:CategoryTradeTax/ram:TypeCode"),
			CategoryTradeTaxCategoryCode:          allowanceCharge.value("ram:CategoryTradeTax/ram:CategoryCode"),
			CategoryTradeTaxRateApplicablePercent: categoryTaxRate,
		}
		inv.SpecifiedTradeAllowanceCharge = append(inv.SpecifiedTradeAllowanceCharge, allowanceCharge)
	}

	// Parse SpecifiedLogisticsServiceCharge and convert to document-level charges
	// Per EN 16931, logistics service charges are document-level charges (BT-99)
	for _, logisticsCharge := range applicableHeaderTradeSettlement.nodes("ram:SpecifiedLogisticsServiceCharge") {
		appliedAmount, err := getDecimal(logisticsCharge, "ram:AppliedAmount")
		if err != nil {
			return err
		}
		categoryTaxRate, err := getDecimal(logisticsCharge, "ram:AppliedTradeTax/ram:RateApplicablePercent")
		if err != nil {
			return err
		}

		charge := AllowanceCharge{
			ChargeIndicator:                       true, // Logistics charges are always charges, not allowances
			ActualAmount:                          appliedAmount,
			Reason:                                logisticsCharge.value("ram:Description"),
			CategoryTradeTaxType:                  logisticsCharge.value("ram:AppliedTradeTax/ram:TypeCode"),
			CategoryTradeTaxCategoryCode:          logisticsCharge.value("ram:AppliedTradeTax/ram:CategoryCode"),
			CategoryTradeTaxRateApplicablePercent: categoryTaxRate,
		}
		inv.SpecifiedTradeAllowanceCharge = append(inv.SpecifiedTradeAllowanceCharge, charge)
	}

	// BR-CO-19: Track BG-14 (INVOICING PERIOD) presence to validate later
	inv.hasBillingPeriodInXML = applicableHeaderTradeSettlement.exists("ram:BillingSpecifiedPeriod")
	inv.BillingSpecifiedPeriodStart, err = parseCIITime(applicableHeaderTradeSettlement, "ram:BillingSpecifiedPeriod/ram:StartDateTime/udt:DateTimeString")
	if err != nil {
		return fmt.Errorf("invalid billing period start date: %w", err)
	}
	inv.BillingSpecifiedPeriodEnd, err = parseCIITime(applicableHeaderTradeSettlement, "ram:BillingSpecifiedPeriod/ram:EndDateTime/udt:DateTimeString")
	if err != nil {
		return fmt.Errorf("invalid billing period end date: %w", err)
	}

	// ram:SpecifiedTradePaymentTerms
	for _, paymentTerm := range applicableHeaderTradeSettlement.nodes("ram:SpecifiedTradePaymentTerms") {
		spt := SpecifiedTradePaymentTerms{}
		spt.Description = paymentTerm.value("ram:Description")
		spt.DueDate, err = parseCIITime(paymentTerm, "ram:DueDateDateTime/udt:DateTimeString")
		if err != nil {
			return err
		}

		spt.DirectDebitMandateID = paymentTerm.value("ram:DirectDebitMandateID")
		inv.SpecifiedTradePaymentTerms = append(inv.SpecifiedTradePaymentTerms, spt)
	}

	for _, att := range applicableHeaderTradeSettlement.nodes("ram:ApplicableTradeTax") {
		tradeTax := TradeTax{}
		tradeTax.CalculatedAmount, err = getDecimal(att, "ram:CalculatedAmount")
		if err != nil {
			return err
		}
		tradeTax.BasisAmount, err = getDecimal(att, "ram:BasisAmount")
		if err != nil {
			return err
		}
		tradeTax.TypeCode = att.value("ram:TypeCode")
		tradeTax.ExemptionReason = att.value("ram:ExemptionReason")
		tradeTax.ExemptionReasonCode = att.value("ram:ExemptionReasonCode")
		tradeTax.CategoryCode = att.value("ram:CategoryCode")
		tradeTax.Percent, err = getDecimal(att, "ram:RateApplicablePercent") // BT-119
		if err != nil {
			return err
		}
		inv.TradeTaxes = append(inv.TradeTaxes, tradeTax)
	}

	summation := applicableHeaderTradeSettlement.node("ram:SpecifiedTradeSettlementHeaderMonetarySummation")

	// BR-12 through BR-15: Track XML element presence to validate later
	// This allows validation to distinguish between missing elements and zero values
	inv.hasLineTotalInXML = summation.exists("ram:LineTotalAmount")
	inv.hasTaxBasisTotalInXML = summation.exists("ram:TaxBasisTotalAmount")
	inv.hasGrandTotalInXML = summation.exists("ram:GrandTotalAmount")
	inv.hasDuePayableAmountInXML = summation.exists("ram:DuePayableAmount")

	inv.LineTotal, err = getDecimal(summation, "ram:LineTotalAmount")
	if err != nil {
		return err
	}
	inv.ChargeTotal, err = getDecimal(summation, "ram:ChargeTotalAmount")
	if err != nil {
		return err
	}
	inv.AllowanceTotal, err = getDecimal(summation, "ram:AllowanceTotalAmount")
	if err != nil {
		return err
	}
	inv.TaxBasisTotal, err = getDecimal(summation, "ram:TaxBasisTotalAmount")
	if err != nil {
		return err
	}

	// BT-110 and BT-111: Parse TaxTotalAmount by matching currencyID (not position)
	// EN 16931 specifies which currency each total must be in, regardless of XML order
	for _, taxTotal := range summation.nodes("ram:TaxTotalAmount") {
		currency := taxTotal.attribute("currencyID")
		amount, err := getDecimal(taxTotal, ".")
		if err != nil {
			return fmt.Errorf("invalid TaxTotalAmount with currency %s: %w", currency, err)
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
			// Track unexpected TaxTotalAmount currencies for validation
			inv.unexpectedTaxCurrencies = append(inv.unexpectedTaxCurrencies, currency)
		}
	}

	inv.GrandTotal, err = getDecimal(summation, "ram:GrandTotalAmount")
	if err != nil {
		return err
	}
	inv.TotalPrepaid, err = getDecimal(summation, "ram:TotalPrepaidAmount")
	if err != nil {
		return err
	}
	inv.DuePayableAmount, err = getDecimal(summation, "ram:DuePayableAmount")
	if err != nil {
		return err
	}

	// BG-3
	for _, refdoc := range applicableHeaderTradeSettlement.nodes("ram:InvoiceReferencedDocument") {
		refDoc := ReferencedDocument{}

		refDoc.Date, err = parseCIITime(refdoc, "ram:FormattedIssueDateTime/qdt:DateTimeString")
		if err != nil {
			return err
		}

		refDoc.ID = refdoc.value("ram:IssuerAssignedID")
		inv.InvoiceReferencedDocument = append(inv.InvoiceReferencedDocument, refDoc)
	}

	// BT-19: Buyer accounting reference
	inv.ReceivableSpecifiedTradeAccountingAccount = applicableHeaderTradeSettlement.value("ram:ReceivableSpecifiedTradeAccountingAccount/ram:ID")

	return nil
}

func parseSpecifiedLineTradeAgreement(specifiedLineTradeAgreement *xmlNode, invoiceLine *InvoiceLine) error {
	var err error

	// BT-132: Referenced purchase order line reference
	invoiceLine.BuyerOrderReferencedDocument = specifiedLineTradeAgreement.value("ram:BuyerOrderReferencedDocument/ram:LineID")

	// BR-26: Track XML element presence to validate later
	invoiceLine.hasNetPriceInXML = specifiedLineTradeAgreement.exists("ram:NetPriceProductTradePrice/ram:ChargeAmount")
	invoiceLine.NetPrice, err = getDecimal(specifiedLineTradeAgreement, "ram:NetPriceProductTradePrice/ram:ChargeAmount")
	if err != nil {
		return err
	}
	// BT-149: Item price base quantity with unit code (from NetPrice)
	invoiceLine.BasisQuantity, err = getDecimal(specifiedLineTradeAgreement, "ram:NetPriceProductTradePrice/ram:BasisQuantity")
	if err != nil {
		return err
	}
	invoiceLine.BasisQuantityUnit = specifiedLineTradeAgreement.node("ram:NetPriceProductTradePrice/ram:BasisQuantity").attribute("unitCode")

	invoiceLine.GrossPrice, err = getDecimal(specifiedLineTradeAgreement, "ram:GrossPriceProductTradePrice/ram:ChargeAmount")
	if err != nil {
		return err
	}
	// ZUGFeRD extended has unbound BT-147
	for _, allowanceCharge := range specifiedLineTradeAgreement.nodes("ram:GrossPriceProductTradePrice/ram:AppliedTradeAllowanceCharge") {
		basisAmount, err := getDecimal(allowanceCharge, "ram:BasisAmount")
		if err != nil {
			return err
		}
		actualAmount, err := getDecimal(allowanceCharge, "ram:ActualAmount")
		if err != nil {
			return err
		}
		calculationPercent, err := getDecimal(allowanceCharge, "ram:CalculationPercent")
		if err != nil {
			return err
		}
		categoryTaxRate, err := getDecimal(allowanceCharge, "ram:CategoryTradeTax/ram:RateApplicablePercent")
		if err != nil {
			return err
		}

		allowanceCharge := AllowanceCharge{
			ChargeIndicator:                       allowanceCharge.value("ram:ChargeIndicator/udt:Indicator") == "true",
			BasisAmount:                           basisAmount,
			ActualAmount:                          actualAmount,
			CalculationPercent:                    calculationPercent,
			ReasonCode:                            allowanceCharge.value("ram:ReasonCode"),
			Reason:                                allowanceCharge.value("ram:Reason"),
			CategoryTradeTaxType:                  allowanceCharge.value("ram:CategoryTradeTax/ram:TypeCode"),
			CategoryTradeTaxCategoryCode:          allowanceCharge.value("ram:CategoryTradeTax/ram:CategoryCode"),
			CategoryTradeTaxRateApplicablePercent: categoryTaxRate,
		}
		invoiceLine.AppliedTradeAllowanceCharge = append(invoiceLine.AppliedTradeAllowanceCharge, allowanceCharge)
	}
	return nil
}

func parseSpecifiedTradeProduct(specifiedTradeProduct *xmlNode, invoiceLine *InvoiceLine) {
	invoiceLine.GlobalID = specifiedTradeProduct.value("ram:GlobalID")
	invoiceLine.GlobalIDType = specifiedTradeProduct.node("ram:GlobalID").attribute("schemeID")
	invoiceLine.ArticleNumber = specifiedTradeProduct.value("ram:SellerAssignedID")
	invoiceLine.ArticleNumberBuyer = specifiedTradeProduct.value("ram:BuyerAssignedID")
	invoiceLine.ItemName = specifiedTradeProduct.value("ram:Name")
	invoiceLine.Description = specifiedTradeProduct.value("ram:Description")

	for _, itm := range specifiedTradeProduct.nodes("ram:ApplicableProductCharacteristic") {
		ch := Characteristic{
			Description: itm.value("ram:Description"),
			Value:       itm.value("ram:Value"),
		}
		invoiceLine.Characteristics = append(invoiceLine.Characteristics, ch)
	}
	for _, itm := range specifiedTradeProduct.nodes("ram:DesignatedProductClassification") {
		ch := Classification{
			ClassCode:     itm.value("ram:ClassCode"),
			ListID:        itm.node("ram:ClassCode").attribute("listID"),
			ListVersionID: itm.node("ram:ClassCode").attribute("listVersionID"),
		}
		invoiceLine.ProductClassification = append(invoiceLine.ProductClassification, ch)
	}

	invoiceLine.OriginTradeCountry = specifiedTradeProduct.value("ram:OriginTradeCountry/ram:ID")
}
