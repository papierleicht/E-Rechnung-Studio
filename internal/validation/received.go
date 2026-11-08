package validation

import (
	"strings"

	"papierleicht.de/e-rechnung-studio/internal/einvoice"
	"papierleicht.de/e-rechnung-studio/internal/model"
)

// Classes of a finding in a received document. They follow the distinction of
// the BMF letters of 15.10.2025 (Rn. 6a, 6b, 35a) and 15.10.2026 (Rn. 22, 23):
// a format error makes the file no e-invoice at all, a business-rule error does
// not, and a business-rule error matters for the tax only if it concerns a
// Pflichtangabe of §§ 14, 14a UStG.
const (
	// ClassFormat: the file violates the syntax or its technical rules
	// (Formatfehler). For the tax it is then an "other invoice".
	ClassFormat = "format"
	// ClassBusiness: the file violates a business rule of the e-invoice standard
	// (Geschäftsregelfehler). It remains an e-invoice for the tax.
	ClassBusiness = "business"
	// ClassHint: a safeguard of this app, not a conformance requirement.
	ClassHint = "hint"
	// ClassUnchecked: the app could not check the document.
	ClassUnchecked = "unchecked"
)

// taxIrrelevantRules are business rules whose violation concerns no Pflichtangabe
// of §§ 14, 14a UStG: buyer reference, seller contact, electronic addresses and
// payment instructions. The BMF letter of 15.10.2025 (Rn. 35a) names the buyer
// reference (BT-10) as its example. Every other rule stays blocking, because it
// can concern an amount, a tax rate, an identifier, an address or the description
// of the supply; the app does not try to tell those cases apart.
var taxIrrelevantRules = map[string]bool{
	// XRechnung
	"BR-DE-1": true, "BR-DE-2": true, "BR-DE-5": true, "BR-DE-6": true, "BR-DE-7": true,
	"BR-DE-15": true, "BR-DE-22": true,
	"BR-DE-23-a": true, "BR-DE-23-b": true, "BR-DE-24-a": true, "BR-DE-24-b": true,
	"BR-DE-25-a": true, "BR-DE-25-b": true, "BR-DE-30": true, "BR-DE-31": true,
	// Peppol: business process, electronic addresses, direct-debit mandate
	"PEPPOL-EN16931-R001": true, "PEPPOL-EN16931-R010": true, "PEPPOL-EN16931-R020": true,
	"PEPPOL-EN16931-R061": true,
	// EN 16931: payment means, electronic address and identifier schemes
	"BR-49": true, "BR-50": true, "BR-61": true, "BR-62": true, "BR-63": true,
	"BR-64": true, "BR-65": true,
}

// sellerIdentifierHints are the app's own format checks of the supplier's tax
// number and VAT ID. A malformed identifier can mean a missing Pflichtangabe, so
// these hints keep blocking; all other hints (IBAN, BIC, payment terms) do not.
var sellerIdentifierHints = map[string]bool{"APP-VAT-FORMAT": true, "APP-TAX-FORMAT": true}

// engineFailures are rule-stage results that mean the check itself failed.
var engineFailures = map[string]bool{"XSLT": true, "SVRL-SERIALIZE": true, "SVRL-PARSE": true, "TIMEOUT": true}

func classOf(issue model.ValidationIssue) string {
	switch issue.Stage {
	case "input":
		return ClassHint
	case "syntax", "xsd":
		return ClassFormat
	case "rules":
		switch {
		case engineFailures[issue.RuleID]:
			return ClassUnchecked
		// The CII and UBL syntax-binding rules check the technical structure.
		case strings.HasPrefix(issue.RuleID, "CII-"), strings.HasPrefix(issue.RuleID, "UBL-"):
			return ClassFormat
		}
		return ClassBusiness
	}
	return ClassUnchecked
}

// blocks reports whether a finding keeps a received document out of the Vorsteuer.
func blocks(issue model.ValidationIssue) bool {
	switch classOf(issue) {
	case ClassFormat, ClassUnchecked:
		return true
	case ClassBusiness:
		// Warnings and notes do not violate a mandatory rule.
		return isError(issue.Severity) && !taxIrrelevantRules[issue.RuleID]
	}
	return sellerIdentifierHints[issue.RuleID] && strings.HasPrefix(issue.Field, "company.")
}

