<script lang="ts">
  import { Dialogs } from "@wailsio/runtime";
  import { serviceErrorMessage } from "../../lib/errors";
  import { isFileDialogCancellation } from "../../lib/fileDialog";
  import { dateLabel } from "../../lib/format";
  import { paymentMoney } from "../../lib/paymentWorkflow";
  import {
    directionLabels,
    kindLabel,
    migration,
    type LegacyDocument,
    type LegacyYear,
  } from "../../lib/migration-api";

  let {
    years,
    total,
    version,
    busy,
    onbusychange,
  }: {
    years: LegacyYear[];
    total: number;
    /** Changes after an import, so the list reloads. */
    version: number;
    busy: boolean;
    onbusychange: (value: boolean) => void;
  } = $props();

  let year = $state("");
  let direction = $state("");
  let query = $state("");
  let documents = $state<LegacyDocument[]>([]);
  let loading = $state(false);
  let error = $state("");
  let notice = $state("");
  const yearOptions = $derived(
    [...new Set(years.map((y) => y.year))].filter((y) => /^\d{4}$/.test(y)),
  );
  const shown = $derived(documents.slice(0, 500));

  // Reload when a filter or the archive changes; typing waits a moment.
  $effect(() => {
    const filter = { year, direction, query, source: "" };
    void version;
    const timer = setTimeout(() => {
      loading = true;
      migration
        .archive(filter)
        .then((list) => {
          documents = list;
          error = "";
        })
        .catch(
          (cause) => (error = serviceErrorMessage(cause, "Der Altbestand ließ sich nicht laden.")),
        )
        .finally(() => (loading = false));
    }, 200);
    return () => clearTimeout(timer);
  });

  function money(value: string, currency: string) {
    return value ? paymentMoney(value, currency || "EUR") : "—";
  }

  async function open(doc: LegacyDocument, sum: string) {
    onbusychange(true);
    error = "";
    try {
      await migration.openFile(doc.id, sum);
    } catch (cause) {
      error = serviceErrorMessage(cause, "Die Datei ließ sich nicht öffnen.");
    } finally {
      onbusychange(false);
    }
  }

  // A whole export goes into a new folder inside the one the customer picks.
  async function saveAll(doc: LegacyDocument) {
    onbusychange(true);
    error = "";
    notice = "";
    try {
      const picked = await Dialogs.OpenFile({
        Title: "Ordner für den Export wählen",
        CanChooseFiles: false,
        CanChooseDirectories: true,
        CanCreateDirectories: true,
        AllowsMultipleSelection: false,
      });
      const dir = Array.isArray(picked) ? picked[0] : picked;
      if (dir) notice = `Gespeichert in ${await migration.saveFiles(doc.id, dir)}`;
    } catch (cause) {
      if (!isFileDialogCancellation(cause))
        error = serviceErrorMessage(cause, "Der Export ließ sich nicht speichern.");
    } finally {
      onbusychange(false);
    }
  }

  async function save(doc: LegacyDocument, sum: string, name: string) {
    onbusychange(true);
    error = "";
    try {
      const path = await Dialogs.SaveFile({
        Title: "Kopie speichern",
        Filename: name.split("/").pop() ?? name,
        CanCreateDirectories: true,
      });
      if (path) await migration.saveFile(doc.id, sum, path);
    } catch (cause) {
      if (!isFileDialogCancellation(cause))
        error = serviceErrorMessage(cause, "Die Kopie ließ sich nicht speichern.");
    } finally {
      onbusychange(false);
    }
  }
</script>

