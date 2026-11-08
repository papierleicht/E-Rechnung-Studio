<script lang="ts">
  import { dismissDialog } from "../../lib/dismissDialog";
  import { onMount } from "svelte";
  import Icon from "../../components/Icon.svelte";
  import * as API from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/app/invoiceservice";
  import type {
    DeletedInvoiceResult,
    InvoiceDeletion,
  } from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/app/models";
  import { serviceErrorMessage } from "../../lib/errors";
  import { deletionReason, deletionReasons } from "../../lib/invoiceDeletion";
  import { documentLabel } from "../../lib/tax";
  let {
    invoiceId,
    kind,
    ondeleted,
    oncorrection,
    oncancel,
  }: {
    invoiceId: string;
    kind: string;
    /** editAgain: the user wants a new draft with the same content. */
    ondeleted: (result: DeletedInvoiceResult, editAgain: boolean) => void;
    oncorrection: () => void;
    oncancel: () => void;
  } = $props();
  let dialog: HTMLDialogElement;
  let check = $state<InvoiceDeletion | null>(null),
    reason = $state(""),
    details = $state(""),
    confirmed = $state(false),
    busy = $state(false),
    error = $state("");
  const label = $derived(documentLabel(kind));
  const text = $derived(deletionReason(reason, details));
  onMount(async () => {
    dialog.showModal();
    try {
      check = await API.InvoiceDeletionCheck(invoiceId);
    } catch (e) {
      error = serviceErrorMessage(
        e,
        "Es ließ sich nicht prüfen, ob die Rechnung gelöscht werden darf.",
      );
    }
  });
  async function remove(editAgain: boolean) {
    if (busy || !check?.allowed) return;
    if (!text) {
      error = "Gib an, warum du die Rechnung löschst.";
      return;
    }
    if (!confirmed) {
      error = "Bestätige, dass niemand die Rechnung erhalten hat.";
      return;
    }
    busy = true;
    error = "";
    try {
      ondeleted(await API.DeleteIssuedInvoice(invoiceId, text), editAgain);
    } catch (e) {
      error = serviceErrorMessage(e, "Die Rechnung ließ sich nicht löschen.");
    } finally {
      busy = false;
    }
  }
</script>

<dialog
  use:dismissDialog
  aria-labelledby="delete-invoice-title"
  bind:this={dialog}
  class="invoice-settings-dialog"
  oncancel={(event) => {
    event.preventDefault();
    if (!busy) oncancel();
  }}
