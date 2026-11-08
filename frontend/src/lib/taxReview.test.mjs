import assert from "node:assert/strict";
import test from "node:test";
import { applyTaxDecision, taxReviewItems } from "./taxReview.ts";

const line = (taxRate, extra = {}) => ({ taxRate, taxCategory: "S", name: "Leistung", ...extra });
const draft = (lines, extra = {}) => ({
  status: "draft",
  taxTreatment: "standard",
  lines,
  ...extra,
});

test("review distinguishes unresolved zero, special rates and invalid inputs", () => {
  const result = taxReviewItems(
    draft(
      ["0", " 0,00 ", "19", "7", "5,5", "7.8", "16", "5", "9", "", "invalid", "-1"].map((r) =>
        line(r),
      ),
    ),
  );
  assert.deepEqual(
    result.map((x) => x.index),
    [0, 1, 4, 5, 6, 7, 8],
  );
  assert.match(result.find((x) => x.rate === 5.5).message, /§ 24/);
  assert.match(result.find((x) => x.rate === 16).message, /2020/);
  assert.deepEqual(taxReviewItems(draft([line("0")], { taxTreatment: "small-business" })), []);
  assert.deepEqual(
    taxReviewItems(
      draft(
        [
          line("0", { taxTreatment: "zero-rated" }),
          line("0", { taxTreatment: "exempt" }),
          line("0"),
        ],
        { taxTreatment: "mixed" },
      ),
    ).map((x) => x.index),
    [2],
  );
});

test("decision keeps other positions and original draft intact, discarding stale hidden overrides", () => {
  const original = draft([
    line("0"),
    line("19", { taxTreatment: "exempt", exemptionReason: "stale", exemptionCode: "stale" }),
  ]);
  const result = applyTaxDecision(original, 0, "exempt", "Steuerfrei nach § 4 Nr. 14 UStG");
  assert.equal(result.taxTreatment, "mixed");
  assert.equal(result.taxSource, "manual");
  assert.equal(result.lines[0].taxCategory, "E");
  assert.match(result.lines[0].exemptionReason, /§ 4/);
  assert.equal(result.lines[1].taxTreatment, "standard");
  assert.equal(result.lines[1].taxRate, "19");
  assert.equal(result.lines[1].exemptionCode, "");
  assert.equal(original.taxTreatment, "standard");
  assert.equal(original.lines[0].taxCategory, "S");
});

test("mixed invoice preserves other explicit exemptions and codes", () => {
  const original = draft(
    [
      line("0"),
      line("0", { taxTreatment: "exempt", exemptionReason: "Befreiung", exemptionCode: "custom" }),
    ],
    { taxTreatment: "mixed" },
  );
  const result = applyTaxDecision(original, 0, "zero-rated");
  assert.deepEqual(result.lines[1], original.lines[1]);
  assert.equal(result.lines[0].taxTreatment, "zero-rated");
  assert.equal(result.lines[0].taxCategory, "Z");
  assert.deepEqual(taxReviewItems(result), []);
});

test("positive S requires a positive rate; exemption requires a reason", () => {
  const original = draft([line("0")]);
  for (const rate of ["0", "", "-7", "NaN", "Infinity", "0x13"])
    assert.throws(() => applyTaxDecision(original, 0, "standard", "", rate));
  assert.equal(applyTaxDecision(original, 0, "standard", "", "7,8").lines[0].taxRate, "7,8");
  assert.throws(() => applyTaxDecision(original, 0, "exempt", "  "));
  assert.throws(() => applyTaxDecision({ ...original, status: "issued" }, 0, "zero-rated"));
});

test("outside-scope cannot silently create invalid mixed invoices; adjustments need coordinated editing", () => {
  assert.throws(
    () => applyTaxDecision(draft([line("0"), line("19")]), 0, "out-of-scope"),
    /eigenen Rechnung/,
  );
  assert.equal(applyTaxDecision(draft([line("0")]), 0, "out-of-scope").lines[0].taxCategory, "O");
  assert.throws(
    () =>
      applyTaxDecision(
        draft([line("0")], { adjustments: [{ taxCategory: "Z", taxRate: "0", amount: "10" }] }),
        0,
        "exempt",
        "Befreiung",
      ),
    /Zuschläge/,
  );
});
