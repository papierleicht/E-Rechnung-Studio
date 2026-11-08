import test from "node:test";
import assert from "node:assert/strict";
import { setupMissing, chooseSetupTax } from "./onboarding.ts";
import { setupTaxGuidance, setupPaymentGuidance } from "./setupGuidance.ts";
import {
  taxChoices,
  paymentChoices,
  chooseInvoiceTaxPreset,
  companyTaxTreatment,
  invoiceTaxPreset,
  invoiceTaxDefaults,
  initializePositionTaxes,
} from "./tax.ts";
import { readFileSync } from "node:fs";
import { compile } from "svelte/compiler";

test("the rendered country pattern accepts two-letter codes", () => {
  const source = readFileSync(
    new URL("../features/settings/CompanySetup.svelte", import.meta.url),
    "utf8",
  );
  const { js } = compile(source, { generate: "server" });
  assert.ok(js.code.includes('pattern="[A-Za-z]{2}"'));
});

function company() {
  return {
    name: "Musterfirma",
    address: { street: "Musterstraße 1", postcode: "12345", city: "Musterstadt", country: "DE" },
    email: "rechnung@example.de",
    vatId: "",
    taxNumber: "123456789",
    iban: "",
    smallBusiness: false,
    defaultTaxTreatment: "standard",
    paymentMeansCode: "10",
    exemptionReason: "",
  };
}
test("every offered tax and payment choice has an explanation", () => {
  for (const { id } of taxChoices) {
    assert.ok(setupTaxGuidance[id]?.meaning);
    assert.ok(setupTaxGuidance[id]?.example);
    assert.ok(setupTaxGuidance[id]?.effect);
  }
  for (const { id } of paymentChoices) assert.ok(setupPaymentGuidance[id]);
});
test("company status and invoice presets are independent", () => {
  const c = company();
  c.exemptionReason = "Eigener Hinweis";
  chooseSetupTax(c, "standard");
  assert.equal(c.exemptionReason, "Eigener Hinweis");
  chooseSetupTax(c, "small-business");
  assert.equal(c.smallBusiness, true);
  assert.equal(c.exemptionReason, "");
  chooseInvoiceTaxPreset(c, "reverse-charge");
  assert.equal(c.smallBusiness, true);
  assert.equal(companyTaxTreatment(c), "reverse-charge");
  chooseSetupTax(c, "unknown");
  assert.equal(c.taxStatusUnclear, true);
  assert.equal(c.invoiceTaxPreset, "reverse-charge");
  chooseSetupTax(c, "standard");
  assert.equal(c.taxStatusUnclear, false);
  assert.equal(c.invoiceTaxPreset, "reverse-charge");
  chooseInvoiceTaxPreset(c, "");
  assert.equal(companyTaxTreatment(c), "standard");
});

test("legacy special defaults survive status changes and can be explicitly disabled", () => {
  const c = company();
  c.defaultTaxTreatment = "exempt";
  c.exemptionReason = "Bestehender Befreiungsgrund";
  assert.equal(invoiceTaxPreset(c), "exempt");
  chooseSetupTax(c, "small-business");
  assert.equal(c.invoiceTaxPreset, "exempt");
  assert.equal(c.exemptionReason, "Bestehender Befreiungsgrund");
  chooseInvoiceTaxPreset(c, "");
  assert.equal(companyTaxTreatment(c), "small-business");
  assert.equal(c.exemptionReason, "");
});

test("new customer invoices inherit the current profile unless tax was explicitly overridden", () => {
  const c = company();
  chooseSetupTax(c, "small-business");
  for (const defaults of [
    undefined,
    { taxTreatment: "standard", taxExemptionReason: "" },
    { taxTreatment: "standard", taxSource: "company", taxExemptionReason: "old" },
  ]) {
    const actual = invoiceTaxDefaults(c, defaults);
    assert.equal(actual.taxTreatment, "small-business");
    assert.equal(actual.taxSource, "company");
    assert.equal(actual.taxExemptionReason, "");
  }
  const manual = invoiceTaxDefaults(c, {
    taxTreatment: "standard",
    taxSource: "manual",
    taxExemptionReason: "",
  });
  assert.equal(manual.taxTreatment, "standard");
  assert.equal(manual.taxSource, "customer");
  assert.equal(
    invoiceTaxDefaults(c, { taxTreatment: "exempt", taxExemptionReason: "Legacy special" })
      .taxExemptionReason,
    "Legacy special",
  );
  chooseInvoiceTaxPreset(c, "reverse-charge");
  assert.equal(invoiceTaxDefaults(c, { taxTreatment: "standard" }).taxTreatment, "reverse-charge");
});

test("enabling small-business replaces a routine standard preset", () => {
  const c = company();
  chooseInvoiceTaxPreset(c, "standard");
  chooseSetupTax(c, "small-business");
  assert.equal(companyTaxTreatment(c), "small-business");
});

test("per-position initialization preserves explicit choices and inherits the prior treatment", () => {
  const d = { lines: [{ taxRate: "19" }, { taxTreatment: "zero-rated", taxRate: "0" }] };
  initializePositionTaxes(d, "small-business", "§ 19 Hinweis");
  assert.equal(d.lines[0].taxTreatment, "small-business");
  assert.equal(d.lines[0].exemptionReason, "§ 19 Hinweis");
  assert.equal(d.lines[1].taxTreatment, "zero-rated");
  initializePositionTaxes(d, "standard");
  assert.equal(d.lines[0].taxTreatment, "small-business");
});

test("profile supports per-position defaults and keeps company status", () => {
  const c = company();
  c.smallBusiness = true;
  chooseInvoiceTaxPreset(c, "mixed");
  assert.equal(c.smallBusiness, true);
  assert.equal(invoiceTaxDefaults(c).taxTreatment, "mixed");
});
test("cash does not request an IBAN, transfers do", () => {
  const c = company();
  assert.deepEqual(setupMissing(c), []);
  c.paymentMeansCode = "58";
  assert.equal(setupMissing(c)[0].step, 4);
});
test("review routes missing details to the relevant interview step", () => {
  const c = company();
  c.address.postcode = "";
  c.email = "";
  c.taxNumber = "";
  assert.deepEqual(
    setupMissing(c).map((item) => item.step),
    [0, 1, 2],
  );
  c.defaultTaxTreatment = "intra-community";
  assert.ok(setupMissing(c).some((item) => item.message.includes("innergemeinschaftliche")));
});
