package gdpdu

import (
	"encoding/xml"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func sample() []Table {
	return []Table{
		{File: "rechnungen.csv", Name: "Rechnungen", Description: "Ausgestellte Rechnungen", From: "2026-01-01", To: "2026-12-31", Keys: 1,
			Columns: []Column{{Name: "ID", Kind: Text}, {Name: "Datum", Kind: Date}, {Name: "Brutto", Kind: Number, Accuracy: 2}},
			Rows:    [][]string{{"re-1", Day("2026-03-01"), Amount("119.00")}, {`a"b` + "\n;", "", "1,00"}}},
		{File: "positionen.csv", Name: "Positionen", Keys: 2,
			Columns: []Column{{Name: "Rechnung", Kind: Text}, {Name: "Position", Kind: Number}, {Name: "Text", Kind: Text}},
			Foreign: []ForeignKey{{Column: "Rechnung", Table: "Rechnungen", Target: "ID"}},
			Rows:    [][]string{{"re-1", "1", "Beratung"}}},
	}
}

func TestCSVFollowsTheStandard(t *testing.T) {
	got := string(sample()[0].CSV())
	want := "\"ID\";\"Datum\";\"Brutto\"\r\n\"re-1\";01.03.2026;119,00\r\n\"a'b ;\";;1,00\r\n"
	if got != want {
		t.Fatalf("got %q", got)
	}
}

func TestNumbersCarryTheDeclaredDecimalPlaces(t *testing.T) {
	table := Table{Columns: []Column{{Name: "A", Kind: Number, Accuracy: 2}, {Name: "B", Kind: Number, Accuracy: 6}, {Name: "C", Kind: Number}},
		Rows: [][]string{{"200", "108,123", "7"}, {"", "1,5", ""}, {"1,25", "0,123456", "1"}}}
	want := "\"A\";\"B\";\"C\"\r\n200,00;108,123000;7\r\n;1,500000;\r\n1,25;0,123456;1\r\n"
	if got := string(table.CSV()); got != want {
		t.Fatalf("got %q", got)
	}
	if shown := table.Filled(); shown[0][0] != "200,00" || shown[1][0] != "" || table.Rows[0][0] != "200" {
		t.Fatalf("%v", shown)
	}
}

func TestIndexIsValidAgainstTheDTD(t *testing.T) {
	raw, err := Index(Supplier{"Atelier", "Leipzig", "Prüfer-Export"}, "Export 1", sample())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `<!DOCTYPE DataSet SYSTEM "gdpdu-01-09-2004.dtd">`) {
		t.Fatal(string(raw))
	}
	if err = xml.Unmarshal(raw, new(struct{})); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"<VariablePrimaryKey>", "<UTF8></UTF8>", "<From>2</From>", "<References>Rechnungen</References>", "<To>ID</To>", "<Accuracy>2</Accuracy>", "<Format>DD.MM.YYYY</Format>"} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("missing %s in\n%s", want, raw)
		}
	}
	xmllint, err := exec.LookPath("xmllint")
	if err != nil {
		t.Skip("xmllint is not installed")
	}
	dir := t.TempDir()
	if err = os.WriteFile(filepath.Join(dir, "index.xml"), raw, 0o600); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(dir, DTDName), DTD, 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(xmllint, "--noout", "--valid", "index.xml")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s\n%s", err, out, raw)
	}
}
