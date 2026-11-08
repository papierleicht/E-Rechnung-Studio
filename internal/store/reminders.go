package store

import (
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/shopspring/decimal"
	"papierleicht.de/e-rechnung-studio/internal/model"
)

// ReminderLevel configures one escalation step. WaitDays counts from the due
// date (first step) or from the deadline named in the previous reminder.
type ReminderLevel struct {
	Title       string `json:"title"`
	Subject     string `json:"subject"`
	Text        string `json:"text"`
	WaitDays    int    `json:"waitDays"`
	PaymentDays int    `json:"paymentDays"`
	Fee         string `json:"fee"`
}

// ReminderSettings always holds exactly three levels: a friendly payment
// reminder and two dunning letters. Fee and interest are informational amounts.
type ReminderSettings struct {
	Levels       []ReminderLevel `json:"levels"`
	InterestRate string          `json:"interestRate"`
}

// Reminder is an immutable history entry. The database rejects updates and
// deletes; only a full backup restore replaces the history as a whole.
type Reminder struct {
	ID             string `json:"id"`
	InvoiceID      string `json:"invoiceId"`
	Number         string `json:"number"`
	Level          int    `json:"level"`
	Title          string `json:"title"`
	Date           string `json:"date"`
	Currency       string `json:"currency"`
	Amount         string `json:"amount"`
	Fee            string `json:"fee"`
	Interest       string `json:"interest"`
	Total          string `json:"total"`
	DueSince       string `json:"dueSince"`
	PaymentDue     string `json:"paymentDue"`
	Installment    bool   `json:"installment"`
	Channel        string `json:"channel"`
	Subject        string `json:"subject"`
	Text           string `json:"text"`
	FileName       string `json:"fileName"`
	DocumentSHA256 string `json:"documentSha256"`
	CreatedAt      string `json:"createdAt"`
}

// ReminderBasis is the actual open position of an issued customer invoice on
// a given day: the remaining balance, or with an installment plan only the
// rates that are already due.
type ReminderBasis struct {
	Balance     PaymentBalance `json:"balance"`
	IssueDate   string         `json:"issueDate"`
	Eligible    bool           `json:"eligible"`
	Reason      string         `json:"reason"`
	Amount      string         `json:"amount"`
	DueSince    string         `json:"dueSince"`
	Installment bool           `json:"installment"`
}

// ReminderProposal suggests the next level. Due marks proposals whose waiting
// period has passed; only those are listed as tasks.
type ReminderProposal struct {
	Today       string    `json:"today"`
	InvoiceID   string    `json:"invoiceId"`
	Number      string    `json:"number"`
	Partner     string    `json:"partner"`
	Currency    string    `json:"currency"`
	Eligible    bool      `json:"eligible"`
	Reason      string    `json:"reason"`
	Due         bool      `json:"due"`
	Complete    bool      `json:"complete"`
	Level       int       `json:"level"`
	Title       string    `json:"title"`
	Amount      string    `json:"amount"`
	Remaining   string    `json:"remaining"`
	Fee         string    `json:"fee"`
	Interest    string    `json:"interest"`
	Total       string    `json:"total"`
	DueSince    string    `json:"dueSince"`
	DaysOverdue int       `json:"daysOverdue"`
	NextDate    string    `json:"nextDate"`
	PaymentDue  string    `json:"paymentDue"`
	Installment bool      `json:"installment"`
	Last        *Reminder `json:"last"`
}

const (
	reminderSettingsKey = "reminder-settings-v1"
	ReminderLevels      = 3
	// ReminderChannelManual is the only channel the app records today. A
	// future mail integration records its own channel via RecordReminder.
	ReminderChannelManual = "manual-export"
	ReminderChannelEmail  = "email"
)

