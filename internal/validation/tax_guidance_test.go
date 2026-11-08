package validation

import (
	"strings"
	"testing"

	"papierleicht.de/e-rechnung-studio/internal/einvoice"
	"papierleicht.de/e-rechnung-studio/internal/model"
)

func TestIntraCommunityIdentifierGuidance(t *testing.T) {
	for _, tc := range []struct{ name, seller, buyer, representative, field, message string }{
		{"customer entered", "", "FR123", "", "company.vatId", "Es fehlt noch deine eigene"},
		{"seller entered", "DE123", "", "", "customer.vatId", "fehlt noch die Umsatzsteuer-ID deines Kunden"},
		{"both missing", "", "", "", "company.vatId", "fehlen deine eigene"},
		{"representative entered", "", "", "DE123", "customer.vatId", "fehlt noch die Umsatzsteuer-ID deines Kunden"},
		{"both present preserve unexpected failure", "DE123", "FR123", "", "customer.vatId", "Für eine innergemeinschaftliche Lieferung"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			inv := &einvoice.Invoice{Seller: einvoice.Party{VATaxRegistration: tc.seller}, Buyer: einvoice.Party{VATaxRegistration: tc.buyer}}
			if tc.representative != "" {
				inv.SellerTaxRepresentativeTradeParty = &einvoice.Party{VATaxRegistration: tc.representative}
			}
			r := German(model.ValidationResult{State: "invalid", Issues: []model.ValidationIssue{{RuleID: "BR-IC-02", Severity: "fatal", Stage: "rules", Message: "original", Location: "/invoice"}}})
			r = explainIntraCommunityIdentifiers(r, inv)
			i := r.Issues[0]
			if i.Field != tc.field || !strings.Contains(i.Message, tc.message) || i.Details != "original" || i.Location != "/invoice" || i.Severity != "fatal" || r.State != "invalid" {
				t.Fatalf("unexpected guidance: %+v", r)
			}
		})
	}
}

func TestReverseChargeIdentifierAlternatives(t *testing.T) {
	for _, rule := range []string{"BR-AE-02", "BR-AE-03", "BR-AE-04"} {
		for _, tc := range []struct{ name, vat, tax, buyer, legal, rep, field string }{
			{"seller missing", "", "", "FR123", "", "", "company.taxNumber"},
			{"both missing", "", "", "", "", "", "company.taxNumber"},
			{"tax number sufficient", "", "123", "", "", "", "customer.vatId"},
			{"representative sufficient", "", "", "", "", "DE123", "customer.vatId"},
			{"buyer register sufficient", "", "", "", "REGISTER", "", "company.taxNumber"},
		} {
			t.Run(rule+"/"+tc.name, func(t *testing.T) {
				inv := &einvoice.Invoice{Seller: einvoice.Party{VATaxRegistration: tc.vat, FCTaxRegistration: tc.tax}, Buyer: einvoice.Party{VATaxRegistration: tc.buyer}}
				if tc.legal != "" {
					inv.Buyer.SpecifiedLegalOrganization = &einvoice.SpecifiedLegalOrganization{ID: tc.legal}
				}
				if tc.rep != "" {
					inv.SellerTaxRepresentativeTradeParty = &einvoice.Party{VATaxRegistration: tc.rep}
				}
				r := German(model.ValidationResult{Issues: []model.ValidationIssue{{RuleID: rule, Message: "original", Stage: "rules"}}})
				r = explainIntraCommunityIdentifiers(r, inv)
				if r.Issues[0].Field != tc.field || r.Issues[0].Details != "original" {
					t.Fatalf("wrong action: %+v", r)
				}
			})
		}
	}
}
