import test from "node:test";
import assert from "node:assert/strict";
import { setupPlanSummary, setupPlusTiers, setupShopSummary } from "./setupOffers.ts";

test("setup offers link to the plans of the booking page", () => {
  assert.deepEqual(
    setupPlusTiers.map((tier) => tier.id),
    ["plus", "backup"],
  );
  assert.ok(setupPlusTiers[0].items.some((item) => item.startsWith("Bank automatisch verbinden")));
});

test("the review names the shop and plan answers", () => {
  const label = (id) => ({ shopify: "Shopify" })[id];
  assert.equal(setupShopSummary("", label), "Übersprungen, Shop-Bestellungen ausgeblendet");
  assert.equal(setupShopSummary("none", label), "Kein Online-Shop, Shop-Bestellungen ausgeblendet");
  assert.equal(setupShopSummary("shopify", label), "Shopify, verbinden direkt nach dem Start");
  assert.equal(setupPlanSummary(""), "Community, kostenlos");
  assert.equal(setupPlanSummary("backup"), "Plus mit Backup: Buchung im Browser geöffnet");
});
