import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import vm from "node:vm";
import ts from "typescript";
import { createInvoicePreview } from "./invoicePreview.ts";
import { waitForFileDialog } from "./fileDialog.ts";
import { grossUnitPrice, netUnitPrice, roundPrice, typedPrice } from "./priceInput.ts";

// Execute the production handlers with controlled service responses. No native
// bridge or database is needed to reproduce rejected and out-of-order requests.
function handlers(file, names, state) {
  const source = readFileSync(new URL(`../${file}`, import.meta.url), "utf8").match(
    /<script lang="ts">([\s\S]*?)<\/script>/,
  )[1];
  const ast = ts.createSourceFile("component.ts", source, ts.ScriptTarget.Latest, true);
  const code = ast.statements
    .filter(
      (node) =>
        ts.isFunctionDeclaration(node) &&
        (names.includes(node.name.text) ||
          (file === "App.svelte" &&
            ["currentPage", "pushPage", "showPage"].includes(node.name.text))),
    )
    .map((node) => node.getText(ast))
    .join("\n");
  const context = vm.createContext({
    clearTimeout,
    setTimeout,
    pageStack: [],
    selectedCustomerId: "",
    ...state,
  });
  vm.runInContext(
    ts.transpileModule(code, {
      compilerOptions: { target: ts.ScriptTarget.ES2022 },
    }).outputText,
    context,
  );
  if (file === "App.svelte")
    context.preview = createInvoicePreview({
      draft: () => context.draft,
      clone: structuredClone,
      request: (draft) => context.API.PreviewInvoice(draft),
      pending: (value) => {
        context.previewPending = value;
      },
      publish: (result) => {
        context.view = result;
      },
      error: (error) => {
        context.view = null;
        context.error = String(error);
      },
    });
  return context;
}
const base = () => ({
  busy: false,
  activeOperations: 0,
  error: "",
  errorDetails: "",
  message: "",
  clone: structuredClone,
  errorDetail: (e) => e.message,
  serviceErrorMessage: (e) => e.message,
});

test("cancelling an export picker unlocks the app and cannot cause a late write", async () => {
  let choose,
    writes = 0;
  const context = handlers("App.svelte", ["run", "pickFile", "exportPDF"], {
    ...base(),
    AbortController,
    waitForFileDialog,
    cancelFileDialog: null,
    draft: { id: "invoice-1", number: "RE-1" },
    isXRechnung: false,
    exportedPDF: "",
    Dialogs: {
      SaveFile: () =>
        new Promise((resolve) => {
          choose = resolve;
        }),
    },
    API: {
      ExportInvoicePDF: async (_, path) => {
        writes++;
        return path;
      },
    },
  });
  const pending = context.exportPDF();
  assert.equal(context.busy, true);
  context.cancelFileDialog();
  await pending;
  assert.equal(context.busy, false);
  assert.equal(context.cancelFileDialog, null);
  assert.equal(context.error, "");
  assert.equal(context.errorDetails, "");
  choose("late.pdf");
  await Promise.resolve();
  assert.equal(writes, 0);
  context.Dialogs.SaveFile = async () => "retry.pdf";
  await context.exportPDF();
  assert.equal(writes, 1);
  assert.equal(context.exportedPDF, "retry.pdf");
  context.Dialogs.SaveFile = async () => {
    throw Error(
      "Invalid dialog call: Dialog.SaveFile failed: error getting selection: cancelled by user",
    );
  };
  await context.exportPDF();
  assert.equal(writes, 1);
  assert.equal(context.error, "");
  assert.equal(context.errorDetails, "");
});

test("the settings button also saves unsaved sections and reports their failure", async () => {
  let saved = 0,
    fail = true;
  const context = handlers("App.svelte", ["run", "saveCompany", "rememberSettings"], {
    ...base(),
    settingsSaving: false,
    pendingSaves: {
      numbers: async () => {
        if (fail) throw Error("Das Format braucht genau eine laufende Nummer.");
        saved++;
      },
    },
    data: { settings: { appearance: "dark" }, company: { name: "Same" } },
    settingsBaseline: JSON.stringify({ appearance: "dark" }),
    companyBaseline: JSON.stringify({ name: "Same" }),
    schedulePreview() {},
    API: { SaveSettings: async (settings, company) => ({ settings, company: company ?? {} }) },
  });
  assert.equal(await context.saveCompany(), false);
  assert.equal(saved, 0);
  fail = false;
  assert.equal(await context.saveCompany(), true);
  assert.equal(saved, 1);
});

