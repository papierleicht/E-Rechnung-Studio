package app

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/wailsapp/wails/v3/pkg/application"
	"papierleicht.de/e-rechnung-studio/internal/einvoice"
	"papierleicht.de/e-rechnung-studio/internal/invoicexml"
	"papierleicht.de/e-rechnung-studio/internal/model"
)

// attachmentTypes are the media types XRechnung allows for embedded documents
// (BT-125). Only these open in another program, and only if the content looks
// like the declared type. The temporary copy takes its extension from the type,
// never from the file name the supplier gave.
var attachmentTypes = map[string]struct {
	extension string
	matches   func([]byte) bool
}{
	"application/pdf": {".pdf", func(b []byte) bool { return bytes.Contains(b[:min(len(b), 1024)], []byte("%PDF-")) }},
	"image/png":       {".png", func(b []byte) bool { return bytes.HasPrefix(b, []byte("\x89PNG\r\n\x1a\n")) }},
	"image/jpeg":      {".jpg", func(b []byte) bool { return bytes.HasPrefix(b, []byte("\xff\xd8\xff")) }},
	"text/csv":        {".csv", func(b []byte) bool { return !bytes.Contains(b, []byte{0}) }},
	"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet": {".xlsx", isZIP},
	"application/vnd.oasis.opendocument.spreadsheet":                    {".ods", isZIP},
}

func isZIP(b []byte) bool { return bytes.HasPrefix(b, []byte("PK\x03\x04")) }

func attachmentType(mime string) string {
	mime, _, _ = strings.Cut(strings.ToLower(strings.TrimSpace(mime)), ";")
	return strings.TrimSpace(mime)
}

// supportingDocument tells BG-24 entries from the bare references to a tender
// or lot (BT-17, type 50) and an invoiced object (BT-18, type 130), which share
// the element. An entry that carries a file or a link is always shown.
func supportingDocument(doc einvoice.Document) bool {
	if len(doc.AttachmentBinaryObject) > 0 || doc.URIID != "" {
		return true
	}
	if doc.TypeCode == "50" || doc.TypeCode == "130" {
		return false
	}
	return doc.IssuerAssignedID != "" || doc.Name != ""
}

func clip(text string, limit int) string {
	text = strings.TrimSpace(text)
	if runes := []rune(text); len(runes) > limit {
		return string(runes[:limit]) + "…"
	}
	return text
}

// attachmentFilename turns the supplier's file name into a plain name for the
// save dialog: no folders, no control or reserved characters, and the extension
// of the declared type if the name has none.
func attachmentFilename(name, extension string, index int) string {
	// Both separators are "/" by now, so path.Base splits the same way on every
	// system; filepath.Base("/") would be `\` on Windows.
	name = path.Base(strings.ReplaceAll(strings.TrimSpace(name), `\`, "/"))
	name = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || strings.ContainsRune(`<>:"/|?*`, r) {
			return -1
		}
		return r
	}, name)
	name = strings.Trim(name, " .")
	if name == "" {
		return fmt.Sprintf("anlage-%d%s", index+1, extension)
	}
	if runes := []rune(name); len(runes) > 120 {
		name = string(runes[:120])
	}
	if filepath.Ext(name) == "" {
		name += extension
	}
	return name
}

func describeAttachment(index int, doc einvoice.Document) model.ReceivedAttachment {
	mime := attachmentType(doc.AttachmentMimeCode)
	entry := model.ReceivedAttachment{
		Index:     index,
		Kind:      "reference",
		Name:      clip(doc.Name, 200),
		Reference: clip(doc.IssuerAssignedID, 200),
		MimeType:  clip(mime, 100),
		Link:      clip(doc.URIID, 500),
	}
	known := attachmentTypes[mime]
	if len(doc.AttachmentBinaryObject) > 0 {
		entry.Kind, entry.Size = "file", len(doc.AttachmentBinaryObject)
		entry.Filename = attachmentFilename(doc.AttachmentFilename, known.extension, index)
		entry.Openable = known.matches != nil && known.matches(doc.AttachmentBinaryObject)
	} else if entry.Link != "" {
		entry.Kind = "link"
	}
	return entry
}

// receivedDocuments reads the supporting documents from the stored original. A
// PDF without invoice data has none.
func (s *InvoiceService) receivedDocuments(id string) ([]einvoice.Document, error) {
	item, data, err := s.store.GetReceived(id)
	if err != nil {
		return nil, err
	}
	invoice, notice, err := receivedInvoiceXML(data, item.Kind)
	if err != nil {
		return nil, err
	}
	if notice != nil {
		return nil, nil
	}
	parsed, err := invoicexml.Parse(invoice)
	if err != nil {
		return nil, errors.New("Die Rechnungsdaten sind nicht lesbar, deshalb lassen sich die Anlagen nicht anzeigen.")
	}
	return parsed.AdditionalReferencedDocument, nil
}

// receivedAttachment returns the entry and the bytes of an embedded document.
func (s *InvoiceService) receivedAttachment(id string, index int) (model.ReceivedAttachment, []byte, error) {
	docs, err := s.receivedDocuments(id)
	if err != nil {
		return model.ReceivedAttachment{}, nil, err
	}
	if index < 0 || index >= len(docs) || !supportingDocument(docs[index]) {
		return model.ReceivedAttachment{}, nil, errors.New("Diese Anlage gibt es nicht.")
	}
	if len(docs[index].AttachmentBinaryObject) == 0 {
		return model.ReceivedAttachment{}, nil, errors.New("Zu diesem Eintrag gehört keine Datei, die Rechnung verweist nur darauf.")
	}
	return describeAttachment(index, docs[index]), docs[index].AttachmentBinaryObject, nil
}

// ReceivedAttachments lists the supporting documents of a received invoice
// without their content. A document can be embedded, linked or only referenced.
func (s *InvoiceService) ReceivedAttachments(id string) ([]model.ReceivedAttachment, error) {
	docs, err := s.receivedDocuments(id)
	if err != nil {
		return nil, err
	}
	list := []model.ReceivedAttachment{}
	for index, doc := range docs {
		if supportingDocument(doc) {
			list = append(list, describeAttachment(index, doc))
		}
	}
	return list, nil
}

// OpenReceivedAttachment displays an embedded document of a known type from a
// private temporary copy. Any other file can be saved and checked by the user.
func (s *InvoiceService) OpenReceivedAttachment(id string, index int) error {
	entry, data, err := s.receivedAttachment(id, index)
	if err != nil {
		return err
	}
	if !entry.Openable {
		return errors.New("Diese Anlage wird nicht geöffnet, weil ihr Typ nicht eindeutig ist. Du kannst sie speichern und selbst prüfen.")
	}
	app := application.Get()
	if app == nil {
		return errors.New("Die Anlage kannst du nur in der App öffnen.")
	}
	return openTemporaryCopy(app, "erechnungstudio-anlage-*"+attachmentTypes[entry.MimeType].extension, data)
}

func (s *InvoiceService) SaveReceivedAttachment(id string, index int, path string) error {
	_, data, err := s.receivedAttachment(id, index)
	if err != nil {
		return err
	}
	// Never truncate an existing file, even if another process creates it after the save dialog.
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("Die Anlage konnte nicht gespeichert werden. Wähle einen neuen Dateinamen: %w", err)
	}
	_, err = file.Write(data)
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		os.Remove(path)
	}
	return err
}
