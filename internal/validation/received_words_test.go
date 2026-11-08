package validation

import (
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/shopspring/decimal"
	"papierleicht.de/e-rechnung-studio/internal/einvoice"
	"papierleicht.de/e-rechnung-studio/internal/model"
)

// issuerVoice finds what makes sense only for the issuer of an invoice: second
// person, imperatives and the menus of this app.
var issuerVoice = regexp.MustCompile(`(?i)\b(dein\w*|dir|dich|du)\b|\b(Ergänze|Prüfe|Trage|Wähle|Füge|Gib|Beschreibe|Korrigiere|Verwende|Lege)\b|erweiterten Menü|Unternehmensdaten|Empfängerdaten|Adressfeld|beim Ausstellen`)

// issuerOnly is what a text for the recipient must never contain. Telling the
// recipient to check something before paying is fine.
var issuerOnly = regexp.MustCompile(`(?i)\bdein\w*|\b(Ergänze|Trage|Wähle|Füge|Gib|Beschreibe|Korrigiere|Verwende|Lege)\b|erweiterten Menü|Unternehmensdaten|Empfängerdaten|Adressfeld|beim Ausstellen`)

func TestEveryIssuerVoicedGermanTextHasAWordingForTheRecipient(t *testing.T) {
	for id, g := range germanRules {
		if !issuerVoice.MatchString(g.message) {
			continue
		}
		issue := model.ValidationIssue{Stage: "rules", RuleID: id, Message: g.message, Field: g.field}
		text := receivedText(issue, nil)
		if text == "" {
			t.Errorf("%s: %q needs a wording for the recipient in receivedWording", id, g.message)
		} else if issuerOnly.MatchString(text) {
			t.Errorf("%s: %q still speaks to the issuer", id, text)
		}
	}
	for id, text := range receivedWording {
		if issuerOnly.MatchString(text) {
			t.Errorf("%s: %q speaks to the issuer", id, text)
		}
	}
}

// The texts German adds beyond the table, and the findings of the app's own
// checks, go through the real functions so that a changed text is noticed.
func TestEveryOtherIssuerVoicedFindingIsWordedForTheRecipient(t *testing.T) {
	const country = "ram:%s/ram:PostalTradeAddress/ram:CountryID: missing"
	german := German(model.ValidationResult{State: "invalid", Issues: []model.ValidationIssue{
		{Stage: "syntax", RuleID: "DOCTYPE", Severity: "error", Message: "DOCTYPE declarations are not allowed"},
		{Stage: "xsd", RuleID: "XSD", Severity: "error", Message: strings.Replace(country, "%s", "SellerTradeParty", 1)},
		{Stage: "xsd", RuleID: "XSD", Severity: "error", Message: strings.Replace(country, "%s", "BuyerTradeParty", 1)},
		{Stage: "rules", RuleID: "XSLT", Severity: "error", Message: "engine failure"},
		{Stage: "rules", RuleID: "BR-CO-09", Severity: "error", Message: "x", Location: "/ram:SellerTradeParty/ram:SpecifiedTaxRegistration"},
		{Stage: "rules", RuleID: "BR-CO-09", Severity: "error", Message: "x", Location: "/ram:BuyerTradeParty/ram:SpecifiedTaxRegistration"},
		{Stage: "rules", RuleID: "BR-CO-09", Severity: "error", Message: "x", Location: "/ram:SellerTaxRepresentativeTradeParty/ram:SpecifiedTaxRegistration"},
		{Stage: "rules", RuleID: "PEPPOL-EN16931-R008", Severity: "error", Message: "x", Location: "/ram:SellerTradeParty/ram:URIUniversalCommunication"},
		{Stage: "rules", RuleID: "PEPPOL-EN16931-R008", Severity: "error", Message: "x", Location: "/ram:BuyerTradeParty/ram:URIUniversalCommunication"},
	}})
	// What a supplier's document with these findings would show: the checks of
	// the app on its parties and payment data.
	invoice := &einvoice.Invoice{
		Seller:       einvoice.Party{VATaxRegistration: "12345", FCTaxRegistration: "1", PostalAddress: &einvoice.PostalAddress{CountryID: "DE"}},
		Buyer:        einvoice.Party{VATaxRegistration: "XX"},
		PaymentMeans: []einvoice.PaymentMeans{{PayeePartyCreditorFinancialAccountIBAN: "DE00", PayeeSpecifiedCreditorFinancialInstitutionBIC: "x"}},
	}
	invoice.DuePayableAmount = decimal.NewFromInt(100)
	german.Issues = append(german.Issues, SupplementalIssues(invoice)...)

	rules := map[string]bool{}
	for _, issue := range german.Issues {
		rules[issue.RuleID] = true
	}
	for _, rule := range []string{"APP-VAT-FORMAT", "APP-TAX-FORMAT", "APP-IBAN", "APP-BIC", "APP-PAYMENT-TERMS"} {
		if !rules[rule] {
			t.Fatalf("the test document no longer provokes %s: %+v", rule, german.Issues)
		}
	}
	got := GermanReceived(german, invoice)
	for i, issue := range got.Issues {
		before := german.Issues[i].Message
		if issuerVoice.MatchString(before) && issue.Message == before {
			t.Errorf("%s was not reworded: %q", issue.RuleID, before)
		}
		if issuerOnly.MatchString(issue.Message) {
			t.Errorf("%s still speaks to the issuer: %q", issue.RuleID, issue.Message)
		}
	}
}

