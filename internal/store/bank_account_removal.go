package store

import (
	"encoding/json"
	"sort"
)

// BankAccountRemoval records that an account and its bookings left the app,
// so the removal stays traceable.
type BankAccountRemoval struct {
	At        string `json:"at"`
	AccountID string `json:"accountId"`
	Name      string `json:"name"`
	IBAN      string `json:"iban"`
	// Source is "bank" for a retrieved account and "csv" for an imported one.
	Source   string `json:"source"`
	Bookings int    `json:"bookings"`
	From     string `json:"from"`
	To       string `json:"to"`
}

const bankAccountRemovalPrefix = "bank-account-removal:"

func (s *Store) RecordBankAccountRemoval(r BankAccountRemoval) error {
	r.At = now()
	return s.writeSetting(bankAccountRemovalPrefix+newID("evt"), r)
}

// BankAccountRemovals lists the removals, oldest first.
func (s *Store) BankAccountRemovals() ([]BankAccountRemoval, error) {
	out := []BankAccountRemoval{}
	rows, err := s.db.Query("SELECT value FROM settings WHERE key LIKE ?", bankAccountRemovalPrefix+"%")
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var raw []byte
		var r BankAccountRemoval
		if err = rows.Scan(&raw); err != nil {
			return out, err
		}
		if err = json.Unmarshal(raw, &r); err != nil {
			return out, err
		}
		out = append(out, r)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].At < out[j].At })
	return out, rows.Err()
}

// BookingsInUse counts what still builds on the given bank bookings: payments
// that are assigned to an invoice and not revoked, and expenses linked to one.
func (s *Store) BookingsInUse(ids map[string]bool) (payments, expenses int, err error) {
	all, err := readPayments(s.db)
	if err != nil {
		return 0, 0, err
	}
	for _, p := range all {
		if p.BookingID != "" && p.RevokedAt == "" && ids[p.BookingID] {
			payments++
		}
	}
	linked, err := s.ActiveExpenses()
	if err != nil {
		return 0, 0, err
	}
	for _, e := range linked {
		if e.BookingID != "" && ids[e.BookingID] {
			expenses++
		}
	}
	return payments, expenses, nil
}
