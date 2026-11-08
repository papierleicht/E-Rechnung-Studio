package engine

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/shopspring/decimal"
	"papierleicht.de/e-rechnung-studio/internal/einvoice"
	"papierleicht.de/e-rechnung-studio/internal/model"
)

func quantity(value string, signed bool) (decimal.Decimal, error) {
	if !signed {
		return parseAmount("Menge", value, true)
	}
	n, err := decimal.NewFromString(strings.ReplaceAll(strings.TrimSpace(value), ",", "."))
	if err != nil || n.IsZero() {
		return decimal.Zero, fmt.Errorf("Die Menge muss eine Zahl ungleich 0 sein.")
	}
	return n, nil
}

func setPayment(inv *einvoice.Invoice, d model.InvoiceDraft, c model.Company) error {
	code := model.PaymentCode(d, c)
	switch code {
	case "10", "30", "58", "97":
	default:
		return fmt.Errorf("Wähle eine unterstützte Zahlungsart.")
	}
	n, _ := strconv.Atoi(code)
	if model.CreditTransfer(code) {
		for _, account := range c.BankAccounts() {
			if account.IBAN == "" {
				continue
			}
			inv.PaymentMeans = append(inv.PaymentMeans, einvoice.PaymentMeans{TypeCode: n, PayeePartyCreditorFinancialAccountIBAN: account.IBAN, PayeeSpecifiedCreditorFinancialInstitutionBIC: account.BIC, PayeePartyCreditorFinancialAccountName: account.Name})
		}
	} else {
		inv.PaymentMeans = []einvoice.PaymentMeans{{TypeCode: n}}
	}
	return nil
}

// EN 16931 has a single exemption per tax category/rate group. Preserve all
// reasons when several exempt treatments share a group; omit conflicting codes.
func mergeExemption(reasons, codes map[string]string, tax model.TaxDetails) {
	if tax.Reason != "" {
		found := false
		for _, reason := range strings.Split(reasons[tax.Category], "; ") {
			found = found || reason == tax.Reason
		}
		if !found {
			if reasons[tax.Category] != "" {
				reasons[tax.Category] += "; "
			}
			reasons[tax.Category] += tax.Reason
		}
	}
	if previous, exists := codes[tax.Category]; exists && previous != tax.Code {
		codes[tax.Category] = ""
	} else if !exists {
		codes[tax.Category] = tax.Code
	}
}

func setAdjustments(inv *einvoice.Invoice, d model.InvoiceDraft) error {
	for i, adjustment := range d.Adjustments {
		amount, err := parseAmount(fmt.Sprintf("Nachlass/Zuschlag %d", i+1), adjustment.Amount, true)
		if err != nil {
			return err
		}
		if !amount.Equal(amount.Round(2)) {
			return fmt.Errorf("Nachlass/Zuschlag %d: Der Betrag darf höchstens zwei Nachkommastellen haben.", i+1)
		}
		category, taxRate := model.AdjustmentTax(d, adjustment)
		rate, err := parseAmount("Steuersatz", taxRate, false)
		if err != nil {
			return err
		}
		inv.SpecifiedTradeAllowanceCharge = append(inv.SpecifiedTradeAllowanceCharge, einvoice.AllowanceCharge{ChargeIndicator: adjustment.Charge, ActualAmount: amount, Reason: strings.TrimSpace(adjustment.Reason), CategoryTradeTaxType: "VAT", CategoryTradeTaxCategoryCode: category, CategoryTradeTaxRateApplicablePercent: rate})
	}
	return nil
}
