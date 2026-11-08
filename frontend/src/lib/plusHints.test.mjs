import test from "node:test";
import assert from "node:assert/strict";
import {
  hintPlan as plan,
  hintPlanFromProductSource as planFromProductSource,
  loadHintPlan as loadPlan,
  resetHintPlan as resetPlan,
  setHintPlan as setPlan,
  unknownHintPlan as unknownPlan,
  withAccount,
} from "./hintPlan.ts";
import {
  DISMISSED_STORAGE_KEY,
  PLUS_INFO_URL,
  dismissPlusHint,
  dismissedPlusHints,
  plusHintRelevant,
  plusHintVisible,
} from "./plusHints.ts";

function memoryStorage(initial = {}) {
  const data = { ...initial };
  return {
    data,
    getItem: (key) => (key in data ? data[key] : null),
    setItem: (key, value) => {
      data[key] = String(value);
    },
  };
}

test("plan status follows the FinTS product source for now", () => {
  assert.deepEqual(planFromProductSource("subscription"), {
    known: true,
    plusActive: true,
    backupActive: false,
    productSource: "subscription",
  });
  for (const source of ["", "own", "build"]) {
    const status = planFromProductSource(source);
    assert.equal(status.plusActive, false, source);
    assert.equal(status.backupActive, false, source);
  }
});

test("hints stay hidden while the plan is unknown", () => {
  for (const topic of ["fints", "plus", "backup"])
    assert.equal(plusHintRelevant(topic, unknownPlan), false, topic);
});

test("FinTS hints only appear without any product ID", () => {
  assert.equal(plusHintRelevant("fints", planFromProductSource("")), true);
  assert.equal(plusHintRelevant("fints", planFromProductSource("own")), false);
  assert.equal(plusHintRelevant("fints", planFromProductSource("build")), false);
  assert.equal(plusHintRelevant("fints", planFromProductSource("subscription")), false);
});

test("general Plus hints hide with Plus, but not with an own registration", () => {
  assert.equal(plusHintRelevant("plus", planFromProductSource("")), true);
  assert.equal(plusHintRelevant("plus", planFromProductSource("own")), true);
  assert.equal(plusHintRelevant("plus", planFromProductSource("subscription")), false);
});

test("backup hints depend only on the backup plan", () => {
  const plus = planFromProductSource("subscription");
  assert.equal(plusHintRelevant("backup", plus), true);
  assert.equal(plusHintRelevant("backup", { ...plus, backupActive: true }), false);
});

test("a dismissed hint stays hidden and is persisted per hint", () => {
  const storage = memoryStorage();
  const status = planFromProductSource("");
  assert.equal(plusHintVisible("t-csv", "fints", status, dismissedPlusHints(storage)), true);
  const dismissed = dismissPlusHint("t-csv", storage);
  assert.equal(plusHintVisible("t-csv", "fints", status, dismissed), false);
  // Reopening reads the stored choice; other hints are unaffected.
  const reopened = dismissedPlusHints(storage);
  assert.equal(reopened.has("t-csv"), true);
  assert.equal(plusHintVisible("t-other", "fints", status, reopened), true);
  assert.ok(JSON.parse(storage.data[DISMISSED_STORAGE_KEY]).includes("t-csv"));
});

test("broken or blocked storage never throws", () => {
  const broken = memoryStorage({ [DISMISSED_STORAGE_KEY]: "{not json" });
  assert.equal(dismissedPlusHints(broken).has("t-x"), false);
  const blocked = {
    getItem: () => {
      throw new Error("blocked");
    },
    setItem: () => {
      throw new Error("blocked");
    },
  };
  assert.doesNotThrow(() => dismissedPlusHints(blocked));
  // The choice still holds for this session.
  assert.equal(dismissPlusHint("t-blocked", blocked).has("t-blocked"), true);
  assert.equal(dismissedPlusHints(null).has("t-blocked"), true);
});

test("plan store loads once and notifies subscribers", async () => {
  resetPlan();
  const seen = [];
  const stop = plan.subscribe((value) => seen.push(value.plusActive));
  let calls = 0;
  const load = async () => {
    calls++;
    return { productSource: "subscription" };
  };
  await Promise.all([loadPlan(load), loadPlan(load)]);
  assert.equal(calls, 1);
  assert.deepEqual(seen, [false, true]);
  setPlan(planFromProductSource("own"));
  assert.deepEqual(seen, [false, true, false]);
  stop();
  resetPlan();
});

test("a failed plan load can be retried", async () => {
  resetPlan();
  await loadPlan(async () => {
    throw new Error("offline");
  });
  let current;
  const stop = plan.subscribe((value) => (current = value));
  assert.equal(current.known, false);
  await loadPlan(async () => ({ productSource: "" }));
  assert.equal(current.known, true);
  stop();
  resetPlan();
});

test("Plus information has one central URL", () => {
  assert.equal(PLUS_INFO_URL, "https://papierleicht.de/#preise");
});

test("the verified subscription silences hints even without a FinTS ID", () => {
  const signedOut = { loaded: true, plusActive: false, backupActive: false };
  const plus = { loaded: true, plusActive: true, backupActive: false };
  const backup = { loaded: true, plusActive: true, backupActive: true };
  const bank = planFromProductSource("");
  assert.equal(plusHintRelevant("fints", withAccount(bank, signedOut)), true);
  assert.equal(plusHintRelevant("fints", withAccount(bank, plus)), false);
  assert.equal(plusHintRelevant("backup", withAccount(bank, plus)), true);
  assert.equal(plusHintRelevant("backup", withAccount(bank, backup)), false);
  // Hints wait until the account status is known.
  assert.equal(
    plusHintRelevant("backup", withAccount(bank, { ...signedOut, loaded: false })),
    false,
  );
});
