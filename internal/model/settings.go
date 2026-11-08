package model

import "encoding/json"

// SettingsSnapshot is returned only after the settings page has been committed.
type SettingsSnapshot struct {
	Settings AppSettings `json:"settings"`
	Company  Company     `json:"company"`
}

// UnmarshalJSON enables automatic checks for legacy settings and backups that
// predate the preference, while preserving an explicitly saved false value.
func (s *AppSettings) UnmarshalJSON(data []byte) error {
	type plain AppSettings
	value := plain{AutomaticUpdates: true}
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	*s = AppSettings(value)
	return nil
}
