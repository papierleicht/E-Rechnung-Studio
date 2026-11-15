package store

import (
	"strings"
	"testing"

	"papierleicht.de/e-rechnung-studio/internal/model"
)

func TestSetPurchasePricesOnIssuedInvoice(t *testing.T) {
	s := openTest(t)
	d := model.NewDraft()
	d.IssueDate, d.DeliveryDate, d.DueDate = "2026-10-05", "2026-10-05", "2026-10-19"
	d.TaxTreatment = "margin-goods"
	d.Lines = []model.InvoiceLine{{ID: "car", Name: "Gebrauchtwagen", Quantity: "1", UnitPrice: "5000", TaxCategory: "E"}}
	saved, err := s.SaveInvoice(d, "5000")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.SetPurchasePrices(saved.ID, map[string]model.PurchaseAssignment{"car": {Price: "4000.00"}}); err == nil {
		t.Fatal("prices of a draft set outside the editor")
	}
	if _, err = s.Issue(saved.ID, saved, "5000", func(model.InvoiceDraft) ([]byte, []byte, error) {
		return []byte("xml"), []byte("pdf"), nil
	}); err != nil {
		t.Fatal(err)
	}
	if err = s.SetPurchasePrices(saved.ID, map[string]model.PurchaseAssignment{"car": {Price: "4000.00", Source: "expense:x", SourceLabel: "Beleg B-1 · Privat"}, "unknown": {Price: "1.00"}}); err != nil {
		t.Fatal(err)
	}
	got, err := s.Invoice(saved.ID)
	if err != nil || got.Lines[0].PurchasePrice != "4000.00" || got.Lines[0].PurchaseSource != "expense:x" || got.Status == "draft" {
		t.Fatalf("invoice %+v, %v", got.Lines, err)
	}
	entries, err := s.AuditEntries(AuditFilter{Object: AuditInvoice})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, e := range entries {
		if e.Action == AuditChanged && strings.Contains(e.Label, "Einkaufspreise") && strings.Contains(e.After, "4000.00") {
			found = true
		}
	}
	if !found {
		t.Fatalf("no log entry in %+v", entries)
	}
	uses, err := s.PurchaseSourceUses()
	if err != nil || len(uses["expense:x"]) != 1 || uses["expense:x"][0] == "Entwurf" {
		t.Fatalf("uses %v, %v", uses, err)
	}
	if _, err = s.LockPeriod("", "2026-10-31"); err != nil {
		t.Fatal(err)
	}
	if err = s.SetPurchasePrices(saved.ID, map[string]model.PurchaseAssignment{"car": {Price: "3000.00"}}); err == nil || !strings.Contains(err.Error(), "festgeschrieben") {
		t.Fatalf("closed period changed: %v", err)
	}
}
