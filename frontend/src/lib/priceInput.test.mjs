import assert from "node:assert/strict";
import test from "node:test";
import { grossUnitPrice, netUnitPrice, roundPrice, typedPrice } from "./priceInput.ts";

test("gross input converts German and dot decimals at common tax rates", () => {
  assert.equal(netUnitPrice("119,00", "19"), "100");
  assert.equal(netUnitPrice("107.00", "7"), "100");
  assert.equal(grossUnitPrice("100", "19"), "119,00");
  assert.equal(grossUnitPrice("100", "7"), "107,00");
});

test("fractional net cents survive conversion and gross prices round commercially", () => {
  assert.equal(netUnitPrice("9,99", "19"), "8.3949579832");
  assert.equal(grossUnitPrice(netUnitPrice("9,99", "19"), "19"), "9,99");
  assert.equal(grossUnitPrice("1.005", "0"), "1,01");
  assert.equal(grossUnitPrice("100", "19,5"), "119,50");
});

test("zero tax preserves the selling price, including free items", () => {
  assert.equal(netUnitPrice("19,99", "0"), "19.99");
  assert.equal(grossUnitPrice("19.99", "0"), "19,99");
  assert.equal(netUnitPrice("0", "19"), "0");
});

test("thousands separators work the way people write prices", () => {
  assert.equal(grossUnitPrice("1.000,00", "19"), "1190,00");
  assert.equal(netUnitPrice("1.190,00", "19"), "1000");
  assert.equal(roundPrice("12.345.678,5"), "12345678,50");
  assert.equal(roundPrice(typedPrice("1.500")), "1500,00");
  assert.equal(typedPrice("1.500,75"), "1500,75");
  assert.equal(roundPrice(typedPrice("0.125")), "0,13");
  assert.equal(roundPrice(typedPrice("1.5")), "1,50");
  assert.equal(grossUnitPrice("1.00,00", "19"), "");
});

test("incomplete or invalid input cannot become an unintended valid price", () => {
  for (const value of ["", "abc", "-1", "1,2,3", "1e3"]) {
    assert.equal(netUnitPrice(value, "19"), "");
    assert.equal(grossUnitPrice(value, "19"), "");
  }
  assert.equal(netUnitPrice("119", ""), "");
  assert.equal(grossUnitPrice("100", "-19"), "");
});

test("cent rounding pads whole euros and rounds ties without floating-point errors", () => {
  for (const [input, expected] of [
    ["1", "1,00"],
    ["0", "0,00"],
    ["1.004", "1,00"],
    ["1.005", "1,01"],
    ["2.675", "2,68"],
    ["9,999", "10,00"],
    ["0,0049", "0,00"],
  ]) {
    assert.equal(roundPrice(input), expected);
  }
  assert.equal(roundPrice(""), "");
  assert.equal(roundPrice("invalid"), "");
});
