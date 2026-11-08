package model

import (
	"fmt"
	"strings"

	"papierleicht.de/e-rechnung-studio/internal/catalog"

	"github.com/shopspring/decimal"
)

// TaxDetails is shared by input validation and XML generation. In mixed mode,
// each line selects its own treatment; otherwise the invoice setting wins.
type TaxDetails struct{ Category, Rate, Reason, Code string }

func SupportedTaxTreatment(value string) bool {
	if value == "mixed" {
		return true
	}
	_, ok := catalog.Tax(value)
	return ok
}

// Older company records use SmallBusiness only. Keep those defaults working.
func (c Company) InvoiceTaxTreatment() string {
	if c.InvoiceTaxPreset != nil {
		if *c.InvoiceTaxPreset != "" {
			return *c.InvoiceTaxPreset
		}
		if c.SmallBusiness {
			return "small-business"
		}
		return "standard"
	}
	if c.SmallBusiness {
		return "small-business"
	}
	if c.DefaultTaxTreatment != "" {
		return c.DefaultTaxTreatment
	}
	return "standard"
}

func LineTax(d InvoiceDraft, line InvoiceLine) (TaxDetails, error) {
	kind, reason, code := d.TaxTreatment, strings.TrimSpace(d.TaxExemptionReason), ""
	if kind == "mixed" {
		kind, reason, code = line.TaxTreatment, strings.TrimSpace(line.ExemptionReason), strings.TrimSpace(line.ExemptionCode)
		if kind == "" {
			kind = "standard"
		}
	}
	tax := TaxDetails{Category: "S", Rate: line.TaxRate, Reason: reason, Code: code}
	treatment, ok := catalog.Tax(kind)
	if !ok {
		return tax, fmt.Errorf("Wähle einen unterstützten Steuerfall.")
	}
	if kind == "standard" {
		if line.TaxCategory != "" {
			tax.Category = line.TaxCategory
		}
		// Numeric zero is zero-rated; leave invalid input for validation.
		if rate, err := decimal.NewFromString(strings.ReplaceAll(strings.TrimSpace(line.TaxRate), ",", ".")); err == nil && rate.IsZero() && tax.Category == "S" {
			tax.Category = "Z"
		}
	} else {
		tax.Category, tax.Rate = treatment.Category, "0"
		if treatment.Code != "" {
			tax.Code = treatment.Code
		}
	}
	if tax.Category == "S" || tax.Category == "Z" {
		tax.Reason, tax.Code = "", ""
	} else if tax.Reason == "" {
		tax.Reason = treatment.Reason
	}
	return tax, nil
}

// AdjustmentTax is the tax group of a document-level allowance or charge. An
// invoice-wide treatment other than standard (e.g. § 19) covers them as well.
func AdjustmentTax(d InvoiceDraft, a InvoiceAdjustment) (category, rate string) {
	if d.TaxTreatment != "mixed" && d.TaxTreatment != "standard" {
		if treatment, ok := catalog.Tax(d.TaxTreatment); ok {
			return treatment.Category, "0"
		}
	}
	return a.TaxCategory, a.TaxRate
}

// Reasons for a credit note or corrected invoice. A changed amount counts in
// the Voranmeldungszeitraum of the correction (§ 17 Abs. 1 Satz 7 UStG); a
// replaced faulty invoice keeps the period of the supply.
const (
	CorrectionChange  = "change"
	CorrectionReplace = "replace"
)

// NormalizeCorrectionReason keeps a supported reason on corrections only.
func NormalizeCorrectionReason(d InvoiceDraft) string {
	if d.Kind != "credit-note" && d.Kind != "corrected-invoice" {
		return ""
	}
	if d.CorrectionReason == CorrectionChange || d.CorrectionReason == CorrectionReplace {
		return d.CorrectionReason
	}
	return ""
}

func HasTaxCategory(d InvoiceDraft, category string) bool {
	for _, line := range d.Lines {
		if tax, err := LineTax(d, line); err == nil && tax.Category == category {
			return true
		}
	}
	return false
}

func PaymentCode(d InvoiceDraft, c Company) string {
	if d.PaymentMeansCode != "" {
		return d.PaymentMeansCode
	}
	if c.PaymentMeansCode != "" {
		return c.PaymentMeansCode
	}
	return "58"
}

func CreditTransfer(code string) bool { return code == "30" || code == "58" }

func (c Company) BankAccounts() []BankAccount {
	accounts := make([]BankAccount, 0, len(c.AdditionalAccounts)+1)
	if c.IBAN != "" {
		accounts = append(accounts, BankAccount{IBAN: c.IBAN, BIC: c.BIC, Name: c.BankName})
	}
	return append(accounts, c.AdditionalAccounts...)
}
