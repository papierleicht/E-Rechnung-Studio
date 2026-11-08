import test from "node:test";
import assert from "node:assert/strict";
import { priceOf } from "./planPrices.ts";

const prices = [
  { plan: "support", interval: "monthly", currency: "EUR", value: "4.90" },
  { plan: "support", interval: "yearly", currency: "EUR", value: "49.00" },
  { plan: "plus", interval: "monthly", currency: "EUR", value: "abc" },
];

test("names a price only as the server sends it", () => {
  assert.equal(priceOf(prices, "support", "monthly").replace(/\s/g, " "), "4,90 €");
  assert.equal(priceOf(prices, "support", "yearly").replace(/\s/g, " "), "49 €");
  assert.equal(priceOf(prices, "plus", "monthly"), "");
  assert.equal(priceOf(prices, "plus", "yearly"), "");
  assert.equal(priceOf([], "support", "monthly"), "");
});