>
  <form
    class="modal page-settings delete-invoice-dialog"
    aria-labelledby="delete-invoice-title"
    onsubmit={(event) => {
      event.preventDefault();
      void remove(false);
    }}
  >
    <header>
      <h2 id="delete-invoice-title">
        {#if check && !check.allowed}{label} kann nicht gelöscht werden{:else}{label}
          {check?.number ?? ""} löschen?{/if}
      </h2>
      <button
        type="button"
        aria-label="Schließen"
        class="dialog-close"
        title="Schließen (Esc)"
        disabled={busy}
        onclick={oncancel}><Icon name="close" /></button
      >
    </header>
    <div class="page-settings-body">
      {#if !check && !error}<p role="status" class="form-help">Wird geprüft …</p>{/if}
      {#if check && !check.allowed}
        <p>
          {label}
          {check.number} hat die App schon verlassen oder steckt in deiner Buchhaltung:
        </p>
        <ul class="deletion-blockers">
          {#each check.blockers ?? [] as blocker}<li>{blocker}</li>{/each}
        </ul>
        <p class="form-help">
          Eine Rechnung, die jemand erhalten hat, muss erhalten bleiben (GoBD). Hebe sie stattdessen
          mit einer Stornorechnung auf. Sie verweist auf die ursprüngliche Rechnung und gleicht den
          Betrag aus. Danach kannst du eine neue, richtige Rechnung ausstellen.
        </p>
      {:else if check}
        <p>
          Diese Rechnung hast du noch niemandem geschickt. Deshalb darfst du sie löschen, aber
          nachvollziehbar:
        </p>
        <ul class="deletion-effects">
          <li>
            Sie verschwindet aus deinen Rechnungen und zählt nicht mehr für Umsatz, Steuern und
            DATEV.
          </li>
          <li>
            Die Nummer <strong>{check.number}</strong> wird nie wieder vergeben. Rechnung und Grund stehen
            im Protokoll gelöschter Rechnungen, damit die Lücke erklärbar bleibt.
          </li>
          {#if check.files?.length}<li>
              Die automatisch exportierte Datei im Exportordner wird entfernt. Kopien, die du selbst
              woanders gespeichert hast, löschst du bitte selbst.
            </li>{/if}
        </ul>
        <fieldset class="deletion-reasons">
          <legend>Warum löschst du die Rechnung?</legend>
          {#each deletionReasons as option}
            <label
              ><input type="radio" name="deletion-reason" value={option} bind:group={reason} />
              {option}</label
            >
          {/each}
          <label class="deletion-details"
            >{reason === "Anderer Grund" ? "Grund" : "Details (optional)"}<input
              bind:value={details}
              maxlength="300"
              required={reason === "Anderer Grund"}
              placeholder={reason === "Anderer Grund"
                ? "z. B. Auftrag storniert, bevor die Rechnung rausging"
                : "z. B. falscher Betrag in Position 2"}
            /></label
          >
        </fieldset>
        <label class="deletion-confirm"
          ><input type="checkbox" bind:checked={confirmed} />
          <span
            >Ich habe die Rechnung niemandem geschickt oder übergeben, weder per E-Mail, Post oder
            Portal noch an meine Steuerkanzlei.</span
          ></label
        >
      {/if}
      {#if error}<p class="export-error" role="alert">{error}</p>{/if}
    </div>
    <footer>
      <button type="button" class="secondary" disabled={busy} onclick={oncancel}
        >{check && !check.allowed ? "Schließen" : "Abbrechen"}</button
      >
      {#if check && !check.allowed}
        {#if kind === "invoice"}<button type="button" class="primary" onclick={oncorrection}
            >Stornorechnung erstellen</button
          >{/if}
      {:else if check}
        <button class="danger-text" disabled={busy || !text || !confirmed}
          ><Icon name="trash" size={16} /> Löschen</button
        >
        <button
          type="button"
          class="primary"
          disabled={busy || !text || !confirmed}
          onclick={() => remove(true)}>Löschen und neu bearbeiten</button
        >
      {/if}
    </footer>
  </form>
</dialog>

<style>
  .delete-invoice-dialog .page-settings-body {
    display: grid;
    gap: 12px;
  }
  .delete-invoice-dialog p,
  .delete-invoice-dialog ul {
    margin: 0;
  }
  .delete-invoice-dialog ul {
    display: grid;
    gap: 6px;
    padding-left: 20px;
    line-height: 1.45;
  }
  .deletion-blockers {
    color: var(--theme-error-text);
  }
  .deletion-reasons {
    display: grid;
    gap: 6px;
    margin: 0;
    padding: 12px;
    border: 1px solid var(--theme-border);
    border-radius: var(--ui-control-radius);
  }
  .deletion-reasons legend {
    padding: 0 4px;
    font-weight: 600;
  }
  .deletion-reasons > label:not(.deletion-details),
  .deletion-confirm {
    display: flex;
    align-items: flex-start;
    gap: 8px;
    cursor: pointer;
  }
  .deletion-details {
    display: grid;
    gap: 4px;
    margin-top: 6px;
  }
  .deletion-confirm {
    padding: 10px 12px;
    border-radius: var(--ui-control-radius);
    background: var(--theme-warning-bg);
    color: var(--theme-warning-text);
    line-height: 1.45;
  }
</style>
