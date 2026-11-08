package app

import "testing"

func TestDraftNumberRefreshOnOpen(t *testing.T) {
	s, err := NewInvoiceService(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err = s.SaveCompany(testCompany()); err != nil {
		t.Fatal(err)
	}
	d := testDraft()
	d.Number = ""
	saved, err := s.SaveInvoice(d)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Draft.Number != "RE-2026-0001" || saved.Draft.PaymentReference != saved.Draft.Number {
		t.Fatal("missing defaults", saved.Draft)
	}
	other, err := s.IssueInvoice(d)
	if err != nil {
		t.Fatal(err)
	}
	if other.Draft.Number != "RE-2026-0001" {
		t.Fatal("draft reserved number")
	}
	opened, err := s.OpenInvoice(saved.Draft.ID)
	if err != nil {
		t.Fatal(err)
	}
	if opened.Draft.Number != "RE-2026-0002" || opened.Draft.PaymentReference != opened.Draft.Number {
		t.Fatal("open did not refresh", opened.Draft)
	}
	issued, err := s.IssueInvoice(saved.Draft)
	if err != nil {
		t.Fatal(err)
	}
	if issued.Draft.Number != "RE-2026-0002" || issued.Draft.PaymentReference != issued.Draft.Number {
		t.Fatal("stale draft not renumbered", issued.Draft)
	}
}