func DefaultReminderSettings() ReminderSettings {
	return ReminderSettings{Levels: []ReminderLevel{
		{
			Title: "Zahlungserinnerung", WaitDays: 7, PaymentDays: 7, Fee: "0.00",
			Subject: "Zahlungserinnerung zur Rechnung {rechnungsnummer}",
			Text:    "Guten Tag,\n\nsicher ist es im Alltag untergegangen: Für unsere Rechnung {rechnungsnummer} vom {rechnungsdatum} konnten wir bisher keinen vollständigen Zahlungseingang feststellen. Offen sind noch {betrag}, fällig seit {faelligkeit}.\n\nWir bitten Sie, den Betrag bis zum {frist} zu überweisen. Sollte sich Ihre Zahlung mit diesem Schreiben überschnitten haben, betrachten Sie es bitte als gegenstandslos.\n\nMit freundlichen Grüßen\n{firma}",
		},
		{
			Title: "1. Mahnung", WaitDays: 3, PaymentDays: 7, Fee: "0.00",
			Subject: "1. Mahnung zur Rechnung {rechnungsnummer}",
			Text:    "Guten Tag,\n\nauf unsere Zahlungserinnerung zur Rechnung {rechnungsnummer} vom {rechnungsdatum} haben wir bisher keinen vollständigen Zahlungseingang erhalten. Offen sind weiterhin {betrag}, fällig seit {faelligkeit}.\n\nBitte überweisen Sie den Betrag bis spätestens {frist}. Falls es Fragen zur Rechnung gibt, melden Sie sich gern bei uns.\n\nMit freundlichen Grüßen\n{firma}",
		},
		{
			Title: "2. Mahnung", WaitDays: 3, PaymentDays: 7, Fee: "0.00",
			Subject: "2. Mahnung zur Rechnung {rechnungsnummer}",
			Text:    "Guten Tag,\n\ntrotz unserer bisherigen Schreiben ist die Rechnung {rechnungsnummer} vom {rechnungsdatum} noch nicht vollständig beglichen. Offen sind {betrag}, fällig seit {faelligkeit}.\n\nWir bitten Sie dringend, den Betrag bis spätestens {frist} zu überweisen. Wenn Sie Fragen haben oder eine Ratenzahlung vereinbaren möchten, sprechen Sie uns bitte an.\n\nMit freundlichen Grüßen\n{firma}",
		},
	}}
}

// NormalizeReminderSettings trims input and rejects values the letters cannot use.
func NormalizeReminderSettings(v ReminderSettings) (ReminderSettings, error) {
	if len(v.Levels) != ReminderLevels {
		return v, errors.New("Gib genau drei Erinnerungsstufen an.")
	}
	out := ReminderSettings{Levels: make([]ReminderLevel, ReminderLevels)}
	for i, l := range v.Levels {
		name := DefaultReminderSettings().Levels[i].Title
		l.Title, l.Subject, l.Text = strings.TrimSpace(l.Title), strings.TrimSpace(l.Subject), strings.TrimSpace(l.Text)
		if l.Title == "" || utf8.RuneCountInString(l.Title) > 80 {
			return v, fmt.Errorf("%s: Gib eine Bezeichnung mit höchstens 80 Zeichen ein.", name)
		}
		if l.Subject == "" || utf8.RuneCountInString(l.Subject) > 200 {
			return v, fmt.Errorf("%s: Gib einen Betreff mit höchstens 200 Zeichen ein.", name)
		}
		if l.Text == "" || utf8.RuneCountInString(l.Text) > 5000 {
			return v, fmt.Errorf("%s: Gib einen Text mit höchstens 5000 Zeichen ein.", name)
		}
		if l.WaitDays < 0 || l.WaitDays > 365 {
			return v, fmt.Errorf("%s: Die Wartezeit muss zwischen 0 und 365 Tagen liegen.", name)
		}
		if l.PaymentDays < 1 || l.PaymentDays > 90 {
			return v, fmt.Errorf("%s: Die Zahlungsfrist muss zwischen 1 und 90 Tagen liegen.", name)
		}
		fee, err := hintAmount(l.Fee, "10000")
		if err != nil {
			return v, fmt.Errorf("%s: Gib eine gültige Mahngebühr zwischen 0 und 10.000 mit höchstens zwei Nachkommastellen ein.", name)
		}
		l.Fee = fee.StringFixed(2)
		out.Levels[i] = l
	}
	if rate := strings.TrimSpace(v.InterestRate); rate != "" {
		r, err := hintAmount(rate, "30")
		if err != nil || r.IsZero() {
			return v, errors.New("Gib einen Zinssatz zwischen 0 und 30 % p. a. mit höchstens zwei Nachkommastellen ein oder lass das Feld leer.")
		}
		out.InterestRate = r.String()
	}
	return out, nil
}