test("failed settings commit preserves both baselines and permits discard or retry", async () => {
  let fail = true;
  const context = handlers(
    "App.svelte",
    ["run", "saveCompany", "rememberSettings", "restoreSettings"],
    {
      ...base(),
      settingsSaving: false,
      pendingSaves: {},
      data: { settings: { appearance: "dark" }, company: { name: "Edited" } },
      settingsBaseline: JSON.stringify({ appearance: "light" }),
      companyBaseline: JSON.stringify({ name: "Original" }),
      schedulePreview() {},
      API: {
        SaveSettings: async (settings, company) => {
          if (fail) throw Error("write failure");
          return { settings, company };
        },
      },
    },
  );
  assert.equal(await context.saveCompany(), false);
  assert.equal(context.busy, false);
  assert.equal(context.settingsSaving, false);
  assert.equal(context.data.settings.appearance, "dark");
  context.restoreSettings();
  assert.equal(context.data.settings.appearance, "light");
  assert.equal(context.data.company.name, "Original");
  context.data.settings.appearance = "dark";
  context.data.company.name = "Retry";
  fail = false;
  assert.equal(await context.saveCompany(), true);
  assert.equal(JSON.parse(context.settingsBaseline).appearance, "dark");
  assert.equal(JSON.parse(context.companyBaseline).name, "Retry");
});

test("failed manual validation cannot leave the previous valid result actionable", async () => {
  let fail = true;
  const context = handlers("App.svelte", ["run", "validate"], {
    ...base(),

    previewPending: true,
    draft: { status: "draft" },
    view: { validation: { state: "valid" } },
    API: {
      ValidateInvoice: async () => {
        if (fail) throw Error("offline");
        return { validation: { state: "valid" } };
      },
    },
  });
  await context.validate();
  assert.equal(context.view, null);
  assert.equal(context.previewPending, false);
  assert.equal(context.busy, false);
  fail = false;
  await context.validate();
  assert.equal(context.view.validation.state, "valid");
});

test("discarding a customer draft while opening another invoice clears unsaved state", async () => {
  const context = handlers(
    "App.svelte",
    ["run", "openInvoice", "hasUnsavedChanges", "rememberDraft", "acceptInvoiceView"],
    {
      ...base(),
      draft: { status: "draft" },
      draftBaseline: "",
      prospectiveCustomer: { name: "Unfinished" },
      newCustomer: () => ({ name: "" }),
      settingsChanged: () => false,
      restoreSettings() {},
      dialogDirty: false,
      unfinishedInvoice: false,
      confirmDiscard: async () => true,

      previewPending: false,
      API: {
        OpenInvoice: async () => ({
          draft: { status: "issued", customer: { name: "Saved" } },
          export: { pdfPath: "saved.pdf" },
        }),
      },
      view: null,
      exportedPDF: "",
      invoiceStep: "overview",
      section: "invoices",
    },
  );
  await context.openInvoice({ id: "saved" });
  assert.equal(context.hasUnsavedChanges(), false);
  assert.equal(context.prospectiveCustomer.name, "");
});

test("received operations block navigation until completion, including on failure", async () => {
  const app = handlers("App.svelte", ["navigate"], {
    financeTab: "overview",
    navigationLocked: false,
    navigationPending: false,
    confirmation: null,
    section: "received",
    navigateTo(next) {
      app.section = next;
    },
  });
  const received = handlers("features/received/ReceivedInvoices.svelte", ["perform"], {
    busy: false,
    error: "",
    message: "",
    disposed: false,
    onbusychange(value) {
      app.navigationLocked = value;
    },
    serviceErrorMessage: (e) => e.message,
  });
  let reject;
  const pending = received.perform(
    () =>
      new Promise((_, no) => {
        reject = no;
      }),
  );
  await app.navigate("settings");
  assert.equal(app.section, "received");
  reject(Error("import failed"));
  await pending;
  assert.equal(received.error, "import failed");
  assert.equal(app.navigationLocked, false);
  await app.navigate("settings");
  assert.equal(app.section, "settings");
});

