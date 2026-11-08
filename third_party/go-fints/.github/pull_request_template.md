## Summary

Describe the change and the FinTS behavior it affects.

## Verification

- [ ] `gofmt` produces no changes
- [ ] `go vet ./...` passes
- [ ] `go test -race ./...` passes
- [ ] `make test-c` passes when the C ABI is affected
- [ ] Protocol changes include sanitized regression tests
- [ ] No credentials, account data, or raw signed FinTS messages are included

## Compatibility

List the negotiated segments/versions involved and explain whether the change
is bank-neutral or an application-level compatibility option.