func TestGermanReceivedKeepsEverythingButTheMessage(t *testing.T) {
	in := model.ValidationResult{State: "invalid", Issues: []model.ValidationIssue{
		{Stage: "rules", RuleID: "BR-06", Severity: "error", Field: "company.name", Message: "Dein Unternehmensname fehlt. Ergänze ihn in den Unternehmensdaten.", Details: "[BR-06] original", Location: "/rsm:Invoice"},
		{Stage: "rules", RuleID: "BR-CO-15", Severity: "warning", Field: "lines", Message: "Der Bruttobetrag stimmt nicht mit Nettobetrag plus Umsatzsteuer überein."},
		{Stage: "rules", RuleID: "BR-22", Severity: "error", Field: "lines.1.quantity", Message: "Position 2: Die Menge der Position fehlt. Trage eine Menge ein."},
	}}
	out := GermanReceived(in, nil)
	if out.Issues[0].Message != "Der Name des Lieferanten fehlt." {
		t.Fatalf("got %q", out.Issues[0].Message)
	}
	if out.Issues[1] != in.Issues[1] {
		t.Errorf("a text without issuer voice stays as it is: %+v", out.Issues[1])
	}
	if out.Issues[2].Message != "Position 2: Die Menge der Position fehlt." {
		t.Errorf("the line stays named: %q", out.Issues[2].Message)
	}
	rest := out.Issues[0]
	rest.Message = in.Issues[0].Message
	if rest != in.Issues[0] {
		t.Errorf("rule, severity, field, details and location must stay: %+v", out.Issues[0])
	}
	if in.Issues[0].Message != "Dein Unternehmensname fehlt. Ergänze ihn in den Unternehmensdaten." {
		t.Error("the argument was changed")
	}
	if again := GermanReceived(out, nil); !slices.Equal(again.Issues, out.Issues) {
		t.Errorf("wording twice changed the result: %+v", again.Issues)
	}
	if got := GermanReceived(model.ValidationResult{State: "valid"}, nil); got.Issues != nil || got.State != "valid" {
		t.Errorf("a result without findings stays as it is: %+v", got)
	}
}

