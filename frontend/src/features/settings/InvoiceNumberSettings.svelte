<script lang="ts">
  import SettingSection from "../../components/SettingSection.svelte";
  import { onMount } from "svelte";
  import * as API from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/app/invoiceservice";
  import type { NumberSettings } from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/app/models";
  import type { NumberUse } from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/store/models";
  import { cached, remember } from "../../lib/cache";
  import { serviceErrorMessage } from "../../lib/errors";
  import { dateLabel } from "../../lib/format";

  let {
    disabled = false,
    onpendingchange = () => {},
  }: {
    disabled?: boolean;
    /** Hands the page a way to save, so its own button covers unsaved numbers too. */
    onpendingchange?: (save: (() => Promise<void>) | null) => void;
  } = $props();
  let settings = $state<NumberSettings | null>(null),
    series = $state(""),
    next = $state(1),
    preview = $state(""),
    last = $state<NumberUse | null>(null),
    previewError = $state(""),
    saved = $state(""),
    busy = $state(false),
    error = $state(""),
    notice = $state("");
  const dirty = $derived(settings !== null && JSON.stringify([series, next]) !== saved);
  const numberSummary = $derived(
    settings
      ? `Nächste Rechnung: ${settings.next || "—"} · Format ${settings.series}${
          settings.customers?.length
            ? ` · ${settings.customers.length} ${settings.customers.length === 1 ? "Kunde" : "Kunden"} mit eigenem Nummernkreis`
            : ""
        }`
      : "",
  );
  $effect(() => {
    onpendingchange(dirty ? persist : null);
    return () => onpendingchange(null);
  });

  function accept(value: NumberSettings) {
    settings = remember("settings:numbers", value);
    series = value.series;
    next = value.nextValue;
    preview = value.next;
    last = value.last ?? null;
    previewError = "";
    saved = JSON.stringify([series, next]);
  }
  // The stored numbers of the last visit show right away; a reload leaves started edits alone.
  const warm = cached<NumberSettings>("settings:numbers");
  if (warm) accept(warm);
  onMount(async () => {
    try {
      const fresh = await API.InvoiceNumberSettings();
      if (!dirty) accept(fresh);
    } catch (e) {
      error = serviceErrorMessage(e, "Die Rechnungsnummern ließen sich nicht laden.");
    }
  });
  // A changed format starts where that series stands; the count can only go up.
  let timer = 0,
    revision = 0;
  function schedulePreview(formatChanged: boolean) {
    window.clearTimeout(timer);
    timer = window.setTimeout(async () => {
      const current = ++revision;
      try {
        const result = await API.PreviewNumberSeries(series, formatChanged ? 0 : Number(next));
        if (current !== revision) return;
        if (formatChanged) next = result.nextValue;
        preview = result.next;
        last = result.last ?? null;
        previewError = "";
      } catch (e) {
        if (current === revision) {
          preview = "";
          previewError = serviceErrorMessage(e, "Dieses Format ist ungültig.");
        }
      }
    }, 250);
  }
  async function persist() {
    busy = true;
    error = "";
    notice = "";
    try {
      accept(await API.SaveInvoiceNumberSettings(series.trim(), Number(next)));
      notice = "Rechnungsnummern gespeichert.";
    } catch (e) {
      error = serviceErrorMessage(e, "Die Rechnungsnummern ließen sich nicht speichern.");
      throw e;
    } finally {
      busy = false;
    }
  }
  async function save() {
    if (!settings || busy || previewError) return;
    await persist().catch(() => {});
  }
  async function removeCustomer(id: string) {
    busy = true;
    error = "";
    try {
      await API.RemoveCustomerNumberSeries(id);
      accept(await API.InvoiceNumberSettings());
      notice = "Der Kunde nutzt wieder den Standard-Nummernkreis.";
    } catch (e) {
      error = serviceErrorMessage(e, "Der Nummernkreis ließ sich nicht entfernen.");
    } finally {
      busy = false;
    }
  }
</script>

