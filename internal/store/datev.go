package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"papierleicht.de/e-rechnung-studio/internal/datev"
)

// DatevSettings configure the DATEV export. All accounts are suggestions the
// user agrees with their tax advisor; nothing here is tax advice.
type DatevSettings struct {
	ConsultantNumber     string `json:"consultantNumber"`
	ClientNumber         string `json:"clientNumber"`
	FiscalYearStartMonth int    `json:"fiscalYearStartMonth"`
	AccountLength        int    `json:"accountLength"`
	Chart                string `json:"chart"` // SKR03 or SKR04
	// PersonAccounts is "collective" (one debtor and one creditor account) or
	// "individual" (a consecutive account per customer and supplier).
	PersonAccounts   string         `json:"personAccounts"`
	Accounts         datev.Accounts `json:"accounts"`
	IncludePayments  bool           `json:"includePayments"`
	IncludeDocuments bool           `json:"includeDocuments"`
	UpdatedAt        string         `json:"updatedAt"`
}

// DatevPersonAccount is a person account number assigned by an earlier
// export. Numbers are never reassigned, so later exports stay consistent.
type DatevPersonAccount struct {
	Kind      string `json:"kind"` // debtor or creditor
	Key       string `json:"key"`
	Number    string `json:"number"`
	Name      string `json:"name"`
	CreatedAt string `json:"createdAt"`
}

const (
	datevKey        = "datev-settings-v1"
	datevPersonsKey = "datev-person-accounts-v1"
)

func DefaultDatevSettings() DatevSettings {
	accounts, _ := datev.DefaultAccounts("SKR03", 4)
	return DatevSettings{FiscalYearStartMonth: 1, AccountLength: 4, Chart: "SKR03", PersonAccounts: "collective", Accounts: accounts, IncludeDocuments: true}
}

// DatevSettings returns the saved settings or unsaved defaults (UpdatedAt empty).
func (s *Store) DatevSettings() (DatevSettings, error) {
	v := DefaultDatevSettings()
	err := s.readSetting(datevKey, &v)
	return v, err
}

// NormalizeDatevSettings trims input and rejects values DATEV cannot import.
// Empty accounts are allowed; the export reports the cases they leave open.
func NormalizeDatevSettings(v DatevSettings) (DatevSettings, error) {
	v.ConsultantNumber = strings.TrimSpace(v.ConsultantNumber)
	v.ClientNumber = strings.TrimSpace(v.ClientNumber)
	if v.ConsultantNumber != "" {
		if n, err := strconv.Atoi(v.ConsultantNumber); err != nil || n < 1001 || n > 9999999 {
			return v, errors.New("Die Beraternummer hat 4 bis 7 Ziffern (1001 bis 9999999).")
		}
	}
	if v.ClientNumber != "" {
		if n, err := strconv.Atoi(v.ClientNumber); err != nil || n < 1 || n > 99999 {
			return v, errors.New("Die Mandantennummer hat 1 bis 5 Ziffern (1 bis 99999).")
		}
	}
	if v.FiscalYearStartMonth < 1 || v.FiscalYearStartMonth > 12 {
		return v, errors.New("Wähle den Monat, in dem dein Wirtschaftsjahr beginnt.")
	}
	if v.AccountLength < 4 || v.AccountLength > 8 {
		return v, errors.New("Die Sachkontenlänge muss zwischen 4 und 8 liegen.")
	}
	if v.Chart != "SKR03" && v.Chart != "SKR04" {
		return v, errors.New("Wähle SKR03 oder SKR04.")
	}
	if v.PersonAccounts != "collective" && v.PersonAccounts != "individual" {
		return v, errors.New("Wähle Sammelkonten oder einzelne Personenkonten.")
	}
	a := &v.Accounts
	r := &a.Revenue
	ledger := []struct {
		label string
		value *string
	}{
		{"Erlöse 19 %", &r.Standard19}, {"Erlöse 7 %", &r.Reduced7}, {"Erlöse 0 %", &r.ZeroRated},
		{"Steuerfreie Erlöse", &r.Exempt}, {"Ausfuhr", &r.Export}, {"Innergemeinschaftliche Lieferung", &r.IntraCommunity},
		{"§ 13b Inland", &r.ReverseCharge}, {"§ 13b übriges EU-Gebiet", &r.ReverseChargeEU}, {"Nicht steuerbar", &r.OutOfScope},
		{"Kleinunternehmer", &r.SmallBusiness}, {"Differenzbesteuerung", &r.Margin},
		{"Aufwandskonto", &a.Expense}, {"Bankkonto", &a.Bank},
	}
	for _, f := range ledger {
		*f.value = strings.TrimSpace(*f.value)
		if err := datev.CheckAccount(f.label, *f.value, v.AccountLength, false); err != nil {
			return v, err
		}
	}
	for category, account := range a.Categories {
		account = strings.TrimSpace(account)
		if account == "" {
			delete(a.Categories, category)
			continue
		}
		if err := datev.CheckAccount("Konto "+category, account, v.AccountLength, false); err != nil {
			return v, err
		}
		a.Categories[category] = account
	}
	a.Debtor, a.Creditor = strings.TrimSpace(a.Debtor), strings.TrimSpace(a.Creditor)
	if err := datev.CheckAccount("Debitorenkonto", a.Debtor, v.AccountLength, true); err != nil {
		return v, err
	}
	if err := datev.CheckAccount("Kreditorenkonto", a.Creditor, v.AccountLength, true); err != nil {
		return v, err
	}
	return v, nil
}

