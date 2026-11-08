package taxforms

import (
	"sort"

	"github.com/shopspring/decimal"
)

// Entry is one record behind a form value; it mirrors the finance report.
type Entry struct {
	Kind   string `json:"kind"`
	ID     string `json:"id"`
	Date   string `json:"date"`
	Label  string `json:"label"`
	Amount string `json:"amount"`
	Note   string `json:"note,omitempty"`
}

// Field is one value to type into the form.
type Field struct {
	Zeile string `json:"zeile"`
	Kz    string `json:"kz"`
	Label string `json:"label"`
	// Amount uses a dot as decimal separator; bases are whole euros.
	Amount  string  `json:"amount"`
	Cents   bool    `json:"cents"`
	Entries []Entry `json:"entries"`
	// Info marks values that are shown for checking only; ELSTER calculates
	// them itself (e.g. the tax on Kz 81) or they are a total.
	Info bool `json:"info,omitempty"`
}

// Issue names records the form leaves out and why.
type Issue struct {
	Code    string  `json:"code"`
	Message string  `json:"message"`
	Entries []Entry `json:"entries"`
}

// Sale is the tax breakdown of one outgoing invoice (or the paid share of it
// under Ist-Versteuerung). Category is the EN 16931 VAT category code. Margin
// marks the Differenzbesteuerung (§ 25a UStG): the taxable margin depends on
// the purchase price, which the app does not know.
type Sale struct {
	Entry        Entry
	Category     string
	Percent      decimal.Decimal
	Net, Tax     decimal.Decimal
	BuyerCountry string
	Margin       bool
}

// Purchase is the tax breakdown of a received invoice or captured receipt.
// EU marks a supplier in another member state where no SellerCountry is known
// (receipts). NoInputTax is set for small businesses: they owe the tax on
// reverse-charge purchases but deduct no input tax.
type Purchase struct {
	Entry         Entry
	Category      string
	Percent       decimal.Decimal
	Net, Tax      decimal.Decimal
	SellerCountry string
	EU            bool
	NoInputTax    bool
}

// OwedTax is the tax the recipient owes on a reverse-charge purchase or an
// intra-community acquisition. Foreign invoices usually state no rate; the
// general rate of 19 % applies then.
func OwedTax(p Purchase) decimal.Decimal {
	percent := p.Percent
	if percent.IsZero() {
		percent = rate19
	}
	return p.Net.Mul(percent).Div(decimal.NewFromInt(100)).Round(2)
}

// Domestic reports a German or unknown customer country.
func Domestic(country string) bool { return country == "" || country == "DE" }

// VATReturn is a prepared Umsatzsteuer-Voranmeldung.
type VATReturn struct {
	Fields []Field `json:"fields"`
	// Payable is Kz 83: positive is paid to the tax office, negative refunded.
	// It is computed the way ELSTER does, from whole-euro bases.
	Payable string  `json:"payable"`
	Issues  []Issue `json:"issues"`
}

var euCountries = map[string]bool{"AT": true, "BE": true, "BG": true, "CY": true, "CZ": true, "DK": true, "EE": true, "ES": true, "FI": true, "FR": true, "GR": true, "EL": true, "HR": true, "HU": true, "IE": true, "IT": true, "LT": true, "LU": true, "LV": true, "MT": true, "NL": true, "PL": true, "PT": true, "RO": true, "SE": true, "SI": true, "SK": true}

// OtherEU reports a member state other than Germany.
func OtherEU(country string) bool { return euCountries[country] }

type fieldDef struct {
	zeile, kz, label string
	cents, info      bool
}

