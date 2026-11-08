package app

import (
	"papierleicht.de/e-rechnung-studio/internal/model"
	"papierleicht.de/e-rechnung-studio/internal/pdfinvoice"
)

// InvoiceDocumentHTML never writes a draft or creates an issued artifact.
// Issued invoices always use the immutable XML and saved party snapshots.
func (s *InvoiceService) InvoiceDocumentHTML(d model.InvoiceDraft) (string, error) {
	if d.ID != "" {
		stored, err := s.store.Invoice(d.ID)
		if err != nil {
			return "", err
		}
		if stored.Status != "draft" {
			xml, err := s.store.Artifact(d.ID, "cii-xml")
			if err != nil {
				return "", err
			}
			return pdfinvoice.HTML(xml, stored.IssuerSnapshot, &stored.Customer, pdfinvoice.PaymentQROptions{Enabled: !stored.DisablePaymentQR, HideDetails: stored.HidePaymentQRDetails})
		}
	}
	xml, draft, company, err := s.previewInvoiceXML(d)
	if err != nil {
		return "", err
	}
	return pdfinvoice.HTML(xml, &company, &draft.Customer, pdfinvoice.PaymentQROptions{Enabled: !draft.DisablePaymentQR, HideDetails: draft.HidePaymentQRDetails})
}

func (s *InvoiceService) InvoicePreviewFonts() string { return pdfinvoice.PreviewFonts() }
