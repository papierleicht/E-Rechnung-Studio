package app

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"papierleicht.de/e-rechnung-studio/internal/model"
)

// PurchaseDocument is a received invoice or recorded receipt a purchase price
// can come from, e.g. the Ankaufbeleg of a used car.
type PurchaseDocument struct {
	// Source is what an invoice line stores: "received:<id>" or "expense:<id>".
	Source  string `json:"source"`
	Kind    string `json:"kind"` // received or expense
	Number  string `json:"number"`
	Partner string `json:"partner"`
	Date    string `json:"date"`
	Gross   string `json:"gross"`
	Label   string `json:"label"`
	// UsedBy names the invoices that already take a purchase price from it.
	UsedBy []string `json:"usedBy"`
}

// PurchaseDocuments lists the documents a purchase price can be taken from,
// newest first. Misimported invoices and cancelled receipts are left out.
func (s *InvoiceService) PurchaseDocuments() ([]PurchaseDocument, error) {
	uses, err := s.store.PurchaseSourceUses()
	if err != nil {
		return nil, err
	}
	out := []PurchaseDocument{}
	received, err := s.store.ListReceived()
	if err != nil {
		return nil, err
	}
	for _, r := range received {
		if r.Status == "misimport" {
			continue
		}
		in := r.Inspection
		d := PurchaseDocument{Source: "received:" + r.ID, Kind: "received", Number: in.InvoiceNumber, Partner: in.SellerName, Date: in.IssueDate, Gross: in.Totals.Gross}
		d.Label = purchaseLabel("Eingangsrechnung", firstNonEmpty(d.Number, r.ReceiptNumber), d.Partner)
		out = append(out, d)
	}
	expenses, err := s.store.Expenses()
	if err != nil {
		return nil, err
	}
	for _, e := range expenses {
		if e.CancelledAt != "" {
			continue
		}
		d := PurchaseDocument{Source: "expense:" + e.ID, Kind: "expense", Number: firstNonEmpty(e.DocumentNumber, e.Number), Partner: e.Supplier, Date: e.Date, Gross: e.Gross}
		d.Label = purchaseLabel("Beleg", e.Number, e.Supplier)
		out = append(out, d)
	}
	for i := range out {
		out[i].UsedBy = append([]string{}, uses[out[i].Source]...)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Date > out[j].Date })
	return out, nil
}

func purchaseLabel(kind, number, partner string) string {
	parts := []string{kind}
	if number != "" {
		parts = append(parts, number)
	}
	label := strings.Join(parts, " ")
	if partner != "" {
		label += " · " + partner
	}
	return label
}

// SetPurchasePrices records the purchase prices of Differenzbesteuerung items
// on an issued invoice and the documents they come from, by line ID. They
// never appear on the invoice; the finance figures use them for the tax on
// the margin.
func (s *InvoiceService) SetPurchasePrices(id string, prices map[string]model.PurchaseAssignment) error {
	if id == "" {
		return errors.New("Die Rechnung wurde nicht gefunden.")
	}
	clean := map[string]model.PurchaseAssignment{}
	for lineID, a := range prices {
		a.Price, a.Source, a.SourceLabel = strings.TrimSpace(a.Price), strings.TrimSpace(a.Source), strings.TrimSpace(a.SourceLabel)
		if a.Source != "" && !strings.HasPrefix(a.Source, "received:") && !strings.HasPrefix(a.Source, "expense:") {
			return errors.New("Der gewählte Beleg ist unbekannt.")
		}
		if a.Price != "" {
			v, ok := parsePurchasePrice(a.Price)
			if !ok {
				return fmt.Errorf("„%s“ ist kein gültiger Einkaufspreis.", a.Price)
			}
			a.Price = v.StringFixed(2)
		}
		clean[lineID] = a
	}
	return s.store.SetPurchasePrices(id, clean)
}
