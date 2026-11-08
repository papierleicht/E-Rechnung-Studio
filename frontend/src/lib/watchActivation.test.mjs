import assert from "node:assert/strict";
import test from "node:test";
import { watchActivation } from "./watchActivation.ts";

function fakeWindow() {
  const listeners = new Map();
  return {
    addEventListener: (type, fn) => listeners.set(type, fn),
    removeEventListener: (type) => listeners.delete(type),
    focus: () => listeners.get("focus")?.(),
    listening: () => listeners.has("focus"),
  };
}

const settle = () => new Promise((resolve) => setImmediate(resolve));

test("checks on every tick and stops once the subscription is active", async (t) => {
  t.mock.timers.enable({ apis: ["setInterval"] });
  const win = fakeWindow();
  let calls = 0;
  watchActivation(async () => ++calls === 3, { every: 1000, target: win, now: () => 0 });
  for (let i = 0; i < 5; i++) {
    t.mock.timers.tick(1000);
    await settle();
  }
  assert.equal(calls, 3);
  assert.equal(win.listening(), false);
});

test("focus checks right away", async () => {
  const win = fakeWindow();
  let calls = 0;
  const watch = watchActivation(async () => (++calls, false), { every: 60_000, target: win });
  win.focus();
  await settle();
  assert.equal(calls, 1);
  watch.stop();
  win.focus();
  await settle();
  assert.equal(calls, 1);
});

test("gives up after the duration", async (t) => {
  t.mock.timers.enable({ apis: ["setInterval"] });
  const win = fakeWindow();
  let clock = 0;
  let calls = 0;
  watchActivation(async () => (++calls, false), {
    every: 1000,
    duration: 2500,
    target: win,
    now: () => clock,
  });
  for (let i = 0; i < 5; i++) {
    clock += 1000;
    t.mock.timers.tick(1000);
    await settle();
  }
  assert.equal(calls, 2);
  assert.equal(win.listening(), false);
});

test("never runs two checks at once and survives a failing check", async () => {
  const win = fakeWindow();
  let release;
  let calls = 0;
  const watch = watchActivation(
    () => {
      calls++;
      if (calls === 1) return new Promise((resolve) => (release = resolve));
      return Promise.reject(new Error("offline"));
    },
    { every: 60_000, target: win },
  );
  win.focus();
  win.focus();
  await settle();
  assert.equal(calls, 1);
  release(false);
  await settle();
  win.focus();
  await settle();
  assert.equal(calls, 2);
  win.focus();
  await settle();
  assert.equal(calls, 3);
  watch.stop();
});
