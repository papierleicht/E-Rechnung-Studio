# Invoice parser fixtures

These small CII and UBL documents were authored for the project's internal
invoice module. They contain fictional identifiers and parties and exercise
syntax mapping, exact decimal values, namespace handling, delivery addresses,
and credit note payment dates. They are not conformance certificates or copies
of external standardization bodies' example invoices.

The inline regression cases in `parser_cii_test.go` and `parser_ubl_test.go`
were adapted from speedata/einvoice's BSD-3-Clause test code; see the package's
`LICENSE` and `NOTICE.md`.

The upstream fixture collections, which are licensed separately, are not part of
this repository.
