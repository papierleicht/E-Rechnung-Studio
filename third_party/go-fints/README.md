# go-fints

[![CI](https://github.com/BergerAPI/go-fints/actions/workflows/ci.yml/badge.svg)](https://github.com/BergerAPI/go-fints/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/BergerAPI/go-fints.svg)](https://pkg.go.dev/github.com/BergerAPI/go-fints)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

`go-fints` is a compact, capability-negotiated FinTS 3.0 library for Go. It can:

- synchronize and open a PIN/TAN FinTS dialog over HTTPS
- discover two-step methods and named TAN media
- complete manual TAN challenges or poll decoupled BestSign approvals
- discover accounts from `HIUPD` and `HKSPA`
- fetch booked transactions with `HKKAZ` versions 5, 6, or 7
- adapt to banks that require their UPD currency as the HKKAZ 5/6
  subaccount marker
- fetch and parse CAMT transaction data with `HKCAZ` when MT940 is unavailable
- derive the domestic account tuple from a valid German IBAN for HKKAZ 5/6
- discover bank operations, versions, formats, limits, and TAN requirements
  from BPD/UPD
- submit negotiated `HKCCS` SEPA credit transfers using the bank's advertised
  `pain.001` format
- follow FinTS pagination/touchdown points
- parse MT940 into typed Go values
- build as an ordinary Go library, a tiny CLI, or a C-compatible shared
  library

It uses only the Go standard library.

## Important scope

This is not a complete FinTS stack. It implements
FinTS 3.0 PIN/TAN plus HKTAN versions 5–7, including manual process 2 and
decoupled process S. Bank behavior is selected from HIBPA/HIPINS/HIUPD and
operation parameter segments such as HIKAZS, HICAZS, and HICCSS.
Optical/chipTAN challenge rendering, key files, direct debits, and batch
payments are not implemented.

Real-bank compatibility varies because banks expose different segment versions
and authentication policies. Use a registered FinTS product ID; banks commonly
reject placeholder product IDs. The library rejects plain HTTP endpoints.
The endpoint and bank code remain explicit configuration because the official
FinTS bank list is licensed and cannot be redistributed freely.

Inspect what a bank actually advertised:

```go
capabilities, err := client.Capabilities(context.Background())
```

## Anonymous bank-parameter inquiry

`Probe` and the CLI `probe` command request public BPD without a customer login:

```sh
go run ./cmd/fints probe \
  --endpoint https://bank.example/fints \
  --bank-code 12345678 \
  --product-id "$FINTS_PRODUCT_ID" \
  --product-version 1.0
```

The Go API accepts a separate `ProbeConfig` with no PIN, user ID or account fields.
The CLI ignores login environment variables and rejects login flags. It uses an
unsigned anonymous FinTS 3.0 inquiry, attempts an anonymous dialog close, rejects
redirects, and bounds the entire operation with a timeout (CLI: 20 seconds;
Go API: 30 seconds by default).

`response_protocol` identifies the validated response protocol;
`advertised_protocols` lists declarations from BPD. An advertised `400` does not
mean this library implements FinTS 4.0. A successful probe establishes neither
authenticated access nor customer-specific permissions. Invalid envelopes,
bank errors, missing BPD and a mismatched bank code fail the probe.

Product identification is limited to 25 ISO-8859-1 characters and its version
to five, as required by HKVVB. Both are validated before network access.

## Bank-neutral core

The Go package contains no bank directory, bank constants, bank profiles, or
bank-code conditionals. It negotiates standard behavior from the endpoint.
Applications that know an endpoint has compatibility requirements can opt into
generic policy fields through `Config.Compatibility`:

```go
config.Compatibility = fints.CompatibilityOptions{
	SkipTransactionSEPAAccountDiscovery: true,
	TransactionAccountDefaults: fints.Account{
		BIC:        "BANKBIC",
		Currency:   "EUR",
		Subaccount: "EUR",
	},
	ForceTANForOperations:        []string{"HKKAZ", "HKCAZ"},
	PreferredSecurityFunctions:   []string{"123", "124"},
	TANMediumRequiredForFunctions: []string{"123", "124"},
	DecoupledPollProcess:          "S", // auto uses S for HKTAN 7; explicit endpoint override
}
```

These options describe protocol behavior rather than a named institution.
Bank-specific presets belong in applications. The bundled CLI demonstrates
this separation with its optional `--fyrst` profile; selecting no profile
leaves every compatibility option unset.

## Go library

```go
package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/BergerAPI/go-fints"
)

func main() {
	client, err := fints.NewClient(fints.Config{
		Endpoint:  "https://bank.example/fints",
		BankCode:  "12345678",
		UserID:    "my-login",
		PIN:       "my-pin",
		ProductID: "YOUR_REGISTERED_PRODUCT_ID",
		TANHandler: func(_ context.Context, challenge fints.TANChallenge) (string, error) {
			fmt.Print(challenge.Challenge, "\nTAN: ")
			var tan string
			fmt.Scanln(&tan)
			return tan, nil
		},
	})
	if err != nil {
		log.Fatal(err)
	}

	accounts, err := client.Accounts(context.Background())
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%+v\n", accounts)

	from, _ := time.Parse(time.DateOnly, "2026-01-01")
	to, _ := time.Parse(time.DateOnly, "2026-01-31")
	transactions, err := client.Transactions(
		context.Background(),
		fints.Account{IBAN: "DE89370400440532013000"},
		from,
		to,
	)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%+v\n", transactions)
}
```

`Transaction.Amount` is an integer in the currency's minor unit. For example,
`-1234` with currency `EUR` means `-12.34 EUR`.

For German IBANs, passing only the IBAN is sufficient even when a bank supports
only HKKAZ version 5 or 6. The library validates the checksum and derives the
eight-digit BLZ and ten-digit national account number. For other countries it
uses IBAN-native HKKAZ7 or HKCAZ when advertised.

Submit one SEPA credit transfer:

```go
result, err := client.SEPACreditTransfer(ctx, fints.SEPACreditTransfer{
	Debtor:       fints.Account{IBAN: "DE89370400440532013000"},
	DebtorName:   "Ada Example",
	CreditorName: "Grace Example",
	CreditorIBAN: "DE12500105170648489890",
	Amount:       1234, // 12.34 EUR
	Purpose:      "Invoice 42",
})
```

This immediately submits the payment. HKCCS availability and `pain.001`
formats are read from BPD; the configured TAN handler completes authorization.

A `Client` caches its synchronized system ID in memory and may be reused.
Supplying `Config.SystemID` skips synchronization. Persisting that ID is left
to the calling application.

## CLI

Build it:

```sh
make cli
```

List accounts:

```sh
export FINTS_ENDPOINT='https://bank.example/fints'
export FINTS_BANK_CODE='12345678'
export FINTS_USER_ID='my-login'
export FINTS_PIN='my-pin'
export FINTS_PRODUCT_ID='YOUR_REGISTERED_PRODUCT_ID'

./dist/fints accounts
```

For FYRST:

```sh
export FINTS_USER_ID='POSTBANK_ID'
export FINTS_PIN='PASSWORD'
export FINTS_PRODUCT_ID='YOUR_REGISTERED_PRODUCT_ID'

# Find the exact BestSign device name.
./dist/fints tan-media --fyrst

# The CLI displays the challenge and waits while you approve it in BestSign.
./dist/fints accounts --fyrst --tan-medium 'BestSign Device'
```

`--fyrst` is implemented entirely in `cmd/fints`: it supplies the endpoint,
bank code, account defaults, security-function preference, forced-SCA
operations, and polling compatibility to the neutral library configuration.

Fetch transactions:

```sh
./dist/fints transactions \
  --iban DE89370400440532013000 \
  --from 2026-01-01 \
  --to 2026-01-31
```

Inspect negotiated bank support:

```sh
./dist/fints capabilities --fyrst
```

Submit a transfer. The explicit `--execute` guard is mandatory:

```sh
./dist/fints transfer --fyrst \
  --debtor-iban DE89370400440532013000 \
  --debtor-name 'Ada Example' \
  --creditor-iban DE12500105170648489890 \
  --creditor-name 'Grace Example' \
  --amount 12.34 \
  --purpose 'Invoice 42' \
  --tan-medium 'BestSign Device' \
  --execute
```

All commands emit indented JSON. Run `./dist/fints help` for all environment
variables and flags. Prefer `FINTS_PIN` over `--pin` to keep the PIN out of
shell history. HTTP requests default to 90 seconds. Slow endpoints can use
`--timeout 2m` or `FINTS_TIMEOUT=2m`; the Go API exposes the same setting as
`Config.Timeout`, and the C ABI uses `FintsConfig.timeout_ms`.

## Shared library (`.so`)

Building in C shared mode produces both the dynamic library and its generated C
header:

```sh
make shared
ls dist/libfints.so dist/libfints.h
```

The C ABI is typed; it does not serialize requests.
`FintsClientHandle` is opaque, `FintsAccountArray` and
`FintsTransactionArray` contain real C arrays, and equivalent typed arrays are
provided for capabilities, TAN methods, and TAN media.

```c
#include "libfints.h"

FintsConfig config = {0};
config.endpoint = "https://bank.example/fints";
config.bank_code = "12345678";
config.user_id = "my-login";
config.pin = "my-pin";
config.product_id = "YOUR_REGISTERED_PRODUCT_ID";

FintsClientHandle client = 0;
FintsError *error = NULL;
int32_t status = FintsClientCreate(&config, &client, &error);
if (status != FINTS_OK) {
    fprintf(stderr, "%s\n", error->message);
    FintsErrorFree(error);
    return 1;
}

FintsAccountArray accounts = {0};
status = FintsAccounts(client, &accounts, &error);
if (status == FINTS_OK) {
    for (size_t i = 0; i < accounts.length; i++) {
        printf("%s\n", accounts.items[i].iban);
    }
    FintsAccountArrayFree(&accounts);
} else {
    fprintf(stderr, "%s\n", error->message);
    FintsErrorFree(error);
}
FintsClientDestroy(client);
```

Transaction retrieval uses a typed account and an owned result array:

```c
FintsAccount account = {0};
account.iban = "DE89370400440532013000";

FintsTransactionArray transactions = {0};
status = FintsTransactions(
    client,
    &account,
    "2026-01-01",
    "2026-01-31",
    &transactions,
    &error
);
if (status == FINTS_OK) {
    for (size_t i = 0; i < transactions.length; i++) {
        FintsTransaction *tx = &transactions.items[i];
        printf("%s %lld %s %s\n",
            tx->booking_date,
            (long long)tx->amount,
            tx->currency,
            tx->description);
    }
    FintsTransactionArrayFree(&transactions);
}
```

`FintsConfig.tan_callback` is a native callback for both manual TANs and
decoupled approvals:

```c
static char *on_tan(
    const FintsTANChallenge *challenge,
    void *user_data
) {
    (void)user_data;
    fprintf(stderr, "%s\n", challenge->challenge);
    if (challenge->decoupled) {
        return NULL; /* client continues polling */
    }
    return read_case_sensitive_tan(); /* copied before callback returns */
}
```

Ownership rules:

- all input structs, strings, and list pointers are borrowed for the duration
  of the call; `FintsClientCreate` copies configuration values;
- every successful array/result output is owned by the caller and must be
  released with its matching `...Free` function;
- every non-NULL `FintsError*` is owned by the caller and released with
  `FintsErrorFree`;
- callback challenge fields are borrowed and valid only during the callback;
  the returned TAN is borrowed and copied immediately;
- destroying a handle does not invalidate already returned result arrays.

`FintsABIVersion()` currently returns `1`. The generated header is the
authoritative ABI declaration. `FintsSEPACreditTransfer` submits a real payment
immediately; confirmation remains the embedding application's responsibility.

Go's `c-shared` build mode requires cgo and a working C toolchain. The output
is platform-specific; build it on the target operating system and
architecture.

## Tests

```sh
make test
```

The tests include:

- wire parsing for escaping and binary FinTS fields;
- MT940 amount, date, funds-code, reversal, and structured-purpose parsing;
- complete mocked synchronization and dialog flows;
- UPD/SEPA account merging;
- paginated `HKKAZ` transaction retrieval;
- IBAN normalization and German domestic-account derivation;
- BPD capability/version/format negotiation;
- HKCAZ CAMT binary extraction and CAMT.052 parsing;
- negotiated pain.001/HKCCS SEPA transfer construction;
- manual, case-sensitive TAN submission;
- configurable security-function preference, medium propagation, and
  decoupled polling;
- TAN-medium discovery;
- typed C array allocation and nested ownership cleanup;
- compiling, linking, and executing a C program against the generated header
  and shared library.

No live bank credentials are used by the test suite.

## Contributing and security

See [CONTRIBUTING.md](CONTRIBUTING.md) for development and protocol-test
guidelines. Report vulnerabilities according to [SECURITY.md](SECURITY.md).
Never attach raw signed FinTS messages or real banking data to a public issue.

## Security notes

- Never log raw FinTS requests: the signed `HNSHA` segment contains the PIN.
- Keep the endpoint on HTTPS and use the bank's documented URL.
- Treat account and transaction data as sensitive financial data.
- Treat `SEPACreditTransfer` as an irreversible external action and confirm all
  payment details before calling it.

## Acknowledgements

This project was heavily inspired by
[HBCI4Java](https://github.com/hbci4j/hbci4java), especially its mature
handling of PIN/TAN dialog initialization, TAN-media discovery, process-2 and
decoupled SCA flows, and bank interoperability edge cases. HBCI4Java is a
separate Java project licensed under LGPL-2.1; `go-fints` is an independent Go
implementation released under the MIT license.

## Protocol references

The implementation follows the FinTS 3.0 message definitions published by
[Financial Transaction Services](https://www.fints.org/de/spezifikation) and
the documented `HKKAZ`/`HIKAZ`, `HKCAZ`/`HICAZ`, and `HKCCS` flows.

## Papierleicht fork: additional read operations

This fork adds bank-advertised read operations. It is **not a complete FinTS
implementation**, nor a claim of feature parity or bank interoperability with
`python-fints` or `fints-rs`. Synthetic tests exercise protocol construction and
response handling; no live customer account has been used for this work.

| Method | Supported request segments | Result |
| --- | --- | --- |
| `Balance(ctx, account)` | HKSAL 5/6/7 | Booked and pending balances, available funds and optional credit/overdraft information |
| `StatementList(ctx, account)` | HKKAU 1/2 | Statement numbers, years, availability and receipt status |
| `Statement(ctx, account, request)` | HKEKA 3/4/5 | Original statement document bytes and associated metadata |
| `Holdings(ctx, account)` | HKWPD 5/6 | Original MT535 report bytes, **not normalized securities positions** |
| `StatusProtocol(ctx, from, to)` | HKPRO 3/4 | Historical order-processing responses, including historical rejections |
| `CommunicationEndpoints(ctx)` | HKKOM 4 | Advertised connection metadata, **never an automatic endpoint redirect** |
| `CreditCardTransactions(ctx, account, cardNumber, from, to)` | DKKKU 2 extension | Original bank-specific response fields, **not normalized booked transactions** |

```go
balance, err := client.Balance(ctx, account)
if err != nil {
    return err
}
// Exact ISO currency minor units, e.g. EUR cents, JPY yen, KWD fils.
fmt.Println(balance.Booked.Amount, balance.Booked.Currency)

statements, err := client.StatementList(ctx, account)
if err != nil {
    return err
}
if len(statements) > 0 && statements[0].Available && statements[0].Year != 0 {
    documents, err := client.Statement(ctx, account, fints.StatementRequest{
        Number: statements[0].Number,
        Year: statements[0].Year,
        Format: fints.StatementFormatPDF,
    })
    if err != nil {
        return err
    }
    // Store each document's original Data bytes securely. Never write bank
    // responses, receipt codes or downloaded documents into diagnostic logs.
    _ = documents
}
```

All added operations negotiate an explicitly implemented version from BPD.
Account-bound operations also honor operation permissions when provided by UPD.
Unavailable operations return an error; they do not silently submit an invented
version. Continuation pages are followed with a 100-page bound; repeated or cyclic
cursors fail instead of returning an incomplete result as successful. A missing
data response is distinguished from an explicit `3010` empty result.

Electronic statement downloads preserve receipt codes but **do not acknowledge
receipt automatically**. Receipt acknowledgement is not implemented. Do not label
a downloaded PDF a completed bank receipt workflow. `StatementFormatPDF` requests
the bank's printable format code; callers must inspect the returned bytes and
format rather than assuming every bank delivers PDF. No electronic-statement
availability is inferred from a successful transaction download.

### Transaction integrity

CAMT .052 reports and .053 statements are parsed using their XML structure. A
batch is split only when every detail provides an explicit amount in the entry
currency and the signed sum reconciles with the bank's total. Otherwise the
aggregate is retained without attributing the total to the first detail's payee.
This fallback is deliberate: consumers should show aggregate entries as such
instead of pretending all underlying payments were individually imported.

Transaction downloads compare explicit HICAZ account identifiers, each CAMT
report's account IBAN (or identifiable national account ID), and MT940 `:25:`
IBAN/national identifiers with the requested account. A contradiction on any
report or continuation page fails the entire result without partial transactions.
Legacy documents without account identification and unrecognizable proprietary
identifiers remain supported; they cannot provide the same identity check.

Booked transaction imports exclude CAMT entries explicitly marked pending or
informational. There is no separate normalized pending-transaction API. MT942 and
pending/booked lifecycle reconciliation remain unsupported. CAMT batch references
may be shared by multiple legitimate transactions; do not deduplicate solely on
an entry-level reference or amount/date/purpose hash.

Amounts use exact integer minor units. Nonzero precision beyond the currency's
minor digits, invalid signs, and integer overflow fail rather than rounding or
truncating. ISO currency exceptions with zero, three or four decimal digits are
handled. Unsupported nonmonetary units are rejected. FinTS MT940 binary payloads
are decoded as ISO-8859-1; CAMT XML retains its own XML encoding.

CAMT date windows honor the bank's advertised maximum when both date bounds are
provided. Unknown XML roots/namespaces, invalid credit/debit indicators, malformed
dates and incomplete responses fail rather than appearing to be a successful
empty synchronization. This parser is not an XML Schema validator and does not
promise every CAMT extension or every bank-specific interpretation.

### Provenance and remaining scope

Additional read operations were independently implemented against FinTS segment
field layouts and observable protocol behavior. The MIT `floffel/fints-rs` and
LGPL `raphaelm/python-fints` repositories were inspected for feature comparison
and protocol definitions. No LGPL implementation source was copied into this
MIT-licensed fork, and no Rust implementation was translated or copied. Existing
upstream license notices remain unchanged.

Not covered: DDV/RDH/RAH hardware/security profiles, optical chipTAN flicker-code
rendering, a complete securities/credit-card parser, standing-order/direct-debit
management, all payment products, electronic-statement receipt acknowledgement,
and a normalized pending-transaction lifecycle. Banks still need to be tested
individually with a registered product ID, customer authorization and the bank's
current authentication requirements.
