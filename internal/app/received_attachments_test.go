package app

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"papierleicht.de/e-rechnung-studio/internal/einvoice"
	"papierleicht.de/e-rechnung-studio/internal/invoicexml"
	"papierleicht.de/e-rechnung-studio/internal/model"
)

var (
	attachmentPDF  = receivedTestPDF(false)
	attachmentHTML = []byte("<html><script>alert(1)</script></html>")
)

// supplierDocuments are the supporting documents (BG-24) a supplier's software
// could attach, followed by the bare reference to an invoiced object (BT-18).
func supplierDocuments() []einvoice.Document {
	return []einvoice.Document{
		{IssuerAssignedID: "SN-2026-03", TypeCode: "916", Name: "Stundennachweis März", AttachmentMimeCode: "application/pdf", AttachmentFilename: "Stundennachweis.pdf", AttachmentBinaryObject: attachmentPDF},
		{IssuerAssignedID: "Vertrag 4711", TypeCode: "916"},
		{IssuerAssignedID: "LINK-1", TypeCode: "916", Name: "Preisblatt", URIID: "https://lieferant.example/preisblatt"},
		{IssuerAssignedID: "OBJ-9", TypeCode: "130"},
		{IssuerAssignedID: "SKRIPT", TypeCode: "916", Name: "Skript", AttachmentMimeCode: "text/html", AttachmentFilename: `..\..\skript.html`, AttachmentBinaryObject: attachmentHTML},
		// A PDF that calls itself a picture does not match its declared type.
		{IssuerAssignedID: "BILD", TypeCode: "916", Name: "Foto", AttachmentMimeCode: "image/png", AttachmentFilename: "foto.png", AttachmentBinaryObject: attachmentPDF},
	}
}

// documentWith rewrites a document the app issued so that it carries the given
// supporting documents, the way a supplier's software would.
func documentWith(t *testing.T, xml, syntax string, docs ...einvoice.Document) string {
	t.Helper()
	invoice, err := invoicexml.Parse([]byte(xml))
	if err != nil {
		t.Fatal(err)
	}
	invoice.AdditionalReferencedDocument = docs
	var out bytes.Buffer
	if err := invoice.Write(&out); err != nil {
		t.Fatal(err)
	}
	written := out.String()
	if syntax == "cii" {
		// The writer has no place for BT-124 in CII; suppliers write it after the reference.
		for _, doc := range docs {
			if doc.URIID != "" {
				id := "<ram:IssuerAssignedID>" + doc.IssuerAssignedID + "</ram:IssuerAssignedID>"
				written = replaceOnce(t, written, id, id+"<ram:URIID>"+doc.URIID+"</ram:URIID>")
			}
		}
	}
	return written
}

func importDocument(t *testing.T, s *InvoiceService, filename string, data []byte) string {
	t.Helper()
	imported, err := s.ImportReceivedInvoice(filename, base64.StdEncoding.EncodeToString(data))
	if err != nil {
		t.Fatal(err)
	}
	return imported.Invoice.ID
}

var attachmentFormats = []struct{ profile, syntax string }{{"xrechnung-cii", "cii"}, {"xrechnung-ubl", "ubl"}}

// BMF letter of 15.10.2025, Rn. 35: supplementary details may sit in an
// attachment, and the recipient has to be able to look at it.
func TestReceivedAttachmentsListEmbeddedLinkedAndReferencedDocuments(t *testing.T) {
	want := []model.ReceivedAttachment{
		{Index: 0, Kind: "file", Name: "Stundennachweis März", Reference: "SN-2026-03", Filename: "Stundennachweis.pdf", MimeType: "application/pdf", Size: len(attachmentPDF), Openable: true},
		{Index: 1, Kind: "reference", Reference: "Vertrag 4711"},
		{Index: 2, Kind: "link", Name: "Preisblatt", Reference: "LINK-1", Link: "https://lieferant.example/preisblatt"},
		{Index: 4, Kind: "file", Name: "Skript", Reference: "SKRIPT", Filename: "skript.html", MimeType: "text/html", Size: len(attachmentHTML)},
		{Index: 5, Kind: "file", Name: "Foto", Reference: "BILD", Filename: "foto.png", MimeType: "image/png", Size: len(attachmentPDF)},
	}
	for _, f := range attachmentFormats {
		t.Run(f.syntax, func(t *testing.T) {
			s := receivedService(t)
			id := importDocument(t, s, "lieferant.xml", []byte(documentWith(t, issuedXML(t, s, f.profile, f.syntax), f.syntax, supplierDocuments()...)))
			got, err := s.ReceivedAttachments(id)
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(got, want) {
				t.Fatalf("got %+v\nwant %+v", got, want)
			}
		})
	}
}

