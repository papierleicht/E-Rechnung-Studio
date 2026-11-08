package pdfinvoice

import (
	"github.com/shopspring/decimal"
	"papierleicht.de/e-rechnung-studio/internal/einvoice"
	"strings"
	"testing"
)

func TestMarginNoticeBelongsToDescriptionNotTaxColumn(t *testing.T) {
	for _, note := range []string{"Gebrauchtgegenstände/Sonderregelung", "Kunstgegenstände/Sonderregelung", "Sammlungsstücke und Antiquitäten/Sonderregelung"} {
		rows := layoutRows([]einvoice.InvoiceLine{
			{ItemName: "Normal", TaxCategoryCode: "S", TaxRateApplicablePercent: decimal.NewFromInt(19)},
			{ItemName: "Sonderregelung", TaxCategoryCode: "E", Note: note},
			{ItemName: "Nullsteuersatz", TaxCategoryCode: "Z"},
		})
		if rows[0].TaxLabel != "19 %" || rows[1].TaxLabel != "—" || rows[1].Details != note || rows[2].TaxLabel != "0 %" {
			t.Fatalf("incorrect tax presentation: %+v", rows)
		}
	}
	legacy := layoutRows([]einvoice.InvoiceLine{{ItemName: "Altbestand", TaxCategoryCode: "E"}}, true)
	if legacy[0].TaxLabel != "—" || !strings.Contains(legacy[0].Details, "Differenzbesteuerung") {
		t.Fatalf("legacy margin position unlabelled: %+v", legacy)
	}
}

func TestLongRowsPreserveEveryCharacter(t *testing.T) {
	name := strings.Repeat("Lange Bezeichnung äöü ", 30)
	description := strings.Repeat("Details\nmit mehreren Zeilen und Umlauten. ", 400)
	rows := layoutRows([]einvoice.InvoiceLine{{LineID: "1", ItemName: name, Description: description}})
	var gotName, gotDescription strings.Builder
	for i, row := range rows {
		gotName.WriteString(row.Heading)
		gotDescription.WriteString(row.Details)
		if i > 0 && !row.Continuation {
			t.Fatal("continuation repeats monetary values")
		}
		if len([]rune(row.Heading)) > 180 || len([]rune(row.Details)) > 300 {
			t.Fatal("unbounded table row")
		}
	}
	if gotName.String() != name || gotDescription.String() != description {
		t.Fatal("text lost during pagination")
	}
}

func TestBasicLineNoteIsVisibleWithoutLosingText(t *testing.T) {
	note := strings.Repeat("Vollständige Positionsnotiz mit Umlauten.\n", 80)
	rows := layoutRows([]einvoice.InvoiceLine{{LineID: "1", ItemName: "Beratung", Note: note}})
	var text strings.Builder
	for _, row := range rows {
		text.WriteString(row.Details)
	}
	if text.String() != note {
		t.Fatal("BASIC position note missing from PDF rows")
	}
}

func TestNotePaginationPreservesTextAndBoundsNewlines(t *testing.T) {
	note := strings.Repeat("Hinweis\n", 100) + "ENDE"
	inv := &einvoice.Invoice{PaymentReference: note}
	var got strings.Builder
	for _, group := range bottomGroups(inv) {
		for _, row := range group {
			got.WriteString(row.Text)
			if strings.Count(row.Text, "\n") > 12 || len([]rune(row.Text)) > 240 {
				t.Fatal("note row can exceed page height")
			}
		}
	}
	if got.String() != note {
		t.Fatal("payment note lost during pagination")
	}
}

func TestAdvanceIsNamedOnTheDocument(t *testing.T) {
	cases := map[string]*einvoice.Invoice{
		"Factur-X":  {InvoiceTypeCode: 386},
		"XRechnung": {InvoiceTypeCode: 380, Notes: []einvoice.Note{{Text: "Anzahlungsrechnung"}}},
	}
	for name, inv := range cases {
		if !isAdvance(inv) {
			t.Errorf("%s: not recognised", name)
		}
	}
	if isAdvance(&einvoice.Invoice{InvoiceTypeCode: 380, Notes: []einvoice.Note{{Text: "Zahlbar sofort"}}}) {
		t.Error("ordinary invoice taken for an advance")
	}
}

func TestGermanMoneyGroupsThousands(t *testing.T) {
	for in, want := range map[string]string{"3400": "3.400,00", "1234567.5": "1.234.567,50", "999.99": "999,99", "-1000": "-1.000,00"} {
		if got := germanMoney(decimal.RequireFromString(in)); got != want {
			t.Errorf("%s: %s, want %s", in, got, want)
		}
	}
}
