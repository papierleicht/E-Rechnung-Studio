package umzug

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/shopspring/decimal"
	"papierleicht.de/e-rechnung-studio/internal/model"
)

// Field is a target the columns of a list can be assigned to.
type Field struct {
	ID       string `json:"id"`
	Label    string `json:"label"`
	Required bool   `json:"required"`
}

// Lists the app can take over.
const (
	ListCustomers = "customers"
	ListProducts  = "products"
)

var customerFields = []Field{
	{"name", "Firma oder Name", true},
	{"firstName", "Vorname", false},
	{"lastName", "Nachname", false},
	{"contactName", "Ansprechpartner", false},
	{"email", "E-Mail", false},
	{"vatId", "USt-IdNr.", false},
	{"street", "Straße", false},
	{"houseNumber", "Hausnummer", false},
	{"additional", "Adresszusatz", false},
	{"postcode", "PLZ", false},
	{"city", "Ort", false},
	{"country", "Land", false},
	{"buyerReference", "Leitweg-ID", false},
}

var productFields = []Field{
	{"name", "Bezeichnung", true},
	{"articleNumber", "Artikelnummer", false},
	{"description", "Beschreibung", false},
	{"unit", "Einheit", false},
	{"unitPrice", "Nettopreis", false},
	{"taxRate", "Steuersatz", false},
}

// Fields returns the targets of a list.
func Fields(list string) []Field {
	if list == ListProducts {
		return productFields
	}
	return customerFields
}

// Column names the programs use, compared by headerKey, best first.
var synonyms = map[string]map[string][]string{
	ListCustomers: {
		"name":           {"firma", "firmenname", "unternehmen", "firmaname", "name1", "kundenname", "kunde", "organisation", "company", "companyname", "name", "anzeigename", "displayname"},
		"firstName":      {"vorname", "firstname", "givenname"},
		"lastName":       {"nachname", "lastname", "familienname", "surname"},
		"contactName":    {"ansprechpartner", "kontaktperson", "kontakt", "contactperson", "contact", "name2"},
		"email":          {"email", "emailadresse", "mail", "emailaddress", "rechnungsemail"},
		"vatId":          {"ustidnr", "ustid", "umsatzsteueridentifikationsnummer", "umsatzsteuerid", "ustidentnr", "vatid", "vatnumber", "ustidnummer"},
		"street":         {"strasse", "strassehausnummer", "strasseundhausnummer", "strassenr", "adresse", "anschrift", "street", "address", "addressline1", "rechnungsadresse"},
		"houseNumber":    {"hausnummer", "hausnr", "nr", "housenumber"},
		"additional":     {"adresszusatz", "zusatz", "addressline2", "co"},
		"postcode":       {"plz", "postleitzahl", "zip", "zipcode", "postalcode", "postcode"},
		"city":           {"ort", "stadt", "city", "wohnort"},
		"country":        {"land", "laendercode", "country", "countrycode", "staat"},
		"buyerReference": {"leitwegid", "leitweg", "kaeuferreferenz", "buyerreference"},
	},
	ListProducts: {
		"name":          {"bezeichnung", "artikelbezeichnung", "artikelname", "name", "titel", "produkt", "produktname", "artikel", "leistung", "title", "product", "kurztext"},
		"articleNumber": {"artikelnummer", "artikelnr", "artnr", "nummer", "sku", "produktnummer", "number", "itemnumber"},
		"description":   {"beschreibung", "langtext", "description", "details", "text"},
		"unit":          {"einheit", "mengeneinheit", "unit", "masseinheit"},
		"unitPrice":     {"nettopreis", "preisnetto", "vknetto", "verkaufspreisnetto", "einzelpreis", "verkaufspreis", "vkpreis", "preis", "price", "unitprice", "netprice"},
		"taxRate":       {"steuersatz", "mwstsatz", "ustsatz", "mwst", "ust", "umsatzsteuer", "taxrate", "vat", "steuer"},
	},
}

// Mapping assigns a column index to each field; missing fields stay unmapped.
type Mapping map[string]int

