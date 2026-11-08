package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"papierleicht.de/e-rechnung-studio/internal/model"
)

// Recurring invoices are plans, not documents: they can change and be deleted
// freely, and the drafts they created stay.
func (s *Store) migrateRecurring() error {
	_, err := s.db.Exec(`
CREATE TABLE IF NOT EXISTS recurring_invoices (id TEXT PRIMARY KEY, next_date TEXT NOT NULL, data BLOB NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL);`)
	return err
}

const recurringColumns = `id,next_date,data,created_at,updated_at`

// ErrRecurringChanged means the plan moved on since it was read, for example
// through a second run. Its draft for that date exists or is no longer wanted.
var ErrRecurringChanged = errors.New("Die Dauerrechnung wurde inzwischen geändert.")

func (s *Store) RecurringInvoices() ([]model.RecurringInvoice, error) {
	open, err := s.recurringOpenDrafts()
	if err != nil {
		return nil, err
	}
	rows, err := s.db.Query(`SELECT data FROM recurring_invoices ORDER BY next_date,id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.RecurringInvoice{}
	for rows.Next() {
		var raw []byte
		if err = rows.Scan(&raw); err != nil {
			return nil, err
		}
		var r model.RecurringInvoice
		if err = json.Unmarshal(raw, &r); err != nil {
			return nil, err
		}
		r.OpenDrafts = open[r.ID]
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) recurringOpenDrafts() (map[string]int, error) {
	rows, err := s.db.Query(`SELECT json_extract(data,'$.recurringId'),COUNT(*) FROM invoices WHERE status='draft' AND json_extract(data,'$.recurringId') IS NOT NULL GROUP BY 1`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	open := map[string]int{}
	for rows.Next() {
		var id string
		var n int
		if err = rows.Scan(&id, &n); err != nil {
			return nil, err
		}
		open[id] = n
	}
	return open, rows.Err()
}

func (s *Store) RecurringInvoice(id string) (model.RecurringInvoice, error) {
	var raw []byte
	var r model.RecurringInvoice
	err := s.db.QueryRow(`SELECT data FROM recurring_invoices WHERE id=?`, id).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return r, errors.New("Die Dauerrechnung wurde nicht gefunden.")
	}
	if err != nil {
		return r, err
	}
	return r, json.Unmarshal(raw, &r)
}

func writeRecurring(q sqlRunner, r model.RecurringInvoice) error {
	raw, err := encode(r)
	if err != nil {
		return err
	}
	_, err = q.Exec(`INSERT INTO recurring_invoices(`+recurringColumns+`) VALUES(?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET next_date=excluded.next_date,data=excluded.data,updated_at=excluded.updated_at`, r.ID, r.NextDate, raw, r.CreatedAt, r.UpdatedAt)
	return err
}

func (s *Store) SaveRecurringInvoice(r model.RecurringInvoice) (model.RecurringInvoice, error) {
	t := now()
	if r.ID == "" {
		r.ID = newID("rec")
		r.CreatedAt = t
	}
	r.UpdatedAt = t
	return r, writeRecurring(s.db, r)
}

func (s *Store) DeleteRecurringInvoice(id string) error {
	result, err := s.db.Exec(`DELETE FROM recurring_invoices WHERE id=?`, id)
	if err != nil {
		return err
	}
	if n, _ := result.RowsAffected(); n != 1 {
		return errors.New("Die Dauerrechnung wurde nicht gefunden.")
	}
	return nil
}

// AddRecurringDraft stores the draft of a due date and moves the plan on in one
// step, so a crash or a second run can never create the same period twice. It
// leaves the customer's payment term alone: nobody decided anything here.
func (s *Store) AddRecurringDraft(next model.RecurringInvoice, dueDate string, v model.InvoiceDraft, gross string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	raw, err := encode(next)
	if err != nil {
		return err
	}
	t := now()
	result, err := tx.Exec(`UPDATE recurring_invoices SET next_date=?,data=?,updated_at=? WHERE id=? AND next_date=?`, next.NextDate, raw, t, next.ID, dueDate)
	if err != nil {
		return err
	}
	if n, _ := result.RowsAffected(); n != 1 {
		return ErrRecurringChanged
	}
	v.ID, v.Status, v.CreatedAt, v.UpdatedAt = newID("inv"), "draft", t, t
	if raw, err = encode(v); err != nil {
		return err
	}
	if _, err = tx.Exec(`INSERT INTO invoices(id,number,customer_name,status,issue_date,gross,data,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?)`, v.ID, v.Number, v.Customer.Name, v.Status, v.IssueDate, gross, raw, v.CreatedAt, v.UpdatedAt); err != nil {
		return err
	}
	return tx.Commit()
}

// Customer returns a stored customer, or nil if it was deleted.
func (s *Store) Customer(id string) (*model.Customer, error) {
	var raw []byte
	err := s.db.QueryRow(`SELECT data FROM customers WHERE id=?`, id).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var c model.Customer
	if err = json.Unmarshal(raw, &c); err != nil {
		return nil, err
	}
	return &c, nil
}

type backupRecurring struct {
	ID        string          `json:"id"`
	NextDate  string          `json:"nextDate"`
	Data      json.RawMessage `json:"data"`
	CreatedAt string          `json:"createdAt"`
	UpdatedAt string          `json:"updatedAt"`
}

func backupRecurringRows(q sqlRunner) ([]backupRecurring, error) {
	rows, err := q.Query(`SELECT ` + recurringColumns + ` FROM recurring_invoices ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []backupRecurring{}
	for rows.Next() {
		var v backupRecurring
		var raw []byte
		if err = rows.Scan(&v.ID, &v.NextDate, &raw, &v.CreatedAt, &v.UpdatedAt); err != nil {
			return nil, err
		}
		v.Data = raw
		items = append(items, v)
	}
	return items, rows.Err()
}

// Backups before version 8 contain no recurring invoices; restoring them clears
// the table like every other restored table.
func restoreRecurring(tx *sql.Tx, items []backupRecurring) error {
	if _, err := tx.Exec(`DELETE FROM recurring_invoices`); err != nil {
		return err
	}
	for _, v := range items {
		if !json.Valid(v.Data) {
			return fmt.Errorf("Die Dauerrechnung %s in der Datensicherung ist ungültig.", v.ID)
		}
		if _, err := tx.Exec(`INSERT INTO recurring_invoices(`+recurringColumns+`) VALUES(?,?,?,?,?)`, v.ID, v.NextDate, []byte(v.Data), v.CreatedAt, v.UpdatedAt); err != nil {
			return err
		}
	}
	return nil
}
