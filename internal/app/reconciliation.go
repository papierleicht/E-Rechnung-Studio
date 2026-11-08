package app

import (
	"errors"
	"fmt"
	"github.com/shopspring/decimal"
	"papierleicht.de/e-rechnung-studio/internal/model"
	"papierleicht.de/e-rechnung-studio/internal/store"
	"papierleicht.de/e-rechnung-studio/internal/taxforms"
	"regexp"
	"sort"
	"strings"
	"unicode"
)

type PaymentSuggestion struct {
	Invoice store.PaymentBalance `json:"invoice"`
	Reasons []string             `json:"reasons"`
	Certain bool                 `json:"certain"`
}
type BookingReview struct {
	Booking     PaymentBooking      `json:"booking"`
	Remaining   string              `json:"remaining"`
	Suggestions []PaymentSuggestion `json:"suggestions"`
	// Expenses are open receipts this outgoing booking may have paid.
	Expenses []ExpenseSuggestion `json:"expenses"`
}

// ExpenseSuggestion is a recorded, unpaid receipt. Reasons is empty when only
// the direction fits.
type ExpenseSuggestion struct {
	ID             string   `json:"id"`
	Number         string   `json:"number"`
	DocumentNumber string   `json:"documentNumber"`
	Supplier       string   `json:"supplier"`
	Date           string   `json:"date"`
	Gross          string   `json:"gross"`
	Reasons        []string `json:"reasons"`
}
type OtherBooking struct {
	Booking   PaymentBooking         `json:"booking"`
	Treatment store.BookingTreatment `json:"treatment"`
}
type Reconciliation struct {
	Other     []OtherBooking        `json:"other"`
	Automatic int                   `json:"automatic"`
	Pending   []BookingReview       `json:"pending"`
	Overview  store.PaymentOverview `json:"overview"`
}

func normalized(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return unicode.ToLower(r)
	}, s)
}
func referenceMatches(description, number string) bool {
	if strings.TrimSpace(number) == "" {
		return false
	}
	pattern := `(?i)(^|[^\pL\pN])` + regexp.QuoteMeta(strings.TrimSpace(number)) + `($|[^\pL\pN])`
	return regexp.MustCompile(pattern).MatchString(description)
}
func bookingUnallocated(b PaymentBooking, payments []store.Payment) (decimal.Decimal, error) {
	scale, err := model.CurrencyScale(b.Data.Currency)
	if err != nil {
		return decimal.Zero, err
	}
	left := decimal.NewFromInt(b.Data.Amount).Shift(-scale)
	for _, p := range payments {
		if p.BookingID == b.ID && p.RevokedAt == "" {
			a, e := decimal.NewFromString(p.Amount)
			if e != nil {
				return left, e
			}
			if p.Direction == "incoming" {
				a = a.Neg()
			}
			left = left.Sub(a)
		}
	}
	return left, nil
}
func suggestionsFor(b PaymentBooking, left decimal.Decimal, invoices []store.PaymentBalance) []PaymentSuggestion {
	out := []PaymentSuggestion{}
	for _, inv := range invoices {
		if inv.Currency != b.Data.Currency || inv.State == "bezahlt" || inv.State == "verrechnet" || inv.PlanNeedsReview {
			continue
		}
		signed := left
		if inv.Direction == "incoming" {
			signed = signed.Neg()
		}
		if !signed.IsPositive() {
			continue
		}
		reasons := []string{}
		ref := referenceMatches(b.Data.Description+" "+b.Data.CustomerReference, inv.Number)
		party := normalized(inv.Partner) != "" && normalized(inv.Partner) == normalized(b.Data.CounterpartyName)
		iban := normalized(inv.IBAN) != "" && normalized(inv.IBAN) == normalized(b.Data.CounterpartyIBAN)
		remaining, err := decimal.NewFromString(inv.Remaining)
		if err != nil {
			continue
		}
		if ref {
			reasons = append(reasons, "Rechnungsnummer")
		}
		if signed.Equal(remaining) {
			reasons = append(reasons, "Betrag")
		}
		if party {
			reasons = append(reasons, "Geschäftspartner")
		}
		if iban {
			reasons = append(reasons, "IBAN")
		}
		if len(reasons) == 0 {
			continue
		}
		out = append(out, PaymentSuggestion{Invoice: inv, Reasons: reasons, Certain: ref && len(strings.TrimSpace(inv.Number)) >= 4 && (party || iban) && signed.LessThanOrEqual(remaining)})
	}
	return out
}

