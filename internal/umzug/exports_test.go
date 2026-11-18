package umzug

import (
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/text/encoding/charmap"
	"papierleicht.de/e-rechnung-studio/internal/datev"
	"papierleicht.de/e-rechnung-studio/internal/gdpdu"
	"papierleicht.de/e-rechnung-studio/internal/model"
)

const guid1 = "6a1b2c3d-1111-2222-3333-444455556666"

func bookingsFile() []byte {
	header := `"EXTF";700;21;"Buchungsstapel";13;20260105120000000;;"RE";"";"";1001;12345;20250101;4;20250101;20251231;"Buchungen";"";1;0;0;"EUR"`
	row := make([]string, len(datev.Columns))
	row[0], row[1], row[6], row[7], row[9], row[10], row[13], row[19] = "119,00", `"S"`, "10001", "8400", "1403", `"RE-2025-7"`, `"Rechnung Beispiel"`, `"BEDI ""`+strings.ToUpper(guid1)+`"""`
	cols := make([]string, len(datev.Columns))
	for i, c := range datev.Columns {
		cols[i] = `"` + c + `"`
	}
	text := header + "\r\n" + strings.Join(cols, ";") + "\r\n" + strings.Join(row, ";") + "\r\n"
	out, _ := charmap.Windows1252.NewEncoder().String(text)
	return []byte(out)
}

func datevPartnersFile() []byte {
	text := `"EXTF";700;16;"Debitoren/Kreditoren";5;20260105120000000;;"RE";"";"";1001;12345;20250101;4;;;"";"";;;;""` + "\r\n" +
		`"Konto";"Name (Adressatentyp Unternehmen)";"Name (Adressatentyp natürl. Person)";"Vorname (Adressatentyp natürl. Person)";"Adressatentyp";"EU-Land";"EU-UStID";"Straße";"Postleitzahl";"Ort";"Land";"E-Mail"` + "\r\n" +
		`10001;"Beispiel Kunde GmbH";"";"";"2";"DE";"123456789";"Hauptstraße 1";"10115";"Berlin";"DE";"info@kunde.example"` + "\r\n" +
		`10002;"";"Muster";"Erika";"1";"";"";"Weg 2";"80331";"München";"DE";""` + "\r\n" +
		`70001;"Lieferant AG";"";"";"2";"";"";"";"";"";"DE";""` + "\r\n"
	out, _ := charmap.Windows1252.NewEncoder().String(text)
	return []byte(out)
}

const documentXML = `<?xml version="1.0" encoding="utf-8"?>
<archive version="5.0" xmlns="http://xml.datev.de/bedi/tps/document/v05.0" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
  <content>
    <document guid="` + guid1 + `">
      <extension xsi:type="File" name="beleg-7.pdf"/>
    </document>
    <document guid="7a1b2c3d-1111-2222-3333-444455556666">
      <extension xsi:type="Invoice" datafile="rechnung-8.xml"><property key="InvoiceType" value="Incoming"/></extension>
      <extension xsi:type="File" name="beleg-8.pdf"/>
    </document>
  </content>
</archive>`

const ledgerXML = `<?xml version="1.0" encoding="utf-8"?>
<LedgerImport xmlns="http://xml.datev.de/bedi/tps/ledger/v040" version="4.0">
  <consolidate consolidatedAmount="59.50" consolidatedDate="2025-06-02" consolidatedInvoiceId="ER-88" consolidatedCurrencyCode="EUR">
    <accountsPayableLedger>
      <date>2025-06-02</date>
      <amount>59.50</amount>
      <invoiceId>ER-88</invoiceId>
      <supplierName>Papierhandel Süd</supplierName>
    </accountsPayableLedger>
  </consolidate>
</LedgerImport>`

