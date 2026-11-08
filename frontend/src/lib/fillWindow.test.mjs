import assert from "node:assert/strict";
import test from "node:test";
import { fillHeight } from "./fillWindow.ts";

test("the element ends at the bottom of the window", () => {
  // 800px window, chat starts at 400px, page padding of 32px below.
  assert.equal(fillHeight(800, 400, 32, 320), 368);
  assert.equal(fillHeight(1200, 400, 32, 320), 768);
});

test("a small window scrolls instead of squeezing the element", () => {
  assert.equal(fillHeight(600, 400, 32, 320), 320);
  assert.equal(fillHeight(800.6, 400.2, 32, 320), 368);
});