func TestReceivedAttachmentsAreFoundInsideAHybridPDF(t *testing.T) {
	s := receivedService(t)
	xml := documentWith(t, issuedXML(t, s, "facturx-en16931", "cii"), "cii", supplierDocuments()[0])
	id := importDocument(t, s, "lieferant.pdf", receivedTestPDF(false, xml))
	got, err := s.ReceivedAttachments(id)
	if err != nil || len(got) != 1 || got[0].Kind != "file" || !got[0].Openable || got[0].Size != len(attachmentPDF) {
		t.Fatalf("got %+v %v", got, err)
	}
}

func TestReceivedDocumentWithoutAttachmentsHasNone(t *testing.T) {
	s := receivedService(t)
	for name, data := range map[string][]byte{
		"lieferant.xml": []byte(issuedXML(t, s, "xrechnung-cii", "cii")),
		"beleg.pdf":     receivedTestPDF(false),
	} {
		got, err := s.ReceivedAttachments(importDocument(t, s, name, data))
		if err != nil || got == nil || len(got) != 0 {
			t.Errorf("%s: got %#v %v", name, got, err)
		}
	}
	if _, err := s.ReceivedAttachments("unbekannt"); err == nil {
		t.Error("an unknown document must be reported")
	}
}

func TestBrokenAttachmentDataIsReportedNotShownAsEmpty(t *testing.T) {
	s := receivedService(t)
	xml := documentWith(t, issuedXML(t, s, "xrechnung-cii", "cii"), "cii", supplierDocuments()[0])
	xml = strings.Replace(xml, base64.StdEncoding.EncodeToString(attachmentPDF), "INVALID!!!BASE64", 1)
	id := importDocument(t, s, "lieferant.xml", []byte(xml))
	if got, err := s.ReceivedAttachments(id); err == nil || got != nil || !strings.Contains(err.Error(), "Anlagen") {
		t.Fatalf("got %+v %v", got, err)
	}
}

func TestReceivedAttachmentIsSavedAndNeverOverwritten(t *testing.T) {
	for _, f := range attachmentFormats {
		t.Run(f.syntax, func(t *testing.T) {
			s := receivedService(t)
			id := importDocument(t, s, "lieferant.xml", []byte(documentWith(t, issuedXML(t, s, f.profile, f.syntax), f.syntax, supplierDocuments()...)))
			dir := t.TempDir()
			target := filepath.Join(dir, "nachweis.pdf")
			if err := s.SaveReceivedAttachment(id, 0, target); err != nil {
				t.Fatal(err)
			}
			if got, _ := os.ReadFile(target); !bytes.Equal(got, attachmentPDF) {
				t.Fatalf("saved %d bytes, want the attachment", len(got))
			}
			if err := s.SaveReceivedAttachment(id, 0, target); err == nil {
				t.Fatal("an existing file must not be overwritten")
			}
			if got, _ := os.ReadFile(target); !bytes.Equal(got, attachmentPDF) {
				t.Fatal("the existing file was changed")
			}
			// A file the app does not open can still be saved.
			script := filepath.Join(dir, "skript.html")
			if err := s.SaveReceivedAttachment(id, 4, script); err != nil {
				t.Fatal(err)
			}
			if got, _ := os.ReadFile(script); !bytes.Equal(got, attachmentHTML) {
				t.Fatal("the unopened type was not saved as it is")
			}
			// Entries without a file, the bare BT-18 reference and positions outside the list.
			for _, index := range []int{-1, 1, 2, 3, 6, 100} {
				target := filepath.Join(dir, fmt.Sprintf("kein-%d", index))
				if err := s.SaveReceivedAttachment(id, index, target); err == nil {
					t.Errorf("index %d has no file to save", index)
				} else if _, statErr := os.Stat(target); statErr == nil {
					t.Errorf("index %d left a file behind", index)
				}
			}
		})
	}
}

