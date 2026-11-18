package app

import (
	"bytes"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	erechnung "github.com/papierleicht/go-e-rechnung-validator"
	"github.com/wailsapp/wails/v3/pkg/application"
	"papierleicht.de/e-rechnung-studio/internal/invoicexml"
	"papierleicht.de/e-rechnung-studio/internal/model"
	"papierleicht.de/e-rechnung-studio/internal/pdfinvoice"
	validator "papierleicht.de/e-rechnung-studio/internal/validation"
)

const maxReceivedSize = 20 << 20

// Preparing a preview never inserts a document. ImportReceivedInvoice is called
// separately after the user has reviewed and confirmed the selected bytes.
func (s *InvoiceService) PrepareReceivedInvoice(filename, encoded string) (model.ReceivedPreview, error) {
	var preview model.ReceivedPreview
	data, kind, err := decodeReceived(filename, encoded)
	if err != nil {
		return preview, err
	}
	preview.Inspection, err = s.inspectReceived(data, kind)
	if err != nil {
		return preview, err
	}
	hash := sha256.Sum256(data)
	existing, _, err := s.store.GetReceived(hex.EncodeToString(hash[:]))
	if err == nil {
		duplicate := []model.ReceivedInvoice{existing}
		if err = s.presentReceived(duplicate); err != nil {
			return preview, err
		}
		preview.Duplicate = &duplicate[0]
	} else if !errors.Is(err, sql.ErrNoRows) {
		return preview, err
	}
	if kind == "pdf" {
		preview.PDF = encoded
	} else if preview.Inspection.Syntax == "" || preview.Inspection.Syntax == "unknown" {
		preview.PreviewError = "Die XML-Datei lässt sich nicht als Rechnung darstellen. Prüfe den Prüfbericht und die XML-Originalansicht."
	} else {
		// Read-only visualisation of the received XML, never a replacement original.
		pdfBytes, err := pdfinvoice.Render(data)
		if err != nil {
			preview.PreviewError = "Für diese XML-Datei gibt es keine vollständige Belegansicht. Prüfe die erkannten Daten und die XML-Originalansicht."
		} else {
			preview.PDF = base64.StdEncoding.EncodeToString(pdfBytes)
		}
	}
	return preview, nil
}

func (s *InvoiceService) ChangeReceivedStatus(id, status, reason string) (model.ReceivedInvoice, error) {
	item, err := s.store.ChangeReceivedStatus(id, status, reason)
	if err != nil {
		return item, err
	}
	items := []model.ReceivedInvoice{item}
	err = s.presentReceived(items)
	return items[0], err
}

func (s *InvoiceService) DeleteReceivedMisimport(id, filename string, noRetentionRequired bool) error {
	return s.store.DeleteReceivedMisimport(id, filename, noRetentionRequired)
}

func decodeReceived(filename, encoded string) ([]byte, string, error) {
	kind := strings.TrimPrefix(strings.ToLower(filepath.Ext(filename)), ".")
	if kind != "xml" && kind != "pdf" {
		return nil, "", errors.New("Wähle eine XML-Datei oder ein PDF aus.")
	}
	if len(encoded) > base64.StdEncoding.EncodedLen(maxReceivedSize) {
		return nil, "", errors.New("Dateien bis 20 MB werden unterstützt.")
	}
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || len(data) == 0 || len(data) > maxReceivedSize {
		return nil, "", errors.New("Die Datei ist leer, zu groß oder lässt sich nicht lesen.")
	}
	return data, kind, nil
}

// InspectInvoiceFile is read-only. ImportReceivedInvoice additionally archives the original.
func (s *InvoiceService) InspectInvoiceFile(filename, encoded string) (model.ImportInspection, error) {
	data, kind, err := decodeReceived(filename, encoded)
	if err != nil {
		return model.ImportInspection{}, err
	}
	return s.inspectReceived(data, kind)
}

func (s *InvoiceService) inspectReceived(data []byte, kind string) (model.ImportInspection, error) {
	invoice, notice, err := receivedInvoiceXML(data, kind)
	if err != nil {
		return model.ImportInspection{}, err
	}
	if notice != nil {
		return validator.AnnotateReceived(*notice), nil
	}
	inspection, err := s.InspectXML(string(invoice))
	if err == nil {
		parsed, parseErr := invoicexml.Parse(invoice)
		if parseErr != nil {
			parsed = nil
		}
		// The findings are worded for the issuer of an invoice, not for its recipient.
		inspection.Validation = validator.GermanReceived(inspection.Validation, parsed)
		if parsed != nil {
			inspection.DocumentType = int(parsed.InvoiceTypeCode)
			inspection.PrecedingInvoices = precedingInvoices(parsed)
			inspection = validator.RelaxSellerIdentifier(inspection, parsed)
		}
	}
	return validator.AnnotateReceived(inspection), err
}

