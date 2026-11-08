package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/BergerAPI/go-fints"
)

const usage = `fints is a small capability-negotiated FinTS 3.0 client.

Usage:
  fints probe --endpoint URL --bank-code BLZ --product-id REGISTERED_ID
  fints accounts [connection flags]
  fints transactions [connection flags] [account flags]
  fints capabilities [connection flags]
  fints tan-methods [connection flags]
  fints tan-media [connection flags]
  fints transfer [connection flags] [transfer flags] --execute

Connection flags may also be set with FINTS_ENDPOINT, FINTS_BANK_CODE,
FINTS_USER_ID, FINTS_CUSTOMER_ID, FINTS_PIN, FINTS_PRODUCT_ID,
FINTS_PRODUCT_VERSION, FINTS_SYSTEM_ID, FINTS_COUNTRY_CODE,
FINTS_SECURITY_FUNCTION, FINTS_TAN_MEDIUM, and FINTS_TIMEOUT.

Examples:
  fints accounts --fyrst --user-id POSTBANK_ID \
    --product-id YOUR_REGISTERED_PRODUCT_ID --tan-medium BESTSIGN_DEVICE

  fints accounts --endpoint https://bank.example/fints --bank-code 12345678 \
    --user-id alice --product-id YOUR_REGISTERED_PRODUCT_ID

  fints transactions --endpoint https://bank.example/fints --bank-code 12345678 \
    --user-id alice --product-id YOUR_REGISTERED_PRODUCT_ID \
    --iban DE89370400440532013000 \
    --from 2026-01-01 --to 2026-01-31

probe requests public bank parameters anonymously. It accepts no login flags and
does not read FINTS_USER_ID, FINTS_CUSTOMER_ID or FINTS_PIN. Advertised versions
are bank metadata, not evidence that this client implements those versions.

The PIN should normally be supplied through FINTS_PIN so it does not appear in
the shell history. Output is JSON.`

const (
	fyrstEndpoint = "https://hbci.postbank.de/banking/hbci.do"
	fyrstBankCode = "10010010"
	fyrstBIC      = "PBNKDEFF"
)

func main() {
	if err := run(context.Background(), os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		fmt.Println(usage)
		return nil
	}

	switch args[0] {
	case "probe":
		config, err := parseProbeFlags(args[1:])
		if err != nil {
			return err
		}
		result, err := fints.Probe(ctx, config)
		if err != nil {
			return err
		}
		return writeJSON(result)

	case "accounts":
		flags := flag.NewFlagSet("accounts", flag.ContinueOnError)
		flags.SetOutput(os.Stderr)
		options := addConnectionFlags(flags)
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		config := options.config()
		config.TANHandler = interactiveTANHandler
		client, err := fints.NewClient(config)
		if err != nil {
			return err
		}
		accounts, err := client.Accounts(ctx)
		if err != nil {
			return err
		}
		return writeJSON(accounts)

	case "transactions":
		flags := flag.NewFlagSet("transactions", flag.ContinueOnError)
		flags.SetOutput(os.Stderr)
		options := addConnectionFlags(flags)
		var account fints.Account
		var fromText, toText string
		flags.StringVar(&account.IBAN, "iban", "", "account IBAN")
		flags.StringVar(&account.BIC, "bic", "", "account BIC")
		flags.StringVar(&account.AccountNumber, "account-number", "", "national account number")
		flags.StringVar(&account.Subaccount, "subaccount", "", "national subaccount marker")
		flags.StringVar(&account.BankCode, "account-bank-code", "", "account bank code (defaults to --bank-code)")
		flags.StringVar(&fromText, "from", "", "first booking date (YYYY-MM-DD)")
		flags.StringVar(&toText, "to", "", "last booking date (YYYY-MM-DD)")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		from, err := parseDateFlag("from", fromText)
		if err != nil {
			return err
		}
		to, err := parseDateFlag("to", toText)
		if err != nil {
			return err
		}
		config := options.config()
		config.TANHandler = interactiveTANHandler
		client, err := fints.NewClient(config)
		if err != nil {
			return err
		}
		transactions, err := client.Transactions(ctx, account, from, to)
		if err != nil {
			return err
		}
		return writeJSON(transactions)

	case "tan-media":
		flags := flag.NewFlagSet("tan-media", flag.ContinueOnError)
		flags.SetOutput(os.Stderr)
		options := addConnectionFlags(flags)
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		client, err := fints.NewClient(options.config())
		if err != nil {
			return err
		}
		media, err := client.TANMedia(ctx)
		if err != nil {
			return err
		}
		return writeJSON(media)

	case "tan-methods":
		flags := flag.NewFlagSet("tan-methods", flag.ContinueOnError)
		flags.SetOutput(os.Stderr)
		options := addConnectionFlags(flags)
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		client, err := fints.NewClient(options.config())
		if err != nil {
			return err
		}
		methods, err := client.TANMethods(ctx)
		if err != nil {
			return err
		}
		return writeJSON(methods)

	case "capabilities":
		flags := flag.NewFlagSet("capabilities", flag.ContinueOnError)
		flags.SetOutput(os.Stderr)
		options := addConnectionFlags(flags)
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		client, err := fints.NewClient(options.config())
		if err != nil {
			return err
		}
		capabilities, err := client.Capabilities(ctx)
		if err != nil {
			return err
		}
		return writeJSON(capabilities)

	case "transfer":
		flags := flag.NewFlagSet("transfer", flag.ContinueOnError)
		flags.SetOutput(os.Stderr)
		options := addConnectionFlags(flags)
		var transfer fints.SEPACreditTransfer
		var amountText, executionText string
		var execute bool
		flags.StringVar(&transfer.Debtor.IBAN, "debtor-iban", "", "debtor account IBAN")
		flags.StringVar(&transfer.Debtor.BIC, "debtor-bic", "", "debtor account BIC")
		flags.StringVar(&transfer.DebtorName, "debtor-name", "", "debtor name")
		flags.StringVar(&transfer.CreditorIBAN, "creditor-iban", "", "creditor IBAN")
		flags.StringVar(&transfer.CreditorBIC, "creditor-bic", "", "creditor BIC")
		flags.StringVar(&transfer.CreditorName, "creditor-name", "", "creditor name")
		flags.StringVar(&amountText, "amount", "", "EUR amount, for example 12.34")
		flags.StringVar(&transfer.Purpose, "purpose", "", "unstructured remittance information")
		flags.StringVar(&transfer.EndToEndReference, "end-to-end-id", "", "end-to-end reference")
		flags.StringVar(&executionText, "execution-date", "", "execution date (YYYY-MM-DD; defaults to today)")
		flags.BoolVar(&execute, "execute", false, "actually submit the transfer")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if !execute {
			return errors.New("refusing to submit a transfer without --execute")
		}
		amount, err := parseAmountFlag(amountText)
		if err != nil {
			return err
		}
		transfer.Amount = amount
		transfer.ExecutionDate, err = parseDateFlag("execution-date", executionText)
		if err != nil {
			return err
		}
		config := options.config()
		config.TANHandler = interactiveTANHandler
		client, err := fints.NewClient(config)
		if err != nil {
			return err
		}
		result, err := client.SEPACreditTransfer(ctx, transfer)
		if err != nil {
			return err
		}
		return writeJSON(result)

	default:
		return fmt.Errorf("unknown command %q; use \"fints help\"", args[0])
	}
}