// expenseSuggestions lists open receipts for an outgoing euro booking, the
// likely ones first.
func expenseSuggestions(b PaymentBooking, left decimal.Decimal, expenses []store.Expense) []ExpenseSuggestion {
	out := []ExpenseSuggestion{}
	if !left.IsNegative() || b.Data.Currency != "EUR" {
		return out
	}
	for _, x := range expenses {
		if x.PaidDate != "" || x.BookingID != "" {
			continue
		}
		s := ExpenseSuggestion{ID: x.ID, Number: x.Number, DocumentNumber: x.DocumentNumber, Supplier: x.Supplier, Date: x.Date, Gross: x.Gross, Reasons: []string{}}
		if referenceMatches(b.Data.Description+" "+b.Data.CustomerReference, x.DocumentNumber) {
			s.Reasons = append(s.Reasons, "Belegnummer")
		}
		if gross, err := decimal.NewFromString(x.Gross); err == nil && gross.Equal(left.Neg()) {
			s.Reasons = append(s.Reasons, "Betrag")
		}
		partner, supplier := normalized(b.Data.CounterpartyName), normalized(x.Supplier)
		if partner != "" && supplier != "" && (strings.Contains(partner, supplier) || strings.Contains(supplier, partner)) {
			s.Reasons = append(s.Reasons, "Geschäftspartner")
		}
		out = append(out, s)
	}
	sort.SliceStable(out, func(i, j int) bool { return len(out[i].Reasons) > len(out[j].Reasons) })
	return out
}

// AssignBookingToExpense marks a recorded receipt as paid by this booking. A
// receipt has no partial payments, so the amounts must match.
func (s *InvoiceService) AssignBookingToExpense(bookingID, expenseID string) (store.Expense, error) {
	s.reconcileMu.Lock()
	defer s.reconcileMu.Unlock()
	var zero store.Expense
	bookings, err := s.PaymentBookings()
	if err != nil {
		return zero, err
	}
	o, err := s.Payments()
	if err != nil {
		return zero, err
	}
	for _, b := range bookings {
		if b.ID != bookingID {
			continue
		}
		left, err := bookingUnallocated(b, o.Payments)
		if err != nil {
			return zero, err
		}
		expenses, err := s.store.ActiveExpenses()
		if err != nil {
			return zero, err
		}
		for _, x := range expenses {
			if x.ID != expenseID {
				continue
			}
			gross, err := decimal.NewFromString(x.Gross)
			switch {
			case err != nil || x.PaidDate != "":
				return zero, errors.New("Dieser Beleg ist schon bezahlt.")
			case b.Data.Currency != "EUR" || !gross.Equal(left.Neg()):
				return zero, fmt.Errorf("Der Umsatz über %s € passt nicht zum Beleg über %s €. Korrigiere den Beleg oder erfasse den Umsatz als eigene Ausgabe.", left.Neg().StringFixed(2), gross.StringFixed(2))
			}
			x.PaidDate, x.PaidWith, x.BookingID = b.Data.BookingDate, "bank", b.ID
			return s.store.SaveExpense(x, validExpenseCategory, taxforms.AssetCategory)
		}
		return zero, errors.New("Der Beleg wurde nicht gefunden.")
	}
	return zero, errors.New("Der Bankumsatz wurde nicht gefunden.")
}

