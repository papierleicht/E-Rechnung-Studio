package app

import (
	"strings"
	"testing"
)

func csvOne(t *testing.T, text string) []csvRow {
	t.Helper()
	rows, err := parseBankCSV(text)
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

func TestBankCSVFormats(t *testing.T) {
	cases := []struct {
		name, text string
		rows       int
		last       csvRow
		amount     string
	}{
		{"App-Format", "Datum;Betrag;Währung;Partner;IBAN;Verwendungszweck;Referenz\n2026-10-15;-20.00;EUR;Lieferant;DE456;RE-456;bank-2\n", 1,
			csvRow{date: "2026-10-15", currency: "EUR", partner: "Lieferant", iban: "DE456", purpose: "RE-456", reference: "bank-2"}, "-20"},
		{"Sparkasse", "\"Auftragskonto\";\"Buchungstag\";\"Valutadatum\";\"Buchungstext\";\"Verwendungszweck\";\"Glaeubiger ID\";\"Mandatsreferenz\";\"Kundenreferenz (End-to-End)\";\"Sammlerreferenz\";\"Lastschrift Ursprungsbetrag\";\"Auslagenersatz Ruecklastschrift\";\"Beguenstigter/Zahlungspflichtiger\";\"Kontonummer/IBAN\";\"BIC (SWIFT-Code)\";\"Betrag\";\"Waehrung\";\"Info\"\n" +
			"\"DE89370400440532013000\";\"30.10.26\";\"30.10.26\";\"GUTSCHR.\";\"RE-2026-0002\";\"\";\"\";\"E2E-1\";\"\";\"\";\"\";\"Stadt Muster\";\"DE75512108001245126199\";\"BYLADEM1\";\"4.104,31\";\"EUR\";\"Umsatz gebucht\"\n", 1,
			csvRow{date: "2026-10-30", currency: "EUR", partner: "Stadt Muster", iban: "DE75512108001245126199", purpose: "RE-2026-0002", reference: "E2E-1"}, "4104.31"},
		{"ING mit Vorspann und zwei Währungsspalten", "Umsätze Girokonto;Zeitraum: 01.10.2026 - 31.10.2026\n\nIBAN;DE89 3704 0044 0532 0130 00\nKontoname;Giro\n\nBuchung;Valuta;Auftraggeber/Empfänger;Buchungstext;Verwendungszweck;Saldo;Währung;Betrag;Währung\n" +
			"28.10.2026;28.10.2026;Cloudwerk GmbH;Lastschrift;CW-88213;11.240,00;EUR;-105,91;EUR\n", 1,
			csvRow{date: "2026-10-28", currency: "EUR", partner: "Cloudwerk GmbH", purpose: "CW-88213"}, "-105.91"},
		{"DKB mit getrennten Namen", "\"Girokonto\";\"DE89\"\n\"\";\"\"\n\"Buchungsdatum\";\"Wertstellung\";\"Status\";\"Zahlungspflichtige*r\";\"Zahlungsempfänger*in\";\"Verwendungszweck\";\"Umsatztyp\";\"IBAN\";\"Betrag (€)\";\"Gläubiger-ID\";\"Mandatsreferenz\";\"Kundenreferenz\"\n" +
			"\"30.10.26\";\"30.10.26\";\"Vorgemerkt\";\"Mara\";\"Laden\";\"Karte\";\"Ausgang\";\"DE1\";\"-5,00\";\"\";\"\";\"\"\n\"29.10.26\";\"29.10.26\";\"Gebucht\";\"Mara Brandt\";\"Druckerei Lindner\";\"Rechnung 1187\";\"Ausgang\";\"DE12\";\"-100,00\";\"\";\"\";\"\"\n", 1,
			csvRow{date: "2026-10-29", currency: "EUR", partner: "Druckerei Lindner", iban: "DE12", purpose: "Rechnung 1187"}, "-100"},
		{"Commerzbank nur Buchungstext", "Buchungstag;Wertstellung;Umsatzart;Buchungstext;Betrag;Währung;IBAN Kontoinhaber;Kategorie\n25.10.2026;25.10.2026;Entgelt;Kontoführung Oktober;-9,90;EUR;DE89370400440532013000;Bank\n", 1,
			csvRow{date: "2026-10-25", currency: "EUR", purpose: "Kontoführung Oktober"}, "-9.9"},
		{"Deutsche Bank Soll und Haben", "Umsätze;;;\nKonto;123;\n\nBuchungstag;Wert;Umsatzart;Begünstigter / Auftraggeber;Verwendungszweck;IBAN;BIC;Kundenreferenz;Mandatsreferenz;Gläubiger ID;Fremde Gebühren;Betrag;Abweichender Empfänger;Anzahl der Aufträge;Anzahl der Schecks;Soll;Haben;Währung\n" +
			"29.10.2026;29.10.2026;SEPA;Kunde;RE-1;DE2;;;;;;;;;;;1.500,00;EUR\n27.10.2026;27.10.2026;SEPA;Lieferant;RE-2;DE3;;;;;;;;;;-250,00;;EUR\nKontostand;;;;;;;;;;;;;;;;;\n", 2,
			csvRow{date: "2026-10-27", currency: "EUR", partner: "Lieferant", iban: "DE3", purpose: "RE-2"}, "-250"},
		{"Volksbank", "Bezeichnung Auftragskonto;IBAN Auftragskonto;BIC Auftragskonto;Bankname Auftragskonto;Buchungstag;Valutadatum;Name Zahlungsbeteiligter;IBAN Zahlungsbeteiligter;BIC (SWIFT-Code) Zahlungsbeteiligter;Buchungstext;Verwendungszweck;Betrag;Waehrung;Saldo nach Buchung;Bemerkung;Kategorie;Steuerrelevant;Glaeubiger ID;Mandatsreferenz\n" +
			"Giro;DE89;GENO;VR;26.10.2026;26.10.2026;Nordlicht;DK50;DABA;Gutschrift;Danke;900,00;EUR;12.000,00;;;;;\n", 1,
			csvRow{date: "2026-10-26", currency: "EUR", partner: "Nordlicht", iban: "DK50", purpose: "Danke"}, "900"},
		{"comdirect mit Fuß und offenem Umsatz", ";\n\"Umsätze Girokonto\";\"Zeitraum: 30 Tage\";\n\"Neuer Kontostand\";\"1.000,00 EUR\";\n\n\"Buchungstag\";\"Wertstellung (Valuta)\";\"Vorgang\";\"Buchungstext\";\"Umsatz in EUR\";\n\"offen\";\"31.10.2026\";\"Karte\";\"Laden\";\"-3,00\";\n\"24.10.2026\";\"24.10.2026\";\"Übertrag\";\"Auftraggeber: Mara Brandt Buchungstext: Privat\";\"-1.500,00\";\n\"Alter Kontostand\";\"2.500,00 EUR\";\n", 1,
			csvRow{date: "2026-10-24", currency: "EUR", purpose: "Auftraggeber: Mara Brandt Buchungstext: Privat"}, "-1500"},
		{"Komma als Trenner", "Date,Amount,Currency,Name,IBAN,Purpose\n2026-10-20,\"1,234.50\",EUR,Kunde,DE9,RE-9\n", 1,
			csvRow{date: "2026-10-20", currency: "EUR", partner: "Kunde", iban: "DE9", purpose: "RE-9"}, "1234.5"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rows := csvOne(t, c.text)
			if len(rows) != c.rows {
				t.Fatalf("got %d rows: %+v", len(rows), rows)
			}
			last := rows[len(rows)-1]
			if last.amount.String() != c.amount {
				t.Errorf("amount %s, want %s", last.amount, c.amount)
			}
			last.amount = c.last.amount
			if last != c.last {
				t.Errorf("got %+v, want %+v", last, c.last)
			}
		})
	}
}

func TestBankCSVExplainsWhatIsWrong(t *testing.T) {
	for text, want := range map[string]string{
		"Name;Wert\nA;1\n":                       "keine erkennbaren Spalten",
		"Buchungstag;Betrag\n32.13.2026;10,00\n": "kein lesbares Datum",
		"Buchungstag;Betrag\n12.10.2026;zehn\n":  "keinen lesbaren Betrag",
		"Buchungstag;Betrag\n":                   "keine gebuchten Umsätze",
	} {
		_, err := parseBankCSV(text)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: got %v, want %q", text, err, want)
		}
	}
}
