<script lang="ts">
  import { dismissDialog } from "../../lib/dismissDialog";
  import { onDestroy, onMount, tick, untrack } from "svelte";
  import Icon from "../../components/Icon.svelte";
  import * as API from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/app/invoiceservice";
  import type { InvoiceDraft } from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/model/models";
  import type {
    InvoiceNumbering,
    NumberCheck,
  } from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/app/models";
  import { serviceErrorMessage } from "../../lib/errors";
  import { dateLabel } from "../../lib/format";
  import { numberChoice, numberSeriesTitle } from "../../lib/invoiceNumber";
  let {
    draft,
    onsave,
    ondirtychange,
    oncancel,
  }: {
    draft: InvoiceDraft;
    /** Receives the number fields; an own series may already have been started. */
    onsave: (value: { number: string; automaticNumber: boolean; numberSeries: string }) => void;
    ondirtychange: (dirty: boolean) => void;
    oncancel: () => void;
  } = $props();
  let dialog: HTMLDialogElement;
  let numbering = $state<InvoiceNumbering | null>(null),
    choice = $state(""),
    own = $state(untrack(() => (draft.automaticNumber ? "" : draft.number))),
    check = $state<NumberCheck | null>(null),
    checking = $state(false),
    continueSeries = $state(false),
    scope = $state<"customer" | "standard">(
      untrack(() => (draft.customerId ? "customer" : "standard")),
    ),
    error = $state(""),
    saving = $state(false);
  let initial = "";
  const manual = $derived(choice === "manual");
  const canContinue = $derived(manual && check?.state === "ok" && Boolean(check.canContinue));
  const suggestions = $derived(
    Array.from(
      new Set(
        [check?.next ?? "", ...(numbering?.options ?? []).map((option) => option.next)].filter(
          (value) => value && value.toLowerCase() !== own.trim().toLowerCase(),
        ),
      ),
    ).slice(0, 4),
  );
  onMount(async () => {
    dialog.showModal();
    try {
      numbering = await API.InvoiceNumberOptions($state.snapshot(draft) as InvoiceDraft);
      choice = numberChoice(numbering);
      initial = JSON.stringify([choice, own, false]);
      if (choice === "manual") await tickCheck();
    } catch (e) {
      error = serviceErrorMessage(e, "Die Nummernkreise ließen sich nicht laden.");
    }
  });
  $effect(() => {
    const current = JSON.stringify([choice, own, continueSeries]);
    ondirtychange(Boolean(numbering) && current !== initial);
  });
  onDestroy(() => ondirtychange(false));
  // Choosing an own number goes straight to its field.
  let ownInput: HTMLInputElement | undefined = $state();
  $effect(() => {
    if (manual && numbering) void tick().then(() => ownInput?.focus());
  });

  // Checks the typed number shortly after typing stops; stale answers are dropped.
  let timer = 0,
    revision = 0;
  function scheduleCheck() {
    window.clearTimeout(timer);
    check = null;
    if (!own.trim()) return;
    checking = true;
    timer = window.setTimeout(tickCheck, 250);
  }
  async function tickCheck() {
    const current = ++revision;
    const value = own.trim();
    if (!value) {
      checking = false;
      return;
    }
    checking = true;
    try {
      const result = await API.CheckInvoiceNumber($state.snapshot(draft) as InvoiceDraft, value);
      if (current !== revision) return;
      check = result;
      if (!result.canContinue) continueSeries = false;
    } catch (e) {
      if (current === revision)
        error = serviceErrorMessage(e, "Die Nummer ließ sich nicht prüfen.");
    } finally {
      if (current === revision) checking = false;
    }
  }
  onDestroy(() => window.clearTimeout(timer));
  function useSuggestion(value: string) {
    own = value;
    choice = "manual";
    void tickCheck();
  }
  async function save() {
    if (saving || !numbering) return;
    error = "";
    if (!manual) {
      const option = numbering.options?.find((item) => item.value === choice);
      onsave({ number: option?.next ?? "", automaticNumber: true, numberSeries: choice });
      return;
    }
    const value = own.trim();
    if (!check || check.number !== value || checking) await tickCheck();
    if (!check || check.state !== "ok") {
      error = check?.message || "Gib eine gültige, freie Rechnungsnummer ein.";
      return;
    }
    if (!(continueSeries && canContinue)) {
      onsave({ number: value, automaticNumber: false, numberSeries: draft.numberSeries ?? "" });
      return;
    }
    saving = true;
    try {
      const customerId = scope === "customer" ? (draft.customerId ?? "") : "";
      await API.StartNumberSeries({
        series: check.series,
        issueDate: draft.issueDate,
        next: check.counter,
        customerId,
      });
      // The started series now yields exactly this number as its next one.
      const numberSeries = customerId || !numbering.customerSeries ? "" : "standard";
      onsave({ number: value, automaticNumber: true, numberSeries });
    } catch (e) {
      error = serviceErrorMessage(e, "Der Nummernkreis ließ sich nicht anlegen.");
    } finally {
      saving = false;
    }
  }
