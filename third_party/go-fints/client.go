package fints

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	fintsVersion         = "300"
	defaultProduct       = "1.0"
	defaultSecurity      = "999"
	defaultHTTPTimeout   = 90 * time.Second
	maxResponseBodyBytes = 16 << 20
)

// Client is a capability-negotiated FinTS 3.0 PIN/TAN client. A Client may be reused
// and is safe for concurrent callers; calls are serialized because a FinTS
// system ID and parameter data are shared between dialogs.
type Client struct {
	config     Config
	httpClient *http.Client

	mu              sync.Mutex
	systemID        string
	bpdVersion      int
	updVersion      int
	hkkazVers       []int
	hkcazVers       []int
	lastAccount     []Account
	capabilities    map[string]*Capability
	allowedSecurity []string
	tanMethods      map[string]TANMethod
	tanPolling      map[string]decoupledPollingParameters
	tanRequirements map[string]bool
	hktabVersion    int
	parametersReady bool
	// authenticatedParametersReady is set only after an initialization using
	// the selected two-step security function has requested fresh UPD.
	authenticatedParametersReady bool
	trace                        []ExchangeDiagnostic
	traceDropped                 int
	// statementBalance is the closing balance of the last Transactions call.
	statementBalance *DatedBalance
}

type dialog struct {
	id             string
	nextMessage    int
	systemID       string
	security       string
	accounts       []Account
	hkkazVersions  []int
	lastTANRequest int
	lastTANOrder   int
}

// NewClient validates config and constructs a Client without performing I/O.
func NewClient(config Config) (result *Client, err error) {
	defer func() { err = failureWithResponse(err, "configure_client", "invalid_configuration", nil, nil) }()
	if strings.TrimSpace(config.Endpoint) == "" {
		return nil, errors.New("FinTS endpoint is required")
	}
	endpoint, err := url.Parse(config.Endpoint)
	if err != nil {
		return nil, fmt.Errorf("parse FinTS endpoint: %w", err)
	}
	if endpoint.Scheme != "https" && !(config.AllowHTTP && endpoint.Scheme == "http") {
		return nil, errors.New("FinTS endpoint must use HTTPS")
	}
	if endpoint.User != nil || endpoint.Fragment != "" {
		return nil, errors.New("FinTS endpoint must not contain credentials or a fragment")
	}
	if endpoint.Host == "" {
		return nil, errors.New("FinTS endpoint must include a host")
	}
	for name, value := range map[string]string{
		"bank code":  config.BankCode,
		"user ID":    config.UserID,
		"PIN":        config.PIN,
		"product ID": config.ProductID,
	} {
		if strings.TrimSpace(value) == "" {
			return nil, fmt.Errorf("%s is required", name)
		}
	}
	if config.CustomerID == "" {
		config.CustomerID = config.UserID
	}
	if config.CountryCode == "" {
		config.CountryCode = "280"
	}
	if len(config.CountryCode) != 3 {
		return nil, errors.New("country code must be the three-digit FinTS country identifier")
	}
	if config.ProductVersion == "" {
		config.ProductVersion = defaultProduct
	}
	if err := validateProductMetadata(config.ProductID, config.ProductVersion); err != nil {
		return nil, err
	}
	if len(config.SecurityFunction) > 3 {
		return nil, errors.New("security function must contain at most three characters")
	}
	pollProcess := strings.ToUpper(strings.TrimSpace(
		config.Compatibility.DecoupledPollProcess,
	))
	switch pollProcess {
	case "", "AUTO":
		config.Compatibility.DecoupledPollProcess = ""
	case "2", "S":
		config.Compatibility.DecoupledPollProcess = pollProcess
	default:
		return nil, errors.New("decoupled poll process must be auto, 2, or S")
	}
	config.Compatibility.ForceTANForOperations = normalizeCodes(
		config.Compatibility.ForceTANForOperations,
	)
	config.Compatibility.PreferredSecurityFunctions = normalizeCodes(
		config.Compatibility.PreferredSecurityFunctions,
	)
	config.Compatibility.TANMediumRequiredForFunctions = normalizeCodes(
		config.Compatibility.TANMediumRequiredForFunctions,
	)
	config.TANMedium = strings.TrimSpace(config.TANMedium)
	if config.PollInterval <= 0 {
		config.PollInterval = 2 * time.Second
	}
	if config.MaxPolls <= 0 {
		config.MaxPolls = 60
	}
	if config.Timeout < 0 {
		return nil, errors.New("HTTP timeout must not be negative")
	}

	httpClient := config.HTTPClient
	if httpClient == nil {
		timeout := config.Timeout
		if timeout == 0 {
			timeout = defaultHTTPTimeout
		}
		httpClient = &http.Client{Timeout: timeout}
	}

	// A 307/308 redirect could replay the entire PIN-bearing FinTS envelope to
	// another host. A shallow copy preserves the caller's transport and timeout
	// without mutating their HTTP client; redirects are never followed.
	httpCopy := *httpClient
	httpCopy.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return errors.New("FinTS redirects are not allowed")
	}
	httpClient = &httpCopy

	systemID := config.SystemID
	if systemID == "" {
		systemID = "0"
	}
	return &Client{
		config:          config,
		httpClient:      httpClient,
		systemID:        systemID,
		tanMethods:      make(map[string]TANMethod),
		tanPolling:      make(map[string]decoupledPollingParameters),
		tanRequirements: make(map[string]bool),
		capabilities:    make(map[string]*Capability),
	}, nil
}

