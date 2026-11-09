import test from "node:test";
import assert from "node:assert/strict";
import { changeRange, needsBuild } from "./ci-changes.mjs";

test("skips the build for documentation the app does not ship", () => {
  for (const files of [
    ["README.md"],
    ["README.md", "docs/screenshots/uebersicht.png"],
    ["CHANGELOG.md", "SECURITY.md", "TRADEMARK.md"],
    [
      ".github/ISSUE_TEMPLATE/fehlerbericht.yml",
      ".github/PULL_REQUEST_TEMPLATE.md",
    ],
  ])
    assert.equal(needsBuild(files), false, files.join(", "));
});

test("builds for code, shipped documents, workflows and unknown changes", () => {
  for (const files of [
    [],
    ["README.md", "main.go"],
    ["frontend/src/App.svelte"],
    ["docs/third-party-notices.md"],
    ["docs/validation-bundle-sha256.json"],
    ["LICENSE"],
    [".github/workflows/ci.yml"],
    ["internal/einvoice/README.md"],
    ["docs/README.md"],
    ["sub/README.md"],
  ])
    assert.equal(needsBuild(files), true, files.join(", "));
});

const a = "a".repeat(40);
const b = "b".repeat(40);

test("uses the pull request's own changes and the pushed range", () => {
  assert.equal(
    changeRange({ pull_request: { base: { sha: a }, head: { sha: b } } }),
    `${a}...${b}`,
  );
  assert.equal(changeRange({ before: a, after: b }), `${a}..${b}`);
});

test("has no range for manual runs, new branches and force pushes", () => {
  assert.equal(changeRange({}), null);
  assert.equal(changeRange({ inputs: { tag: "" } }), null);
  assert.equal(changeRange({ before: "0".repeat(40), after: b }), null);
  assert.equal(changeRange({ before: a, after: b, forced: true }), null);
  assert.equal(
    changeRange({ pull_request: { base: {}, head: { sha: b } } }),
    null,
  );
});
