<script lang="ts">
  import InvoiceHTML from "./InvoiceHTML.svelte";
  import InvoiceFieldsDialog from "./InvoiceFieldsDialog.svelte";
  import InvoiceNumberDialog from "./InvoiceNumberDialog.svelte";
  import { applyNumberChoice } from "../../lib/invoiceNumber";
  import Icon from "../../components/Icon.svelte";
  import { taxLabel } from "../../lib/tax";
  import type {
    Company,
    InvoiceDraft,
  } from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/model/models";
  let {
    draft = $bindable(),
    company,
    kind = "invoice",
    validUntil = $bindable(""),
    editable,
    load,
    renderKey = "",
    disabled = false,
    previewPending = false,
    onchange,
    onsettings,
    onedittax,
    oneditcompany,
    onchangecustomer,
    oneditline,
    oneditorstatechange,
  }: {
    draft: InvoiceDraft;
    company: Company;
    /** Quotes use the same editor; their document carries the quote's own state. */
    kind?: "invoice" | "quote";
    validUntil?: string;
    /** Defaults to the draft status; a quote decides by its own status. */
    editable?: boolean;
    load?: (draft: InvoiceDraft) => Promise<string>;
    renderKey?: string;
    disabled?: boolean;
    previewPending?: boolean;
    onchange: () => void;
    /** Opens the invoice or quote settings dialog. */
    onsettings: () => void;
    onedittax: () => void;
    oneditcompany: () => void;
    onchangecustomer: () => void;
    oneditline: (index: number) => void;
    oneditorstatechange: (dirty: boolean) => void;
  } = $props();
  let editing = $state(""),
    focusField = $state("");
  const canEdit = $derived(editable ?? draft.status === "draft");
  function edit(part: string) {
    if (disabled || !canEdit) return;
    focusField = "";
    if (part === "number" && kind !== "invoice") part = "dates";
    if (part === "company") oneditcompany();
    else if (part === "lines") oneditline(0);
    else editing = part;
  }
  export function editField(field: string) {
    const part = ["disablePaymentQR", "hidePaymentQRDetails"].includes(field)
      ? "qr"
      : field === "number"
        ? "number"
        : field.startsWith("customer.")
          ? "customer"
          : ["paymentTerms", "paymentReference", "notes", "taxExemptionReason"].includes(field)
            ? "payment"
            : "dates";
    edit(part);
    focusField = field;
  }
  function save(value: InvoiceDraft, quote: { validUntil: string }) {
    if (value.taxExemptionReason !== draft.taxExemptionReason) draft.taxSource = "manual";
    if (editing === "qr") {
      draft.disablePaymentQR = value.disablePaymentQR;
      draft.hidePaymentQRDetails = value.hidePaymentQRDetails;
    } else if (editing === "customer") draft.customer = value.customer;
    else if (editing === "dates") {
      for (const key of [
        "issueDate",
        "deliveryDate",
        "periodStart",
        "periodEnd",
        "dueDate",
        "deliveryCountry",
        "buyerReference",
      ] as const)
        draft[key] = value[key];
      draft.advancePayment = value.advancePayment;
      draft.customer.paymentTermDays = value.customer.paymentTermDays;
      if (kind === "quote") validUntil = quote.validUntil;
    } else
      for (const key of [
        "paymentTerms",
        "paymentReference",
        "notes",
        "taxExemptionReason",
      ] as const)
        draft[key] = value[key];
    editing = "";
    onchange();
  }
</script>