func hintAmount(value, limit string) (decimal.Decimal, error) {
	value = strings.ReplaceAll(strings.TrimSpace(value), ",", ".")
	if value == "" {
		return decimal.Zero, nil
	}
	if len(value) > 32 || strings.ContainsAny(value, "eE") {
		return decimal.Zero, errors.New("invalid")
	}
	d, err := decimal.NewFromString(value)
	if err != nil || d.IsNegative() || !d.Equal(d.Round(2)) || d.GreaterThan(decimal.RequireFromString(limit)) {
		return decimal.Zero, errors.New("invalid")
	}
	return d, nil
}

func (s *Store) ReminderSettings() (ReminderSettings, error) {
	v := DefaultReminderSettings()
	if err := s.readSetting(reminderSettingsKey, &v); err != nil {
		return v, err
	}
	if len(v.Levels) != ReminderLevels {
		v.Levels = DefaultReminderSettings().Levels
	}
	return v, nil
}

func (s *Store) SaveReminderSettings(v ReminderSettings) (ReminderSettings, error) {
	v, err := NormalizeReminderSettings(v)
	if err != nil {
		return v, err
	}
	return v, s.writeSetting(reminderSettingsKey, v)
}

const reminderTriggers = `
CREATE TRIGGER IF NOT EXISTS invoice_reminders_immutable_update BEFORE UPDATE ON invoice_reminders BEGIN SELECT RAISE(ABORT, 'Die Mahnhistorie ist unveränderlich.'); END;
CREATE TRIGGER IF NOT EXISTS invoice_reminders_immutable_delete BEFORE DELETE ON invoice_reminders BEGIN SELECT RAISE(ABORT, 'Die Mahnhistorie ist unveränderlich.'); END;`

// migrateReminders adds the history table to new and existing databases.
func migrateReminders(db interface {
	Exec(string, ...any) (sql.Result, error)
}) error {
	_, err := db.Exec(`
CREATE TABLE IF NOT EXISTS invoice_reminders (id TEXT PRIMARY KEY, invoice_id TEXT NOT NULL, level INTEGER NOT NULL, created_at TEXT NOT NULL, data BLOB NOT NULL, pdf BLOB NOT NULL);
CREATE INDEX IF NOT EXISTS invoice_reminders_invoice ON invoice_reminders(invoice_id, created_at);` + reminderTriggers)
	return err
}

type backupReminder struct {
	ID        string          `json:"id"`
	InvoiceID string          `json:"invoiceId"`
	Level     int             `json:"level"`
	CreatedAt string          `json:"createdAt"`
	Data      json.RawMessage `json:"data"`
	PDF       string          `json:"pdf,omitempty"`
	// PDFBlob is the SHA-256 of the PDF when a snapshot keeps it separately.
	PDFBlob string `json:"pdfBlob,omitempty"`
}

func backupReminders(q sqlRunner, c *blobCollector) ([]backupReminder, error) {
	rows, err := q.Query(`SELECT id,invoice_id,level,created_at,data,length(pdf) FROM invoice_reminders ORDER BY created_at,id`)
	if err != nil {
		return nil, err
	}
	out := []backupReminder{}
	var sizes []int64
	for rows.Next() {
		var v backupReminder
		var data []byte
		var size int64
		if err = rows.Scan(&v.ID, &v.InvoiceID, &v.Level, &v.CreatedAt, &data, &size); err != nil {
			rows.Close()
			return nil, err
		}
		v.Data = json.RawMessage(data)
		out, sizes = append(out, v), append(sizes, size)
	}
	if err = rows.Close(); err != nil {
		return nil, err
	}
	for i := range out {
		v := &out[i]
		if c != nil {
			if v.PDFBlob, err = c.reference(sourceReminder+v.ID, sizes[i]); err != nil {
				return nil, err
			}
			continue
		}
		pdf, err := readBlob(q, sourceReminder+v.ID)
		if err != nil {
			return nil, err
		}
		v.PDF = base64.StdEncoding.EncodeToString(pdf)
	}
	return out, nil
}

