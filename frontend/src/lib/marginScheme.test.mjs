import assert from "node:assert/strict";
import test from "node:test";
import { marginLines, otherUses, purchasePriceFrom } from "./marginScheme.ts";

test("finds the margin positions of an invoice", () => {
  const lines = [{ id: "a" }, { id: "b" }];
  assert.deepEqual(marginLines({ taxTreatment: "margin-art", lines }), lines);
  assert.deepEqual(marginLines({ taxTreatment: "standard", lines }), []);
  assert.deepEqual(marginLines({ taxTreatment: "mixed", lines: null }), []);
  const mixed = [
    { id: "a", taxTreatment: "margin-goods" },
    { id: "b", taxTreatment: "standard" },
    { id: "c" },
  ];
  assert.deepEqual(
    marginLines({ taxTreatment: "mixed", lines: mixed }).map((l) => l.id),
    ["a"],
  );
});

test("spreads a receipt over the quantity", () => {
  assert.equal(purchasePriceFrom("4000.00", "1"), "4000,00");
  assert.equal(purchasePriceFrom("100", "3"), "33,33");
  assert.equal(purchasePriceFrom("50", "2,5"), "20,00");
  assert.equal(purchasePriceFrom("x", "1"), "");
});

test("warns about uses on other invoices only", () => {
  assert.deepEqual(otherUses(["RE-1", "RE-2"], "RE-1", true), ["RE-2"]);
  assert.deepEqual(otherUses(["RE-1"], "RE-1", false), ["RE-1"]);
  assert.deepEqual(otherUses(["Entwurf"], "", true), []);
});