<SettingSection title="Rechnungsnummern" summary={numberSummary}>
  <p class="form-help">
    Jede Rechnung braucht eine fortlaufende Nummer, die es nur einmal gibt. Die App zählt
    automatisch weiter. Platzhalter: <code>{"{JJJJ}"}</code> Jahr, <code>{"{JJ}"}</code> Jahr
    zweistellig,
    <code>{"{MM}"}</code> Monat, <code>{"{NNNN}"}</code> laufende Nummer mit mindestens so vielen Stellen.
    Mit Jahr oder Monat im Format beginnt die Zählung in jedem Zeitraum neu.
  </p>
  {#if settings}
    <div class="form-grid cols-4">
      <label class="span-2"
        >Format<input
          bind:value={series}
          maxlength="40"
          spellcheck="false"
          autocomplete="off"
          aria-invalid={previewError ? "true" : undefined}
          oninput={() => schedulePreview(true)}
        /></label
      >
      <label class="span-1"
        >Nächste Nummer<input
          type="number"
          min={settings.minValue}
          step="1"
          bind:value={next}
          oninput={() => schedulePreview(false)}
        /></label
      >
      <div class="span-1 number-preview" aria-live="polite">
        <span>Nächste Rechnung</span>
        <strong>{preview || "—"}</strong>
      </div>
    </div>
    {#if previewError}<p class="export-error" role="alert">{previewError}</p>{:else}<p
        class="form-help"
      >
        {last
          ? `Zuletzt vergeben: ${last.number}${last.deleted ? " (gelöscht)" : ""} am ${dateLabel(last.issueDate)}. `
          : ""}Du kannst die Zählung erhöhen, etwa um die Nummern aus deinem bisherigen Programm
        fortzusetzen. Zurück auf eine vergebene Nummer geht es nicht.
      </p>{/if}
    {#if settings.customers?.length}
      <h3>Kunden mit eigenem Nummernkreis</h3>
      <ul class="customer-series">
        {#each settings.customers as customer (customer.customerId)}
          <li>
            <span><strong>{customer.name}</strong> <small>{customer.pattern}</small></span>
            <span>Nächste: {customer.next}</span>
            <button
              type="button"
              class="text"
              disabled={disabled || busy}
              onclick={() => removeCustomer(customer.customerId)}>Standard verwenden</button
            >
          </li>
        {/each}
      </ul>
    {:else}
      <p class="form-help">
        Einen eigenen Nummernkreis für einen Kunden legst du in der Rechnung an: Klicke dort auf die
        Rechnungsnummer und wähle „Eigene Nummer“.
      </p>
    {/if}
    <div class="settings-save">
      {#if notice}<span class="number-notice" role="status">{notice}</span>{/if}
      <button
        type="button"
        class="primary"
        disabled={disabled || busy || !dirty || Boolean(previewError)}
        onclick={save}>{busy ? "Wird gespeichert …" : "Rechnungsnummern speichern"}</button
      >
    </div>
  {/if}
  {#if error}<p class="export-error" role="alert">{error}</p>{/if}
</SettingSection>

<style>
  .number-preview {
    display: grid;
    align-content: end;
    gap: 4px;
    padding-bottom: 8px;
  }
  .number-preview span {
    color: var(--theme-muted);
    font-size: 12px;
  }
  .number-preview strong {
    font-variant-numeric: tabular-nums;
    overflow-wrap: anywhere;
  }
  code {
    font-size: 12px;
  }
  .number-notice {
    align-self: center;
    color: var(--theme-success-text);
    font-size: 13px;
  }
  h3 {
    margin: 16px 0 8px;
    font-size: 13px;
  }
  .customer-series {
    display: grid;
    gap: 6px;
    margin: 0 0 12px;
    padding: 0;
    list-style: none;
  }
  .customer-series li {
    display: grid;
    grid-template-columns: minmax(0, 1fr) auto auto;
    align-items: center;
    gap: 12px;
    padding: 8px 12px;
    border: 1px solid var(--theme-border);
    border-radius: var(--ui-control-radius);
  }
  .customer-series li > span:first-child {
    overflow-wrap: anywhere;
  }
  .customer-series li > span:nth-child(2) {
    white-space: nowrap;
  }
  .customer-series small {
    color: var(--theme-muted);
  }
</style>
