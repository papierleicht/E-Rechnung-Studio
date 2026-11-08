package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"papierleicht.de/e-rechnung-studio/internal/model"
	"papierleicht.de/e-rechnung-studio/internal/pdfinvoice"
)

func TestHTMLPreviewSnapshotAndEscaping(t *testing.T) {
	s, err := NewInvoiceService(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	c := testCompany()
	if _, err = s.SaveCompany(c); err != nil {
		t.Fatal(err)
	}
	d := testDraft()
	d.Number = "HTML-TEST"
	d.Lines[0].Name = `<script>alert("x")</script>`
	html, err := s.InvoiceDocumentHTML(d)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(html, "<script>") || !strings.Contains(html, "&lt;script&gt;") {
		t.Fatal("invoice text was not escaped")
	}
	d.Lines[0].Name = "Unveränderlicher Artikel"
	issued, err := s.IssueInvoice(d)
	if err != nil {
		t.Fatal(err)
	}
	changed := issued.Draft
	changed.Status = "draft"
	changed.Number = "MUTATED"
	changed.Customer.Name = "MUTATED"
	html, err = s.InvoiceDocumentHTML(changed)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(html, "MUTATED") || !strings.Contains(html, "HTML-TEST") {
		t.Fatal("issued snapshot was changed")
	}
	empty := model.NewDraft()
	if _, err = s.InvoiceDocumentHTML(empty); err != nil {
		t.Fatal(err)
	}
	if empty.Number != "" || empty.Lines[0].Name != "" {
		t.Fatal("preview mutated draft")
	}
}

func TestHTMLPreviewComparisonFixtures(t *testing.T) {
	dir := os.Getenv("E_RECHNUNG_STUDIO_HTML_TEST_OUTPUT")
	if dir == "" {
		t.Skip("optional visual/performance fixtures")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	s, err := NewInvoiceService(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err = s.SaveCompany(testCompany()); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(dir, "fonts.css"), []byte(s.InvoicePreviewFonts()), 0600); err != nil {
		t.Fatal(err)
	}
	counts := []int{1, 40}
	if os.Getenv("E_RECHNUNG_STUDIO_HTML_PAGINATION_MATRIX") != "" {
		counts = []int{1, 5, 10, 15, 20, 25, 30, 40, 50, 60, 70, 80, 90, 100}
	}
	if os.Getenv("E_RECHNUNG_STUDIO_HTML_STRESS_MATRIX") != "" {
		counts = []int{3, 9, 18}
	}
	for _, count := range counts {
		d := testDraft()
		d.Number = "HTML-Vergleich"
		d.PaymentMeansCode = "58"
		line := d.Lines[0]
		d.Lines = nil
		for i := 0; i < count; i++ {
			l := line
			l.Name = fmt.Sprintf("Beratung %02d", i+1)
			l.Description = "Konzeption und Umsetzung\nMit vollständiger Dokumentation."
			if os.Getenv("E_RECHNUNG_STUDIO_HTML_STRESS_MATRIX") != "" {
				if count == 3 {
					l.Description = strings.Repeat("Längere Beschreibung mit mehreren Abschnitten. ", 18)
				}
				if count == 9 {
					l.Description = ""
				}
				if count == 18 {
					l.Description = strings.Repeat("Zusätzliche Leistung\n", 1+i%5)
				}
			}
			d.Lines = append(d.Lines, l)
		}
		if os.Getenv("E_RECHNUNG_STUDIO_HTML_STRESS_MATRIX") != "" && count == 3 {
			d.Notes = strings.Repeat("Ausführlicher Hinweis mit Zahlungsinformationen.\n", 30)
		}
		xml, d, c, err := s.previewInvoiceXML(d)
		if err != nil {
			t.Fatal(err)
		}
		var html string
		var pdf []byte
		for _, kind := range []string{"html", "pdf"} {
			start := time.Now()
			runs := 3
			if os.Getenv("E_RECHNUNG_STUDIO_HTML_PAGINATION_MATRIX") != "" {
				runs = 1
			}
			for i := 0; i < runs; i++ {
				if kind == "html" {
					html, err = pdfinvoice.HTML(xml, &c, &d.Customer, pdfinvoice.PaymentQROptions{Enabled: true})
				} else {
					pdf, err = pdfinvoice.RenderWithSnapshot(xml, &c, &d.Customer, pdfinvoice.PaymentQROptions{Enabled: true})
				}
				if err != nil {
					t.Fatal(err)
				}
			}
			t.Logf("%d lines %s average: %s", count, kind, time.Since(start)/time.Duration(runs))
		}
		stem := filepath.Join(dir, fmt.Sprintf("compact-%d", count))
		if err = os.WriteFile(stem+".html", []byte(html), 0600); err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(stem+".pdf", pdf, 0600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestLongPartyNamesKeepTheDocumentLayout(t *testing.T) {
	s, err := NewInvoiceService(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	render := func(seller, buyer string) string {
		t.Helper()
		c := testCompany()
		if seller != "" {
			c.Name = seller
		}
		if _, err := s.SaveCompany(c); err != nil {
			t.Fatal(err)
		}
		d := testDraft()
		if buyer != "" {
			d.Customer.Name = buyer
		}
		xml, d, c, err := s.previewInvoiceXML(d)
		if err != nil {
			t.Fatal(err)
		}
		pdf, err := pdfinvoice.RenderWithSnapshot(xml, &c, &d.Customer, pdfinvoice.PaymentQROptions{Enabled: true})
		if err != nil || !strings.HasPrefix(string(pdf), "%PDF") {
			t.Fatalf("PDF not rendered: %v", err)
		}
		html, err := pdfinvoice.HTML(xml, &c, &d.Customer, pdfinvoice.PaymentQROptions{Enabled: true})
		if err != nil {
			t.Fatal(err)
		}
		return html
	}

	// Normal names keep the single large heading.
	html := render("", "")
	if !strings.Contains(html, "<h1>Muster GmbH</h1><p class=\"sender\">") || strings.Contains(html, `class="seller-`) {
		t.Fatal("short seller name no longer set as before")
	}

	branch := "Zweigniederlassung Nordrhein-Westfalen und Rheinland-Pfalz Donaudampfschifffahrtsgesellschaftskapitänsmütze"
	buyer := "Café Morgenrot – " + branch
	html = render("Atelier Lindner – "+branch, buyer)
	if !strings.Contains(html, "<h1>Atelier Lindner</h1><p class=\"seller-subtitle\">"+branch+"</p>") {
		t.Fatal("long seller name not split into heading and subheading")
	}
	// The full legal name stays in the return address and the footer.
	if strings.Count(html, "Atelier Lindner – "+branch) < 2 || !strings.Contains(html, "<p class=\"recipient-name\">"+buyer+"</p>") {
		t.Fatal("full party names missing")
	}
	if !strings.Contains(html, `data-page-count="1"`) {
		t.Fatal("long names pushed a one-position invoice onto a second page")
	}

	long := "Muster GmbH Internationale Handelsgesellschaft mit beschränkter Haftung & Co. KG"
	html = render(long, "")
	if !strings.Contains(html, "<h1 class=\"seller-compact\">"+strings.ReplaceAll(long, "&", "&amp;")+"</h1>") {
		t.Fatal("long seller name without a break not set smaller")
	}
}
