package pdfinvoice

import (
	"bytes"
	"strings"
	"testing"
)

func TestRenderPlainWritesAMultiPageDocument(t *testing.T) {
	var body strings.Builder
	body.WriteString("<html><body><h1>Verfahrensdokumentation</h1><h2>1. Überblick</h2>")
	for i := 0; i < 80; i++ {
		body.WriteString("<p>Ein Absatz mit Umlauten: Prüfung, „Anführungszeichen“ und – Gedankenstrich.</p>")
	}
	body.WriteString("<table><thead><tr><th>Feld</th><th>Art</th></tr></thead><tbody><tr><td>Belegnummer</td><td>Text</td></tr></tbody></table>")
	body.WriteString("<ul><li>Erster Punkt</li><li>Zweiter Punkt</li></ul></body></html>")
	data, err := RenderPlain("Verfahrensdokumentation", "Atelier", body.String())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(data, []byte("%PDF")) {
		t.Fatalf("not a PDF: %.20q", data)
	}
	if pages := bytes.Count(data, []byte("/Type /Page\n")) + bytes.Count(data, []byte("/Type/Page\n")); pages == 1 {
		t.Fatalf("the text must flow over several pages")
	}
}
