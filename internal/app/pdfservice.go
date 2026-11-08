package app

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"papierleicht.de/e-rechnung-studio/internal/model"
	"papierleicht.de/e-rechnung-studio/internal/pdfinvoice"
)

func (s *InvoiceService) invoicePDF(id string) ([]byte, error) {
	d, err := s.store.Invoice(id)
	if err != nil {
		return nil, err
	}
	if d.Status == "draft" {
		return nil, errors.New("Stelle die Rechnung zuerst verbindlich aus, bevor du sie exportierst.")
	}
	if data, err := s.store.Artifact(id, "facturx-pdf"); err == nil {
		return data, nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	xml, err := s.store.Artifact(id, "cii-xml")
	if err != nil {
		return nil, err
	}
	data, err := s.renderInvoicePDF(xml, d)
	if err != nil {
		return nil, err
	}
	return s.store.CacheArtifact(id, "facturx-pdf", data)
}

func (s *InvoiceService) renderInvoicePDF(xml []byte, draft model.InvoiceDraft) ([]byte, error) {
	if s.validator.Validate(xml).State != "valid" {
		return nil, errors.New("Die gespeicherte E-Rechnung besteht die Prüfung nicht. Der PDF-Export wurde deshalb abgebrochen.")
	}
	data, err := pdfinvoice.RenderWithSnapshot(xml, draft.IssuerSnapshot, &draft.Customer, pdfinvoice.PaymentQROptions{Enabled: !draft.DisablePaymentQR, HideDetails: draft.HidePaymentQRDetails})
	if err != nil {
		return nil, fmt.Errorf("Die E-Rechnungs-PDF konnte nicht erstellt werden: %w", err)
	}
	return data, nil
}

// ExportInvoicePDF writes a PDF with the immutable XML atomically after file selection.
func (s *InvoiceService) ExportInvoicePDF(id, path string) (string, error) {
	if !strings.EqualFold(filepath.Ext(path), ".pdf") {
		return "", errors.New("Speichere die E-Rechnung bitte mit der Dateiendung .pdf.")
	}
	data, err := s.invoicePDF(id)
	if err != nil {
		return "", err
	}
	return writeExport(path, data, "PDF")
}

// ExportInvoiceXML writes the immutable XML of an issued invoice.
func (s *InvoiceService) ExportInvoiceXML(id, path string) (string, error) {
	if !strings.EqualFold(filepath.Ext(path), ".xml") {
		return "", errors.New("Speichere die E-Rechnung bitte mit der Dateiendung .xml.")
	}
	draft, err := s.store.Invoice(id)
	if err != nil {
		return "", err
	}
	if draft.Status == "draft" {
		return "", errors.New("Stelle die Rechnung zuerst verbindlich aus, bevor du sie exportierst.")
	}
	data, err := s.store.Artifact(id, "cii-xml")
	if err != nil {
		return "", err
	}
	return writeExport(path, data, "XML-Datei")
}

func writeExport(path string, data []byte, label string) (string, error) {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".erechnungstudio-*"+filepath.Ext(path))
	if err != nil {
		return "", fmt.Errorf("Die %s konnte am gewählten Speicherort nicht gespeichert werden. Wähle einen anderen Ort: %w", label, err)
	}
	defer os.Remove(tmp.Name())
	if _, err = tmp.Write(data); err != nil {
		tmp.Close()
		return "", err
	}
	if err = tmp.Sync(); err != nil {
		tmp.Close()
		return "", err
	}
	if err = tmp.Close(); err != nil {
		return "", err
	}
	if err = os.Rename(tmp.Name(), path); err != nil {
		return "", fmt.Errorf("Die %s konnte nicht gespeichert werden: %w", label, err)
	}
	return path, nil
}

func (s *InvoiceService) previewInvoiceXML(d model.InvoiceDraft) ([]byte, model.InvoiceDraft, model.Company, error) {
	c, err := s.store.Company()
	if err != nil {
		return nil, d, c, err
	}
	d, err = s.store.RefreshDraftNumber(d)
	if err != nil {
		return nil, d, c, err
	}
	// These placeholders are display-only. Validation and issuance use the
	// untouched draft and still reject missing invoice data.
	d.Lines = append([]model.InvoiceLine(nil), d.Lines...)
	for i := range d.Lines {
		if strings.TrimSpace(d.Lines[i].Name) == "" {
			d.Lines[i].Name = "Position hinzufügen …"
		}
		if strings.TrimSpace(d.Lines[i].UnitPrice) == "" {
			d.Lines[i].UnitPrice = "0"
		}
	}
	inv, err := s.engine.Build(d, c)
	if err != nil {
		return nil, d, c, fmt.Errorf("Die Vorschau ist noch nicht möglich: %w", err)
	}
	xml, err := s.engine.XML(inv)
	if err != nil {
		return nil, d, c, err
	}
	return xml, d, c, nil
}
