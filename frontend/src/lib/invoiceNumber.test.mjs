import assert from "node:assert/strict";
import test from "node:test";
import { applyNumberChoice, numberChoice, numberSeriesTitle } from "./invoiceNumber.ts";

const options = [
  { value: "standard", kind: "standard", label: "Standard", next: "RE-2026-0004" },
  { value: "", kind: "customer", label: "Müller GmbH", next: "MUE-2026-008" },
];

test("the dialog preselects the effective series or the own number", () => {
  assert.equal(numberChoice({ selected: "", options }), "");
  assert.equal(numberChoice({ selected: "manual", options }), "manual");
  // An unknown series falls back to the first option instead of an empty choice.
  assert.equal(numberChoice({ selected: "X-{NNN}", options }), "standard");
  assert.equal(numberChoice({ selected: "", options: [] }), "manual");
});

test("series titles name the customer", () => {
  assert.equal(numberSeriesTitle(options[1]), "Fortlaufend für Müller GmbH");
  assert.equal(numberSeriesTitle(options[0]), "Fortlaufend (Standard)");
  assert.equal(numberSeriesTitle({ kind: "recent" }), "Fortlaufend wie zuletzt verwendet");
});

test("the payment reference follows the number unless the user changed it", () => {
  const draft = { number: "RE-2026-0004", paymentReference: "RE-2026-0004", automaticNumber: true };
  applyNumberChoice(draft, { number: "2026/118", automaticNumber: false, numberSeries: "" });
  assert.deepEqual(draft, {
    number: "2026/118",
    paymentReference: "2026/118",
    automaticNumber: false,
    numberSeries: "",
  });
  const custom = { number: "RE-1", paymentReference: "Bestellung 7", automaticNumber: true };
  applyNumberChoice(custom, { number: "RE-2", automaticNumber: true, numberSeries: "standard" });
  assert.equal(custom.paymentReference, "Bestellung 7");
});