// HKVVB3 limits product identification to 25 characters and its version to 5.
// FinTS encodes textual fields in ISO-8859-1: each encodable rune is one data
// character, regardless of its UTF-8 size or the protocol escaping it requires.
func validateProductMetadata(productID, productVersion string) error {
	for _, field := range []struct {
		name  string
		value string
		limit int
	}{
		{"product ID", productID, 25},
		{"product version", productVersion, 5},
	} {
		if strings.TrimSpace(field.value) == "" {
			return fmt.Errorf("%s is required", field.name)
		}
		characters := 0
		for _, r := range field.value {
			if r > 255 {
				return fmt.Errorf("%s must only contain ISO-8859-1 characters", field.name)
			}
			characters++
		}
		if characters > field.limit {
			return fmt.Errorf("%s must contain at most %d ISO-8859-1 characters", field.name, field.limit)
		}
	}
	return nil
}

func normalizeCodes(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.ToUpper(strings.TrimSpace(value))
		if value != "" && !containsString(result, value) {
			result = append(result, value)
		}
	}
	return result
}

// Accounts opens a FinTS dialog and returns the accounts advertised by the
// bank. It combines UPD account metadata with the optional HKSPA SEPA account
// list.
func (c *Client) Accounts(ctx context.Context) ([]Account, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	d, err := c.openDialog(ctx)
	if err != nil {
		return nil, err
	}
	defer c.endDialog(ctx, d)

	accounts := append([]Account(nil), d.accounts...)
	response, err := c.executeCommand(ctx, d, "HKSPA:{n}:1'", "HKSPA")
	if err == nil {
		accounts = mergeAccounts(accounts, accountsFromHISPA(response))
	} else {
		var bankErr *BankError
		if !errors.As(err, &bankErr) || len(accounts) == 0 {
			return nil, err
		}
		// Optional HKSPA may fall back only when UPD actually returned accounts.
		// Otherwise preserve the original bank error and its diagnostic context.
	}
	if len(accounts) == 0 {
		return nil, failureWithResponse(errors.New("bank returned no account information"), "accounts", "no_accounts", []string{"HKSPA:"}, response)
	}
	for index := range accounts {
		accounts[index] = normalizeAccount(accounts[index])
	}
	c.lastAccount = append([]Account(nil), accounts...)
	return accounts, nil
}

// Transactions fetches booked MT940 transactions for account in the inclusive
// date range. A zero date leaves that side of the range open.

func (c *Client) Transactions(ctx context.Context, account Account, from, to time.Time) ([]Transaction, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if account.IBAN == "" && account.AccountNumber == "" {
		return nil, errors.New("account requires an IBAN or account number")
	}
	if !from.IsZero() && !to.IsZero() && from.After(to) {
		return nil, errors.New("transaction start date must not be after end date")
	}
	c.statementBalance = nil
	account = normalizeAccount(account)
	if account.CountryCode == "" {
		account.CountryCode = c.config.CountryCode
	}
	if account.BankCode == "" {
		account.BankCode = c.config.BankCode
	}

	d, err := c.openDialog(ctx)
	if err != nil {
		return nil, err
	}
	defer c.endDialog(ctx, d)

	account = resolveAccount(account, d.accounts)
	if account.IBAN != "" &&
		!c.config.Compatibility.SkipTransactionSEPAAccountDiscovery {
		if response, spaErr := c.executeCommand(ctx, d, "HKSPA:{n}:1'", "HKSPA"); spaErr == nil {
			d.accounts = mergeAccounts(d.accounts, accountsFromHISPA(response))
			account = resolveAccount(account, d.accounts)
		}
	}
	account = c.transactionAccountDefaults(account)
	version := selectHKKAZVersion(d.hkkazVersions, account)
	if len(d.hkkazVersions) == 0 && len(c.hkcazVers) > 0 {
		return c.transactionsCAMT(ctx, d, account, from, to)
	}
	if version == 7 && account.IBAN == "" {
		if len(c.hkcazVers) > 0 {
			return c.transactionsCAMT(ctx, d, account, from, to)
		}
		return nil, errors.New("HKKAZ version 7 requires the account IBAN")
	}
	if version < 7 && account.AccountNumber == "" {
		if len(c.hkcazVers) > 0 && account.IBAN != "" {
			return c.transactionsCAMT(ctx, d, account, from, to)
		}
		return nil, fmt.Errorf(
			"HKKAZ version %d requires a national account number that could not be derived from IBAN %q",
			version,
			account.IBAN,
		)
	}
	var booked []byte
	maxDays := 0
	if capability := c.capabilities["HKKAZ"]; capability != nil {
		maxDays = capability.MaxDays
	}
	for _, dateRange := range splitDateRange(from, to, maxDays) {
		touchdown := ""
		seenTouchdowns := make(map[string]bool)
		for page := 0; page < 100; page++ {
			response, resolvedAccount, resolvedVersion, sendErr := c.executeHKKAZ(
				ctx,
				d,
				version,
				account,
				dateRange.from,
				dateRange.to,
				touchdown,
			)
			account = resolvedAccount
			version = resolvedVersion
			if sendErr != nil {
				if len(c.hkcazVers) > 0 && account.IBAN != "" &&
					shouldFallbackToCAMT(sendErr) {
					transactions, camtErr := c.transactionsCAMT(ctx, d, account, from, to)
					if camtErr == nil {
						return transactions, nil
					}
					return nil, fmt.Errorf(
						"HKKAZ transaction retrieval failed (%v); HKCAZ fallback failed: %w",
						sendErr,
						camtErr,
					)
				}
				return nil, sendErr
			}
			if len(findSegments(response, "HIKAZ")) == 0 {
				return nil, errors.New("bank returned no HIKAZ transaction response")
			}
			booked = append(booked, transactionData(response)...)
			next := touchdownPoint(response)
			if next == "" {
				break
			}
			if seenTouchdowns[next] {
				return nil, errors.New("FinTS transaction pagination repeated a continuation token")
			}
			seenTouchdowns[next] = true
			touchdown = next
			if page == 99 {
				return nil, errors.New("FinTS transaction pagination exceeded 100 pages")
			}
		}
	}

	if len(booked) == 0 {
		return []Transaction{}, nil
	}
	transactions, err := parseMT940ForAccount(booked, account)
	if err != nil {
		return nil, err
	}
	c.statementBalance = mt940ClosingBalance(booked)
	return transactions, nil
}