// Kennzahlen of USt 1 A 2026 the app can fill, in form order.
var vatFields = []fieldDef{
	{"13", "81", "Steuerpflichtige Umsätze zum Steuersatz von 19 % (Bemessungsgrundlage)", false, false},
	{"14", "86", "Steuerpflichtige Umsätze zum Steuersatz von 7 % (Bemessungsgrundlage)", false, false},
	{"15", "87", "Steuerpflichtige Umsätze zum Steuersatz von 0 % (Bemessungsgrundlage)", false, false},
	{"16", "35", "Umsätze zu anderen Steuersätzen (Bemessungsgrundlage)", false, false},
	{"16", "36", "Umsätze zu anderen Steuersätzen (Steuer)", true, false},
	{"19", "41", "Innergemeinschaftliche Lieferungen an Abnehmer mit USt-IdNr.", false, false},
	{"22", "43", "Weitere steuerfreie Umsätze mit Vorsteuerabzug (z. B. Ausfuhrlieferungen)", false, false},
	{"23", "48", "Steuerfreie Umsätze ohne Vorsteuerabzug", false, false},
	{"25", "89", "Innergemeinschaftliche Erwerbe zum Steuersatz von 19 % (Bemessungsgrundlage)", false, false},
	{"26", "93", "Innergemeinschaftliche Erwerbe zum Steuersatz von 7 % (Bemessungsgrundlage)", false, false},
	{"30", "46", "Sonstige Leistungen eines EU-Unternehmers, Steuer schuldest du (§ 13b Abs. 1 UStG, Bemessungsgrundlage)", false, false},
	{"30", "47", "Sonstige Leistungen eines EU-Unternehmers (§ 13b Abs. 1 UStG, Steuer)", true, false},
	{"32", "84", "Andere Leistungen, Steuer schuldest du (§ 13b Abs. 2 UStG, Bemessungsgrundlage)", false, false},
	{"32", "85", "Andere Leistungen (§ 13b Abs. 2 UStG, Steuer)", true, false},
	{"34", "60", "Umsätze, für die dein Kunde die Steuer schuldet (§ 13b Abs. 5 UStG)", false, false},
	{"35", "21", "Nicht steuerbare sonstige Leistungen an EU-Unternehmer (§ 18b Satz 1 Nr. 2 UStG)", false, false},
	{"36", "45", "Übrige nicht steuerbare Umsätze (Leistungsort nicht im Inland)", false, false},
	{"38", "66", "Vorsteuerbeträge aus Rechnungen anderer Unternehmer", true, false},
	{"39", "61", "Vorsteuer aus innergemeinschaftlichem Erwerb", true, false},
	{"41", "67", "Vorsteuer aus Leistungen im Sinne des § 13b UStG", true, false},
	{"49", "39", "Abzug der Sondervorauszahlung (Dauerfristverlängerung)", true, false},
}

var (
	rate19 = decimal.NewFromInt(19)
	rate7  = decimal.NewFromInt(7)
)

// issueList collects the records a form leaves out, one list per code.
type issueList map[string]*Issue

func (l issueList) add(code, message string, e Entry) {
	if l[code] == nil {
		l[code] = &Issue{Code: code, Message: message, Entries: []Entry{}}
	}
	l[code].Entries = append(l[code].Entries, e)
}

func (l issueList) sorted() []Issue {
	codes := make([]string, 0, len(l))
	for code := range l {
		codes = append(codes, code)
	}
	sort.Strings(codes)
	out := []Issue{}
	for _, code := range codes {
		out = append(out, *l[code])
	}
	return out
}

const marginMessage = "Verkäufe mit Differenzbesteuerung (§ 25a UStG) sind nicht enthalten, weil die App deine Einkaufspreise nicht kennt. Steuerpflichtig ist die Marge: Verkaufspreis minus Einkaufspreis, geteilt durch 1,19. Addiere sie zu Kennzahl 81; ist sie negativ, zählt sie mit 0."

