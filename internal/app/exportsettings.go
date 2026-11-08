package app

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"unicode"

	"github.com/wailsapp/wails/v3/pkg/application"
	"papierleicht.de/e-rechnung-studio/internal/model"
	"papierleicht.de/e-rechnung-studio/internal/validation"
)

// GetAppSettings supplies settings to Bootstrap and export operations.
//
//wails:ignore
func (s *InvoiceService) GetAppSettings() (model.AppSettings, error) {
	v, err := s.store.AppSettings()
	if err != nil {
		return v, err
	}
	if v.Appearance == "" {
		v.Appearance = "system"
	}
	if v.ExportDirectory == "" {
		v.ExportDirectory = os.Getenv("E_RECHNUNG_STUDIO_EXPORT_DIR")
		if v.ExportDirectory == "" {
			documents, err := documentsDirectory()
			if err != nil {
				return v, err
			}
			v.ExportDirectory = filepath.Join(documents, "Rechnungen")
		}
	}
	return v, nil
}

func (s *InvoiceService) SaveAppSettings(v model.AppSettings) (model.AppSettings, error) {
	v, err := s.prepareAppSettings(v)
	if err != nil {
		return v, err
	}
	return v, s.store.SaveAppSettings(v)
}

// SaveSettings validates both parts before committing either one.
func (s *InvoiceService) SaveSettings(v model.AppSettings, company *model.Company) (model.SettingsSnapshot, error) {
	var result model.SettingsSnapshot
	if company != nil {
		c := model.NormalizeCompany(*company)
		if issues := validation.CompanyIssues(c); len(issues) > 0 {
			return result, errors.New(issues[0].Message)
		}
		company = &c
		result.Company = c
	} else {
		c, err := s.store.Company()
		if err != nil {
			return result, err
		}
		result.Company = c
	}
	settings, err := s.prepareAppSettings(v)
	if err != nil {
		return result, err
	}
	result.Settings = settings
	return result, s.store.SaveSettings(settings, company)
}

func (s *InvoiceService) prepareAppSettings(v model.AppSettings) (model.AppSettings, error) {
	switch v.Appearance {
	case "", "system":
		v.Appearance = "system"
	case "light", "dark":
	default:
		return v, errors.New("Wähle für die Darstellung System, Hell oder Dunkel.")
	}
	for id := range v.Sections {
		if !slices.Contains(model.MenuSections, id) {
			delete(v.Sections, id)
		}
	}
	if len(v.Sections) == 0 {
		v.Sections = nil
	}
	v.ExportDirectory = strings.TrimSpace(v.ExportDirectory)
	if !filepath.IsAbs(v.ExportDirectory) {
		return v, errors.New("Wähle den vollständigen Pfad eines Ordners für den Rechnungsexport.")
	}
	v.ExportDirectory = filepath.Clean(v.ExportDirectory)
	current, err := s.GetAppSettings()
	if err != nil {
		return v, err
	}
	// Appearance changes must not depend on a previously selected external
	// export drive being connected or writable. Exports check the folder again.
	if v.ExportDirectory == filepath.Clean(current.ExportDirectory) {
		return v, nil
	}
	if err := os.MkdirAll(v.ExportDirectory, 0o700); err != nil {
		return v, fmt.Errorf("Der Exportordner konnte nicht angelegt werden. Prüfe den Pfad: %w", err)
	}
	probe, err := os.CreateTemp(v.ExportDirectory, ".erechnungstudio-test-*")
	if err != nil {
		return v, fmt.Errorf("Der Exportordner lässt sich nicht beschreiben: %w", err)
	}
	probe.Close()
	os.Remove(probe.Name())
	return v, nil
}

