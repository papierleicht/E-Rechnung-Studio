# Third-party notices and validation data

E-Rechnung-Studio depends on pinned Go source snapshots in `third_party` and bundles a
subset of the ZUGFeRD/Factur-X rule package. Preserve the upstream notices when
packaging or redistributing the application.

## Bank and provider names

The application bundles no third-party logos. Banks, payment providers and
e-mail providers are identified by name only, shown with a neutral letter
tile (`frontend/src/components/Monogram.svelte`). Naming a provider to
indicate what the app connects to is covered by § 23 MarkenG; reproducing
its figurative mark is not necessary for that purpose (BGH I ZR 33/10). Names
remain the property of their owners and do not imply endorsement.

## GDPdU data description

`internal/gdpdu/gdpdu-01-09-2004.dtd` is the document type definition of the
Beschreibungsstandard for handing data to the German tax authorities. The
standard requires this file unchanged next to `index.xml`, so it is shipped as
published.

## papierleicht invoice core, derived from Speedata einvoice

- Application source: `internal/einvoice`
- Upstream baseline revision: `4042b0c17014239cb9da1e1042f5b9951dc1d49a`
- License: BSD 3-Clause
- Copyright: 2024 Patrick Gundlach, Berlin
- Notice sources: `internal/einvoice/LICENSE`, `internal/einvoice/NOTICE.md`

The model, calculations and XML field mappings/writers are a locally maintained
adaptation, not an independently authored replacement. The parser uses local
namespace-aware XML traversal. The original module and its general XPath stack
are no longer application dependencies. Its unused validation engine, commands
and tooling are not included in the internal package. The package's own test
fixtures are original; the upstream fixture collections are not distributed.

The offline license generator explicitly includes the internal package's notices
even though it no longer appears in `go list -m`.

## Payment QR encoder

`internal/paymentqr` provides the byte-mode, error-correction-M QR encoder used
for EPC payment images. `speedata/barcode` is still a transitive dependency of
`htmlbag`; its MIT notice remains in the PDF notices and offline license catalog.

## knroy/go-xml

- Local source: `third_party/go-xml`
- Pinned revision: `3b6e68522004277934566df82a1c065e68454d9f`
- License: MIT
- Copyright: 2026 Komol Nath Roy
- Notice source: `third_party/go-xml/LICENSE`

## Factur-X/ZUGFeRD validation material

