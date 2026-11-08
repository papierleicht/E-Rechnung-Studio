# Bundled Go dependencies

These directories preserve upstream source snapshots, licenses, notices and
module metadata. They are ordinary repository files, not Git submodules.
Runtime modules use local replacements so no sibling checkout is required.

| Directory | Upstream | Baseline commit | Use |
| --- | --- | --- | --- |
| go-xml | https://github.com/knroy/go-xml | 3b6e68522004277934566df82a1c065e68454d9f | Unmodified runtime snapshot |
| go-fints | https://github.com/BergerAPI/go-fints | 503f38cc3a8e406a165c093726df77c18d31d5fe | Locally modified runtime fork |

Runtime placeholder versions are resolved exclusively through local replacements;
the commits above identify their upstream baselines. Invoice parsing, calculation
and writing live in `internal/einvoice`, a documented adaptation of
speedata/einvoice (BSD-3-Clause) with its own XML traversal; its `LICENSE` and
`NOTICE.md` name the upstream commit. Do not remove upstream notices from any
snapshot or from the adapted package. The banking changes are documented in
`go-fints/PAPIERLEICHT.md` and the fork section of `go-fints/README.md`.

For an unmodified runtime snapshot, export the selected upstream commit with
`git archive` and replace its snapshot, preserving all upstream licenses and
notices. Reconcile upstream updates with local changes in the FinTS fork rather
than overwriting them. Upstream changes to speedata/einvoice are ported to
`internal/einvoice` by hand and tested. Update this table and docs/third-party-notices.md, run `go mod tidy`,
`go test ./...` and `wails3 build DEV=true`, then regenerate the offline license
catalog using frontend/scripts/license-catalog.mjs as described in the notices
document.
