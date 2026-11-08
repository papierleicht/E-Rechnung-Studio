package validation

import (
	"fmt"
	"papierleicht.de/e-rechnung-studio/internal/invoicelogo"
	"papierleicht.de/e-rechnung-studio/internal/model"
	"regexp"
	"strings"
)

var germanVAT = regexp.MustCompile(`^DE[0-9]{9}$`)
var internationalVAT = regexp.MustCompile(`^[A-Z]{2}[A-Z0-9]{2,14}$`)
var germanTax = regexp.MustCompile(`^([0-9]{10,11}|[0-9]{13}|[0-9]{2,3}/[0-9]{3}/[0-9]{5}|[0-9]{3}/[0-9]{4}/[0-9]{4})$`)
var ibanShape = regexp.MustCompile(`^[A-Z]{2}[0-9]{2}[A-Z0-9]{11,30}$`)
var germanIBAN = regexp.MustCompile(`^DE[0-9]{20}$`)
var bicShape = regexp.MustCompile(`^[A-Z]{4}[A-Z]{2}[A-Z0-9]{2}([A-Z0-9]{3})?$`)

// Local format checks do not certify that an identifier has been assigned.
func validVAT(value string) bool {
	value = strings.ToUpper(model.CompactIdentifier(value))
	if strings.HasPrefix(value, "DE") {
		return germanVAT.MatchString(value)
	}
	return internationalVAT.MatchString(value)
}

// ISO 13616 / ISO 7064 MOD97-10; German BBANs contain 18 digits.
// National lengths from SWIFT IBAN Registry, release 101 (December 2025).
// https://www.swift.com/sites/default/files/files/iban-registry-v101.pdf
var ibanLengths = map[string]int{
	"AD": 24, "AL": 28, "AT": 20, "BE": 16, "BG": 22, "CH": 21, "CY": 28, "CZ": 24,
	"DE": 22, "DK": 18, "EE": 20, "ES": 24, "FI": 18, "FR": 27, "GB": 22, "GI": 23,
	"GR": 27, "HR": 21, "HU": 28, "IE": 22, "IS": 26, "IT": 27, "LI": 21, "LT": 20,
	"LU": 20, "LV": 21, "MC": 27, "MD": 24, "ME": 22, "MK": 19, "MT": 31, "NL": 18,
	"NO": 15, "PL": 28, "PT": 25, "RO": 24, "RS": 22, "SE": 24, "SI": 19, "SK": 24,
	"SM": 27, "VA": 22,
	"AE": 23, "AZ": 28, "BA": 20, "BH": 22, "BI": 27, "BR": 29, "BY": 28, "CR": 22,
	"DJ": 27, "DO": 28, "EG": 29, "FK": 18, "FO": 18, "GE": 22, "GL": 18, "GT": 28,
	"HN": 28, "IL": 23, "IQ": 23, "JO": 30, "KW": 30, "KZ": 20, "LB": 28, "LC": 32,
	"LY": 25, "MN": 20, "MR": 27, "MU": 30, "NI": 28, "OM": 23, "PK": 24, "PS": 29,
	"QA": 29, "RU": 33, "SA": 24, "SC": 31, "SD": 18, "SO": 23, "ST": 25, "SV": 28,
	"TL": 23, "TN": 24, "TR": 26, "UA": 29, "VG": 24, "XK": 20, "YE": 30,
}

func validIBAN(value string) bool {
	value = strings.ToUpper(model.CompactIdentifier(value))
	if !ibanShape.MatchString(value) || (strings.HasPrefix(value, "DE") && !germanIBAN.MatchString(value)) {
		return false
	}
	if length, ok := ibanLengths[value[:2]]; !ok || len(value) != length {
		return false
	}
	// 00, 01 and 99 can have the same remainder as canonical check digits.
	if value[2:4] < "02" || value[2:4] > "98" {
		return false
	}
	remainder := 0
	for _, c := range value[4:] + value[:4] {
		if c >= '0' && c <= '9' {
			remainder = (remainder*10 + int(c-'0')) % 97
		} else {
			remainder = (remainder*100 + int(c-'A') + 10) % 97
		}
	}
	return remainder == 1
}

func localIssue(field, rule, message string) model.ValidationIssue {
	return model.ValidationIssue{Stage: "input", RuleID: rule, Severity: "error", Field: field, Message: message}
}

