package store

import (
	"errors"
	"papierleicht.de/e-rechnung-studio/internal/model"
	"path/filepath"
	"testing"
)

func TestAutomaticNumberSequence(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	fresh := model.NewDraft()
	fresh.IssueDate = "2026-09-15"
	refresh := func(d model.InvoiceDraft) model.InvoiceDraft {
		t.Helper()
		d, e := s.RefreshDraftNumber(d)
		if e != nil {
			t.Fatal(e)
		}
		return d
	}
	issue := func(d model.InvoiceDraft) model.InvoiceDraft {
		t.Helper()
		d, e := s.SaveInvoice(d, "100")
		if e != nil {
			t.Fatal(e)
		}
		d, e = s.Issue(d.ID, d, "100", func(model.InvoiceDraft) ([]byte, []byte, error) { return []byte("xml"), []byte("pdf"), nil })
		if e != nil {
			t.Fatal(e)
		}
		return d
	}
	a, b := refresh(fresh), refresh(fresh)
	if a.Number != "RE-2026-0001" || a.PaymentReference != a.Number || !a.AutomaticNumber || b.Number != a.Number {
		t.Fatalf("bad initial previews: %+v / %+v", a, b)
	}
	a = issue(a)
	b = refresh(b)
	if b.Number != "RE-2026-0002" || b.PaymentReference != b.Number {
		t.Fatalf("stale preview %+v", b)
	}
	// A stale preview is resolved again in the issuance transaction.
	c := refresh(fresh)
	b = issue(b)
	c = issue(c)
	if c.Number != "RE-2026-0003" || c.PaymentReference != c.Number {
		t.Fatalf("stale issuance %+v", c)
	}
	if refresh(a).Number != a.Number {
		t.Fatal("issued number changed")
	}
	custom := fresh
	custom.Number = "RE-2026-0042"
	custom.PaymentReference = "Bestellung 7"
	custom = issue(custom)
	next := refresh(fresh)
	if next.Number != "RE-2026-0043" {
		t.Fatalf("manual standard number ignored: %s", next.Number)
	}
	next.PaymentReference = "Mein Verwendungszweck"
	next.IssueDate = "2027-01-01"
	next = refresh(next)
	if next.Number != "RE-2027-0001" || next.PaymentReference != "Mein Verwendungszweck" {
		t.Fatalf("year/custom reference: %+v", next)
	}
	// Failed artifact generation must not consume a sequence number.
	failed, e := s.SaveInvoice(refresh(fresh), "100")
	if e != nil {
		t.Fatal(e)
	}
	_, e = s.Issue(failed.ID, failed, "100", func(model.InvoiceDraft) ([]byte, []byte, error) { return nil, nil, errors.New("test") })
	if e == nil {
		t.Fatal("expected failure")
	}
	if refresh(fresh).Number != "RE-2026-0043" {
		t.Fatal("failed issuance consumed number")
	}
}