// BuildVATReturn maps sales and purchases of one period to USt 1 A.
// special is the Sondervorauszahlung to deduct (zero if none).
func BuildVATReturn(sales []Sale, purchases []Purchase, special decimal.Decimal) VATReturn {
	amounts := map[string]decimal.Decimal{}
	entries := map[string][]Entry{}
	issues := issueList{}
	add := func(kz string, v decimal.Decimal, e Entry) {
		amounts[kz] = amounts[kz].Add(v)
		e.Amount = v.StringFixed(2)
		entries[kz] = append(entries[kz], e)
	}
	for _, s := range sales {
		switch s.Category {
		case "S":
			switch {
			case s.Percent.Equal(rate19):
				add("81", s.Net, s.Entry)
			case s.Percent.Equal(rate7):
				add("86", s.Net, s.Entry)
			case s.Percent.IsZero():
				add("87", s.Net, s.Entry)
			default:
				add("35", s.Net, s.Entry)
				add("36", s.Tax, s.Entry)
			}
		case "Z":
			add("87", s.Net, s.Entry)
		case "E":
			if s.Margin {
				e := s.Entry
				e.Note = "Verkaufspreis " + FormatGerman(s.Net.Add(s.Tax).StringFixed(2), true) + " €"
				issues.add("margin_scheme", marginMessage, e)
			} else {
				add("48", s.Net, s.Entry)
			}
		case "K":
			add("41", s.Net, s.Entry)
		case "G":
			add("43", s.Net, s.Entry)
		case "AE":
			switch {
			case OtherEU(s.BuyerCountry):
				add("21", s.Net, s.Entry)
			case Domestic(s.BuyerCountry):
				add("60", s.Net, s.Entry)
			default:
				// A service for a business outside the EU is taxed where the
				// customer is, not under § 13b in Germany.
				add("45", s.Net, s.Entry)
			}
		case "O":
			if Domestic(s.BuyerCountry) {
				issues.add("domestic_out_of_scope", "Nicht steuerbare Umsätze mit Kunden in Deutschland, etwa echter Schadensersatz, gehören nicht in die Voranmeldung. Nur wenn der Leistungsort im Ausland liegt, trägst du den Betrag selbst in Kennzahl 45 ein.", s.Entry)
			} else {
				add("45", s.Net, s.Entry)
			}
		default:
			issues.add("unsupported_sale", "Umsätze mit dieser Steuerkategorie ordnet die App keiner Kennzahl zu. Trage sie selbst ein.", s.Entry)
		}
	}
	for _, p := range purchases {
		owed := OwedTax(p)
		switch p.Category {
		case "AE":
			if p.EU || OtherEU(p.SellerCountry) {
				add("46", p.Net, p.Entry)
				add("47", owed, p.Entry)
			} else {
				add("84", p.Net, p.Entry)
				add("85", owed, p.Entry)
			}
			if !p.NoInputTax {
				add("67", owed, p.Entry)
			}
		case "K":
			if p.Percent.Equal(rate7) {
				add("93", p.Net, p.Entry)
			} else {
				add("89", p.Net, p.Entry)
			}
			if !p.NoInputTax {
				add("61", owed, p.Entry)
			}
		case "Z", "E", "O", "G":
			// No input tax.
		default:
			// Credit notes and corrections reduce the input tax.
			if !p.NoInputTax && !p.Tax.IsZero() {
				add("66", p.Tax, p.Entry)
			}
		}
	}
	if special.IsPositive() {
		add("39", special, Entry{Kind: "figure", ID: "special", Label: "Festgesetzte Sondervorauszahlung"})
	}

	base := func(kz string) decimal.Decimal { return amounts[kz].Truncate(0) }
	tax := func(kz string, pct decimal.Decimal) decimal.Decimal {
		return base(kz).Mul(pct).Div(decimal.NewFromInt(100)).Round(2)
	}
	payable := tax("81", rate19).Add(tax("86", rate7)).Add(amounts["36"]).
		Add(tax("89", rate19)).Add(tax("93", rate7)).Add(amounts["47"]).Add(amounts["85"]).
		Sub(amounts["66"]).Sub(amounts["61"]).Sub(amounts["67"]).Sub(amounts["39"])

	r := VATReturn{Fields: []Field{}, Payable: payable.StringFixed(2), Issues: issues.sorted()}
	for _, d := range vatFields {
		v, ok := amounts[d.kz]
		if !ok {
			continue
		}
		amount := v.Truncate(0).StringFixed(0)
		if d.cents {
			amount = v.Round(2).StringFixed(2)
		}
		list := entries[d.kz]
		sort.SliceStable(list, func(i, j int) bool { return list[i].Date < list[j].Date })
		r.Fields = append(r.Fields, Field{Zeile: d.zeile, Kz: d.kz, Label: d.label, Amount: amount, Cents: d.cents, Entries: list, Info: d.info})
	}
	r.Fields = append(r.Fields, Field{Zeile: "50", Kz: "83", Label: "Verbleibende Umsatzsteuer-Vorauszahlung (negativ: Überschuss)", Amount: r.Payable, Cents: true, Entries: []Entry{}, Info: true})
	return r
}

// FormatGerman renders "1234.5" as "1.234,50" (cents) or "1.234" (whole euros).
func FormatGerman(value string, cents bool) string {
	d, err := decimal.NewFromString(value)
	if err != nil {
		return value
	}
	places := int32(0)
	if cents {
		places = 2
	}
	s := d.Abs().StringFixed(places)
	whole, frac := s, ""
	if cents {
		whole, frac = s[:len(s)-3], s[len(s)-2:]
	}
	var out []byte
	for i := range whole {
		if i > 0 && (len(whole)-i)%3 == 0 {
			out = append(out, '.')
		}
		out = append(out, whole[i])
	}
	if cents {
		out = append(out, ',')
		out = append(out, frac...)
	}
	if d.IsNegative() {
		return "-" + string(out)
	}
	return string(out)
}
