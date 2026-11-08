package app

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"papierleicht.de/e-rechnung-studio/internal/model"
	"papierleicht.de/e-rechnung-studio/internal/pdfinvoice"
)

// A 4:1 brand mark with transparent padding, as typically exported by design tools.
func testLogoUpload(t *testing.T) string {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 1400, 500))
	for y := 50; y < 450; y++ {
		for x := 100; x < 1300; x++ {
			c := color.NRGBA{R: 57, G: 126, B: 107, A: 255}
			if x > 500 && (y/40)%2 == 0 {
				c = color.NRGBA{R: 41, G: 63, B: 58, A: 255}
			}
			img.SetNRGBA(x, y, c)
		}
	}
	var out bytes.Buffer
	if err := png.Encode(&out, img); err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(out.Bytes())
}

func TestCompanyLogoOnPreviewPDFAndIssuedSnapshot(t *testing.T) {
	s, err := NewInvoiceService(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err = s.PrepareCompanyLogo(base64.StdEncoding.EncodeToString([]byte("kein Bild"))); err == nil {
		t.Fatal("non-image logo accepted")
	}
	upload, err := s.PrepareCompanyLogo(testLogoUpload(t))
	if err != nil {
		t.Fatal(err)
	}
	// Padding is trimmed: 1200×400 content scaled into 900×360 storage bounds.
	if upload.Width != 900 || upload.Height != 300 || upload.Warning != "" {
		t.Fatalf("unexpected normalized logo: %d×%d %q", upload.Width, upload.Height, upload.Warning)
	}
	settings := model.AppSettings{ExportDirectory: t.TempDir()}
	c := testCompany()
	invalid := c
	invalid.Logo = base64.StdEncoding.EncodeToString([]byte("beschädigt"))
	if _, err = s.SaveSettings(settings, &invalid); err == nil {
		t.Fatal("corrupt logo saved")
	}
	c.Logo = upload.Logo
	if _, err = s.SaveSettings(settings, &c); err != nil {
		t.Fatal(err)
	}

	d := testDraft()
	html, err := s.InvoiceDocumentHTML(d)
	if err != nil {
		t.Fatal(err)
	}
	// 3:1 fits the 45×20 mm box at its full width.
	if !strings.Contains(html, `class="invoice-logo"`) || !strings.Contains(html, `src="data:image/png;base64,`+upload.Logo+`"`) || !strings.Contains(html, "width:45.00mm;height:15.00mm") {
		t.Fatal("draft preview does not show the company logo")
	}
	if dir := os.Getenv("E_RECHNUNG_STUDIO_LOGO_TEST_OUTPUT"); dir != "" {
		xml, previewDraft, company, err := s.previewInvoiceXML(d)
		if err != nil {
			t.Fatal(err)
		}
		pdf, err := pdfinvoice.RenderWithSnapshot(xml, &company, &previewDraft.Customer, pdfinvoice.PaymentQROptions{Enabled: true})
		if err != nil {
			t.Fatal(err)
		}
		html, err := pdfinvoice.HTML(xml, &company, &previewDraft.Customer, pdfinvoice.PaymentQROptions{Enabled: true})
		if err != nil {
			t.Fatal(err)
		}
		for name, data := range map[string][]byte{"logo.pdf": pdf, "logo.html": []byte(html), "fonts.css": []byte(s.InvoicePreviewFonts())} {
			if err = os.WriteFile(filepath.Join(dir, name), data, 0600); err != nil {
				t.Fatal(err)
			}
		}
	}

	issued, err := s.IssueInvoice(d)
	if err != nil {
		t.Fatal(err)
	}
	if issued.Draft.Profile != "facturx-en16931" || issued.Draft.IssuerSnapshot == nil || issued.Draft.IssuerSnapshot.Logo != upload.Logo {
		t.Fatal("issued invoice did not keep the logo snapshot")
	}
	pdf, err := s.invoicePDF(issued.Draft.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(pdf, []byte("/Subtype/Image")) && !bytes.Contains(pdf, []byte("/Subtype /Image")) {
		t.Fatal("issued PDF has no embedded logo image")
	}

	// Removing the logo later affects new drafts only, never issued invoices.
	c.Logo = ""
	if _, err = s.SaveSettings(settings, &c); err != nil {
		t.Fatal(err)
	}
	html, err = s.InvoiceDocumentHTML(issued.Draft)
	if err != nil || !strings.Contains(html, upload.Logo) {
		t.Fatalf("issued preview lost its logo: %v", err)
	}
	next := testDraft()
	next.Number = "TEST-2026-002"
	html, err = s.InvoiceDocumentHTML(next)
	if err != nil || strings.Contains(html, `class="invoice-logo"`) {
		t.Fatalf("removed logo still shown on new draft: %v", err)
	}

	// Received/foreign XML is rendered without an issuer and never gets our logo.
	xml, err := s.store.Artifact(issued.Draft.ID, "cii-xml")
	if err != nil {
		t.Fatal(err)
	}
	foreign, err := pdfinvoice.Render(xml)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(foreign, []byte("/Subtype/Image")) || bytes.Contains(foreign, []byte("/Subtype /Image")) {
		t.Fatal("logo rendered without an issuer snapshot")
	}
}
