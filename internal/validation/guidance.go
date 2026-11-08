package validation

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/shopspring/decimal"
	"papierleicht.de/e-rechnung-studio/internal/model"
	"papierleicht.de/e-rechnung-studio/internal/profile"
)

// InputIssues collects actionable input errors before an XML schema can stop at
// the first malformed value. It supplements, never replaces, official validation.
func InputIssues(d model.InvoiceDraft, c model.Company) []model.ValidationIssue {
	c = model.NormalizeCompany(c)
	issues := append(CompanyIssues(c), CustomerIssues(d.Customer)...)
	if c.TaxStatusUnclear {
		issues = append(issues, localIssue("company.taxStatusUnclear", "APP-TAX-STATUS", "Kläre unter Unternehmen → Umsatzsteuer, ob du die Kleinunternehmerregelung nutzt. Bis dahin kannst du Entwürfe speichern, aber keine Rechnungen ausstellen."))
	}
	add := func(field, message string) {
		issues = append(issues, model.ValidationIssue{Stage: "input", RuleID: "INPUT", Severity: "error", Field: field, Message: message})
	}
	required := func(field, value, label string) {
		if strings.TrimSpace(value) == "" {
			add(field, label+" fehlt noch. Ergänze die Angabe.")
		}
	}
	required("company.name", c.Name, "Dein Unternehmensname")
	required("company.address.country", c.Address.Country, "Das Land deines Unternehmens (z. B. DE)")
	required("customer.name", d.Customer.Name, "Der Name des Rechnungsempfängers")
	required("customer.address.country", d.Customer.Address.Country, "Das Land des Rechnungsempfängers (z. B. DE)")
	if f, err := profile.Get(d.Profile); err != nil {
		add("profile", err.Error())
	} else if d.Syntax != f.Syntax {
		add("profile", "Die XML-Syntax passt nicht zum gewählten Format. Wähle das E-Rechnungsformat noch einmal aus.")
	} else if f.XRechnung {
		if model.HasTaxCategory(d, "K") {
			a := model.DeliveryLocation(d)
			required("deliveryCity", a.City, "Der tatsächliche Lieferort für XRechnung")
			required("deliveryPostcode", a.Postcode, "Die Postleitzahl der Lieferadresse für XRechnung")
		}
		required("buyerReference", d.BuyerReference, "Die Käuferreferenz / Leitweg-ID für XRechnung")
		required("company.contactName", c.ContactName, "Der Ansprechpartner für XRechnung")
		required("company.phone", c.Phone, "Die Telefonnummer für Rückfragen")
		required("company.email", c.Email, "Die Kontakt-E-Mail deines Unternehmens")
		if model.CreditTransfer(model.PaymentCode(d, c)) && len(c.BankAccounts()) == 0 {
			required("company.iban", c.IBAN, "Die IBAN für die Überweisung")
		}
		required("company.endpoint", c.Endpoint, "Die elektronische Adresse deines Unternehmens")
		required("company.endpointScheme", c.EndpointScheme, "Das Adressschema deines Unternehmens")
		required("customer.endpoint", d.Customer.Endpoint, "Die elektronische Adresse des Empfängers")
		required("customer.endpointScheme", d.Customer.EndpointScheme, "Das Adressschema des Empfängers")
	}
	for _, date := range []struct {
		field, value, label string
		required            bool
	}{
		{"issueDate", d.IssueDate, "Rechnungsdatum", true},
		{"deliveryDate", d.DeliveryDate, "Leistungsdatum", false},
		{"dueDate", d.DueDate, "Fälligkeitsdatum", false},
		{"periodStart", d.PeriodStart, "Startdatum des Leistungszeitraums", false},
		{"periodEnd", d.PeriodEnd, "Enddatum des Leistungszeitraums", false},
	} {
		if date.value == "" && !date.required {
			continue
		}
		if _, err := time.Parse("2006-01-02", date.value); err != nil {
			add(date.field, "Wähle ein gültiges "+date.label+".")
		}
	}
	if d.PeriodStart != "" && d.PeriodEnd != "" && d.PeriodEnd < d.PeriodStart {
		add("periodEnd", "Das Ende des Leistungszeitraums darf nicht vor dessen Beginn liegen.")
	}
	if !model.SupportedTaxTreatment(d.TaxTreatment) {
		add("taxTreatment", "Wähle einen unterstützten Steuerfall.")
	}
	if len(d.Lines) == 0 {
		add("lines", "Die Rechnung enthält noch keine Position. Füge eine Leistung oder einen Artikel hinzu.")
	}
	for i, line := range d.Lines {
		prefix := fmt.Sprintf("lines.%d.", i)
		label := fmt.Sprintf("Position %d: ", i+1)
		required(prefix+"name", line.Name, label+"die Bezeichnung")
		for _, amount := range []struct {
			field, value, label string
			positive            bool
		}{
			{"quantity", line.Quantity, "Menge", true}, {"unitPrice", line.UnitPrice, "Einzelpreis", false}, {"taxRate", line.TaxRate, "Umsatzsteuersatz", false},
		} {
			tax, _ := model.LineTax(d, line)
			if amount.field == "taxRate" && tax.Category != "S" {
				continue
			}
			value := strings.TrimSpace(amount.value)
			if value == "" {
				add(prefix+amount.field, label+"Gib einen Wert für „"+amount.label+"“ ein.")
				continue
			}
			n, err := decimal.NewFromString(strings.ReplaceAll(value, ",", "."))
			if err != nil {
				add(prefix+amount.field, label+amount.label+" muss eine Zahl sein, z. B. 19,50.")
				continue
			}
			if amount.field == "quantity" && d.Kind == "corrected-invoice" {
				if n.IsZero() {
					add(prefix+amount.field, label+"Die Menge darf nicht 0 sein.")
				}
			} else if amount.positive && !n.IsPositive() {
				add(prefix+amount.field, label+"Die Menge muss größer als 0 sein.")
			} else if n.IsNegative() {
				add(prefix+amount.field, label+amount.label+" darf nicht negativ sein.")
			}
		}
	}
	return append(issues, AdvancedIssues(d, c)...)
}

