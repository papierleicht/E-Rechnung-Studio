package pdfinvoice

import (
	"bytes"
	"fmt"

	pdf "github.com/boxesandglue/baseline-pdf"
	"github.com/boxesandglue/boxesandglue/backend/document"
	"github.com/boxesandglue/boxesandglue/frontend"
	"github.com/boxesandglue/htmlbag"
)

const plainCSS = `
body { font-size: 10pt; line-height: 1.5; }
h1 { font-size: 22pt; margin: 0 0 3mm; }
h2 { font-size: 14pt; margin: 7mm 0 2mm; }
h3 { font-family: sans; font-size: 10pt; font-weight: bold; margin: 4mm 0 1mm; }
p { margin: 0 0 2mm; }
li { margin: 0 0 1mm; }
table { margin: 2mm 0 4mm; }
th { font-size: 8pt; text-align: left; color: #586b63; padding: 1mm 2mm 1mm 0; border-bottom: 0.3mm solid #293f3a; }
td { font-size: 8.5pt; padding: 1mm 2mm 1mm 0; border-bottom: 0.2mm solid #d2dbd1; }
`

// RenderPlain sets a text document in the look of the invoices: headings,
// paragraphs, lists and tables, as an archivable PDF/A.
func RenderPlain(title, author, html string) ([]byte, error) {
	renderMu.Lock()
	defer renderMu.Unlock()
	var out bytes.Buffer
	fe, err := frontend.NewForWriter(&out)
	if err != nil {
		return nil, err
	}
	var missingGlyph rune
	fe.MissingGlyphFunc = func(_ *pdf.Face, r rune) {
		if missingGlyph == 0 {
			missingGlyph = r
		}
	}
	fe.Doc.Format = document.FormatPDFA3b
	fe.Doc.Title, fe.Doc.Author, fe.Doc.Creator = title, author, creatorName
	if _, err = fe.Doc.LoadSRGBColorprofile(); err != nil {
		return nil, err
	}
	cb, err := htmlbag.New(fe, htmlbag.NewCSSParserWithDefaults())
	if err != nil {
		return nil, err
	}
	if err = cb.AddCSS(invoiceCSS + pdfCSS + plainCSS); err != nil {
		return nil, err
	}
	if err = cb.InitPage(); err != nil {
		return nil, err
	}
	text, err := cb.HTMLToText(html)
	if err != nil {
		return nil, err
	}
	if err = cb.OutputPagesFromText(text); err != nil {
		return nil, err
	}
	if missingGlyph != 0 {
		return nil, fmt.Errorf("Das Zeichen %q kann in der PDF-Schrift nicht dargestellt werden.", missingGlyph)
	}
	if err = fe.Finish(); err != nil {
		return nil, err
	}
	if err = fe.Doc.PDFWriter.FinishAndClose(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}
