import assert from "node:assert/strict";
import test from "node:test";
import { createUpdateMonitor, UPDATE_INTERVAL } from "./updateMonitor.ts";
function fixture() {
  let now = 1_000_000,
    enabled = true,
    online = true,
    calls = 0,
    fail = false;
  const storage = new Map(),
    published = [];
  const options = {
    currentVersion: "1.0",
    enabled: () => enabled,
    online: () => online,
    now: () => now,
    storage: { getItem: (k) => storage.get(k) ?? null, setItem: (k, v) => storage.set(k, v) },
    check: async () => {
      calls++;
      if (fail) throw Error("offline");
      return { currentVersion: "1.0", updateAvailable: true, availableVersion: "1.1" };
    },
    publish: (s) => published.push(s),
  };
  return {
    options,
    published,
    storage,
    get calls() {
      return calls;
    },
    advance: (n) => (now += n),
    enable: (v) => (enabled = v),
    online: (v) => (online = v),
    fail: (v) => (fail = v),
  };
}
test("checks require consent and network, then repeat every interval", async () => {
  const f = fixture();
  const m = createUpdateMonitor(f.options);
  f.enable(false);
  await m.tick();
  f.enable(true);
  f.online(false);
  await m.tick();
  assert.equal(f.calls, 0);
  f.online(true);
  await m.tick();
  assert.equal(f.calls, 1);
  assert.equal(f.published[0].availableVersion, "1.1");
  f.advance(UPDATE_INTERVAL - 1);
  await m.tick();
  assert.equal(f.calls, 1);
  f.advance(1);
  await m.tick();
  assert.equal(f.calls, 2);
});
test("every start checks again but shows the cached result first", async () => {
  const f = fixture();
  await createUpdateMonitor(f.options).tick();
  assert.equal(f.calls, 1);
  f.advance(60_000);
  const published = f.published.length;
  const m = createUpdateMonitor(f.options);
  assert.equal(f.published.length, published + 1);
  assert.equal(f.published.at(-1).availableVersion, "1.1");
  await m.tick();
  assert.equal(f.calls, 2);
});
test("a manual check runs with automatic checks off and reports failures", async () => {
  const f = fixture();
  f.enable(false);
  const m = createUpdateMonitor(f.options);
  const status = await m.checkNow();
  assert.equal(status.availableVersion, "1.1");
  assert.equal(f.calls, 1);
  assert.equal(f.published.at(-1).availableVersion, "1.1");
  f.fail(true);
  await assert.rejects(m.checkNow(), /offline/);
  assert.equal(f.calls, 2);
});
test("a manual check joins a running automatic check", async () => {
  const f = fixture();
  let release;
  f.options.check = () => {
    f.options.calls = (f.options.calls ?? 0) + 1;
    return new Promise((r) => (release = r));
  };
  const m = createUpdateMonitor(f.options);
  const automatic = m.tick();
  const manual = m.checkNow();
  assert.equal(f.options.calls, 1);
  release({ currentVersion: "1.0", updateAvailable: false });
  await automatic;
  assert.equal((await manual).updateAvailable, false);
});
test("failed checks silently retry after backoff", async () => {
  const f = fixture();
  const m = createUpdateMonitor(f.options);
  f.fail(true);
  await m.tick();
  assert.equal(f.published.length, 0);
  await m.tick();
  assert.equal(f.calls, 1);
  f.advance(15 * 60 * 1000);
  f.fail(false);
  await m.tick();
  assert.equal(f.calls, 2);
});
test("concurrent ticks coalesce and stopped monitors ignore late results", async () => {
  const f = fixture();
  let release;
  let calls = 0;
  f.options.check = () => {
    calls++;
    return new Promise((r) => (release = r));
  };
  const m = createUpdateMonitor(f.options);
  const pending = m.tick();
  await m.tick();
  assert.equal(calls, 1);
  m.stop();
  release({ currentVersion: "1.0", updateAvailable: false });
  await pending;
  assert.equal(f.published.length, 0);
  assert.equal(f.storage.size, 0);
});
test("a cached result of another app version is not shown", async () => {
  const f = fixture();
  await createUpdateMonitor(f.options).tick();
  const published = f.published.length;
  f.options.currentVersion = "1.1";
  createUpdateMonitor(f.options);
  assert.equal(f.published.length, published);
});
