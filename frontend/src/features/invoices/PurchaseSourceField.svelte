<script lang="ts">
  import { dismissDialog } from "../../lib/dismissDialog";
  import Icon from "../../components/Icon.svelte";
  import LinkIcon from "@lucide/svelte/icons/link";
  import * as API from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/app/invoiceservice";
  import type { PurchaseDocument } from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/app/models";
  import { amountLabel, dateLabel } from "../../lib/format";
  import { serviceErrorMessage } from "../../lib/errors";
  import { purchasePriceFrom, otherUses } from "../../lib/marginScheme";

  let {
    source = "",
    label = "",
    quantity = "1",
    /** The invoice number, so its own use of a document is no warning. */
    invoiceNumber = "",
    disabled = false,
    onchange,
  }: {
    source?: string;
    label?: string;
    quantity?: string;
    invoiceNumber?: string;
    disabled?: boolean;
    /** A chosen document with the price per unit it suggests, or null to unlink. */
    onchange: (choice: { source: string; label: string; price: string } | null) => void;
  } = $props();

  let dialog = $state<HTMLDialogElement>();
  let choosing = $state(false),
    documents = $state<PurchaseDocument[] | null>(null),
    query = $state(""),
    error = $state("");
  const shown = $derived(
    (documents ?? []).filter((d) =>
      `${d.label} ${d.number} ${d.date} ${d.gross}`
        .toLowerCase()
        .includes(query.trim().toLowerCase()),
    ),
  );

  async function open() {
    choosing = true;
    error = "";
    await Promise.resolve();
    dialog?.showModal();
    try {
      documents = (await API.PurchaseDocuments()) ?? [];
    } catch (e) {
      error = serviceErrorMessage(e, "Die Belege ließen sich nicht laden.");
      documents = [];
    }
  }
  function close() {
    dialog?.close();
    choosing = false;
    query = "";
  }
  function choose(d: PurchaseDocument) {
    onchange({ source: d.source, label: d.label, price: purchasePriceFrom(d.gross, quantity) });
    close();
  }
</script>

<div class="purchase-source">
  {#if source}
    <span class="purchase-source-label"
      ><LinkIcon size={14} aria-hidden="true" /> Aus {label || "einem Beleg"}</span
    >
    <button type="button" class="text" {disabled} onclick={open}>Ändern</button>
    <button type="button" class="text" {disabled} onclick={() => onchange(null)}>Lösen</button>
  {:else}
    <button type="button" class="text" {disabled} onclick={open}>Aus Beleg übernehmen …</button>
  {/if}
</div>

{#if choosing}
  <dialog
    use:dismissDialog
    bind:this={dialog}
    aria-labelledby="purchase-source-title"
    class="invoice-settings-dialog"
    oncancel={(event) => {
      event.preventDefault();
      close();
    }}
  >
    <div class="modal page-settings purchase-source-dialog">
      <header>
        <h2 id="purchase-source-title">Beleg für den Einkaufspreis</h2>
        <button
          type="button"
          aria-label="Schließen"
          class="dialog-close"
          title="Schließen (Esc)"
          onclick={close}><Icon name="close" /></button
        >
      </header>
      <div class="page-settings-body">
        <p>
          Wähle den Ankaufbeleg, die Eingangsrechnung oder die Quittung, mit der du den Gegenstand
          gekauft hast. Die App übernimmt den Betrag als Einkaufspreis; du kannst ihn danach
          anpassen.
        </p>
        <input
          type="search"
          aria-label="Belege durchsuchen"
          placeholder="Suchen"
          bind:value={query}
        />
        {#if error}<p class="export-error" role="alert">{error}</p>{/if}
        {#if documents === null}
          <p role="status">Belege werden geladen …</p>
        {:else if !shown.length}
          <p>
            {documents.length
              ? "Kein Beleg passt zur Suche."
              : "Noch keine Belege. Erfasse den Ankauf unter „Ausgaben & Belege“ oder importiere die Eingangsrechnung."}
          </p>
        {:else}
          <ul class="purchase-source-list">
            {#each shown as d (d.source)}
              {@const others = otherUses(d.usedBy ?? [], invoiceNumber, d.source === source)}
              <li>
                <button type="button" class:current={d.source === source} onclick={() => choose(d)}>
                  <span class="purchase-source-name">{d.label}</span>
                  <span class="purchase-source-meta"
                    >{d.date ? dateLabel(d.date) : ""}{d.date && d.gross ? " · " : ""}{d.gross
                      ? amountLabel(d.gross)
                      : ""}</span
                  >
                  {#if others.length}<span class="purchase-source-used"
                      >Schon zugeordnet: {others.join(", ")}</span
                    >{/if}
                </button>
              </li>
            {/each}
          </ul>
        {/if}
      </div>
      <footer>
        <button type="button" class="secondary" onclick={close}>Abbrechen</button>
      </footer>
    </div>
  </dialog>
{/if}

<style>
  .purchase-source {
    display: flex;
    flex-wrap: wrap;
    gap: 4px 12px;
    align-items: center;
    min-width: 0;
    font-size: 13px;
  }
  .purchase-source-label {
    display: inline-flex;
    gap: 4px;
    align-items: center;
    min-width: 0;
    overflow-wrap: anywhere;
  }
  .purchase-source-dialog .page-settings-body {
    display: grid;
    gap: 12px;
  }
  .purchase-source-dialog p {
    margin: 0;
    line-height: 1.45;
  }
  .purchase-source-list {
    display: grid;
    gap: 6px;
    max-height: 360px;
    margin: 0;
    padding: 0;
    overflow: auto;
    list-style: none;
  }
  .purchase-source-list button {
    display: grid;
    gap: 2px;
    width: 100%;
    padding: 10px 12px;
    border: 1px solid var(--theme-border);
    border-radius: var(--ui-control-radius);
    background: var(--theme-surface);
    color: inherit;
    text-align: left;
    cursor: pointer;
  }
  .purchase-source-list button:hover,
  .purchase-source-list button.current {
    border-color: var(--theme-focus);
  }
  .purchase-source-name {
    overflow-wrap: anywhere;
  }
  .purchase-source-meta {
    color: var(--theme-muted);
    font-size: 12px;
  }
  .purchase-source-used {
    color: var(--theme-warning-text);
    font-size: 12px;
  }
</style>
