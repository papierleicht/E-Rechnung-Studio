package store

import (
	"papierleicht.de/e-rechnung-studio/internal/model"
	"path/filepath"
	"testing"
	"time"
)

func TestCustomerPaymentTermPersistsWithoutOverwritingContact(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	c, err := s.SaveCustomer(model.Customer{Name: "Original"})
	if err != nil {
		t.Fatal(err)
	}
	d := model.NewDraft()
	d.CustomerID = c.ID
	d.Customer = c
	d.IssueDate = "2026-12-20"
	d.DueDate = "2027-01-19"
	c.Name = "Updated contact"
	if _, err = s.SaveCustomer(c); err != nil {
		t.Fatal(err)
	}
	d, err = s.SaveInvoice(d, "100")
	if err != nil {
		t.Fatal(err)
	}
	check := func(want int) {
		t.Helper()
		cs, e := s.Customers()
		if e != nil {
			t.Fatal(e)
		}
		if len(cs) != 1 || cs[0].Name != "Updated contact" || cs[0].PaymentTermDays == nil || *cs[0].PaymentTermDays != want {
			t.Fatalf("unexpected customer: %+v", cs)
		}
	}
	check(30)
	d.DueDate = d.IssueDate
	if _, err = s.Issue(d.ID, d, "100", func(model.InvoiceDraft) ([]byte, []byte, error) {
		return []byte("immutable XML"), []byte("immutable PDF"), nil
	}); err != nil {
		t.Fatal(err)
	}
	check(0)
	d.DueDate = "2027-01-01"
	if _, err = s.SaveInvoice(d, "100"); err == nil {
		t.Fatal("issued invoice modified")
	}
	check(0)
}
func TestNewDraftDefaultsToFourteenDays(t *testing.T) {
	d := model.NewDraft()
	issue, e := time.Parse("2006-01-02", d.IssueDate)
	if e != nil {
		t.Fatal(e)
	}
	if d.DueDate != issue.AddDate(0, 0, 14).Format("2006-01-02") {
		t.Fatalf("due date %s", d.DueDate)
	}
}

func TestCustomerTaxSourcePersists(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	c, err := s.SaveCustomer(model.Customer{Name: "Test"})
	if err != nil {
		t.Fatal(err)
	}
	d := model.NewDraft()
	d.CustomerID = c.ID
	d.Customer = c
	for _, source := range []string{"company", "manual", "customer", "company"} {
		d.TaxSource = source
		d, err = s.SaveInvoice(d, "0")
		if err != nil {
			t.Fatal(err)
		}
		customers, err := s.Customers()
		if err != nil {
			t.Fatal(err)
		}
		if customers[0].InvoiceSettings == nil || customers[0].InvoiceSettings.TaxSource != source {
			t.Fatalf("lost tax source %s", source)
		}
	}
}
