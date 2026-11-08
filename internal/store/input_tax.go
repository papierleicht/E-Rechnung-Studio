package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"strings"

	"github.com/shopspring/decimal"
	"papierleicht.de/e-rechnung-studio/internal/model"
)

const inputTaxPrefix = "received-input-tax:"

const inputTaxLabel = "Vorsteuer-Entscheidung zu einem Eingangsbeleg"

// normalizeInputTaxChoice checks a decision and brings it into its stored form.
func normalizeInputTaxChoice(c model.InputTaxChoice) (model.InputTaxChoice, error) {
	c.Note = strings.TrimSpace(c.Note)
	if len([]rune(c.Note)) > 500 {
		return c, errors.New("Die Begründung darf höchstens 500 Zeichen lang sein.")
	}
	switch c.Mode {
	case model.InputTaxFull, model.InputTaxNone, model.InputTaxOnPayment:
		c.Amount = ""
	case model.InputTaxAmount:
		amount, err := decimal.NewFromString(strings.ReplaceAll(strings.TrimSpace(c.Amount), ",", "."))
		if err != nil || amount.IsNegative() {
			return c, errors.New("Gib die Vorsteuer als Betrag ab 0 Euro an, zum Beispiel 94,20.")
		}
		c.Amount = amount.StringFixed(2)
	default:
		return c, errors.New("Wähle aus, wie die Vorsteuer in deine Zahlen eingehen soll.")
	}
	return c, nil
}

// SaveInputTaxChoice stores the user's decision on a received document. The
// decision goes into the change log and a closed period keeps the one it has.
func (s *Store) SaveInputTaxChoice(c model.InputTaxChoice) (model.InputTaxChoice, error) {
	c, err := normalizeInputTaxChoice(c)
	if err != nil {
		return c, err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return c, err
	}
	defer tx.Rollback()
	var exists int
	if err = tx.QueryRow(`SELECT 1 FROM received_invoices WHERE id=?`, c.ReceivedID).Scan(&exists); err != nil {
		return c, err
	}
	if err = refuseLockedReceived(tx, c.ReceivedID); err != nil {
		return c, err
	}
	key := inputTaxPrefix + c.ReceivedID
	var before []byte
	err = tx.QueryRow("SELECT value FROM settings WHERE key=?", key).Scan(&before)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return c, err
	}
	if before != nil {
		var previous model.InputTaxChoice
		if err = json.Unmarshal(before, &previous); err != nil {
			return c, err
		}
		if previous.Mode == c.Mode && previous.Amount == c.Amount && previous.Note == c.Note {
			return previous, nil
		}
	}
	c.DecidedAt = now()
	raw, err := json.Marshal(c)
	if err != nil {
		return c, err
	}
	if _, err = tx.Exec("INSERT INTO settings(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value", key, raw); err != nil {
		return c, err
	}
	action := AuditCreated
	if before != nil {
		action = AuditChanged
	}
	if err = s.logChange(tx, AuditEntry{Action: action, Object: AuditInputTax, ObjectID: c.ReceivedID, Label: inputTaxLabel, Before: string(before), After: string(raw), Sum: jsonSum(raw)}); err != nil {
		return c, err
	}
	return c, tx.Commit()
}

// ClearInputTaxChoice drops the user's decision, so the app's suggestion applies again.
func (s *Store) ClearInputTaxChoice(id string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = refuseLockedReceived(tx, id); err != nil {
		return err
	}
	key := inputTaxPrefix + id
	var before []byte
	err = tx.QueryRow("SELECT value FROM settings WHERE key=?", key).Scan(&before)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if _, err = tx.Exec("DELETE FROM settings WHERE key=?", key); err != nil {
		return err
	}
	if err = s.logChange(tx, AuditEntry{Action: AuditDeleted, Object: AuditInputTax, ObjectID: id, Label: inputTaxLabel, Before: string(before), Sum: auditGone}); err != nil {
		return err
	}
	return tx.Commit()
}

// InputTaxChoices returns the user's decisions by received document.
func (s *Store) InputTaxChoices() (map[string]model.InputTaxChoice, error) {
	rows, err := s.db.Query("SELECT value FROM settings WHERE key LIKE ?", inputTaxPrefix+"%")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]model.InputTaxChoice{}
	for rows.Next() {
		var raw []byte
		if err = rows.Scan(&raw); err != nil {
			return nil, err
		}
		var c model.InputTaxChoice
		if err = json.Unmarshal(raw, &c); err != nil {
			return nil, err
		}
		out[c.ReceivedID] = c
	}
	return out, rows.Err()
}
