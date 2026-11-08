package fints

import (
	"context"
	"net/http"
	"time"
)

// Config contains the connection and login data for a FinTS 3.0 endpoint.
//
// ProductID must be a product identifier registered with the German Banking
// Industry Committee. The PIN is only sent inside the FinTS PIN/TAN envelope
// over HTTPS and is never included in errors.
type Config struct {
	Endpoint         string     `json:"endpoint"`
	CountryCode      string     `json:"country_code,omitempty"`
	BankCode         string     `json:"bank_code"`
	UserID           string     `json:"user_id"`
	CustomerID       string     `json:"customer_id,omitempty"`
	PIN              string     `json:"pin"`
	ProductID        string     `json:"product_id"`
	ProductVersion   string     `json:"product_version,omitempty"`
	SystemID         string     `json:"system_id,omitempty"`
	SecurityFunction string     `json:"security_function,omitempty"`
	TANMedium        string     `json:"tan_medium,omitempty"`
	HKTANVersion     int        `json:"hktan_version,omitempty"`
	TANHandler       TANHandler `json:"-"`
	// OnTANPoll runs before each decoupled status request. Return an error to
	// cancel polling. Callbacks must not call other methods on this Client.
	OnTANPoll    func(context.Context, TANChallenge) error `json:"-"`
	AutoPoll     bool                                      `json:"auto_poll,omitempty"`
	PollInterval time.Duration                             `json:"-"`
	MaxPolls     int                                       `json:"max_polls,omitempty"`
	Timeout      time.Duration                             `json:"-"`
	HTTPClient   *http.Client                              `json:"-"`
	AllowHTTP    bool                                      `json:"-"`
	// Compatibility contains opt-in protocol behavior for endpoints whose
	// implementation differs from their advertised BPD/UPD. The core library
	// never selects these options from a bank code.
	Compatibility CompatibilityOptions `json:"compatibility,omitempty"`
}

// CompatibilityOptions describes endpoint behavior without naming or
// hard-coding a bank. Applications can define reusable bank profiles outside
// the library and pass their choices here.
type CompatibilityOptions struct {
	SkipTransactionSEPAAccountDiscovery bool     `json:"skip_transaction_sepa_account_discovery,omitempty"`
	TransactionAccountDefaults          Account  `json:"transaction_account_defaults,omitempty"`
	ForceTANForOperations               []string `json:"force_tan_for_operations,omitempty"`
	PreferredSecurityFunctions          []string `json:"preferred_security_functions,omitempty"`
	TANMediumRequiredForFunctions       []string `json:"tan_medium_required_for_security_functions,omitempty"`
	// DecoupledPollProcess may be empty/"auto", "2", or "S". Empty/"auto"
	// uses S for HKTAN version 7 and process 2 for older segment versions.
	DecoupledPollProcess string `json:"decoupled_poll_process,omitempty"`
}

// TANHandler is called when the bank requests a second factor. For a manual
// challenge it must return the case-sensitive TAN. For a decoupled challenge,
// it should display the challenge and return an empty string; the client then
// polls for approval. The handler must respect context cancellation and must
// not call other methods on this Client (operations are serialized).
type TANHandler func(context.Context, TANChallenge) (string, error)

// TANChallenge describes an HKTAN challenge returned by the bank.
type TANChallenge struct {
	Method        string `json:"method"`
	Medium        string `json:"medium,omitempty"`
	Process       string `json:"process"`
	HKTANVersion  int    `json:"hktan_version,omitempty"`
	TaskReference string `json:"task_reference"`
	Challenge     string `json:"challenge,omitempty"`
	Decoupled     bool   `json:"decoupled"`
	PollAttempt   int    `json:"poll_attempt,omitempty"`
	// ChallengeHHD preserves the binary HHD_UC payload for optical TAN devices.
	// Image and ImageMIMEType are populated for supported raster matrix codes.
	// Render Challenge as text, never as trusted HTML from the bank.
	ChallengeHHD  []byte `json:"challenge_hhd,omitempty"`
	ImageMIMEType string `json:"image_mime_type,omitempty"`
	Image         []byte `json:"image,omitempty"`
}

// TANMethod is a two-step mechanism advertised in HITANS.
type TANMethod struct {
	SecurityFunction string `json:"security_function"`
	Name             string `json:"name"`
	HKTANVersion     int    `json:"hktan_version"`
	TANProcess       string `json:"tan_process"`
	MediumRequired   bool   `json:"medium_required"`
	Decoupled        bool   `json:"decoupled"`
}

// TANMedium is an active TAN device or app advertised in HITAB.
type TANMedium struct {
	Name             string `json:"name"`
	Class            string `json:"class,omitempty"`
	Status           string `json:"status,omitempty"`
	SecurityFunction string `json:"security_function,omitempty"`
	MobileNumber     string `json:"mobile_number,omitempty"`
}