type guidance struct{ message, field string }

// These explanations refer to the bundled EN16931 assertions. Original messages
// are retained as Details for diagnostics, including assertions without a translation.
var germanRules = map[string]guidance{
	"BR-DE-27":            {"Die Telefonnummer für Rückfragen muss mindestens drei Ziffern enthalten.", "company.phone"},
	"BR-DE-28":            {"Prüfe die Schreibweise deiner Kontakt-E-Mail-Adresse.", "company.email"},
	"BR-DE-10":            {"Ergänze den Lieferort im erweiterten Menü.", "deliveryCity"},
	"BR-DE-11":            {"Ergänze die Postleitzahl der Lieferadresse im erweiterten Menü.", "deliveryPostcode"},
	"BR-DE-1":             {"Ergänze deine Bankverbindung für die Zahlungsanweisung der XRechnung.", "company.iban"},
	"BR-DE-2":             {"XRechnung benötigt einen Ansprechpartner mit Telefon und E-Mail.", "company.contactName"},
	"BR-DE-5":             {"Ergänze den Ansprechpartner deines Unternehmens.", "company.contactName"},
	"BR-DE-6":             {"Ergänze deine Telefonnummer für Rückfragen.", "company.phone"},
	"BR-DE-7":             {"Ergänze die Kontakt-E-Mail deines Unternehmens.", "company.email"},
	"BR-DE-15":            {"Ergänze die Käuferreferenz bzw. Leitweg-ID, die dir der Empfänger mitgeteilt hat.", "buyerReference"},
	"PEPPOL-EN16931-R001": {"Der Geschäftsprozess für XRechnung fehlt.", "businessProcess"},
	"PEPPOL-EN16931-R010": {"Ergänze die elektronische Adresse des Empfängers im erweiterten Menü.", "customer.endpoint"},
	"PEPPOL-EN16931-R020": {"Ergänze deine elektronische Absenderadresse in den Unternehmensdaten.", "company.endpoint"},
	"PEPPOL-EN16931-R008": {"Die XML-Datei enthält leere Felder. Ergänze die fehlenden Angaben oder entferne ungenutzte optionale XML-Felder. Die genaue Stelle steht unter „Prüfdetails“.", ""},
	"BR-E-02":             {"Für steuerbefreite Positionen fehlt eine steuerliche Kennung des Verkäufers. Ergänze deine Steuernummer oder USt-IdNr. in den Unternehmensdaten.", "company.taxNumber"},
	"BR-E-05":             {"Bei einer von Umsatzsteuer befreiten Position muss der Steuersatz 0 % betragen. Prüfe den Steuerfall.", "taxTreatment"},
	"BR-IC-12":            {"Für die innergemeinschaftliche Lieferung fehlt das Lieferland. Ergänze das tatsächliche Lieferland bei den Rechnungsdaten.", "deliveryCountry"},
	"BR-IC-02":            {"Für eine innergemeinschaftliche Lieferung werden die USt-IdNr. des Verkäufers (oder seines Steuervertreters) und die USt-IdNr. des Käufers benötigt. Prüfe die Unternehmens- und Empfängerdaten.", "customer.vatId"},
	"BR-12":               {"Die Summe der Nettobeträge aller Positionen fehlt in der XML-Datei.", ""},
	"BR-13":               {"Der Gesamtbetrag ohne Umsatzsteuer fehlt in der XML-Datei.", ""},
	"BR-14":               {"Der Gesamtbetrag einschließlich Umsatzsteuer fehlt in der XML-Datei.", ""},
	"BR-15":               {"Der noch zu zahlende Betrag fehlt in der XML-Datei.", ""},
	"BR-21":               {"Eine Position hat keine Positionsnummer.", "lines"},
	"BR-22":               {"Die Menge der Position fehlt. Trage eine Menge ein.", "@line.quantity"},
	"BR-23":               {"Die Mengeneinheit der Position fehlt, z. B. Stück oder Stunde.", "@line.unit"},
	"BR-24":               {"Der Nettobetrag der Position fehlt in der XML-Datei.", "lines"},
	"BR-25":               {"Die Bezeichnung der Position fehlt. Beschreibe die Leistung oder den Artikel.", "@line.name"},
	"BR-26":               {"Der Netto-Einzelpreis der Position fehlt.", "@line.unitPrice"},
	"BR-27":               {"Der Netto-Einzelpreis darf nicht negativ sein.", "@line.unitPrice"},
	"BR-28":               {"Der Brutto-Einzelpreis darf nicht negativ sein.", ""},
	"BR-29":               {"Das Ende des Abrechnungszeitraums darf nicht vor dessen Beginn liegen.", ""},
	"BR-30":               {"Das Ende des Leistungszeitraums einer Position darf nicht vor dessen Beginn liegen.", ""},
	"BR-45":               {"In der Steueraufschlüsselung fehlt der steuerpflichtige Nettobetrag.", ""},
	"BR-46":               {"In der Steueraufschlüsselung fehlt der Steuerbetrag.", ""},
	"BR-47":               {"In der Steueraufschlüsselung fehlt die Steuerkategorie.", "taxTreatment"},
	"BR-48":               {"In der Steueraufschlüsselung fehlt der Steuersatz.", "taxTreatment"},
	"BR-49":               {"Bei den Zahlungsangaben fehlt die Zahlungsart.", "paymentMeansCode"},
	"BR-50":               {"Für die Überweisung fehlt die Kontokennung. Ergänze deine IBAN in den Unternehmensdaten.", "company.iban"},
	"BR-61":               {"Für die gewählte Überweisung fehlt die Kontokennung. Ergänze deine IBAN in den Unternehmensdaten.", "company.iban"},
	"BR-CO-09":            {"Prüfe die USt-IdNr. von Verkäufer und Käufer. Sie muss mit dem Länderkürzel beginnen, z. B. DE; für Griechenland ist EL zulässig.", "company.vatId"},
	"BR-CO-10":            {"Die Summe der Positionsbeträge stimmt nicht mit der ausgewiesenen Nettosumme überein.", "lines"},
	"BR-CO-11":            {"Die Summe der Rechnungsnachlässe stimmt nicht mit den einzelnen Nachlässen überein.", ""},
	"BR-CO-12":            {"Die Summe der Rechnungszuschläge stimmt nicht mit den einzelnen Zuschlägen überein.", ""},
	"BR-CO-13":            {"Der Gesamtbetrag ohne Steuer stimmt nicht: Er muss der Positionssumme abzüglich Nachlässen und zuzüglich Zuschlägen entsprechen.", "lines"},
	"BR-CO-14":            {"Die gesamte Umsatzsteuer stimmt nicht mit der Summe der Steuerbeträge je Steuerkategorie überein.", "taxTreatment"},
	"BR-CO-15":            {"Der Bruttobetrag stimmt nicht mit Nettobetrag plus Umsatzsteuer überein.", "lines"},
	"BR-CO-16":            {"Der Zahlbetrag stimmt nicht mit Bruttobetrag abzüglich bereits gezahltem Betrag und zuzüglich Rundungsbetrag überein.", ""},
	"BR-CO-17":            {"Ein Steuerbetrag ist falsch berechnet. Er muss dem steuerpflichtigen Nettobetrag mal Steuersatz entsprechen, auf zwei Nachkommastellen gerundet.", "taxTreatment"},
	"BR-CO-18":            {"Die Aufschlüsselung der Umsatzsteuer fehlt in der XML-Datei.", "taxTreatment"},
	"BR-S-05":             {"Bei Regelbesteuerung muss der Steuersatz der Position größer als 0 sein. Prüfe den Steuersatz und den gewählten Steuerfall.", "@line.taxRate"},
	"BR-01":               {"Die Kennung des E-Rechnungsformats fehlt. Prüfe das Format der XML-Datei.", ""},
	"BR-02":               {"Die Rechnungsnummer fehlt. In der App wird sie beim Ausstellen vergeben.", ""},
	"BR-03":               {"Das Rechnungsdatum fehlt. Wähle ein Rechnungsdatum.", "issueDate"},
	"BR-04":               {"Die Belegart fehlt in der XML-Datei.", "kind"},
	"BR-05":               {"Die Rechnungswährung fehlt in der XML-Datei.", ""},
	"BR-06":               {"Dein Unternehmensname fehlt. Ergänze ihn in den Unternehmensdaten.", "company.name"},
	"BR-07":               {"Der Name des Rechnungsempfängers fehlt. Trage ihn im Adressfeld ein.", "customer.name"},
	"BR-08":               {"Die Anschrift deines Unternehmens fehlt.", "company.address.street"},
	"BR-09":               {"Das Land deines Unternehmens fehlt. Gib z. B. DE für Deutschland ein.", "company.address.country"},
	"BR-10":               {"Die Anschrift des Rechnungsempfängers fehlt.", "customer.address.street"},
	"BR-11":               {"	Das Land des Rechnungsempfängers fehlt. Gib z. B. DE für Deutschland ein.", "customer.address.country"},
	"BR-16":               {"Die Rechnung braucht mindestens eine Position. Füge eine Leistung oder einen Artikel hinzu.", "lines"},
	"BR-CO-25":            {"Es fehlen Zahlungsangaben. Ergänze ein Fälligkeitsdatum oder Zahlungsbedingungen, z. B. „Zahlbar innerhalb von 14 Tagen“.", "paymentTerms"},
	"BR-CO-26":            {"Eine Verkäuferkennung fehlt. Ergänze deine Steuernummer oder USt-IdNr. in den Unternehmensdaten. Ohne USt-IdNr. verwendet die App die Steuernummer zusätzlich als Verkäuferkennung.", "company.taxNumber"},
	"BR-S-02":             {"Für Positionen mit Umsatzsteuer fehlt eine steuerliche Kennung des Verkäufers. Ergänze deine Steuernummer oder USt-IdNr. in den Unternehmensdaten.", "company.taxNumber"},
	"BR-AE-02":            {"Für die Steuerschuldnerschaft des Empfängers werden steuerliche Kennungen des Verkäufers und eine USt-IdNr. oder Registerkennung des Käufers 	benötigt. Prüfe die Unternehmens- und Empfängerdaten.", "customer.vatId"},
}

