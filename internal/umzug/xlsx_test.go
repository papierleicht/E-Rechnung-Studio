package umzug

import "testing"

func testWorkbook(t *testing.T) []byte {
	t.Helper()
	return zipOf(t, map[string][]byte{
		"[Content_Types].xml":        []byte(`<?xml version="1.0"?><Types/>`),
		"xl/workbook.xml":            []byte(`<?xml version="1.0" encoding="UTF-8"?><workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets><sheet name="Artikel" sheetId="1" r:id="rId7"/></sheets></workbook>`),
		"xl/_rels/workbook.xml.rels": []byte(`<?xml version="1.0"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId7" Type="worksheet" Target="worksheets/artikel.xml"/></Relationships>`),
		"xl/sharedStrings.xml":       []byte(`<?xml version="1.0"?><sst xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><si><t>Bezeichnung</t></si><si><t>VK netto</t></si><si><r><t>Bera</t></r><r><t>tung</t></r></si><si><t>Einheit</t></si><si><t>Std</t></si></sst>`),
		"xl/worksheets/artikel.xml": []byte(`<?xml version="1.0"?><worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData>
<row r="1"><c r="A1" t="s"><v>0</v></c><c r="B1" t="s"><v>1</v></c><c r="D1" t="s"><v>3</v></c></row>
<row r="2"><c r="A2" t="s"><v>2</v></c><c r="B2"><v>1234.5</v></c><c r="D2" t="s"><v>4</v></c></row>
<row r="3"><c r="A3" t="inlineStr"><is><t>Fachbuch</t></is></c><c r="B3"><v>24.899999999999999</v></c></row>
</sheetData></worksheet>`),
	})
}

func TestReadXLSX(t *testing.T) {
	data := testWorkbook(t)
	if !isXLSX(data) {
		t.Fatal("workbook not recognized")
	}
	tab, err := ReadList(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(tab.Columns) != 4 || tab.Columns[3] != "Einheit" || tab.Rows[0][0] != "Beratung" || tab.Rows[0][1] != "1234,5" || tab.Rows[1][1] != "24,9" {
		t.Fatalf("table %+v", tab)
	}
	rows := Products(tab, GuessMapping(ListProducts, tab.Columns), nil)
	if rows[0].Item.UnitPrice != "1234.5" || rows[0].Item.Unit != "HUR" || rows[1].Item.UnitPrice != "24.9" {
		t.Fatalf("products %+v", rows)
	}
	if isXLSX(zipOf(t, map[string][]byte{"a.txt": []byte("x")})) {
		t.Fatal("plain ZIP taken for a workbook")
	}
}
