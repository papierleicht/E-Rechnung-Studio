import test from "node:test";
import assert from "node:assert/strict";
import { isFileDialogCancellation, waitForFileDialog } from "./fileDialog.ts";

test("file selection returns paths and hides native user cancellation", async () => {
  const signal = new AbortController().signal;
  assert.equal(await waitForFileDialog(async () => "invoice.pdf", signal), "invoice.pdf");
  assert.equal(await waitForFileDialog(async () => "", signal), "");
  const nativeCancellation = new Error(
    "Invalid dialog call: Dialog.SaveFile failed: error getting selection: cancelled by user",
  );
  assert.equal(
    await waitForFileDialog(async () => Promise.reject(nativeCancellation), signal),
    undefined,
  );
  assert.equal(isFileDialogCancellation(nativeCancellation), true);
  assert.equal(isFileDialogCancellation(new Error("permission denied")), false);
  await assert.rejects(
    waitForFileDialog(async () => {
      throw Error("native error");
    }, signal),
    /native error/,
  );
  await assert.rejects(
    waitForFileDialog(() => {
      throw Error("sync error");
    }, signal),
    /sync error/,
  );
});

test("stalled dialog times out and late selection cannot trigger an export", async () => {
  let choose;
  let writes = 0;
  const result = waitForFileDialog(
    () =>
      new Promise((resolve) => {
        choose = resolve;
      }),
    new AbortController().signal,
    5,
  ).then(() => {
    writes++;
  });
  await assert.rejects(result, /nicht geantwortet/);
  choose("late.pdf");
  await Promise.resolve();
  assert.equal(writes, 0);
  assert.equal(
    await waitForFileDialog(async () => "retry.pdf", new AbortController().signal),
    "retry.pdf",
  );
});

test("explicit cancellation releases selection and ignores its later response", async () => {
  const controller = new AbortController();
  let choose;
  const result = waitForFileDialog(
    () =>
      new Promise((resolve) => {
        choose = resolve;
      }),
    controller.signal,
  );
  controller.abort();
  assert.equal(await result, undefined);
  choose("late.pdf");
  let opened = false;
  assert.equal(
    await waitForFileDialog(async () => {
      opened = true;
    }, controller.signal),
    undefined,
  );
  assert.equal(opened, false);
});
