package validation

import (
	"regexp"
	"strings"

	"papierleicht.de/e-rechnung-studio/internal/einvoice"
	"papierleicht.de/e-rechnung-studio/internal/model"
)

// German speaks to the issuer of an invoice: "Ergänze deine Steuernummer".
// Shown for a document someone else issued, such a text sends the recipient to
// fix the supplier's invoice. receivedWording states the same findings as plain
// facts about the document instead. Rules without an entry have no issuer voice.
var receivedWording = map[string]string{
	// XRechnung
	"BR-DE-1":  "Die Rechnung enthält keine Zahlungsanweisung (Zahlungsart und Bankverbindung).",
	"BR-DE-2":  "Die Rechnung enthält keinen Ansprechpartner des Lieferanten mit Telefon und E-Mail (XRechnung).",
	"BR-DE-5":  "Der Lieferant hat keinen Ansprechpartner angegeben.",
	"BR-DE-6":  "Der Lieferant hat keine Telefonnummer für Rückfragen angegeben.",
	"BR-DE-7":  "Der Lieferant hat keine Kontakt-E-Mail angegeben.",
	"BR-DE-10": "Die Lieferadresse nennt keinen Ort.",
	"BR-DE-11": "Die Lieferadresse nennt keine Postleitzahl.",
	"BR-DE-15": "Die Rechnung enthält keine Käuferreferenz (Leitweg-ID).",
	"BR-DE-28": "Die Kontakt-E-Mail-Adresse des Lieferanten ist ungültig geschrieben.",
	// Peppol
	"PEPPOL-EN16931-R008": "Die XML-Datei enthält leere Felder. Die genaue Stelle steht unter „Prüfdetails“.",
	"PEPPOL-EN16931-R010": "Die elektronische Adresse des Empfängers fehlt in der Rechnung.",
	"PEPPOL-EN16931-R020": "Die elektronische Adresse des Lieferanten fehlt in der Rechnung.",
	// EN 16931
	"BR-01":    "Die Kennung des E-Rechnungsformats fehlt in der Datei.",
	"BR-02":    "Die Rechnungsnummer fehlt.",
	"BR-03":    "Das Rechnungsdatum fehlt.",
	"BR-06":    "Der Name des Lieferanten fehlt.",
	"BR-07":    "Der Name des Empfängers fehlt.",
	"BR-08":    "Die Anschrift des Lieferanten fehlt.",
	"BR-09":    "Das Land des Lieferanten fehlt.",
	"BR-10":    "Die Anschrift des Empfängers fehlt.",
	"BR-11":    "Das Land des Empfängers fehlt.",
	"BR-16":    "Die Rechnung enthält keine Position.",
	"BR-22":    "Die Menge der Position fehlt.",
	"BR-25":    "Die Bezeichnung der Position fehlt.",
	"BR-50":    "Für die Überweisung fehlt die Kontokennung (IBAN) in der Rechnung.",
	"BR-61":    "Für die Überweisung fehlt die Kontokennung (IBAN) in der Rechnung.",
	"BR-CO-25": "Es fehlen Zahlungsangaben: Die Rechnung nennt weder ein Fälligkeitsdatum noch Zahlungsbedingungen.",
	"BR-CO-26": "Die Rechnung enthält keine Verkäuferkennung (EN 16931: BT-29, BT-30 oder BT-31).",
	// Tax categories
	"BR-E-02":  "Für steuerbefreite Positionen fehlt eine steuerliche Kennung des Lieferanten (Steuernummer oder USt-IdNr.).",
	"BR-E-05":  "Bei einer von Umsatzsteuer befreiten Position muss der Steuersatz 0 % betragen.",
	"BR-S-02":  "Für Positionen mit Umsatzsteuer fehlt eine steuerliche Kennung des Lieferanten (Steuernummer oder USt-IdNr.).",
	"BR-S-05":  "Bei Regelbesteuerung muss der Steuersatz der Position größer als 0 sein.",
	"BR-IC-02": "Die USt-IdNr. von Lieferant und Empfänger reichen für die innergemeinschaftliche Lieferung nicht aus.",
	"BR-IC-12": "Für die innergemeinschaftliche Lieferung fehlt das Lieferland in der Rechnung.",
	"BR-AE-02": "Die steuerlichen Kennungen von Lieferant und Empfänger reichen für die Steuerschuldnerschaft des Empfängers nicht aus.",
	// The app's own checks
	"APP-TAX-FORMAT":    "Die Steuernummer des Lieferanten hat kein gültiges Format: erwartet werden 10 oder 11 Ziffern im Landesformat (auch mit Schrägstrichen) oder 13 Ziffern im Bundesschema.",
	"APP-IBAN":          "Die IBAN auf der Rechnung ist ungültig: Aufbau, Länge oder Prüfziffer stimmen nicht. Prüfe sie, bevor du überweist.",
	"APP-BIC":           "Die BIC auf der Rechnung hat kein gültiges Format: Sie besteht aus 8 oder 11 Zeichen.",
	"APP-PAYMENT-TERMS": "Die Rechnung nennt weder ein Fälligkeitsdatum noch Zahlungsbedingungen für den offenen Betrag.",
}

// linePrefix is the "Position 2: " German puts in front of findings of a line.
var linePrefix = regexp.MustCompile(`^Position \d+: `)

