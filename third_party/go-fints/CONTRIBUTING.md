# Contributing

Contributions are welcome, especially sanitized interoperability fixes backed
by protocol-level tests.

## Development

Requirements:

- Go 1.22 or newer;
- `make`;
- a C compiler for the typed shared-library integration test.

Run the same checks used by CI:

```sh
find . -type f -name '*.go' -print0 | xargs -0 gofmt -w
go vet ./...
go test -race ./...
make test-c
```

Protocol changes should include a mock-bank regression test that asserts the
outgoing segment composition and the relevant response sequence. Keep the core
bank-neutral: institution-specific endpoints, defaults, and workarounds belong
in applications or explicit `CompatibilityOptions`.

## Sensitive information

Never commit or paste:

- PINs, TANs, customer IDs, system IDs, or TAN-medium identifiers;
- real IBANs, account numbers, names, transaction details, or balances;
- raw signed FinTS messages, because `HNSHA` contains the PIN;
- unredacted production-bank responses.

Use synthetic identifiers in tests and reduce interoperability reports to the
necessary segment names, versions, locations, and response codes.

## Pull requests

Keep changes focused and explain the negotiated FinTS behavior they affect.
All commits must be compatible with the MIT license. If implementation work was
informed by another project or specification, identify it in the pull request.
