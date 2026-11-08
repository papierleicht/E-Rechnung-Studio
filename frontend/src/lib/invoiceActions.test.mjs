import test from "node:test";
import assert from "node:assert/strict";
import { createInvoiceActions } from "./invoiceActions.ts";

test("export failure preserves issued invoice and concurrent issue calls are blocked", async () => {
  const state = {
    draft: { id: "1", status: "draft" },
    view: { validation: { state: "valid" } },
    busy: false,
    issuing: false,
    previewPending: false,
  };
  let finish,
    calls = 0,
    error = "",
    exported = "";
  const actions = createInvoiceActions({
    state: () => state,
    api: {
      IssueInvoice: () => {
        calls++;
        return new Promise((resolve) => {
          finish = resolve;
        });
      },
    },
    run: async (operation) => {
      error = "";
      return operation();
    },
    accept: (result) => {
      state.draft = result.draft;
      state.view = result;
    },
    message() {},
    error: (value) => {
      error = value;
    },
    exported: (value) => {
      exported = value;
    },
    issuing: (value) => {
      state.issuing = value;
    },
    refresh: async () => {},
    showCompletion: async () => {},
    exportXML: async () => {},
  });
  const pending = actions.issue();
  await actions.issue();
  assert.equal(calls, 1);
  finish({
    draft: { id: "1", kind: "invoice", number: "RE-1", status: "issued" },
    export: { pdfPath: "", error: "Kein Schreibzugriff" },
    validation: { state: "valid" },
  });
  await pending;
  assert.equal(state.draft.status, "issued");
  assert.equal(state.issuing, false);
  assert.equal(error, "Kein Schreibzugriff");
  assert.equal(exported, "");
  assert.equal(calls, 1);
});

test("rejected issuance releases its guard without accepting a new invoice", async () => {
  let issuing = false,
    accepted = false;
  const actions = createInvoiceActions({
    state: () => ({
      draft: { status: "draft" },
      view: { validation: { state: "valid" } },
      issuing,
    }),
    api: {
      IssueInvoice: async () => {
        throw Error("Prüfung fehlgeschlagen");
      },
    },
    run: async (operation) => {
      try {
        return await operation();
      } catch {
        return undefined;
      }
    },
    issuing: (value) => {
      issuing = value;
    },
    accept: () => {
      accepted = true;
    },
  });
  await actions.issue();
  assert.equal(issuing, false);
  assert.equal(accepted, false);
});
