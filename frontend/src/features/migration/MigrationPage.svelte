<script lang="ts">
  import { onMount } from "svelte";
  import { Dialogs } from "@wailsio/runtime";
  import FolderInputIcon from "@lucide/svelte/icons/folder-input";
  import Icon from "../../components/Icon.svelte";
  import Tabs from "../../components/Tabs.svelte";
  import GuideView from "./GuideView.svelte";
  import FileImportDialog from "./FileImportDialog.svelte";
  import ListImportDialog from "./ListImportDialog.svelte";
  import ArchiveView from "./ArchiveView.svelte";
  import { serviceErrorMessage } from "../../lib/errors";
  import { isFileDialogCancellation } from "../../lib/fileDialog";
  import { dateLabel } from "../../lib/format";
  import {
    daysLeft,
    migration,
    type FilePreview,
    type LegacyYear,
    type ListPreview,
    type MoveList,
    type Source,
  } from "../../lib/migration-api";

  let {
    initialSource = "",
    onarchive = () => {},
    onchanged,
    onbusychange,
  }: {
    /** The program chosen in the setup, opened first. */
    initialSource?: string;
    /** Reports how many documents the legacy archive holds. */
    onarchive?: (total: number) => void;
    /** Customers and articles changed: the lists elsewhere reload. */
    onchanged: () => Promise<void>;
    onbusychange: (value: boolean) => void;
  } = $props();

  const tabs = [
    { id: "move", label: "Umzug" },
    { id: "archive", label: "Altbestand" },
  ] as const;
  let tab = $state<"move" | "archive">("move");
  let sources = $state<Source[]>([]);
  let years = $state<LegacyYear[]>([]);
  let total = $state(0);
  let selectedId = $state("");
  let loading = $state(true);
  let busy = $state(false);
  let error = $state("");
  let notice = $state("");
  let archiveVersion = $state(0);

  let filePaths = $state<string[]>([]);
  let filePreview = $state<FilePreview | null>(null);
  let listPath = $state("");
  let listPreview = $state<ListPreview | null>(null);
  let dialogError = $state("");

  const selected = $derived(sources.find((source) => source.id === selectedId) ?? null);

  $effect(() => onbusychange(busy));

  async function load() {
    const overview = await migration.overview();
    sources = overview.sources;
    years = overview.years;
    total = overview.total;
    onarchive(total);
    if (!selectedId && sources.length)
      selectedId = sources.some((source) => source.id === initialSource)
        ? initialSource
        : sources[0].id;
  }

  onMount(() => {
    load()
      .catch((cause) => (error = serviceErrorMessage(cause, "Der Umzug ließ sich nicht laden.")))
      .finally(() => (loading = false));
  });

  async function run<T>(work: () => Promise<T>): Promise<T | undefined> {
    busy = true;
    error = "";
    notice = "";
    try {
      return await work();
    } catch (cause) {
      if (!isFileDialogCancellation(cause))
        error = serviceErrorMessage(cause, "Das hat nicht geklappt.");
      return undefined;
    } finally {
      busy = false;
    }
  }

  function asList(value: string | string[] | undefined): string[] {
    if (!value) return [];
    return (Array.isArray(value) ? value : [value]).filter(Boolean);
  }

  async function chooseFiles(directories: boolean) {
    const paths = await run(async () =>
      asList(
        await Dialogs.OpenFile({
          Title: directories ? "Ordner mit Rechnungen wählen" : "Dateien oder ZIP-Export wählen",
          CanChooseFiles: !directories,
          CanChooseDirectories: directories,
          AllowsMultipleSelection: true,
          ...(directories
            ? {}
            : {
                Filters: [
                  {
                    DisplayName: "Rechnungen, Belege und ZIP",
                    Pattern: "*.pdf;*.xml;*.zip;*.png;*.jpg;*.jpeg;*.tif;*.tiff",
                  },
                ],
              }),
        }),
      ),
    );
    if (!paths?.length) return;
    const preview = await run(() => migration.previewFiles(paths));
    if (!preview) return;
    filePaths = paths;
    dialogError = "";
    filePreview = preview;
  }

  async function importFiles() {
    if (!selected) return;
    busy = true;
    dialogError = "";
    try {
      const result = await migration.importFiles(selected.id, filePaths);
      filePreview = null;
      notice =
        `${result.imported === 1 ? "1 Dokument" : `${result.imported} Dokumente`} im Altbestand abgelegt.` +
        (result.duplicates ? ` ${result.duplicates} waren schon da.` : "") +
        (result.failed.length
          ? ` ${result.failed.length} ließen sich nicht übernehmen: ${result.failed
              .slice(0, 3)
              .map((item) => item.origin)
              .join(", ")}${result.failed.length > 3 ? " …" : ""}`
          : "");
      archiveVersion++;
      await load();
    } catch (cause) {
      dialogError = serviceErrorMessage(cause, "Die Dateien ließen sich nicht übernehmen.");
    } finally {
      busy = false;
    }
  }

  async function chooseList(list: MoveList) {
    const path = await run(
      async () =>
        asList(
          await Dialogs.OpenFile({
            Title: list === "customers" ? "Kundenliste wählen" : "Artikelliste wählen",
            CanChooseFiles: true,
            CanChooseDirectories: false,
            AllowsMultipleSelection: false,
            Filters: [{ DisplayName: "CSV- oder Textliste", Pattern: "*.csv;*.txt;*.tsv" }],
          }),
        )[0],
    );
    if (!path) return;
    const preview = await run(() => migration.previewList(list, path, null));
    if (!preview) return;
    listPath = path;
    dialogError = "";
    listPreview = preview;
  }

  async function remap(mapping: Record<string, number>) {
    if (!listPreview) return;
    busy = true;
    dialogError = "";
    try {
      listPreview = await migration.previewList(listPreview.list as MoveList, listPath, mapping);
    } catch (cause) {
      dialogError = serviceErrorMessage(cause, "Die Liste ließ sich nicht neu lesen.");
    } finally {
      busy = false;
    }
  }

  async function importList() {
    if (!listPreview) return;
    const list = listPreview.list as MoveList;
    busy = true;
    dialogError = "";
    try {
      const result = await migration.importList(list, listPath, listPreview.mapping);
      listPreview = null;
      const noun = list === "customers" ? "Kunden" : "Artikel";
      notice =
        `${result.created} ${noun} übernommen.` +
        (result.existing ? ` ${result.existing} gab es schon.` : "") +
        (result.failed.length ? ` ${result.failed.length} Zeilen übersprungen.` : "");
      await onchanged();
    } catch (cause) {
      dialogError = serviceErrorMessage(cause, "Die Liste ließ sich nicht übernehmen.");
    } finally {
      busy = false;
    }
  }

  function fileName(path: string) {
    return path.split(/[\\/]/).pop() ?? path;
  }

  function deadlineText(source: Source) {
    const left = daysLeft(source.deadline);
    if (left === null) return "";
    if (left < 0) return `eingestellt seit ${dateLabel(source.deadline!)}`;
    if (left === 0) return "wird heute eingestellt";
    return `endet am ${dateLabel(source.deadline!)} · noch ${left === 1 ? "1 Tag" : `${left} Tage`}`;
  }