// sourceField maps the XML location of a finding to the input that produces it,
// so every finding can jump to its source, including untranslated rules.
func sourceField(location string) string {
	if location == "" {
		return ""
	}
	has := func(names ...string) bool {
		for _, name := range names {
			if regexp.MustCompile(`[:/]` + name + `(?:\[|/|\s|$)`).MatchString(location) {
				return true
			}
		}
		return false
	}
	if match := lineLocation.FindStringSubmatch(location); len(match) > 1 {
		n, _ := strconv.Atoi(match[1])
		return fmt.Sprintf("lines.%d", n-1)
	}
	if has("IncludedSupplyChainTradeLineItem", "InvoiceLine", "CreditNoteLine") {
		return "lines"
	}
	party := ""
	switch {
	case has("SellerTradeParty", "AccountingSupplierParty"):
		party = "company"
	case has("BuyerTradeParty", "AccountingCustomerParty"):
		party = "customer"
	}
	if party != "" {
		switch {
		case has("URIUniversalCommunication", "EndpointID"):
			return party + ".endpoint"
		case has("EmailURIUniversalCommunication", "ElectronicMail"):
			return party + ".email"
		case party == "company" && has("TelephoneUniversalCommunication", "Telephone"):
			return "company.phone"
		case has("DefinedTradeContact", "Contact"):
			return party + ".contactName"
		case has("CountryID", "IdentificationCode"):
			return party + ".address.country"
		case has("PostcodeCode", "PostalZone"):
			return party + ".address.postcode"
		case has("CityName"):
			return party + ".address.city"
		case has("LineTwo", "AdditionalStreetName"):
			return party + ".address.additional"
		case has("PostalTradeAddress", "PostalAddress"):
			return party + ".address.street"
		case has("SpecifiedTaxRegistration", "PartyTaxScheme"):
			return party + ".vatId"
		}
		return party + ".name"
	}
	for _, m := range []struct {
		field string
		names []string
	}{
		{"company.iban", []string{"PayeePartyCreditorFinancialAccount", "PayeeSpecifiedCreditorFinancialInstitution", "PayeeFinancialAccount"}},
		{"paymentMeansCode", []string{"SpecifiedTradeSettlementPaymentMeans", "PaymentMeans"}},
		{"dueDate", []string{"DueDateDateTime", "DueDate"}},
		{"paymentTerms", []string{"SpecifiedTradePaymentTerms", "PaymentTerms"}},
		{"paymentReference", []string{"PaymentReference", "PaymentID"}},
		{"adjustments", []string{"SpecifiedTradeAllowanceCharge", "AllowanceCharge"}},
		{"periodStart", []string{"BillingSpecifiedPeriod", "InvoicePeriod"}},
		{"taxExemptionReason", []string{"ExemptionReason", "ExemptionReasonCode", "TaxExemptionReason", "TaxExemptionReasonCode"}},
		{"taxTreatment", []string{"ApplicableTradeTax", "TaxTotal", "TaxSubtotal"}},
		{"deliveryDate", []string{"ActualDeliverySupplyChainEvent", "ActualDeliveryDate"}},
		{"deliveryCity", []string{"ShipToTradeParty", "Delivery"}},
		{"precedingInvoiceNumber", []string{"InvoiceReferencedDocument", "BillingReference"}},
		{"buyerReference", []string{"BuyerReference"}},
		{"issueDate", []string{"IssueDateTime", "IssueDate"}},
		{"kind", []string{"TypeCode", "InvoiceTypeCode", "CreditNoteTypeCode"}},
		{"notes", []string{"IncludedNote", "Note"}},
		{"businessProcess", []string{"BusinessProcessSpecifiedDocumentContextParameter", "ProfileID"}},
		{"profile", []string{"GuidelineSpecifiedDocumentContextParameter", "CustomizationID"}},
		{"lines", []string{"SpecifiedTradeSettlementHeaderMonetarySummation", "LegalMonetaryTotal"}},
	} {
		if has(m.names...) {
			return m.field
		}
	}
	return ""
}

