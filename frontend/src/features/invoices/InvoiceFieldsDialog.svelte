<script lang="ts">
  import { dismissDialog } from "../../lib/dismissDialog";
  import { onDestroy, onMount, untrack } from "svelte";
  import Icon from "../../components/Icon.svelte";
  import PaymentQRControls from "./PaymentQRControls.svelte";
  import { clone, dueAfter, paymentDaysBetween } from "../../lib/invoiceDraft";
  import type {
    Company,
    InvoiceDraft,
  } from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/model/models";
  let {
    draft,
    company,
    part,
    kind = "invoice",
    validUntil = "",
    focusField = "",
    onsave,
    ondirtychange,
    oncancel,
    onchoose,
  }: {
    draft: InvoiceDraft;
    company: Company;
    part: string;
    /** Quotes share these dialogs; they name their dates differently and have no due date. */
    kind?: "invoice" | "quote";
    validUntil?: string;
    focusField?: string;
    onsave: (draft: InvoiceDraft, quote: { validUntil: string }) => void;
    ondirtychange: (dirty: boolean) => void;
    oncancel: () => void;
    onchoose: () => void;
  } = $props();
  let dialog: HTMLDialogElement;
  let working: InvoiceDraft = $state(untrack(() => clone(draft)));
  let workingValidUntil = $state(untrack(() => validUntil));
  const initial = untrack(() => JSON.stringify([draft, validUntil]));
  const title = $derived(
    part === "customer"
      ? "Empfänger"
      : part === "dates"
        ? kind === "quote"
          ? "Angebotsdaten"
          : "Rechnungsdaten"
        : part === "qr"
          ? "QR-Code-Einstellungen"
          : "Zahlung und Hinweise",
  );
  onMount(() => {
    dialog.showModal();
    if (focusField) dialog.querySelector<HTMLElement>(`[data-field="${focusField}"]`)?.focus();
  });
  function updateDue() {
    working.dueDate = dueAfter(working.issueDate, working.customer.paymentTermDays ?? undefined);
  }
  function updateTerm() {
    const days = paymentDaysBetween(working.issueDate, working.dueDate);
    if (days !== undefined) working.customer.paymentTermDays = days;
  }
  const canMarkAdvance = $derived(
    kind === "invoice" && !company.smallBusiness && working.kind !== "credit-note",
  );
  const advance = $derived(canMarkAdvance && !!working.advancePayment);
  $effect(() => ondirtychange(JSON.stringify([working, workingValidUntil]) !== initial));
  onDestroy(() => ondirtychange(false));
</script>

<dialog
  use:dismissDialog
  aria-labelledby="invoice-fields-title"
  bind:this={dialog}
  class="invoice-settings-dialog"
  oncancel={(event) => {
    event.preventDefault();
    oncancel();
  }}