func TestDatevExportWithDocumentPackage(t *testing.T) {
	dir := t.TempDir()
	belege := zipOf(t, map[string][]byte{"document.xml": []byte(documentXML), "rechnung-8.xml": []byte(ledgerXML), "beleg-7.pdf": []byte("%PDF sieben"), "beleg-8.pdf": []byte("%PDF acht")})
	write(t, filepath.Join(dir, "sevdesk-export.zip"), zipOf(t, map[string][]byte{
		"EXTF_Buchungsstapel_2025.csv":    bookingsFile(),
		"EXTF_GP_Stamm_2025.csv":          datevPartnersFile(),
		"DATEV_Rechnungsausgang_2025.zip": belege,
		"liste.csv":                       []byte("a;b\n1;2\n"),
	}))
	c, err := Collect([]string{dir})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	p, err := Analyze(c)
	if err != nil {
		t.Fatal(err)
	}
	docs := map[string]model.LegacyDocument{}
	for _, g := range p.Groups {
		d, err := Read(g, "sevdesk", Own{})
		if err != nil {
			t.Fatal(err)
		}
		docs[d.Files[0].Name] = d
	}
	seven := docs["beleg-7.pdf"]
	if seven.Basis != "datev" || seven.Number != "RE-2025-7" || seven.Date != "2025-03-14" || seven.Gross != "119.00" || seven.Direction != "outgoing" || seven.Partner != "Beispiel Kunde GmbH" {
		t.Fatalf("booking link %+v", seven)
	}
	eight := docs["beleg-8.pdf"]
	if eight.Number != "ER-88" || eight.Date != "2025-06-02" || eight.Gross != "59.50" || eight.Direction != "incoming" || eight.Partner != "Papierhandel Süd" {
		t.Fatalf("ledger %+v", eight)
	}
	var exports []model.LegacyDocument
	for _, d := range docs {
		if d.Kind == "export" {
			exports = append(exports, d)
		}
	}
	if len(exports) != 2 {
		t.Fatalf("exports %d: %+v", len(exports), docs)
	}
	for _, e := range exports {
		if strings.HasPrefix(e.Number, "DATEV-Export") && (len(e.Files) != 2 || e.Date != "2025-12-31") {
			t.Fatalf("datev export %+v", e)
		}
		if e.Number == "DATEV-Belegpaket" && len(e.Files) != 2 {
			t.Fatalf("package %+v", e)
		}
	}
	if len(p.Tables) != 1 || p.Tables[0].Format != "datev" || p.Tables[0].Rows != 2 {
		t.Fatalf("tables %+v", p.Tables)
	}
	if len(p.Skipped) != 1 || !strings.HasSuffix(p.Skipped[0].Origin, "liste.csv") {
		t.Fatalf("skipped %+v", p.Skipped)
	}
	table, err := LoadExportTable(c, p.Tables[0])
	if err != nil {
		t.Fatal(err)
	}
	rows := Customers(table, GuessMapping(ListCustomers, table.Columns), nil)
	if len(rows) != 3 || rows[0].Item.Name != "Beispiel Kunde GmbH" || rows[0].Item.VATID != "DE123456789" || rows[1].Item.Name != "Erika Muster" || rows[1].Item.Address.City != "München" || rows[2].Error == "" {
		t.Fatalf("customers %+v", rows)
	}
}

func TestGdpduExport(t *testing.T) {
	customers := gdpdu.Table{File: "kunden.csv", Name: "Kunden", Description: "Debitoren", Columns: []gdpdu.Column{
		{Name: "Kundennummer", Kind: gdpdu.Text}, {Name: "Firma", Kind: gdpdu.Text}, {Name: "Straße", Kind: gdpdu.Text},
		{Name: "PLZ", Kind: gdpdu.Text}, {Name: "Ort", Kind: gdpdu.Text},
	}, Rows: [][]string{{"1", "Müller GmbH", "Hauptstr. 1", "10115", "Berlin"}, {"2", "Schmidt KG", "Weg 3", "50667", "Köln"}}}
	journal := gdpdu.Table{File: "journal.csv", Name: "Journal", Columns: []gdpdu.Column{{Name: "Datum", Kind: gdpdu.Date}, {Name: "Betrag", Kind: gdpdu.Number, Accuracy: 2}}, Rows: [][]string{{"01.02.2025", "10,00"}}}
	index, err := gdpdu.Index(gdpdu.Supplier{Name: "Test"}, "Jahr 2025", []gdpdu.Table{customers, journal})
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "z3")
	write(t, filepath.Join(dir, "index.xml"), index)
	write(t, filepath.Join(dir, gdpdu.DTDName), gdpdu.DTD)
	write(t, filepath.Join(dir, "kunden.csv"), customers.CSV())
	write(t, filepath.Join(dir, "journal.csv"), journal.CSV())
	write(t, filepath.Join(dir, "Belege", "RE-1.pdf"), []byte("%PDF eins"))
	c, err := Collect([]string{dir})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	p, err := Analyze(c)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Groups) != 2 || len(p.Skipped) != 0 {
		t.Fatalf("groups %+v skipped %+v", p.Groups, p.Skipped)
	}
	var export model.LegacyDocument
	for _, g := range p.Groups {
		if d, _ := Read(g, "lexware-office", Own{}); d.Kind == "export" {
			export = d
		}
	}
	if export.Number != "Prüferexport (GDPdU) Jahr 2025" || len(export.Files) != 4 {
		t.Fatalf("export %+v", export)
	}
	if len(p.Tables) != 1 || p.Tables[0].Rows != 2 || p.Tables[0].Table != "Kunden" {
		t.Fatalf("tables %+v", p.Tables)
	}
	table, err := LoadExportTable(c, p.Tables[0])
	if err != nil {
		t.Fatal(err)
	}
	rows := Customers(table, GuessMapping(ListCustomers, table.Columns), nil)
	if len(rows) != 2 || rows[0].Item.Name != "Müller GmbH" || rows[1].Item.Address.City != "Köln" {
		t.Fatalf("customers %+v", rows)
	}
}