<!-- Invoices and quotes share this editor: the same hint, settings, tax line and
     clickable document. -->
{#if canEdit}
  <div class="document-edit-help no-print">
    <span class="document-edit-hint">
      <Icon name="edit" size={15} />
      <span class="hint-long">Klicke einen Abschnitt an, um ihn zu bearbeiten.</span>
      <span class="hint-short">Zum Bearbeiten anklicken</span>
    </span>
    <span class="document-edit-actions">
      {#if kind === "invoice"}<button
          class="text"
          {disabled}
          title="Nummernkreis wählen oder eigene Nummer vergeben"
          onclick={() => edit("number")}
        >
          <Icon name="edit" size={16} />
          Nr. {draft.number || "—"}
          <span class="number-mode">{draft.automaticNumber ? "automatisch" : "eigene"}</span>
        </button>{/if}
      <button class="text" {disabled} onclick={onsettings}>
        <Icon name="page-settings" size={16} />
        {kind === "invoice" ? "Rechnungseinstellungen" : "Angebotseinstellungen"}
      </button>
    </span>
  </div>
{/if}
<div class="document-stage">
  <p class="form-help tax-line">
    <span
      >Steuerbehandlung: {taxLabel(draft.taxTreatment)} · {draft.taxSource === "company"
        ? "Aus Rechnungsvorgaben"
        : draft.taxSource === "customer"
          ? "Aus Kundenvorgaben"
          : kind === "invoice"
            ? "Nur für diese Rechnung"
            : "Nur für dieses Angebot"}</span
    >
    {#if canEdit}<button type="button" class="text" {disabled} onclick={onedittax}>Ändern</button
      >{/if}
  </p>
  {#if canEdit && kind === "invoice" && draft.disablePaymentQR}
    <div class="qr-settings-action no-print">
      <button type="button" class="text" {disabled} onclick={() => edit("qr")}
        >QR-Code-Einstellungen</button
      >
    </div>
  {/if}
  {#if draft.recurringName && draft.status === "draft"}<p class="recurring-draft-note no-print">
      Entwurf aus der Dauerrechnung „{draft.recurringName}“. Prüfe Positionen, Zeitraum und Datum,
      bevor du ihn ausstellst.
    </p>{/if}
  <InvoiceHTML
    draft={canEdit === (draft.status === "draft")
      ? draft
      : { ...draft, status: canEdit ? "draft" : "issued" }}
    {company}
    {disabled}
    pending={previewPending}
    onedit={edit}
    {load}
    {renderKey}
    numberEditable={kind === "invoice"}
  />
</div>
{#if editing === "number"}<InvoiceNumberDialog
    {draft}
    onsave={(value) => {
      applyNumberChoice(draft, value);
      editing = "";
      onchange();
    }}
    ondirtychange={oneditorstatechange}
    oncancel={() => (editing = "")}
  />{:else if editing}<InvoiceFieldsDialog
    {draft}
    {company}
    {kind}
    {validUntil}
    part={editing}
    {focusField}
    onsave={save}
    ondirtychange={oneditorstatechange}
    oncancel={() => (editing = "")}
    onchoose={() => {
      editing = "";
      onchangecustomer();
    }}
  />{/if}

<style>
  /* Hint and actions share one row down to the 900px minimum window; below
     about 720px of row width the hint switches to its short form. */
  .document-edit-help {
    flex-wrap: nowrap;
    container-type: inline-size;
  }
  .document-edit-hint {
    min-width: 0;
  }
  .hint-long,
  .hint-short {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .hint-short {
    display: none;
  }
  @container (max-width: 720px) {
    .hint-long {
      display: none;
    }
    .hint-short {
      display: inline;
    }
  }
  .document-edit-actions {
    display: inline-flex;
    flex: none;
    gap: 16px;
  }
  /* "Ändern" stays at the end of the first line when a long tax label wraps. */
  .document-stage > .tax-line {
    flex-wrap: nowrap;
    align-items: baseline;
  }
  .tax-line > span {
    min-width: 0;
  }
  .tax-line > button {
    flex: none;
  }
  .number-mode {
    padding: 1px 6px;
    border-radius: 999px;
    background: var(--theme-surface-muted);
    color: var(--theme-muted);
    font-size: 11px;
    font-weight: 500;
  }
  .qr-settings-action {
    max-width: 210mm;
    margin: 0 auto 12px;
    text-align: right;
  }
</style>
