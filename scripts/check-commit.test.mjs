import test from "node:test";
import assert from "node:assert/strict";
import { validateMessage } from "./check-commit.mjs";

test("accepts feature, fix, release and dependency commits", () => {
  for (const message of [
    "feat(invoices): add gross prices",
    "fix(ui): align fields",
    "deps(wails): update runtime",
    "chore(main): release 0.2.0",
    "ci: check pull requests",
  ])
    assert.doesNotThrow(() => validateMessage(message));
});
test("rejects unclassified, empty, unknown and overly long titles", () => {
  for (const message of [
    "Update Wails",
    "fix: ",
    "feature: new UI",
    `fix: ${"x".repeat(100)}`,
  ])
    assert.throws(() => validateMessage(message));
});
test("requires breaking-change explanation and accepts body-based breaking changes", () => {
  assert.throws(() => validateMessage("feat(backup)!: change format"));
  assert.doesNotThrow(() =>
    validateMessage(
      "feat(backup)!: change format\n\nBREAKING CHANGE: Old imports require conversion.",
    ),
  );
  assert.doesNotThrow(() =>
    validateMessage(
      "feat(backup): change format\n\nBREAKING CHANGE: Old imports require conversion.",
    ),
  );
});
