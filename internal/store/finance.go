package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/shopspring/decimal"
	"papierleicht.de/e-rechnung-studio/internal/model"
)

// AccountBalance is a balance the user read from the bank. The app does not
// fetch balances, so each one carries the date it was valid for.
type AccountBalance struct {
	AccountID string `json:"accountId"`
	Amount    string `json:"amount"`
	Date      string `json:"date"`
}

type FinanceSettings struct {
	// TaxBasis is "soll" (by date of supply) or "ist" (by payment date).
	TaxBasis string `json:"taxBasis"`
	// ReservePercent is a flat share of the result the user sets aside for
	// income tax. Empty means not configured; it is not a tax forecast.
	ReservePercent string           `json:"reservePercent"`
	Balances       []AccountBalance `json:"balances"`
	// VATPeriod is how often the Umsatzsteuer-Voranmeldung is filed:
	// "monthly", "quarterly" or "yearly" (only the annual return). Empty means
	// not chosen yet.
	VATPeriod string `json:"vatPeriod,omitempty"`
	// VATExtension is the Dauerfristverlängerung; SpecialPrepayment the
	// Sondervorauszahlung deducted in the last period of the year.
	VATExtension      bool   `json:"vatExtension,omitempty"`
	SpecialPrepayment string `json:"specialPrepayment,omitempty"`
	// Filing is who transmits the returns: "self" (default) or "advisor".
	Filing string `json:"filing,omitempty"`
	// Activity is what the business is for income tax: "trade" (Gewerbe,
	// Anlage G, Gewerbesteuer) or "freelance" (freiberuflich, Anlage S, no
	// Gewerbesteuer). Empty means not chosen yet; the user decides.
	Activity  string `json:"activity,omitempty"`
	UpdatedAt string `json:"updatedAt"`
}

const financeKey = "finance-settings-v1"

func (s *Store) FinanceSettings() (FinanceSettings, error) {
	f := FinanceSettings{TaxBasis: "soll", Balances: []AccountBalance{}}
	if err := s.readSetting(financeKey, &f); err != nil {
		return f, err
	}
	if f.Balances == nil {
		f.Balances = []AccountBalance{}
	}
	return f, nil
}

// SaveFinanceSettings keeps every saved version as history so changed
// balances or reserve rates remain traceable.
func (s *Store) SaveFinanceSettings(f FinanceSettings) (FinanceSettings, error) {
	if f.TaxBasis != "soll" && f.TaxBasis != "ist" {
		return f, errors.New("Wähle Soll- oder Ist-Versteuerung aus.")
	}
	f.ReservePercent = strings.TrimSpace(strings.ReplaceAll(f.ReservePercent, ",", "."))
	if f.ReservePercent != "" {
		p, err := decimal.NewFromString(f.ReservePercent)
		if err != nil || p.IsNegative() || p.GreaterThan(decimal.NewFromInt(100)) || !p.Equal(p.Round(2)) {
			return f, errors.New("Gib einen Rücklagensatz zwischen 0 und 100 % ein.")
		}
		f.ReservePercent = p.String()
	}
	switch f.VATPeriod {
	case "", "monthly", "quarterly", "yearly":
	default:
		return f, errors.New("Wähle monatlich, vierteljährlich oder jährlich.")
	}
	switch f.Filing {
	case "", "self", "advisor":
	default:
		return f, errors.New("Wähle, wer deine Steuererklärungen übermittelt.")
	}
	switch f.Activity {
	case "", "trade", "freelance":
	default:
		return f, errors.New("Wähle Gewerbe oder freiberuflich.")
	}
	f.SpecialPrepayment = strings.TrimSpace(strings.ReplaceAll(f.SpecialPrepayment, ",", "."))
	if f.SpecialPrepayment != "" {
		p, err := decimal.NewFromString(f.SpecialPrepayment)
		if err != nil || p.IsNegative() || !p.Equal(p.Round(2)) {
			return f, errors.New("Gib die Sondervorauszahlung als Betrag ein.")
		}
		f.SpecialPrepayment = p.StringFixed(2)
	}
	if !f.VATExtension || f.VATPeriod != "monthly" {
		// The Sondervorauszahlung only applies to monthly filers with extension.
		f.SpecialPrepayment = ""
	}
	seen := map[string]bool{}
	balances := []AccountBalance{}
	for _, b := range f.Balances {
		b.AccountID = strings.TrimSpace(b.AccountID)
		if b.AccountID == "" || len(b.AccountID) > 100 || seen[b.AccountID] {
			return f, errors.New("Gib jedes Konto nur einmal und mit gültiger Kennung an.")
		}
		seen[b.AccountID] = true
		amount, err := model.ParseAmount(b.Amount)
		if err != nil || !amount.Equal(amount.Round(2)) {
			return f, errors.New("Gib einen gültigen Kontostand mit höchstens zwei Nachkommastellen ein.")
		}
		if _, err = time.Parse(time.DateOnly, b.Date); err != nil {
			return f, errors.New("Gib das Datum des Kontostands an.")
		}
		b.Amount = amount.StringFixed(2)
		balances = append(balances, b)
	}
	f.Balances = balances
	f.UpdatedAt = now()
	raw, err := json.Marshal(f)
	if err != nil {
		return f, err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return f, err
	}
	defer tx.Rollback()
	if err = s.writeTracked(tx, financeKey, raw); err != nil {
		return f, err
	}
	if _, err = tx.Exec("INSERT INTO settings(key,value) VALUES(?,?)", "finance-settings-history:"+newID("evt"), raw); err != nil {
		return f, err
	}
	return f, tx.Commit()
}

// IssuedDocument is an issued invoice with the XML generated at issue time.
// Reports read tax data from that XML, never from today's company settings.
type IssuedDocument struct {
	Draft model.InvoiceDraft
	XML   []byte
}

func (s *Store) IssuedDocuments() ([]IssuedDocument, error) {
	rows, err := s.db.Query(`SELECT i.data, (SELECT a.data FROM artifacts a WHERE a.invoice_id=i.id AND a.kind='cii-xml' ORDER BY a.id DESC LIMIT 1) FROM invoices i WHERE i.status<>'draft' ORDER BY i.issue_date, i.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []IssuedDocument{}
	for rows.Next() {
		var raw []byte
		var xml sql.RawBytes
		if err = rows.Scan(&raw, &xml); err != nil {
			return nil, err
		}
		var d IssuedDocument
		if err = json.Unmarshal(raw, &d.Draft); err != nil {
			return nil, err
		}
		d.XML = append([]byte(nil), xml...)
		if s.scope.Load().covers(d.Draft.IssueDate) {
			out = append(out, d)
		}
	}
	return out, rows.Err()
}
