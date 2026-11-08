package validation

import "testing"

func TestScenarioLevelsFollowKoSITConfiguration(t *testing.T) {
	const path = "rules/xrechnung/scenarios.xml"
	cases := []struct{ scenario, id, want string }{
		{"EN16931 XRechnung (UBL Invoice)", "UBL-CR-646", "error"},
		{"EN16931 XRechnung (UBL Invoice)", "BR-CL-23", "warning"},
		{"EN16931 XRechnung (UBL CreditNote)", "BR-CL-21", "warning"},
		{"EN16931 XRechnung (UBL CreditNote)", "UBL-CR-646", ""},
		{"EN16931 XRechnung (CII)", "CII-SR-452", "error"},
		{"EN16931 XRechnung (CII)", "CII-SR-475", "information"},
	}
	for _, c := range cases {
		levels, err := scenarioLevels(path, c.scenario)
		if err != nil {
			t.Fatal(err)
		}
		if got := levels[c.id]; got != c.want {
			t.Errorf("%s %s = %q, want %q", c.scenario, c.id, got, c.want)
		}
	}
	if _, err := scenarioLevels(path, "missing"); err == nil {
		t.Error("unknown scenario must fail")
	}
}
