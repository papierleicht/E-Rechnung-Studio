package app

import (
	"strings"

	"papierleicht.de/e-rechnung-studio/internal/einvoice"
	"papierleicht.de/e-rechnung-studio/internal/model"
)

// correctionMessage is raised where a received correction and the invoice it
// refers to both count. The BMF letter of 15.10.2026 (Rn. 20, 21) leaves open
// which document type a correction has, and whether it replaces the invoice or
// adds to it. The file does not say either, so the app does not net them: the
// user says in the document which of them applies.
const correctionMessage = "Eine Berichtigung kann die Ursprungsrechnung ersetzen oder ergänzen. Ersetzt sie sie, steht deren Vorsteuer doppelt in der Summe. Die Datei zeigt das nicht, deshalb rechnet die App beide ein. Prüfe, welche Rechnung gilt, und entscheide es im Beleg."

// precedingInvoices reads the invoices a document refers to (BG-3).
func precedingInvoices(inv *einvoice.Invoice) []model.PrecedingInvoice {
	var out []model.PrecedingInvoice
	for _, ref := range inv.InvoiceReferencedDocument {
		if number := strings.TrimSpace(ref.ID); number != "" {
			preceding := model.PrecedingInvoice{Number: number}
			if !ref.Date.IsZero() {
				preceding.Date = ref.Date.Format("2006-01-02")
			}
			out = append(out, preceding)
		}
	}
	return out
}

func foldedText(text string) string { return strings.ToLower(strings.Join(strings.Fields(text), " ")) }

// correctionOpen says whether a correction still waits for the user: it has an
// original in the list, and a decision on neither of the two says which applies.
func correctionOpen(id string, originals map[string]model.ReceivedInvoice, choices map[string]model.InputTaxChoice) (model.ReceivedInvoice, bool) {
	original, ok := originals[id]
	if !ok || choiceOf(choices, id) != nil || choiceOf(choices, original.ID) != nil {
		return original, false
	}
	return original, true
}

// receivedOriginals maps each received correction to the invoice it refers to,
// if that invoice of the same supplier was received as well. A correction is a
// document of type 384, or one that names a preceding invoice. Credit notes are
// left out: they count negative anyway.
func receivedOriginals(list []model.ReceivedInvoice) map[string]model.ReceivedInvoice {
	bySupplierAndNumber := map[string]model.ReceivedInvoice{}
	for _, v := range list {
		supplier, number := foldedText(v.Inspection.SellerName), foldedText(v.Inspection.InvoiceNumber)
		if v.Status != "misimport" && supplier != "" && number != "" {
			bySupplierAndNumber[supplier+"\x00"+number] = v
		}
	}
	out := map[string]model.ReceivedInvoice{}
	for _, v := range list {
		in := v.Inspection
		if v.Status == "misimport" || in.DocumentType == 381 || (in.DocumentType != 384 && len(in.PrecedingInvoices) == 0) {
			continue
		}
		for _, ref := range in.PrecedingInvoices {
			if original, ok := bySupplierAndNumber[foldedText(in.SellerName)+"\x00"+foldedText(ref.Number)]; ok && original.ID != v.ID {
				out[v.ID] = original
				break
			}
		}
	}
	return out
}
