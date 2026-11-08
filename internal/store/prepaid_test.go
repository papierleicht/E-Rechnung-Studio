package store

import (
	"reflect"
	"strings"
	"testing"

	"papierleicht.de/e-rechnung-studio/internal/model"
)

func TestIssueRecognizesFullyPrepaidInvoices(t *testing.T) {
	cases := []struct {
		name, kind, gross, prepaid, status string
	}{
		{name: "fully paid invoice", kind: "invoice", gross: "119.00", prepaid: "119.0", status: "paid"},
		{name: "German decimal payment", kind: "invoice", gross: "119.00", prepaid: "119,00", status: "paid"},
		{name: "fully paid correction", kind: "corrected-invoice", gross: "119.00", prepaid: "119.00", status: "paid"},
		{name: "partial payment", kind: "invoice", gross: "119.00", prepaid: "50.00", status: "issued"},
		{name: "unpaid invoice", kind: "invoice", gross: "119.00", prepaid: "", status: "issued"},
		{name: "zero invoice", kind: "invoice", gross: "0.00", prepaid: "0.00", status: "issued"},
		{name: "credit note", kind: "credit-note", gross: "119.00", prepaid: "119.00", status: "issued"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			s := openShopTestStore(t)
			draft := model.NewDraft()
			draft.Kind, draft.PrepaidAmount = test.kind, test.prepaid
			draft, err := s.SaveInvoice(draft, test.gross)
			if err != nil {
				t.Fatal(err)
			}
			issued, err := s.Issue(draft.ID, draft, test.gross, func(model.InvoiceDraft) ([]byte, []byte, error) {
				return []byte("immutable XML"), []byte("immutable PDF"), nil
			})
			if err != nil {
				t.Fatal(err)
			}
			if issued.Status != test.status {
				t.Fatalf("status=%s, want=%s", issued.Status, test.status)
			}
			if test.status == "paid" && issued.PaidAt == "" {
				t.Fatalf("payment recognition timestamp missing, issued=%q", issued.IssuedAt)
			}
			if test.status != "paid" && issued.PaidAt != "" {
				t.Fatalf("unpaid invoice has payment timestamp=%q", issued.PaidAt)
			}
			persisted, err := s.Invoice(issued.ID)
			if err != nil || !reflect.DeepEqual(persisted, issued) {
				t.Fatalf("persisted=%+v, err=%v", persisted, err)
			}
			list, err := s.Invoices()
			if err != nil || len(list) != 1 || list[0].Status != test.status {
				t.Fatalf("invoice summary=%+v, err=%v", list, err)
			}
			// The payment ledger decides the status; a full prepayment is one entry.
			overview, err := s.PaymentOverview()
			if err != nil {
				t.Fatal(err)
			}
			want := 0
			if test.status == "paid" {
				want = 1
			}
			if len(overview.Payments) != want {
				t.Fatalf("payments=%+v, want %d", overview.Payments, want)
			}
			if want == 1 {
				p := overview.Payments[0]
				if p.InvoiceID != issued.ID || p.Amount != "119.00" || p.Date != issued.IssueDate || p.Source != "manual" {
					t.Fatalf("prepayment=%+v", p)
				}
			}
		})
	}
}

func TestIssuedShopInvoiceBooksTheShopPayment(t *testing.T) {
	for _, test := range []struct {
		name, paidAt, date, reason string
	}{
		{name: "shop reports the payment", paidAt: "2026-10-13T09:30:00Z", date: "2026-10-13", reason: "Bestellung #1001 im Shop bezahlt"},
		{name: "shop reports no date", date: "", reason: "Der Shop nennt kein Zahlungsdatum"},
	} {
		t.Run(test.name, func(t *testing.T) {
			s := openShopTestStore(t)
			order := seedShopOrder(t, s)
			if test.paidAt != "" {
				order.PaidAt, order.UpdatedAt = test.paidAt, "2026-10-14T13:00:00Z"
				var err error
				if order, _, err = s.UpsertShopOrder(order); err != nil {
					t.Fatal(err)
				}
			}
			draft, err := s.CreateShopInvoice(order.ID, shopDraft(order), order.Gross)
			if err != nil {
				t.Fatal(err)
			}
			issued, err := s.Issue(draft.ID, draft, order.Gross, func(model.InvoiceDraft) ([]byte, []byte, error) {
				return []byte("immutable XML"), []byte("immutable PDF"), nil
			})
			if err != nil || issued.Status != "paid" {
				t.Fatalf("issued=%+v, err=%v", issued, err)
			}
			overview, err := s.PaymentOverview()
			if err != nil || len(overview.Payments) != 1 {
				t.Fatalf("payments=%+v, err=%v", overview.Payments, err)
			}
			p := overview.Payments[0]
			date := test.date
			if date == "" {
				date = issued.IssueDate
			}
			if p.Source != "shop" || p.Date != date || p.Amount != "119.00" || !strings.Contains(p.Reason, test.reason) {
				t.Fatalf("shop payment=%+v", p)
			}
			if len(overview.Invoices) != 1 || overview.Invoices[0].State != "bezahlt" {
				t.Fatalf("balances=%+v", overview.Invoices)
			}
		})
	}
}