// GuessMapping assigns columns by their names. The customer can change it.
func GuessMapping(list string, columns []string) Mapping {
	keys := make([]string, len(columns))
	for i, c := range columns {
		keys[i] = headerKey(c)
	}
	used := map[int]bool{}
	m := Mapping{}
	for _, f := range Fields(list) {
		for _, name := range synonyms[list][f.ID] {
			found := -1
			for i, k := range keys {
				if k == name && !used[i] {
					found = i
					break
				}
			}
			if found >= 0 {
				m[f.ID], used[found] = found, true
				break
			}
		}
	}
	return m
}

func cell(row []string, m Mapping, field string) string {
	at, ok := m[field]
	if !ok || at < 0 || at >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[at])
}

// CustomerProposal is one row of a customer list, ready to be saved, with a
// note when the app had to assume something or when the row cannot be taken over.
type CustomerProposal struct {
	Row   int            `json:"row"`
	Item  model.Customer `json:"item"`
	Note  string         `json:"note,omitempty"`
	Error string         `json:"error,omitempty"`
	// Existing names the customer that already exists; such rows are not saved.
	Existing string `json:"existing,omitempty"`
}

// ProductProposal is one row of an article list, like CustomerProposal.
type ProductProposal struct {
	Row      int           `json:"row"`
	Item     model.Product `json:"item"`
	Note     string        `json:"note,omitempty"`
	Error    string        `json:"error,omitempty"`
	Existing string        `json:"existing,omitempty"`
}

var countries = map[string]string{
	"deutschland": "DE", "germany": "DE", "d": "DE", "brd": "DE", "de": "DE",
	"oesterreich": "AT", "austria": "AT", "a": "AT", "at": "AT",
	"schweiz": "CH", "switzerland": "CH", "ch": "CH",
	"niederlande": "NL", "netherlands": "NL", "nl": "NL", "belgien": "BE", "belgium": "BE", "be": "BE",
	"frankreich": "FR", "france": "FR", "fr": "FR", "italien": "IT", "italy": "IT", "it": "IT",
	"spanien": "ES", "spain": "ES", "es": "ES", "polen": "PL", "poland": "PL", "pl": "PL",
	"luxemburg": "LU", "luxembourg": "LU", "lu": "LU", "daenemark": "DK", "denmark": "DK", "dk": "DK",
	"tschechien": "CZ", "czechia": "CZ", "cz": "CZ", "grossbritannien": "GB", "vereinigteskoenigreich": "GB", "unitedkingdom": "GB", "gb": "GB", "uk": "GB",
	"usa": "US", "vereinigtestaaten": "US", "unitedstates": "US", "us": "US",
}

var countryCode = regexp.MustCompile(`^[A-Z]{2}$`)

