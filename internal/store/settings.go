package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"papierleicht.de/e-rechnung-studio/internal/model"
)

func (s *Store) readSetting(key string, target any) error {
	var raw []byte
	err := s.db.QueryRow("SELECT value FROM settings WHERE key=?", key).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, target)
}

// writeTracked stores a setting the books depend on and logs a real change.
func (s *Store) writeTracked(q sqlRunner, key string, raw []byte) error {
	var old []byte
	err := q.QueryRow("SELECT value FROM settings WHERE key=?", key).Scan(&old)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if string(old) == string(raw) {
		return nil
	}
	if _, err = q.Exec("INSERT INTO settings(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value", key, raw); err != nil {
		return err
	}
	action := AuditChanged
	if old == nil {
		action = AuditCreated
	}
	return s.logChange(q, AuditEntry{Action: action, Object: AuditSetting, ObjectID: key, Label: settingLabel(key), Before: string(old), After: string(raw), Sum: jsonSum(raw)})
}

func (s *Store) writeSetting(key string, value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = s.db.Exec("INSERT INTO settings(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value", key, raw)
	return err
}

func (s *Store) AppSettings() (v model.AppSettings, err error) {
	v.AutomaticUpdates = true
	err = s.readSetting("app", &v)
	return
}
func (s *Store) SaveAppSettings(v model.AppSettings) error { return s.writeSetting("app", v) }

// SaveSettings commits the settings page as one unit. An unchanged company is
// omitted so appearance-only saves cannot overwrite or revalidate its data.
func (s *Store) SaveSettings(v model.AppSettings, company *model.Company) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	write := func(key string, value any) error {
		raw, err := json.Marshal(value)
		if err != nil {
			return err
		}
		_, err = tx.Exec("INSERT INTO settings(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value", key, raw)
		return err
	}
	if company != nil {
		raw, err := json.Marshal(company)
		if err != nil {
			return err
		}
		if err = s.writeTracked(tx, "company", raw); err != nil {
			return err
		}
	}
	if err := write("app", v); err != nil {
		return err
	}
	return tx.Commit()
}
func (s *Store) InvoiceExport(id string) (v model.InvoiceExport, err error) {
	err = s.readSetting("export:"+id, &v)
	return
}
func (s *Store) SaveInvoiceExport(id string, v model.InvoiceExport) error {
	return s.writeSetting("export:"+id, v)
}
