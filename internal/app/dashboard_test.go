package app

import (
	"strings"
	"testing"

	"papierleicht.de/e-rechnung-studio/internal/model"
	"papierleicht.de/e-rechnung-studio/internal/store"
)

func TestDashboardGroupsOpenItemsAndTasks(t *testing.T) {
	in := baseInput()
	credit := invoice19("c", "GS-1", "2026-10-02")
	credit.Credit = true
	in.Issued = []issuedTax{invoice19("a", "RE-1", "2026-10-05"), invoice19("b", "RE-2", "2026-10-06"), credit}
	in.Overview = store.PaymentOverview{
		Payments: []store.Payment{pay("p1", "b", "outgoing", "100.00", "2026-10-20", "bk1")},
		Invoices: []store.PaymentBalance{
			{ID: "a", Direction: "outgoing", Currency: "EUR", Remaining: "1190.00", DueDate: "2026-10-30"},
			{ID: "b", Direction: "outgoing", Currency: "EUR", Remaining: "1090.00", DueDate: "2026-11-15"},
			{ID: "d", Direction: "outgoing", Currency: "USD", Remaining: "50.00", DueDate: "2026-10-01"},
			{ID: "e", Direction: "outgoing", Currency: "EUR", Remaining: "0.00", DueDate: "2026-10-01", PlanNeedsReview: true},
			{ID: "c", Direction: "outgoing", Currency: "EUR", Remaining: "119.00", DueDate: "2026-10-10"},
			{ID: "r", Direction: "incoming", Currency: "EUR", Remaining: "300.00", DueDate: "2026-12-01"},
		},
	}
	// bk1 is fully allocated, bk2 is classified, bk3 (from an earlier
	// quarter) is still open.
	in.Bookings = []PaymentBooking{booking("bk1", "2026-10-20", 10_000), booking("bk2", "2026-10-21", -5_000), booking("bk3", "2026-05-02", 7_000)}
	in.Treatments["bk2"] = store.BookingTreatment{BookingID: "bk2", Kind: "private"}
	in.Received = []model.ReceivedInvoice{
		{ID: "x", Status: "active", Inspection: model.ImportInspection{IssueDate: "2026-10-01", Validation: model.ValidationResult{State: "valid"}}},
		{ID: "y", Status: "active", Inspection: model.ImportInspection{Validation: model.ValidationResult{State: "valid"}}},
		{ID: "z", Status: "misimport"},
	}

	r := buildDashboard(in, buildFinanceReport(in))
	if r.Receivable != (DashboardBucket{Count: 3, Amount: "2280.00", Foreign: 1}) {
		t.Errorf("receivable %+v", r.Receivable)
	}
	if r.ReceivableOverdue != (DashboardBucket{Count: 2, Amount: "1190.00", Foreign: 1}) {
		t.Errorf("receivable overdue %+v", r.ReceivableOverdue)
	}
	// The credit note is money to pay back, not a receivable.
	if r.Payable != (DashboardBucket{Count: 2, Amount: "419.00"}) {
		t.Errorf("payable %+v", r.Payable)
	}
	if r.PayableOverdue != (DashboardBucket{Count: 1, Amount: "119.00"}) {
		t.Errorf("payable overdue %+v", r.PayableOverdue)
	}
	var next []string
	for _, b := range r.NextDue {
		next = append(next, b.ID)
	}
	if strings.Join(next, ",") != "d,a,b" {
		t.Errorf("next due %v", next)
	}
	if r.UnassignedBookings != 1 || r.ReceivedToReview != 1 || r.PlansToReview != 1 {
		t.Errorf("tasks %+v", r)
	}
	for _, issue := range r.FinanceIssues {
		if dashboardOwnIssues[issue.Code] {
			t.Errorf("finance issue %q is already its own task", issue.Code)
		}
	}
}

func TestDashboardLeavesRefundedReductionsOutOfReceivables(t *testing.T) {
	in := baseInput()
	in.Overview = store.PaymentOverview{Invoices: []store.PaymentBalance{
		{ID: "a", Direction: "outgoing", Currency: "EUR", Remaining: "19.00", AwaitingCredit: "19.00", DueDate: "2026-10-01"},
		{ID: "b", Direction: "outgoing", Currency: "EUR", Remaining: "100.00", AwaitingCredit: "40.00", DueDate: "2026-10-01"},
	}}
	r := buildDashboard(in, buildFinanceReport(in))
	if r.Receivable != (DashboardBucket{Count: 1, Amount: "60.00"}) || r.RefundsToCredit != 2 {
		t.Fatalf("receivable %+v, refunds %d", r.Receivable, r.RefundsToCredit)
	}
}

func TestDashboardCountsOnlyMissedRatesAsOverdue(t *testing.T) {
	in := baseInput()
	in.Overview = store.PaymentOverview{Invoices: []store.PaymentBalance{
		{ID: "a", Direction: "outgoing", Currency: "EUR", Remaining: "600.00", DueDate: "2026-09-15", Installments: true, NextDate: "2026-11-15", OverdueAmount: "0.00"},
		{ID: "b", Direction: "outgoing", Currency: "EUR", Remaining: "600.00", DueDate: "2026-09-15", Installments: true, NextDate: "2026-10-01", OverdueAmount: "200.00"},
	}}
	r := buildDashboard(in, buildFinanceReport(in))
	if r.ReceivableOverdue != (DashboardBucket{Count: 1, Amount: "200.00"}) {
		t.Fatalf("overdue %+v", r.ReceivableOverdue)
	}
	if r.NextDue[0].ID != "b" {
		t.Fatalf("the missed rate comes first: %+v", r.NextDue)
	}
}