test("HTML preview discards late responses after a newer invoice is selected", async () => {
  let resolve;
  const context = handlers("features/invoices/InvoiceHTML.svelte", ["render"], {
    performance,
    Promise,
    revision: 1,
    loading: true,
    fontsPromise: Promise.resolve("fonts"),
    load: undefined,
    InvoiceDocumentHTML: () => new Promise((r) => (resolve = r)),
    prepareInvoice() {
      throw Error("stale invoice reached the screen");
    },
  });
  const pending = context.render({}, 1);
  context.revision = 2;
  resolve("<html>Old invoice</html>");
  await pending;
  assert.equal(context.loading, true);
});

test("HTML preview failure hides stale data and releases the loading state", async () => {
  let cleared = false;
  const context = handlers("features/invoices/InvoiceHTML.svelte", ["render"], {
    performance,
    Promise,
    revision: 1,
    loading: true,
    rendered: true,
    error: "",
    fontsPromise: Promise.resolve("fonts"),
    load: undefined,
    InvoiceDocumentHTML: async () => {
      throw Error("HTML unavailable");
    },
    frame: {
      contentDocument: {
        body: {
          replaceChildren() {
            cleared = true;
          },
        },
      },
    },
  });
  await context.render({}, 1);
  assert.equal(context.rendered, false);
  assert.equal(context.loading, false);
  assert.equal(cleared, true);
  assert.match(context.error, /HTML unavailable/);
});

test("unreadable backup reports an error, releases busy state and never calls restore", async () => {
  let restored = false;
  const context = handlers("App.svelte", ["run", "importBackup"], {
    ...base(),
    confirmAction: async () => true,
    pickFile: async () => "/tmp/sicherung.zip",
    Dialogs: { OpenFile: async () => "" },
    restoreLoss: () => "",
    BackupAPI: {
      InspectArchive: async () => {
        throw Error("Die Datei ist keine Sicherung von E-Rechnung-Studio.");
      },
      ImportArchive: async () => {
        restored = true;
      },
    },
  });
  await context.importBackup();
  assert.equal(restored, false);
  assert.equal(context.busy, false);
  assert.match(context.error, /keine Sicherung von E-Rechnung-Studio/);
});

test("failed old preview cannot clear another invoice opened meanwhile", async () => {
  let reject;
  const context = handlers("App.svelte", [], {
    ...base(),
    draft: { id: "old", status: "draft" },

    previewPending: false,
    view: null,
    API: {
      PreviewInvoice: () =>
        new Promise((_, no) => {
          reject = no;
        }),
    },
  });
  const pending = context.preview.refresh();
  context.draft = { id: "new", status: "draft" };
  const currentView = { draft: context.draft, validation: { state: "valid" } };
  context.view = currentView;
  reject(Error("old preview failed"));
  await pending;
  assert.equal(context.view, currentView);
  assert.equal(context.error, "");
});

test("position-list correction opens the position editor", async () => {
  let edited = false;
  const context = handlers("App.svelte", ["resolveField"], {
    editLines() {
      edited = true;
    },
    tick: async () => {},
    section: "invoices",
    invoiceStep: "document",
    pageSettingsOpen: false,
    document: { querySelectorAll: () => [] },
    invoiceDocument: { editField() {} },
  });
  await context.resolveField("lines");
  assert.equal(edited, true);
});

test("accepting a saved or opened invoice cancels older checks and clears pending state", async () => {
  for (const failure of [false, true]) {
    let finish, reject;
    const context = handlers("App.svelte", ["acceptInvoiceView"], {
      ...base(),
      draft: { id: "old", status: "draft" },

      previewPending: false,
      view: null,
      API: {
        PreviewInvoice: () =>
          new Promise((yes, no) => {
            finish = yes;
            reject = no;
          }),
      },
    });
    const pending = context.preview.refresh();
    const saved = { draft: { id: "saved", status: "draft" }, validation: { state: "valid" } };
    context.acceptInvoiceView(saved);
    assert.equal(context.previewPending, false);
    if (failure) reject(Error("late failure"));
    else finish({ draft: { id: "old" }, validation: { state: "invalid" } });
    await pending;
    assert.equal(context.view, saved);
    assert.equal(context.draft, saved.draft);
    assert.equal(context.error, "");
  }
});