func TestDeletedIssuedShopInvoiceKeepsDuplicateProtection(t *testing.T) {
	s := openShopTestStore(t)
	order := seedShopOrder(t, s)
	order.Paid, order.Status, order.UpdatedAt = false, "open", "2026-10-14T13:00:00Z"
	order, _, err := s.UpsertShopOrder(order)
	if err != nil {
		t.Fatal(err)
	}
	draft, err := s.CreateShopInvoice(order.ID, shopDraft(order), order.Gross)
	if err != nil {
		t.Fatal(err)
	}
	draft.IssueDate = "2026-10-14"
	issued, err := s.Issue(draft.ID, draft, order.Gross, func(model.InvoiceDraft) ([]byte, []byte, error) {
		return []byte("immutable XML"), []byte("immutable PDF"), nil
	})
	if err != nil || issued.Status != "issued" {
		t.Fatalf("issued=%+v, err=%v", issued, err)
	}
	if _, _, err = s.DeleteIssuedInvoice(issued.ID, "Doppelt angelegt", func(model.InvoiceDraft, InvoiceLinks) error { return nil }); err != nil {
		t.Fatal(err)
	}
	stored, err := s.ShopOrder(order.ID)
	if err != nil || stored.InvoiceID != issued.ID || stored.ReviewReason != shopInvoiceMissingReason {
		t.Fatalf("order after deletion=%+v, %v", stored, err)
	}
	// A later poll keeps the tombstone, so no second invoice appears by itself.
	order.UpdatedAt = "2026-10-14T14:00:00Z"
	resynced, _, err := s.UpsertShopOrder(order)
	if err != nil || resynced.ReviewReason != shopInvoiceMissingReason {
		t.Fatalf("resynced=%+v, %v", resynced, err)
	}
}

func TestFullyPrepaidInvoiceCannotBeReopened(t *testing.T) {
	s := openShopTestStore(t)
	draft := model.NewDraft()
	draft.PrepaidAmount = "119,00"
	draft, err := s.SaveInvoice(draft, "119.00")
	if err != nil {
		t.Fatal(err)
	}
	issued, err := s.Issue(draft.ID, draft, "119.00", func(model.InvoiceDraft) ([]byte, []byte, error) {
		return []byte("immutable XML"), []byte("immutable PDF"), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.SetPaid(issued.ID, false); err == nil {
		t.Fatal("fully prepaid invoice reopened despite immutable payment snapshot")
	}
	persisted, err := s.Invoice(issued.ID)
	if err != nil || !reflect.DeepEqual(persisted, issued) {
		t.Fatalf("reopen attempt changed invoice: %+v, %v", persisted, err)
	}

	// A later payment on an invoice without an immutable full prepayment can
	// still be corrected through the existing payment-status control.
	unpaid := model.NewDraft()
	unpaid, err = s.SaveInvoice(unpaid, "119.00")
	if err != nil {
		t.Fatal(err)
	}
	unpaid, err = s.Issue(unpaid.ID, unpaid, "119.00", func(model.InvoiceDraft) ([]byte, []byte, error) {
		return []byte("immutable XML"), []byte("immutable PDF"), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.SetPaid(unpaid.ID, true); err != nil {
		t.Fatal(err)
	}
	if err = s.SetPaid(unpaid.ID, false); err != nil {
		t.Fatal(err)
	}
	persisted, err = s.Invoice(unpaid.ID)
	if err != nil || persisted.Status != "issued" || persisted.PaidAt != "" {
		t.Fatalf("ordinary invoice payment status=%+v, %v", persisted, err)
	}
}
