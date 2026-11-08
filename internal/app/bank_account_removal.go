package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"papierleicht.de/e-rechnung-studio/internal/banking"
	"papierleicht.de/e-rechnung-studio/internal/store"
)

// RemoveBankAccount deletes an account and all its bookings from this app, be
// it a retrieved account or one known only from CSV imports. The bank's own
// data is untouched. Bookings still assigned to an invoice or linked to an
// expense keep the account in place: those assignments are records of their
// own and are revoked first. The removal is logged.
//
//wails:ignore
func (s *InvoiceService) RemoveBankAccount(accountID string) (banking.RemovedAccount, error) {
	s.reconcileMu.Lock()
	defer s.reconcileMu.Unlock()
	s.paymentMu.Lock()
	defer s.paymentMu.Unlock()
	removed := banking.RemovedAccount{}
	if accountID == "" {
		return removed, banking.ErrAccountNotFound
	}
	rows, err := s.paymentBookings()
	if err != nil {
		return removed, err
	}
	ids := map[string]bool{}
	keep := rows[:0:0]
	source := "csv"
	for _, b := range rows {
		if b.AccountID != accountID {
			keep = append(keep, b)
			continue
		}
		ids[b.ID] = true
		if b.Source == "bank" {
			source = "bank"
		}
	}
	payments, expenses, err := s.store.BookingsInUse(ids)
	if err != nil {
		return removed, err
	}
	if payments > 0 || expenses > 0 {
		return removed, fmt.Errorf("Dieses Konto hat noch %s. Hebe sie zuerst auf, dann lässt sich das Konto entfernen.", inUse(payments, expenses))
	}
	// The snapshot goes first: a failure below only brings the rows back
	// from the bank data, while the other order would leave orphans.
	snapshot, err := json.Marshal(keep)
	if err != nil {
		return removed, err
	}
	if err = s.store.SavePaymentBankData(snapshot); err != nil {
		return removed, err
	}
	if s.bank != nil {
		removed, err = s.bank.RemoveAccount(accountID)
		if err == nil {
			source = "bank"
		} else if !errors.Is(err, banking.ErrAccountNotFound) {
			return removed, err
		}
	}
	if source == "csv" {
		if len(ids) == 0 {
			return removed, banking.ErrAccountNotFound
		}
		name := accountID
		if labels, e := s.store.CSVAccountLabels(); e == nil && labels[accountID] != "" {
			name = labels[accountID]
		}
		removed = banking.RemovedAccount{Name: name, IBAN: accountID, Bookings: len(ids)}
		for _, b := range rows {
			if b.AccountID != accountID {
				continue
			}
			if d := b.Data.BookingDate; removed.From == "" || d < removed.From {
				removed.From = d
			}
			if d := b.Data.BookingDate; d > removed.To {
				removed.To = d
			}
		}
	}
	if err = s.store.RecordBankAccountRemoval(store.BankAccountRemoval{AccountID: accountID, Name: removed.Name, IBAN: removed.IBAN, Source: source, Bookings: removed.Bookings, From: removed.From, To: removed.To}); err != nil {
		return removed, err
	}
	// A name would otherwise come back with the next import of the same IBAN.
	return removed, s.store.SetCSVAccountLabel(accountID, "")
}

// CSVAccountLabels names the accounts known only from CSV imports, by IBAN.
//
//wails:ignore
func (s *InvoiceService) CSVAccountLabels() (map[string]string, error) {
	return s.store.CSVAccountLabels()
}

// SetCSVAccountLabel names an account known only from CSV imports; an empty
// label removes the name.
//
//wails:ignore
func (s *InvoiceService) SetCSVAccountLabel(accountID, label string) error {
	label, err := banking.NormalizeAccountLabel(label)
	if err != nil {
		return err
	}
	s.paymentMu.Lock()
	defer s.paymentMu.Unlock()
	rows, err := s.paymentBookings()
	if err != nil {
		return err
	}
	for _, b := range rows {
		if b.AccountID == accountID && b.Source == "csv" {
			return s.store.SetCSVAccountLabel(accountID, label)
		}
	}
	return banking.ErrAccountNotFound
}

// inUse names what keeps bookings in use, e.g. "2 Zuordnungen zu Rechnungen".
func inUse(payments, expenses int) string {
	plural := func(n int, one, many string) string {
		if n == 1 {
			return "1 " + one
		}
		return fmt.Sprintf("%d %s", n, many)
	}
	parts := []string{}
	if payments > 0 {
		parts = append(parts, plural(payments, "Zuordnung", "Zuordnungen")+" zu Rechnungen")
	}
	if expenses > 0 {
		parts = append(parts, plural(expenses, "Verknüpfung", "Verknüpfungen")+" mit Ausgaben")
	}
	return strings.Join(parts, " und ")
}
