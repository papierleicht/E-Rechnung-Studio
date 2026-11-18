package umzug

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	erechnung "github.com/papierleicht/go-e-rechnung-validator"
	"golang.org/x/text/encoding/charmap"
	"papierleicht.de/e-rechnung-studio/internal/model"
	"papierleicht.de/e-rechnung-studio/internal/pdfinvoice"
)

func testXML(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "einvoice", "testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func zipOf(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for name, data := range files {
		f, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		f.Write(data)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func write(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestCollectWalksFoldersAndNestedArchives(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "export")
	write(t, filepath.Join(root, "2024", "RE-1.pdf"), []byte("%PDF-1"))
	write(t, filepath.Join(root, "2024", "RE-1.xml"), testXML(t, "cii.xml"))
	write(t, filepath.Join(root, ".DS_Store"), []byte("x"))
	write(t, filepath.Join(root, "liste.csv"), []byte("a;b"))
	write(t, filepath.Join(root, "leer.pdf"), nil)
	inner := zipOf(t, map[string][]byte{"belege/foto.jpg": []byte("jpg"), "__MACOSX/belege/._foto.jpg": []byte("x")})
	outer := zipOf(t, map[string][]byte{"rechnungen/RE-2.pdf": []byte("%PDF-2"), "belege.zip": inner, "../ausserhalb.pdf": []byte("%PDF-3")})
	write(t, filepath.Join(root, "steuerberater.zip"), outer)

	c, err := Collect([]string{root})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	var origins []string
	for _, e := range c.Entries {
		origins = append(origins, e.Origin)
	}
	want := []string{
		"export/2024/RE-1.pdf", "export/2024/RE-1.xml", "export/liste.csv",
		"export/steuerberater.zip/ausserhalb.pdf",
		"export/steuerberater.zip/belege.zip/belege/foto.jpg",
		"export/steuerberater.zip/rechnungen/RE-2.pdf",
	}
	if strings.Join(origins, "|") != strings.Join(want, "|") {
		t.Fatalf("entries\n%v\nwant\n%v", origins, want)
	}
	// Lists are collected; Analyze decides about them.
	if len(c.Skipped) != 1 {
		t.Fatalf("skipped %+v", c.Skipped)
	}
	data, err := c.Entries[4].Open()
	if err != nil || string(data) != "jpg" {
		t.Fatalf("nested file %q err=%v", data, err)
	}
	groups := Groups(c)
	if len(groups) != 4 || len(groups[0].Entries) != 2 || groups[0].Entries[0].Name != "RE-1.pdf" {
		t.Fatalf("groups %+v", groups)
	}
	p, err := Analyze(c)
	if err != nil || len(p.Groups) != 4 || len(p.Skipped) != 2 {
		t.Fatalf("plan %+v err=%v", p, err)
	}
}

func TestReadRecognizesEInvoicesAndKeepsPlainFiles(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "lieferant.xml"), testXML(t, "cii.xml"))
	write(t, filepath.Join(dir, "gutschrift.xml"), testXML(t, "ubl-creditnote.xml"))
	write(t, filepath.Join(dir, "scan.pdf"), []byte("%PDF-1.4 scan"))
	c, err := Collect([]string{dir})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	docs := map[string]model.LegacyDocument{}
	for _, g := range Groups(c) {
		d, err := Read(g, "andere", Own{Name: "Beispiel Kunde"})
		if err != nil {
			t.Fatal(err)
		}
		docs[g.Entries[0].Name] = d
	}
	invoice := docs["lieferant.xml"]
	if !invoice.EInvoice || invoice.Kind != "invoice" || invoice.Number == "" || invoice.Date == "" || invoice.Gross == "" {
		t.Fatalf("invoice %+v", invoice)
	}
	// The own company is the buyer of the test invoice: it was received.
	if invoice.Direction != "incoming" || invoice.Partner != "Beispiel Lieferant" {
		t.Fatalf("direction %q partner %q", invoice.Direction, invoice.Partner)
	}
	if docs["gutschrift.xml"].Kind != "credit-note" {
		t.Fatalf("credit note %+v", docs["gutschrift.xml"])
	}
	scan := docs["scan.pdf"]
	if scan.EInvoice || scan.Kind != "document" || scan.Number != "" || scan.SourceID == "" || string(scan.Files[0].Data) != "%PDF-1.4 scan" {
		t.Fatalf("scan %+v", scan)
	}
	// Without a match the direction stays open.
	g := Groups(c)
	for _, gr := range g {
		if gr.Entries[0].Name == "lieferant.xml" {
			d, _ := Read(gr, "andere", Own{Name: "Jemand anderes"})
			if d.Direction != "" {
				t.Fatalf("guessed direction %q", d.Direction)
			}
		}
	}
}

