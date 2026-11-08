package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/shopspring/decimal"
	"papierleicht.de/e-rechnung-studio/internal/model"
)

// Payment entries are immutable. Revocation records a reason and timestamp;
// corrections are new entries. Bank snapshots travel with the ordinary backup.
type Payment struct {
	ID        string          `json:"id"`
	InvoiceID string          `json:"invoiceId"`
	Direction string          `json:"direction"`
	Amount    string          `json:"amount"`
	Currency  string          `json:"currency"`
	Date      string          `json:"date"`
	Source    string          `json:"source"`
	BookingID string          `json:"bookingId"`
	Booking   json.RawMessage `json:"booking,omitempty"`
	Reason    string          `json:"reason"`
	// Reduction marks a refund the customer no longer owes, as the user decided.
	Reduction        bool   `json:"reduction,omitempty"`
	CreatedAt        string `json:"createdAt"`
	RevokedAt        string `json:"revokedAt"`
	RevocationReason string `json:"revocationReason"`
}
type PaymentBalance struct {
	Installments    bool   `json:"installments"`
	NextDate        string `json:"nextDate"`
	NextAmount      string `json:"nextAmount"`
	OverdueAmount   string `json:"overdueAmount"`
	PlanNeedsReview bool   `json:"planNeedsReview"`
	ID              string `json:"id"`
	Direction       string `json:"direction"`
	Number          string `json:"number"`
	Partner         string `json:"partner"`
	IBAN            string `json:"iban"`
	Currency        string `json:"currency"`
	DueDate         string `json:"dueDate"`
	Gross           string `json:"gross"`
	Paid            string `json:"paid"`
	Remaining       string `json:"remaining"`
	Overpaid        string `json:"overpaid"`
	State           string `json:"state"`
	// Credit marks a credit note; its remaining amount is owed to the customer.
	Credit bool `json:"credit"`
	// Credited is the part settled between an invoice and its credit notes.
	Credited string `json:"credited,omitempty"`
	// AwaitingCredit is the refunded part of Remaining that no credit note covers yet.
	AwaitingCredit string `json:"awaitingCredit,omitempty"`
	corrects       string
}

// StateAwaitingCredit is an invoice whose whole rest was refunded as a reduction.
const StateAwaitingCredit = "gutschrift-offen"

// Collectable is what the customer still owes: refunds waiting for a credit
// note are not asked for again.
func (b PaymentBalance) Collectable() decimal.Decimal {
	remaining, _ := decimal.NewFromString(b.Remaining)
	awaiting, _ := decimal.NewFromString(b.AwaitingCredit)
	return remaining.Sub(awaiting)
}

type PaymentOverview struct {
	Invoices []PaymentBalance `json:"invoices"`
	Payments []Payment        `json:"payments"`
}

const paymentKey = "payment-ledger-v1"

func paymentLabel(p Payment) string {
	return fmt.Sprintf("Zahlung %s %s vom %s", p.Amount, p.Currency, germanDay(p.Date))
}

func readPayments(q interface{ QueryRow(string, ...any) *sql.Row }) ([]Payment, error) {
	out := []Payment{}
	var raw []byte
	err := q.QueryRow("SELECT value FROM settings WHERE key=?", paymentKey).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(raw, &out)
	return out, err
}

// Each change also appends an immutable follow-up entry. The ledger's revocation
// fields are a convenient projection; the original and correction remain in audit.
type paymentEvent struct {
	ID      string  `json:"id"`
	Action  string  `json:"action"`
	At      string  `json:"at"`
	Reason  string  `json:"reason"`
	Payment Payment `json:"payment"`
}

