package validation

import (
	"fmt"
	"strings"
	"time"

	"github.com/shopspring/decimal"
	"papierleicht.de/e-rechnung-studio/internal/model"
)

func supportedPayment(code string) bool {
	return code == "10" || code == "30" || code == "58" || code == "97"
}

// AdvancedIssues checks relationships that individual form fields cannot check.
// Official profile rules are still run against the generated document afterwards.
func AdvancedIssues(d model.InvoiceDraft, c model.Company) []model.ValidationIssue {
	var issues []model.ValidationIssue
	add := func(field, text string) { issues = append(issues, localIssue(field, "INPUT", text)) }
	if !supportedPayment(model.PaymentCode(d, c)) {
		add("paymentMeansCode", "Wähle eine unterstützte Zahlungsart.")
	}
	if d.PrepaidAmount != "" {
		prepaid, err := decimal.NewFromString(strings.ReplaceAll(d.PrepaidAmount, ",", "."))
		if err != nil || prepaid.IsNegative() || !prepaid.Equal(prepaid.Round(2)) {
			add("prepaidAmount", "Gib einen bereits bezahlten Betrag ab 0 mit höchstens zwei Nachkommastellen ein.")
		}
	}
	switch d.Kind {
	case "invoice", "credit-note":
	case "corrected-invoice":
		if strings.TrimSpace(d.PrecedingInvoiceNumber) == "" {
			add("precedingInvoiceNumber", "Gib die Nummer der ursprünglichen Rechnung an.")
		}
	default:
		add("kind", "Wähle eine unterstützte Belegart.")
	}
	if d.PrecedingInvoiceDate != "" {
		if _, err := time.Parse("2006-01-02", d.PrecedingInvoiceDate); err != nil {
			add("precedingInvoiceDate", "Wähle ein gültiges Datum der ursprünglichen Rechnung.")
		}
	}
	groups := map[string]decimal.Decimal{}
	hasOutside, hasOther := false, false
	for i, line := range d.Lines {
		field := fmt.Sprintf("lines.%d.", i)
		tax, err := model.LineTax(d, line)
		if err != nil {
			add(field+"taxTreatment", err.Error())
			continue
		}
		hasOutside = hasOutside || tax.Category == "O"
		hasOther = hasOther || tax.Category != "O"
		if !strings.Contains("|S|Z|E|AE|K|G|O|", "|"+tax.Category+"|") {
			add(field+"taxTreatment", "Die Steuerkategorie wird nicht unterstützt.")
		}
		if tax.Category != "S" && tax.Category != "Z" && tax.Reason == "" && tax.Code == "" {
			reasonField := "taxExemptionReason"
			if d.TaxTreatment == "mixed" {
				reasonField = field + "exemptionReason"
			}
			add(reasonField, "Ergänze den Grund für die Steuerbefreiung.")
		}
		rate, rateErr := decimal.NewFromString(strings.ReplaceAll(tax.Rate, ",", "."))
		if rateErr != nil {
			continue
		}
		if (tax.Category == "S" && !rate.IsPositive()) || (tax.Category != "S" && !rate.IsZero()) {
			add(field+"taxRate", "Der Steuersatz passt nicht zum Steuerfall. Nutze bei 0 % den Nullsteuersatz oder einen passenden Befreiungsgrund.")
		}
		q, _ := decimal.NewFromString(strings.ReplaceAll(line.Quantity, ",", "."))
		price, _ := decimal.NewFromString(strings.ReplaceAll(line.UnitPrice, ",", "."))
		key := tax.Category + "|" + rate.String()
		groups[key] = groups[key].Add(q.Mul(price).Round(2))
	}
	if hasOutside && hasOther {
		add("taxTreatment", "Nicht steuerbare Umsätze (Kategorie O) brauchen eine eigene Rechnung und lassen sich nicht mit anderen Steuerkategorien kombinieren.")
	}
	adjusted := map[string]decimal.Decimal{}
	for key, value := range groups {
		adjusted[key] = value
	}
	for i, adjustment := range d.Adjustments {
		field := fmt.Sprintf("adjustments.%d.", i)
		amount, err := decimal.NewFromString(strings.ReplaceAll(adjustment.Amount, ",", "."))
		if err != nil || !amount.IsPositive() || !amount.Equal(amount.Round(2)) {
			add(field+"amount", "Gib einen positiven Nettobetrag mit höchstens zwei Nachkommastellen ein.")
			continue
		}
		if strings.TrimSpace(adjustment.Reason) == "" {
			add(field+"reason", "Gib eine Begründung für den Nachlass oder Zuschlag an.")
		}
		category, taxRate := model.AdjustmentTax(d, adjustment)
		rate, err := decimal.NewFromString(strings.ReplaceAll(taxRate, ",", "."))
		key := category + "|" + rate.String()
		if _, ok := groups[key]; err != nil || !ok {
			add(field+"taxCategory", "Wähle eine Steuergruppe aus den vorhandenen Positionen.")
			continue
		}
		if adjustment.Charge {
			adjusted[key] = adjusted[key].Add(amount)
		} else {
			adjusted[key] = adjusted[key].Sub(amount)
		}
	}
	if d.Kind != "corrected-invoice" {
		for _, value := range adjusted {
			if value.IsNegative() {
				add("adjustments", "Die Nachlässe übersteigen den Nettobetrag ihrer Steuergruppe.")
				break
			}
		}
	}
	return issues
}
