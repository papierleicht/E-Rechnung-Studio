package app

import (
	"errors"
	"fmt"
	"papierleicht.de/e-rechnung-studio/internal/model"
	"time"
)

// CreateCorrectionDraft creates a separate credit note referencing the original.
// The original remains issued; only issuing the new document makes the correction final.
func (s *InvoiceService) CreateCorrectionDraft(id string) (model.InvoiceView, error) {
	original, err := s.store.Invoice(id)
	if err != nil {
		return model.InvoiceView{}, err
	}
	if original.Status == "draft" || original.Kind != "invoice" {
		return model.InvoiceView{}, errors.New("Eine Korrekturgutschrift kannst du nur für eine ausgestellte Rechnung erstellen.")
	}
	draft := original
	draft.CorrectsInvoiceID = original.ID
	draft.PrecedingInvoiceNumber, draft.PrecedingInvoiceDate = original.Number, original.IssueDate
	draft.ID, draft.Number, draft.Kind, draft.Status = "", "", "credit-note", "draft"
	draft.CreatedAt, draft.UpdatedAt, draft.IssuedAt, draft.PaidAt = "", "", "", ""
	draft.OriginalXML, draft.GeneratedXML, draft.GeneratedAt, draft.LastValidationState = "", "", "", ""
	draft.IssuerSnapshot = nil
	draft.QuoteID, draft.QuoteNumber = "", ""
	draft.RecurringID, draft.RecurringName = "", ""
	draft.PrepaidAmount = ""
	draft.IssueDate = time.Now().Format("2006-01-02")
	draft.DueDate, draft.PaymentTerms, draft.PaymentReference = "", "", ""
	draft.PaymentTerms = "Erstattung oder Verrechnung nach Vereinbarung."
	draft.Notes = fmt.Sprintf("Rechnungskorrektur zu Rechnung %s vom %s.", original.Number, original.IssueDate)
	draft.Lines = append([]model.InvoiceLine(nil), original.Lines...)
	draft.Adjustments = append([]model.InvoiceAdjustment(nil), original.Adjustments...)
	return s.SaveInvoice(draft)
}
