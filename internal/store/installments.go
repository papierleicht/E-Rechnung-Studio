package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/shopspring/decimal"
	"papierleicht.de/e-rechnung-studio/internal/model"
	"strings"
	"time"
)

type Installment struct {
	Date   string `json:"date"`
	Amount string `json:"amount"`
}
type InstallmentPlan struct {
	InvoiceID    string        `json:"invoiceId"`
	Currency     string        `json:"currency"`
	BaselinePaid string        `json:"baselinePaid"`
	Rates        []Installment `json:"rates"`
	Note         string        `json:"note"`
	At           string        `json:"at"`
	Active       bool          `json:"active"`
}

func readPlan(q interface{ QueryRow(string, ...any) *sql.Row }, id string) (InstallmentPlan, error) {
	var p InstallmentPlan
	var raw []byte
	err := q.QueryRow("SELECT value FROM settings WHERE key=?", "installments:"+id).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return p, nil
	}
	if err != nil {
		return p, err
	}
	err = json.Unmarshal(raw, &p)
	return p, err
}
func (s *Store) InstallmentPlan(id string) (InstallmentPlan, error) { return readPlan(s.db, id) }
func (s *Store) PreviewInstallments(id, rate, first string) (InstallmentPlan, error) {
	o, err := s.PaymentOverview()
	if err != nil {
		return InstallmentPlan{}, err
	}
	for _, b := range o.Invoices {
		if b.ID == id && b.Direction == "outgoing" {
			scale, err := model.CurrencyScale(b.Currency)
			if err != nil {
				return InstallmentPlan{}, err
			}
			if len(rate) > 64 || strings.ContainsAny(rate, "eE") {
				return InstallmentPlan{}, errors.New("Gib einen gültigen Ratenbetrag ein.")
			}
			r, err := decimal.NewFromString(rate)
			if err != nil || !r.IsPositive() || !r.Equal(r.Round(scale)) {
				return InstallmentPlan{}, errors.New("Gib einen gültigen, positiven Ratenbetrag ein.")
			}
			start, err := time.Parse(time.DateOnly, first)
			if err != nil {
				return InstallmentPlan{}, errors.New("Gib die erste Fälligkeit ein.")
			}
			remaining, _ := decimal.NewFromString(b.Remaining)
			p := InstallmentPlan{InvoiceID: id, Currency: b.Currency, BaselinePaid: b.Paid, Active: true, Rates: []Installment{}}
			for i := 0; remaining.IsPositive(); i++ {
				if i >= 120 {
					return p, errors.New("Wähle einen höheren Ratenbetrag (höchstens 120 Raten).")
				}
				month := time.Date(start.Year(), start.Month()+time.Month(i), 1, 0, 0, 0, 0, time.UTC)
				last := month.AddDate(0, 1, -1).Day()
				day := min(start.Day(), last)
				date := time.Date(month.Year(), month.Month(), day, 0, 0, 0, 0, time.UTC)
				amount := decimal.Min(remaining, r)
				p.Rates = append(p.Rates, Installment{Date: date.Format(time.DateOnly), Amount: amount.StringFixed(scale)})
				remaining = remaining.Sub(amount)
			}
			if len(p.Rates) == 0 {
				return p, errors.New("Diese Rechnung ist bereits bezahlt.")
			}
			return p, nil
		}
	}
	return InstallmentPlan{}, errors.New("Die Kundenrechnung wurde nicht gefunden.")
}
func (s *Store) SaveInstallmentPlan(p InstallmentPlan) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	b, err := paymentTarget(tx, "outgoing", p.InvoiceID)
	if err != nil {
		return err
	}
	payments, err := readPayments(tx)
	if err != nil {
		return err
	}
	if b, err = settledBalance(tx, b, payments); err != nil {
		return err
	}
	scale, err := model.CurrencyScale(b.Currency)
	if err != nil {
		return err
	}
	if len(p.Note) > 2000 {
		return errors.New("Die Notiz darf höchstens 2000 Zeichen enthalten.")
	}
	p.Currency = b.Currency
	p.At = now()
	if p.Active {
		if len(p.Rates) == 0 || len(p.Rates) > 120 {
			return errors.New("Lege zwischen einer und 120 Raten an.")
		}
		sum := decimal.Zero
		previous := ""
		for i, r := range p.Rates {
			if _, e := time.Parse(time.DateOnly, r.Date); e != nil || r.Date < previous {
				return errors.New("Gib gültige, aufsteigend sortierte Fälligkeiten ein.")
			}
			if len(r.Amount) > 64 || strings.ContainsAny(r.Amount, "eE") {
				return errors.New("Gib einen gültigen Ratenbetrag ein.")
			}
			a, e := decimal.NewFromString(r.Amount)
			if e != nil || !a.IsPositive() || !a.Equal(a.Round(scale)) {
				return errors.New("Gib gültige, positive Ratenbeträge ein.")
			}
			p.Rates[i].Amount = a.StringFixed(scale)
			sum = sum.Add(a)
			previous = r.Date
		}
		remaining, _ := decimal.NewFromString(b.Remaining)
		if !remaining.IsPositive() || !sum.Equal(remaining) {
			return errors.New("Die Raten müssen zusammen genau den aktuellen Restbetrag ergeben. Berechne den Plan neu.")
		}
		p.BaselinePaid = b.Paid
	} else {
		old, e := readPlan(tx, p.InvoiceID)
		if e != nil {
			return e
		}
		if !old.Active {
			return errors.New("Es gibt keine aktive Ratenvereinbarung.")
		}
		if strings.TrimSpace(p.Note) == "" {
			return errors.New("Gib einen Grund für das Aufheben des Ratenplans an.")
		}
		old.Active = false
		old.Note = p.Note
		old.At = p.At
		p = old
	}
	raw, err := json.Marshal(p)
	if err != nil {
		return err
	}
	if _, err = tx.Exec("INSERT INTO settings(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value", "installments:"+p.InvoiceID, raw); err != nil {
		return err
	}
	// Every revision is retained alongside the current plan, including cancellation.
	if _, err = tx.Exec("INSERT INTO settings(key,value) VALUES(?,?)", "installment-history:"+newID("plan"), raw); err != nil {
		return err
	}
	return tx.Commit()
}
func applyPlan(b PaymentBalance, p InstallmentPlan) PaymentBalance {
	return applyPlanAt(b, p, time.Now().Format(time.DateOnly))
}

