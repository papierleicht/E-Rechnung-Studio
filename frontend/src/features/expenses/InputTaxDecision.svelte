<script lang="ts">
  import * as API from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/app/invoiceservice";
  import type {
    ImportInspection,
    ReceivedInvoice,
  } from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/model/models";
  import { serviceErrorMessage } from "../../lib/errors";
  import { amountLabel } from "../../lib/format";

  let {
    item,
    ondecision,
  }: {
    item: ReceivedInvoice;
    ondecision: (inspection: ImportInspection) => void;
  } = $props();

  const SUGGESTION = "suggestion";
  const status = $derived(item.inspection.inputTax);
  const inEuro = $derived(!item.inspection.currency || item.inspection.currency === "EUR");
  // The tax of a document in another currency is no amount for the Vorsteuer.
  const suggestion = $derived(
    status?.tax && inEuro
      ? `Vorschlag der App: ${amountLabel(status.tax)} ${status.onPayment ? "mit der Zahlung " : ""}einrechnen`
      : "Vorschlag der App: nicht einrechnen",
  );

  let mode = $state(SUGGESTION);
  let amount = $state("");
  let note = $state("");
  let busy = $state(false);
  let error = $state("");
  let saved = $state(false);
  // The choice in force shows as a value; the form opens on "Ändern" or while a
  // receipt still waits for a decision.
  let editing = $state(false);
  const choice = $derived(item.inspection.inputTax?.choice);
  const current = $derived.by(() => {
    switch (choice?.mode) {
      case "full":
        return `Die ganze Steuer: ${amountLabel(item.inspection.totals.tax)}`;
      case "payment":
        return "Mit der Zahlung, jeweils der bezahlte Anteil";
      case "amount":
        return `Eigener Betrag: ${amountLabel(choice.amount ?? "")}`;
      case "none":
        return "Nichts einrechnen";
      default:
        return suggestion;
    }
  });

  // The form follows the open receipt and what is stored for it.
  $effect(() => {
    const choice = item.inspection.inputTax?.choice;
    mode = choice?.mode ?? SUGGESTION;
    amount = (choice?.amount ?? "").replace(".", ",");
    note = choice?.note ?? "";
    error = "";
    editing = !choice && (!status?.allowed || !!status?.reason);
  });

  async function save() {
    busy = true;
    error = "";
    saved = false;
    try {
      ondecision(
        mode === SUGGESTION
          ? await API.ClearInputTaxChoice(item.id)
          : await API.SaveInputTaxChoice({ receivedId: item.id, mode, amount, note }),
      );
      saved = true;
      editing = false;
    } catch (cause) {
      error = serviceErrorMessage(cause, "Die Entscheidung konnte nicht gespeichert werden.");
    } finally {
      busy = false;
    }
  }
</script>

<div class="input-tax-decision">
  <div class="setting-row">
    <span class="setting-label">Vorsteuer</span>
    <span class="setting-value">{current}</span>
    <button
      class="secondary"
      type="button"
      aria-expanded={editing}
      disabled={busy}
      onclick={() => {
        editing = !editing;
        saved = false;
      }}>{editing ? "Schließen" : "Ändern"}</button
    >
  </div>
  {#if saved}<p class="form-help" role="status">
      Gespeichert. Deine Zahlen sind aktualisiert.
    </p>{/if}
  {#if editing}<div class="input-tax-form">
      <p class="form-help">
        Die App rechnet nach deinen Belegen und prüft nicht, ob du die Vorsteuer abziehen darfst.
        Das hängt vom Einzelfall ab. Wenn du unsicher bist, frag deine Steuerberatung.
      </p>
      {#if item.locked}<p class="form-help">{item.locked}</p>{/if}
      <fieldset disabled={busy || !!item.locked}>
        <legend>Was soll in deine Zahlen eingehen?</legend>
        <label><input type="radio" bind:group={mode} value={SUGGESTION} /> {suggestion}</label>
        {#if inEuro}<label
            ><input type="radio" bind:group={mode} value="full" />
            {status?.onPayment
              ? "Die ganze Steuer zum Rechnungsdatum"
              : "Die ganze Steuer der Rechnung"}: {amountLabel(item.inspection.totals.tax)}</label
          >
          {#if !status?.onPayment || mode === "payment"}<label
              ><input type="radio" bind:group={mode} value="payment" /> Erst mit der Zahlung, jeweils
              der bezahlte Anteil</label
            >{/if}{/if}
        <label><input type="radio" bind:group={mode} value="amount" /> Ein eigener Betrag</label>
        {#if mode === "amount"}<label class="field"
            >Vorsteuer in Euro<input
              inputmode="decimal"
              placeholder="0,00"
              bind:value={amount}
            /></label
          >{/if}
        <label><input type="radio" bind:group={mode} value="none" /> Nichts einrechnen</label>
        {#if mode !== SUGGESTION}<label class="field"
            >Deine Begründung (freiwillig)<input maxlength="500" bind:value={note} /></label
          >{/if}
        <button class="secondary" onclick={save}>Entscheidung speichern</button>
      </fieldset>
      {#if error}<p class="input-tax-error" role="alert">{error}</p>{/if}
    </div>{/if}
</div>

<style>
  .input-tax-decision,
  .input-tax-form {
    display: grid;
    gap: 8px;
  }
  fieldset {
    display: grid;
    gap: 8px;
    min-width: 0;
    margin: 8px 0 0;
    padding: 0;
    border: 0;
  }
  legend {
    margin-bottom: 6px;
    padding: 0;
    font-size: 13px;
  }
  fieldset > label:not(.field) {
    display: flex;
    gap: 8px;
    align-items: baseline;
  }
  .field {
    display: flex;
    flex-direction: column;
    gap: 4px;
    margin-left: 22px;
  }
  button {
    justify-self: start;
  }
  .input-tax-error {
    color: var(--theme-error-text);
  }
</style>
