package app

import (
	"database/sql"
	"errors"

	"papierleicht.de/e-rechnung-studio/internal/banking"
)

// finTSProductID is empty in official releases: papierleicht's registered ID
// reaches paying customers at runtime and must not be embedded in a binary.
// Self-built distributions with their own registration can set it with
// -ldflags="-X papierleicht.de/e-rechnung-studio/internal/app.finTSProductID=<registered ID>".
var finTSProductID = ""

type BankingService struct {
	service  *banking.Service
	invoices *InvoiceService
}

// NewBankingService stores its data in the application database db.
func NewBankingService(dataDir string, db *sql.DB) (*BankingService, error) {
	s, e := banking.New(db, dataDir, finTSProductID)
	if e != nil {
		return nil, e
	}
	return &BankingService{service: s}, nil
}

//wails:ignore
func (s *BankingService) Close() error                 { return s.service.Close() }
func (s *BankingService) SetProductID(id string) error { return s.service.SetProductID(id) }

// Overview adds the names of accounts known only from CSV imports.
func (s *BankingService) Overview() (banking.Overview, error) {
	o, err := s.service.Overview()
	if err != nil || s.invoices == nil {
		return o, err
	}
	o.CSVLabels, err = s.invoices.CSVAccountLabels()
	return o, err
}
func (s *BankingService) Connect(c banking.Connection, pin string, remember bool) (banking.Operation, error) {
	return s.service.Connect(c, pin, remember)
}
func (s *BankingService) Sync(accountID, pin, from, to string) (banking.Operation, error) {
	return s.service.Sync(accountID, pin, from, to)
}
func (s *BankingService) Status(id string) (banking.Operation, error) {
	result, err := s.service.Status(id)
	if err == nil && result.Status == "done" && s.invoices != nil {
		if _, e := s.invoices.ReconcilePayments(); e != nil {
			result.Message += " Deine Umsätze sind gespeichert. Starte den Rechnungsabgleich unter Konto & Finanzen bitte noch einmal selbst."
		}
	}
	return result, err
}
func (s *BankingService) SubmitTAN(id, challengeID, tan string) error {
	return s.service.SubmitTAN(id, challengeID, tan)
}
func (s *BankingService) Cancel(id string) error { return s.service.Cancel(id) }
func (s *BankingService) Bookings(id string) ([]banking.Booking, error) {
	return s.service.Bookings(id)
}

// SetAccountLabel names a retrieved account or, failing that, one known only
// from CSV imports.
func (s *BankingService) SetAccountLabel(id, label string) error {
	err := s.service.SetAccountLabel(id, label)
	if s.invoices == nil || !errors.Is(err, banking.ErrAccountNotFound) {
		return err
	}
	return s.invoices.SetCSVAccountLabel(id, label)
}

// RemoveAccount deletes an account with all its bookings from the app.
func (s *BankingService) RemoveAccount(id string) (banking.RemovedAccount, error) {
	if s.invoices == nil {
		return banking.RemovedAccount{}, errors.New("Das Konto lässt sich gerade nicht entfernen.")
	}
	return s.invoices.RemoveBankAccount(id)
}
func (s *BankingService) ForgetPIN(id string) error { return s.service.ForgetPIN(id) }