<div class="archive">
  <p class="archive-intro">
    Hier liegt, was du aus anderen Programmen mitgebracht hast: unverändert, durchsuchbar und in
    jeder Sicherung. Der Altbestand zählt nicht in Umsatzsteuer, Ergebnis und offene Posten.
  </p>

  {#if total === 0}
    <div class="archive-empty">
      <h2>Noch kein Altbestand</h2>
      <p>Übernimm Rechnungen und Belege im Reiter „Umzug“.</p>
    </div>
  {:else}
    <section aria-labelledby="archive-years-heading">
      <h2 id="archive-years-heading" class="archive-heading">Je Jahr</h2>
      <p class="archive-note">
        Vergleiche Anzahl und Summen mit den Auswertungen deines alten Programms. Summen gibt es nur
        für E-Rechnungen; Gutschriften sind abgezogen.
      </p>
      <!-- svelte-ignore a11y_no_noninteractive_tabindex -->
      <div class="archive-table-wrap" tabindex="0" role="region" aria-label="Altbestand je Jahr">
        <table class="archive-table">
          <thead
            ><tr
              ><th>Jahr</th><th>Art</th><th class="num">Dokumente</th><th class="num">mit Daten</th
              ><th class="num">Netto</th><th class="num">Umsatzsteuer</th><th class="num">Brutto</th
              ></tr
            ></thead
          >
          <tbody>
            {#each years as y (y.year + y.direction)}
              <tr>
                <td>{y.year}</td>
                <td>{directionLabels[y.direction] ?? y.direction}</td>
                <td class="num">{y.count}</td>
                <td class="num">{y.structured}</td>
                <td class="num">{y.structured ? money(y.net, "EUR") : "—"}</td>
                <td class="num">{y.structured ? money(y.tax, "EUR") : "—"}</td>
                <td class="num">{y.structured ? money(y.gross, "EUR") : "—"}</td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    </section>

    <section aria-labelledby="archive-list-heading">
      <h2 id="archive-list-heading" class="archive-heading">
        Dokumente <span>{documents.length}</span>
      </h2>
      <div class="archive-filters">
        <input
          class="invoice-search"
          type="search"
          aria-label="Altbestand durchsuchen"
          placeholder="Nummer, Partner oder Dateiname"
          bind:value={query}
        />
        <label class="archive-select"
          ><span>Jahr</span><select bind:value={year}
            ><option value="">Alle Jahre</option>{#each yearOptions as y}<option value={y}
                >{y}</option
              >{/each}</select
          ></label
        >
        <label class="archive-select"
          ><span>Art</span><select bind:value={direction}
            ><option value="">Alle</option><option value="outgoing">Ausgangsrechnungen</option
            ><option value="incoming">Eingangsrechnungen</option></select
          ></label
        >
      </div>
      {#if error}<p class="archive-error" role="alert">{error}</p>{/if}
      {#if notice}<p class="archive-notice" role="status">{notice}</p>{/if}
      {#if loading && !documents.length}
        <p class="archive-note" role="status">Wird geladen …</p>
      {:else if documents.length}
        <ul class="archive-list" aria-busy={loading}>
          {#each shown as doc (doc.id)}
            <li class="archive-row">
              <div class="archive-main">
                <strong title={doc.origin}>{doc.number || doc.files[0]?.name}</strong>
                <small
                  >{doc.date ? dateLabel(doc.date) : "ohne Datum"} · {kindLabel(
                    doc.kind,
                    doc.basis,
                  )}{doc.basis
                    ? ` · ${directionLabels[doc.direction] ?? doc.direction}`
                    : ""}{doc.kind === "export" ? ` · ${doc.files.length} Dateien` : ""} · aus
                  {doc.source}</small
                >
              </div>
              <span class="archive-partner" title={doc.partner}>{doc.partner || "—"}</span>
              <span class="archive-amount"
                >{doc.basis && doc.gross ? money(doc.gross, doc.currency) : ""}</span
              >
              <div class="archive-actions">
                {#if doc.kind === "export"}
                  <button class="secondary" disabled={busy} onclick={() => saveAll(doc)}
                    >Export speichern</button
                  >
                {:else}
                  {#each doc.files as file (file.sha256)}
                    <button
                      class="secondary"
                      disabled={busy}
                      title={`${file.name} öffnen`}
                      onclick={() => open(doc, file.sha256)}
                      >{doc.files.length > 1
                        ? file.name.split(".").pop()?.toUpperCase()
                        : "Öffnen"}</button
                    >
                  {/each}
                  <button
                    class="text"
                    disabled={busy}
                    onclick={() => save(doc, doc.files[0].sha256, doc.files[0].name)}
                    >Kopie speichern</button
                  >
                {/if}
              </div>
            </li>
          {/each}
        </ul>
        {#if documents.length > shown.length}
          <p class="archive-note">
            Gezeigt werden die ersten {shown.length}. Grenze die Liste mit Jahr oder Suche ein.
          </p>
        {/if}
      {:else}
        <p class="archive-note">Keine Dokumente passen zu dieser Auswahl.</p>
      {/if}
    </section>
  {/if}
</div>

<style>
  .archive {
    display: grid;
    grid-template-columns: minmax(0, 1fr);
    gap: 22px;
    min-width: 0;
  }
  .archive-intro,
  .archive-note {
    margin: 0;
    color: var(--theme-muted);
    font-size: 13px;
    line-height: 1.6;
  }
  .archive-note {
    font-size: 12px;
    margin: 4px 0 10px;
  }
  .archive-heading {
    margin: 0;
    font-size: 14px;
  }
  .archive-heading span {
    color: var(--theme-muted);
    font-weight: 500;
  }
  .archive-empty {
    padding: 24px;
    border: 1px solid var(--theme-border);
    border-radius: 11px;
    background: var(--theme-surface);
  }
  .archive-empty h2,
  .archive-empty p {
    margin: 0;
  }
  .archive-empty p {
    margin-top: 6px;
    color: var(--theme-muted);
    font-size: 13px;
  }
  .archive-table-wrap {
    overflow-x: auto;
    border: 1px solid var(--theme-border);
    border-radius: 9px;
    background: var(--theme-surface);
  }
  .archive-table {
    width: 100%;
    border-collapse: collapse;
    font-size: 12px;
  }
  .archive-table th {
    text-align: left;
    color: var(--theme-muted);
    font-weight: 600;
  }
  .archive-table th,
  .archive-table td {
    padding: 8px 12px;
    border-bottom: 1px solid var(--theme-border);
    white-space: nowrap;
  }
  .archive-table tr:last-child td {
    border-bottom: 0;
  }
  .num {
    text-align: right !important;
  }
  .archive-filters {
    display: flex;
    flex-wrap: wrap;
    gap: 10px;
    align-items: end;
    margin: 10px 0 12px;
  }
  .archive-filters input {
    flex: 1 1 16rem;
    min-width: 0;
  }
  .archive-select {
    display: grid;
    gap: 3px;
    font-size: 12px;
    color: var(--theme-muted);
  }
  .archive-error {
    margin: 0 0 10px;
    padding: 10px;
    border-radius: 6px;
    background: var(--theme-error-bg);
    color: var(--theme-error-text);
    font-size: 12px;
  }
  .archive-notice {
    margin: 0 0 10px;
    padding: 10px;
    border-radius: 6px;
    background: var(--theme-success-bg);
    color: var(--theme-success-text);
    font-size: 12px;
    overflow-wrap: anywhere;
  }
  .archive-list {
    margin: 0;
    padding: 0;
    list-style: none;
    border: 1px solid var(--theme-border);
    border-radius: 9px;
    background: var(--theme-surface);
  }
  .archive-row {
    display: grid;
    grid-template-columns: minmax(0, 2fr) minmax(0, 1.4fr) auto auto;
    gap: 6px 16px;
    align-items: center;
    padding: 10px 14px;
    border-bottom: 1px solid var(--theme-border);
  }
  .archive-row:last-child {
    border-bottom: 0;
  }
  .archive-main {
    min-width: 0;
  }
  .archive-main strong,
  .archive-main small,
  .archive-partner {
    display: block;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .archive-main strong {
    font-size: 13px;
  }
  .archive-main small,
  .archive-partner {
    font-size: 12px;
    color: var(--theme-muted);
  }
  .archive-amount {
    font-size: 13px;
    white-space: nowrap;
    text-align: right;
  }
  .archive-actions {
    display: flex;
    flex-wrap: wrap;
    gap: 6px;
    justify-content: flex-end;
  }
  @media (max-width: 1000px) {
    .archive-row {
      grid-template-columns: minmax(0, 1fr) auto;
    }
    .archive-partner {
      grid-column: 1;
    }
    .archive-actions {
      grid-column: 1 / -1;
      justify-content: flex-start;
    }
  }
</style>