// Supplied identifiers are checked even on partial company records. Missing
// identifiers remain the responsibility of invoice/profile completeness rules.
func CompanyIssues(c model.Company) []model.ValidationIssue {
	c = model.NormalizeCompany(c)
	issues := []model.ValidationIssue{}
	if c.InvoiceTaxPreset != nil && *c.InvoiceTaxPreset != "" && (!model.SupportedTaxTreatment(*c.InvoiceTaxPreset) || *c.InvoiceTaxPreset == "small-business") {
		issues = append(issues, localIssue("company.invoiceTaxPreset", "APP-TAX-PRESET", "Wähle eine unterstützte Rechnungsvorgabe. Die Kleinunternehmerregelung legst du im Unternehmensprofil fest."))
	}
	if c.DefaultTaxTreatment != "" && !model.SupportedTaxTreatment(c.DefaultTaxTreatment) {
		issues = append(issues, localIssue("company.defaultTaxTreatment", "APP-TAX-TREATMENT", "Wähle eine unterstützte Abrechnungsart."))
	}
	if model.CompactIdentifier(c.VATID) != "" && !validVAT(c.VATID) {
		issues = append(issues, localIssue("company.vatId", "APP-VAT-FORMAT", "Prüfe die USt-IdNr. Eine deutsche USt-IdNr. besteht aus DE und genau neun Ziffern; eine Steuernummer gehört in das separate Feld."))
	}
	if c.TaxNumber != "" && c.Address.Country == "DE" && !germanTax.MatchString(c.TaxNumber) {
		issues = append(issues, localIssue("company.taxNumber", "APP-TAX-FORMAT", "Prüfe die Steuernummer: 10 oder 11 Ziffern im Landesformat (auch mit Schrägstrichen) oder 13 Ziffern im Bundesschema. Führende Nullen gehören dazu."))
	}
	if c.IBAN != "" && !validIBAN(c.IBAN) {
		issues = append(issues, localIssue("company.iban", "APP-IBAN", "Prüfe die IBAN: Aufbau, Länge oder Prüfziffer stimmen nicht. Eine deutsche IBAN hat 22 Zeichen."))
	}
	if c.BIC != "" && !bicShape.MatchString(c.BIC) {
		issues = append(issues, localIssue("company.bic", "APP-BIC", "Prüfe die BIC. Sie muss aus 8 oder 11 Zeichen im BIC-Format bestehen."))
	}
	seen := map[string]bool{c.IBAN: true}
	for i, account := range c.AdditionalAccounts {
		field := fmt.Sprintf("company.additionalAccounts.%d.", i)
		if !validIBAN(account.IBAN) {
			issues = append(issues, localIssue(field+"iban", "APP-IBAN", "Ergänze eine gültige zusätzliche IBAN."))
		}
		if account.IBAN != "" && seen[account.IBAN] {
			issues = append(issues, localIssue(field+"iban", "APP-IBAN-DUPLICATE", "Diese IBAN ist bereits hinterlegt."))
		}
		seen[account.IBAN] = true
		if account.BIC != "" && !bicShape.MatchString(account.BIC) {
			issues = append(issues, localIssue(field+"bic", "APP-BIC", "Prüfe die BIC der zusätzlichen Bankverbindung."))
		}
	}
	if c.PaymentMeansCode != "" && !supportedPayment(c.PaymentMeansCode) {
		issues = append(issues, localIssue("company.paymentMeansCode", "APP-PAYMENT", "Wähle eine unterstützte Zahlungsart."))
	}
	if _, err := invoicelogo.Decode(c.Logo); err != nil {
		issues = append(issues, localIssue("company.logo", "APP-LOGO", err.Error()))
	}
	return issues
}

func CustomerIssues(c model.Customer) []model.ValidationIssue {
	if model.CompactIdentifier(c.VATID) != "" && !validVAT(c.VATID) {
		return []model.ValidationIssue{localIssue("customer.vatId", "APP-VAT-FORMAT", "Prüfe die USt-IdNr. des Empfängers. Eine deutsche USt-IdNr. besteht aus DE und genau neun Ziffern; ausländische Kennungen beginnen mit dem Länderkürzel.")}
	}
	return nil
}
