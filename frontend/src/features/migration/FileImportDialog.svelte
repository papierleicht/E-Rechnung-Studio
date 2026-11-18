<script lang="ts">
  import FormDialog from "../../components/FormDialog.svelte";
  import { dateLabel } from "../../lib/format";
  import { paymentMoney } from "../../lib/paymentWorkflow";
  import { directionLabels, kindLabel, type FilePreview } from "../../lib/migration-api";
  let {
    preview,
    sourceName,
    busy = false,
    error = "",
    onconfirm,
    onclose,
  }: {
    preview: FilePreview;
    sourceName: string;
    busy?: boolean;
    error?: string;
    onconfirm: () => void;
    onclose: () => void;
  } = $props();
  // Long exports show their first rows; the counts above cover all of them.
  const shown = $derived(preview.items.slice(0, 300));
  let showSkipped = $state(false);
  function money(value: string, currency: string) {
    return value ? paymentMoney(value, currency || "EUR") : "—";
  }
</script>

<FormDialog
  title="Rechnungen und Belege übernehmen"
  description={`Aus ${sourceName} in den Altbestand. Die Dateien bleiben unverändert und zählen nicht in deine Zahlen.`}
  {busy}
  {error}
  {onclose}
  onsubmit={onconfirm}
>
  <ul class="move-counts" aria-label="Ergebnis der Prüfung">
    <li><strong>{preview.new}</strong> neu</li>
    <li><strong>{preview.eInvoices}</strong> davon E-Rechnungen mit Daten</li>
    <li><strong>{preview.duplicates}</strong> schon im Altbestand</li>
    <li><strong>{preview.skipped.length}</strong> übersprungen</li>
  </ul>
  {#if preview.items.length}
    <!-- svelte-ignore a11y_no_noninteractive_tabindex -->
    <div class="move-table-wrap" tabindex="0" role="region" aria-label="Gefundene Dokumente">
      <table class="move-table">
        <thead
          ><tr><th>Datei</th><th>Art</th><th>Partner</th><th class="move-amount">Betrag</th></tr
          ></thead
        >
        <tbody>
          {#each shown as item (item.origin)}
            <tr class:move-duplicate={item.duplicate}>
              <td
                ><strong title={item.origin}>{item.number || item.name}</strong><small
                  >{item.date ? dateLabel(item.date) : item.origin}</small
                ></td
              >
              <td
                >{item.duplicate
                  ? "schon vorhanden"
                  : kindLabel(item.kind, item.basis)}{#if item.basis}<small
                    >{directionLabels[item.direction] ?? item.direction}</small
                  >{:else if item.kind === "export"}<small
                    >{item.files === 1 ? "1 Datei" : `${item.files} Dateien`}</small
                  >{/if}</td
              >
              <td class="move-partner" title={item.partner}>{item.partner || "—"}</td>
              <td class="move-amount">{money(item.gross, item.currency)}</td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
    {#if preview.items.length > shown.length}
      <p class="move-note">
        Gezeigt werden die ersten {shown.length} von {preview.items.length} Dokumenten.
      </p>
    {/if}
  {:else}
    <p class="move-note">In der Auswahl gibt es keine PDF-, XML- oder Bilddateien.</p>
  {/if}
  {#if preview.tables.length}
    <ul class="move-tables" aria-label="Kunden im Export">
      {#each preview.tables as table (table.origin + table.table)}
        <li>
          <strong>{table.label}</strong>: {table.rows === 1 ? "1 Kunde" : `${table.rows} Kunden`}.
          Nach dem Übernehmen kannst du sie mit „Kunden aus dem Export prüfen“ ansehen und
          übernehmen.
        </li>
      {/each}
    </ul>
  {/if}
  {#if preview.skipped.length}
    <button type="button" class="text move-toggle" onclick={() => (showSkipped = !showSkipped)}
      >{showSkipped ? "Übersprungene ausblenden" : "Übersprungene Dateien zeigen"}</button
    >
    {#if showSkipped}
      <ul class="move-skipped">
        {#each preview.skipped.slice(0, 200) as skipped}
          <li>
            <span title={skipped.origin}>{skipped.origin}</span><small>{skipped.reason}</small>
          </li>
        {/each}
      </ul>
    {/if}
  {/if}
  <p class="move-note">
    Nummer, Datum und Betrag liest die App aus E-Rechnungen (ZUGFeRD, XRechnung) und aus den Daten
    eines DATEV-Exports. Einfache PDFs und Fotos übernimmt sie als Datei, ohne etwas zu raten.
    Exporte für Steuerberatung und Betriebsprüfung legt sie als Ganzes ab.
  </p>

  {#snippet footer()}
    <button type="button" class="secondary" disabled={busy} onclick={onclose}>Abbrechen</button>
    <button type="submit" class="primary" disabled={busy || preview.new === 0}
      >{busy
        ? "Wird übernommen …"
        : preview.new === 1
          ? "1 Dokument übernehmen"
          : `${preview.new} Dokumente übernehmen`}</button
    >
  {/snippet}
</FormDialog>

<style>
  .move-counts {
    display: flex;
    flex-wrap: wrap;
    gap: 8px 18px;
    margin: 0 0 14px;
    padding: 0;
    list-style: none;
    font-size: 13px;
    color: var(--theme-muted);
  }
  .move-counts strong {
    color: var(--theme-text);
  }
  .move-table-wrap {
    max-height: 320px;
    overflow: auto;
    overscroll-behavior: contain;
    border: 1px solid var(--theme-border);
    border-radius: 8px;
  }
  .move-table {
    width: 100%;
    table-layout: fixed;
    border-collapse: collapse;
    font-size: 12px;
  }
  .move-table th:nth-child(1) {
    width: 40%;
  }
  .move-table th:nth-child(2) {
    width: 22%;
  }
  .move-table th:nth-child(4) {
    width: 18%;
  }
  .move-table th {
    position: sticky;
    top: 0;
    background: var(--theme-surface);
    text-align: left;
    font-weight: 600;
    color: var(--theme-muted);
  }
  .move-table th,
  .move-table td {
    padding: 7px 10px;
    border-bottom: 1px solid var(--theme-border);
    vertical-align: top;
  }
  .move-table td strong,
  .move-table td small {
    display: block;
    max-width: 100%;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .move-table small {
    color: var(--theme-muted);
  }
  .move-partner {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .move-toggle {
    margin-top: 10px;
  }
  .move-amount {
    text-align: right !important;
    white-space: nowrap;
  }
  .move-duplicate {
    color: var(--theme-muted);
  }
  .move-note {
    margin: 10px 0 0;
    color: var(--theme-muted);
    font-size: 12px;
    line-height: 1.5;
  }
  .move-tables {
    margin: 12px 0 0;
    padding: 10px 12px 10px 28px;
    border-radius: 8px;
    background: var(--theme-info-bg);
    color: var(--theme-info-text);
    font-size: 12px;
    line-height: 1.5;
  }
  .move-skipped {
    display: grid;
    gap: 4px;
    max-height: 160px;
    overflow: auto;
    margin: 8px 0 0;
    padding: 0;
    list-style: none;
    font-size: 12px;
  }
  .move-skipped li {
    display: flex;
    flex-wrap: wrap;
    gap: 2px 10px;
    min-width: 0;
  }
  .move-skipped span {
    min-width: 0;
    overflow-wrap: anywhere;
  }
  .move-skipped small {
    color: var(--theme-muted);
  }
</style>
