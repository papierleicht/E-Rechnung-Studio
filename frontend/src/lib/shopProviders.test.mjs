import test from "node:test";
import assert from "node:assert/strict";
import {
  emptyShopCredentials,
  shopProviderByID,
  shopProviders,
  shopifyClientFields,
  wixSiteIdPattern,
} from "./shopProviders.ts";
import { shopProviderLabel } from "./shopOrders.ts";

test("all supported shops expose their own credentials and correct customer-facing label", () => {
  const expected = [
    ["shopify", "Shopify", ["accessToken"]],
    ["woocommerce", "WooCommerce", ["consumerKey", "consumerSecret"]],
    ["shopware", "Shopware 6", ["clientId", "clientSecret"]],
    ["gambio", "Gambio", ["username", "password"]],
    ["wix", "Wix", ["apiKey"]],
    ["prestashop", "PrestaShop", ["apiKey"]],
  ];
  assert.deepEqual(
    shopProviders.map((provider) => provider.id),
    expected.map(([id]) => id),
  );
  for (const [id, label, keys] of expected) {
    const provider = shopProviderByID(id);
    assert.equal(shopProviderLabel(id), label);
    assert.deepEqual(
      provider.fields.map((field) => field.key),
      keys,
    );
    assert.ok(provider.fields.every((field) => field.label.length > 0));
  }
  assert.equal(shopProviderByID("unknown"), undefined);
  assert.equal(shopProviderLabel("unknown"), "unknown");
  assert.deepEqual(
    shopifyClientFields.map((field) => field.key),
    ["clientId", "clientSecret"],
  );
});

test("provider instructions use official HTTPS documentation and PrestaShop requests only reading", () => {
  const domains = [
    "shopify.dev",
    "developer.woocommerce.com",
    "docs.shopware.com",
    "developers.gambio.de",
    "dev.wix.com",
    "devdocs.prestashop-project.org",
  ];
  for (const [index, provider] of shopProviders.entries()) {
    const guide = new URL(provider.guide);
    assert.equal(guide.protocol, "https:");
    assert.equal(guide.hostname, domains[index]);
    assert.equal(guide.username, "");
    assert.equal(guide.password, "");
    assert.ok(provider.setup.length > 0);
  }
  assert.deepEqual(shopProviderByID("prestashop").readResources, [
    "orders",
    "order_details",
    "addresses",
    "customers",
    "currencies",
    "countries",
    "taxes",
    "order_states",
    "order_payments",
    "order_slip",
  ]);
});

test("new credential state clears every provider secret without reusing a previous object", () => {
  const old = emptyShopCredentials();
  for (const key of Object.keys(old)) old[key] = "previous-provider-secret";
  const fresh = emptyShopCredentials();
  for (const provider of shopProviders) {
    for (const field of provider.fields) assert.equal(fresh[field.key], "");
  }
  for (const field of shopifyClientFields) assert.equal(fresh[field.key], "");
  assert.equal(
    Object.values(fresh).every((value) => value === ""),
    true,
  );
  assert.equal(Object.hasOwn(fresh, "siteId"), false);
  assert.equal(old.apiKey, "previous-provider-secret");
});

test("Wix website validation accepts complete UUIDs and rejects incomplete or foreign identifiers", () => {
  const pattern = new RegExp(`^(?:${wixSiteIdPattern})$`);
  assert.equal(pattern.test("11111111-1111-4111-8111-111111111111"), true);
  assert.equal(pattern.test("ABCDEF01-2345-6789-ABCD-EF0123456789"), true);
  for (const value of [
    "",
    "not-a-uuid",
    "11111111111141118111111111111111",
    "11111111-1111-4111-8111-11111111111x",
  ])
    assert.equal(pattern.test(value), false);
});