test("gross position input updates unit net, follows tax changes and ignores quantity", () => {
  const context = handlers(
    "features/invoices/PositionEditor.svelte",
    ["effectiveRate", "grossPrice", "setGrossPrice"],
    {
      mixedTax: false,
      taxTreatment: "standard",
      grossInputs: {},
      netInputs: {},
      grossUnitPrice,
      netUnitPrice,
    },
  );
  const line = { id: "line", unitPrice: "100", taxRate: "19", quantity: "3" };
  assert.equal(context.grossPrice(line), "119,00");
  context.setGrossPrice(line, "59,50");
  assert.equal(line.unitPrice, "50");
  assert.equal(context.grossPrice(line), "59,50");
  line.quantity = "-2";
  assert.equal(context.grossPrice(line), "59,50");
  line.taxRate = "7";
  assert.equal(context.grossPrice(line), "53,50");
  line.unitPrice = "200";
  assert.equal(context.grossPrice(line), "214,00");
  context.mixedTax = true;
  for (const treatment of ["small-business", "zero-rated", "reverse-charge", "margin-goods"]) {
    line.taxTreatment = treatment;
    context.setGrossPrice(line, "19,99");
    assert.equal(line.unitPrice, "19.99");
    assert.equal(context.grossPrice(line), "19,99");
  }
  line.taxTreatment = "standard";
  assert.equal(context.grossPrice(line), "21,39");
  context.setGrossPrice(line, "");
  assert.equal(line.unitPrice, "");
  assert.equal(context.grossPrice(line), "");
});

test("price fields round edits to cents without truncating converted net prices on focus or save", () => {
  const context = handlers(
    "features/invoices/PositionEditor.svelte",
    [
      "effectiveRate",
      "grossPrice",
      "setGrossPrice",
      "netPrice",
      "setNetPrice",
      "finishNetPrice",
      "finishGrossPrice",
    ],
    {
      mixedTax: false,
      taxTreatment: "standard",
      grossInputs: {},
      netInputs: {},
      grossUnitPrice,
      netUnitPrice,
      roundPrice,
      typedPrice,
    },
  );
  const line = { id: "line", unitPrice: "100", taxRate: "19" };
  assert.equal(context.netPrice(line), "100,00");
  context.setGrossPrice(line, "9,995");
  context.finishGrossPrice(line);
  assert.equal(context.grossPrice(line), "10,00");
  assert.equal(context.netPrice(line), "8,40");
  assert.equal(line.unitPrice, "8.4033613445");
  context.finishNetPrice(line);
  assert.equal(line.unitPrice, "8.4033613445");
  context.setNetPrice(line, "1,005");
  assert.equal(context.netPrice(line), "1,005");
  context.finishNetPrice(line);
  assert.equal(line.unitPrice, "1,01");
  assert.equal(context.grossPrice(line), "1,20");
  context.setNetPrice(line, "invalid");
  context.finishNetPrice(line);
  assert.equal(context.netPrice(line), "invalid");
});