// SaveDatevSettings keeps every saved version as history, like the finance settings.
func (s *Store) SaveDatevSettings(v DatevSettings) (DatevSettings, error) {
	v, err := NormalizeDatevSettings(v)
	if err != nil {
		return v, err
	}
	v.UpdatedAt = now()
	raw, err := json.Marshal(v)
	if err != nil {
		return v, err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return v, err
	}
	defer tx.Rollback()
	if err = s.writeTracked(tx, datevKey, raw); err != nil {
		return v, err
	}
	if _, err = tx.Exec("INSERT INTO settings(key,value) VALUES(?,?)", "datev-settings-history:"+newID("evt"), raw); err != nil {
		return v, err
	}
	return v, tx.Commit()
}

func (s *Store) DatevPersonAccounts() ([]DatevPersonAccount, error) {
	v := []DatevPersonAccount{}
	err := s.readSetting(datevPersonsKey, &v)
	return v, err
}

// AddDatevPersonAccounts stores newly assigned person accounts. It refuses
// keys or numbers that were assigned in the meantime instead of reassigning.
func (s *Store) AddDatevPersonAccounts(added []DatevPersonAccount) error {
	if len(added) == 0 {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var raw []byte
	existing := []DatevPersonAccount{}
	if err := tx.QueryRow("SELECT value FROM settings WHERE key=?", datevPersonsKey).Scan(&raw); err == nil {
		if err := json.Unmarshal(raw, &existing); err != nil {
			return err
		}
	}
	keys, numbers := map[string]bool{}, map[string]bool{}
	for _, p := range existing {
		keys[p.Kind+"|"+p.Key], numbers[p.Number] = true, true
	}
	for _, p := range added {
		if keys[p.Kind+"|"+p.Key] || numbers[p.Number] {
			return fmt.Errorf("Das Personenkonto %s ist inzwischen vergeben. Starte den Export noch einmal.", p.Number)
		}
		keys[p.Kind+"|"+p.Key], numbers[p.Number] = true, true
		p.CreatedAt = now()
		existing = append(existing, p)
	}
	if raw, err = json.Marshal(existing); err != nil {
		return err
	}
	if _, err = tx.Exec("INSERT INTO settings(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value", datevPersonsKey, raw); err != nil {
		return err
	}
	return tx.Commit()
}
