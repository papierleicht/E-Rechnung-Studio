package taxforms

import (
	"sort"
	"strings"

	"github.com/shopspring/decimal"
)

// ZMSale is a tax-free delivery of goods to a business in another member
// state (Kz 41) or a service for one (Kz 21), as reported in the
// Zusammenfassende Meldung (§ 18a UStG).
type ZMSale struct {
	Entry    Entry
	VATID    string
	Country  string
	Partner  string
	Services bool
	Net      decimal.Decimal
}

// ZMLine is one row of the Zusammenfassende Meldung: the sum per customer
// VAT ID, separately for goods and for services (Kennzeichen 1).
type ZMLine struct {
	VATID    string  `json:"vatId"`
	Country  string  `json:"country"`
	Partner  string  `json:"partner"`
	Services bool    `json:"services"`
	Amount   string  `json:"amount"` // whole euros
	Entries  []Entry `json:"entries"`
}

// ZMReport is a prepared Zusammenfassende Meldung of one Meldezeitraum.
type ZMReport struct {
	Lines  []ZMLine `json:"lines"`
	Total  string   `json:"total"`
	Issues []Issue  `json:"issues"`
}

// NormalizeVATID removes spaces, dots and dashes from a VAT ID.
func NormalizeVATID(v string) string {
	return strings.ToUpper(strings.NewReplacer(" ", "", ".", "", "-", "").Replace(strings.TrimSpace(v)))
}

// BuildZM sums the sales of one Meldezeitraum per VAT ID. Amounts are whole
// euros like the bases of the Voranmeldung, so both reports agree.
func BuildZM(sales []ZMSale) ZMReport {
	type key struct {
		vatID    string
		services bool
	}
	sums := map[key]*ZMLine{}
	totals := map[key]decimal.Decimal{}
	issues := issueList{}
	for _, s := range sales {
		vatID := NormalizeVATID(s.VATID)
		if len(vatID) < 4 {
			issues.add("zm_missing_vat_id", "Für diese Rechnungen an Unternehmen in anderen EU-Ländern fehlt die USt-IdNr. des Kunden. Ohne sie ist die Meldung unvollständig, und das kann die Steuerfreiheit gefährden.", s.Entry)
			continue
		}
		k := key{vatID, s.Services}
		if sums[k] == nil {
			sums[k] = &ZMLine{VATID: vatID, Country: vatID[:2], Partner: s.Partner, Services: s.Services, Entries: []Entry{}}
		}
		e := s.Entry
		e.Amount = s.Net.StringFixed(2)
		sums[k].Entries = append(sums[k].Entries, e)
		totals[k] = totals[k].Add(s.Net)
	}
	r := ZMReport{Lines: []ZMLine{}, Issues: issues.sorted()}
	total := decimal.Zero
	for k, line := range sums {
		amount := totals[k].Truncate(0)
		line.Amount = amount.StringFixed(0)
		total = total.Add(amount)
		r.Lines = append(r.Lines, *line)
	}
	sort.Slice(r.Lines, func(i, j int) bool {
		if r.Lines[i].VATID != r.Lines[j].VATID {
			return r.Lines[i].VATID < r.Lines[j].VATID
		}
		return !r.Lines[i].Services
	})
	r.Total = total.StringFixed(0)
	return r
}

// ZMGoodsLimit is the quarterly sum of deliveries of goods above which the
// Zusammenfassende Meldung is due monthly (§ 18a Abs. 1 UStG).
var ZMGoodsLimit = decimal.NewFromInt(50000)