// Customers turns the rows of a list into customers. existing are the
// customers already in the app; a name or USt-IdNr. that is already there is
// not taken over twice.
func Customers(t Table, m Mapping, existing []model.Customer) []CustomerProposal {
	known := map[string]string{}
	for _, c := range existing {
		known[nameKey(c.Name)] = c.Name
		if v := strings.ToUpper(model.CompactIdentifier(c.VATID)); v != "" {
			known["vat:"+v] = c.Name
		}
	}
	out := make([]CustomerProposal, 0, len(t.Rows))
	for i, row := range t.Rows {
		p := CustomerProposal{Row: i + 2}
		c := model.Customer{
			Name: cell(row, m, "name"), ContactName: cell(row, m, "contactName"), Email: cell(row, m, "email"),
			VATID: strings.ToUpper(model.CompactIdentifier(cell(row, m, "vatId"))), BuyerReference: cell(row, m, "buyerReference"),
		}
		person := strings.TrimSpace(cell(row, m, "firstName") + " " + cell(row, m, "lastName"))
		if c.Name == "" {
			c.Name = person
		} else if c.ContactName == "" && person != "" && nameKey(person) != nameKey(c.Name) {
			c.ContactName = person
		}
		street := cell(row, m, "street")
		if n := cell(row, m, "houseNumber"); n != "" && !strings.HasSuffix(street, n) {
			street = strings.TrimSpace(street + " " + n)
		}
		c.Address = model.Address{Street: street, Additional: cell(row, m, "additional"), Postcode: cell(row, m, "postcode"), City: cell(row, m, "city")}
		country := cell(row, m, "country")
		switch code := countries[headerKey(country)]; {
		case code != "":
			c.Address.Country = code
		case countryCode.MatchString(strings.ToUpper(country)):
			c.Address.Country = strings.ToUpper(country)
		case country == "":
			c.Address.Country = "DE"
			if c.Address.Street != "" || c.Address.City != "" {
				p.Note = "Kein Land angegeben, Deutschland angenommen."
			}
		default:
			c.Address.Country = "DE"
			p.Note = fmt.Sprintf("Land „%s“ nicht erkannt, Deutschland angenommen. Bitte prüfen.", country)
		}
		// XRechnung needs an electronic address; the e-mail is the usual one.
		if c.BuyerReference != "" && c.Email != "" {
			c.Endpoint, c.EndpointScheme = c.Email, "EM"
		}
		p.Item = model.NormalizeCustomer(c)
		switch {
		case p.Item.Name == "":
			p.Error = "Kein Name: Diese Zeile wird nicht übernommen."
		case known[nameKey(p.Item.Name)] != "":
			p.Existing = known[nameKey(p.Item.Name)]
		case p.Item.VATID != "" && known["vat:"+p.Item.VATID] != "":
			p.Existing = known["vat:"+p.Item.VATID]
		default:
			known[nameKey(p.Item.Name)] = p.Item.Name
			if p.Item.VATID != "" {
				known["vat:"+p.Item.VATID] = p.Item.Name
			}
		}
		out = append(out, p)
	}
	return out
}

var units = map[string]string{
	"c62": "C62", "stk": "C62", "stck": "C62", "st": "C62", "stueck": "C62", "stuck": "C62", "pcs": "C62", "pc": "C62", "piece": "C62", "pieces": "C62", "x": "C62", "anzahl": "C62", "ea": "C62",
	"hur": "HUR", "std": "HUR", "stunde": "HUR", "stunden": "HUR", "h": "HUR", "hour": "HUR", "hours": "HUR", "hrs": "HUR",
	"day": "DAY", "tag": "DAY", "tage": "DAY", "days": "DAY", "pt": "DAY", "personentag": "DAY", "personentage": "DAY",
	"kgm": "KGM", "kg": "KGM", "kilogramm": "KGM",
	"mtr": "MTR", "m": "MTR", "meter": "MTR", "lfm": "MTR",
	"ls": "LS", "pauschal": "LS", "pauschale": "LS", "psch": "LS", "pausch": "LS", "flat": "LS", "lumpsum": "LS",
	"wee": "WEE", "woche": "WEE", "wochen": "WEE", "week": "WEE", "weeks": "WEE",
	"mon": "MON", "monat": "MON", "monate": "MON", "month": "MON", "months": "MON", "mt": "MON",
	"ltr": "LTR", "l": "LTR", "liter": "LTR",
	"mtk": "MTK", "m2": "MTK", "qm": "MTK", "quadratmeter": "MTK",
	"mtq": "MTQ", "m3": "MTQ", "cbm": "MTQ", "kubikmeter": "MTQ",
	"kmt": "KMT", "km": "KMT", "kilometer": "KMT",
	"tne": "TNE", "t": "TNE", "tonne": "TNE", "tonnen": "TNE",
}

