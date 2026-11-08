package validation

import (
	"strings"
	"testing"

	"papierleicht.de/e-rechnung-studio/internal/model"
)

func TestInputIssuesCollectMissingFields(t *testing.T) {
	d := model.NewDraft()
	d.Lines = append(d.Lines, model.InvoiceLine{Name: "Beratung", Quantity: "-2", UnitPrice: "abc", TaxRate: "19"})
	issues := InputIssues(d, model.Company{})
	fields := map[string]string{}
	for _, issue := range issues {
		fields[issue.Field] = issue.Message
	}
	for _, field := range []string{"company.name", "company.address.country", "customer.name", "customer.address.country", "lines.0.name", "lines.0.unitPrice", "lines.1.quantity", "lines.1.unitPrice"} {
		if fields[field] == "" {
			t.Errorf("missing guidance for %s", field)
		}
	}
	if !strings.Contains(fields["lines.1.quantity"], "Position 2") {
		t.Fatal("position number missing")
	}
}

func TestInputIssuesAcceptCommaAndZeroPrice(t *testing.T) {
	d := model.NewDraft()
	d.Customer.Name, d.Customer.Address.Country = "Kunde", "DE"
	d.Lines = []model.InvoiceLine{{Name: "Leistung", Quantity: "1,5", UnitPrice: "0", TaxRate: "19"}}
	c := model.Company{Name: "Firma", Address: model.Address{Country: "DE"}}
	if issues := InputIssues(d, c); len(issues) > 0 {
		t.Fatalf("valid input rejected: %+v", issues)
	}
	d.TaxTreatment = "small-business"
	d.Lines[0].TaxRate = ""
	if issues := InputIssues(d, c); len(issues) > 0 {
		t.Fatalf("effective zero rate rejected: %+v", issues)
	}
}

func TestGermanPreservesDiagnosticsAndSeverity(t *testing.T) {
	for _, tc := range []struct{ id, stage, original, location, field, contains string }{
		{"BR-07", "rules", "Buyer name is required", "/BuyerTradeParty", "customer.name", "Rechnungsempfängers"},
		{"XSD", "xsd", "/SellerTradeParty/PostalTradeAddress: expected CountryID", "", "company.address.country", "Land deines Unternehmens"},
		{"BR-25", "rules", "Item name is required", "/*:IncludedSupplyChainTradeLineItem[2]", "lines.1.name", "Position 2"},
		{"BR-25", "rules", "Item name is required", "/*:IncludedSupplyChainTradeLineItem[namespace-uri()='urn:un:unece:uncefact:data:standard:ReusableAggregateBusinessInformationEntity:100'][3]", "lines.2.name", "Position 3"},
		{"PEPPOL-EN16931-R008", "rules", "Document MUST not contain empty elements.", "/*:CrossIndustryInvoice[1]/*:SupplyChainTradeTransaction[1]/*:ApplicableHeaderTradeAgreement[1]/*:SellerTradeParty[1]/*:URIUniversalCommunication[1]/*:URIID[1]", "company.endpoint", "Absenderadresse ist leer"},
		{"PEPPOL-EN16931-R008", "rules", "Document MUST not contain empty elements.", "/*:CrossIndustryInvoice[1]/*:SupplyChainTradeTransaction[1]/*:ApplicableHeaderTradeAgreement[1]/*:BuyerTradeParty[1]/*:URIUniversalCommunication[1]/*:URIID[1]", "customer.endpoint", "Empfängers ist leer"},
		{"PEPPOL-EN16931-R008", "rules", "Document MUST not contain empty elements.", "/*:CrossIndustryInvoice[1]/*:ExchangedDocument[1]/*:IncludedNote[1]", "notes", "leere Felder"},
		{"BR-UNKNOWN", "rules", "A future assertion", "/*:CrossIndustryInvoice[1]/*:SupplyChainTradeTransaction[1]/*:ApplicableHeaderTradeAgreement[1]/*:SellerTradeParty[1]/*:PostalTradeAddress[1]/*:PostcodeCode[1]", "company.address.postcode", "keine deutsche Erläuterung"},
		{"BR-UNKNOWN", "rules", "A future assertion", "/*:Invoice[1]/*:InvoiceLine[4]/*:Item[1]", "lines.3", "keine deutsche Erläuterung"},
		{"BR-UNKNOWN", "rules", "A future assertion", "/*:CrossIndustryInvoice[1]/*:SupplyChainTradeTransaction[1]/*:ApplicableHeaderTradeSettlement[1]/*:SpecifiedTradePaymentTerms[1]", "paymentTerms", "keine deutsche Erläuterung"},
		{"BR-UNKNOWN", "rules", "A future assertion", "/*:Invoice[1]/*:AccountingCustomerParty[1]/*:Party[1]/*:EndpointID[1]", "customer.endpoint", "keine deutsche Erläuterung"},
		{"BR-UNKNOWN", "rules", "A future assertion", "/future", "", "keine deutsche Erläuterung"},
		{"DOCTYPE", "syntax", "DOCTYPE is forbidden", "", "", "nicht erlaubte"},
	} {
		t.Run(tc.id, func(t *testing.T) {
			r := German(model.ValidationResult{State: "invalid", Issues: []model.ValidationIssue{{RuleID: tc.id, Stage: tc.stage, Severity: "fatal", Message: tc.original, Location: tc.location}}})
			i := r.Issues[0]
			if r.State != "invalid" || i.Severity != "fatal" || i.RuleID != tc.id || i.Details != tc.original || i.Location != tc.location || i.Field != tc.field || !strings.Contains(i.Message, tc.contains) {
				t.Fatalf("translation lost meaning or metadata: %+v", r)
			}
			if twice := German(r); twice.Issues[0].Details != tc.original {
				t.Fatal("original overwritten on second translation")
			}
		})
	}
}

func TestGermanUBLPositionAndVATLocations(t *testing.T) {
	for _, tc := range []struct{ rule, location, field string }{
		{"BR-25", "/*:Invoice[1]/*:InvoiceLine[2]/*:Item[1]", "lines.1.name"},
		{"BR-25", "/*:CreditNote[1]/*:CreditNoteLine[3]/*:Item[1]", "lines.2.name"},
		{"BR-CO-09", "/*:Invoice[1]/*:AccountingCustomerParty[1]/*:Party[1]/*:PartyTaxScheme[1]", "customer.vatId"},
		{"BR-CO-09", "/*:CrossIndustryInvoice[1]/*:BuyerTradeParty[1]/*:SpecifiedTaxRegistration[1]", "customer.vatId"},
	} {
		t.Run(tc.location, func(t *testing.T) {
			result := German(model.ValidationResult{Issues: []model.ValidationIssue{{RuleID: tc.rule, Stage: "rules", Message: "original", Location: tc.location}}})
			if result.Issues[0].Field != tc.field {
				t.Fatalf("got %s, want %s", result.Issues[0].Field, tc.field)
			}
		})
	}
}
