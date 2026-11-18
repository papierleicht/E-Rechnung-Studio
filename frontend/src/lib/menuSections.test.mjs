import assert from "node:assert/strict";
import test from "node:test";
import { optionalSections, sectionVisible, setupSections } from "./menuSections.ts";

test("the shop area follows a connected shop until the customer decides", () => {
  assert.equal(sectionVisible("shops", null, { shopConnected: false }), false);
  assert.equal(sectionVisible("shops", {}, { shopConnected: true }), true);
  assert.equal(sectionVisible("shops", { shops: true }, { shopConnected: false }), true);
  assert.equal(sectionVisible("shops", { shops: false }, { shopConnected: true }), false);
});

test("other areas show unless hidden", () => {
  assert.equal(sectionVisible("products", undefined, { shopConnected: false }), true);
  assert.equal(sectionVisible("products", { products: false }, { shopConnected: false }), false);
  // Core areas cannot be hidden.
  assert.equal(sectionVisible("invoices", { invoices: false }, { shopConnected: false }), true);
  assert.deepEqual(
    optionalSections.map((s) => s.id),
    ["shops", "products", "migration"],
  );
});

test("the move area shows for movers and while the archive holds documents", () => {
  assert.equal(sectionVisible("migration", null, { shopConnected: false }), false);
  assert.equal(sectionVisible("migration", {}, { shopConnected: false, hasLegacy: true }), true);
  assert.equal(sectionVisible("migration", { migration: true }, { shopConnected: false }), true);
  assert.equal(
    sectionVisible("migration", { migration: false }, { shopConnected: false, hasLegacy: true }),
    false,
  );
});

test("the setup shows the shop area only for a chosen shop", () => {
  assert.deepEqual(setupSections(null, "shopify"), { shops: true });
  assert.deepEqual(setupSections({ products: false }, ""), { products: false, shops: false });
});

test("the setup shows the move area only for someone moving", () => {
  assert.deepEqual(setupSections(null, "", "invoiz"), { shops: false, migration: true });
  assert.deepEqual(setupSections({ migration: true }, "", ""), { shops: false, migration: false });
});
