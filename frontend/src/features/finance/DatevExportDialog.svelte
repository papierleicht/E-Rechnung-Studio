<script lang="ts">
  import SettingSection from "../../components/SettingSection.svelte";
  import { onMount, tick } from "svelte";
  import * as API from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/app/invoiceservice";
  import type {
    DatevExportResult,
    DatevPreview,
  } from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/app/models";
  import type { DatevSettings } from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/store/models";
  import type { Category } from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/taxforms/models";
  import FormDialog from "../../components/FormDialog.svelte";
  import PlusHint from "../../components/PlusHint.svelte";
  import { amountLabel, dateLabel } from "../../lib/format";
  import { serviceErrorMessage } from "../../lib/errors";
  import { revenueFields, severityLabels } from "../../lib/datevExport";

  let { from, to, onclose }: { from: string; to: string; onclose: () => void } = $props();

  let preview = $state<DatevPreview | null>(null);
  let settings = $state<DatevSettings | null>(null);
  let result = $state<DatevExportResult | null>(null);
  let doneSection = $state<HTMLElement>();
  let busy = $state(false),
    error = $state(""),
    notice = $state("");
  let showSettings = $state(false);
  let categories = $state<Category[]>([]);

  const months = Array.from({ length: 12 }, (_, i) =>
    new Date(2026, i, 1).toLocaleDateString("de-DE", { month: "long" }),
  );
  const blocking = $derived((preview?.issues ?? []).filter((i) => i.severity === "blocking"));
  const listed = $derived((preview?.issues ?? []).filter((i) => i.severity !== "blocking"));

  async function load() {
    error = "";
    try {
      const [nextPreview, nextSettings, nextCategories] = await Promise.all([
        API.DatevExportPreview(from, to),
        API.DatevSettings(),
        API.ExpenseCategories(),
      ]);
      categories = nextCategories ?? [];
      preview = nextPreview;
      settings = nextSettings;
      if (!nextSettings.updatedAt) showSettings = true;
    } catch (e) {
      error = serviceErrorMessage(e, "Die Prüfliste konnte nicht erstellt werden.");
    }
  }

  async function run(action: () => Promise<void>, fallback: string) {
    busy = true;
    error = "";
    notice = "";
    try {
      await action();
    } catch (e) {
      error = serviceErrorMessage(e, fallback);
    } finally {
      busy = false;
    }
  }

  function setCategoryAccount(id: string, value: string) {
    if (settings) settings.accounts.categories = { ...settings.accounts.categories, [id]: value };
  }

  const saveSettings = () =>
    run(async () => {
      if (!settings) return;
      settings = await API.SaveDatevSettings(settings);
      notice = "DATEV-Einstellungen gespeichert.";
      await load();
    }, "Die Einstellungen konnten nicht gespeichert werden.");

  const applyDefaults = () =>
    run(async () => {
      if (!settings) return;
      settings.accounts = await API.DatevDefaultAccounts(settings.chart, settings.accountLength);
      notice = "Standardkonten eingesetzt. Prüfe sie und speichere die Einstellungen.";
    }, "Die Standardkonten konnten nicht geladen werden.");

  const exportNow = () =>
    run(async () => {
      result = await API.ExportDatev(from, to);
      preview = result.preview;
      showSettings = false;
      await tick();
      doneSection?.scrollIntoView({ block: "center" });
    }, "Der DATEV-Export hat nicht geklappt. Versuche es noch einmal.");

  const openFolder = () =>
    run(async () => {
      if (result?.csvPath) await API.OpenDatevExportFolder(result.csvPath);
    }, "Der Ordner konnte nicht geöffnet werden.");

  onMount(() => void load());
</script>

<FormDialog
  title="Für die Steuerkanzlei exportieren (DATEV)"
  description={`${dateLabel(from)} bis ${dateLabel(to)} · DATEV-Buchungsstapel (EXTF) mit Prüfliste`}
  {busy}
  {error}
  {onclose}
  onsubmit={exportNow}
