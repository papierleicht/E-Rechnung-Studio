import test from "node:test";
import assert from "node:assert/strict";
import { pdfRasterSize } from "./pdfSizing.ts";

test("PDF text is supersampled at twice the physical display resolution", () => {
  for (const width of [360, 620, 793.7]) {
    for (const density of [1, 1.25, 1.5, 2, 3]) {
      const size = pdfRasterSize(595.28, 841.89, width, density);
      const targetScale = Math.min(
        (width * density * 2) / 595.28,
        Math.sqrt(16_000_000 / (595.28 * 841.89)),
      );
      assert.ok(Math.abs(size.width - 595.28 * targetScale) < 1.01);
      assert.ok(Math.abs(size.height - 841.89 * targetScale) < 1.01);
    }
  }
});

test("large and landscape PDF pages stay within the canvas budget", () => {
  for (const [width, height] of [
    [595, 842],
    [842, 595],
    [100, 10000],
  ]) {
    const size = pdfRasterSize(width, height, 5000, 4);
    assert.ok(size.width * size.height <= 16_000_000);
    assert.ok(size.width <= 8192 && size.height <= 8192);
    assert.ok(size.width > 0 && size.height > 0);
  }
});
