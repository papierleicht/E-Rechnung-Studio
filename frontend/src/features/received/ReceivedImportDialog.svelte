<script lang="ts">
  import type { ReceivedPreview } from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/model/models";
  import FormDialog from "../../components/FormDialog.svelte";
  import ReceivedPDFPreview from "./ReceivedPDFPreview.svelte";
  import InputTaxNotice from "../../components/InputTaxNotice.svelte";
  import ValidationReport from "../../components/ValidationReport.svelte";
  import { dateLabel } from "../../lib/format";
  import { paymentMoney } from "../../lib/paymentWorkflow";
  let {
    filename,
    encoded,
    preview,
    busy,
    error,
    onclose,
    onconfirm,
    onexisting,
  }: {
    filename: string;
    encoded: string;
    preview: ReceivedPreview;
    busy: boolean;
    error: string;
    onclose: () => void;
    onconfirm: () => void;
    onexisting: () => void;
  } = $props();
  const isXML = $derived(filename.toLowerCase().endsWith(".xml"));
  const xml = $derived(
    isXML ? new TextDecoder().decode(Uint8Array.from(atob(encoded), (c) => c.charCodeAt(0))) : "",
  );
</script>

<FormDialog
  title="Rechnung vor dem Hinzufügen prüfen"
  description={filename}
  {busy}
  {error}
  {onclose}
>
  {#if preview.duplicate}<p class="import-notice">
      Diese Originaldatei ist schon gespeichert. Wir öffnen den vorhandenen Beleg; sein Status
      bleibt erhalten.
    </p>{:else}<p class="import-notice">
      Noch nicht gespeichert. Kontrolliere Empfänger, Leistungen und Betrag, bevor du die Rechnung
      übernimmst.
    </p>{/if}
  <dl>
    <div>
      <dt>Verkäufer</dt>
      <dd>{preview.inspection.sellerName || "Nicht erkannt"}</dd>
    </div>
    <div>
      <dt>Empfänger</dt>
      <dd>{preview.inspection.buyerName || "Nicht erkannt"}</dd>
    </div>
    <div>
      <dt>Nummer / Datum</dt>
      <dd>
        {preview.inspection.invoiceNumber || "—"} · {preview.inspection.issueDate
          ? dateLabel(preview.inspection.issueDate)
          : "—"}
      </dd>
    </div>
    <div>
      <dt>Gesamtbetrag</dt>
      <dd>
        {preview.inspection.totals.gross
          ? paymentMoney(preview.inspection.totals.gross, preview.inspection.currency || "EUR")
          : "—"}
      </dd>
    </div>
  </dl>
  <ValidationReport result={preview.inspection.validation} received />
  <InputTaxNotice status={preview.inspection.inputTax} />
  {#if preview.pdf}
    <p class="caption">
      {isXML
        ? "Lesbare Darstellung der XML-Rechnungsdaten. Wir speichern die Original-XML."
        : "Original-PDF. Das Prüfergebnis bezieht sich auf die eingebetteten Rechnungsdaten."}
    </p>
    <ReceivedPDFPreview encoded={preview.pdf} />
  {/if}
  {#if preview.previewError}<p role="status">{preview.previewError}</p>{/if}
  {#if isXML}<details class="detail-row">
      <summary
        ><span class="detail-title">XML-Originalansicht</span><span class="detail-state"
          >Rohdaten der E-Rechnung</span
        ></summary
      >
      <pre>{xml}</pre>
    </details>{/if}
  {#snippet footer()}
    <button type="button" class="secondary" disabled={busy} onclick={onclose}>Abbrechen</button>
    {#if preview.duplicate}<button
        type="button"
        class="primary"
        disabled={busy}
        onclick={onexisting}>Vorhandenen Beleg öffnen</button
      >{:else}<button type="button" class="primary" disabled={busy} onclick={onconfirm}
        >{busy ? "Wird gespeichert …" : "Zum Rechnungseingang hinzufügen"}</button
      >{/if}
  {/snippet}
</FormDialog>

<style>
  .import-notice,
  .caption {
    font-size: 13px;
    color: var(--theme-info-text);
    line-height: 1.5;
  }
  dl {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 14px;
    font-size: 13px;
  }
  dl > div {
    min-width: 0;
  }
  dt {
    color: var(--theme-muted);
    font-size: 12px;
    margin-bottom: 4px;
  }
  dd {
    margin: 0;
    overflow-wrap: anywhere;
  }
  details {
    margin-top: 16px;
  }
  pre {
    white-space: pre-wrap;
    overflow-wrap: anywhere;
    font-size: 11px;
    max-height: 300px;
    overflow: auto;
    overscroll-behavior: contain;
    scrollbar-gutter: stable;
  }
</style>