test("template selection creates a fresh draft and preserves the editor when loading fails", async () => {
  const source = { id: "issued", status: "paid", customer: { name: "Original" } };
  const saved = { draft: { id: "copy", status: "draft", customer: source.customer } };
  let fail = true;
  let saves = 0;
  const context = handlers("App.svelte", ["run", "chooseInvoiceTemplate", "createTemplateDraft"], {
    ...base(),
    draft: { id: "current" },
    exportedPDF: "",
    prospectiveCustomer: { name: "" },
    invoiceStep: "customer",
    hasUnsavedChanges: () => false,
    invoiceFromTemplate: (draft) => ({ ...draft, id: "", status: "draft" }),
    API: {
      OpenInvoice: async () => {
        if (fail) throw new Error("Load failed");
        return { draft: source };
      },
      SaveInvoice: async (draft) => {
        saves++;
        assert.equal(draft.id, "");
        return saved;
      },
    },
    acceptInvoiceView: (result) => {
      context.draft = result.draft;
    },
    rememberDraft: () => {},
    newCustomer: () => ({ name: "" }),
    restoreSettings: () => {},
    refreshApplicationLists: async () => {},
    scrollPageToTop: () => {},
  });
  await context.chooseInvoiceTemplate({ id: "issued", status: "paid" });
  assert.equal(context.draft.id, "current");
  assert.equal(context.invoiceStep, "customer");
  assert.equal(saves, 0);
  fail = false;
  await context.chooseInvoiceTemplate({ id: "issued", status: "paid" });
  assert.equal(context.draft.id, "copy");
  assert.equal(context.invoiceStep, "document");
  assert.equal(saves, 1);
  await context.chooseInvoiceTemplate({ id: "draft", status: "draft" });
  assert.equal(saves, 1);
  context.hasUnsavedChanges = () => true;
  context.confirmDiscard = async () => false;
  await context.chooseInvoiceTemplate({ id: "issued", status: "issued" });
  assert.equal(saves, 1);
});

test("the invoices navigation returns from invoice steps without discarding entered data", async () => {
  const draft = { id: "", customer: { name: "Unsaved customer" } };
  const context = handlers("App.svelte", ["navigate", "navigateTo"], {
    financeTab: "overview",
    message: "",
    navigationLocked: false,
    navigationPending: false,
    confirmation: null,
    section: "invoices",
    invoiceStep: "customer",
    draft,
    prospectiveCustomer: { name: "Customer being entered" },
    tick: () => Promise.resolve(),
    scrollPageToTop: () => {},
  });
  for (const step of ["customer", "document"]) {
    context.invoiceStep = step;
    await context.navigate("invoices");
    assert.equal(context.section, "invoices");
    assert.equal(context.invoiceStep, "overview");
    assert.equal(context.draft, draft);
    assert.equal(context.prospectiveCustomer.name, "Customer being entered");
  }
  for (const guard of ["navigationLocked", "navigationPending", "confirmation"]) {
    context.invoiceStep = "customer";
    context[guard] = true;
    await context.navigate("invoices");
    assert.equal(context.invoiceStep, "customer");
    context[guard] = false;
  }
});

test("deleting a draft returns to the invoice overview even if list refresh fails", async () => {
  for (const refreshFails of [false, true]) {
    const context = handlers("App.svelte", ["run", "deleteDraft", "rememberDraft", "navigateTo"], {
      ...base(),
      draft: { id: "deleted", status: "draft" },
      view: { draft: { id: "deleted" } },
      data: { invoices: [{ id: "deleted" }, { id: "kept" }] },
      section: "invoices",
      invoiceStep: "document",
      exportedPDF: "old.pdf",
      prospectiveCustomer: { name: "" },
      draftBaseline: "",
      confirmAction: async () => true,
      API: { DeleteDraft: async (id) => assert.equal(id, "deleted") },
      newCustomer: () => ({ name: "" }),
      refreshApplicationLists: async () => {
        if (refreshFails) throw new Error("Refresh failed");
      },
      tick: () => Promise.resolve(),
      scrollPageToTop: () => {},
    });
    await context.deleteDraft();
    assert.equal(context.invoiceStep, "overview");
    assert.equal(context.draft, null);
    assert.equal(context.view, null);
    assert.equal(context.draftBaseline, "");
    assert.equal(context.exportedPDF, "");
    assert.equal(context.data.invoices.length, 1);
    assert.equal(context.data.invoices[0].id, "kept");
  }
});

test("cancelled or failed draft deletion keeps the current invoice open", async () => {
  for (const confirmed of [false, true]) {
    const draft = { id: "keep", status: "draft" };
    const context = handlers("App.svelte", ["run", "deleteDraft"], {
      ...base(),
      draft,
      invoiceStep: "document",
      confirmAction: async () => confirmed,
      API: {
        DeleteDraft: async () => {
          throw new Error("Deletion failed");
        },
      },
    });
    await context.deleteDraft();
    assert.equal(context.draft, draft);
    assert.equal(context.invoiceStep, "document");
  }
});
