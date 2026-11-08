package app

import (
	"bytes"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	pdf "github.com/speedata/pdfdisassembler"
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
	fail := func(err error) ([]byte, *model.ImportInspection, error) { return nil, nil, err }
	r, err := pdf.Open(bytes.NewReader(data), pdf.WithMaxStreamSize(maxReceivedSize))
	if err != nil {
		return fail(fmt.Errorf("Die PDF ist beschädigt, verschlüsselt oder nicht lesbar: %w", err))
	}
	defer r.Close()
	if count, err := r.PageCount(); err != nil || count == 0 {
		return fail(errors.New("Die PDF enthält keine lesbaren Seiten."))
	}
	cat, err := r.Catalog()
	if err != nil {
		return fail(err)
	}
	attachments := r.EmbeddedFiles()
	// Associated files may be registered in /AF without an EmbeddedFiles name tree.
	if associated, ok := cat.Array("AF"); ok {
		for _, ref := range associated {
			spec, err := r.ResolveDict(ref)
			if err != nil {
				return fail(errors.New("Ein PDF-Anhang ist nicht lesbar."))
			}
			attachments = append(attachments, pdf.EmbeddedFile{Spec: spec})
		}
	}
	if len(attachments) > 128 {
		return fail(errors.New("Die PDF enthält zu viele Anhänge (maximal 128)."))
	}
	seen := map[*pdf.Dict]bool{}
	var candidates [][]byte
	var otherXML [][]byte
	decodedSize := 0
	for _, attachment := range attachments {
		if seen[attachment.Spec] {
			continue
		}
		seen[attachment.Spec] = true
		name := attachment.Name
		if n, ok := attachment.Spec.String("UF"); ok {
			name = string(n)
		} else if n, ok := attachment.Spec.String("F"); ok {
			name = string(n)
		}
		ef, ok := attachment.Spec.Dict("EF")
		if !ok {
			return fail(errors.New("Ein PDF-Anhang enthält keine lesbaren Daten."))
		}
		stream, ok := ef.Get("UF")
		if !ok {
			stream, ok = ef.Get("F")
		}
		if !ok {
			return fail(errors.New("Ein PDF-Anhang enthält keine lesbaren Daten."))
		}
		content, err := r.DecodeStream(stream)
		if err != nil {
			return fail(fmt.Errorf("PDF-Anhang konnte nicht gelesen werden: %w", err))
		}
		decodedSize += len(content)
		if decodedSize > maxReceivedSize {
			return fail(errors.New("Die entpackten PDF-Anhänge überschreiten 20 MB."))
		}
		if invoiceXML(content) {
			candidates = append(candidates, content)
		} else if strings.HasSuffix(strings.ToLower(name), ".xml") {
			otherXML = append(otherXML, content)
		}
	}
	// Still inspect malformed XML attachments, but don't let unrelated metadata
	// displace a recognizable invoice.
	if len(candidates) == 0 {
		candidates = otherXML
	}
	if len(candidates) == 0 {
		notice := receivedNotice("PDF-NO-XML", "Keine eingebetteten Rechnungsdaten gefunden. Du kannst dieses PDF als Beleg speichern, es gilt aber nicht als strukturierte E-Rechnung. Ein PDF kann noch ausreichen, wenn du zugestimmt hast: für Leistungen bis Ende 2026, für Leistungen im Jahr 2027 nur, wenn der Aussteller im Vorjahr höchstens 800.000 € Gesamtumsatz hatte (§ 27 Abs. 38 UStG). Ob das auf diese Rechnung zutrifft, prüft die App nicht.")
		return nil, &notice, nil
	}
	if len(candidates) != 1 {
		notice := receivedNotice("PDF-MULTIPLE-XML", "Die PDF enthält mehrere Rechnungsdateien. Eine eindeutige Prüfung ist deshalb nicht möglich.")
		return nil, &notice, nil
	}
	return candidates[0], nil, nil
}

func invoiceXML(data []byte) bool {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	for {
		token, err := decoder.Token()
		if err != nil {
			return false
		}
		if start, ok := token.(xml.StartElement); ok {
			return start.Name.Local == "CrossIndustryInvoice" || start.Name.Local == "Invoice" || start.Name.Local == "CreditNote"
		}
	}
}

func receivedNotice(rule, message string) model.ImportInspection {
	return model.ImportInspection{Validation: model.ValidationResult{State: "unsupported", Issues: []model.ValidationIssue{{Stage: "import", RuleID: rule, Severity: "warning", Message: message}}}}
}

func (s *InvoiceService) ImportReceivedInvoice(filename, encoded string) (model.ReceivedImport, error) {
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
