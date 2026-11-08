import test from "node:test";
import assert from "node:assert/strict";
import {
  confirmationLine,
  journey,
  lastSavedLabel,
  nextAction,
  openStage,
  securityState,
} from "./securityLevel.ts";

const none = {
  plusActive: false,
  backupActive: false,
  keyConfigured: false,
  auto: false,
  lastBackupAt: "",
  lastError: "",
  overdueDays: 0,
  logProblem: false,
};
const plus = { ...none, plusActive: true };
const running = {
  ...plus,
  backupActive: true,
  keyConfigured: true,
  auto: true,
  lastBackupAt: "2026-11-01T10:00:00Z",
};
const done = (state) => state.steps.map((step) => step.done);
const nextLabel = (input) => nextAction(journey({ bankConnected: true, ...input }))?.label;

test("Community stays on level 0 and leads to Plus", () => {
  const state = securityState(none);
  assert.equal(state.level, 0);
  assert.equal(nextLabel(none), "Plus buchen");
  assert.deepEqual(done(state), [false, false, false]);
});

test("Plus closes two steps and leads to the backup", () => {
  const state = securityState(plus);
  assert.equal(state.level, 2);
  assert.equal(nextLabel(plus), "Backup hinzubuchen");
  assert.deepEqual(done(state), [true, true, false]);
});

test("a running backup closes the last step", () => {
  const state = securityState(running);
  assert.equal(state.level, 3);
  assert.equal(nextLabel(running), undefined);
  assert.deepEqual(done(state), [true, true, true]);
});

test("a booked backup counts only once it works", () => {
  const notSetUp = securityState({ ...plus, backupActive: true });
  assert.equal(notSetUp.level, 2);
  assert.equal(nextLabel({ ...plus, backupActive: true }), "Sicherung einrichten");
  for (const broken of [
    { auto: false },
    { lastError: "x" },
    { overdueDays: 4 },
    { lastBackupAt: "" },
  ]) {
    const state = securityState({ ...running, ...broken });
    assert.equal(state.level, 2);
    assert.equal(nextLabel({ ...running, ...broken }), "Sicherung prüfen");
  }
});

test("a problem with the log costs the watched step, even with a backup", () => {
  for (const input of [
    { ...plus, logProblem: true },
    { ...running, logProblem: true },
  ]) {
    const state = securityState(input);
    assert.equal(state.level, 1);
    assert.equal(nextLabel(input), "Protokoll prüfen");
    assert.deepEqual(done(state), [true, false, false]);
  }
});

test("a backup without Plus proves nothing", () => {
  assert.equal(securityState({ ...running, plusActive: false }).level, 0);
});

const now = new Date(2026, 10, 1, 12);
const anchors = {
  plan: true,
  firstAnchor: new Date(2026, 9, 2, 8).toISOString(),
  lastAnchor: new Date(2026, 10, 1, 8).toISOString(),
  comparedAt: "x",
  conflicts: 0,
};

test("confirmationLine tells how long the log has been confirmed", () => {
  assert.equal(
    confirmationLine(anchors, now),
    "Seit 30 Tagen extern bestätigt, zuletzt heute, keine Abweichung.",
  );
  assert.equal(
    confirmationLine({ ...anchors, comparedAt: "" }, now),
    "Seit 30 Tagen extern bestätigt, zuletzt heute.",
  );
  assert.equal(
    confirmationLine({ ...anchors, conflicts: 2 }, now),
    "Seit 30 Tagen extern bestätigt, zuletzt heute, 2 Abweichungen.",
  );
  assert.equal(
    confirmationLine(
      { ...anchors, firstAnchor: anchors.lastAnchor, lastAnchor: anchors.lastAnchor },
      now,
    ),
    "Seit heute extern bestätigt, zuletzt heute, keine Abweichung.",
  );
});

test("confirmationLine without receipts names the way there", () => {
  const empty = { ...anchors, firstAnchor: "", lastAnchor: "" };
  assert.equal(confirmationLine(empty, now), "Wird bei der nächsten Verbindung erstellt");
  assert.equal(confirmationLine({ ...empty, plan: false }, now), "Gehört zu Plus");
});

