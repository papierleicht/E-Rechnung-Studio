import assert from "node:assert/strict";
import test from "node:test";
import { periodRange } from "./financePeriod.ts";

test("period ranges cover whole months, quarters and years", () => {
  assert.deepEqual(periodRange("month", 2028, 1), { from: "2028-02-01", to: "2028-02-29" });
  assert.deepEqual(periodRange("month", 2026, 11), { from: "2026-12-01", to: "2026-12-31" });
  assert.deepEqual(periodRange("quarter", 2026, 3), { from: "2026-10-01", to: "2026-12-31" });
  assert.deepEqual(periodRange("quarter", 2026, 0), { from: "2026-01-01", to: "2026-03-31" });
  assert.deepEqual(periodRange("year", 2026, 0), { from: "2026-01-01", to: "2026-12-31" });
});
