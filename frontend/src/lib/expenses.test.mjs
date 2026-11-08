import test from "node:test";
import assert from "node:assert/strict";
import {
  assetTreatment,
  attachmentType,
  elsterValue,
  isReverseCharge,
  normalizeAmount,
  reverseChargeHint,
  splitGross,
  usefulLives,
} from "./expenses.ts";

test("gross amounts split like the Go side", () => {
  assert.deepEqual(splitGross("119,00", "19"), { net: 100, tax: 19 });
  assert.deepEqual(splitGross("10.70", "7"), { net: 10, tax: 0.7 });
  assert.deepEqual(splitGross("1.190,00", "19"), { net: 1000, tax: 190 });
  assert.deepEqual(splitGross("50", "none"), { net: 50, tax: 0 });
  assert.deepEqual(splitGross("500", "reverse-charge-eu"), { net: 500, tax: 0 });
  // Restaurant bill: 59,50 € drinks at 19 %, 42,80 € food at 7 %.
  assert.deepEqual(splitGross("102,30", "mixed", "42,80"), { net: 90, tax: 12.3 });
  assert.equal(splitGross("102,30", "mixed", ""), null);
  assert.equal(splitGross("102,30", "mixed", "102,30"), null);
  assert.equal(splitGross("", "19"), null);
  assert.equal(splitGross("-3", "19"), null);
  assert.equal(normalizeAmount("1.190,5"), "1190.50");
  assert.equal(normalizeAmount("abc"), "");
});

test("reverse-charge receipts name the tax owed", () => {
  assert.equal(isReverseCharge("reverse-charge-eu"), true);
  assert.equal(isReverseCharge("reverse-charge"), true);
  assert.equal(isReverseCharge("none"), false);
  assert.match(reverseChargeHint(500, true), /Kleinunternehmer.*95,00/);
  assert.match(reverseChargeHint(500, false), /95,00.*Vorsteuer/);
});

test("purchases explain whether they are depreciated", () => {
  assert.match(assetTreatment(500, 5), /voll abgesetzt/);
  assert.match(assetTreatment(500, 0), /voll abgesetzt/);
  assert.match(assetTreatment(3000, 0), /Wähle/);
  assert.match(assetTreatment(2000, 1), /voll abgeschrieben/);
  assert.match(assetTreatment(3000, 3), /über 3 Jahre/);
});

test("useful lives follow the BMF letter and the AfA table", () => {
  const years = Object.fromEntries(usefulLives.map((l) => [l.label.split(",")[0], l.years]));
  assert.equal(years["Computer"], 1);
  assert.equal(years["Kamera"], 7);
  assert.ok(usefulLives.some((l) => l.years === 1 && l.label.includes("Drucker")));
  assert.ok(!usefulLives.some((l) => l.years === 0));
});

test("ELSTER values use German decimals without grouping", () => {
  assert.equal(elsterValue("1234.5", true), "1234,50");
  assert.equal(elsterValue("1234", false), "1234");
  assert.equal(elsterValue("-12.30", true), "-12,30");
});

test("attachments accept PDFs and photos", () => {
  assert.equal(attachmentType("Beleg.PDF"), "application/pdf");
  assert.equal(attachmentType("foto.jpeg"), "image/jpeg");
  assert.equal(attachmentType("notiz.txt"), "");
});