// Probe deliberately has its own flags, so login credentials cannot reach an
// anonymous request through connection flags or the environment.
func parseProbeFlags(args []string) (fints.ProbeConfig, error) {
	var config fints.ProbeConfig
	flags := flag.NewFlagSet("probe", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	flags.StringVar(&config.Endpoint, "endpoint", env("FINTS_ENDPOINT"), "FinTS HTTPS endpoint")
	flags.StringVar(&config.BankCode, "bank-code", env("FINTS_BANK_CODE"), "German bank code (BLZ)")
	flags.StringVar(&config.ProductID, "product-id", env("FINTS_PRODUCT_ID"), "registered FinTS product ID")
	flags.StringVar(&config.ProductVersion, "product-version", valueOr(env("FINTS_PRODUCT_VERSION"), "1.0"), "product version (at most 5 characters)")
	flags.DurationVar(&config.Timeout, "timeout", durationOrInvalid(env("FINTS_TIMEOUT"), 20*time.Second), "total probe timeout including cleanup")
	if err := flags.Parse(args); err != nil {
		return config, err
	}
	if flags.NArg() != 0 {
		return config, errors.New("probe accepts flags only")
	}
	return config, nil
}

type connectionOptions struct {
	value fints.Config
	fyrst bool
}

func addConnectionFlags(flags *flag.FlagSet) *connectionOptions {
	options := &connectionOptions{}
	config := &options.value
	flags.BoolVar(&options.fyrst, "fyrst", false, "use FYRST/Postbank endpoint and BestSign defaults")
	flags.StringVar(&config.Endpoint, "endpoint", env("FINTS_ENDPOINT"), "FinTS HTTPS endpoint")
	flags.StringVar(&config.CountryCode, "country-code", valueOr(env("FINTS_COUNTRY_CODE"), "280"), "three-digit FinTS country identifier")
	flags.StringVar(&config.BankCode, "bank-code", env("FINTS_BANK_CODE"), "German bank code (BLZ)")
	flags.StringVar(&config.UserID, "user-id", env("FINTS_USER_ID"), "FinTS login/user ID")
	flags.StringVar(&config.CustomerID, "customer-id", env("FINTS_CUSTOMER_ID"), "customer ID (defaults to user ID)")
	flags.StringVar(&config.PIN, "pin", env("FINTS_PIN"), "FinTS PIN (prefer FINTS_PIN)")
	flags.StringVar(&config.ProductID, "product-id", env("FINTS_PRODUCT_ID"), "registered FinTS product ID")
	flags.StringVar(&config.ProductVersion, "product-version", valueOr(env("FINTS_PRODUCT_VERSION"), "1.0"), "product version")
	flags.StringVar(&config.SystemID, "system-id", env("FINTS_SYSTEM_ID"), "cached FinTS system ID")
	flags.StringVar(&config.SecurityFunction, "security-function", env("FINTS_SECURITY_FUNCTION"), "PIN/TAN security function (auto-detected by default)")
	flags.StringVar(&config.TANMedium, "tan-medium", env("FINTS_TAN_MEDIUM"), "exact TAN medium/device name")
	flags.IntVar(&config.HKTANVersion, "hktan-version", 0, "HKTAN version (auto-detected by default)")
	flags.BoolVar(&config.AutoPoll, "auto-poll", true, "poll for decoupled approval")
	flags.DurationVar(&config.PollInterval, "poll-interval", 2*time.Second, "decoupled approval polling interval")
	flags.IntVar(&config.MaxPolls, "max-polls", 60, "maximum decoupled approval polls")
	flags.DurationVar(
		&config.Timeout,
		"timeout",
		durationOrInvalid(env("FINTS_TIMEOUT"), 90*time.Second),
		"HTTP request timeout (for example 90s or 2m)",
	)
	return options
}

func (options *connectionOptions) config() fints.Config {
	config := options.value
	if options.fyrst {
		if config.Endpoint == "" {
			config.Endpoint = fyrstEndpoint
		}
		if config.BankCode == "" {
			config.BankCode = fyrstBankCode
		}
		if config.CustomerID == "" {
			config.CustomerID = config.UserID
		}
		config.AutoPoll = true
		config.Compatibility = fints.CompatibilityOptions{
			SkipTransactionSEPAAccountDiscovery: true,
			TransactionAccountDefaults: fints.Account{
				BIC:        fyrstBIC,
				Subaccount: "EUR",
				Currency:   "EUR",
			},
			ForceTANForOperations:         []string{"HKKAZ", "HKCAZ", "HKTAB"},
			PreferredSecurityFunctions:    []string{"921", "920"},
			TANMediumRequiredForFunctions: []string{"921", "920"},
			DecoupledPollProcess:          "2",
		}
	}
	return config
}

func env(name string) string {
	return strings.TrimSpace(os.Getenv(name))
}

func valueOr(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func durationOrInvalid(value string, fallback time.Duration) time.Duration {
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return -1
	}
	return parsed
}

func parseDateFlag(name, value string) (time.Time, error) {
	if value == "" {
		return time.Time{}, nil
	}
	parsed, err := time.Parse(time.DateOnly, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("--%s must use YYYY-MM-DD: %w", name, err)
	}
	return parsed, nil
}

func writeJSON(value any) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		return errors.New("write JSON output: " + err.Error())
	}
	return nil
}

