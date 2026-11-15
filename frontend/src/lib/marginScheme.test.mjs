import assert from "node:assert/strict";
import test from "node:test";
import { marginLines } from "./marginScheme.ts";

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
