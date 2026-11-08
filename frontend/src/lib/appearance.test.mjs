import assert from "node:assert/strict";
import test from "node:test";
import { normalizeAppearance, resolveAppearance } from "./appearance.ts";

test("appearance defaults to the system setting", () => {
  assert.equal(normalizeAppearance(), "system");
  assert.equal(normalizeAppearance("unknown"), "system");
  assert.equal(resolveAppearance("system", false), "light");
  assert.equal(resolveAppearance("system", true), "dark");
});

test("manual appearance overrides the system setting", () => {
  assert.equal(resolveAppearance("light", true), "light");
  assert.equal(resolveAppearance("dark", false), "dark");
});
