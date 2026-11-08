// Package taxforms maps the app's records to the official forms a self-filing
// business transfers to ELSTER: the Umsatzsteuer-Voranmeldung (USt 1 A 2026)
// and the Anlage EÜR (Vordruck 2025). It only prepares values; it neither
// files nor gives tax advice. Line numbers (Zeile) and Kennzahlen (Kz) follow
// the BMF form templates of 29.12.2025 (USt 1 A) and August 2025 (Anlage EÜR).
package taxforms

import "github.com/shopspring/decimal"

// Category is an expense group of the Anlage EÜR. Labels use plain words;
// Zeile and Kz name the form field.
type Category struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Hint  string `json:"hint"`
	Zeile string `json:"zeile"`
	Kz    string `json:"kz"`
	// Line names the form line when several categories share it.
	Line string `json:"-"`
	// Deductible is the share that reduces the profit; the rest is shown
	// as not deductible (Zeile 63 Bewirtung: 70 %).
	Deductible decimal.Decimal `json:"-"`
	// NonDeductibleKz receives the remaining share, if any.
	NonDeductibleKz string `json:"-"`
}

// AssetCategory marks purchases used for several years. Net amounts above
// GWGLimit are depreciated (Zeile 33), others are deducted at once (Zeile 36).
const AssetCategory = "asset"

// GWGLimit is the net limit for geringwertige Wirtschaftsgüter (§ 6 Abs. 2 EStG).
var GWGLimit = decimal.NewFromInt(800)

var full = decimal.NewFromInt(1)

// Categories lists the expense groups offered in the app, most common first.
var Categories = []Category{
	{ID: "arbeitsmittel", Label: "Büro & Arbeitsmittel", Hint: "Bürobedarf, Porto, Fachliteratur, kleine Werkzeuge", Zeile: "51", Kz: "229", Deductible: full},
	{ID: "edv", Label: "Software & IT", Hint: "Abos, Hosting, Wartung, IT-Dienstleistungen", Zeile: "50", Kz: "228", Deductible: full},
	{ID: "telekommunikation", Label: "Telefon & Internet", Hint: "Mobilfunk, Festnetz, Internetanschluss", Zeile: "43", Kz: "280", Deductible: full},
	{ID: "fremdleistungen", Label: "Fremdleistungen", Hint: "Freie Mitarbeiter, Subunternehmer, eingekaufte Leistungen für Kunden", Zeile: "29", Kz: "110", Deductible: full},
	{ID: "waren", Label: "Waren & Material", Hint: "Einkauf zum Weiterverkauf, Material für Aufträge", Zeile: "27", Kz: "100", Deductible: full},
	{ID: "werbung", Label: "Werbung", Hint: "Anzeigen, Website-Werbung, Flyer, Messen", Zeile: "54", Kz: "224", Deductible: full},
	{ID: "reise", Label: "Übernachtung & Reisenebenkosten", Hint: "Hotel, Parken, Gepäck bei Geschäftsreisen (Fahrkarten und Flüge unter „Bahn, Flug & Nahverkehr“)", Zeile: "44", Kz: "221", Deductible: full},
	{ID: "fahrtkosten", Label: "Bahn, Flug & Nahverkehr", Hint: "Fahrkarten und Flüge für Geschäftsreisen", Zeile: "70", Kz: "146", Line: fahrtLine, Deductible: full},
	{ID: "fahrzeug", Label: "Fahrzeugkosten", Hint: "Tanken, Reparatur, Wartung eines Fahrzeugs im Betriebsvermögen. Für dein Privatauto setzt du stattdessen 0,30 € je dienstlich gefahrenem Kilometer an (Zeile 71).", Zeile: "70", Kz: "146", Line: fahrtLine, Deductible: full},
	{ID: "fahrzeug_steuern", Label: "Kfz-Steuer & -Versicherung", Hint: "Kfz-Steuer, Kfz-Versicherung, Maut", Zeile: "69", Kz: "145", Deductible: full},
	{ID: "miete", Label: "Miete für Geschäftsräume", Hint: "Büro, Werkstatt, Lager (nicht das häusliche Arbeitszimmer)", Zeile: "39", Kz: "150", Deductible: full},
	{ID: "versicherung", Label: "Versicherungen & Beiträge", Hint: "Betriebshaftpflicht, Kammer- und Verbandsbeiträge, Gebühren", Zeile: "49", Kz: "223", Deductible: full},
	{ID: "beratung", Label: "Beratung & Buchführung", Hint: "Rechtsberatung, Steuerberatung, Buchhaltungssoftware", Zeile: "46", Kz: "194", Deductible: full},
	{ID: "fortbildung", Label: "Fortbildung", Hint: "Kurse, Seminare, Konferenzgebühren (ohne Reisekosten)", Zeile: "45", Kz: "281", Deductible: full},
	{ID: "reparatur", Label: "Reparatur & Wartung", Hint: "Instandhaltung von Geräten und Einrichtung (ohne Gebäude und Fahrzeug)", Zeile: "48", Kz: "225", Deductible: full},
	{ID: "leasing", Label: "Miete & Leasing von Geräten", Hint: "Geleaste oder gemietete Geräte (ohne Fahrzeug)", Zeile: "47", Kz: "222", Deductible: full},
	{ID: "personal", Label: "Personal", Hint: "Löhne, Gehälter, Sozialversicherung", Zeile: "30", Kz: "120", Deductible: full},
	{ID: "bank", Label: "Bankgebühren", Hint: "Kontoführung, Kartengebühren, Zahlungsdienste", Zeile: "60", Kz: "183", Line: otherLine, Deductible: full},
	{ID: "zinsen", Label: "Kreditzinsen", Hint: "Zinsen für betriebliche Kredite (ohne Kredite für Anschaffungen)", Zeile: "56", Kz: "234", Deductible: full},
	{ID: "bewirtung", Label: "Bewirtung", Hint: "Geschäftsessen mit Kunden; 70 % mindern den Gewinn. Im Restaurant gelten für Speisen 7 %, für Getränke 19 % – wähle dann „19 % und 7 % gemischt“.", Zeile: "63", Kz: "175", Deductible: decimal.RequireFromString("0.7"), NonDeductibleKz: "165"},
	{ID: "geschenke", Label: "Geschenke bis 50 € je Empfänger", Hint: "Geschenke an Geschäftspartner bis 50 € netto im Jahr pro Person; teurere sind gar nicht abziehbar", Zeile: "62", Kz: "174", Deductible: full},
	{ID: AssetCategory, Label: "Anschaffung (mehrere Jahre genutzt)", Hint: "Computer, Möbel, Maschinen; über 800 € netto wird abgeschrieben", Zeile: "33/36", Kz: "130/132", Deductible: full},
	{ID: "sonstige", Label: "Sonstige Betriebsausgaben", Hint: "Alles, was in keine andere Gruppe passt. Nicht die Gewerbesteuer: Sie mindert den Gewinn nicht, ordne den Bankumsatz als „Gewerbesteuer“ ein.", Zeile: "60", Kz: "183", Line: otherLine, Deductible: full},
}

const (
	fahrtLine = "Sonstige Fahrtkosten (ohne AfA und Zinsen)"
	otherLine = "Übrige unbeschränkt abziehbare Betriebsausgaben"
)

// CategoryByID returns the category or false.
func CategoryByID(id string) (Category, bool) {
	for _, c := range Categories {
		if c.ID == id {
			return c, true
		}
	}
	return Category{}, false
}
