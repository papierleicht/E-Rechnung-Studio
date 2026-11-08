package app

import (
	"fmt"
	"sort"
	"time"

	"papierleicht.de/e-rechnung-studio/internal/store"
	validator "papierleicht.de/e-rechnung-studio/internal/validation"

	"github.com/shopspring/decimal"
)

// DashboardBucket is a group of open items. Amount sums EUR items only; items
// in other currencies are counted in Foreign.
type DashboardBucket struct {
	Count   int    `json:"count"`
	Amount  string `json:"amount"`
	Foreign int    `json:"foreign"`
}

// DashboardSummary answers "what needs attention today". Open items and tasks
// cover all periods; only FinanceIssues refer to the current quarter.
type DashboardSummary struct {
	Today string `json:"today"`

	// Money still to receive: open customer invoices.
	Receivable        DashboardBucket `json:"receivable"`
	ReceivableOverdue DashboardBucket `json:"receivableOverdue"`
	// Money still to pay: received invoices and credit notes to pay out.
	Payable        DashboardBucket `json:"payable"`
	PayableOverdue DashboardBucket `json:"payableOverdue"`
	// NextDue lists open customer invoices, earliest due date first.
	NextDue []store.PaymentBalance `json:"nextDue"`

	UnassignedBookings int `json:"unassignedBookings"`
	ReceivedToReview   int `json:"receivedToReview"`
	PlansToReview      int `json:"plansToReview"`
	RefundsToCredit    int `json:"refundsToCredit"`
	// TaxProfileMissing means no tax dates can show until the profile is set.
	TaxProfileMissing bool `json:"taxProfileMissing"`

	FinanceLabel string `json:"financeLabel"`
	FinanceFrom  string `json:"financeFrom"`
	FinanceTo    string `json:"financeTo"`
	// FinanceIssues are the open points of the quarter's finance report that
	// the dashboard does not already list as its own task.
	FinanceIssues []FinanceIssue `json:"financeIssues"`
}

const nextDueLimit = 5

// Issue codes the dashboard shows as separate, period-independent tasks.
var dashboardOwnIssues = map[string]bool{"unassigned_bookings": true, "received_needs_review": true}

func (s *InvoiceService) DashboardSummary() (DashboardSummary, error) {
	now := time.Now()
	quarter := (int(now.Month()) - 1) / 3
	start := time.Date(now.Year(), time.Month(quarter*3+1), 1, 0, 0, 0, 0, time.Local)
	end := start.AddDate(0, 3, -1)
	in, err := s.loadFinanceInput(start.Format(time.DateOnly), end.Format(time.DateOnly))
	if err != nil {
		return DashboardSummary{}, err
	}
	summary := buildDashboard(in, buildFinanceReport(in))
	summary.FinanceLabel = fmt.Sprintf("Q%d %d", quarter+1, now.Year())
	return summary, nil
}

func buildDashboard(in financeInput, report FinanceReport) DashboardSummary {
	r := DashboardSummary{Today: in.Today, FinanceFrom: in.From, FinanceTo: in.To, FinanceIssues: []FinanceIssue{}, NextDue: []store.PaymentBalance{}}
	credit := map[string]bool{}
	for _, v := range in.Issued {
		credit[v.ID] = v.Credit
	}
	sums := map[*DashboardBucket]decimal.Decimal{}
	add := func(b *DashboardBucket, currency string, amount decimal.Decimal) {
		b.Count++
		if currency != "" && currency != "EUR" {
			b.Foreign++
			return
		}
		sums[b] = sums[b].Add(amount)
	}
	r.TaxProfileMissing = !in.SmallBusiness && taxSetup(in.Settings, in.SmallBusiness).VATPeriod == ""
	for _, b := range in.Overview.Invoices {
		if b.PlanNeedsReview {
			r.PlansToReview++
		}
		if b.AwaitingCredit != "" {
			r.RefundsToCredit++
		}
		remaining := b.Collectable()
		if !remaining.IsPositive() {
			continue
		}
		total, overdue := &r.Payable, &r.PayableOverdue
		if b.Direction == "outgoing" && !credit[b.ID] {
			total, overdue = &r.Receivable, &r.ReceivableOverdue
			r.NextDue = append(r.NextDue, b)
		}
		add(total, b.Currency, remaining)
		if b.Installments {
			// An agreed plan replaces the invoice's due date: only missed rates are overdue.
			if due, _ := decimal.NewFromString(b.OverdueAmount); due.IsPositive() {
				add(overdue, b.Currency, decimal.Min(due, remaining))
			}
		} else if b.DueDate != "" && b.DueDate < in.Today {
			add(overdue, b.Currency, remaining)
		}
	}
	for _, b := range []*DashboardBucket{&r.Receivable, &r.ReceivableOverdue, &r.Payable, &r.PayableOverdue} {
		b.Amount = money(sums[b])
	}
	// Invoices without a due date come last.
	sort.SliceStable(r.NextDue, func(i, j int) bool {
		a, b := nextDueDate(r.NextDue[i]), nextDueDate(r.NextDue[j])
		if (a == "") != (b == "") {
			return b == ""
		}
		return a < b
	})
	if len(r.NextDue) > nextDueLimit {
		r.NextDue = r.NextDue[:nextDueLimit]
	}

	// A booking paid through a receipt is settled, as in the finance report,
	// and so is a PDF whose amounts were captured as a receipt.
	expenseOfBooking, expenseOfReceived := map[string]bool{}, map[string]bool{}
	for _, x := range in.Expenses {
		if x.BookingID != "" {
			expenseOfBooking[x.BookingID] = true
		}
		if x.ReceivedID != "" {
			expenseOfReceived[x.ReceivedID] = true
		}
	}
	for _, b := range in.Bookings {
		if in.Treatments[b.ID].Kind != "" || expenseOfBooking[b.ID] {
			continue
		}
		if left, err := bookingUnallocated(b, in.Overview.Payments); err == nil && !left.IsZero() {
			r.UnassignedBookings++
		}
	}
	// Same rule as the review hint in Ausgaben & Belege: a document needs a look
	// when it has no date or its check found something that can concern the Vorsteuer.
	for _, v := range in.Received {
		if v.Status == "misimport" || expenseOfReceived[v.ID] || choiceOf(in.InputTaxChoices, v.ID) != nil {
			continue
		}
		if v.Inspection.IssueDate == "" || !validator.ReceivedInputTax(v.Inspection.Validation).Allowed {
			r.ReceivedToReview++
		}
	}
	for _, issue := range report.Issues {
		if !dashboardOwnIssues[issue.Code] {
			r.FinanceIssues = append(r.FinanceIssues, issue)
		}
	}
	return r
}

// nextDueDate is the date a customer has to pay next: with a plan, its next rate.
func nextDueDate(b store.PaymentBalance) string {
	if b.Installments && b.NextDate != "" {
		return b.NextDate
	}
	return b.DueDate
}
