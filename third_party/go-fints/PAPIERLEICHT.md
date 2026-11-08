# Papierleicht source snapshot

Origin: https://github.com/BergerAPI/go-fints
Base commit: 503f38cc3a8e406a165c093726df77c18d31d5fe
License: MIT (see LICENSE)

This is a locally modified source snapshot, not an unmodified upstream release.
The parent application's go.mod resolves the module to this directory, so builds
need neither sibling checkouts nor an external runtime service.

Local additions include read-only balances/statements/holdings/card/status/endpoint
APIs, challenge images, session metadata access, progress callbacks, strict parser
and pagination handling, account-identity guards and financial precision fixes.
The fork-specific README section describes supported API versions and limitations.
The app exposes only account discovery and booked-transaction reads through its UI.

Provenance and remaining scope: see the fork section of README.md in this directory.

No Python LGPL source has been copied into this MIT source tree. No bank directory
or real bank credentials/captures are bundled. Preserve the original MIT notice.