func shouldFallbackToCAMT(err error) bool {
	// A different transaction format can resolve account-layout incompatibility,
	// but it cannot resolve authentication, TAN-device, or dialog-state errors.
	return bankErrorHasCode(err, "9160")
}

func (c *Client) transactionAccountDefaults(account Account) Account {
	defaults := c.config.Compatibility.TransactionAccountDefaults
	if account.BIC == "" {
		account.BIC = defaults.BIC
	}
	if account.Currency == "" {
		account.Currency = defaults.Currency
	}
	if defaults.Subaccount != "" &&
		(account.Subaccount == "" || account.Subaccount == "00") {
		account.Subaccount = defaults.Subaccount
	}
	if account.AccountNumber == "" {
		account.AccountNumber = defaults.AccountNumber
	}
	if account.CountryCode == "" {
		account.CountryCode = defaults.CountryCode
	}
	if account.BankCode == "" {
		account.BankCode = defaults.BankCode
	}
	return account
}

// executeHKKAZ applies response-driven account compatibility retries. HKKAZ 7
// can fall back to an advertised national version, while HKKAZ 5/6 can replace
// a rejected numeric subaccount marker with the UPD currency.
func (c *Client) executeHKKAZ(
	ctx context.Context,
	d *dialog,
	version int,
	account Account,
	from, to time.Time,
	touchdown string,
) ([]segment, Account, int, error) {
	command := buildHKKAZ(version, account, from, to, touchdown)
	response, err := c.executeCommand(ctx, d, command, "HKKAZ")
	if err == nil {
		return response, account, version, nil
	}
	if !isMissingAccountComponent(err) {
		return nil, account, version, err
	}

	// In the international account structure used by HKKAZ 7, component 2,2
	// is the BIC. If the bank rejects it and also advertised a national HKKAZ
	// version, retry that version using the already resolved domestic tuple.
	if version == 7 && account.AccountNumber != "" {
		if nationalVersion := highestNationalHKKAZVersion(d.hkkazVersions); nationalVersion != 0 {
			version = nationalVersion
			command = buildHKKAZ(version, account, from, to, touchdown)
			response, err = c.executeCommand(ctx, d, command, "HKKAZ")
			if err == nil {
				return response, account, version, nil
			}
			if !isMissingAccountComponent(err) {
				return nil, account, version, fmt.Errorf(
					"HKKAZ %d fallback after HKKAZ 7 rejected account element 2,2: %w",
					version,
					err,
				)
			}
		}
	}

	currency := strings.ToUpper(strings.TrimSpace(account.Currency))
	if currency == "" {
		currency = strings.ToUpper(strings.TrimSpace(
			c.config.Compatibility.TransactionAccountDefaults.Currency,
		))
	}
	if version < 7 && currency != "" && account.Subaccount != currency {
		account.Subaccount = currency
		command = buildHKKAZ(version, account, from, to, touchdown)
		response, err = c.executeCommand(ctx, d, command, "HKKAZ")
		if err == nil {
			return response, account, version, nil
		}
		return nil, account, version, fmt.Errorf(
			"HKKAZ %d retry with subaccount %q: %w",
			version,
			currency,
			err,
		)
	}

	if version == 7 {
		return nil, account, version, fmt.Errorf(
			"HKKAZ 7 rejected account element 2,2 (BIC %q) and no advertised national fallback succeeded: %w",
			account.BIC,
			err,
		)
	}
	return nil, account, version, fmt.Errorf(
		"HKKAZ %d rejected account element 2,2 (subaccount %q, currency %q): %w",
		version,
		account.Subaccount,
		currency,
		err,
	)
}