func parseAmountFlag(value string) (int64, error) {
	value = strings.TrimSpace(strings.ReplaceAll(value, ",", "."))
	parts := strings.Split(value, ".")
	if value == "" || len(parts) > 2 {
		return 0, errors.New("--amount must be a positive decimal amount")
	}
	whole, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || whole < 0 {
		return 0, errors.New("--amount must be a positive decimal amount")
	}
	fraction := "00"
	if len(parts) == 2 {
		if len(parts[1]) > 2 {
			return 0, errors.New("--amount supports at most two decimal places")
		}
		fraction = parts[1] + strings.Repeat("0", 2-len(parts[1]))
	}
	cents, err := strconv.ParseInt(fraction, 10, 64)
	if err != nil {
		return 0, errors.New("--amount must be a positive decimal amount")
	}
	result := whole*100 + cents
	if result <= 0 {
		return 0, errors.New("--amount must be greater than zero")
	}
	return result, nil
}

func interactiveTANHandler(ctx context.Context, challenge fints.TANChallenge) (string, error) {
	if challenge.Challenge != "" {
		fmt.Fprintln(os.Stderr, challenge.Challenge)
	}
	if challenge.Decoupled {
		fmt.Fprintln(os.Stderr, "Approve the request in your authentication app or device; waiting for confirmation...")
		return "", nil
	}
	fmt.Fprint(os.Stderr, "TAN: ")
	value, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil {
		return "", err
	}
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	default:
	}
	// TAN values are case-sensitive. Trim only surrounding terminal
	// whitespace, never normalize the value.
	return strings.TrimSpace(value), nil
}
