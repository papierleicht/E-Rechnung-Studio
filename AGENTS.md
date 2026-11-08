# Repository instructions

E-Rechnung-Studio is one desktop product (Go/Wails + Svelte). Follow these rules
when preparing commits or pull requests. The layout of the code is described in
the README under "Projektstruktur".

## Commits and pull requests

- Use Conventional Commits: `type(optional-scope): concise description`.
- Allowed types: `feat`, `fix`, `deps`, `perf`, `docs`, `style`, `refactor`, `test`,
  `build`, `ci`, `chore`, `revert`. Use lowercase scopes and titles <= 100 characters.
- Describe the actual user impact, not the implementation language. A visible UI
  bug fix is `fix(ui)`, not `style`. `style` is code formatting without behavior changes.
- `feat` introduces functionality; `fix` corrects behavior; `perf` improves runtime
  performance. These changes are release-relevant.
- Use `deps(wails): ...` for a dependency update intended for distribution (patch).
  Use `chore(deps): ...` for maintenance that does not need its own release.
  If the change implements a feature or fixes a specific bug, prefer `feat` or `fix`.
- Breaking changes use `!` and a `BREAKING CHANGE:` explanation of impact and migration
  in the commit body and PR description. This includes incompatible CLI/import/backup
  contracts or removal of a supported operating system. Internal refactoring or a
  framework major update alone is not necessarily breaking.
- Keep each commit/PR focused. Use squash merges, with the PR title as commit title
  and PR body as commit body, preserving the breaking-change explanation.
- Test a message with `node scripts/check-commit.mjs --file <message-file>`.
- Commit only the requested changes; preserve other working-tree changes.

## Versions and releases

- `version.txt` is the product version; application code embeds it. The GitHub release workflow
  maintains the manifest and synchronized package/installer metadata.
- Do not manually bump versions, create release tags, or publish releases as part
  of ordinary feature/fix work. Use the release workflow for an explicit release task.
- `fix`, `deps`, `perf`: patch. `feat`: minor. Before 1.0, breaking changes: minor;
  from 1.0 onward: major. Other nonbreaking types do not trigger their own release.
- The manual GitHub release workflow updates a release PR. After merging it,
  run the workflow again to create a `vX.Y.Z` tag and a draft GitHub release.
  Publish only after artifacts and platform acceptance are complete.
- Never rewrite a published version/tag. Fix mistakes in a new release.
- Keep Wails Go module, frontend runtime and Makefile CLI versions compatible.
- Do not edit vendored code under `third_party` for unrelated tasks.

## Verification

Run checks appropriate to the change. Before a release/CI change run:

```sh
node --test scripts/*.test.mjs
node scripts/check-version.mjs
npm --prefix frontend run check
npm --prefix frontend test
npm --prefix frontend run format:check
npm --prefix frontend run build
go test ./...
```

For UI changes inspect the affected layout, including the 900px minimum window.
For Wails changes also check macOS/Windows builds, window lifecycle, native file
pickers, PDF export and updater behavior; report platform checks not performed.
Never describe a local check as a successful GitHub Actions run.
