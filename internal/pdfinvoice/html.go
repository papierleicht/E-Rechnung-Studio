package pdfinvoice

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"html/template"
	"os"
	"strings"

	"github.com/boxesandglue/htmlbag/fonts/crimsonprobold"
	"github.com/boxesandglue/htmlbag/fonts/crimsonproregular"
	"github.com/boxesandglue/htmlbag/fonts/texgyreherosbold"
	"github.com/boxesandglue/htmlbag/fonts/texgyreherosregular"
	"papierleicht.de/e-rechnung-studio/internal/model"
)

// HTML uses the exact escaped template, XML amounts, snapshot enrichment and
// layout styles used by the PDF exporter. A typesetting-only pass supplies the
// authoritative page assignment without finalizing or parsing a PDF file.
func HTML(xml []byte, issuer *model.Company, buyer *model.Customer, paymentQR PaymentQROptions) (string, error) {
	return documentHTML(xml, issuer, buyer, paymentQR, nil)
}

func documentHTML(xml []byte, issuer *model.Company, buyer *model.Customer, paymentQR PaymentQROptions, quote *QuoteDetails) (string, error) {
	inv, err := presentationInvoice(xml, issuer, buyer)
	if err != nil {
		return "", err
	}
	var plan pagePlan
	if _, err = renderWithPlan(xml, issuer, buyer, &plan, paymentQR, quote); err != nil {
		return "", err
	}
	var qrSource template.HTMLAttr
	if paymentQR.Enabled {
		path, err := paymentQRImage(inv)
		if err != nil {
			return "", err
		}
		if path != "" {
			defer os.Remove(path)
			data, err := os.ReadFile(path)
			if err != nil {
				return "", err
			}
			qrSource = template.HTMLAttr(`src="data:image/png;base64,` + base64.StdEncoding.EncodeToString(data) + `"`)
		}
	}
	logo, err := issuerLogo(issuer)
	if err != nil {
		return "", err
	}
	var logoSource template.HTMLAttr
	if logo != nil {
		logoSource = template.HTMLAttr(`src="data:image/png;base64,` + logo.Encode() + `"`)
	}
	var out bytes.Buffer
	margin := isMarginSchemeInvoice(inv)
	err = invoiceTemplate.Execute(&out, invoiceTemplateData{
		Invoice: inv, Rows: layoutRows(inv.InvoiceLines, margin), BottomGroups: bottomGroups(inv, margin),
		MarginScheme: margin, PaymentQR: qrSource, HidePaymentQRDetails: paymentQR.HideDetails,
		Logo: logoAttributes(logoSource, logo), Quote: quote,
	})
	if err != nil {
		return "", err
	}
	// Only renderer-owned CSS is inserted as markup. All invoice fields continue
	// through html/template's escaping. No user HTML is accepted.
	head := "<head><meta charset=\"utf-8\"><style>" + invoiceCSS + "</style></head>"
	return flowDocument(strings.Replace(out.String(), "<body>", head+"<body>", 1), &plan)
}

// Loaded once by the frontend; identical font bytes to htmlbag's PDF defaults.
func PreviewFonts() string {
	var css strings.Builder
	for _, font := range []struct {
		family string
		weight int
		data   []byte
	}{
		{"sans", 400, texgyreherosregular.TTF}, {"sans", 700, texgyreherosbold.TTF},
		{"serif", 400, crimsonproregular.TTF}, {"serif", 700, crimsonprobold.TTF},
	} {
		fmt.Fprintf(&css, "@font-face{font-family:'%s';font-weight:%d;src:url(data:font/otf;base64,%s)}\n", font.family, font.weight, base64.StdEncoding.EncodeToString(font.data))
	}
	return css.String()
}
