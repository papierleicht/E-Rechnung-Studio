import test from "node:test";
import assert from "node:assert/strict";
import { createInvoicePreview } from "./invoicePreview.ts";

test("rapid edits debounce to the newest snapshot; disposal cancels queued work", async (t) => {
  t.mock.timers.enable({ apis: ["setTimeout"] });
  let draft = { status: "draft", number: "first" },
    pending = false;
  const calls = [];
  const preview = createInvoicePreview({
    draft: () => draft,
    clone: structuredClone,
    request: async (value) => {
      calls.push(value);
      return { draft: value };
    },
    publish() {},
    pending(value) {
      pending = value;
    },
    error(error) {
      throw error;
    },
  });
  preview.schedule();
  t.mock.timers.tick(200);
  draft.number = "latest";
  preview.schedule();
  t.mock.timers.tick(349);
  assert.equal(calls.length, 0);
  assert.equal(pending, true);
  t.mock.timers.tick(1);
  await Promise.resolve();
  assert.deepEqual(calls, [{ status: "draft", number: "latest" }]);
  assert.equal(pending, false);
  preview.schedule();
  preview.cancel();
  t.mock.timers.tick(1000);
  assert.equal(calls.length, 1);
  assert.equal(pending, false);
});

test("manual validation supersedes an in-flight automatic preview", async () => {
  const draft = { status: "draft" };
  let finish, result;
  const preview = createInvoicePreview({
    draft: () => draft,
    clone: structuredClone,
    request: () =>
      new Promise((resolve) => {
        finish = resolve;
      }),
    publish(value) {
      result = value;
    },
    pending() {},
    error(error) {
      throw error;
    },
  });
  const automatic = preview.refresh();
  await preview.check(
    async () => ({ validation: { state: "valid" } }),
    (value) => {
      result = value;
    },
  );
  finish({ validation: { state: "invalid" } });
  await automatic;
  assert.equal(result.validation.state, "valid");
});