</script>

<header class="topbar">
  <div>
    <h1>Umzug &amp; Altbestand</h1>
  </div>
</header>
<p class="page-intro">
  Bring Rechnungen, Belege, Kunden und Artikel aus deinem bisherigen Programm mit. Der Umzug ist
  kostenlos.
</p>
<Tabs
  {tabs}
  active={tab}
  label="Umzug und Altbestand"
  disabled={busy}
  onchange={(next) => (tab = next)}
/>

<section class="move-workspace" aria-busy={busy || loading}>
  {#if error}<p class="move-feedback move-error" role="alert">{error}</p>{/if}
  {#if notice}<p class="move-feedback move-success" role="status">{notice}</p>{/if}

  {#if tab === "move"}
    {#if loading}
      <p class="move-muted" role="status">Anleitungen werden geladen …</p>
    {:else}
      <div class="move-layout">
        <nav class="move-sources" aria-label="Bisheriges Programm">
          <p class="move-sources-title">Woher kommst du?</p>
          {#each sources as source (source.id)}
            <button
              type="button"
              class="move-source"
              class:active={source.id === selectedId}
              aria-current={source.id === selectedId ? "true" : undefined}
              disabled={busy}
              onclick={() => (selectedId = source.id)}
            >
              <span class="move-source-name">{source.name}</span>
              {#if source.deadline}<span class="move-deadline">{deadlineText(source)}</span
                >{:else}<span class="move-source-summary">{source.summary}</span>{/if}
            </button>
          {/each}
          <p class="move-sources-more">
            Weitere Programme wie Lexware Office, sevDesk und DATEV-Exporte folgen. Bis dahin hilft
            „Anderes Programm“.
          </p>
        </nav>

        {#if selected}
          <article class="move-guide" aria-labelledby="move-guide-title">
            <div class="move-guide-head">
              <div class="move-guide-icon">
                <FolderInputIcon size={22} stroke-width="1.6" aria-hidden="true" />
              </div>
              <div class="move-guide-title">
                <h2 id="move-guide-title">
                  Umzug aus {selected.name} <span class="move-beta">Beta</span>
                </h2>
                <p>{selected.summary}</p>
              </div>
            </div>
            <div class="move-actions" role="group" aria-label="Übernehmen">
              {#if selected.ways.includes("files")}
                <button class="primary" disabled={busy} onclick={() => chooseFiles(true)}
                  ><Icon name="folder" size={16} /> Ordner übernehmen</button
                >
                <button class="secondary" disabled={busy} onclick={() => chooseFiles(false)}
                  >Dateien oder ZIP</button
                >
              {/if}
              {#if selected.ways.includes("customers")}
                <button class="secondary" disabled={busy} onclick={() => chooseList("customers")}
                  >Kunden aus Liste</button
                >
              {/if}
              {#if selected.ways.includes("products")}
                <button class="secondary" disabled={busy} onclick={() => chooseList("products")}
                  >Artikel aus Liste</button
                >
              {/if}
            </div>
            <GuideView text={selected.guide} checked={selected.checked} />
            {#if selected.vendor && selected.id !== "andere"}
              <p class="move-trademark">
                {selected.name} ist eine Marke von {selected.vendor}. papierleicht steht mit dem
                Anbieter in keiner Verbindung.
              </p>
            {/if}
          </article>
        {/if}
      </div>
    {/if}
  {:else}
    <ArchiveView {years} {total} version={archiveVersion} {busy} onbusychange={(v) => (busy = v)} />
  {/if}
</section>

{#if filePreview && selected}
  <FileImportDialog
    preview={filePreview}
    sourceName={selected.name}
    {busy}
    error={dialogError}
    onconfirm={importFiles}
    onclose={() => (filePreview = null)}
  />
{/if}
{#if listPreview}
  <ListImportDialog
    preview={listPreview}
    fileName={fileName(listPath)}
    {busy}
    error={dialogError}
    onremap={remap}
    onconfirm={importList}
    onclose={() => (listPreview = null)}
  />
{/if}

<style>
  .move-workspace {
    display: grid;
    gap: 16px;
    min-width: 0;
  }
  .move-feedback {
    margin: 0;
    padding: 10px 12px;
    border-radius: 8px;
    font-size: 13px;
    line-height: 1.5;
    overflow-wrap: anywhere;
  }
  .move-error {
    background: var(--theme-error-bg);
    color: var(--theme-error-text);
    border: 1px solid var(--theme-error-border);
  }
  .move-success {
    background: var(--theme-success-bg);
    color: var(--theme-success-text);
    border: 1px solid var(--theme-success-border);
  }
  .move-muted {
    color: var(--theme-muted);
    font-size: 13px;
  }
  .move-layout {
    display: grid;
    grid-template-columns: minmax(12rem, 15rem) minmax(0, 1fr);
    gap: 20px;
    align-items: start;
  }
  .move-sources {
    display: grid;
    gap: 6px;
    min-width: 0;
  }
  .move-sources-title {
    margin: 0 0 4px;
    font-size: 12px;
    font-weight: 600;
    color: var(--theme-muted);
  }
  .move-source {
    display: grid;
    gap: 3px;
    width: 100%;
    min-width: 0;
    padding: 10px 12px;
    text-align: left;
    border: 1px solid var(--theme-border);
    border-radius: 9px;
    background: var(--theme-surface);
    color: var(--theme-text);
    cursor: pointer;
  }
  .move-source:hover {
    background: var(--theme-hover);
  }
  .move-source.active {
    border-color: var(--theme-accent);
    box-shadow: inset 3px 0 0 var(--theme-accent);
  }
  .move-source-name {
    font-weight: 600;
    font-size: 13px;
  }
  .move-source-summary,
  .move-deadline {
    font-size: 11px;
    line-height: 1.4;
    color: var(--theme-muted);
    display: -webkit-box;
    -webkit-line-clamp: 2;
    line-clamp: 2;
    -webkit-box-orient: vertical;
    overflow: hidden;
  }
  .move-deadline {
    color: var(--theme-warning-text);
    font-weight: 600;
  }
  .move-sources-more {
    margin: 6px 0 0;
    font-size: 11px;
    line-height: 1.5;
    color: var(--theme-muted);
  }
  .move-guide {
    min-width: 0;
    padding: 20px;
    border: 1px solid var(--theme-border);
    border-radius: 11px;
    background: var(--theme-surface);
  }
  .move-guide-head {
    display: flex;
    gap: 12px;
    align-items: flex-start;
  }
  .move-guide-icon {
    display: grid;
    place-items: center;
    flex-shrink: 0;
    width: 40px;
    height: 40px;
    border-radius: 9px;
    background: var(--theme-surface-muted);
    color: var(--theme-accent);
  }
  .move-guide-title {
    min-width: 0;
  }
  .move-guide-title h2 {
    margin: 0;
    font-size: 16px;
    overflow-wrap: anywhere;
  }
  .move-guide-title p {
    margin: 4px 0 0;
    font-size: 13px;
    color: var(--theme-muted);
    line-height: 1.5;
  }
  .move-beta {
    display: inline-block;
    margin-left: 6px;
    padding: 2px 6px;
    border-radius: 5px;
    font-size: 10px;
    font-weight: 600;
    vertical-align: middle;
    color: var(--theme-info-text);
    background: var(--theme-info-bg);
  }
  .move-actions {
    display: flex;
    flex-wrap: wrap;
    gap: 8px;
    margin: 16px 0 20px;
    padding-bottom: 18px;
    border-bottom: 1px solid var(--theme-border);
  }
  .move-trademark {
    margin: 16px 0 0;
    font-size: 11px;
    color: var(--theme-muted);
  }
  @media (max-width: 1040px) {
    .move-layout {
      grid-template-columns: minmax(0, 1fr);
    }
    .move-sources {
      grid-template-columns: repeat(auto-fill, minmax(min(12rem, 100%), 1fr));
    }
    .move-sources-title,
    .move-sources-more {
      grid-column: 1 / -1;
    }
  }
</style>