func highestNationalHKKAZVersion(versions []int) int {
	selected := 0
	for _, version := range versions {
		if version >= 5 && version <= 6 && version > selected {
			selected = version
		}
	}
	return selected
}

func isMissingAccountComponent(err error) bool {
	var bankErr *BankError
	if !errors.As(err, &bankErr) {
		return false
	}
	for _, detail := range bankErr.Details {
		if detail.Code == "9160" && strings.TrimSpace(detail.ReferenceElement) == "2,2" {
			return true
		}
	}
	return false
}

type transactionDateRange struct {
	from time.Time
	to   time.Time
}

func splitDateRange(from, to time.Time, maxDays int) []transactionDateRange {
	if maxDays <= 0 || from.IsZero() || to.IsZero() {
		return []transactionDateRange{{from: from, to: to}}
	}
	var result []transactionDateRange
	start := from
	for !start.After(to) {
		end := start.AddDate(0, 0, maxDays-1)
		if end.After(to) {
			end = to
		}
		result = append(result, transactionDateRange{from: start, to: end})
		start = end.AddDate(0, 0, 1)
	}
	return result
}

func (c *Client) openDialog(ctx context.Context) (result *dialog, err error) {
	defer func() { err = failureWithResponse(err, "open_dialog", "", nil, nil) }()
	if err := c.ensureAuthentication(ctx); err != nil {
		return nil, err
	}

	security := c.config.SecurityFunction
	if security == "" {
		security = defaultSecurity
	}
	d := &dialog{id: "0", nextMessage: 1, systemID: c.systemID, security: security}
	requestedUPDVersion := c.updVersion
	if security != defaultSecurity && !c.authenticatedParametersReady {
		// HBCI4Java explicitly resets the local UPD version before its first
		// authenticated initialization. The version announced during the
		// one-step synchronization does not mean that this client already has
		// the user-specific PSD2 parameters.
		requestedUPDVersion = 0
	}
	commands := []string{
		fmt.Sprintf("HKIDN:{n}:2+%s:%s+%s+%s+1'",
			escape(c.config.CountryCode), escape(c.config.BankCode), escape(c.config.CustomerID), escape(c.systemID)),
		fmt.Sprintf("HKVVB:{n}:3+%d+%d+0+%s+%s'",
			c.bpdVersion, requestedUPDVersion, escape(c.config.ProductID), escape(c.config.ProductVersion)),
	}
	if security != defaultSecurity {
		method := c.tanMethods[security]
		if method.MediumRequired && c.config.TANMedium == "" {
			media, err := c.tanMediaLocked(ctx)
			if err != nil {
				return nil, err
			}
			if len(media) == 1 {
				c.config.TANMedium = media[0].Name
			} else {
				return nil, &TANMediumRequiredError{Media: media}
			}
		}
		commands = append(commands, buildHKTAN(c.hktanVersion(), "4", "HKIDN", "", c.config.TANMedium))
	}
	response, err := c.sendCommands(ctx, d, commands)
	if err != nil {
		return nil, fmt.Errorf("initialize FinTS dialog: %w", err)
	}
	dialogID := responseDialogID(response)
	if dialogID == "" || dialogID == "0" {
		return nil, failureWithResponse(errors.New("bank did not return a dialog ID"), "dialog_init", "missing_dialog_id", nil, response)
	}
	d.id = dialogID
	tanResponse, err := c.completeTAN(ctx, d, response)
	if err != nil {
		c.endDialog(ctx, d)
		return nil, fmt.Errorf("authenticate FinTS dialog: %w", err)
	}
	response = append(response, tanResponse...)
	d.accounts = accountsFromUPD(response)
	d.hkkazVersions = hkkazVersions(response)
	c.updateParameters(response)
	if security != defaultSecurity {
		c.authenticatedParametersReady = true
	}
	if len(d.hkkazVersions) == 0 {
		d.hkkazVersions = append([]int(nil), c.hkkazVers...)
	}
	return d, nil
}

func (c *Client) synchronize(ctx context.Context) (err error) {
	defer func() { err = failureWithResponse(err, "synchronize", "", nil, nil) }()
	d := &dialog{id: "0", nextMessage: 1, systemID: "0", security: defaultSecurity}
	commands := []string{
		fmt.Sprintf("HKIDN:{n}:2+%s:%s+%s+0+1'",
			escape(c.config.CountryCode), escape(c.config.BankCode), escape(c.config.CustomerID)),
		fmt.Sprintf("HKVVB:{n}:3+0+0+0+%s+%s'",
			escape(c.config.ProductID), escape(c.config.ProductVersion)),
		"HKSYN:{n}:3+0'",
	}
	response, err := c.sendCommands(ctx, d, commands)
	if err != nil {
		return fmt.Errorf("synchronize FinTS system ID: %w", err)
	}
	systemID := ""
	for _, s := range findSegments(response, "HISYN") {
		systemID = component(s, 0, 0)
		if systemID != "" {
			break
		}
	}
	if systemID == "" || systemID == "0" {
		return failureWithResponse(errors.New("bank did not return a system ID"), "synchronize", "missing_system_id", commands, response)
	}
	d.id = responseDialogID(response)
	d.systemID = systemID
	c.systemID = systemID
	c.config.SystemID = systemID
	c.updateParameters(response)
	c.endDialog(ctx, d)
	return nil
}