var lineLocation = regexp.MustCompile(`(?:IncludedSupplyChainTradeLineItem|InvoiceLine|CreditNoteLine)(?:\[namespace-uri\(\)='[^']*'\])?\[(\d+)\]`)

func German(result model.ValidationResult) model.ValidationResult {
	for i := range result.Issues {
		issue := &result.Issues[i]
		if issue.Details != "" || (issue.Stage == "input" && issue.Field != "") {
			continue
		}
		original := strings.TrimSpace(issue.Message)
		issue.Details = original
		if g, ok := germanRules[issue.RuleID]; ok {
			issue.Message, issue.Field = g.message, g.field
			if issue.RuleID == "BR-CO-09" {
				switch {
				case strings.Contains(issue.Location, "BuyerTradeParty"), strings.Contains(issue.Location, "AccountingCustomerParty"):
					issue.Field = "customer.vatId"
					issue.Message = "Die Umsatzsteuer-ID deines Kunden braucht ein gültiges Länderkürzel am Anfang, zum Beispiel DE oder FR. Prüfe die Angabe in den Empfängerdaten."
				case strings.Contains(issue.Location, "SellerTaxRepresentativeTradeParty"), strings.Contains(issue.Location, "TaxRepresentativeParty"):
					issue.Field = ""
					issue.Message = "Die Umsatzsteuer-ID des Steuervertreters braucht ein gültiges Länderkürzel. Korrigiere diese Angabe in der ursprünglichen Rechnungsdatei."
				}
			}
			if issue.RuleID == "PEPPOL-EN16931-R008" && strings.Contains(issue.Location, "URIUniversalCommunication") {
				switch {
				case strings.Contains(issue.Location, "SellerTradeParty"):
					issue.Field = "company.endpoint"
					issue.Message = "Deine elektronische Absenderadresse ist leer. Trage sie unter Unternehmen → Elektronische Absenderadresse ein, z. B. deine E-Mail-Adresse."
				case strings.Contains(issue.Location, "BuyerTradeParty"):
					issue.Field = "customer.endpoint"
					issue.Message = "Die elektronische Adresse des Empfängers ist leer. Ergänze sie im erweiterten Menü oder entferne die gewählte Adressart."
				}
			}
			if strings.HasPrefix(issue.Field, "@line.") {
				if match := lineLocation.FindStringSubmatch(issue.Location); len(match) > 1 {
					n, _ := strconv.Atoi(match[1])
					issue.Field = fmt.Sprintf("lines.%d.%s", n-1, strings.TrimPrefix(issue.Field, "@line."))
					issue.Message = "Position " + match[1] + ": " + issue.Message
				} else {
					issue.Field = "lines"
				}
			}
			continue
		}
		switch issue.RuleID {
		case "DOCTYPE":
			issue.Message = "Die XML-Datei enthält eine nicht erlaubte Dokumenttyp-Deklaration (DOCTYPE). Verwende eine Rechnungsdatei ohne diese Deklaration."
		case "UNSUPPORTED":
			issue.Message = "Diese Kombination aus Profil und XML-Syntax wird nicht unterstützt. Verfügbar sind ZUGFeRD/Factur-X BASIC, EN 16931 und EXTENDED sowie XRechnung 3.0 in CII oder UBL."
		case "XML", "PARSE":
			issue.Message = "Die Datei konnte nicht als Rechnungs-XML gelesen werden. Vielleicht ist sie beschädigt oder enthält ungültiges XML."
		case "TIMEOUT":
			issue.Message = "Die Prüfung hat zu lange gedauert. Versuche es noch einmal. Die Rechnung ist bisher nicht als gültig bestätigt."
		case "XSD":
			issue.Message = "Der Aufbau oder ein Wert der XML-Datei entspricht nicht dem gewählten Rechnungsformat. Prüfe die betroffene Angabe; technische Einzelheiten findest du unter „Prüfdetails“."
			party := ""
			if strings.Contains(original, "BuyerTradeParty") {
				party = "customer"
			}
			if strings.Contains(original, "SellerTradeParty") {
				party = "company"
			}
			if party != "" && strings.Contains(original, "CountryID") {
				who := "des Rechnungsempfängers"
				if party == "company" {
					who = "deines Unternehmens"
				}
				issue.Message = "Das Land " + who + " fehlt oder ist ungültig. Verwende einen zweistelligen Ländercode, z. B. DE."
				issue.Field = party + ".address.country"
			}
		default:
			if issue.Stage == "rules" && strings.HasPrefix(issue.RuleID, "BR-") {
				issue.Message = "Die E-Rechnung erfüllt die Regel " + issue.RuleID + " nicht. 				Für diese Regel liegt noch keine deutsche Erläuterung vor. Die Originalmeldung und die betroffene XML-Stelle findest du unter „Prüfdetails“."
			} else {
				issue.Message = "Die Prüfung konnte nicht vollständig abgeschlossen werden. Prüfe deine Eingaben und versuche es noch einmal. Technische Einzelheiten findest du unter „Prüfdetails“."
			}
		}
	}
	for i := range result.Issues {
		if issue := &result.Issues[i]; issue.Field == "" && issue.Stage != "input" {
			if issue.Field = sourceField(issue.Location); issue.Field == "" {
				issue.Field = sourceField(issue.Details)
			}
		}
	}
	return result
}
