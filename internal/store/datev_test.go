package store

import (
	"path/filepath"
	"testing"
)

func TestDatevSettingsValidationAndPersonAccounts(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	v, err := s.DatevSettings()
	if err != nil || v.UpdatedAt != "" || v.Chart != "SKR03" || v.Accounts.Revenue.Standard19 != "8400" {
		t.Fatalf("defaults %+v %v", v, err)
	}
	v.ConsultantNumber, v.ClientNumber = " 29098 ", "55003"
	saved, err := s.SaveDatevSettings(v)
	if err != nil || saved.ConsultantNumber != "29098" || saved.UpdatedAt == "" {
		t.Fatalf("save %+v %v", saved, err)
	}
	for name, change := range map[string]func(*DatevSettings){
		"consultant": func(v *DatevSettings) { v.ConsultantNumber = "12" },
		"client":     func(v *DatevSettings) { v.ClientNumber = "123456" },
		"length":     func(v *DatevSettings) { v.AccountLength = 5 }, // accounts still have four/five digits
		"chart":      func(v *DatevSettings) { v.Chart = "SKR07" },
		"letters":    func(v *DatevSettings) { v.Accounts.Expense = "49a0" },
	} {
		bad := saved
		change(&bad)
		if _, err := s.SaveDatevSettings(bad); err == nil {
			t.Errorf("%s: invalid settings saved", name)
		}
	}
	first := []DatevPersonAccount{{Kind: "debtor", Key: "customer:1", Number: "10000", Name: "A"}}
	if err := s.AddDatevPersonAccounts(first); err != nil {
		t.Fatal(err)
	}
	if err := s.AddDatevPersonAccounts([]DatevPersonAccount{{Kind: "debtor", Key: "customer:2", Number: "10000"}}); err == nil {
		t.Fatal("number assigned twice")
	}
	persons, err := s.DatevPersonAccounts()
	if err != nil || len(persons) != 1 || persons[0].CreatedAt == "" {
		t.Fatalf("persons %+v %v", persons, err)
	}
}
