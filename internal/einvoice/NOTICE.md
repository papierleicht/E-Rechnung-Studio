# Origin and license

This package contains software derived from
[speedata/einvoice](https://github.com/speedata/einvoice), upstream commit
[`4042b0c17014239cb9da1e1042f5b9951dc1d49a`](https://github.com/speedata/einvoice/commit/4042b0c17014239cb9da1e1042f5b9951dc1d49a).
The starting point was the repository's existing vendored snapshot.

Copyright 2024 Patrick Gundlach, Berlin

The upstream code is licensed under the BSD 3-Clause License. [LICENSE](LICENSE)
contains the original copyright, conditions and disclaimer without modification.
Those terms continue to apply to the derived implementation; moving the code into
the application does not remove its third-party origin or notice obligations.

The model, profile identifiers, decimal calculations, CII/UBL field mappings and
writer/calculation regression tests are adapted from that snapshot. The local
package excludes the upstream CLI, PDF command implementation, generated code
lists and semantic validation engine. The PEPPOL file retains only the profile
identifier needed by output profile selection. Parser XML traversal is maintained
locally using the Go standard library instead of the upstream XPath stack. See
the source files and tests for the current implementation.

The small XML files in [`testdata`](testdata/README.md) are original project test
fixtures. Inline examples retained with the upstream tests remain part of the
BSD-derived test code. Separately licensed external fixture collections from the
upstream snapshot are not copied into this package.

This package does not include upstream PDF command code; the unrelated pdfcpu
notice from the upstream CLI does not describe this package. The application's
remaining PDF and other runtime dependencies retain their own distribution notices.
