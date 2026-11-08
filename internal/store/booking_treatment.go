package store

import (
	"encoding/json"
	"errors"
	"strings"
)

type BookingTreatment struct {
	BookingID string `json:"bookingId"`
	Kind      string `json:"kind"`
	Note      string `json:"note"`
	At        string `json:"at"`
}

// BookingTreatments includes restored entries (empty Kind), so that restoring a
// booking requires manual review instead of triggering automatic allocation.
func (s *Store) BookingTreatments() ([]BookingTreatment, error) {
	out := []BookingTreatment{}
	rows, err := s.db.Query("SELECT value FROM settings WHERE key LIKE 'booking-treatment:%'")
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var raw []byte
		var t BookingTreatment
		if err = rows.Scan(&raw); err != nil {
			return out, err
		}
		if err = json.Unmarshal(raw, &t); err != nil {
			return out, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}
func (s *Store) SaveBookingTreatment(t BookingTreatment) error {
	switch t.Kind {
	case "", "reserve", "transfer", "private", "tax", "vat", "tradetax", "other":
	default:
		return errors.New("Wähle eine gültige Einordnung aus.")
	}
	if len(t.Note) > 2000 || strings.TrimSpace(t.BookingID) == "" {
		return errors.New("Gib den Umsatz und eine gültige Notiz an.")
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
	for _, p := range payments {
		if p.BookingID == t.BookingID && p.RevokedAt == "" {
			return errors.New("Hebe zuerst die Rechnungszuordnung dieses Umsatzes auf.")
		}
	}
	t.At = now()
	raw, err := json.Marshal(t)
	if err != nil {
		return err
	}
	if _, err = tx.Exec("INSERT INTO settings(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value", "booking-treatment:"+t.BookingID, raw); err != nil {
		return err
	}
	if _, err = tx.Exec("INSERT INTO settings(key,value) VALUES(?,?)", "booking-treatment-history:"+newID("evt"), raw); err != nil {
		return err
	}
	if err = s.logChange(tx, AuditEntry{Action: AuditChanged, Object: AuditBooking, ObjectID: t.BookingID, Label: "Einordnung eines Bankumsatzes", Reason: t.Note, After: string(raw), Sum: jsonSum(raw)}); err != nil {
		return err
	}
	return tx.Commit()
}