const stageOf = (input, id) => journey(input).find((stage) => stage.id === id);

test("Plus raises time saved and protection in one station", () => {
  const kinds = stageOf({ ...plus, bankConnected: true }, "plus").items.map((item) => item.kind);
  assert.ok(kinds.includes("time") && kinds.includes("safe"));
  assert.ok(stageOf({ ...plus, bankConnected: true }, "plus").items.every((item) => item.done));
});

test("each station names what really is left to do", () => {
  const community = { ...none, bankConnected: false };
  assert.deepEqual(stageOf(community, "plus").action, { label: "Plus buchen", kind: "bookPlus" });
  assert.deepEqual(stageOf(community, "backup").action, {
    label: "Plus mit Backup buchen",
    kind: "bookBackup",
  });

  const noBank = { ...plus, bankConnected: false };
  assert.deepEqual(stageOf(noBank, "plus").action, { label: "Bank verbinden", kind: "banking" });
  assert.equal(stageOf(noBank, "plus").items[0].done, false);

  const plusOnly = { ...plus, bankConnected: true };
  assert.equal(stageOf(plusOnly, "plus").action, null);
  assert.deepEqual(stageOf(plusOnly, "backup").action, {
    label: "Backup hinzubuchen",
    kind: "website",
  });

  const booked = { ...plus, backupActive: true, bankConnected: true };
  assert.equal(stageOf(booked, "backup").reached, false);
  assert.deepEqual(stageOf(booked, "backup").action, {
    label: "Sicherung einrichten",
    kind: "setup",
  });
  assert.equal(
    stageOf({ ...running, lastError: "x", bankConnected: true }, "backup").action.label,
    "Sicherung prüfen",
  );
  assert.equal(
    stageOf({ ...running, logProblem: true, bankConnected: true }, "plus").action.kind,
    "audit",
  );
});

test("a finished way has nothing left to do", () => {
  const stages = journey({ ...running, bankConnected: true });
  assert.ok(stages.every((stage) => stage.reached && !stage.action));
  assert.ok(stages.flatMap((stage) => stage.items).every((item) => item.done));
  assert.equal(openStage(stages), "backup");
});

test("what protects comes before what saves time and what costs money", () => {
  assert.equal(nextLabel({ ...plus, bankConnected: false }), "Bank verbinden");
  assert.equal(
    nextLabel({ ...plus, backupActive: true, bankConnected: false }),
    "Sicherung einrichten",
  );
  assert.equal(openStage(journey({ ...plus, backupActive: true, bankConnected: false })), "backup");
});

test("the first station with a step opens by itself", () => {
  assert.equal(openStage(journey({ ...none, bankConnected: false })), "plus");
  assert.equal(openStage(journey({ ...plus, bankConnected: true })), "backup");
});

test("lastSavedLabel stays short", () => {
  const at = new Date(2026, 10, 1, 9, 5);
  assert.equal(
    lastSavedLabel(at.toISOString(), new Date(2026, 10, 1, 12)),
    "Gesichert heute, 09:05",
  );
  assert.equal(
    lastSavedLabel(at.toISOString(), new Date(2026, 10, 2, 12)),
    "Gesichert gestern, 09:05",
  );
  assert.equal(lastSavedLabel(at.toISOString(), new Date(2026, 10, 5, 12)), "Gesichert am 01.11.");
  assert.equal(lastSavedLabel("", new Date()), "");
});

test("Community Support fills the first bar, only the free version has none", () => {
  const support = securityState({ ...none, supportActive: true });
  assert.equal(support.level, 1);
  assert.equal(support.title, "Mit Support und Community");
  assert.ok(support.steps.every((step) => !step.done));
  const [first] = journey({ ...none, supportActive: true, bankConnected: false });
  assert.equal(first.title, "Community Support");
  assert.equal(securityState(none).level, 0);
  // Plus with a problem in the log stays on 1 with its warning.
  assert.notEqual(securityState({ ...plus, logProblem: true }).title, support.title);
});
