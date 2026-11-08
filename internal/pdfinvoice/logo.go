package pdfinvoice

import (
	"fmt"
	"html/template"
	"os"

	"papierleicht.de/e-rechnung-studio/internal/invoicelogo"
	"papierleicht.de/e-rechnung-studio/internal/model"
)

// Only the issuer snapshot (or current settings for drafts) supplies a logo.
// Received invoices are rendered without an issuer and never show our logo.
func issuerLogo(issuer *model.Company) (*invoicelogo.Logo, error) {
	if issuer == nil {
		return nil, nil
	}
	return invoicelogo.Decode(issuer.Logo)
}

// Both renderers get the same fixed millimetre size, computed from the
// stored pixels, so preview and PDF place the logo identically.
func logoAttributes(source template.HTMLAttr, logo *invoicelogo.Logo) template.HTMLAttr {
	if source == "" || logo == nil {
		return ""
	}
	width, height := logo.PrintSize()
	return source + template.HTMLAttr(fmt.Sprintf(` alt="" style="width:%.2fmm;height:%.2fmm"`, width, height))
}

// htmlbag reads images from the filesystem. The private temporary file is
// removed by the caller immediately after rendering.
func logoImageFile(logo *invoicelogo.Logo) (string, error) {
	if logo == nil {
		return "", nil
	}
	f, err := os.CreateTemp("", "ers-logo-*.png")
	if err != nil {
		return "", err
	}
	_, err = f.Write(logo.PNG)
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		os.Remove(f.Name())
		return "", err
	}
	return f.Name(), nil
}