// applyPlanAt evaluates the plan on a given day; reminders use it with an explicit date.
func applyPlanAt(b PaymentBalance, p InstallmentPlan, today string) PaymentBalance {
	if !p.Active {
		return b
	}
	b.Installments = true
	paid, _ := decimal.NewFromString(b.Paid)
	baseline, _ := decimal.NewFromString(p.BaselinePaid)
	covered := decimal.Max(paid.Sub(baseline), decimal.Zero)
	b.PlanNeedsReview = paid.LessThan(baseline)
	scale, _ := model.CurrencyScale(b.Currency)
	due := decimal.Zero
	for _, r := range p.Rates {
		amount, _ := decimal.NewFromString(r.Amount)
		left := decimal.Max(amount.Sub(covered), decimal.Zero)
		covered = decimal.Max(covered.Sub(amount), decimal.Zero)
		if left.IsPositive() && b.NextDate == "" {
			b.NextDate = r.Date
			b.NextAmount = left.StringFixed(scale)
		}
		if r.Date < today {
			due = due.Add(left)
		}
	}
	b.OverdueAmount = due.StringFixed(scale)
	if b.State != "bezahlt" && b.State != "verrechnet" && b.State != StateAwaitingCredit {
		b.State = "offen"
		if paid.IsPositive() {
			b.State = "teilbezahlt"
		}
		if due.IsPositive() {
			b.State = "überfällig"
		}
	}
	return b
}
