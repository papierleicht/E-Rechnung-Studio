import test from "node:test";
import assert from "node:assert/strict";
import { incomingTotals } from "./bankSummary.ts";

test("account turnover counts incoming bookings and keeps currencies separate", () => {
  const bookings = [
    { data: { amount: 1200, currency: "EUR" } },
    { data: { amount: -400, currency: "EUR" } },
    { data: { amount: 300, currency: "EUR" } },
    { data: { amount: 500, currency: "USD" } },
  ];
  assert.deepEqual(incomingTotals(bookings, "EUR"), [
    ["EUR", 1500],
    ["USD", 500],
  ]);
  assert.deepEqual(incomingTotals([], "EUR"), [["EUR", 0]]);
});