func TestGdpduEncodingsAndHeaderlessTables(t *testing.T) {
	index := `<?xml version="1.0" encoding="ISO-8859-1"?><!DOCTYPE DataSet SYSTEM "gdpdu-01-08-2002.dtd">
<DataSet><Version>1.0</Version><Media><Name>Export</Name><Table><URL>Adressen.txt</URL><Name>Adressen</Name><ANSI/><Range><From>1</From></Range>
<VariableLength><ColumnDelimiter>,</ColumnDelimiter><TextEncapsulator>"</TextEncapsulator>
<VariablePrimaryKey><Name>Nr</Name><AlphaNumeric/></VariablePrimaryKey>
<VariableColumn><Name>Name</Name><AlphaNumeric/></VariableColumn><VariableColumn><Name>PLZ</Name><AlphaNumeric/></VariableColumn><VariableColumn><Name>Ort</Name><AlphaNumeric/></VariableColumn>
</VariableLength></Table></Media></DataSet>`
	idx, err := readIndex([]byte(index))
	if err != nil || len(idx.Tables) != 1 || idx.Tables[0].Delimiter != "," || len(idx.Tables[0].Columns) != 4 {
		t.Fatalf("index %+v err=%v", idx, err)
	}
	data, _ := charmap.Windows1252.NewEncoder().String("1,\"Bäcker Größe\",10115,Berlin\r\n2,Zweite,80331,München\r\n")
	table, err := readGdpduTable(idx.Tables[0], []byte(data))
	if err != nil || len(table.Rows) != 2 || table.Rows[0][1] != "Bäcker Größe" || table.Rows[1][3] != "München" {
		t.Fatalf("table %+v err=%v", table, err)
	}
}

func TestListsInsideAnExport(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "backup")
	write(t, filepath.Join(dir, "clients.csv"), []byte("Kundennummer;Firma;Straße;PLZ;Ort;E-Mail\nK-1;Muster GmbH;Hauptstr. 1;10115;Berlin;a@muster.example\n"))
	write(t, filepath.Join(dir, "articles.xlsx"), testWorkbook(t))
	// An invoice list names the customer's address too, but it is no customer list.
	write(t, filepath.Join(dir, "invoices.csv"), []byte("Rechnungsnummer;Datum;Firma;Straße;PLZ;Ort;Brutto\nRE-1;01.02.2025;Muster GmbH;Hauptstr. 1;10115;Berlin;119,00\n"))
	write(t, filepath.Join(dir, "settings.csv"), []byte("Schlüssel;Wert\nwaehrung;EUR\n"))
	write(t, filepath.Join(dir, "pdf", "RE-1.pdf"), []byte("%PDF eins"))
	c, err := Collect([]string{dir})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	p, err := Analyze(c)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Tables) != 2 || len(p.Groups) != 1 || len(p.Skipped) != 2 {
		t.Fatalf("tables %+v groups %d skipped %+v", p.Tables, len(p.Groups), p.Skipped)
	}
	byList := map[string]ExportTable{}
	for _, table := range p.Tables {
		byList[table.List] = table
	}
	customers, products := byList[ListCustomers], byList[ListProducts]
	if customers.Format != "list" || customers.Label != "clients" || customers.Rows != 1 || products.Label != "articles" || products.Rows != 2 {
		t.Fatalf("tables %+v", p.Tables)
	}
	table, err := LoadExportTable(c, products)
	if err != nil || len(table.Rows) != 2 || table.Rows[0][0] != "Beratung" {
		t.Fatalf("products %+v err=%v", table, err)
	}
	if table, err = LoadExportTable(c, customers); err != nil || table.Rows[0][1] != "Muster GmbH" {
		t.Fatalf("customers %+v err=%v", table, err)
	}
}

func TestListKind(t *testing.T) {
	for _, tc := range []struct {
		columns []string
		want    string
	}{
		{[]string{"Firma", "PLZ", "Ort"}, ListCustomers},
		{[]string{"Name", "E-Mail"}, ""},
		{[]string{"Vorname", "Nachname", "Name", "E-Mail"}, ListCustomers},
		{[]string{"Bezeichnung", "Preis", "Einheit"}, ListProducts},
		{[]string{"Bezeichnung", "Einheit"}, ""},
		{[]string{"Belegnummer", "Firma", "PLZ", "Ort"}, ""},
	} {
		if got := ListKind(tc.columns); got != tc.want {
			t.Errorf("%v: %q, want %q", tc.columns, got, tc.want)
		}
	}
}