func (c *Client) updateParameters(response []segment) {
	for _, s := range findSegments(response, "HIBPA") {
		if version, err := strconv.Atoi(component(s, 0, 0)); err == nil {
			c.bpdVersion = version
		}
	}
	for _, s := range findSegments(response, "HIUPA") {
		if version, err := strconv.Atoi(component(s, 1, 0)); err == nil {
			c.updVersion = version
		}
	}
	versions := hkkazVersions(response)
	if len(versions) > 0 {
		c.hkkazVers = versions
	}
	c.updateTANParameters(response)
	c.updateCapabilities(response)
	c.parametersReady = true
}

func (c *Client) endDialog(ctx context.Context, d *dialog) {
	if d == nil || d.id == "" || d.id == "0" {
		return
	}
	defer func() { d.id = "0" }()
	if ctx.Err() != nil {
		return
	}
	// Cleanup must not turn a user cancellation or failed request into a long
	// wait, even when a custom HTTP client has no timeout.
	closeCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	_, _ = c.sendCommands(closeCtx, d, []string{
		fmt.Sprintf("HKEND:{n}:1+%s'", escape(d.id)),
	})
}

func (c *Client) sendCommands(ctx context.Context, d *dialog, commands []string) ([]segment, error) {
	return c.sendCommandsWithTAN(ctx, d, commands, "")
}

func (c *Client) sendCommandsWithTAN(ctx context.Context, d *dialog, commands []string, tan string) (result []segment, err error) {
	step := "build_message"
	var received []segment
	defer func() { err = failureWithResponse(err, step, "", commands, received) }()
	messageNumber := d.nextMessage
	if messageNumber == 0 {
		messageNumber = 1
	}
	requestDialogID := d.id
	defer func() { c.recordExchange(d, requestDialogID, messageNumber, commands, received, err) }()
	d.lastTANRequest, d.lastTANOrder = submittedTANReferences(commands)
	d.nextMessage = messageNumber + 1

	security := d.security
	if security == "" {
		security = defaultSecurity
	}
	raw, err := c.buildMessage(d.id, messageNumber, d.systemID, security, tan, commands)
	if err != nil {
		return nil, err
	}
	step = "transport"
	responseRaw, err := c.post(ctx, raw)
	if err != nil {
		return nil, err
	}
	step = "parse_response"
	response, err := parseSegments(responseRaw)
	if err != nil {
		return nil, failureWithResponse(errors.New("parse FinTS response: invalid protocol data"), "parse_response", "invalid_protocol_data", nil, nil)
	}
	received = response
	step = "bank_response"
	synchronizeDialogMessageNumber(d, response, messageNumber)
	if bankErr := firstBankError(response); bankErr != nil {
		typed, genericOnly := bankErr.(*BankError)
		tanInProgress := hasResponseCode(response, "0030", "3955", "3956")
		// Some endpoints wrap a valid SCA challenge in the global 9050
		// "partially erroneous" status. It is not a failed command when HITAN
		// and a TAN-in-progress response are present.
		if !genericOnly || typed.Code != "9050" || !tanInProgress {
			redacted := redactBankError(bankErr, c.config.PIN, tan)
			var diagnostic *BankError
			if errors.As(redacted, &diagnostic) {
				return nil, bankErrorWithRequestSegments(diagnostic, commands)
			}
			return nil, redacted
		}
	}
	return response, nil
}

// bankErrorWithRequestSegments retains only local command names as diagnostic
// metadata. Never derive these names from bank text or retain command fields.
func bankErrorWithRequestSegments(bankErr *BankError, commands []string) *BankError {
	copyErr := *bankErr
	copyErr.RequestSegments = nil
	for _, command := range commands {
		end := strings.IndexByte(command, ':')
		if end < 5 || end > 6 {
			continue
		}
		name := command[:end]
		valid := true
		for index := 0; index < len(name); index++ {
			if name[index] < 'A' || name[index] > 'Z' {
				valid = false
				break
			}
		}
		if !valid || containsString(copyErr.RequestSegments, name) {
			continue
		}
		copyErr.RequestSegments = append(copyErr.RequestSegments, name)
		if len(copyErr.RequestSegments) == 16 {
			break
		}
	}
	return &copyErr
}

func synchronizeDialogMessageNumber(d *dialog, response []segment, sent int) {
	returned := 0
	for _, s := range findSegments(response, "HNHBK") {
		if value, err := strconv.Atoi(component(s, 3, 0)); err == nil {
			returned = value
			break
		}
	}
	if returned == 0 {
		for _, s := range findSegments(response, "HNHBS") {
			if value, err := strconv.Atoi(component(s, 0, 0)); err == nil {
				returned = value
				break
			}
		}
	}
	if returned >= sent && returned+1 > d.nextMessage {
		d.nextMessage = returned + 1
	}
}

