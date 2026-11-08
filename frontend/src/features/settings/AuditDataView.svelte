<script lang="ts">
  import { Dialogs } from "@wailsio/runtime";
  import * as API from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/app/auditservice";
  import { filterRows, sortRows } from "../../lib/audit";
  import { serviceErrorMessage } from "../../lib/errors";

  let { from, to }: { from: string; to: string } = $props();

  type View = {
    name: string;
    title: string;
    columns: string[];
    numeric: boolean[];
    rows: string[][];
  };
  type Info = { name: string; title: string; rows: number };

  /** Rows drawn at once; the search and the CSV file cover all of them. */
  const shownRows = 300;

  let tables = $state<Info[]>([]);
  let selected = $state("");
  let view = $state<View | null>(null);
  let query = $state("");
  let sortColumn = $state(-1);
  let ascending = $state(true);
  let error = $state("");
  let message = $state("");

  const rows = $derived.by(() => {
    if (!view) return [];
    const found = filterRows(view.rows, query);
    return sortColumn < 0
      ? found
      : sortRows(found, sortColumn, view.numeric[sortColumn] ?? false, ascending);
  });

  async function loadTables() {
    error = "";
    try {
      tables = ((await API.Tables(from, to)) ?? []) as Info[];
      if (!tables.some((t) => t.name === selected)) selected = tables[0]?.name ?? "";
      await loadTable();
    } catch (cause) {
      error = serviceErrorMessage(cause, "Die Daten ließen sich nicht laden.");
    }
  }

  async function loadTable() {
    if (!selected) return;
    error = "";
    sortColumn = -1;
    try {
      view = (await API.Table(selected, from, to)) as unknown as View;
    } catch (cause) {
      error = serviceErrorMessage(cause, "Die Tabelle ließ sich nicht laden.");
    }
  }

  function sortBy(column: number) {
    ascending = sortColumn === column ? !ascending : true;
    sortColumn = column;
  }

  async function saveCSV() {
    error = "";
    try {
      const path = await Dialogs.SaveFile({
        Title: "Auswertung speichern",
        Filename: `${selected}_${from}_${to}.csv`,
      });
      if (!path) return;
      const saved = await API.SaveTable(path, selected, from, to, query);
      message = `${saved} Zeilen gespeichert.`;
    } catch (cause) {
      error = serviceErrorMessage(cause, "Die Datei ließ sich nicht speichern.");
    }
  }

  $effect(() => {
    if (from && to) void loadTables();
  });
</script>

<div class="audit-data">
  <div class="audit-data-tools">
    <label
      >Tabelle<select bind:value={selected} onchange={loadTable}>
        {#each tables as t (t.name)}<option value={t.name}>{t.title} ({t.rows})</option>{/each}
      </select></label
    >
    <label>Suchen<input bind:value={query} placeholder="in allen Spalten" type="search" /></label>
    <button type="button" class="secondary" disabled={!view} onclick={saveCSV}
      >Als CSV speichern</button
    >
  </div>
  {#if error}<p class="audit-error" role="alert">{error}</p>{/if}
  {#if message}<p class="settings-help" role="status">{message}</p>{/if}
  {#if view}
    <p class="settings-help">
      {rows.length}
      {rows.length === 1 ? "Zeile" : "Zeilen"}{rows.length > shownRows
        ? `, die ersten ${shownRows} werden angezeigt`
        : ""}. Ein Klick auf eine Spalte sortiert.
    </p>
    <div class="audit-table">
      <table>
        <thead>
          <tr>
            {#each view.columns as column, index}
              <th
                aria-sort={sortColumn === index ? (ascending ? "ascending" : "descending") : "none"}
                ><button type="button" onclick={() => sortBy(index)}>{column}</button></th
              >
            {/each}
          </tr>
        </thead>
        <tbody>
          {#each rows.slice(0, shownRows) as row}
            <tr>
              {#each view.columns as _, index}
                <td class:numeric={view.numeric[index]} title={row[index] ?? ""}
                  >{row[index] ?? ""}</td
                >
              {/each}
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
  {/if}
</div>

<style>
  .audit-data {
    display: grid;
    gap: 10px;
    min-width: 0;
  }
  .audit-data-tools {
    display: flex;
    flex-wrap: wrap;
    align-items: end;
    gap: 10px;
  }
  .audit-data-tools label {
    display: grid;
    gap: 4px;
    flex: 1 1 200px;
    min-width: 0;
  }
  .audit-error {
    margin: 0;
    color: var(--theme-error-text);
  }
  .audit-table {
    max-height: 420px;
    overflow: auto;
    overscroll-behavior: contain;
    border: 1px solid var(--theme-border);
    border-radius: 8px;
  }
  table {
    border-collapse: collapse;
    font-size: 12px;
    min-width: 100%;
  }
  th {
    position: sticky;
    top: 0;
    background: var(--theme-surface);
    text-align: left;
    white-space: nowrap;
  }
  th button {
    all: unset;
    cursor: pointer;
    display: block;
    padding: 6px 10px;
    font-weight: 600;
  }
  th button:focus-visible {
    outline: 2px solid var(--theme-accent, currentColor);
    outline-offset: -2px;
  }
  td {
    padding: 5px 10px;
    border-top: 1px solid var(--theme-border);
    white-space: nowrap;
    max-width: 320px;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  td.numeric {
    text-align: right;
    font-variant-numeric: tabular-nums;
  }
</style>
