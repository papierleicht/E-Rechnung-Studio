<script lang="ts">
  import FormDialog from "../../components/FormDialog.svelte";
  import { amountLabel } from "../../lib/format";
  import { invoiceUnits } from "../../lib/invoiceDraft";
  import type { ListPreview } from "../../lib/migration-api";
  let {
    preview,
    fileName,
    busy = false,
    error = "",
    onremap,
    onconfirm,
    onclose,
  }: {
    preview: ListPreview;
    fileName: string;
    busy?: boolean;
    error?: string;
    onremap: (mapping: Record<string, number>) => void;
    onconfirm: () => void;
    onclose: () => void;
  } = $props();
  const customers = $derived(preview.list === "customers");
  type Row = { row: number; name: string; detail: string; note: string; state: string };
  const rows = $derived<Row[]>(
    customers
      ? preview.customers.map((p) => ({
          row: p.row,
          name: p.item.name,
          detail: [p.item.address.postcode, p.item.address.city].filter(Boolean).join(" "),
          note: p.error || (p.existing ? `Gibt es schon: ${p.existing}` : p.note || ""),
          state: p.error ? "error" : p.existing ? "existing" : "new",
        }))
      : preview.products.map((p) => ({
          row: p.row,
          name: p.item.name,
          detail: p.error
            ? ""
            : `${amountLabel(p.item.unitPrice)} · ${unitName(p.item.unit)} · ${p.item.taxRate} %`,
          note: p.error || (p.existing ? `Gibt es schon: ${p.existing}` : p.note || ""),
          state: p.error ? "error" : p.existing ? "existing" : "new",
        })),
  );
  const shown = $derived(rows.slice(0, 200));
  const existing = $derived(rows.filter((row) => row.state === "existing").length);
  const failing = $derived(rows.filter((row) => row.state === "error").length);
  function unitName(code: string) {
    return invoiceUnits.find(([id]) => id === code)?.[1] ?? code;
  }
  function choose(field: string, value: string) {
    const next = { ...preview.mapping } as Record<string, number>;
    if (value === "") delete next[field];
    else next[field] = Number(value);
    onremap(next);
  }
  const stateLabel: Record<string, string> = {
    new: "Neu",
    existing: "Vorhanden",
    error: "Wird übersprungen",
  };
</script>

<FormDialog
  title={customers ? "Kunden aus Liste übernehmen" : "Artikel aus Liste übernehmen"}
  description={`${fileName} · ${preview.encoding} · getrennt durch ${preview.delimiter}`}
  {busy}
  {error}
  {onclose}
  onsubmit={onconfirm}
>
  <fieldset class="move-mapping">
    <legend>Spalten zuordnen</legend>
    <p class="move-note">
      Die App hat die Spalten nach ihren Namen zugeordnet. Passt etwas nicht, wähle die richtige
      Spalte.
    </p>
    <div class="move-mapping-grid">
      {#each preview.fields as field (field.id)}
        <label
          ><span>{field.label}{field.required ? " *" : ""}</span><select
            value={preview.mapping[field.id] === undefined ? "" : String(preview.mapping[field.id])}
            onchange={(event) => choose(field.id, event.currentTarget.value)}
            ><option value="">— nicht übernehmen —</option
            >{#each preview.columns as column, index}<option value={String(index)}
                >{column || `Spalte ${index + 1}`}</option
              >{/each}</select
          ></label
        >
      {/each}
    </div>
  </fieldset>

  <ul class="move-counts" aria-label="Ergebnis der Prüfung">
    <li><strong>{preview.new}</strong> neu</li>
    <li><strong>{existing}</strong> schon vorhanden</li>
    <li><strong>{failing}</strong> übersprungen</li>
  </ul>
  <!-- svelte-ignore a11y_no_noninteractive_tabindex -->
  <div class="move-table-wrap" tabindex="0" role="region" aria-label="Zeilen der Liste">
    <table class="move-table">
      <thead
        ><tr><th>Zeile</th><th>{customers ? "Name" : "Bezeichnung"}</th><th>Status</th></tr></thead
      >
      <tbody>
        {#each shown as row (row.row)}
          <tr class:move-muted={row.state !== "new"}>
            <td>{row.row}</td>
            <td
              ><strong title={row.name}>{row.name || "—"}</strong>{#if row.detail}<small
                  >{row.detail}</small
                >{/if}</td
            >
            <td
              ><span
                class="move-state"
                class:move-warning={row.state === "error" || (row.state === "new" && !!row.note)}
                >{stateLabel[row.state]}</span
              >{#if row.note}<small class="move-row-note">{row.note}</small>{/if}</td
            >
          </tr>
        {/each}
      </tbody>
    </table>
  </div>
  {#if rows.length > shown.length}
    <p class="move-note">Gezeigt werden die ersten {shown.length} von {rows.length} Zeilen.</p>
  {/if}
  <p class="move-note">
    Was schon da ist, legt die App nicht doppelt an. Hinweise zeigen, wo sie etwas angenommen hat,
    zum Beispiel ein Land oder einen Steuersatz. Das kannst du nach dem Übernehmen im Kunden oder
    Artikel ändern.
  </p>

  {#snippet footer()}
    <button type="button" class="secondary" disabled={busy} onclick={onclose}>Abbrechen</button>
    <button type="submit" class="primary" disabled={busy || preview.new === 0}
      >{busy
        ? "Wird übernommen …"
        : `${preview.new} ${customers ? "Kunden" : "Artikel"} übernehmen`}</button
    >
  {/snippet}
</FormDialog>

<style>
  .move-mapping {
    margin: 0 0 14px;
    padding: 0;
    border: 0;
    min-width: 0;
  }
  .move-mapping legend {
    font-weight: 600;
    font-size: 13px;
    padding: 0;
  }
  .move-mapping-grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(min(13rem, 100%), 1fr));
    gap: 8px 12px;
    margin-top: 8px;
  }
  .move-mapping-grid label {
    display: grid;
    gap: 3px;
    min-width: 0;
    font-size: 12px;
  }
  .move-mapping-grid span {
    color: var(--theme-muted);
  }
  .move-mapping-grid select {
    min-width: 0;
    width: 100%;
  }
  .move-counts {
    display: flex;
    flex-wrap: wrap;
    gap: 8px 18px;
    margin: 0 0 10px;
    padding: 0;
    list-style: none;
    font-size: 13px;
    color: var(--theme-muted);
  }
  .move-counts strong {
    color: var(--theme-text);
  }
  .move-table-wrap {
    max-height: 280px;
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
  .move-table th:first-child {
    width: 4rem;
  }
  .move-table th:last-child {
    width: 40%;
  }
  .move-table td:first-child {
    color: var(--theme-muted);
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
  .move-table .move-row-note {
    white-space: normal;
    overflow-wrap: anywhere;
    margin-top: 3px;
  }
  .move-muted {
    color: var(--theme-muted);
  }
  .move-state {
    display: inline-block;
    border-radius: 5px;
    padding: 2px 6px;
    font-size: 10px;
    font-weight: 600;
    background: var(--theme-surface-muted);
  }
  .move-warning {
    color: var(--theme-warning-text);
    background: var(--theme-warning-bg);
  }
  .move-note {
    margin: 10px 0 0;
    color: var(--theme-muted);
    font-size: 12px;
    line-height: 1.5;
  }
</style>