// ExportIssuedInvoice is safe to retry: immutable artifacts are reused and
// existing files with different contents are never replaced.
func (s *InvoiceService) ExportIssuedInvoice(id string) (result model.InvoiceExport, err error) {
	s.exportMu.Lock()
	defer s.exportMu.Unlock()
	draft, err := s.store.Invoice(id)
	if err != nil {
		return result, err
	}
	if draft.Status == "draft" {
		return result, errors.New("Stelle die Rechnung zuerst verbindlich aus, bevor du sie exportierst.")
	}
	result, err = s.store.InvoiceExport(id)
	if err != nil {
		return result, err
	}
	result.Error = ""
	defer func() {
		if err != nil {
			result.Error = fmt.Sprintf("Die Rechnung ist ausgestellt, aber der automatische Export hat nicht geklappt: %v", err)
		}
		if saveErr := s.store.SaveInvoiceExport(id, result); saveErr != nil && err == nil {
			err = saveErr
		}
	}()
	settings, err := s.GetAppSettings()
	if err != nil {
		return result, err
	}
	if err = os.MkdirAll(settings.ExportDirectory, 0o700); err != nil {
		return result, err
	}
	pdf, err := s.invoicePDF(id)
	if err != nil {
		return result, err
	}
	prefix := "Rechnung-"
	if draft.Kind == "credit-note" {
		prefix = "Gutschrift-"
	}
	name := prefix + exportFilename(draft.Number)
	result.PDFPath, err = exportArtifact(settings.ExportDirectory, name, ".pdf", result.PDFPath, pdf)
	if err != nil {
		return result, err
	}
	if strings.HasPrefix(draft.Profile, "xrechnung-") {
		xml, readErr := s.store.Artifact(id, "cii-xml")
		if readErr != nil {
			return result, readErr
		}
		result.XMLPath, err = exportArtifact(settings.ExportDirectory, name, ".xml", result.XMLPath, xml)
	}
	return result, err
}

func (s *InvoiceService) invoicePDFExportPath(id string) (string, error) {
	// Opening an existing export does not require its folder preference (or
	// XRechnung's separate XML export) to still be available.
	pdf, err := s.invoicePDF(id)
	if err != nil {
		return "", err
	}
	result, err := s.store.InvoiceExport(id)
	if err != nil {
		return "", err
	}
	existing, readErr := os.ReadFile(result.PDFPath)
	if readErr != nil || !filepath.IsAbs(result.PDFPath) || !strings.EqualFold(filepath.Ext(result.PDFPath), ".pdf") || !bytes.Equal(existing, pdf) {
		result, err = s.ExportIssuedInvoice(id)
		if err != nil {
			return "", fmt.Errorf("Die PDF konnte nicht geöffnet werden: %w", err)
		}
	}
	return result.PDFPath, nil
}

func (s *InvoiceService) OpenInvoicePDF(id string) (string, error) {
	path, err := s.invoicePDFExportPath(id)
	if err != nil {
		return "", err
	}
	if app := application.Get(); app != nil {
		if err := app.Browser.OpenFile(path); err != nil {
			return "", fmt.Errorf("Die PDF konnte nicht geöffnet werden: %w", err)
		}
	} else {
		return "", errors.New("Die PDF kannst du nur in der App öffnen.")
	}
	return path, nil
}

func exportFilename(number string) string {
	runes := []rune(strings.TrimSpace(number))
	if len(runes) > 100 {
		runes = runes[:100]
	}
	name := strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || strings.ContainsRune(`<>:"/\|?*`, r) {
			return '-'
		}
		return r
	}, string(runes))
	name = strings.Trim(name, ". ")
	if name == "" {
		return "Beleg"
	}
	return name
}

func exportArtifact(dir, name, extension, previous string, data []byte) (string, error) {
	if filepath.IsAbs(previous) && strings.EqualFold(filepath.Ext(previous), extension) {
		if existing, err := os.ReadFile(previous); err == nil && bytes.Equal(existing, data) {
			return previous, nil
		}
	}
	for i := 0; i < 10000; i++ {
		filename := name
		if i > 0 {
			filename = fmt.Sprintf("%s (%d)", name, i+1)
		}
		path := filepath.Join(dir, filename+extension)
		file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if errors.Is(err, os.ErrExist) {
			if existing, readErr := os.ReadFile(path); readErr == nil && bytes.Equal(existing, data) {
				return path, nil
			}
			continue
		}
		if err != nil {
			return "", err
		}
		_, err = file.Write(data)
		if err == nil {
			err = file.Sync()
		}
		closeErr := file.Close()
		if err == nil {
			err = closeErr
		}
		if err != nil {
			os.Remove(path)
			return "", err
		}
		return path, nil
	}
	return "", errors.New("Wähle einen anderen Exportordner. Dort gibt es schon zu viele gleichnamige Dateien.")
}