>
  {#if notice}<p class="datev-notice" role="status">{notice}</p>{/if}
  {#if !preview || !settings}
    <p class="datev-hint">Prüfliste wird erstellt …</p>
  {:else}
    {#if result}
      <section class="datev-done" aria-label="Export abgeschlossen" bind:this={doneSection}>
        <strong>Export erstellt · {result.preview.bookings} Buchungen</strong>
        <span>{result.csvPath}</span>
        {#if result.zipPath}<span>{result.zipPath}</span>{/if}
        <button type="button" class="secondary" onclick={openFolder}>Ordner öffnen</button>
      </section>
    {:else}
      <p class="datev-summary">
        {preview.bookings} Buchungen aus {preview.invoices} Rechnungen, {preview.creditNotes}
        Gutschriften, {preview.received} Eingangsrechnungen, {preview.expenses} erfassten Ausgaben{#if settings.includePayments}
          {" "}und {preview.payments} Zahlungen{/if}.{#if settings.includeDocuments && preview.documents}
          {" "}{preview.documents} Belege kommen als ZIP dazu.{/if}
        {#if preview.newAccounts}Dafür legt papierleicht {preview.newAccounts} neue Personenkonten an.{/if}
      </p>
    {/if}

    <section class="datev-checklist" aria-label="Prüfliste">
      {#each [...blocking, ...listed] as issue (issue.code)}
        {#if issue.entries?.length}<details class="check-item {issue.severity}">
            <summary
              ><span class="tag">{severityLabels[issue.severity] ?? issue.severity}</span>
              {issue.message}<span class="count">{issue.entries.length}</span></summary
            >
            <ul>
              {#each issue.entries as entry (entry.kind + entry.id)}
                <li>
                  <span>{entry.date ? dateLabel(entry.date) : ""}</span>
                  <span
                    >{entry.label}{#if entry.note}
                      · {entry.note}{/if}</span
                  >
                  <span>{entry.amount ? amountLabel(entry.amount) : ""}</span>
                </li>
              {/each}
            </ul>
          </details>{:else}<p class="check-item {issue.severity}">
            <span class="tag">{severityLabels[issue.severity] ?? issue.severity}</span>
            {issue.message}
          </p>{/if}
      {:else}
        <p class="datev-ok">Keine offenen Punkte. Alle Belege des Zeitraums sind enthalten.</p>
      {/each}
    </section>

    <SettingSection
      title="Einstellungen für die Kanzlei"
      summary={`Beraternummer ${settings.consultantNumber || "fehlt"} · Mandantennummer ${settings.clientNumber || "fehlt"} · ${settings.chart}`}
      bind:open={showSettings}
    >
      <div class="datev-settings">
        <p class="datev-hint">
          <strong>Mit der Kanzlei abstimmen:</strong> Nummern, Kontenrahmen und Konten sind Vorschläge.
          Deine Steuerkanzlei prüft die Kontierung; papierleicht ersetzt keine Steuerberatung.
        </p>
        <div class="form-grid cols-2">
          <label
            >Beraternummer<input
              bind:value={settings.consultantNumber}
              inputmode="numeric"
              maxlength="7"
            /></label
          >
          <label
            >Mandantennummer<input
              bind:value={settings.clientNumber}
              inputmode="numeric"
              maxlength="5"
            /></label
          >
          <label
            >Wirtschaftsjahr beginnt im<select bind:value={settings.fiscalYearStartMonth}>
              {#each months as name, i}<option value={i + 1}>{name}</option>{/each}
            </select></label
          >
          <label
            >Sachkontenlänge<select bind:value={settings.accountLength}>
              {#each [4, 5, 6, 7, 8] as n}<option value={n}>{n} Stellen</option>{/each}
            </select></label
          >
          <label
            >Kontenrahmen<select bind:value={settings.chart}>
              <option value="SKR03">SKR03</option><option value="SKR04">SKR04</option>
            </select></label
          >
          <label
            >Personenkonten<select bind:value={settings.personAccounts}>
              <option value="collective">Sammelkonten</option>
              <option value="individual">Fortlaufend je Kunde/Lieferant</option>
            </select></label
          >
          <label
            >{settings.personAccounts === "individual"
              ? "Erstes Debitorenkonto"
              : "Debitorenkonto"}<input
              bind:value={settings.accounts.debtor}
              inputmode="numeric"
            /></label
          >
          <label
            >{settings.personAccounts === "individual"
              ? "Erstes Kreditorenkonto"
              : "Kreditorenkonto"}<input
              bind:value={settings.accounts.creditor}
              inputmode="numeric"
            /></label
          >
          {#each revenueFields as field (field.key)}
            <label
              >{field.label}<input
                bind:value={settings.accounts.revenue[field.key]}
                inputmode="numeric"
              /></label
            >
          {/each}
          <label
            >Aufwandskonto ohne Kategorie<input
              bind:value={settings.accounts.expense}
              inputmode="numeric"
            /></label
          >
          <label
            >Bankkonto für Zahlungen<input
              bind:value={settings.accounts.bank}
              inputmode="numeric"
            /></label
          >
        </div>
        {#if categories.length}<SettingSection
            title="Konten je Ausgabenkategorie"
            summary={`${Object.values(settings.accounts.categories ?? {}).filter(Boolean).length} von ${categories.length} mit eigenem Konto, sonst das Standardkonto`}
          >
            <div class="form-grid cols-2">
              {#each categories as category (category.id)}
                <label
                  >{category.label}<input
                    value={settings.accounts.categories?.[category.id] ?? ""}
                    oninput={(e) => setCategoryAccount(category.id, e.currentTarget.value)}
                    placeholder="Standardkonto"
                    inputmode="numeric"
                  /></label
                >
              {/each}
            </div>
          </SettingSection>{/if}
        <label
          ><input type="checkbox" bind:checked={settings.includePayments} /> Zahlungen als Buchungen exportieren
          (nur wenn die Kanzlei die Kontoauszüge nicht selbst bucht)</label
        >
        {#if settings.includePayments}<PlusHint
            id="datev-bank-sync"
            topic="fints"
            text="Mit Plus rufst du die Bankumsätze per FinTS ab – ohne CSV-Import vor dem Export."
          />{/if}
        <label
          ><input type="checkbox" bind:checked={settings.includeDocuments} /> Original-PDF/XML als ZIP
          beilegen</label
        >
        <div class="datev-actions">
          <button type="button" class="secondary" onclick={applyDefaults}
            >Standardkonten für {settings.chart} einsetzen</button
          >
          <button type="button" class="secondary" onclick={saveSettings}
            >Einstellungen speichern</button
          >
        </div>
      </div>
    </SettingSection>

    {#if preview.notCovered?.length}
      <div class="detail-list datev-not-covered">
        <details>
          <summary
            ><span class="detail-title">Nicht enthalten</span><span class="detail-state"
              >{preview.notCovered.length}
              {preview.notCovered.length === 1 ? "Punkt" : "Punkte"}</span
            ></summary
          >
          <ul>
            {#each preview.notCovered as item}<li>{item}</li>{/each}
          </ul>
        </details>
      </div>
    {/if}
  {/if}

  {#snippet footer()}
    <span class="footer-spacer"></span><button
      type="button"
      class="secondary"
      disabled={busy}
      onclick={onclose}>{result ? "Schließen" : "Abbrechen"}</button
    >{#if !result}<button class="primary" disabled={busy || !preview?.ready}
        >{busy ? "Wird exportiert …" : "Exportieren"}</button
      >{/if}
  {/snippet}
</FormDialog>

<style>
  .datev-summary,
  .datev-hint,
  .datev-ok,
  .datev-notice {
    margin: 0 0 12px;
    font-size: 13px;
    line-height: 1.5;
  }
  .datev-hint {
    color: var(--theme-muted);
    font-size: 12px;
  }
  .datev-notice {
    color: var(--theme-success-text);
  }
  .datev-ok {
    color: var(--theme-success-text);
  }
  .datev-checklist {
    display: flex;
    flex-direction: column;
    gap: 6px;
    margin-bottom: 16px;
    font-size: 13px;
  }
  .check-item {
    margin: 0;
    padding: 8px 10px;
    border-radius: 8px;
    border: 1px solid var(--theme-border);
  }
  .check-item.blocking {
    background: var(--theme-error-bg);
    color: var(--theme-error-text);
    border-color: transparent;
  }
  .check-item.excluded {
    background: var(--theme-warning-bg);
    color: var(--theme-warning-text);
    border-color: var(--theme-warning-border);
  }
  /* Only items with entries open; a chevron after the count shows it. */
  summary {
    list-style: none;
    cursor: pointer;
  }
  summary::-webkit-details-marker {
    display: none;
  }
  summary::after {
    content: "";
    display: inline-block;
    width: 5px;
    height: 5px;
    margin: 0 2px 2px 8px;
    border: solid currentColor;
    border-width: 0 1.5px 1.5px 0;
    transform: rotate(45deg);
  }
  details[open] > summary::after {
    margin-bottom: -1px;
    transform: rotate(-135deg);
  }
  .tag {
    font-weight: 600;
    margin-right: 4px;
  }
  .count {
    display: inline-block;
    margin-left: 6px;
    padding: 0 6px;
    border-radius: 999px;
    background: var(--theme-surface);
    color: var(--theme-text);
    font-size: 11px;
  }
  .datev-checklist ul,
  .datev-not-covered ul {
    margin: 6px 0 0;
    padding: 0;
    list-style: none;
    color: var(--theme-text);
  }
  .datev-checklist li {
    display: grid;
    grid-template-columns: 80px minmax(0, 1fr) auto;
    gap: 8px;
    padding: 2px 0;
  }
  .datev-checklist li > :nth-child(2) {
    overflow-wrap: anywhere;
  }
  .datev-checklist li > :first-child,
  .datev-checklist li > :last-child {
    white-space: nowrap;
  }
  .datev-checklist summary,
  .datev-not-covered li {
    overflow-wrap: anywhere;
  }
  .datev-not-covered li {
    padding: 2px 0;
    font-size: 12px;
  }
  .datev-not-covered {
    margin-bottom: 12px;
  }
  .datev-settings > label {
    display: flex;
    gap: 8px;
    align-items: flex-start;
    margin-top: 10px;
    font-size: 13px;
  }
  .datev-settings > label > input {
    flex-shrink: 0;
  }
  .datev-actions {
    display: flex;
    flex-wrap: wrap;
    gap: 8px;
    margin-top: 12px;
  }
  .datev-done {
    display: flex;
    flex-direction: column;
    gap: 4px;
    align-items: flex-start;
    padding: 10px 12px;
    margin-bottom: 12px;
    border-radius: 8px;
    background: var(--theme-success-bg);
    color: var(--theme-success-text);
    font-size: 13px;
    overflow-wrap: anywhere;
  }
</style>
