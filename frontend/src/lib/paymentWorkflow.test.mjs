import test from "node:test";
import assert from "node:assert/strict";
import { decimalInput, plainDecimal } from "./format.ts";
import { collectable, paymentTitle, paymentDescription } from "./paymentWorkflow.ts";
const invoice = {
  state: "teilbezahlt",
  remaining: "600.00",
  paid: "400.00",
  gross: "1000.00",
  currency: "EUR",
  overpaid: "0.00",
  installments: false,
  planNeedsReview: false,
};
test("multiple payments show the rest without implying an installment agreement", () => {
  assert.equal(paymentTitle(invoice), "Teilweise bezahlt");
  assert.match(paymentDescription(invoice), /600,00/);
  assert.doesNotMatch(paymentDescription(invoice), /Rate/);
});
test("explicit agreements show the next installment and flag revoked prior payments", () => {
  const planned = { ...invoice, installments: true, nextAmount: "200.00", nextDate: "2026-11-01" };
  assert.equal(paymentTitle(planned), "Ratenzahlung");
  assert.match(paymentDescription(planned), /200,00.*01\.11\.2026/);
  assert.equal(paymentTitle({ ...planned, planNeedsReview: true }), "Ratenplan prüfen");
  assert.equal(paymentTitle({ ...planned, state: "überfällig" }), "Rate überfällig");
});
test("a refunded reduction asks for the credit note, not for the money", () => {
  const refunded = {
    ...invoice,
    state: "gutschrift-offen",
    remaining: "19.00",
    awaitingCredit: "19.00",
  };
  assert.equal(paymentTitle(refunded), "Gutschrift fehlt");
  assert.match(paymentDescription(refunded), /19,00.*erstattet/);
});
test("the collectable amount leaves out refunds waiting for a credit note", () => {
  assert.equal(collectable({ remaining: "19.00", awaitingCredit: "19.00" }), 0);
  assert.equal(collectable({ remaining: "100.10", awaitingCredit: "40.05" }), 60.05);
  assert.equal(collectable({ remaining: "19.00" }), 19);
});
test("typed German amounts reach the services as plain decimals", () => {
  assert.equal(plainDecimal("1.234,50"), "1234.50");
  assert.equal(plainDecimal("452,20"), "452.20");
  assert.equal(plainDecimal("916.30"), "916.30");
  assert.equal(decimalInput("916.30"), "916,30");
});