func (s *Store) writePayments(tx *sql.Tx, payments []Payment) error {
	old, err := readPayments(tx)
	if err != nil {
		return err
	}
	known := map[string]Payment{}
	for _, p := range old {
		known[p.ID] = p
	}
	events := []paymentEvent{}
	var eventRaw []byte
	err = tx.QueryRow("SELECT value FROM settings WHERE key='payment-events-v1'").Scan(&eventRaw)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if err == nil {
		if err = json.Unmarshal(eventRaw, &events); err != nil {
			return err
		}
	}
	for _, p := range payments {
		prior, exists := known[p.ID]
		if !exists {
			events = append(events, paymentEvent{ID: newID("evt"), Action: "recorded", At: p.CreatedAt, Reason: p.Reason, Payment: p})
			if err = s.logChange(tx, AuditEntry{Action: AuditRecorded, Object: AuditPayment, ObjectID: p.ID, Label: paymentLabel(p), Reason: p.Reason, After: stateJSON(p), Sum: paymentSum(p)}); err != nil {
				return err
			}
		}
		if p.RevokedAt != "" && prior.RevokedAt == "" {
			events = append(events, paymentEvent{ID: newID("evt"), Action: "revoked", At: p.RevokedAt, Reason: p.RevocationReason, Payment: p})
			if err = s.logChange(tx, AuditEntry{Action: AuditRevoked, Object: AuditPayment, ObjectID: p.ID, Label: paymentLabel(p), Reason: p.RevocationReason, Before: stateJSON(prior), After: stateJSON(p), Sum: paymentSum(p)}); err != nil {
				return err
			}
		}
	}
	eventRaw, err = json.Marshal(events)
	if err != nil {
		return err
	}
	if _, err = tx.Exec("INSERT INTO settings(key,value) VALUES('payment-events-v1',?) ON CONFLICT(key) DO UPDATE SET value=excluded.value", eventRaw); err != nil {
		return err
	}
	raw, err := json.Marshal(payments)
	if err != nil {
		return err
	}
	_, err = tx.Exec("INSERT INTO settings(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value", paymentKey, raw)
	return err
}

func paymentTarget(q interface{ QueryRow(string, ...any) *sql.Row }, direction, id string) (PaymentBalance, error) {
	b := PaymentBalance{ID: id, Direction: direction}
	var raw []byte
	if direction == "outgoing" {
		var status string
		err := q.QueryRow("SELECT number,customer_name,gross,status,data FROM invoices WHERE id=?", id).Scan(&b.Number, &b.Partner, &b.Gross, &status, &raw)
		if err != nil {
			return b, err
		}
		if status == "draft" {
			return b, errors.New("Entwürfe können keine Zahlungen erhalten.")
		}
		var v struct {
			Kind, Currency, DueDate string
			CorrectsInvoiceID       string
			Customer                struct{ IBAN string }
		}
		if err = json.Unmarshal(raw, &v); err != nil {
			return b, err
		}
		b.Currency = v.Currency
		b.DueDate = v.DueDate
		b.IBAN = v.Customer.IBAN
		b.Credit = v.Kind == "credit-note"
		b.corrects = v.CorrectsInvoiceID
	} else if direction == "incoming" {
		err := q.QueryRow("SELECT data FROM received_invoices WHERE id=?", id).Scan(&raw)
		if err != nil {
			return b, err
		}
		var v struct {
			Status     string
			Inspection struct {
				InvoiceNumber, SellerName, Currency, DueDate, IBAN string
				Totals                                             struct{ Gross, Due string }
			}
		}
		if err = json.Unmarshal(raw, &v); err != nil {
			return b, err
		}
		if v.Status == "misimport" {
			return b, errors.New("Fehlimporte können nicht zugeordnet werden.")
		}
		b.DueDate = v.Inspection.DueDate
		b.IBAN = v.Inspection.IBAN
		b.Number = v.Inspection.InvoiceNumber
		b.Partner = v.Inspection.SellerName
		b.Currency = v.Inspection.Currency
		b.Gross = v.Inspection.Totals.Gross
		// A final invoice deducts earlier payments (BT-113). What is left to settle
		// is the amount due (BT-115), not the gross total.
		if due := v.Inspection.Totals.Due; due != "" && b.Gross != "" {
			if _, parseErr := decimal.NewFromString(due); parseErr == nil {
				b.Gross = due
			}
		}
	} else {
		return b, errors.New("Ungültige Rechnungsart.")
	}
	if b.Currency == "" {
		b.Currency = "EUR"
	}
	return b, nil
}
func balance(b PaymentBalance, payments []Payment) PaymentBalance {
	scale, _ := model.CurrencyScale(b.Currency)
	gross, _ := decimal.NewFromString(b.Gross)
	paid, reduced := decimal.Zero, decimal.Zero
	for _, p := range payments {
		if p.InvoiceID == b.ID && p.Direction == b.Direction && p.RevokedAt == "" {
			v, _ := decimal.NewFromString(p.Amount)
			paid = paid.Add(v)
			if p.Reduction {
				reduced = reduced.Sub(v)
			}
		}
	}
	credited, _ := decimal.NewFromString(b.Credited)
	remaining := gross.Sub(paid).Sub(credited)
	b.Paid = paid.StringFixed(scale)
	b.Remaining = decimal.Max(remaining, decimal.Zero).StringFixed(scale)
	b.Overpaid = decimal.Max(remaining.Neg(), decimal.Zero).StringFixed(scale)
	b.AwaitingCredit = ""
	awaiting := decimal.Min(reduced, decimal.Max(remaining, decimal.Zero))
	if awaiting.IsPositive() {
		b.AwaitingCredit = awaiting.StringFixed(scale)
	}
	collectable := remaining.Sub(awaiting)
	b.State = "offen"
	if remaining.LessThanOrEqual(decimal.Zero) {
		b.State = "bezahlt"
		if credited.IsPositive() {
			b.State = "verrechnet"
		}
	} else if !collectable.IsPositive() {
		b.State = StateAwaitingCredit
	} else if paid.IsPositive() {
		b.State = "teilbezahlt"
	}
	if collectable.IsPositive() && b.DueDate != "" && b.DueDate < time.Now().Format(time.DateOnly) {
		b.State = "überfällig"
	}
	return b
}

