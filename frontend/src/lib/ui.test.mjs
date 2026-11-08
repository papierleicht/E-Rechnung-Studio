import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";

const source = (name) => readFileSync(new URL(`../${name}`, import.meta.url), "utf8");

test("per-position tax selection stays inside the position row", () => {
  const editor = source("features/invoices/PositionEditor.svelte");
  assert.doesNotMatch(editor, /position-tax-details|colspan="7"/);
  assert.match(
    editor,
    /<td class="position-tax-cell">[\s\S]*?data-field=\{`lines\.\$\{index\}\.taxTreatment`\}[\s\S]*?<\/td/,
  );
  assert.match(editor, /aria-label=\{`Steuerfall Position/);
  assert.match(editor, /required=\{line.taxTreatment === "exempt" && !line.exemptionCode\}/);
});

test("shared UI rules follow component and responsive styles", () => {
  const main = source("main.ts");
  assert.ok(main.indexOf("./styles/ui.css") > main.indexOf("./styles/responsive.css"));
  assert.ok(main.indexOf("./styles/windows.css") > main.indexOf("./styles/ui.css"));
  const css = source("styles/ui.css");
  assert.match(css, /label:has\(> input\[type="checkbox"\]\)/);
  assert.match(css, /flex-direction: row/);
  assert.match(css, /justify-content: flex-end/);
  assert.match(css, /focus-visible/);
});

test("Windows motion stays brief, preserves invoice text and respects reduced motion", () => {
  const css = source("styles/windows.css");
  assert.match(css, /--win-motion-fast:\s*90ms/);
  assert.match(css, /--win-motion-layout:\s*160ms/);
  assert.match(css, /\.workspace > \.document-stage[\s\S]*win-fade-enter/);
  assert.doesNotMatch(css, /\.document-stage[^\{]*\{[^}]*transform:/);
  assert.match(css, /prefers-reduced-motion:\s*reduce/);
  assert.match(css, /animation-duration:\s*0\.01ms !important/);
  assert.match(css, /@media print[\s\S]*animation:\s*none !important/);
});

test("each page saves with one button in its own header, not in the window toolbar", () => {
  const app = source("App.svelte");
  assert.equal((app.match(/onclick=\{saveDraft\}/g) || []).length, 1);
  assert.match(app, /<PageHeader[\s\S]*?\{#snippet actions\(\)\}[\s\S]*?onclick=\{saveDraft\}/);
  assert.match(app, /onsave=\{saveCompany\}/);
  // Only the setup screen keeps a window toolbar; in the app the page head is the top row.
  assert.equal((app.match(/class="window-toolbar/g) || []).length, 1);
  const settings = source("features/settings/SettingsPage.svelte");
  assert.equal((settings.match(/onclick=\{onsave\}/g) || []).length, 1);
  assert.match(settings, /<header class="topbar">[\s\S]*?onclick=\{onsave\}[\s\S]*?<\/header>/);
  assert.doesNotMatch(settings, /saveExportSettings|saveCompanyAndReturn/);
  assert.doesNotMatch(
    source("features/invoices/InvoiceCompletion.svelte"),
    /onclick=\{saveDraft\}/,
  );
});

test("shared and specialized modal dialogs keep actions in a footer", () => {
  for (const name of [
    "components/FormDialog",
    "features/invoices/InvoiceSettingsDialog",
    "features/invoices/InvoiceFieldsDialog",
    "features/invoices/PositionEditor",
  ]) {
    const text = source(`${name}.svelte`);
    assert.match(text, /<dialog/);
    assert.match(text, /<header>/);
    assert.match(text, /<footer>/);
    assert.match(text, /oncancel=/);
  }
  for (const name of [
    "features/customers/CustomerDialog",
    "features/products/ProductDialog",
    "features/received/ReceivedImportDialog",
    "features/received/ReceivedManageDialog",
    "features/received/XMLInspectionDialog",
    "features/settings/AboutDialog",
  ]) {
    assert.match(source(`${name}.svelte`), /<FormDialog/);
  }
});

test("invoice layout is reserved for separate customization", () => {
  const dialog = source("features/invoices/InvoiceSettingsDialog.svelte");
  assert.doesNotMatch(dialog, /Seitenlayout|data-field="layout"|bind:value=\{working\.layout\}/);

  const app = source("App.svelte");
  assert.doesNotMatch(app, /draft\.layout|defaults\?\.layout/);

  const models = source("../bindings/papierleicht.de/e-rechnung-studio/internal/model/models.ts");
  assert.doesNotMatch(models, /"layout"/);

  const preview = source("lib/htmlInvoice.ts");
  assert.match(preview, /padding:12mm 14mm 32mm/);
  assert.doesNotMatch(preview, /data-layout/);

  const service = source(
    "../bindings/papierleicht.de/e-rechnung-studio/internal/app/invoiceservice.ts",
  );
  assert.doesNotMatch(service, /InvoiceDocumentPDF/);
});
