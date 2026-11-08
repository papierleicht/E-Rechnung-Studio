import test from "node:test";
import assert from "node:assert/strict";
import { resolveSettingsNavigation } from "./settingsNavigation.ts";

test("save before navigating waits for persistence", async () => {
  const calls = [];
  let complete;
  const saving = new Promise((resolve) => {
    complete = resolve;
  });
  const result = resolveSettingsNavigation("save", {
    save: () => {
      calls.push("save");
      return saving;
    },
    discard: () => calls.push("discard"),
    navigate: () => calls.push("navigate"),
  });
  assert.deepEqual(calls, ["save"]);
  complete(true);
  assert.equal(await result, true);
  assert.deepEqual(calls, ["save", "navigate"]);
});

test("failed save preserves edits and keeps settings open; retry can succeed", async () => {
  const calls = [];
  let success = false;
  const actions = {
    save: async () => success,
    discard: () => calls.push("discard"),
    navigate: () => calls.push("navigate"),
  };
  assert.equal(await resolveSettingsNavigation("save", actions), false);
  assert.deepEqual(calls, []);
  success = true;
  assert.equal(await resolveSettingsNavigation("save", actions), true);
  assert.deepEqual(calls, ["navigate"]);
});

test("cancel retains edits; discard restores settings before navigation", async () => {
  const calls = [];
  const actions = {
    save: async () => {
      calls.push("save");
      return true;
    },
    discard: () => calls.push("discard"),
    navigate: () => calls.push("navigate"),
  };
  assert.equal(await resolveSettingsNavigation("cancel", actions), false);
  assert.deepEqual(calls, []);
  assert.equal(await resolveSettingsNavigation("discard", actions), true);
  assert.deepEqual(calls, ["discard", "navigate"]);
});