// ReconcilePayments only books a unique reference with corroborating party/IBAN
// evidence. A prior revocation or restored hidden booking requires a fresh
// manual confirmation.
func (s *InvoiceService) ReconcilePayments() (Reconciliation, error) {
	s.reconcileMu.Lock()
	defer s.reconcileMu.Unlock()
	r := Reconciliation{Pending: []BookingReview{}, Other: []OtherBooking{}}
	treatments, err := s.store.BookingTreatments()
	if err != nil {
		return r, err
	}
	excluded := map[string]store.BookingTreatment{}
	for _, t := range treatments {
		excluded[t.BookingID] = t
	}
	// A booking captured as receipt is settled; it appears under the receipts.
	expenses, err := s.store.ActiveExpenses()
	if err != nil {
		return r, err
	}
	asExpense := map[string]bool{}
	for _, x := range expenses {
		if x.BookingID != "" {
			asExpense[x.BookingID] = true
		}
	}
	bookings, err := s.PaymentBookings()
	if err != nil {
		return r, err
	}
	o, err := s.Payments()
	if err != nil {
		return r, err
	}
	for _, b := range bookings {
		if asExpense[b.ID] {
			continue
		}
		treatment, previouslyTreated := excluded[b.ID]
		if previouslyTreated && treatment.Kind != "" {
			r.Other = append(r.Other, OtherBooking{Booking: b, Treatment: treatment})
			continue
		}
		left, e := bookingUnallocated(b, o.Payments)
		if e != nil || left.IsZero() {
			continue
		}
		revoked, allocated := false, false
		for _, p := range o.Payments {
			if p.BookingID == b.ID {
				if p.RevokedAt != "" {
					revoked = true
				} else {
					allocated = true
				}
			}
		}
		suggestions := suggestionsFor(b, left, o.Invoices)
		strong := []PaymentSuggestion{}
		referenceCount := 0
		for _, suggestion := range suggestions {
			if suggestion.Certain {
				strong = append(strong, suggestion)
			}
			for _, why := range suggestion.Reasons {
				if why == "Rechnungsnummer" {
					referenceCount++
					break
				}
			}
		}
		if len(strong) == 1 && referenceCount == 1 && !revoked && !allocated && !previouslyTreated {
			target := strong[0].Invoice
			_, e = s.RecordPayment(store.Payment{InvoiceID: target.ID, Direction: target.Direction, BookingID: b.ID, Source: b.Source, Reason: "Automatischer Bankabgleich: " + strings.Join(strong[0].Reasons, ", ")})
			if e != nil {
				return r, e
			}
			r.Automatic++
			o, err = s.Payments()
			if err != nil {
				return r, err
			}
			continue
		}
		scale, _ := model.CurrencyScale(b.Data.Currency)
		r.Pending = append(r.Pending, BookingReview{Booking: b, Remaining: left.StringFixed(scale), Suggestions: suggestions, Expenses: expenseSuggestions(b, left, expenses)})
	}
	r.Overview = o
	return r, nil
}
func (s *InvoiceService) InstallmentPlan(id string) (store.InstallmentPlan, error) {
	return s.store.InstallmentPlan(id)
}
func (s *InvoiceService) PreviewInstallments(id, rate, first string) (store.InstallmentPlan, error) {
	return s.store.PreviewInstallments(id, rate, first)
}
func (s *InvoiceService) SaveInstallmentPlan(p store.InstallmentPlan) error {
	if p.InvoiceID == "" {
		return errors.New("Wähle eine Kundenrechnung aus.")
	}
	return s.store.SaveInstallmentPlan(p)
}

func (s *InvoiceService) SetBookingTreatment(id, kind, note string) error {
	rows, err := s.PaymentBookings()
	if err != nil {
		return err
	}
	for _, b := range rows {
		if b.ID == id {
			return s.store.SaveBookingTreatment(store.BookingTreatment{BookingID: id, Kind: kind, Note: note})
		}
	}
	return errors.New("Bankumsatz nicht gefunden.")
}