// Only the six types XRechnung allows open, and only if the content fits the type.
func TestOnlyKnownAttachmentTypesOpen(t *testing.T) {
	s := receivedService(t)
	id := importDocument(t, s, "lieferant.xml", []byte(documentWith(t, issuedXML(t, s, "xrechnung-cii", "cii"), "cii", supplierDocuments()...)))
	// Without the running app a document of a known type gets as far as the display.
	if err := s.OpenReceivedAttachment(id, 0); err == nil || !strings.Contains(err.Error(), "nur in der App") {
		t.Fatalf("got %v", err)
	}
	for index, want := range map[int]string{
		4:  "nicht geöffnet", // text/html
		5:  "nicht geöffnet", // a PDF declared as PNG
		1:  "keine Datei",
		2:  "keine Datei",
		3:  "gibt es nicht", // BT-18
		99: "gibt es nicht",
		-1: "gibt es nicht",
	} {
		if err := s.OpenReceivedAttachment(id, index); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("index %d: got %v, want %q", index, err, want)
		}
	}
}

func TestAttachmentTypesOpenOnlyWithMatchingContent(t *testing.T) {
	zip := []byte("PK\x03\x04rest")
	for _, tc := range []struct {
		mime    string
		content []byte
		want    bool
	}{
		{"application/pdf", attachmentPDF, true},
		{"APPLICATION/PDF; charset=binary", attachmentPDF, true},
		{"application/pdf", []byte("MZ\x90\x00 not a pdf"), false},
		{"image/png", []byte("\x89PNG\r\n\x1a\nrest"), true},
		{"image/png", attachmentPDF, false},
		{"image/jpeg", []byte("\xff\xd8\xff\xe0rest"), true},
		{"text/csv", []byte("a;b\n1;2\n"), true},
		{"text/csv", []byte("a;b\x00\n"), false},
		{"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", zip, true},
		{"application/vnd.oasis.opendocument.spreadsheet", zip, true},
		{"application/vnd.oasis.opendocument.spreadsheet", attachmentPDF, false},
		{"application/x-msdownload", []byte("MZ"), false},
		{"text/html", attachmentHTML, false},
		{"application/zip", zip, false},
		{"", attachmentPDF, false},
	} {
		got := describeAttachment(0, einvoice.Document{IssuerAssignedID: "X", AttachmentMimeCode: tc.mime, AttachmentBinaryObject: tc.content})
		if got.Openable != tc.want {
			t.Errorf("%q: openable %v, want %v", tc.mime, got.Openable, tc.want)
		}
	}
}

func TestAttachmentFilenamesAreNeverPaths(t *testing.T) {
	for _, tc := range []struct{ in, extension, want string }{
		{"Stundennachweis.pdf", ".pdf", "Stundennachweis.pdf"},
		{"../../etc/passwd", "", "passwd"},
		{`..\..\Windows\system.ini`, "", "system.ini"},
		{`C:\Users\x\bericht`, ".pdf", "bericht.pdf"},
		{"na<me>:?.csv", ".csv", "name.csv"},
		{"zeile\nsprung.pdf", ".pdf", "zeilesprung.pdf"},
		{"", ".png", "anlage-3.png"},
		{"..", ".pdf", "anlage-3.pdf"},
		{"  . ", ".pdf", "anlage-3.pdf"},
		{"/", ".pdf", "anlage-3.pdf"},
		{"unbekannt", "", "unbekannt"},
		{strings.Repeat("a", 300) + ".pdf", ".pdf", strings.Repeat("a", 120) + ".pdf"},
	} {
		if got := attachmentFilename(tc.in, tc.extension, 2); got != tc.want {
			t.Errorf("%q: got %q, want %q", tc.in, got, tc.want)
		}
	}
}
