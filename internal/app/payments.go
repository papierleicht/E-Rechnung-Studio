package app

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	fints "github.com/BergerAPI/go-fints"
	"github.com/shopspring/decimal"
	"papierleicht.de/e-rechnung-studio/internal/model"
	"papierleicht.de/e-rechnung-studio/internal/store"
)

type PaymentBooking struct {
	ID        string            `json:"id"`
	AccountID string            `json:"accountId"`
	Source    string            `json:"source"`
	Data      fints.Transaction `json:"data"`
	Duplicate bool              `json:"duplicate"`
}

func (s *InvoiceService) Payments() (store.PaymentOverview, error) { return s.store.PaymentOverview() }
func (s *InvoiceService) PaymentBookings() ([]PaymentBooking, error) {
	s.paymentMu.Lock()
	defer s.paymentMu.Unlock()
	return s.paymentBookings()
}
func (s *InvoiceService) paymentBookings() ([]PaymentBooking, error) {
	raw, err := s.store.PaymentBankData()
	if err != nil {
		return nil, err
	}
	out := []PaymentBooking{}
	if err = json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	known := map[string]int{}
	for i, b := range out {
		known[b.ID] = i
	}
	if s.bank != nil {
		overview, e := s.bank.Overview()
		if e != nil {
			return nil, e
		}
		for _, a := range overview.Accounts {
			rows, e := s.bank.Bookings(a.ID)
			if e != nil {
				return nil, e
			}
			for _, b := range rows {
				current := PaymentBooking{ID: b.ID, AccountID: b.AccountID, Source: "bank", Data: b.Data}
				if index, exists := known[b.ID]; exists {
					out[index] = current
				} else {
					known[b.ID] = len(out)
					out = append(out, current)
				}
			}
		}
	}
	if s.ReadOnly() {
		return out, nil
	}
	raw, err = json.Marshal(out)
	if err != nil {
		return nil, err
	}
	return out, s.store.SavePaymentBankData(raw)
}
func (s *InvoiceService) RecordPayment(p store.Payment) (store.Payment, error) {
	// Imported amounts and provenance are resolved locally, never trusted from UI.
	if p.Source != "manual" {
		rows, err := s.PaymentBookings()
		if err != nil {
			return p, err
		}
		found := false
		for _, b := range rows {
			if b.ID == p.BookingID {
				found = true
				p.Source = b.Source
				p.Currency = b.Data.Currency
				p.Date = b.Data.BookingDate
				scale, e := model.CurrencyScale(b.Data.Currency)
				if e != nil {
					return p, errors.New("Die Währung des Bankumsatzes ist ungültig.")
				}
				amount := decimal.NewFromInt(b.Data.Amount).Shift(-scale)
				if p.Direction == "incoming" {
					amount = amount.Neg()
				}
				if strings.TrimSpace(p.Amount) != "" {
					if len(p.Amount) > 64 || strings.ContainsAny(p.Amount, "eE") {
						return p, errors.New("Gib einen gültigen Zuordnungsbetrag ein.")
					}
					part, e := decimal.NewFromString(p.Amount)
					if e != nil || !part.IsPositive() || !part.Equal(part.Round(scale)) || part.GreaterThan(amount.Abs()) {
						return p, errors.New("Gib einen positiven Zuordnungsbetrag ein, der nicht höher ist als der Bankumsatz.")
					}
					if amount.IsNegative() {
						part = part.Neg()
					}
					amount = part
				}
				p.Amount = amount.StringFixed(scale)
				p.Booking, _ = json.Marshal(b)
				break
			}
		}
		if !found {
			return p, errors.New("Bankumsatz nicht gefunden.")
		}
	} else {
		p.BookingID = ""
		p.Booking = nil
	}
	return s.store.AddPayment(p)
}
func (s *InvoiceService) RevokePayment(id, reason string) error {
	return s.store.RevokePayment(id, reason)
}

// PreviewPaymentCSV reads a bank's statement export. Preview never writes.
// Import rechecks duplicates.
func (s *InvoiceService) PreviewPaymentCSV(account, text string) ([]PaymentBooking, error) {
	if strings.TrimSpace(account) == "" {
		return nil, errors.New("Gib eine eindeutige Kontokennung ein, z. B. die IBAN.")
	}
	if len(text) > 5_000_000 {
		return nil, errors.New("Die CSV-Datei ist zu groß (maximal 5 MB).")
	}
	rows, err := parseBankCSV(text)
	if err != nil {
		return nil, err
	}
	existing, err := s.PaymentBookings()
	if err != nil {
		return nil, err
	}
	known := map[string]bool{}
	for _, b := range existing {
		known[b.ID] = true
	}
	out := []PaymentBooking{}
	counts := map[string]int{}
	for i, row := range rows {
		scale, e := model.CurrencyScale(row.currency)
		if e != nil {
			return nil, fmt.Errorf("Die Währung „%s“ im Umsatz %d ist kein gültiger ISO-Code.", row.currency, i+1)
		}
		if !row.amount.Equal(row.amount.Round(scale)) || row.amount.Shift(scale).Abs().GreaterThan(decimal.NewFromInt(9007199254740991)) {
			return nil, fmt.Errorf("Der Betrag von Umsatz %d (%s) ist ungültig.", i+1, row.date)
		}
		b := PaymentBooking{AccountID: strings.ToUpper(strings.ReplaceAll(account, " ", "")), Source: "csv", Data: fints.Transaction{BookingDate: row.date, Amount: row.amount.Shift(scale).IntPart(), Currency: row.currency, CounterpartyName: row.partner, CounterpartyIBAN: row.iban, Description: row.purpose, BankReference: row.reference}}
		identity, _ := json.Marshal([]any{b.AccountID, b.Data})
		h := sha256.Sum256(identity)
		fp := hex.EncodeToString(h[:])
		counts[fp]++
		h = sha256.Sum256([]byte(fmt.Sprintf("%s:%d", fp, counts[fp])))
		b.ID = "csv-" + hex.EncodeToString(h[:])
		b.Duplicate = known[b.ID]
		known[b.ID] = true
		out = append(out, b)
	}
	return out, nil
}
func (s *InvoiceService) ImportPaymentCSV(account, text string) (int, error) {
	rows, err := s.PreviewPaymentCSV(account, text)
	if err != nil {
		return 0, err
	}
	s.paymentMu.Lock()
	defer s.paymentMu.Unlock()
	existing, err := s.paymentBookings()
	if err != nil {
		return 0, err
	}
	known := map[string]bool{}
	for _, b := range existing {
		known[b.ID] = true
	}
	added := 0
	for _, b := range rows {
		if !known[b.ID] {
			b.Duplicate = false
			existing = append(existing, b)
			known[b.ID] = true
			added++
		}
	}
	raw, err := json.Marshal(existing)
	if err != nil {
		return 0, err
	}
	return added, s.store.SavePaymentBankData(raw)
}