func TestReadFindsTheInvoiceInsideAPDF(t *testing.T) {
	pdf, err := pdfinvoice.Render(testXML(t, "cii.xml"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = erechnung.ExtractXML(pdf); err != nil {
		t.Skip("the preview PDF carries no XML")
	}
	dir := t.TempDir()
	write(t, filepath.Join(dir, "RE.pdf"), pdf)
	c, err := Collect([]string{dir})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	d, err := Read(Groups(c)[0], "andere", Own{VATID: "DE 123456789"})
	if err != nil || !d.EInvoice || d.Direction != "outgoing" {
		t.Fatalf("doc %+v err=%v", d, err)
	}
}

func TestReadTableEncodingsAndDelimiters(t *testing.T) {
	ansi, _ := charmap.Windows1252.NewEncoder().String("Firma;Straße;Ort\r\n\"Müller; Söhne\";Hauptstraße 1;Köln\r\n")
	utf16 := []byte{0xFF, 0xFE}
	for _, r := range "Name\tOrt\nA\tB\n" {
		utf16 = append(utf16, byte(r), 0)
	}
	cases := []struct {
		name, data, encoding, delimiter string
		columns                         int
		first                           string
	}{
		{"ansi", ansi, "Windows-1252 (ANSI)", "Semikolon", 3, "Müller; Söhne"},
		{"bom", "\xEF\xBB\xBFName,Ort\nA,B\n", "UTF-8", "Komma", 2, "A"},
		{"sep", "sep=|\nName|Ort\n\nA|B\n", "UTF-8", "senkrechter Strich", 2, "A"},
		{"utf16", string(utf16), "UTF-16", "Tabulator", 2, "A"},
	}
	for _, c := range cases {
		tab, err := ReadTable([]byte(c.data))
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if tab.Encoding != c.encoding || tab.Delimiter != c.delimiter || len(tab.Columns) != c.columns || tab.Rows[0][0] != c.first {
			t.Fatalf("%s: %+v", c.name, tab)
		}
	}
	if _, err := ReadTable([]byte("Name;Ort\n")); err == nil {
		t.Fatal("header only accepted")
	}
}

func TestCustomersFromAList(t *testing.T) {
	tab, err := ReadTable([]byte("Kundennr;Firma;Vorname;Nachname;Straße;Hausnr.;PLZ;Ort;Land;E-Mail;USt-IdNr\n" +
		"1;Beispiel GmbH;Erika;Muster;Hauptstr.;5;10115;Berlin;Deutschland;info@example.com;DE 123 456 789\n" +
		"2;;Max;Mustermann;Weg 1;;1010;Wien;Österreich;;\n" +
		"3;Schon Da AG;;;;;;;;;\n" +
		"4;;;;;;;;;;\n" +
		"5;Neu KG;;;;;;;Atlantis;;\n"))
	if err != nil {
		t.Fatal(err)
	}
	m := GuessMapping(ListCustomers, tab.Columns)
	if m["name"] != 1 || m["houseNumber"] != 5 || m["vatId"] != 10 {
		t.Fatalf("mapping %+v", m)
	}
	rows := Customers(tab, m, []model.Customer{{Name: "Schon Da AG"}})
	first := rows[0].Item
	if first.Name != "Beispiel GmbH" || first.ContactName != "Erika Muster" || first.Address.Street != "Hauptstr. 5" || first.Address.Country != "DE" || first.VATID != "DE123456789" {
		t.Fatalf("first %+v", first)
	}
	if rows[1].Item.Name != "Max Mustermann" || rows[1].Item.Address.Country != "AT" {
		t.Fatalf("person %+v", rows[1].Item)
	}
	if rows[2].Existing != "Schon Da AG" || rows[3].Error == "" || rows[4].Note == "" || rows[4].Item.Address.Country != "DE" {
		t.Fatalf("rows %+v", rows[2:])
	}
}

func TestProductsFromAList(t *testing.T) {
	tab, err := ReadTable([]byte("Art.-Nr.;Bezeichnung;Einheit;VK netto;MwSt\n" +
		"A1;Beratung;Std;1.234,50 €;19%\n" +
		"A2;Buch;Stück;12.50;7,00\n" +
		"A3;Spende;;5;0\n" +
		"A4;Kaputt;Stk;abc;19\n" +
		"A5;Ohne Satz;Kiste;3,00;\n"))
	if err != nil {
		t.Fatal(err)
	}
	m := GuessMapping(ListProducts, tab.Columns)
	rows := Products(tab, m, nil)
	if p := rows[0].Item; p.Unit != "HUR" || p.UnitPrice != "1234.5" || p.TaxRate != "19" || p.TaxCategory != "S" || p.ArticleNumber != "A1" {
		t.Fatalf("first %+v", p)
	}
	if p := rows[1].Item; p.Unit != "C62" || p.UnitPrice != "12.5" || p.TaxRate != "7" {
		t.Fatalf("second %+v", p)
	}
	if p := rows[2]; p.Item.TaxCategory != "E" || p.Note == "" {
		t.Fatalf("zero rate %+v", p)
	}
	if rows[3].Error == "" {
		t.Fatalf("bad price accepted %+v", rows[3])
	}
	if p := rows[4]; p.Item.Unit != "C62" || p.Item.TaxRate != "19" || !strings.Contains(p.Note, "Kiste") {
		t.Fatalf("assumptions %+v", p)
	}
}

func TestAmounts(t *testing.T) {
	for in, want := range map[string]string{"1.234,50": "1234.5", "1,234.50": "1234.5", "12.50": "12.5", "1.234": "1234", "EUR 3,00": "3", "": "0", "-2,5": "-2.5"} {
		v, ok := Amount(in)
		if !ok || v.String() != want {
			t.Fatalf("%q -> %s %v", in, v, ok)
		}
	}
	if _, ok := Amount("1,2,3"); ok {
		t.Fatal("ambiguous amount accepted")
	}
}

func TestEverySourceHasAGuide(t *testing.T) {
	sources, err := Sources()
	if err != nil {
		t.Fatal(err)
	}
	if len(sources) < 3 || sources[0].ID != "invoiz" || sources[len(sources)-1].ID != "andere" {
		t.Fatalf("order %+v", sources)
	}
	for _, s := range sources {
		if s.Name == "" || s.Summary == "" || s.Checked == "" || len(s.Ways) == 0 {
			t.Fatalf("%s: incomplete %+v", s.ID, s)
		}
		for _, heading := range []string{"## Vorher wissen", "## In papierleicht", "## Prüfen"} {
			if !strings.Contains(s.Guide, heading) {
				t.Fatalf("%s: guide lacks %q", s.ID, heading)
			}
		}
	}
	if _, ok := SourceByID("fakturama"); !ok {
		t.Fatal("fakturama missing")
	}
}
