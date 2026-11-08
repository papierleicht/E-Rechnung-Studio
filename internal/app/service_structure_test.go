package app

import (
	"reflect"
	"testing"
)

func TestSavedViewAndApplicationLists(t *testing.T) {
	s, err := NewInvoiceService(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err := s.SaveCompany(testCompany()); err != nil {
		t.Fatal(err)
	}
	saved, err := s.SaveInvoice(testDraft())
	if err != nil {
		t.Fatal(err)
	}
	persisted, err := s.store.Invoice(saved.Draft.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(saved.Draft, persisted) {
		t.Fatal("returned draft differs from persisted draft")
	}
	if saved.Draft.ID == "" || saved.Draft.Status != "draft" || saved.Draft.UpdatedAt == "" {
		t.Fatal("missing saved metadata")
	}
	reopened, err := s.OpenInvoice(saved.Draft.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(saved.Totals, reopened.Totals) || !reflect.DeepEqual(saved.Validation, reopened.Validation) {
		t.Fatal("saved calculations differ from reopened invoice")
	}
	lists, err := s.ApplicationLists()
	if err != nil {
		t.Fatal(err)
	}
	if len(lists.Invoices) != 1 || lists.Invoices[0].ID != saved.Draft.ID {
		t.Fatalf("missing saved invoice: %+v", lists)
	}
	bootstrap, err := s.Bootstrap()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(lists.Customers, bootstrap.Customers) || !reflect.DeepEqual(lists.Products, bootstrap.Products) || !reflect.DeepEqual(lists.Invoices, bootstrap.Invoices) {
		t.Fatal("list refresh differs from bootstrap")
	}
}
