package validation

import "testing"

// Unreadable input is a failed check (state "error"), not an invalid invoice,
// as before the official rules moved to go-e-rechnung-validator.
func TestUnreadableInputIsReportedAsSyntaxError(t *testing.T) {
	v, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for name, c := range map[string]struct{ data, rule string }{
		"malformed":  {"<kein xml", "XML"},
		"doctype":    {`<?xml version="1.0"?><!DOCTYPE x><x/>`, "DOCTYPE"},
		"no invoice": {`<?xml version="1.0"?><x/>`, "XML"},
	} {
		got := v.validate([]byte(c.data))
		if got.State != "error" || len(got.Issues) != 1 || got.Issues[0].Stage != "syntax" || got.Issues[0].RuleID != c.rule {
			t.Errorf("%s: %+v", name, got)
		}
	}
}