Runtime files under `internal/validation/rules/facturx-*` come from the German
ZUGFeRD 2.5.2 / Factur-X 1.09.2 information package, published by FeRD on 4 August
2026. Source: [FeRD publications](https://www.ferd-net.de/downloads/veroeffentlichungen).

The BASIC, EN16931 and EXTENDED schema sets, compiled business-rule stylesheets
and code databases are preserved without editing. Unused duplicate code databases
at the profile root are omitted; the stylesheets load their adjacent copies.
File hashes are recorded in
`docs/validation-bundle-sha256.json`.

FeRD identifies Apache-2.0 as the license for the standard's technical artifacts
(schemas and Schematron) in its [published usage terms](https://www.ferd-net.de/ueber-uns/ressourcen-1/veroeffentlichungen/haftungsausschluss-und-nutzungsrechte).
The Apache-2.0 text is included in `docs/pdf-licenses/pdfjs-dist-Apache-2.0.txt`
and in the offline license catalog. The upstream package also contains the
unified specification (`Dokumentation/0_FACTUR-X 1.09.2 DE.pdf`); that specification
is not bundled here. No project license supersedes upstream terms.

## XRechnung validation material

`internal/validation/rules/xrechnung` contains the required CII 16B and UBL 2.1
Invoice/CreditNote XSDs plus CEN and XRechnung compiled Schematron from the local
official `xrechnung-3.0.2-validator-configuration-2026-08-31` bundle. `scenarios.xml`
is preserved and its standard-scenario severity overrides are applied by the app.
Original files remain unchanged. For complete document inputs only, the loader
adapts the unsupported `match="root()"` pattern to `match="/"` in memory.
The test-only Saxon runner uses the unmodified originals.

Upstream EN16931 and KoSIT license/notice files are copied to `docs/xml-licenses/`
and included in the macOS bundle. UBL and UN/CEFACT notices are retained in the
schema headers. Sources and test scope are recorded in `profile-test-matrix.md`.

## Go PDF renderer

PDF/A-3b rendering uses boxesandglue v0.2.101, htmlbag v0.0.102 (which includes
the former csshtml) and baseline-pdf v1.1.34. Their notices and those of the PDF
dependencies are copied to `docs/pdf-licenses/` and included in the macOS bundle.
Embedded fonts are TeX Gyre Heros (GUST Font License), Crimson Pro (SIL OFL),
and CamingoCode (CC BY-ND 3.0), supplied by htmlbag. Font copyright and license
metadata and license texts are included under `docs/pdf-licenses/`.

## PDF.js viewer

The document editor renders local PDFs with Mozilla PDF.js (`pdfjs-dist`
6.5.438, Apache-2.0). The legacy browser build and worker are bundled in the
application; no remote viewer, fonts or CDN are used. Its license is preserved
in `docs/pdf-licenses/pdfjs-dist-Apache-2.0.txt` and included in the macOS bundle.

## Lucide Svelte icons

UI icons use the official `@lucide/svelte` package, pinned to 1.70.0.
Lucide is ISC-licensed; Feather-derived icons additionally carry the MIT notice.
Both permit commercial redistribution, including proprietary applications,
provided the copyright and permission notices are retained. No license fee or
source-disclosure requirement is imposed by these licenses.

The complete upstream license is preserved in
`frontend/src/lib/lucide-LICENSE.txt`, included in the offline catalog and available
under Einstellungen → Über E-Rechnung-Studio → Lizenzen & Drittanbieterhinweise.
This ensures the notices ship with the embedded frontend on every platform.
When updating the dependency, update this license copy and regenerate the catalog.

## Offline license catalog

`frontend/src/lib/license-catalog.json` collects installed Go modules, the Go
standard library, installed npm packages (including separately labeled build
tools), and preserved PDF/font/XML notices. The catalog is bundled locally and
loaded only when opening the About dialog. Regenerate it using
`node frontend/scripts/license-catalog.mjs` (JSON on stdout) after dependency updates.
The generator uses the bundled source snapshots for Go replacements and does not substitute
their placeholder versions for a verified upstream release.

`is-reference` and `locate-character` publish MIT metadata but no standalone license
text, neither in the npm packages nor in their upstream repositories. Their entries
in `docs/ui-licenses/` therefore name the license from the metadata, record author and
source, and add the standard MIT text; they state that no original text by the author
exists. This inventory is not a legal clearance of the application or of the Factur-X
package discussed above.

## Where the notices ship

- **In every build:** the offline catalog above, under Einstellungen → Über
  E-Rechnung-Studio → Lizenzen & Drittanbieterhinweise. This covers Windows and Linux
  too, whose packages contain only the executable (the updater requires a single entry).
- **macOS bundle:** `Contents/Resources` holds this file and the license folders.
- **Linux `.deb`/`.rpm`/Arch packages (nfpm):** `/usr/share/doc/e-rechnung-studio/`.
- **Release asset `third-party-licenses.zip`:** `LICENSE`, this file,
  `third-party-licenses.txt` (the complete catalog as plain text, including all Go
  module and npm package texts, generated by `frontend/scripts/license-text.mjs`),
  and the folders `pdf-licenses`, `xml-licenses` and `ui-licenses`.

Sources checked on 2026-09-14:
- https://lucide.dev/license
- https://lucide.dev/guide/svelte/getting-started
- Installed package `node_modules/@lucide/svelte/LICENSE`

## FinTS banking and OS credential vault

- `third_party/go-fints` is a modified MIT source snapshot of
  `github.com/BergerAPI/go-fints`, baseline commit
  `503f38cc3a8e406a165c093726df77c18d31d5fe`. Its MIT notice is retained in
  `third_party/go-fints/LICENSE` and the offline license catalog.
- `github.com/zalando/go-keyring` v0.2.8 provides the native credential-store
  adapter. Its license and that of the Windows credential adapter are included
  in the offline catalog.
  The same adapter stores the SMTP password for e-mail sending; SMTP and MIME
  use only the Go standard library.
- Rust/Python reference repositories are not bundled runtime dependencies. They
  were inspected for comparison only; no code was translated or copied, see the
  provenance section of `third_party/go-fints/README.md`.

Implementation scope and practical limits of the fork:
`third_party/go-fints/PAPIERLEICHT.md` and the fork section of
`third_party/go-fints/README.md`.