// replaceReminders is the only path that removes history: a full restore
// replaces it with the backup's history inside the restore transaction.
func replaceReminders(tx *sql.Tx, values []backupReminder, open BlobOpener) error {
	if _, err := tx.Exec(`DROP TRIGGER IF EXISTS invoice_reminders_immutable_update; DROP TRIGGER IF EXISTS invoice_reminders_immutable_delete; DELETE FROM invoice_reminders;`); err != nil {
		return err
	}
	for _, v := range values {
		var pdf []byte
		var err error
		if v.PDFBlob != "" {
			pdf, err = openBlob(open, v.PDFBlob)
		} else {
			pdf, err = base64.StdEncoding.DecodeString(v.PDF)
		}
		if err != nil {
			return fmt.Errorf("Die Zahlungserinnerung %s ist ungültig: %w", v.ID, err)
		}
		var r Reminder
		if err = json.Unmarshal(v.Data, &r); err != nil {
			return fmt.Errorf("Die Zahlungserinnerung %s ist ungültig: %w", v.ID, err)
		}
		if _, err = tx.Exec(`INSERT INTO invoice_reminders(id,invoice_id,level,created_at,data,pdf) VALUES(?,?,?,?,?,?)`, v.ID, v.InvoiceID, v.Level, v.CreatedAt, []byte(v.Data), pdf); err != nil {
			return err
		}
	}
	_, err := tx.Exec(reminderTriggers)
	return err
}

// Reminders lists the history of one invoice, oldest first. An empty id lists all.
func (s *Store) Reminders(invoiceID string) ([]Reminder, error) {
	list, err := readReminders(s.db, invoiceID)
	if err != nil || s.scope.Load() == nil {
		return list, err
	}
	kept := make([]Reminder, 0, len(list))
	for _, r := range list {
		if s.scope.Load().covers(r.Date) {
			kept = append(kept, r)
		}
	}
	return kept, nil
}

