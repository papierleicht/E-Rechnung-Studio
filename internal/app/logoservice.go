package app

import (
	"encoding/base64"
	"errors"

	"papierleicht.de/e-rechnung-studio/internal/invoicelogo"
)

// CompanyLogoUpload is a normalized logo ready to be saved with the company profile.
type CompanyLogoUpload struct {
	Logo    string `json:"logo"`
	Width   int    `json:"width"`
	Height  int    `json:"height"`
	Warning string `json:"warning"`
}

// PrepareCompanyLogo converts an uploaded PNG/JPEG into the stored invoice logo.
// Nothing is persisted; the logo is saved together with the settings page.
func (s *InvoiceService) PrepareCompanyLogo(encoded string) (CompanyLogoUpload, error) {
	if base64.StdEncoding.DecodedLen(len(encoded)) > invoicelogo.MaxFileSize+3 {
		return CompanyLogoUpload{}, errors.New("Das Logo darf höchstens 10 MB groß sein.")
	}
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return CompanyLogoUpload{}, errors.New("Die Logo-Datei lässt sich nicht lesen. Wähle eine andere Datei.")
	}
	logo, err := invoicelogo.Normalize(data)
	if err != nil {
		return CompanyLogoUpload{}, err
	}
	return CompanyLogoUpload{Logo: logo.Encode(), Width: logo.Width, Height: logo.Height, Warning: logo.Warning()}, nil
}
