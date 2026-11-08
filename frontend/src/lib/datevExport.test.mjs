import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { revenueFields, severityLabels } from "./datevExport.ts";

const source = (name) => readFileSync(new URL(`../${name}`, import.meta.url), "utf8");

test("DATEV settings offer every revenue tax case once", () => {
  const keys = revenueFields.map((f) => f.key);
  assert.equal(new Set(keys).size, keys.length);
  for (const key of [
    "standard19",
    "reduced7",
    "reverseCharge",
    "intraCommunity",
    "smallBusiness",
    "margin",
  ])
    assert.ok(keys.includes(key), key);
  assert.deepEqual(Object.keys(severityLabels), ["blocking", "excluded", "review"]);
});

test("DATEV export is a free finance action with a checklist and advisor note", () => {
  const page = source("features/finance/TaxesPage.svelte");
  assert.match(page, /title="Für die Steuerkanzlei/);
  assert.match(page, /DATEV-Export<\/button/);
  assert.match(page, /onclick=\{\(\) => \(datevOpen = true\)\}/);
  const dialog = source("features/finance/DatevExportDialog.svelte");
  assert.match(dialog, /<FormDialog/);
  assert.match(dialog, /Mit der Kanzlei abstimmen/);
  assert.match(dialog, /disabled=\{busy \|\| !preview\?\.ready\}/);
  assert.doesNotMatch(dialog, /Upgrade|Premium|plusActive|backupActive/);
});
