import assert from "node:assert/strict";
import test from "node:test";
import { taxGroups } from "./tax.ts";

test("zero-percent positions and explicit zero-rated positions share adjustment group Z", () => {
  const groups = taxGroups({
    taxTreatment: "mixed",
    lines: [
      { taxTreatment: "standard", taxCategory: "S", taxRate: "0,00" },
      { taxTreatment: "zero-rated", taxRate: "19" },
      { taxTreatment: "standard", taxCategory: "S", taxRate: "19" },
      { taxTreatment: "small-business", taxRate: "0" },
    ],
  });
  assert.deepEqual(
    groups.map((g) => g.key),
    ["Z|0", "S|19", "E|0"],
  );
  assert.equal(groups[0].label, "Nullsteuersatz · 0 %");
});
