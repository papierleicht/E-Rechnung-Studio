package validation

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"papierleicht.de/e-rechnung-studio/internal/model"
)

// InvoiceIssues checks what only an invoice proper needs (quotes do not): the
// Pflichtangaben of § 14 Abs. 4 Satz 1 Nr. 1 and 6 UStG that the e-invoice
// standard leaves optional. The business rules of EN 16931 and XRechnung require
// only a country code in the addresses and no date of supply, so a document can
// pass them and still lack these. The BMF letter of 15.10.2026 (Rn. 8, 23)
// requires the Pflichtangaben to be complete and correct in the structured part.
func InvoiceIssues(d model.InvoiceDraft, c model.Company) []model.ValidationIssue {
	c = model.NormalizeCompany(c)
	d.Customer = model.NormalizeCustomer(d.Customer)
	var issues []model.ValidationIssue
	add := func(field, message string) {
		issues = append(issues, model.ValidationIssue{Stage: "input", RuleID: "INPUT", Severity: "error", Field: field, Message: message})
	}
	// The date of supply (BT-72) is always required, even next to a period of
	// supply: the CII schema needs its delivery group, and an empty one violates
	// PEPPOL-EN16931-R008, which the check reports as a warning that blocks issuing.
	if strings.TrimSpace(d.DeliveryDate) == "" {
		add("deliveryDate", "Das Leistungsdatum fehlt noch. Gib den Tag der Leistung an, bei mehreren Tagen den letzten. Einen Leistungszeitraum kannst du zusätzlich angeben. Der Zeitpunkt der Leistung ist eine Pflichtangabe auf Rechnungen.")
	}
	address := func(prefix, owner string, a model.Address) {
		if strings.TrimSpace(a.Street) == "" {
			add(prefix+".street", "Die Straße "+owner+" fehlt noch. Die vollständige Anschrift ist eine Pflichtangabe auf Rechnungen.")
		}
		if strings.TrimSpace(a.City) == "" {
			add(prefix+".city", "Der Ort "+owner+" fehlt noch. Die vollständige Anschrift ist eine Pflichtangabe auf Rechnungen.")
		}
		// Not every country has postcodes; Germany does.
		if a.Country == "DE" && strings.TrimSpace(a.Postcode) == "" {
			add(prefix+".postcode", "Die Postleitzahl "+owner+" fehlt noch. Die vollständige Anschrift ist eine Pflichtangabe auf Rechnungen.")
		}
	}
	address("customer.address", "des Rechnungsempfängers", d.Customer.Address)
	address("company.address", "deines Unternehmens", c.Address)
	return issues
}

// genericWords name a supply without saying what it consists of.
var genericWords = map[string]bool{
	"arbeit": true, "arbeiten": true, "arbeitszeit": true, "aufwand": true, "auftrag": true,
	"beratung": true, "dienstleistung": true, "dienstleistungen": true, "diverses": true,
	"honorar": true, "leistung": true, "leistungen": true, "lieferung": true, "material": true,
	"pauschale": true, "projekt": true, "projektarbeit": true, "service": true,
	"sonstiges": true, "stunden": true, "stunde": true, "std": true, "tag": true, "tage": true,
	"support": true,
}

// connectors carry no information about the supply.
var connectors = map[string]bool{
	"und": true, "oder": true, "sowie": true, "für": true, "zu": true, "zur": true, "zum": true,
	"von": true, "vom": true, "der": true, "die": true, "das": true, "des": true, "im": true,
	"in": true, "am": true, "an": true, "mit": true, "nach": true, "laut": true,
}

// briefSupply reports a line whose name is a single word or only generic words
// and which has no description.
func briefSupply(line model.InvoiceLine) bool {
	if strings.TrimSpace(line.Description) != "" {
		return false
	}
	var words []string
	for _, word := range strings.FieldsFunc(strings.ToLower(line.Name), func(r rune) bool { return !unicode.IsLetter(r) }) {
		if !connectors[word] {
			words = append(words, word)
		}
	}
	if len(words) <= 1 {
		return true
	}
	for _, word := range words {
		if !genericWords[word] {
			return false
		}
	}
	return true
}

// AdviceIssues returns hints that never block issuing: they carry the severity
// "information", so the state of the check stays valid. The BMF letter of
// 15.10.2026 (Rn. 10 to 12) requires the structured part of an e-invoice to name
// type, extent and content of the supply so exactly that the kind of supply, the
// place and the tax rate follow from it; it applies to e-invoices issued from
// 01.01.2027. The app cannot judge the wording, so it only points out lines that
// name the supply in a single word or only in generic words.
func AdviceIssues(d model.InvoiceDraft) []model.ValidationIssue {
	if d.Kind != "invoice" {
		return nil
	}
	var issues []model.ValidationIssue
	for i, line := range d.Lines {
		name := strings.TrimSpace(line.Name)
		if name == "" || !briefSupply(line) {
			continue
		}
		if utf8.RuneCountInString(name) > 40 {
			name = string([]rune(name)[:40]) + "…"
		}
		issues = append(issues, model.ValidationIssue{
			Stage: "advice", RuleID: "APP-DESCRIPTION", Severity: "information", Field: fmt.Sprintf("lines.%d.description", i),
			Message: fmt.Sprintf("Position %d („%s“): Die Leistung ist knapp beschrieben. Nenne Art, Umfang und Inhalt, zum Beispiel „Beratung zur Umsatzsteuer, 2 Std.“. Für E-Rechnungen, die ab dem 01.01.2027 ausgestellt werden, muss der strukturierte Teil die Leistung so genau benennen, dass Leistungsart, Ort und Steuersatz erkennbar sind.", i+1, name),
		})
	}
	return issues
}
