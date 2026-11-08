package mail

import (
	"encoding/json"
	"errors"
)

type backupData struct {
	Version  int        `json:"version"`
	Settings *Settings  `json:"settings,omitempty"`
	Log      []LogEntry `json:"log"`
}

// ExportBackup returns account, templates and send log. The SMTP password stays
// in the operating-system keyring and is deliberately never exported.
func (s *Service) ExportBackup() (json.RawMessage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	doc := backupData{Version: 1}
	var count int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM mail_settings").Scan(&count); err != nil {
		return nil, err
	}
	if count > 0 {
		settings, err := s.settings()
		if err != nil {
			return nil, err
		}
		settings.HasPassword = false
		doc.Settings = &settings
	}
	var err error
	if doc.Log, err = s.query("SELECT data FROM mail_log ORDER BY sent_at,id"); err != nil {
		return nil, err
	}
	return json.Marshal(doc)
}

// RestoreBackup replaces the send log. Account settings are replaced when the
// backup contains them; otherwise (older backups) the local account is kept.
// A nil value restores an empty log.
func (s *Service) RestoreBackup(raw json.RawMessage) error {
	doc := backupData{Version: 1}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &doc); err != nil || doc.Version != 1 {
			return errors.New("Die Sicherung der E-Mail-Daten ist ungültig.")
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.Exec("DELETE FROM mail_log"); err != nil {
		return err
	}
	if doc.Settings != nil {
		doc.Settings.HasPassword = false
		data, marshalErr := json.Marshal(doc.Settings)
		if marshalErr != nil {
			return marshalErr
		}
		if _, err = tx.Exec("INSERT INTO mail_settings(id,data) VALUES(1,?) ON CONFLICT(id) DO UPDATE SET data=excluded.data", data); err != nil {
			return err
		}
	}
	for _, entry := range doc.Log {
		if entry.ID == "" || entry.DocumentType == "" || entry.DocumentID == "" || (entry.Status != StatusSent && entry.Status != StatusFailed) {
			return errors.New("Ein Eintrag im E-Mail-Versandprotokoll der Sicherung ist ungültig.")
		}
		data, marshalErr := json.Marshal(entry)
		if marshalErr != nil {
			return marshalErr
		}
		if _, err = tx.Exec("INSERT INTO mail_log(id,document_type,document_id,sent_at,status,data) VALUES(?,?,?,?,?,?)",
			entry.ID, entry.DocumentType, entry.DocumentID, entry.SentAt, entry.Status, data); err != nil {
			return err
		}
	}
	return tx.Commit()
}