// creditSettlements nets issued credit notes against the invoices they correct,
// oldest first. Both sides are settled by the same amount; what a credit note
// exceeds the open invoice by stays open as money owed to the customer.
func creditSettlements(q rowQuerier, payments []Payment) (map[string]decimal.Decimal, error) {
	rows, err := q.Query("SELECT id FROM invoices WHERE status<>'draft' AND CAST(data AS TEXT) LIKE '%\"correctsInvoiceId\"%' ORDER BY issue_date, number")
	if err != nil {
		return nil, err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	settled := map[string]decimal.Decimal{}
	for _, id := range ids {
		credit, err := paymentTarget(q, "outgoing", id)
		if err != nil {
			return nil, err
		}
		if !credit.Credit || credit.corrects == "" {
			continue
		}
		original, err := paymentTarget(q, "outgoing", credit.corrects)
		if errors.Is(err, sql.ErrNoRows) || err == nil && original.Currency != credit.Currency {
			continue
		}
		if err != nil {
			return nil, err
		}
		left, _ := decimal.NewFromString(balance(credit, payments).Remaining)
		open, _ := decimal.NewFromString(balance(original, payments).Remaining)
		amount := decimal.Min(left, open.Sub(settled[original.ID]))
		if amount.IsPositive() {
			settled[credit.ID] = settled[credit.ID].Add(amount)
			settled[original.ID] = settled[original.ID].Add(amount)
		}
	}
	return settled, nil
}

// settledBalance is the balance of one invoice after payments and credit notes.
func settledBalance(q rowQuerier, b PaymentBalance, payments []Payment) (PaymentBalance, error) {
	settled, err := creditSettlements(q, payments)
	if err != nil {
		return b, err
	}
	if amount := settled[b.ID]; b.Direction == "outgoing" && amount.IsPositive() {
		scale, _ := model.CurrencyScale(b.Currency)
		b.Credited = amount.StringFixed(scale)
	}
	return balance(b, payments), nil
}

func (s *Store) PaymentOverview() (PaymentOverview, error) {
	o := PaymentOverview{Invoices: []PaymentBalance{}}
	var err error
	o.Payments, err = readPayments(s.db)
	if err != nil {
		return o, err
	}
	rows, err := s.db.Query("SELECT id,'outgoing' FROM invoices WHERE status<>'draft' UNION ALL SELECT id,'incoming' FROM received_invoices WHERE COALESCE(json_extract(data,'$.status'),'active')<>'misimport'")
	if err != nil {
		return o, err
	}
	type target struct{ id, direction string }
	targets := []target{}
	for rows.Next() {
		var t target
		if err = rows.Scan(&t.id, &t.direction); err != nil {
			rows.Close()
			return o, err
		}
		targets = append(targets, t)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return o, err
	}
	settled, err := creditSettlements(s.db, o.Payments)
	if err != nil {
		return o, err
	}
	for _, t := range targets {
		b, e := paymentTarget(s.db, t.direction, t.id)
		if e != nil {
			return o, e
		}
		if _, e = decimal.NewFromString(b.Gross); e != nil {
			continue
		}
		if amount := settled[b.ID]; b.Direction == "outgoing" && amount.IsPositive() {
			scale, _ := model.CurrencyScale(b.Currency)
			b.Credited = amount.StringFixed(scale)
		}
		b = balance(b, o.Payments)
		if b.Direction == "outgoing" {
			p, e := readPlan(s.db, b.ID)
			if e != nil {
				return o, e
			}
			b = applyPlan(b, p)
		}
		if b.DueDate == "" || s.scope.Load().covers(b.DueDate) {
			o.Invoices = append(o.Invoices, b)
		}
	}
	if scope := s.scope.Load(); scope != nil {
		kept := []Payment{}
		for _, p := range o.Payments {
			if scope.covers(p.Date) {
				kept = append(kept, p)
			}
		}
		o.Payments = kept
	}
	return o, nil
}
func (s *Store) AddPayment(p Payment) (Payment, error) {
	if len(p.Amount) > 64 || strings.ContainsAny(p.Amount, "eE") {
		return p, errors.New("Gib einen gültigen Zahlungsbetrag ein.")
	}
	scale, err := model.CurrencyScale(p.Currency)
	if err != nil {
		return p, errors.New("Gib eine gültige ISO-Währung ein.")
	}
	amount, err := decimal.NewFromString(p.Amount)
	if err != nil || amount.IsZero() || !amount.Equal(amount.Round(scale)) || amount.Shift(scale).Abs().GreaterThan(decimal.NewFromInt(9007199254740991)) {
		return p, errors.New("Gib einen gültigen Betrag mit den Nachkommastellen der Währung ein.")
	}
	if _, err = time.Parse(time.DateOnly, p.Date); err != nil {
		return p, errors.New("Das Zahlungsdatum ist ungültig.")
	}
	if p.Reduction && (!amount.IsNegative() || p.Direction != "outgoing") {
		return p, errors.New("Nur eine Erstattung an einen Kunden kann den geschuldeten Betrag mindern.")
	}
	p.Reason = strings.TrimSpace(p.Reason)
	if p.Reason == "" || len(p.Reason) > 2000 {
		return p, errors.New("Gib einen Buchungsgrund an.")
	}

	if p.Source != "manual" && p.Source != "bank" && p.Source != "csv" {
		return p, errors.New("Die Zahlungsquelle ist ungültig.")
	}
	tx, err := s.db.Begin()
	if err != nil {
		return p, err
	}
	defer tx.Rollback()
	target, err := paymentTarget(tx, p.Direction, p.InvoiceID)
	if err != nil {
		return p, err
	}
	if _, e := decimal.NewFromString(target.Gross); e != nil {
		return p, errors.New("Die Rechnung hat noch keinen auslesbaren Gesamtbetrag.")
	}
	if target.Currency != p.Currency {
		return p, errors.New("Die Währungen stimmen nicht überein.")
	}
	payments, err := readPayments(tx)
	if err != nil {
		return p, err
	}
	if p.BookingID != "" {
		var raw []byte
		var treatment BookingTreatment
		e := tx.QueryRow("SELECT value FROM settings WHERE key=?", "booking-treatment:"+p.BookingID).Scan(&raw)
		if e != nil && !errors.Is(e, sql.ErrNoRows) {
			return p, e
		}
		if e == nil {
			if e = json.Unmarshal(raw, &treatment); e != nil {
				return p, e
			}
			if treatment.Kind != "" {
				return p, errors.New("Hebe zuerst die Einordnung als andere Geldbewegung auf.")
			}
		}
		used := decimal.Zero
		for _, old := range payments {
			if old.BookingID == p.BookingID && old.RevokedAt == "" {
				if old.InvoiceID == p.InvoiceID && old.Direction == p.Direction {
					return p, errors.New("Dieser Bankumsatz ist dieser Rechnung bereits zugeordnet. Hebe zuerst die Zuordnung auf.")
				}
				v, _ := decimal.NewFromString(old.Amount)
				if old.Direction == "incoming" {
					v = v.Neg()
				}
				used = used.Add(v)
			}
		}
		var snapshot struct{ Data struct{ Amount int64 } }
		if len(p.Booking) > 0 {
			if err = json.Unmarshal(p.Booking, &snapshot); err != nil {
				return p, err
			}
			total := decimal.NewFromInt(snapshot.Data.Amount).Shift(-scale)
			allocated := amount
			if p.Direction == "incoming" {
				allocated = allocated.Neg()
			}
			if total.IsZero() || allocated.Sign() != total.Sign() || used.Add(allocated).Abs().GreaterThan(total.Abs()) {
				return p, errors.New("Die Zuordnung übersteigt den noch verfügbaren Betrag des Bankumsatzes.")
			}
		}
	}

	p.ID = newID("pay")
	p.CreatedAt = now()
	p.RevokedAt = ""
	p.RevocationReason = ""
	p.Amount = amount.StringFixed(scale)
	payments = append(payments, p)
	if err = s.writePayments(tx, payments); err != nil {
		return p, err
	}
	if err = syncPaymentStatus(tx, p.Direction, p.InvoiceID, payments); err != nil {
		return p, err
	}
	return p, tx.Commit()
}
func syncPaymentStatus(tx *sql.Tx, direction, id string, payments []Payment) error {
	if direction != "outgoing" {
		return nil
	}
	b, err := paymentTarget(tx, direction, id)
	if err != nil {
		return err
	}
	if b, err = settledBalance(tx, b, payments); err != nil {
		return err
	}
	status, paidAt := "issued", ""
	if b.State == "bezahlt" {
		status = "paid"
		paidAt = now()
	}
	_, err = tx.Exec("UPDATE invoices SET status=?,data=CAST(json_set(data,'$.status',?,'$.paidAt',?,'$.updatedAt',?) AS BLOB),updated_at=? WHERE id=?", status, status, paidAt, now(), now(), id)
	return err
}
func (s *Store) RevokePayment(id, reason string) error {
	reason = strings.TrimSpace(reason)
	if reason == "" || len(reason) > 2000 {
		return errors.New("Gib einen Grund für die Aufhebung an.")
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	payments, err := readPayments(tx)
	if err != nil {
		return err
	}
	for i, p := range payments {
		if p.ID == id {
			if p.RevokedAt != "" {
				return errors.New("Die Zuordnung ist bereits aufgehoben.")
			}
			payments[i].RevokedAt = now()
			payments[i].RevocationReason = reason
			if err = s.writePayments(tx, payments); err != nil {
				return err
			}
			if err = syncPaymentStatus(tx, p.Direction, p.InvoiceID, payments); err != nil {
				return err
			}
			return tx.Commit()
		}
	}
	return fmt.Errorf("Die Zahlung wurde nicht gefunden")
}

// PaymentBankData is a credential-free snapshot, included in existing backups.
func (s *Store) PaymentBankData() (json.RawMessage, error) {
	v := json.RawMessage(`[]`)
	err := s.readSetting("payment-bank-v1", &v)
	return v, err
}
func (s *Store) SavePaymentBankData(v json.RawMessage) error {
	return s.writeSetting("payment-bank-v1", v)
}

func (s *Store) migratePayments() error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = s.migratePaymentsIn(tx); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) migratePaymentsIn(tx *sql.Tx) error {
	var count int
	if err := tx.QueryRow("SELECT COUNT(*) FROM settings WHERE key=?", paymentKey).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	rows, err := tx.Query("SELECT id,gross,COALESCE(json_extract(data,'$.currency'),'EUR'),COALESCE(NULLIF(json_extract(data,'$.paidAt'),''),updated_at) FROM invoices WHERE status='paid'")
	if err != nil {
		return err
	}
	payments := []Payment{}
	for rows.Next() {
		p := Payment{Direction: "outgoing", Source: "manual", Reason: "Übernahme des bisherigen Bezahlt-Status", CreatedAt: now()}
		var at string
		if err = rows.Scan(&p.InvoiceID, &p.Amount, &p.Currency, &at); err != nil {
			rows.Close()
			return err
		}
		p.ID = "legacy-" + p.InvoiceID
		p.Date = at
		if len(p.Date) > 10 {
			p.Date = p.Date[:10]
		}
		payments = append(payments, p)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	return s.writePayments(tx, payments)
}
