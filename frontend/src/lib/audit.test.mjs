import assert from "node:assert/strict";
import test from "node:test";
import { auditActionLabel, defaultAuditPeriod, filterRows, sortRows } from "./audit.ts";

test("names log actions and keeps unknown ones readable", () => {
  assert.equal(auditActionLabel("cancelled"), "Storniert");
  assert.equal(auditActionLabel("neu"), "neu");
});

test("suggests the last three full years", () => {
  assert.deepEqual(defaultAuditPeriod(new Date(2026, 9, 30)), {
    from: "2023-01-01",
    to: "2025-12-31",
  });
});

test("filters over all columns without regard to case", () => {
  const rows = [
    ["B-2026-0001", "Papierhandel"],
    ["B-2026-0002", "Tankstelle"],
  ];
  assert.deepEqual(filterRows(rows, "TANK"), [rows[1]]);
  assert.equal(filterRows(rows, " ").length, 2);
});

test("sorts numbers by value, dates by day and text by the alphabet", () => {
  const rows = [
    ["9,50", "02.01.2026", "Öl"],
    ["100,00", "31.12.2025", "Apfel"],
    ["20,00", "15.06.2026", "Zange"],
  ];
  assert.deepEqual(
    sortRows(rows, 0, true, true).map((r) => r[0]),
    ["9,50", "20,00", "100,00"],
  );
  assert.deepEqual(
    sortRows(rows, 1, false, false).map((r) => r[1]),
    ["15.06.2026", "02.01.2026", "31.12.2025"],
  );
  assert.deepEqual(
    sortRows(rows, 2, false, true).map((r) => r[2]),
    ["Apfel", "Öl", "Zange"],
  );
  assert.equal(rows[0][0], "9,50", "the original order stays untouched");
});
