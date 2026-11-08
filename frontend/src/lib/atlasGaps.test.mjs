import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { defaultProfile, fillElectronicAddress } from "./invoiceDraft.ts";
import { quoteFromTemplate } from "./quotes.ts";
import { reverseChargeHint } from "./taxReview.ts";

test("a Leitweg-ID preselects XRechnung and an ordinary reference does not", () => {
  assert.equal(defaultProfile({ buyerReference: "04011000-1234512345-06" }), "xrechnung-ubl");
  assert.equal(defaultProfile({ buyerReference: "Bestellung 4711" }), "facturx-en16931");
  assert.equal(defaultProfile({ buyerReference: "" }), "facturx-en16931");
});

test("the customer's e-mail fills an empty electronic address only", () => {
  const empty = { email: "a@b.example", endpoint: "", endpointScheme: "" };
  fillElectronicAddress(empty);
  assert.deepEqual([empty.endpoint, empty.endpointScheme], ["a@b.example", "EM"]);
  const own = { email: "a@b.example", endpoint: "991-1", endpointScheme: "0204" };
  fillElectronicAddress(own);
  assert.equal(own.endpoint, "991-1");
});

test("a quote template keeps customer and positions but no identity", () => {
  const copy = quoteFromTemplate(
    {
      id: "q1",
      number: "AN-1",
      status: "sent",
      document: {
        id: "d",
        number: "AN-1",
        status: "sent",
        issuerSnapshot: {},
        customer: { name: "Kunde" },
        lines: [{ id: "l1", name: "Leistung" }],
      },
    },
    "2026-11-02",
  );
  assert.equal(copy.id, "");
  assert.equal(copy.status, "draft");
  assert.equal(copy.document.number, "");
  assert.equal(copy.document.issueDate, "2026-11-02");
  assert.equal(copy.document.customer.name, "Kunde");
  assert.equal(copy.document.lines[0].name, "Leistung");
  assert.notEqual(copy.document.lines[0].id, "l1");
  assert.equal(copy.document.issuerSnapshot, undefined);
});

test("the reverse charge note needs a customer in another EU state with a VAT ID", () => {
  const draft = (country, vatId, taxTreatment = "standard") => ({
    status: "draft",
    taxTreatment,
    customer: { vatId, address: { country } },
  });
  assert.equal(reverseChargeHint(draft("DK", "DK12345678")), true);
  assert.equal(reverseChargeHint(draft("DE", "DE811907980")), false);
  assert.equal(reverseChargeHint(draft("DK", "")), false);
  assert.equal(reverseChargeHint(draft("US", "123")), false);
  assert.equal(reverseChargeHint(draft("DK", "DK12345678", "reverse-charge")), false);
});

test("the tax profile task opens the tax page, not the finance overview", () => {
  const url = new URL("../features/dashboard/Dashboard.svelte", import.meta.url);
  assert.match(readFileSync(url, "utf8"), /task\.id === "tax-profile" \? ontaxes\(""\)/);
});
