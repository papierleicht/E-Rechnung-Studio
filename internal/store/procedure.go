package store

import (
	"encoding/json"
	"fmt"
)

// The Verfahrensdokumentation describes how the books are kept (GoBD Rz. 151 ff.).
// The app writes it from what it knows and a few answers of the business. Each
// generated version stays stored as a row, so the backup carries it along and
// the history of the description stays traceable (Rz. 154).

const procedureProfileKey = "procedure-profile"

// ProcedureProfile holds the answers only the business can give.
type ProcedureProfile struct {
	// Responsible is who records and checks the receipts.
	Responsible string `json:"responsible"`
	// Advisor names the tax advisor, if there is one.
	Advisor string `json:"advisor"`
	// PaperReceipts is "none", "keep", "scan-keep" or "scan-destroy".
	PaperReceipts string `json:"paperReceipts"`
	// Cash tells whether the business takes cash.
	Cash bool `json:"cash"`
	// OtherSystems lists further systems with tax-relevant data.
	OtherSystems string `json:"otherSystems"`
	// Devices lists the computers the program runs on.
	Devices string `json:"devices"`
	// BackupRoutine says how local copies are kept in addition to the program.
	BackupRoutine string `json:"backupRoutine"`
}

// ProcedureVersion is one generated description.
type ProcedureVersion struct {
	Version    int    `json:"version"`
	At         string `json:"at"`
	AppVersion string `json:"appVersion"`
	HTML       string `json:"html,omitempty"`
}

func (s *Store) ProcedureProfile() (ProcedureProfile, error) {
	var p ProcedureProfile
	err := s.readSetting(procedureProfileKey, &p)
	return p, err
}

func (s *Store) SaveProcedureProfile(p ProcedureProfile) error {
	return s.writeSetting(procedureProfileKey, p)
}

// AddProcedureVersion stores a new version and notes it in the change log.
func (s *Store) AddProcedureVersion(html string) (ProcedureVersion, error) {
	v := ProcedureVersion{At: now(), AppVersion: s.version, HTML: html}
	tx, err := s.db.Begin()
	if err != nil {
		return v, err
	}
	defer tx.Rollback()
	if err = tx.QueryRow(`SELECT COUNT(*)+1 FROM settings WHERE key LIKE 'procedure-version:%'`).Scan(&v.Version); err != nil {
		return v, err
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return v, err
	}
	id := fmt.Sprintf("%04d", v.Version)
	if _, err = tx.Exec("INSERT INTO settings(key,value) VALUES(?,?)", "procedure-version:"+id, raw); err != nil {
		return v, err
	}
	if err = s.logChange(tx, AuditEntry{Action: AuditCreated, Object: AuditProcedure, ObjectID: id, Label: fmt.Sprintf("Verfahrensdokumentation Version %d", v.Version), Sum: jsonSum(raw)}); err != nil {
		return v, err
	}
	return v, tx.Commit()
}

// ProcedureVersions lists the versions without their text, newest first.
func (s *Store) ProcedureVersions() ([]ProcedureVersion, error) {
	rows, err := s.db.Query(`SELECT key,value FROM settings WHERE key LIKE 'procedure-version:%' ORDER BY key DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ProcedureVersion{}
	for rows.Next() {
		var key string
		var raw []byte
		if err = rows.Scan(&key, &raw); err != nil {
			return nil, err
		}
		var v ProcedureVersion
		if err = json.Unmarshal(raw, &v); err != nil {
			return nil, err
		}
		v.HTML = ""
		out = append(out, v)
	}
	return out, rows.Err()
}

// ProcedureVersionText returns one version with its text. Version 0 is the newest.
func (s *Store) ProcedureVersionText(version int) (ProcedureVersion, error) {
	var v ProcedureVersion
	key := "procedure-version:" + fmt.Sprintf("%04d", version)
	if version == 0 {
		err := s.db.QueryRow(`SELECT key FROM settings WHERE key LIKE 'procedure-version:%' ORDER BY key DESC LIMIT 1`).Scan(&key)
		if err != nil {
			return v, err
		}
	}
	return v, s.readSetting(key, &v)
}

// ProgramVersions lists the versions noted in the log, oldest first.
func (s *Store) ProgramVersions() ([]AuditEntry, error) {
	list, err := s.AuditEntries(AuditFilter{Object: AuditProgram})
	if err != nil {
		return nil, err
	}
	for i, j := 0, len(list)-1; i < j; i, j = i+1, j-1 {
		list[i], list[j] = list[j], list[i]
	}
	return list, nil
}