// GermanReceived words the findings of a document someone else issued for its
// recipient. Rule, severity, field and the original message stay as they are.
// The parsed document, if there is one, tells which identifiers a party lacks.
func GermanReceived(result model.ValidationResult, inv *einvoice.Invoice) model.ValidationResult {
	if len(result.Issues) == 0 {
		return result
	}
	result.Issues = append([]model.ValidationIssue(nil), result.Issues...)
	for i := range result.Issues {
		issue := &result.Issues[i]
		if text := receivedText(*issue, inv); text != "" {
			issue.Message = linePrefix.FindString(issue.Message) + text
		}
	}
	return result
}

// receivedText returns the wording for the recipient, or "" if the finding needs none.
func receivedText(issue model.ValidationIssue, inv *einvoice.Invoice) string {
	party := func(buyer, seller, representative string) string {
		switch {
		case strings.Contains(issue.Location, "BuyerTradeParty"), strings.Contains(issue.Location, "AccountingCustomerParty"):
			return buyer
		case strings.Contains(issue.Location, "SellerTaxRepresentativeTradeParty"), strings.Contains(issue.Location, "TaxRepresentativeParty"):
			return representative
		}
		return seller
	}
	if text := intraCommunityWording(issue.RuleID, inv); text != "" {
		return text
	}
	switch issue.RuleID {
	case "BR-CO-09":
		return party(
			"Die USt-IdNr. des Empfängers braucht ein gültiges Länderkürzel am Anfang, zum Beispiel DE oder FR.",
			"Die USt-IdNr. des Lieferanten braucht ein gültiges Länderkürzel am Anfang, zum Beispiel DE oder FR; für Griechenland ist EL zulässig.",
			"Die USt-IdNr. des Steuervertreters braucht ein gültiges Länderkürzel am Anfang.")
	case "PEPPOL-EN16931-R008":
		if strings.Contains(issue.Location, "URIUniversalCommunication") {
			return party("Die elektronische Adresse des Empfängers ist leer.", "Die elektronische Adresse des Lieferanten ist leer.", "")
		}
	case "APP-VAT-FORMAT":
		if strings.HasPrefix(issue.Field, "customer.") {
			return "Die USt-IdNr. des Empfängers hat kein gültiges Format. Eine deutsche USt-IdNr. besteht aus DE und genau neun Ziffern; ausländische Kennungen beginnen mit dem Länderkürzel."
		}
		return "Die USt-IdNr. des Lieferanten hat kein gültiges Format. Eine deutsche USt-IdNr. besteht aus DE und genau neun Ziffern."
	case "XSD":
		if strings.HasPrefix(issue.Message, "Das Land ") && strings.Contains(issue.Message, "Verwende einen zweistelligen Ländercode") {
			if strings.HasPrefix(issue.Field, "customer.") {
				return "Das Land des Empfängers fehlt oder ist kein zweistelliger Ländercode."
			}
			return "Das Land des Lieferanten fehlt oder ist kein zweistelliger Ländercode."
		}
	case "DOCTYPE":
		return "Die XML-Datei enthält eine nicht erlaubte Dokumenttyp-Deklaration (DOCTYPE). Bitte den Lieferanten um eine Rechnungsdatei ohne diese Deklaration."
	}
	if text, ok := receivedWording[issue.RuleID]; ok {
		return text
	}
	// The generic fallback of German asks the issuer to check their input.
	if strings.Contains(issue.Message, "Prüfe deine Eingaben und versuche") {
		return strings.Replace(issue.Message, "Prüfe deine Eingaben und versuche", "Versuche", 1)
	}
	return ""
}

// intraCommunityWording names the party whose tax identifier a reverse-charge or
// intra-community invoice lacks. It needs the parsed document.
func intraCommunityWording(rule string, inv *einvoice.Invoice) string {
	if inv == nil {
		return ""
	}
	sellerMissing, buyerMissing, sellerTaxMissing, buyerIdentifierMissing := identifierGaps(inv)
	switch rule {
	case "BR-AE-02", "BR-AE-03", "BR-AE-04":
		switch {
		case sellerTaxMissing && buyerIdentifierMissing:
			return "Für diese Rechnung mit Steuerschuld beim Empfänger fehlen steuerliche Kennungen auf beiden Seiten: die Steuernummer oder USt-IdNr. des Lieferanten und die USt-IdNr. oder Registerkennung des Empfängers."
		case sellerTaxMissing:
			return "Für diese Rechnung mit Steuerschuld beim Empfänger fehlt die Steuernummer oder USt-IdNr. des Lieferanten."
		case buyerIdentifierMissing:
			return "Für diese Rechnung mit Steuerschuld beim Empfänger fehlt die USt-IdNr. oder zulässige Registerkennung des Empfängers."
		}
	case "BR-IC-02", "BR-IC-03", "BR-IC-04":
		switch {
		case sellerMissing && buyerMissing:
			return "Die Rechnung weist eine steuerfreie Lieferung in ein anderes EU-Land aus, nennt aber weder die USt-IdNr. des Lieferanten noch die des Empfängers. Eine Steuernummer reicht dafür nicht aus."
		case sellerMissing:
			return "Die Rechnung weist eine steuerfreie Lieferung in ein anderes EU-Land aus, nennt aber nicht die USt-IdNr. des Lieferanten. Eine Steuernummer reicht dafür nicht aus."
		case buyerMissing:
			return "Die Rechnung weist eine steuerfreie Lieferung in ein anderes EU-Land aus, nennt aber nicht die USt-IdNr. des Empfängers."
		}
	}
	return ""
}