func (c *Client) buildMessage(dialogID string, messageNumber int, systemID, securityFunction, tan string, commands []string) ([]byte, error) {
	now := time.Now()
	securityReference, err := randomReference()
	if err != nil {
		return nil, err
	}

	profileVersion := 1
	if securityFunction != defaultSecurity {
		profileVersion = 2
	}
	inner := fmt.Sprintf(
		"HNSHK:2:4+PIN:%d+%s+%s+1+1+1::%s+1+1:%s:%s+1:999:1+6:10:16+%s:%s:%s:S:0:0'",
		profileVersion,
		escape(securityFunction),
		securityReference,
		escape(systemID),
		now.Format("20060102"),
		now.Format("150405"),
		escape(c.config.CountryCode),
		escape(c.config.BankCode),
		escape(c.config.UserID),
	)
	nextSegment := 3
	for _, command := range commands {
		inner += strings.ReplaceAll(command, "{n}", strconv.Itoa(nextSegment))
		nextSegment++
	}
	signature := escape(c.config.PIN)
	if tan != "" {
		signature += ":" + escape(tan)
	}
	inner += fmt.Sprintf("HNSHA:%d:2+%s++%s'", nextSegment, securityReference, signature)
	endSegment := nextSegment + 1

	hbciHeader := fmt.Sprintf(
		"HNHBK:1:3+000000000000+%s+%s+%d'",
		fintsVersion,
		escape(dialogID),
		messageNumber,
	)
	encryptionHeader := fmt.Sprintf(
		"HNVSK:998:3+PIN:%d+998+1+1::%s+1:%s:%s+2:2:13:%s:5:1+%s:%s:%s:V:0:0+0'",
		profileVersion,
		escape(systemID),
		now.Format("20060102"),
		now.Format("150405"),
		binary([]byte("00000000")),
		escape(c.config.CountryCode),
		escape(c.config.BankCode),
		escape(c.config.UserID),
	)
	encodedInner, err := encodeWireText([]byte(inner))
	if err != nil {
		return nil, err
	}
	encodedHeader, err := encodeWireText([]byte(hbciHeader + encryptionHeader))
	if err != nil {
		return nil, err
	}
	body := "HNVSD:999:1+" + binary(encodedInner) + "'"
	trailer := fmt.Sprintf("HNHBS:%d:1+%d'", endSegment, messageNumber)
	message := string(encodedHeader) + body + trailer
	size := fmt.Sprintf("%012d", len([]byte(message)))
	message = strings.Replace(message, "000000000000", size, 1)
	return []byte(message), nil
}

func (c *Client) post(ctx context.Context, message []byte) ([]byte, error) {
	encoded := base64.StdEncoding.EncodeToString(message)
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.config.Endpoint, strings.NewReader(encoded))
	if err != nil {
		return nil, fmt.Errorf("create FinTS request: %w", err)
	}
	request.Header.Set("Content-Type", "text/plain; charset=ISO-8859-1")
	request.Header.Set("Accept", "text/plain")
	request.Header.Set("User-Agent", "go-fints/"+c.config.ProductVersion)

	response, err := c.httpClient.Do(request)
	if err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("send FinTS request: request context ended: %w", err)
		}
		if errors.Is(err, context.DeadlineExceeded) && c.httpClient.Timeout > 0 {
			return nil, fmt.Errorf(
				"send FinTS request timed out after %s while awaiting the bank response; increase Config.Timeout: %w",
				c.httpClient.Timeout,
				err,
			)
		}
		return nil, fmt.Errorf("send FinTS request: %w", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBodyBytes+1))
	if err != nil {
		return nil, failureWithResponse(fmt.Errorf("read FinTS response: %w", err), "read_response", "response_read_error", nil, nil)
	}
	if len(body) > maxResponseBodyBytes {
		return nil, failureWithResponse(errors.New("FinTS response exceeds 16 MiB"), "read_response", "response_too_large", nil, nil)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, httpFailure(fmt.Errorf("FinTS endpoint returned HTTP %d", response.StatusCode), response.StatusCode)
	}

	body = []byte(strings.TrimSpace(string(body)))
	decoded := make([]byte, base64.StdEncoding.DecodedLen(len(body)))
	n, decodeErr := base64.StdEncoding.Decode(decoded, body)
	if decodeErr == nil {
		return decoded[:n], nil
	}
	// A few test/dummy servers return the raw FinTS wire message. Accepting it
	// makes the transport tolerant without weakening HTTPS requirements.
	if strings.HasPrefix(string(body), "HNHBK:") {
		return body, nil
	}
	return nil, failureWithResponse(fmt.Errorf("decode base64 FinTS response: %w", decodeErr), "decode_response", "invalid_base64", nil, nil)
}

func randomReference() (string, error) {
	var buffer [4]byte
	if _, err := rand.Read(buffer[:]); err != nil {
		return "", fmt.Errorf("generate security reference: %w", err)
	}
	value := uint32(buffer[0])<<24 | uint32(buffer[1])<<16 | uint32(buffer[2])<<8 | uint32(buffer[3])
	return fmt.Sprintf("%07d", value%10_000_000), nil
}

func responseDialogID(segments []segment) string {
	for _, s := range findSegments(segments, "HNHBK") {
		if id := component(s, 2, 0); id != "" {
			return id
		}
	}
	return ""
}

