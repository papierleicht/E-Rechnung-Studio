<script lang="ts">
  import { dismissDialog } from "../../lib/dismissDialog";
  import { onMount } from "svelte";
  import Icon from "../../components/Icon.svelte";
  import InvoiceHTML from "./InvoiceHTML.svelte";
  import * as API from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/app/invoiceservice";
  import type { DeletedInvoice } from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/store/models";
  import type {
    Company,
    InvoiceDraft,
  } from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/model/models";
  import { serviceErrorMessage } from "../../lib/errors";
  import { amountLabel, dateLabel } from "../../lib/format";
  import { sentAtLabel } from "../../lib/mail";
  import { documentLabel } from "../../lib/tax";
  let {
    company,
    busy = false,
    ondraft,
    onclose,
  }: {
    company: Company;
    busy?: boolean;
    /** Starts a new draft with the content of a deleted invoice. */
    ondraft: (id: string) => void;
    onclose: () => void;
  } = $props();
  let dialog: HTMLDialogElement;
  let entries = $state<DeletedInvoice[] | null>(null),
    selected = $state<DeletedInvoice | null>(null),
    error = $state("");
  onMount(async () => {
    dialog.showModal();
    try {
      entries = (await API.DeletedInvoices()) ?? [];
    } catch (e) {
      error = serviceErrorMessage(e, "Das Protokoll ließ sich nicht laden.");
    }
  });
  // The archived document comes from the stored XML, never from the editor.
  const placeholder = { status: "issued" } as InvoiceDraft;
</script>

<dialog
  use:dismissDialog
  aria-labelledby="deleted-invoices-title"
  bind:this={dialog}
  class="invoice-settings-dialog deleted-invoices"
  oncancel={(event) => {
    event.preventDefault();
    if (selected) selected = null;
    else onclose();
  }}
>
  <div class="modal page-settings" role="document">
    <header>
      <h2 id="deleted-invoices-title">
        {selected
          ? `${documentLabel(selected.kind)} ${selected.number} · gelöscht`
          : "Gelöschte Rechnungen"}
      </h2>
      <button
        type="button"
        aria-label="Schließen"
        class="dialog-close"
        title="Schließen (Esc)"
        onclick={onclose}><Icon name="close" /></button
      >
    </header>
    <div class="page-settings-body">
      {#if selected}
        <dl class="deleted-facts">
          <div>
            <dt>Ausgestellt</dt>
            <dd>{dateLabel(selected.issueDate)}</dd>
          </div>
          <div>
            <dt>Gelöscht</dt>
            <dd>{sentAtLabel(selected.deletedAt)}</dd>
          </div>
          <div>
            <dt>Betrag</dt>
            <dd>{amountLabel(selected.gross)}</dd>
          </div>
          <div class="wide">
            <dt>Grund</dt>
            <dd>{selected.reason}</dd>
          </div>
        </dl>
        <p class="deleted-banner">
          Ungültig: Diese Rechnung wurde gelöscht, bevor sie verschickt wurde.
        </p>
        <InvoiceHTML
          draft={placeholder}
          {company}
          disabled
          onedit={() => {}}
          load={() => API.DeletedInvoiceHTML(selected!.id)}
          renderKey={selected.id}
        />
      {:else}
        <p class="form-help">
          Ausgestellte Rechnungen, die du vor dem Versand gelöscht hast. Jede bleibt mit Grund und
          Originaldatei erhalten, und ihre Nummer wird nicht wieder vergeben. So kannst du jede
          Lücke in deinen Rechnungsnummern erklären.
        </p>
        {#if entries === null && !error}<p role="status" class="form-help">Wird geladen …</p>{/if}
        {#if entries?.length === 0}<p>Du hast noch keine ausgestellte Rechnung gelöscht.</p>{/if}
        {#if entries?.length}
          <ul class="deleted-list">
            {#each entries as entry (entry.id)}
              <li>
                <button type="button" class="deleted-entry" onclick={() => (selected = entry)}>
                  <span class="deleted-title"
                    ><strong>{entry.number}</strong>
                    <small
                      >{entry.customerName || "Ohne Kunde"} · {dateLabel(entry.issueDate)}</small
                    ></span
                  >
                  <span class="deleted-reason" title={entry.reason}>{entry.reason}</span>
                  <span class="deleted-amount">{amountLabel(entry.gross)}</span>
                  <Icon name="next" size={16} />
                </button>
              </li>
            {/each}
          </ul>
        {/if}
      {/if}
      {#if error}<p class="export-error" role="alert">{error}</p>{/if}
    </div>
    <footer>
      {#if selected}
        <button type="button" class="secondary" onclick={() => (selected = null)}
          ><Icon name="back" size={16} /> Zur Liste</button
        >
        <button type="button" class="primary" disabled={busy} onclick={() => ondraft(selected!.id)}
          >Als neuen Entwurf übernehmen</button
        >
      {:else}
        <button type="button" class="secondary" onclick={onclose}>Schließen</button>
      {/if}
    </footer>
  </div>
</dialog>

<style>
  .deleted-invoices :global(.page-settings) {
    width: min(920px, calc(100vw - 32px));
  }
  .deleted-list {
    display: grid;
    gap: 6px;
    margin: 0;
    padding: 0;
    list-style: none;
  }
  .deleted-entry {
    display: grid;
    grid-template-columns: minmax(0, 1fr) minmax(0, 1.4fr) auto auto;
    align-items: center;
    gap: 12px;
    width: 100%;
    padding: 10px 12px;
    border: 1px solid var(--theme-border);
    border-radius: var(--ui-control-radius);
    background: var(--theme-surface);
    color: var(--theme-text);
    text-align: left;
    cursor: pointer;
  }
  .deleted-entry:hover {
    background: var(--theme-hover);
  }
  .deleted-title {
    display: grid;
    min-width: 0;
  }
  .deleted-title small {
    overflow-wrap: anywhere;
  }
  .deleted-entry small,
  .deleted-reason {
    color: var(--theme-muted);
    font-size: 12px;
  }
  .deleted-reason {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .deleted-amount {
    white-space: nowrap;
    font-variant-numeric: tabular-nums;
    font-weight: 600;
  }
  .deleted-facts {
    display: grid;
    grid-template-columns: repeat(3, minmax(0, 1fr));
    gap: 8px 16px;
    margin: 0 0 12px;
  }
  .deleted-facts .wide {
    grid-column: 1 / -1;
  }
  .deleted-facts dt {
    color: var(--theme-muted);
    font-size: 12px;
  }
  .deleted-facts dd {
    margin: 0;
    overflow-wrap: anywhere;
  }
  .deleted-banner {
    margin: 0 0 12px;
    padding: 8px 12px;
    border-radius: var(--ui-control-radius);
    background: var(--theme-error-bg);
    color: var(--theme-error-text);
    font-weight: 600;
  }
</style>
