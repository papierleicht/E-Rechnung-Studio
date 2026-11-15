package taxforms

import "testing"

func TestKnownMarginGoesToKz81(t *testing.T) {
	sales := []Sale{
		{Entry: Entry{ID: "used"}, Category: "E", Net: d("1190"), Margin: true, MarginKnown: true, MarginBase: d("400"), MarginTax: d("76")},
		{Entry: Entry{ID: "open"}, Category: "E", Net: d("500"), Margin: true},
	}
	r := BuildVATReturn(sales, nil, d("0"))
	if f, ok := field(r.Fields, "81"); !ok || f.Amount != "400" {
		t.Fatalf("Kz 81 = %+v", f)
	}
	if _, ok := field(r.Fields, "48"); ok {
		t.Error("purchase price share reported as tax-free sale")
	}
	codes := issueCodes(r.Issues)
	if codes["margin_scheme"] != 1 || len(r.Issues[0].Entries) != 1 || r.Issues[0].Entries[0].ID != "open" {
		t.Fatalf("issues %+v", r.Issues)
	}
}

func TestKnownMarginSplitsEUR(t *testing.T) {
	income := []Income{{Entry: Entry{ID: "used"}, Category: "E", Net: d("1190"), Margin: true, MarginKnown: true, MarginTax: d("76")}}
	r := BuildEUR(2026, false, income, nil, nil, nil, nil)
	if f, ok := field(r.Income, "112"); !ok || f.Amount != "1114.00" {
		t.Fatalf("Kz 112 = %+v", f)
	}
	if f, ok := field(r.Income, "140"); !ok || f.Amount != "76.00" {
		t.Fatalf("Kz 140 = %+v", f)
	}
	if len(r.Issues) != 0 {
		t.Fatalf("issues %+v", r.Issues)
	}
}
