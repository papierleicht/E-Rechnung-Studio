package main

import (
	"flag"
	"testing"
	"time"
)

func TestParseAmountFlag(t *testing.T) {
	for input, expected := range map[string]int64{
		"12.34": 1234,
		"12,3":  1230,
		"1":     100,
		"0.01":  1,
	} {
		actual, err := parseAmountFlag(input)
		if err != nil || actual != expected {
			t.Fatalf("parseAmountFlag(%q) = %d, %v; want %d", input, actual, err, expected)
		}
	}
	for _, input := range []string{"", "0", "-1", "1.234", "abc"} {
		if _, err := parseAmountFlag(input); err == nil {
			t.Fatalf("parseAmountFlag(%q) returned no error", input)
		}
	}
}

func TestFYRSTConnectionDefaults(t *testing.T) {
	for _, name := range []string{
		"FINTS_ENDPOINT", "FINTS_BANK_CODE", "FINTS_CUSTOMER_ID",
		"FINTS_SECURITY_FUNCTION", "FINTS_TAN_MEDIUM",
	} {
		t.Setenv(name, "")
	}
	flags := flag.NewFlagSet("test", flag.ContinueOnError)
	options := addConnectionFlags(flags)
	if err := flags.Parse([]string{
		"--fyrst",
		"--user-id", "postbank-id",
		"--pin", "password",
		"--product-id", "product",
	}); err != nil {
		t.Fatal(err)
	}
	config := options.config()
	if config.Endpoint != fyrstEndpoint ||
		config.BankCode != fyrstBankCode ||
		config.CustomerID != "postbank-id" ||
		config.HKTANVersion != 0 ||
		!config.AutoPoll ||
		config.Compatibility.DecoupledPollProcess != "2" ||
		!config.Compatibility.SkipTransactionSEPAAccountDiscovery {
		t.Fatalf("FYRST config = %#v", config)
	}
	if got := config.Compatibility.TransactionAccountDefaults; got.BIC != fyrstBIC ||
		got.Currency != "EUR" ||
		got.Subaccount != "EUR" {
		t.Fatalf("FYRST account defaults = %#v", got)
	}
	if len(config.Compatibility.ForceTANForOperations) != 3 ||
		config.Compatibility.PreferredSecurityFunctions[0] != "921" ||
		config.Compatibility.TANMediumRequiredForFunctions[0] != "921" {
		t.Fatalf("FYRST compatibility profile = %#v", config.Compatibility)
	}
}

func TestConnectionTimeoutFlagAndEnvironment(t *testing.T) {
	t.Setenv("FINTS_TIMEOUT", "2m")
	flags := flag.NewFlagSet("test", flag.ContinueOnError)
	options := addConnectionFlags(flags)
	if err := flags.Parse([]string{"--timeout", "3m"}); err != nil {
		t.Fatal(err)
	}
	if options.config().Timeout != 3*time.Minute {
		t.Fatalf("timeout = %s, want 3m", options.config().Timeout)
	}

	t.Setenv("FINTS_TIMEOUT", "not-a-duration")
	flags = flag.NewFlagSet("test-invalid", flag.ContinueOnError)
	options = addConnectionFlags(flags)
	if options.config().Timeout >= 0 {
		t.Fatalf("invalid environment timeout = %s", options.config().Timeout)
	}
}