func firstBankError(segments []segment) error {
	var details []BankResponseError
	for _, kind := range []string{"HIRMG", "HIRMS"} {
		for _, s := range findSegments(segments, kind) {
			for i := range s.Fields {
				code := component(s, i, 0)
				if strings.HasPrefix(code, "9") {
					values := s.Fields[i].components
					params := []string(nil)
					if len(values) > 3 {
						params = append(params, values[3:]...)
					}
					details = append(details, BankResponseError{
						Code:             code,
						Message:          component(s, i, 2),
						Segment:          s.Header.Reference,
						ReferenceElement: component(s, i, 1),
						Params:           params,
					})
				}
			}
		}
	}
	if len(details) == 0 {
		return nil
	}
	primary := details[0]
	for _, detail := range details {
		// 9050 is a generic message-level wrapper. Prefer the segment-level
		// rejection that explains which value or operation failed.
		if detail.Code != "9050" {
			primary = detail
			break
		}
	}
	return &BankError{
		Code:    primary.Code,
		Message: primary.Message,
		Details: details,
	}
}

func tanRequired(segments []segment) error {
	code := ""
	for _, kind := range []string{"HIRMG", "HIRMS"} {
		for _, s := range findSegments(segments, kind) {
			for i := range s.Fields {
				candidate := component(s, i, 0)
				if candidate == "0030" || candidate == "3955" {
					code = candidate
				}
			}
		}
	}
	if code == "" {
		return nil
	}
	challenge := ""
	for _, s := range findSegments(segments, "HITAN") {
		for i := range s.Fields {
			value := component(s, i, 0)
			if len(value) > len(challenge) {
				challenge = value
			}
		}
	}
	return &TANRequiredError{Code: code, Challenge: challenge}
}

func accountsFromUPD(segments []segment) []Account {
	var accounts []Account
	for _, s := range findSegments(segments, "HIUPD") {
		account := Account{
			AccountNumber:       component(s, 0, 0),
			Subaccount:          component(s, 0, 1),
			CountryCode:         component(s, 0, 2),
			BankCode:            component(s, 0, 3),
			IBAN:                component(s, 1, 0),
			Type:                component(s, 3, 0),
			Currency:            component(s, 4, 0),
			OwnerName:           strings.TrimSpace(strings.Join([]string{component(s, 5, 0), component(s, 6, 0)}, " ")),
			ProductName:         component(s, 7, 0),
			SupportedOperations: make(map[string]bool),
		}
		for _, field := range s.Fields {
			value := valuesAt(field.components, 0)
			if (len(value) == 5 || len(value) == 6) && strings.HasPrefix(value, "HK") || value == "DKKKU" {
				account.SupportedOperations[value] = true
			}
		}
		if len(account.SupportedOperations) == 0 {
			account.SupportedOperations = nil
		}
		if account.AccountNumber != "" || account.IBAN != "" {
			accounts = append(accounts, account)
		}
	}
	return accounts
}

func accountsFromHISPA(segments []segment) []Account {
	var accounts []Account
	for _, s := range findSegments(segments, "HISPA") {
		for i := range s.Fields {
			if component(s, i, 0) != "J" {
				continue
			}
			account := Account{
				IBAN:          component(s, i, 1),
				BIC:           component(s, i, 2),
				AccountNumber: component(s, i, 3),
				Subaccount:    component(s, i, 4),
				CountryCode:   component(s, i, 5),
				BankCode:      component(s, i, 6),
			}
			if account.IBAN != "" || account.AccountNumber != "" {
				accounts = append(accounts, account)
			}
		}
	}
	return accounts
}

func mergeAccounts(base, extra []Account) []Account {
	result := append([]Account(nil), base...)
	for _, candidate := range extra {
		match := -1
		for i := range result {
			if candidate.IBAN != "" && result[i].IBAN == candidate.IBAN {
				match = i
				break
			}
			if candidate.AccountNumber != "" &&
				result[i].AccountNumber == candidate.AccountNumber &&
				(result[i].BankCode == "" || candidate.BankCode == "" || result[i].BankCode == candidate.BankCode) {
				match = i
				break
			}
		}
		if match < 0 {
			result = append(result, candidate)
			continue
		}
		mergeAccount(&result[match], candidate)
	}
	return result
}

func mergeAccount(destination *Account, source Account) {
	if destination.IBAN == "" {
		destination.IBAN = source.IBAN
	}
	if destination.BIC == "" {
		destination.BIC = source.BIC
	}
	if destination.AccountNumber == "" {
		destination.AccountNumber = source.AccountNumber
	}
	if destination.Subaccount == "" {
		destination.Subaccount = source.Subaccount
	}
	if destination.BankCode == "" {
		destination.BankCode = source.BankCode
	}
	if destination.CountryCode == "" {
		destination.CountryCode = source.CountryCode
	}
	if len(source.SupportedOperations) > 0 {
		if destination.SupportedOperations == nil {
			destination.SupportedOperations = make(map[string]bool)
		}
		for operation, supported := range source.SupportedOperations {
			destination.SupportedOperations[operation] = supported
		}
	}
}

