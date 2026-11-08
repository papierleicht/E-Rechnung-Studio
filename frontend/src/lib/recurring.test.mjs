import test from "node:test";
import assert from "node:assert/strict";
import {
  billsInAdvance,
  recurringBadgeClass,
  recurringDraftsLabel,
  recurringIntervalLabel,
  recurringRunMessage,
  recurringStatusLabel,
} from "./recurring.ts";

test("recurring labels read as German", () => {
  assert.equal(recurringIntervalLabel("half-yearly"), "halbjährlich");
  assert.equal(recurringStatusLabel("active"), "Aktiv");
  assert.equal(recurringStatusLabel("paused"), "Pausiert");
  assert.equal(recurringStatusLabel("ended"), "Beendet");
  assert.equal(recurringBadgeClass("active"), "paid");
  assert.equal(recurringBadgeClass("ended"), "draft");
});

test("waiting drafts are counted in words", () => {
  assert.equal(recurringDraftsLabel(0), "");
  assert.equal(recurringDraftsLabel(1), "1 Entwurf wartet");
  assert.equal(recurringDraftsLabel(3), "3 Entwürfe warten");
  assert.match(recurringRunMessage(1), /Neuer Entwurf/);
  assert.match(recurringRunMessage(2), /2 neue Entwürfe/);
});

test("a period starting with or after the invoice counts as billed in advance", () => {
  assert.equal(billsInAdvance("2026-12-01", "2026-12-01"), true);
  assert.equal(billsInAdvance("2026-12-01", "2026-11-01"), false);
  assert.equal(billsInAdvance("", "2026-11-01"), false);
});