func TestGermanReceivedNamesThePartyOfTheFinding(t *testing.T) {
	word := func(issue model.ValidationIssue, inv *einvoice.Invoice) string {
		return GermanReceived(model.ValidationResult{Issues: []model.ValidationIssue{issue}}, inv).Issues[0].Message
	}
	for location, want := range map[string]string{
		"/ram:SellerTradeParty/ram:SpecifiedTaxRegistration":                  "Die USt-IdNr. des Lieferanten braucht",
		"/ram:BuyerTradeParty/ram:SpecifiedTaxRegistration":                   "Die USt-IdNr. des Empfängers braucht",
		"/ram:SellerTaxRepresentativeTradeParty/ram:SpecifiedTaxRegistration": "Die USt-IdNr. des Steuervertreters braucht",
	} {
		if got := word(model.ValidationIssue{Stage: "rules", RuleID: "BR-CO-09", Message: "x", Location: location}, nil); !strings.HasPrefix(got, want) {
			t.Errorf("%s: got %q", location, got)
		}
	}
	for field, want := range map[string]string{
		"company.vatId":  "Die USt-IdNr. des Lieferanten hat kein gültiges Format.",
		"customer.vatId": "Die USt-IdNr. des Empfängers hat kein gültiges Format.",
	} {
		if got := word(model.ValidationIssue{Stage: "input", RuleID: "APP-VAT-FORMAT", Field: field, Message: "Prüfe die USt-IdNr."}, nil); !strings.HasPrefix(got, want) {
			t.Errorf("%s: got %q", field, got)
		}
	}
	if got := word(model.ValidationIssue{Stage: "xsd", RuleID: "XSD", Field: "customer.address.country", Message: "Das Land des Rechnungsempfängers fehlt oder ist ungültig. Verwende einen zweistelligen Ländercode, z. B. DE."}, nil); got != "Das Land des Empfängers fehlt oder ist kein zweistelliger Ländercode." {
		t.Errorf("got %q", got)
	}
	// Any other schema finding keeps the general text.
	generic := "Der Aufbau oder ein Wert der XML-Datei entspricht nicht dem gewählten Rechnungsformat."
	if got := word(model.ValidationIssue{Stage: "xsd", RuleID: "XSD", Field: "company.address.country", Message: generic}, nil); got != generic {
		t.Errorf("got %q", got)
	}
}

func TestGermanReceivedNamesTheMissingIdentifier(t *testing.T) {
	word := func(rule string, inv *einvoice.Invoice) string {
		return GermanReceived(model.ValidationResult{Issues: []model.ValidationIssue{{Stage: "rules", RuleID: rule, Message: "x"}}}, inv).Issues[0].Message
	}
	both := &einvoice.Invoice{}
	if got := word("BR-IC-02", both); !strings.Contains(got, "weder die USt-IdNr. des Lieferanten noch die des Empfängers") {
		t.Errorf("got %q", got)
	}
	onlyBuyer := &einvoice.Invoice{Seller: einvoice.Party{VATaxRegistration: "DE123456789"}}
	if got := word("BR-IC-03", onlyBuyer); !strings.Contains(got, "nicht die USt-IdNr. des Empfängers") {
		t.Errorf("got %q", got)
	}
	if got := word("BR-AE-02", both); !strings.Contains(got, "auf beiden Seiten") {
		t.Errorf("got %q", got)
	}
	taxNumberOnly := &einvoice.Invoice{Seller: einvoice.Party{FCTaxRegistration: "12/345/67890"}}
	taxNumberOnly.Seller.VATaxRegistration = ""
	if got := word("BR-AE-04", taxNumberOnly); !strings.Contains(got, "fehlt die USt-IdNr. oder zulässige Registerkennung des Empfängers") {
		t.Errorf("got %q", got)
	}
	// Without the parsed document, or with every identifier present, the general wording stays.
	complete := &einvoice.Invoice{Seller: einvoice.Party{VATaxRegistration: "DE123456789"}, Buyer: einvoice.Party{VATaxRegistration: "FR12345678901"}}
	for _, inv := range []*einvoice.Invoice{nil, complete} {
		if got := word("BR-IC-02", inv); got != receivedWording["BR-IC-02"] {
			t.Errorf("got %q", got)
		}
	}
}