>
  <form
    class="modal page-settings"
    onsubmit={(event) => {
      event.preventDefault();
      onsave(working, { validUntil: workingValidUntil });
    }}
    aria-labelledby="invoice-fields-title"
  >
    <header>
      <h2 id="invoice-fields-title">{title}</h2>
      <button
        type="button"
        aria-label="Schließen"
        class="dialog-close"
        title="Schließen (Esc)"
        onclick={oncancel}><Icon name="close" /></button
      >
    </header>
    <div class="page-settings-body">
      <div class="form-grid cols-2">
        {#if part === "customer"}
          <label class="wide-setting"
            >Name<input data-field="customer.name" bind:value={working.customer.name} /></label
          >
          <label
            >Ansprechpartner<input
              data-field="customer.contactName"
              bind:value={working.customer.contactName}
            /></label
          ><label
            >E-Mail<input
              type="email"
              data-field="customer.email"
              bind:value={working.customer.email}
            /></label
          >
          <label class="wide-setting"
            >Straße und Hausnummer<input
              data-field="customer.address.street"
              bind:value={working.customer.address.street}
            /></label
          >
          <label class="wide-setting"
            >Adresszusatz<input
              data-field="customer.address.additional"
              bind:value={working.customer.address.additional}
            /></label
          >
          <label
            >Postleitzahl<input
              data-field="customer.address.postcode"
              bind:value={working.customer.address.postcode}
            /></label
          ><label
            >Ort<input
              data-field="customer.address.city"
              bind:value={working.customer.address.city}
            /></label
          >
          <label
            >Land<input
              data-field="customer.address.country"
              maxlength="2"
              bind:value={working.customer.address.country}
            /></label
          ><label
            >USt-IdNr.<input
              data-field="customer.vatId"
              bind:value={working.customer.vatId}
            /></label
          >
          <button type="button" class="text" onclick={onchoose}>Anderen Kunden auswählen</button>
        {:else if part === "dates"}
          <label
            >{kind === "quote" ? "Angebotsdatum" : "Rechnungsdatum"}<input
              type="date"
              data-field="issueDate"
              bind:value={working.issueDate}
              onchange={() => kind === "invoice" && updateDue()}
            /></label
          >
          {#if kind === "quote"}<label
              >Gültig bis<input
                type="date"
                data-field="validUntil"
                required
                bind:value={workingValidUntil}
              /></label
            >{/if}
          {#if canMarkAdvance}<label class="wide-setting advance-toggle">
              <input
                type="checkbox"
                data-field="advancePayment"
                bind:checked={working.advancePayment}
              />
              <span>Anzahlungsrechnung: Die Leistung steht noch aus</span>
            </label>
            <p class="form-help wide-setting">
              Nur du kannst das festlegen, die App leitet es nicht aus dem Datum ab. Bei
              Soll-Versteuerung rechnet sie die Umsatzsteuer dann dem Zahlungseingang zu, nur für
              den bezahlten Anteil. Bei Ist-Versteuerung ändert sich an der Steuer nichts.
            </p>{/if}
          <label
            >{advance ? "Voraussichtliches Leistungsdatum" : "Leistungsdatum"}<input
              type="date"
              data-field="deliveryDate"
              bind:value={working.deliveryDate}
            /></label
          >
          <label
            >Leistung von<input
              type="date"
              data-field="periodStart"
              bind:value={working.periodStart}
            /></label
          ><label
            >Leistung bis<input
              type="date"
              data-field="periodEnd"
              bind:value={working.periodEnd}
            /></label
          >
          {#if kind === "invoice"}<label
              >Fällig am<input
                type="date"
                data-field="dueDate"
                bind:value={working.dueDate}
                onchange={updateTerm}
              /></label
            >
            <label
              >Lieferland<input
                maxlength="2"
                data-field="deliveryCountry"
                bind:value={working.deliveryCountry}
                placeholder={working.customer.address.country}
              /></label
            >{/if}
          <label class="wide-setting"
            >Kundenreferenz<input
              data-field="buyerReference"
              bind:value={working.buyerReference}
            /></label
          >
        {:else if part === "qr"}
          <div class="wide-setting"><PaymentQRControls bind:draft={working} {company} /></div>
        {:else}
          <label class="wide-setting"
            >Zahlungsbedingungen<textarea
              data-field="paymentTerms"
              bind:value={working.paymentTerms}
              placeholder="Zahlbar innerhalb von 14 Tagen"></textarea></label
          >
          {#if kind === "invoice"}<p class="form-help wide-setting">
              Hast du mit deinem Kunden im Voraus Skonto, Rabatt oder Bonus vereinbart, der noch
              nicht im Preis steckt? Nenne die Vereinbarung hier, zum Beispiel „2 % Skonto bei
              Zahlung innerhalb von 10 Tagen laut Vereinbarung vom 01.03.2026“. Ein allgemeiner
              Hinweis auf die schriftliche Vereinbarung genügt, wenn du sie bei Bedarf schnell
              vorlegen kannst (§ 14 Abs. 4 Satz 1 Nr. 7 UStG).
            </p>
            <details class="form-help wide-setting link-toggle">
              <summary>Anzahlungen und Restrechnung</summary>
              <p>
                Steht die Leistung noch aus, setze unter „Rechnungsdaten“ das Häkchen
                „Anzahlungsrechnung“. Als Leistungsdatum genügt dann in der Regel der
                voraussichtliche Zeitpunkt oder Monat. Was geleistet wird, muss aber schon
                feststehen (Abschnitt 14.8 Abs. 1 und 4 UStAE). Steht der Zahltag schon fest und
                weicht er vom Rechnungsdatum ab, nenne ihn zusätzlich unter „Hinweis“ (§ 14 Abs. 4
                Satz 1 Nr. 6 UStG).
              </p>
              <p>
                Bei Soll-Versteuerung entsteht die Umsatzsteuer einer Anzahlung in der Regel im
                Zeitraum des Zahlungseingangs (§ 13 Abs. 1 Nr. 1 Buchst. a Satz 4 UStG). Die App
                rechnet deshalb nur bezahlte Anteile ein, zum Datum der Zahlung. Unbezahlte
                Anzahlungsrechnungen zeigt sie als offenen Punkt. Das ist eine Rechenhilfe; ob eine
                Zahlung eine Anzahlung ist, entscheidest du.
              </p>
              <p>
                Eine Endrechnung, die Anzahlungen mit ihrer Steuer absetzt, schreibt die App nicht.
                Stattdessen bietet sich eine Restrechnung an: Sie nennt nur das restliche Entgelt
                und die Steuer darauf, nicht die Anzahlungen und deren Steuer. Unter „Hinweis“
                kannst du zusätzlich das Gesamtentgelt ohne Steuer abzüglich der Anzahlungen ohne
                Steuer angeben (Abschnitt 14.8 Abs. 11 UStAE). Ob das zu deinem Fall passt, klärst
                du am besten mit deiner Steuerberatung.
              </p>
            </details>{/if}
          {#if kind === "invoice"}<label class="wide-setting"
              >Zahlungsreferenz/Verwendungszweck<input
                data-field="paymentReference"
                bind:value={working.paymentReference}
              /></label
            >{/if}
          <label class="wide-setting"
            >Hinweis<textarea data-field="notes" bind:value={working.notes}></textarea></label
          >
          {#if !["standard", "zero-rated", "mixed"].includes(working.taxTreatment)}<label
              class="wide-setting"
              >Steuerlicher Befreiungshinweis<textarea
                data-field="taxExemptionReason"
                bind:value={working.taxExemptionReason}></textarea></label
            >{/if}
        {/if}
      </div>
    </div>
    <footer>
      <button type="button" class="secondary" onclick={oncancel}>Abbrechen</button><button
        class="primary">Übernehmen</button
      >
    </footer>
  </form>
</dialog>

<style>
  .advance-toggle {
    display: flex;
    align-items: center;
    gap: 8px;
  }
  .advance-toggle input {
    width: auto;
    margin: 0;
  }
</style>
