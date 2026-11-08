import assert from "node:assert/strict";
import test from "node:test";
import { readPageText } from "./pdfText.ts";

test("PDF text reads multiple chunks without ReadableStream async iteration (WKWebView)", async () => {
  const chunks = [{ items: [{ str: "Rechnung" }] }, { items: [{ str: "Beschreibung" }] }];
  let released = false;
  const page = {
    streamTextContent: () => ({
      getReader: () => ({
        read: async () => (chunks.length ? { value: chunks.shift(), done: false } : { done: true }),
        releaseLock: () => {
          released = true;
        },
      }),
    }),
  };
  assert.deepEqual(await readPageText(page), {
    items: [{ str: "Rechnung" }, { str: "Beschreibung" }],
  });
  assert.equal(released, true);
});

test("a failed PDF text stream releases its lock and preserves the failure", async () => {
  let released = false;
  const failure = new Error("PDF text unavailable");
  const page = {
    streamTextContent: () => ({
      getReader: () => ({
        read: async () => {
          throw failure;
        },
        releaseLock: () => {
          released = true;
        },
      }),
    }),
  };
  await assert.rejects(readPageText(page), failure);
  assert.equal(released, true);
});