// Amount reads a price as the programs write it: "1.234,50 €", "1234.5",
// "EUR 12,00". An empty text is zero.
func Amount(s string) (decimal.Decimal, bool) {
	rest := strings.TrimSpace(strings.NewReplacer("EUR", "", "€", "", "%", "").Replace(strings.ToUpper(s)))
	if rest != "" && !strings.ContainsAny(rest, "0123456789") {
		return decimal.Zero, false
	}
	s = strings.Map(func(r rune) rune {
		if (r >= '0' && r <= '9') || r == ',' || r == '.' || r == '-' {
			return r
		}
		return -1
	}, s)
	if s == "" {
		return decimal.Zero, true
	}
	comma, dot := strings.LastIndex(s, ","), strings.LastIndex(s, ".")
	switch {
	case comma >= 0 && dot >= 0 && comma > dot:
		s = strings.ReplaceAll(s, ".", "")
		s = strings.Replace(s, ",", ".", 1)
	case comma >= 0 && dot >= 0:
		s = strings.ReplaceAll(s, ",", "")
	case comma >= 0:
		if strings.Count(s, ",") > 1 {
			return decimal.Zero, false
		}
		s = strings.Replace(s, ",", ".", 1)
	case dot >= 0 && (strings.Count(s, ".") > 1 || len(s)-dot-1 == 3):
		// German thousands: "1.234" or "1.234.567".
		s = strings.ReplaceAll(s, ".", "")
	}
	v, err := decimal.NewFromString(s)
	return v, err == nil
}

// Products turns the rows of a list into articles. existing are the articles
// already in the app; an article number or name that is there is not taken over twice.
func Products(t Table, m Mapping, existing []model.Product) []ProductProposal {
	known := map[string]string{}
	for _, p := range existing {
		known[nameKey(p.Name)] = p.Name
		if p.ArticleNumber != "" {
			known["no:"+strings.ToLower(p.ArticleNumber)] = p.Name
		}
	}
	out := make([]ProductProposal, 0, len(t.Rows))
	for i, row := range t.Rows {
		p := ProductProposal{Row: i + 2}
		item := model.Product{Name: cell(row, m, "name"), ArticleNumber: cell(row, m, "articleNumber"), Description: cell(row, m, "description")}
		var notes []string
		unit := cell(row, m, "unit")
		item.Unit = units[headerKey(unit)]
		if item.Unit == "" {
			item.Unit = "C62"
			if unit != "" {
				notes = append(notes, fmt.Sprintf("Einheit „%s“ nicht erkannt, Stück angenommen.", unit))
			}
		}
		price, ok := Amount(cell(row, m, "unitPrice"))
		switch {
		case !ok || price.IsNegative():
			p.Error = fmt.Sprintf("Preis „%s“ ist keine gültige Zahl.", cell(row, m, "unitPrice"))
		default:
			item.UnitPrice = price.String()
		}
		rateText := cell(row, m, "taxRate")
		rate, ok := Amount(rateText)
		if ok && rate.IsPositive() && rate.LessThan(decimal.NewFromInt(1)) {
			rate = rate.Mul(decimal.NewFromInt(100))
		}
		switch {
		case !ok || rate.IsNegative() || rate.GreaterThan(decimal.NewFromInt(100)):
			if p.Error == "" {
				p.Error = fmt.Sprintf("Steuersatz „%s“ ist keine gültige Zahl.", rateText)
			}
		case rateText == "":
			item.TaxRate, item.TaxCategory = "19", "S"
			notes = append(notes, "Kein Steuersatz angegeben, 19 % angenommen. Bitte prüfen.")
		case rate.IsZero():
			item.TaxRate, item.TaxCategory = "0", "E"
			notes = append(notes, "0 %: als steuerbefreit übernommen. Bitte im Artikel prüfen, welcher Steuerfall gilt.")
		default:
			item.TaxRate, item.TaxCategory = rate.String(), "S"
		}
		p.Item, p.Note = item, strings.Join(notes, " ")
		switch {
		case p.Error != "":
		case item.Name == "":
			p.Error = "Keine Bezeichnung: Diese Zeile wird nicht übernommen."
		case item.ArticleNumber != "" && known["no:"+strings.ToLower(item.ArticleNumber)] != "":
			p.Existing = known["no:"+strings.ToLower(item.ArticleNumber)]
		case known[nameKey(item.Name)] != "":
			p.Existing = known[nameKey(item.Name)]
		default:
			known[nameKey(item.Name)] = item.Name
			if item.ArticleNumber != "" {
				known["no:"+strings.ToLower(item.ArticleNumber)] = item.Name
			}
		}
		out = append(out, p)
	}
	return out
}
