import test from "node:test";
import assert from "node:assert/strict";
import { lookupGermanBank, normalizeIBAN } from "./bankLookup.ts";

test("normalizes spaces and lower-case letters", () => {
  assert.equal(normalizeIBAN("de89 3704 0044 0532 0130 00"), "DE89370400440532013000");
});

test("finds the bank and BIC for a valid German IBAN", () => {
  assert.deepEqual(lookupGermanBank("DE89 3704 0044 0532 0130 00"), {
    name: "Commerzbank",
    bic: "COBADEFFXXX",
  });
});

test("does not fill details for incomplete, foreign, or invalid German IBANs", () => {
  assert.equal(lookupGermanBank("DE89 3704"), undefined);
  assert.equal(lookupGermanBank("GB82 WEST 1234 5698 7654 32"), undefined);
  assert.equal(lookupGermanBank("DE88 3704 0044 0532 0130 00"), undefined);
});
