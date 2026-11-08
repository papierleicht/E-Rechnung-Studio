package datev

import (
	"errors"
	"fmt"
	"strings"
)

// RevenueAccounts maps each tax case of an outgoing invoice to a revenue
// account. An empty account leaves the case unconfigured; affected invoices
// are then reported instead of being exported.
type RevenueAccounts struct {
	Standard19      string `json:"standard19"`      // 19 % USt
	Reduced7        string `json:"reduced7"`        // 7 % USt
	ZeroRated       string `json:"zeroRated"`       // 0 % (§ 12 Abs. 3 UStG)
	Exempt          string `json:"exempt"`          // steuerfrei, § 4 Nr. 8 ff. UStG
	Export          string `json:"export"`          // Ausfuhr, § 4 Nr. 1a UStG
	IntraCommunity  string `json:"intraCommunity"`  // innergemeinschaftliche Lieferung
	ReverseCharge   string `json:"reverseCharge"`   // § 13b UStG, Empfänger im Inland
	ReverseChargeEU string `json:"reverseChargeEU"` // sonstige Leistung, Empfänger im übrigen EU-Gebiet
	OutOfScope      string `json:"outOfScope"`      // im Inland nicht steuerbar
	SmallBusiness   string `json:"smallBusiness"`   // Kleinunternehmer § 19 UStG
	Margin          string `json:"margin"`          // Differenzbesteuerung § 25a UStG
}

// Accounts are all accounts the export books to.
type Accounts struct {
	Revenue RevenueAccounts `json:"revenue"`
	// Expense is the default expense account for received invoices; input
	// tax is booked with BU-Schlüssel 9 (19 %) or 8 (7 %).
	Expense string `json:"expense"`
	// Categories maps an expense category to its account. Categories left
	// out fall back to the standard account of the chart, then to Expense.
	Categories map[string]string `json:"categories,omitempty"`
	// Bank is the money account for optional payment bookings.
	Bank string `json:"bank"`
	// Debtor and Creditor are the collective accounts, or the first number
	// of consecutive person accounts.
	Debtor   string `json:"debtor"`
	Creditor string `json:"creditor"`
}

// Standard accounts with four digits. Sources: DATEV-Kontenrahmen SKR 03
// (Stand 2024) and SKR 04 (Stand 2026). The revenue accounts for 19 %, 7 %,
// 0 %, tax-free, intra-community and § 13b cases are automatic accounts (AM),
// so bookings carry no BU-Schlüssel; the small-business and margin accounts
// book no tax. They are suggestions to agree with the tax advisor.
var standard = map[string]Accounts{
	"SKR03": {Revenue: RevenueAccounts{
		Standard19: "8400", Reduced7: "8300", ZeroRated: "8290", Exempt: "8100", Export: "8120",
		IntraCommunity: "8125", ReverseCharge: "8337", ReverseChargeEU: "8336", OutOfScope: "8338",
		SmallBusiness: "8195", Margin: "8193",
	}, Expense: "4980", Bank: "1200"},
	"SKR04": {Revenue: RevenueAccounts{
		Standard19: "4400", Reduced7: "4300", ZeroRated: "4290", Exempt: "4100", Export: "4120",
		IntraCommunity: "4125", ReverseCharge: "4337", ReverseChargeEU: "4336", OutOfScope: "4338",
		SmallBusiness: "4184", Margin: "4138",
	}, Expense: "6850", Bank: "1800"},
}

// standardCategories are the expense accounts per category. None is an
// automatic account, so bookings carry the input tax as BU-Schlüssel. Purchases
// used for several years go to "sonstige Betriebs- und Geschäftsausstattung".
var standardCategories = map[string]map[string]string{
	"SKR03": {
		"arbeitsmittel": "4930", "edv": "4806", "telekommunikation": "4920", "fremdleistungen": "3100", "waren": "3200",
		"werbung": "4600", "reise": "4670", "fahrtkosten": "4673", "fahrzeug": "4530", "fahrzeug_steuern": "4520",
		"miete": "4210", "versicherung": "4360", "beratung": "4950", "fortbildung": "4945", "reparatur": "4805",
		"leasing": "4810", "personal": "4120", "bank": "4970", "zinsen": "2110", "bewirtung": "4650",
		"geschenke": "4630", "asset": "0490", "sonstige": "4900",
	},
	"SKR04": {
		"arbeitsmittel": "6815", "edv": "6495", "telekommunikation": "6805", "fremdleistungen": "5900", "waren": "5200",
		"werbung": "6600", "reise": "6670", "fahrtkosten": "6673", "fahrzeug": "6530", "fahrzeug_steuern": "6520",
		"miete": "6310", "versicherung": "6400", "beratung": "6825", "fortbildung": "6821", "reparatur": "6470",
		"leasing": "6840", "personal": "6020", "bank": "6855", "zinsen": "7310", "bewirtung": "6640",
		"geschenke": "6610", "asset": "0690", "sonstige": "6300",
	},
}

func padAccount(s string, length int) string {
	if length <= len(s) {
		return s
	}
	return s + strings.Repeat("0", length-len(s))
}

// CategoryAccount is the account for an expense category under these settings.
func CategoryAccount(a Accounts, chart string, length int, category string) string {
	if account := strings.TrimSpace(a.Categories[category]); account != "" {
		return account
	}
	if account := standardCategories[chart][category]; account != "" {
		return padAccount(account, length)
	}
	return a.Expense
}

// DefaultAccounts returns the suggested accounts for a chart and account
// length. Longer account numbers are padded with zeros on the right, as DATEV
// does when a client uses more than four digits; person accounts are one
// digit longer (debtors from 10000, creditors from 70000 at length 4).
func DefaultAccounts(chart string, length int) (Accounts, error) {
	a, ok := standard[chart]
	if !ok {
		return Accounts{}, errors.New("Wähle SKR03 oder SKR04.")
	}
	if length < 4 || length > 8 {
		return Accounts{}, errors.New("Die Sachkontenlänge muss zwischen 4 und 8 liegen.")
	}
	pad := func(s string) string { return s + strings.Repeat("0", length-len(s)) }
	r := &a.Revenue
	for _, p := range []*string{&r.Standard19, &r.Reduced7, &r.ZeroRated, &r.Exempt, &r.Export, &r.IntraCommunity, &r.ReverseCharge, &r.ReverseChargeEU, &r.OutOfScope, &r.SmallBusiness, &r.Margin, &a.Expense, &a.Bank} {
		*p = pad(*p)
	}
	a.Categories = map[string]string{}
	for category, account := range standardCategories[chart] {
		a.Categories[category] = pad(account)
	}
	a.Debtor = "1" + strings.Repeat("0", length)
	a.Creditor = "7" + strings.Repeat("0", length)
	return a, nil
}

// CheckAccount validates an entered account number. Ledger accounts have at
// most length digits; person accounts exactly one digit more. Empty is
// allowed and means "not configured".
func CheckAccount(label, account string, length int, person bool) error {
	if account == "" {
		return nil
	}
	if !digits.MatchString(account) || strings.Trim(account, "0") == "" {
		return fmt.Errorf("%s: Gib nur Ziffern ein.", label)
	}
	if person && len(account) != length+1 {
		return fmt.Errorf("%s: Personenkonten haben bei Sachkontenlänge %d genau %d Stellen.", label, length, length+1)
	}
	if !person && len(account) > length {
		return fmt.Errorf("%s: Sachkonten haben bei Sachkontenlänge %d höchstens %d Stellen.", label, length, length)
	}
	return nil
}