// Account is an account advertised in the user's UPD or by HKSPA.
type Account struct {
	IBAN          string `json:"iban,omitempty"`
	BIC           string `json:"bic,omitempty"`
	AccountNumber string `json:"account_number,omitempty"`
	Subaccount    string `json:"subaccount,omitempty"`
	CountryCode   string `json:"country_code,omitempty"`
	BankCode      string `json:"bank_code,omitempty"`
	Currency      string `json:"currency,omitempty"`
	Type          string `json:"type,omitempty"`
	OwnerName     string `json:"owner_name,omitempty"`
	ProductName   string `json:"product_name,omitempty"`
	// SupportedOperations contains the FinTS segment names advertised for
	// this account in UPD, for example HKKAZ or HKCCS.
	SupportedOperations map[string]bool `json:"supported_operations,omitempty"`
}

// Transaction is one booked MT940 transaction. Amount is expressed in the
// currency's minor unit (for EUR, cents); debits are negative.
type Transaction struct {
	BookingDate        string `json:"booking_date"`
	ValueDate          string `json:"value_date,omitempty"`
	Amount             int64  `json:"amount"`
	Currency           string `json:"currency"`
	Type               string `json:"type,omitempty"`
	CustomerReference  string `json:"customer_reference,omitempty"`
	BankReference      string `json:"bank_reference,omitempty"`
	CounterpartyName   string `json:"counterparty_name,omitempty"`
	CounterpartyIBAN   string `json:"counterparty_iban,omitempty"`
	CounterpartyBankID string `json:"counterparty_bank_id,omitempty"`
	Description        string `json:"description,omitempty"`
}

// Capability describes one operation negotiated from the bank's BPD.
type Capability struct {
	Operation   string   `json:"operation"`
	Versions    []int    `json:"versions"`
	Formats     []string `json:"formats,omitempty"`
	TANRequired bool     `json:"tan_required"`
	MaxDays     int      `json:"max_days,omitempty"`
}

// SEPACreditTransfer describes one SEPA credit transfer. Amount is expressed
// in the currency's minor unit (EUR cents).
type SEPACreditTransfer struct {
	Debtor            Account   `json:"debtor"`
	DebtorName        string    `json:"debtor_name"`
	CreditorName      string    `json:"creditor_name"`
	CreditorIBAN      string    `json:"creditor_iban"`
	CreditorBIC       string    `json:"creditor_bic,omitempty"`
	Amount            int64     `json:"amount"`
	Currency          string    `json:"currency,omitempty"`
	Purpose           string    `json:"purpose,omitempty"`
	EndToEndReference string    `json:"end_to_end_reference,omitempty"`
	ExecutionDate     time.Time `json:"execution_date,omitempty"`
}

// TransferResult is returned after the bank accepted a SEPA transfer.
type TransferResult struct {
	Accepted       bool              `json:"accepted"`
	Operation      string            `json:"operation"`
	PainDescriptor string            `json:"pain_descriptor"`
	Responses      []ResponseMessage `json:"responses,omitempty"`
}

// ResponseMessage is a decoded HIRMG or HIRMS response.
type ResponseMessage struct {
	Code             string   `json:"code"`
	Message          string   `json:"message"`
	Segment          int      `json:"segment,omitempty"`
	ReferenceElement string   `json:"reference_element,omitempty"`
	Params           []string `json:"params,omitempty"`
}

// API describes the public library surface.
type API interface {
	Accounts(context.Context) ([]Account, error)
	Transactions(context.Context, Account, time.Time, time.Time) ([]Transaction, error)
	Capabilities(context.Context) ([]Capability, error)
	SEPACreditTransfer(context.Context, SEPACreditTransfer) (*TransferResult, error)
	TANMethods(context.Context) ([]TANMethod, error)
	TANMedia(context.Context) ([]TANMedium, error)
	SetTANMedium(string)
	SetTANMethod(string) error
	State() ClientState
	Balance(context.Context, Account) (*AccountBalance, error)
	StatementList(context.Context, Account) ([]StatementInfo, error)
	Statement(context.Context, Account, StatementRequest) ([]ElectronicStatement, error)
	Holdings(context.Context, Account) ([]HoldingsReport, error)
	CreditCardTransactions(context.Context, Account, string, time.Time, time.Time) ([]CreditCardResponse, error)
	StatusProtocol(context.Context, time.Time, time.Time) ([]StatusEntry, error)
	CommunicationEndpoints(context.Context) ([]BankCommunication, error)
}

// ClientState contains reusable connection metadata, never PINs, TANs, or active
// dialogs. It still contains personal identifiers and must be stored locally
// with the same access protection as account metadata. Restore its fields into
// Config only for the same endpoint, bank, and user; bank parameters are fetched
// afresh on the next connection. It does not resume a pending TAN challenge.
type ClientState struct {
	SystemID         string `json:"system_id"`
	SecurityFunction string `json:"security_function"`
	TANMedium        string `json:"tan_medium,omitempty"`
}