</script>

<dialog
  use:dismissDialog
  aria-labelledby="invoice-number-title"
  bind:this={dialog}
  class="invoice-settings-dialog"
  oncancel={(event) => {
    event.preventDefault();
    oncancel();
  }}
>
  <form
    class="modal page-settings invoice-number-dialog"
    onsubmit={(event) => {
      event.preventDefault();
      void save();
    }}
    aria-labelledby="invoice-number-title"
  >
    <header>
      <h2 id="invoice-number-title">Rechnungsnummer</h2>
      <button
        type="button"
        aria-label="Schließen"
        class="dialog-close"
        title="Schließen (Esc)"
        onclick={oncancel}><Icon name="close" /></button
      >
    </header>
    <div class="page-settings-body">
      <p class="form-help">
        Jede Nummer darf nur einmal vorkommen. Mehrere Nummernkreise, etwa je Kunde, sind erlaubt,
        solange jede Rechnung eindeutig einem Kreis zugeordnet ist. Endgültig vergeben wird die
        Nummer beim Ausstellen.
      </p>
      {#if !numbering && !error}<p role="status" class="form-help">
          Nummernkreise werden geladen …
        </p>{/if}
      {#if numbering}
        <div class="number-choices" role="radiogroup" aria-label="Rechnungsnummer wählen">
          {#each numbering.options ?? [] as option (option.value + option.kind)}
            <label class="number-choice" class:selected={choice === option.value}>
              <input type="radio" name="number-choice" value={option.value} bind:group={choice} />
              <span class="number-choice-text">
                <strong>{numberSeriesTitle(option)}</strong>
                <small
                  >Nummernkreis {option.pattern} · {option.last
                    ? `zuletzt ${option.last.number}${option.last.deleted ? " (gelöscht)" : ""} am ${dateLabel(option.last.issueDate)}`
                    : "noch keine Rechnung"}</small
                >
              </span>
              <span class="number-choice-value">{option.next}</span>
            </label>
          {/each}
          <label class="number-choice" class:selected={manual}>
            <input type="radio" name="number-choice" value="manual" bind:group={choice} />
            <span class="number-choice-text">
              <strong>Eigene Nummer</strong>
              <small
                >Zum Beispiel, um die Nummern aus deinem bisherigen Programm fortzusetzen.</small
              >
            </span>
          </label>
        </div>
        {#if manual}
          <div class="own-number">
            <label for="own-invoice-number">Rechnungsnummer</label>
            <input
              bind:this={ownInput}
              id="own-invoice-number"
              data-field="number"
              bind:value={own}
              maxlength="36"
              autocomplete="off"
              spellcheck="false"
              placeholder={numbering.options?.[0]?.next}
              aria-invalid={check && check.state !== "ok" ? "true" : undefined}
              aria-describedby="own-number-status"
              oninput={scheduleCheck}
            />
            <p id="own-number-status" class="number-status {check?.state ?? ''}" role="status">
              {#if checking}Wird geprüft …{:else if check}<Icon
                  name={check.state === "ok" ? "check" : "close"}
                  size={14}
                />
                {check.message}{:else}Buchstaben ohne Umlaute, Ziffern und „-“, „/“, „_“, „.“, „:“,
                „+“, höchstens 36 Zeichen.{/if}
            </p>
            {#if check && !checking}
              {#each check.hints ?? [] as hint}<p class="number-hint">{hint}</p>{/each}
            {/if}
            {#if suggestions.length}
              <div class="number-suggestions">
                <span>Freie Nummern:</span>
                {#each suggestions as suggestion}<button
                    type="button"
                    class="suggestion"
                    onclick={() => useSuggestion(suggestion)}>{suggestion}</button
                  >{/each}
              </div>
            {/if}
            {#if canContinue && check}
              <div class="continue-series">
                <label class="checkbox-row"
                  ><input type="checkbox" bind:checked={continueSeries} />
                  <span
                    >Ab {check.number} fortlaufend weiterzählen
                    <small>Nummernkreis {check.pattern}</small></span
                  ></label
                >
                {#if continueSeries}
                  <div class="continue-scope" role="radiogroup" aria-label="Weiterzählen für">
                    {#if draft.customerId}<label
                        ><input type="radio" value="customer" bind:group={scope} />
                        <span
                          >Nur für {draft.customer.name || "diesen Kunden"}<small
                            >Eigener Nummernkreis für diesen Kunden. Andere Kunden bleiben beim
                            Standard.</small
                          ></span
                        ></label
                      >{/if}
                    <label
                      ><input type="radio" value="standard" bind:group={scope} />
                      <span
                        >Für alle neuen Rechnungen<small
                          >Wird dein Standard-Nummernkreis. Ändern kannst du ihn in den
                          Einstellungen unter „Rechnung“.</small
                        ></span
                      ></label
                    >
                  </div>
                {/if}
              </div>
            {/if}
          </div>
        {/if}
      {/if}
      {#if error}<p class="export-error" role="alert">{error}</p>{/if}
    </div>
    <footer>
      <button type="button" class="secondary" onclick={oncancel}>Abbrechen</button><button
        class="primary"
        disabled={!numbering || saving || (manual && (checking || check?.state !== "ok"))}
        >{saving ? "Wird übernommen …" : "Übernehmen"}</button
      >
    </footer>
  </form>
</dialog>

<style>
  .invoice-number-dialog .page-settings-body {
    display: grid;
    gap: 14px;
  }
  .number-choices {
    display: grid;
    gap: 6px;
  }
  .number-choice {
    display: grid;
    grid-template-columns: auto minmax(0, 1fr) auto;
    align-items: center;
    gap: 10px;
    padding: 10px 12px;
    border: 1px solid var(--theme-border);
    border-radius: var(--ui-control-radius);
    cursor: pointer;
  }
  .number-choice.selected {
    border-color: var(--theme-accent);
    background: var(--theme-selected);
  }
  .number-choice-text {
    display: grid;
    gap: 2px;
    min-width: 0;
  }
  .number-choice-text small,
  .continue-series small {
    display: block;
    color: var(--theme-muted);
    font-size: 12px;
  }
  .number-choice-value {
    font-variant-numeric: tabular-nums;
    font-weight: 600;
    white-space: nowrap;
  }
  .own-number {
    display: grid;
    gap: 6px;
  }
  .own-number input:not([type]) {
    font-variant-numeric: tabular-nums;
  }
  .number-status {
    display: flex;
    align-items: center;
    gap: 6px;
    margin: 0;
    font-size: 13px;
    color: var(--theme-muted);
  }
  .number-status.ok {
    color: var(--theme-success-text);
  }
  .number-status.invalid,
  .number-status.taken {
    color: var(--theme-error-text);
  }
  .number-hint {
    margin: 0;
    padding: 8px 10px;
    border-radius: var(--ui-control-radius);
    background: var(--theme-info-bg);
    color: var(--theme-info-text);
    font-size: 13px;
    line-height: 1.45;
  }
  .number-suggestions {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 6px;
    font-size: 13px;
    color: var(--theme-muted);
  }
  .suggestion {
    padding: 3px 10px;
    border: 1px solid var(--theme-border);
    border-radius: 999px;
    background: var(--theme-surface);
    color: var(--theme-text);
    font-variant-numeric: tabular-nums;
    cursor: pointer;
  }
  .suggestion:hover {
    background: var(--theme-hover);
  }
  .continue-series {
    display: grid;
    gap: 8px;
    padding-top: 10px;
    border-top: 1px solid var(--theme-border);
  }
  .checkbox-row,
  .continue-scope label {
    display: flex;
    align-items: flex-start;
    gap: 8px;
    cursor: pointer;
  }
  .checkbox-row > span,
  .continue-scope label > span {
    min-width: 0;
    overflow-wrap: anywhere;
  }
  .continue-scope {
    display: grid;
    gap: 8px;
    padding-left: 24px;
  }
</style>
