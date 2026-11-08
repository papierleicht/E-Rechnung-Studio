# Invoice core

`papierleicht.de/e-rechnung-studio/internal/einvoice` is papierleicht's maintained
invoice model, decimal calculations and CII/UBL XML import/export implementation.
It is derived
from `github.com/speedata/einvoice`, not an independent implementation. The exact
origin and redistribution terms are in [NOTICE.md](NOTICE.md) and [LICENSE](LICENSE).

The package provides the subset used by the application:

- CII, UBL Invoice and UBL CreditNote parsing with namespace-aware XML traversal;
- the invoice, party, line, payment and tax models shared by import/export;
- commercial decimal rounding, VAT breakdowns, allowances/charges and totals;
- profile-aware CII and UBL serialization, including Factur-X and XRechnung fields.

This package does **not** provide the upstream `Validate` API, the upstream CLI,
PDF tools, generated code lists or its independent business-rule engine. Successful
parsing or writing does not establish invoice compliance. The application's
`internal/validation` package checks the bundled XSD and Schematron rules and
performs supplemental checks. That validation remains necessary.

For a newly constructed invoice, set the specification identifier and all required
business data, call `UpdateApplicableTradeTax`, then `UpdateTotals`, and serialize
with `Write`. `SchemaTypeUnknown` retains the CII default. Unknown profiles and
Minimum/BasicWL preserve supplied totals rather than recomputing line-based totals.
Decimal arithmetic remains provided by `github.com/shopspring/decimal`; XML
serialization remains provided by `github.com/beevik/etree`.

Upstream calculation and writer regression tests are retained, including their
assertions for round trips, omitted tax rates, currency totals and rounding. Local
contract tests cover output failures, default/unsupported schemas, escaped text
and grouping lines. Run the package tests from the repository root:

```sh
go test ./internal/einvoice
```

Changes to the model or XML mappings need corresponding parser/writer tests and the
application's generation, import and validation checks. Keep the license and
provenance with further adaptations and with binary redistribution notices.
