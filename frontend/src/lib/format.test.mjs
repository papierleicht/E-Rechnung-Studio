import test from "node:test";
import assert from "node:assert/strict";
import { dateLabel, localToday } from "./format.ts";

test("today follows the local calendar day, not UTC", () => {
  // Shortly after midnight in Germany the UTC date is still the day before.
  const night = new Date(2026, 9, 28, 0, 30);
  assert.equal(localToday(night), "2026-10-28");
  assert.equal(dateLabel(localToday(night)), "28.10.2026");
});
