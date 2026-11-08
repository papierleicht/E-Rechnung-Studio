import test from "node:test";
import assert from "node:assert/strict";
import {
  shopImportDate,
  shopMoney,
  shopOrderState,
  shopOrderReviewReason,
  shopDate,
  shopPaymentLabel,
} from "./shopOrders.ts";

test("a linked invoice remains openable when a changed order also needs review", () => {
  assert.equal(
    shopOrderState({ invoiceId: "invoice-1", reviewReason: "Bestellung geändert" }),
    "invoice",
  );
  assert.equal(shopOrderState({ invoiceId: "", reviewReason: "Steuerdaten fehlen" }), "review");
  assert.equal(shopOrderState({ invoiceId: "", reviewReason: "" }), "ready");
  assert.equal(
    shopOrderState({ invoiceId: "", reviewReason: "", conversionError: "Firmenadresse fehlt" }),
    "review",
  );
  assert.equal(
    shopOrderReviewReason({
      reviewReason: "Shopdaten fehlen",
      conversionError: "Firmenadresse fehlt",
    }),
    "Shopdaten fehlen",
  );
});

test("shop totals preserve the imported currency and reject malformed amounts", () => {
  assert.equal(shopMoney("119.00", "EUR"), "119,00\u00a0€");
  assert.match(shopMoney("119.00", "USD"), /119,00/);
  assert.notEqual(shopMoney("119.00", "USD"), shopMoney("119.00", "EUR"));
  for (const gross of ["", "NaN", "Infinity", "12,00", "12e3"])
    assert.equal(shopMoney(gross, "EUR"), "—");
});

test("the initial import period is thirty local calendar days across month and year boundaries", () => {
  assert.equal(shopImportDate(new Date(2026, 9, 15, 12)), "2026-09-15");
  assert.equal(shopImportDate(new Date(2026, 0, 15, 12)), "2025-12-16");
});

test("shop calendar dates retain their source day while actual timestamps use the local timezone", () => {
  const originalTimezone = process.env.TZ;
  process.env.TZ = "Europe/Berlin";
  try {
    const lateOrder = "2026-10-15T23:30:00Z";
    assert.equal(shopDate(lateOrder, false, true), "15.10.2026");
    assert.equal(shopDate(lateOrder), "16.10.2026");
    assert.match(shopDate(lateOrder, true), /^16\.10\.2026/);
    assert.equal(shopDate("invalid", false, true), "—");
  } finally {
    if (originalTimezone === undefined) delete process.env.TZ;
    else process.env.TZ = originalTimezone;
  }
});

test("source review never presents an ambiguous payment as definitively open", () => {
  assert.equal(
    shopPaymentLabel({ paid: false, reviewReason: "Zahlungsstatus unklar" }),
    "Zahlung prüfen",
  );
  assert.equal(shopPaymentLabel({ paid: false, reviewReason: "" }), "Zahlung offen");
  assert.equal(
    shopPaymentLabel({ paid: true, reviewReason: "Bestellung geändert" }),
    "Bereits bezahlt",
  );
});
