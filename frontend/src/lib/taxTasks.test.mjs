import test from "node:test";
import assert from "node:assert/strict";
import { taxTasks } from "./taxTasks.ts";

const period = (id, label, to, due, extra = {}) => ({
  period: { id, label, to, due },
  overdue: false,
  started: true,
  ...extra,
});

const input = {
  year: 2026,
  vat: [
    period("2025-Q3", "3. Quartal 2025", "2025-09-30", "2025-10-10"), // before the app was used
    period("2025-Q4", "4. Quartal 2025", "2025-12-31", "2026-01-12", {
      filing: { submittedOn: "2026-01-08" },
    }),
    period("2026-Q2", "2. Quartal 2026", "2026-06-30", "2026-07-10", { overdue: true }),
    period("2026-Q3", "3. Quartal 2026", "2026-09-30", "2026-11-10"),
    period("2026-Q4", "4. Quartal 2026", "2026-12-31", "2027-02-10"),
    period("2026-Q1", "1. Quartal 2026", "2026-03-31", "2026-05-11", {
      filing: { submittedOn: "2026-05-02" },
      changed: true,
    }),
  ],
  zm: [period("2026-Q3", "3. Quartal 2026", "2026-09-30", "2026-10-26", { overdue: true })],
  annual: [
    { year: 2025, due: "2026-07-31" },
    { year: 2026, due: "2027-08-02" },
  ],
};

test("open filings come first, previous years only while still open", () => {
  const groups = taxTasks(input, "2026-10-28");
  assert.deepEqual(
    groups.now.map((t) => [t.id, t.state]),
    [
      ["vat:2026-Q1", "changed"],
      ["vat:2026-Q2", "overdue"],
      ["annual:2025", "overdue"],
      ["zm:2026-Q3", "overdue"],
      ["vat:2026-Q3", "due"],
    ],
  );
  assert.deepEqual(
    groups.soon.map((t) => t.id),
    ["vat:2026-Q4", "annual:2026"],
  );
  // The transmitted Q4/2025 belongs to last year and is not listed as done here.
  assert.deepEqual(groups.done, []);
});

test("periods before the first record in the app are left out", () => {
  const groups = taxTasks({ ...input, since: "2026-09-09" }, "2026-10-28");
  assert.deepEqual(
    groups.now.map((t) => t.id),
    ["vat:2026-Q1", "zm:2026-Q3", "vat:2026-Q3"],
  );
  assert.deepEqual(
    groups.soon.map((t) => t.id),
    ["vat:2026-Q4", "annual:2026"],
  );
});

test("the archive shows one year completely", () => {
  const groups = taxTasks(input, "2026-10-28", true);
  const ids = [...groups.now, ...groups.soon, ...groups.done].map((t) => t.id);
  assert.ok(ids.every((id) => id.includes("2026")));
  const last = taxTasks({ ...input, year: 2025 }, "2026-10-28", true);
  assert.deepEqual(
    last.done.map((t) => t.id),
    ["vat:2025-Q4"],
  );
  assert.ok(last.now.some((t) => t.id === "vat:2025-Q3"));
});