func resolveAccount(account Account, advertised []Account) Account {
	for _, candidate := range advertised {
		candidate = normalizeAccount(candidate)
		matchesIBAN := account.IBAN != "" && candidate.IBAN == account.IBAN
		matchesNumber := account.AccountNumber != "" &&
			candidate.AccountNumber == account.AccountNumber &&
			(account.BankCode == "" || candidate.BankCode == "" || account.BankCode == candidate.BankCode)
		if !matchesIBAN && !matchesNumber {
			continue
		}
		if matchesIBAN && candidate.AccountNumber != "" {
			// The domestic tuple supplied in UPD is authoritative. Some banks
			// do not use the raw ten-digit BBAN account portion expected from
			// generic German IBAN decomposition.
			account.AccountNumber = candidate.AccountNumber
		}
		mergeAccount(&account, candidate)
		if account.Currency == "" {
			account.Currency = candidate.Currency
		}
		if account.Type == "" {
			account.Type = candidate.Type
		}
		if account.OwnerName == "" {
			account.OwnerName = candidate.OwnerName
		}
		if account.ProductName == "" {
			account.ProductName = candidate.ProductName
		}
		break
	}
	return account
}

func hkkazVersions(segments []segment) []int {
	seen := make(map[int]bool)
	var versions []int
	for _, s := range findSegments(segments, "HIKAZS") {
		if s.Header.Version >= 5 && s.Header.Version <= 7 && !seen[s.Header.Version] {
			seen[s.Header.Version] = true
			versions = append(versions, s.Header.Version)
		}
	}
	return versions
}

func selectHKKAZVersion(versions []int, account Account) int {
	selected := 0
	for _, version := range versions {
		if version < 5 || version > 7 {
			continue
		}
		if version == 7 && account.IBAN == "" {
			continue
		}
		if version < 7 && account.AccountNumber == "" {
			continue
		}
		if version > selected {
			selected = version
		}
	}
	if selected == 0 {
		for _, version := range versions {
			if version >= 5 && version <= 7 && version > selected {
				selected = version
			}
		}
	}
	if selected == 0 {
		if account.IBAN != "" && account.AccountNumber == "" {
			return 7
		}
		return 6
	}
	return selected
}

func buildHKKAZ(version int, account Account, from, to time.Time, touchdown string) string {
	countryCode := account.CountryCode
	if countryCode == "" {
		countryCode = "280"
	}
	var accountField string
	switch version {
	case 7:
		accountField = strings.Join([]string{
			escape(account.IBAN),
			escape(account.BIC),
			escape(account.AccountNumber),
			escape(account.Subaccount),
			escape(countryCode),
			escape(account.BankCode),
		}, ":")
	case 5:
		accountField = strings.Join([]string{
			escape(account.AccountNumber),
			escape(account.Subaccount),
			escape(countryCode),
			escape(account.BankCode),
		}, ":")
	default:
		version = 6
		accountField = strings.Join([]string{
			escape(account.AccountNumber),
			escape(account.Subaccount),
			escape(countryCode),
			escape(account.BankCode),
		}, ":")
	}
	dateFrom, dateTo := "", ""
	if !from.IsZero() {
		dateFrom = from.Format("20060102")
	}
	if !to.IsZero() {
		dateTo = to.Format("20060102")
	}
	return fmt.Sprintf("HKKAZ:{n}:%d+%s+N+%s+%s++%s'",
		version, accountField, dateFrom, dateTo, escape(touchdown))
}

func transactionData(segments []segment) []byte {
	var result []byte
	for _, s := range findSegments(segments, "HIKAZ") {
		if len(s.Fields) > 0 {
			result = append(result, s.Fields[0].raw...)
		}
	}
	return result
}

func touchdownPoint(segments []segment) string {
	for _, s := range findSegments(segments, "HIRMS") {
		for i := range s.Fields {
			if component(s, i, 0) == "3040" {
				return component(s, i, 3)
			}
		}
	}
	return ""
}

// State returns credential-free metadata after an operation. It waits for any
// running operation and must not be called from TANHandler or OnTANPoll.
func (c *Client) State() ClientState {
	c.mu.Lock()
	defer c.mu.Unlock()
	return ClientState{SystemID: c.systemID, SecurityFunction: c.config.SecurityFunction, TANMedium: c.config.TANMedium}
}

func redactBankError(err error, secrets ...string) error {
	var bankErr *BankError
	if !errors.As(err, &bankErr) {
		return err
	}
	redact := func(value string) string {
		for _, secret := range secrets {
			if secret != "" {
				value = strings.ReplaceAll(value, secret, "[redacted]")
			}
		}
		return value
	}
	// Bank response codes and segment positions are useful diagnostics. Free
	// text/parameters are untrusted and may echo credentials from the request.
	copyErr := *bankErr
	copyErr.Message = redact(copyErr.Message)
	copyErr.Details = append([]BankResponseError(nil), bankErr.Details...)
	for i := range copyErr.Details {
		detail := &copyErr.Details[i]
		detail.Message = redact(detail.Message)
		detail.ReferenceElement = redact(detail.ReferenceElement)
		detail.Params = append([]string(nil), detail.Params...)
		for j := range detail.Params {
			detail.Params[j] = redact(detail.Params[j])
		}
	}
	return &copyErr
}