// receivedInvoiceXML returns the invoice XML of a received file: the file
// itself or the single invoice attachment of a PDF. A notice explains why a
// readable PDF has no usable invoice data.
func receivedInvoiceXML(data []byte, kind string) ([]byte, *model.ImportInspection, error) {
	if kind == "xml" {
		return data, nil, nil
	}
	invoice, err := erechnung.ExtractXML(data)
	switch {
	case errors.Is(err, erechnung.ErrNoInvoiceXML):
		notice := receivedNotice("PDF-NO-XML", "Keine eingebetteten Rechnungsdaten gefunden. Du kannst dieses PDF als Beleg speichern, es gilt aber nicht als strukturierte E-Rechnung. Ein PDF kann noch ausreichen, wenn du zugestimmt hast: für Leistungen bis Ende 2026, für Leistungen im Jahr 2027 nur, wenn der Aussteller im Vorjahr höchstens 800.000 € Gesamtumsatz hatte (§ 27 Abs. 38 UStG). Ob das auf diese Rechnung zutrifft, prüft die App nicht.")
		return nil, &notice, nil
	case errors.Is(err, erechnung.ErrMultipleInvoiceXML):
		notice := receivedNotice("PDF-MULTIPLE-XML", "Die PDF enthält mehrere Rechnungsdateien. Eine eindeutige Prüfung ist deshalb nicht möglich.")
		return nil, &notice, nil
	case err != nil:
		return nil, nil, fmt.Errorf("Die PDF oder einer ihrer Anhänge ist beschädigt, verschlüsselt, zu groß oder nicht lesbar: %w", err)
	}
	return invoice, nil, nil
}

func receivedNotice(rule, message string) model.ImportInspection {
	return model.ImportInspection{Validation: model.ValidationResult{State: "unsupported", Issues: []model.ValidationIssue{{Stage: "import", RuleID: rule, Severity: "warning", Message: message}}}}
}

func (s *InvoiceService) ImportReceivedInvoice(filename, encoded string) (model.ReceivedImport, error) {
	recordUse("beleg_import")
	data, kind, err := decodeReceived(filename, encoded)
	if err != nil {
		return model.ReceivedImport{}, err
	}
	inspection, err := s.inspectReceived(data, kind)
	if err != nil {
		return model.ReceivedImport{}, err
	}
	hash := sha256.Sum256(data)
	item := model.ReceivedInvoice{ID: hex.EncodeToString(hash[:]), Filename: filepath.Base(strings.ReplaceAll(filename, "\\", "/")), Kind: kind, ImportedAt: time.Now().UTC().Format(time.RFC3339Nano), Size: len(data), Inspection: inspection}
	item.Status = "active"
	item.History = []model.ReceivedEvent{{Action: "imported", At: item.ImportedAt}}
	imported, err := s.store.SaveReceived(item, data)
	if err != nil {
		return imported, err
	}
	items := []model.ReceivedInvoice{imported.Invoice}
	err = s.presentReceived(items)
	imported.Invoice = items[0]
	return imported, err
}

func (s *InvoiceService) ListReceivedInvoices() ([]model.ReceivedInvoice, error) {
	items, err := s.store.ListReceived()
	if err != nil {
		return items, err
	}
	return items, s.presentReceived(items)
}

func (s *InvoiceService) RecheckReceivedInvoice(id string) (model.ImportInspection, error) {
	item, data, err := s.store.GetReceived(id)
	if err != nil {
		return model.ImportInspection{}, err
	}
	inspection, err := s.inspectReceived(data, item.Kind)
	if err != nil {
		return model.ImportInspection{}, err
	}
	if err = s.store.UpdateReceivedInspection(id, inspection); err != nil {
		return inspection, err
	}
	return s.presentedInspection(id)
}

// A private temporary copy lets the OS display the original PDF without modifying the archive.
func (s *InvoiceService) OpenReceivedPDF(id string) error {
	item, data, err := s.store.GetReceived(id)
	if err != nil {
		return err
	}
	if item.Kind != "pdf" {
		return errors.New("Dieser Beleg ist keine PDF.")
	}
	app := application.Get()
	if app == nil {
		return errors.New("Die PDF kannst du nur in der App öffnen.")
	}
	return openTemporaryCopy(app, "erechnungstudio-eingang-*.pdf", data)
}

// openTemporaryCopy writes data to a new private file and lets the OS open it.
func openTemporaryCopy(app *application.App, pattern string, data []byte) error {
	file, err := os.CreateTemp("", pattern)
	if err != nil {
		return err
	}
	_, err = file.Write(data)
	closeErr := file.Close()
	if err != nil {
		os.Remove(file.Name())
		return err
	}
	if closeErr != nil {
		os.Remove(file.Name())
		return closeErr
	}
	return app.Browser.OpenFile(file.Name())
}

func (s *InvoiceService) SaveReceivedOriginal(id, path string) error {
	_, data, err := s.store.GetReceived(id)
	if err != nil {
		return err
	}
	// Never truncate an existing file, even if another process creates it after the save dialog.
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("Das Original konnte nicht gespeichert werden. Wähle einen neuen Dateinamen: %w", err)
	}
	_, err = io.Copy(file, bytes.NewReader(data))
	closeErr := file.Close()
	if err != nil {
		os.Remove(path)
		return err
	}
	return closeErr
}