func readReminders(q interface {
	Query(string, ...any) (*sql.Rows, error)
}, invoiceID string) ([]Reminder, error) {
	rows, err := q.Query(`SELECT data FROM invoice_reminders WHERE ?='' OR invoice_id=? ORDER BY created_at,id`, invoiceID, invoiceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Reminder{}
	for rows.Next() {
		var raw []byte
		var r Reminder
		if err = rows.Scan(&raw); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(raw, &r); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ReminderPDF returns the document exactly as it was recorded.
func (s *Store) ReminderPDF(id string) (Reminder, []byte, error) {
	var raw, pdf []byte
	var r Reminder
	err := s.db.QueryRow(`SELECT data,pdf FROM invoice_reminders WHERE id=?`, id).Scan(&raw, &pdf)
	if errors.Is(err, sql.ErrNoRows) {
		return r, nil, errors.New("Die Zahlungserinnerung wurde nicht gefunden.")
	}
	if err != nil {
		return r, nil, err
	}
	err = json.Unmarshal(raw, &r)
	return r, pdf, err
}

type rowQuerier interface {
	Query(string, ...any) (*sql.Rows, error)
	QueryRow(string, ...any) *sql.Row
}

func reminderBasis(q rowQuerier, id, today string) (ReminderBasis, error) {
	var kind, issueDate string
	if err := q.QueryRow(`SELECT COALESCE(json_extract(data,'$.kind'),''),issue_date FROM invoices WHERE id=?`, id).Scan(&kind, &issueDate); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ReminderBasis{}, errors.New("Die Kundenrechnung wurde nicht gefunden.")
		}
		return ReminderBasis{}, err
	}
	b, err := paymentTarget(q, "outgoing", id)
	if err != nil {
		return ReminderBasis{}, err
	}
	payments, err := readPayments(q)
	if err != nil {
		return ReminderBasis{}, err
	}
	plan, err := readPlan(q, id)
	if err != nil {
		return ReminderBasis{}, err
	}
	if b, err = settledBalance(q, b, payments); err != nil {
		return ReminderBasis{}, err
	}
	return evaluateReminderBasis(b, plan, kind, issueDate, today), nil
}

// evaluateReminderBasis decides whether an invoice may be reminded at all.
func evaluateReminderBasis(b PaymentBalance, plan InstallmentPlan, kind, issueDate, today string) ReminderBasis {
	r := ReminderBasis{Balance: b, IssueDate: issueDate}
	scale, _ := model.CurrencyScale(b.Currency)
	remaining, err := decimal.NewFromString(b.Remaining)
	switch {
	case kind == "credit-note":
		r.Reason = "Für Gutschriften werden keine Zahlungserinnerungen erstellt."
	case err == nil && !remaining.IsPositive() && b.State == "verrechnet":
		r.Reason = "Die Rechnung ist durch eine Gutschrift ausgeglichen."
	case err != nil || !remaining.IsPositive():
		r.Reason = "Die Rechnung ist vollständig bezahlt."
	case !b.Collectable().IsPositive():
		r.Reason = "Der offene Betrag wurde erstattet und wird nicht mehr geschuldet. Mit der Korrekturgutschrift ist die Rechnung ausgeglichen."
	case plan.Active:
		b = applyPlanAt(b, plan, today)
		r.Balance, r.Installment = b, true
		due, _ := decimal.NewFromString(b.OverdueAmount)
		if b.PlanNeedsReview {
			r.Reason = "Prüfe zuerst die Ratenvereinbarung. Eine frühere Zahlung wurde korrigiert."
			break
		}
		if !due.IsPositive() {
			r.Reason = "Die vereinbarten Raten werden bisher plangemäß gezahlt."
			break
		}
		// The oldest rate that is still (partly) open after crediting payments.
		paid, _ := decimal.NewFromString(b.Paid)
		baseline, _ := decimal.NewFromString(plan.BaselinePaid)
		covered := decimal.Max(paid.Sub(baseline), decimal.Zero)
		for _, rate := range plan.Rates {
			amount, _ := decimal.NewFromString(rate.Amount)
			left := amount.Sub(covered)
			covered = decimal.Max(covered.Sub(amount), decimal.Zero)
			if left.IsPositive() {
				r.DueSince = rate.Date
				break
			}
		}
		r.Eligible, r.Amount = true, due.StringFixed(scale)
	case b.DueDate == "":
		r.Reason = "Für diese Rechnung ist keine Fälligkeit hinterlegt."
	case b.DueDate >= today:
		r.Reason = "Die Rechnung ist noch nicht fällig."
	default:
		r.Balance.Remaining = b.Collectable().StringFixed(scale)
		r.Eligible, r.Amount, r.DueSince = true, r.Balance.Remaining, b.DueDate
	}
	return r
}

// ProposeReminder suggests the next level for the current overdue position.
// A reminder cycle belongs to one due date: once an installment plan moves the
// oldest open rate forward, the next cycle starts again with a friendly reminder.
func ProposeReminder(basis ReminderBasis, history []Reminder, cfg ReminderSettings, today string) ReminderProposal {
	b := basis.Balance
	p := ReminderProposal{Today: today, InvoiceID: b.ID, Number: b.Number, Partner: b.Partner, Currency: b.Currency, Remaining: b.Remaining,
		Eligible: basis.Eligible, Reason: basis.Reason, Installment: basis.Installment, DueSince: basis.DueSince, Level: 1}
	if len(cfg.Levels) != ReminderLevels {
		cfg = DefaultReminderSettings()
	}
	cycle := []Reminder{}
	for _, r := range history {
		if r.InvoiceID == b.ID && r.DueSince == basis.DueSince {
			cycle = append(cycle, r)
		}
	}
	if len(history) > 0 {
		last := history[len(history)-1]
		p.Last = &last
	}
	if !basis.Eligible {
		return p
	}
	start := basis.DueSince
	if len(cycle) > 0 {
		start = cycle[len(cycle)-1].PaymentDue
		p.Level = min(len(cycle)+1, ReminderLevels)
		p.Complete = len(cycle) >= ReminderLevels
	}
	level := cfg.Levels[p.Level-1]
	p.Title = level.Title
	scale, _ := model.CurrencyScale(b.Currency)
	amount, _ := decimal.NewFromString(basis.Amount)
	fee, _ := hintAmount(level.Fee, "10000")
	interest := decimal.Zero
	todayTime, _ := time.Parse(time.DateOnly, today)
	if since, err := time.Parse(time.DateOnly, basis.DueSince); err == nil {
		p.DaysOverdue = max(int(todayTime.Sub(since).Hours()/24), 0)
		if rate, e := hintAmount(cfg.InterestRate, "30"); e == nil && rate.IsPositive() {
			interest = amount.Mul(rate).Div(decimal.NewFromInt(100)).Mul(decimal.NewFromInt(int64(p.DaysOverdue))).Div(decimal.NewFromInt(365)).Round(scale)
		}
	}
	p.Amount = amount.StringFixed(scale)
	p.Fee = fee.StringFixed(scale)
	p.Interest = interest.StringFixed(scale)
	p.Total = amount.Add(fee).Add(interest).StringFixed(scale)
	p.PaymentDue = todayTime.AddDate(0, 0, level.PaymentDays).Format(time.DateOnly)
	if begin, err := time.Parse(time.DateOnly, start); err == nil {
		p.NextDate = begin.AddDate(0, 0, level.WaitDays).Format(time.DateOnly)
	}
	p.Due = !p.Complete && p.NextDate != "" && start < today && p.NextDate <= today
	return p
}

// InvoiceReminderProposal evaluates one invoice on the given day.
func (s *Store) InvoiceReminderProposal(id, today string) (ReminderProposal, error) {
	cfg, err := s.ReminderSettings()
	if err != nil {
		return ReminderProposal{}, err
	}
	basis, err := reminderBasis(s.db, id, today)
	if err != nil {
		return ReminderProposal{}, err
	}
	history, err := s.Reminders(id)
	if err != nil {
		return ReminderProposal{}, err
	}
	return ProposeReminder(basis, history, cfg, today), nil
}

// ReminderProposals evaluates all issued customer invoices that still have an open balance.
func (s *Store) ReminderProposals(today string) ([]ReminderProposal, error) {
	cfg, err := s.ReminderSettings()
	if err != nil {
		return nil, err
	}
	rows, err := s.db.Query(`SELECT id FROM invoices WHERE status='issued' ORDER BY issue_date,id`)
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
	history, err := s.Reminders("")
	if err != nil {
		return nil, err
	}
	byInvoice := map[string][]Reminder{}
	for _, r := range history {
		byInvoice[r.InvoiceID] = append(byInvoice[r.InvoiceID], r)
	}
	out := []ReminderProposal{}
	for _, id := range ids {
		basis, e := reminderBasis(s.db, id, today)
		if e != nil {
			return nil, e
		}
		if _, e = decimal.NewFromString(basis.Balance.Gross); e != nil {
			continue
		}
		out = append(out, ProposeReminder(basis, byInvoice[id], cfg, today))
	}
	return out, nil
}

// RecordReminder logs a created reminder together with its document. The
// open position is re-evaluated in the same transaction, so a payment that
// arrived meanwhile is never overtaken by a stale letter.
func (s *Store) RecordReminder(r Reminder, pdf []byte, today string) (Reminder, error) {
	if r.Channel != ReminderChannelManual && r.Channel != ReminderChannelEmail {
		return r, errors.New("Dieser Versandweg ist für die Zahlungserinnerung unbekannt.")
	}
	if r.Level < 1 || r.Level > ReminderLevels {
		return r, errors.New("Diese Erinnerungsstufe gibt es nicht.")
	}
	if len(pdf) == 0 {
		return r, errors.New("Das Erinnerungsdokument ist leer.")
	}
	tx, err := s.db.Begin()
	if err != nil {
		return r, err
	}
	defer tx.Rollback()
	basis, err := reminderBasis(tx, r.InvoiceID, today)
	if err != nil {
		return r, err
	}
	if !basis.Eligible {
		return r, errors.New(basis.Reason)
	}
	if basis.Amount != r.Amount || basis.DueSince != r.DueSince || basis.Balance.Currency != r.Currency {
		return r, errors.New("Der offene Betrag hat sich inzwischen geändert. Erstelle die Erinnerung noch einmal neu.")
	}
	sum := sha256.Sum256(pdf)
	r.ID = newID("rem")
	r.CreatedAt = now()
	r.Date = today
	r.Number = basis.Balance.Number
	r.Installment = basis.Installment
	r.DocumentSHA256 = hex.EncodeToString(sum[:])
	raw, err := json.Marshal(r)
	if err != nil {
		return r, err
	}
	if _, err = tx.Exec(`INSERT INTO invoice_reminders(id,invoice_id,level,created_at,data,pdf) VALUES(?,?,?,?,?,?)`, r.ID, r.InvoiceID, r.Level, r.CreatedAt, raw, pdf); err != nil {
		return r, err
	}
	if err = s.logChange(tx, AuditEntry{Action: AuditCreated, Object: AuditReminder, ObjectID: r.ID, Label: fmt.Sprintf("Zahlungserinnerung Stufe %d zu %s", r.Level, r.Number), After: string(raw), Sum: reminderSum(raw, pdf)}); err != nil {
		return r, err
	}
	return r, tx.Commit()
}