// ReceivedInputTax makes the app's suggestion from the stored findings: whether a
// received document goes into the Vorsteuer of the app's figures. It does not
// judge whether the document entitles to the deduction, and the user can decide
// otherwise. Format errors, business-rule errors that can concern a Pflichtangabe,
// malformed supplier identifiers and documents the app could not check stay out
// until then; findings that concern no Pflichtangabe do not.
func ReceivedInputTax(result model.ValidationResult) model.InputTaxStatus {
	if result.State == "valid" {
		return model.InputTaxStatus{Allowed: true}
	}
	blockers := map[string]bool{}
	for _, issue := range result.Issues {
		if blocks(issue) {
			blockers[classOf(issue)] = true
		}
	}
	switch result.State {
	case "warning", "invalid":
		// A failed document without any finding cannot be explained away.
		if len(result.Issues) == 0 {
			blockers[ClassUnchecked] = true
		}
	default:
		blockers[ClassUnchecked] = true
	}
	for _, class := range []string{ClassFormat, ClassBusiness, ClassHint, ClassUnchecked} {
		if blockers[class] {
			return blockedInputTax(class)
		}
	}
	return model.InputTaxStatus{Allowed: true, Reason: "Die technische Prüfung meldet nur Hinweise zu Angaben, die in der Regel keine Pflichtangaben nach § 14 UStG sind, etwa Käuferreferenz, Kontakt- oder Zahlungsangaben. Die App rechnet die Steuer deshalb ein. Ob die Rechnung inhaltlich stimmt und du die Vorsteuer abziehen darfst, prüft sie nicht."}
}

func blockedInputTax(class string) model.InputTaxStatus {
	const decide = " Die App rechnet die Steuer deshalb nicht von selbst ein. Du entscheidest unten, ob und wie viel davon zählt."
	s := model.InputTaxStatus{Blocker: class}
	switch class {
	case ClassFormat:
		s.Short = "Formatfehler"
		s.Reason = "Die Datei hat einen Formatfehler. Steuerlich ist sie dann in der Regel keine E-Rechnung, sondern eine sonstige Rechnung. Das kann in den Übergangsfristen noch ausreichen, und auch danach ist ein Vorsteuerabzug nicht ausgeschlossen." + decide
	case ClassBusiness:
		s.Short = "Regelverstoß prüfen"
		s.Reason = "Die Datei verletzt eine Prüfregel, die eine Pflichtangabe betreffen kann, etwa Beträge, Steuersatz, Kennungen oder Anschrift. Bei einem Fehler kann der Lieferant die Rechnung berichtigen." + decide
	case ClassHint:
		s.Short = "Kennung prüfen"
		s.Reason = "Die Steuernummer oder USt-IdNr. des Lieferanten sieht ungewöhnlich aus. Vergleiche sie mit dem Beleg." + decide
	default:
		s.Blocker = ClassUnchecked
		s.Short = "nicht geprüft"
		s.Reason = "Die App konnte diese Rechnung nicht vollständig prüfen." + decide
	}
	return s
}

// AnnotateReceived marks every finding of a received document with its class and
// adds the Vorsteuer status. Both derive from the stored findings, so documents
// inspected by an earlier version are covered as well. The argument stays intact.
func AnnotateReceived(inspection model.ImportInspection) model.ImportInspection {
	if inspection.Validation.Issues != nil {
		issues := make([]model.ValidationIssue, len(inspection.Validation.Issues))
		for i, issue := range inspection.Validation.Issues {
			issue.Class = classOf(issue)
			issues[i] = issue
		}
		inspection.Validation.Issues = issues
	}
	status := ReceivedInputTax(inspection.Validation)
	inspection.InputTax = &status
	return inspection
}

// RelaxSellerIdentifier handles BR-CO-26 for a received document. The rule wants
// a seller identifier (BT-29), a legal registration identifier (BT-30) or a VAT ID
// (BT-31) so that the buyer's software can identify the supplier. The Pflichtangabe
// of § 14 Abs. 4 Satz 1 Nr. 2 UStG is met by the tax number (BT-32) just as well, so
// a document that carries one keeps the finding as a warning with an explanation.
// Without any tax identifier the finding stays an error.
func RelaxSellerIdentifier(inspection model.ImportInspection, inv *einvoice.Invoice) model.ImportInspection {
	if inv == nil || strings.TrimSpace(inv.Seller.FCTaxRegistration) == "" {
		return inspection
	}
	issues := make([]model.ValidationIssue, len(inspection.Validation.Issues))
	copy(issues, inspection.Validation.Issues)
	relaxed, blocking := false, false
	for i, issue := range issues {
		if issue.Stage == "rules" && issue.RuleID == "BR-CO-26" && isError(issue.Severity) {
			issues[i].Severity = "warning"
			issues[i].Message = "Die Rechnung nennt keine Verkäuferkennung nach EN 16931 (BT-29, BT-30 oder BT-31), aber eine Steuernummer des Lieferanten. § 14 Abs. 4 UStG verlangt Steuernummer oder USt-IdNr."
			relaxed = true
		}
		blocking = blocking || isError(issues[i].Severity)
	}
	if !relaxed {
		return inspection
	}
	inspection.Validation.Issues = issues
	if inspection.Validation.State == "invalid" && !blocking {
		inspection.Validation.State = "warning"
	}
	return inspection
}

// isError reports whether a severity violates a mandatory rule.
func isError(severity string) bool {
	return severity == "" || severity == "error" || severity == "fatal"
}
