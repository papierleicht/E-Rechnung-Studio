package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestProbeFlagsNeverImportLoginCredentials(t *testing.T) {
	for _, name := range []string{"FINTS_USER_ID", "FINTS_CUSTOMER_ID", "FINTS_PIN", "FINTS_SYSTEM_ID", "FINTS_TAN_MEDIUM"} {
		t.Setenv(name, "must-not-enter-probe")
	}
	t.Setenv("FINTS_ENDPOINT", "https://bank.example/fints")
	t.Setenv("FINTS_BANK_CODE", "12345678")
	t.Setenv("FINTS_PRODUCT_ID", "test-product")
	t.Setenv("FINTS_PRODUCT_VERSION", "1.0")
	t.Setenv("FINTS_TIMEOUT", "20s")
	config, err := parseProbeFlags(nil)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "must-not-enter-probe") {
		t.Fatal("anonymous probe imported login credentials")
	}
	for _, flag := range []string{"--pin", "--user-id", "--customer-id", "--system-id"} {
		if _, err := parseProbeFlags([]string{flag, "unused"}); err == nil {
			t.Fatalf("probe accepted login flag %s", flag)
		}
	}
	if _, err := parseProbeFlags([]string{"unexpected-positional-argument"}); err == nil {
		t.Fatal("probe accepted a positional argument")
	}
}
