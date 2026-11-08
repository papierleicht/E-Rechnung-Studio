<script lang="ts">
  import type { InvoiceSummary } from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/model/models";
  import PaymentStatus from "../payments/PaymentStatus.svelte";
  import type { PaymentBalance } from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/store/models";
  import Icon from "../../components/Icon.svelte";
  import { amountLabel, dateLabel, statusLabel } from "../../lib/format";
  let {
    invoices,
    balances = [],
    onopen,
    busy = false,
    actionLabel,
    currentId,
  }: {
    invoices: InvoiceSummary[];
    balances?: PaymentBalance[];
    onopen: (invoice: InvoiceSummary) => void;
    busy?: boolean;
    actionLabel?: string;
    /** The invoice open in the editor: its row resumes it instead of reloading. */
    currentId?: string;
  } = $props();
</script>

<div class="invoice-rows">
  {#each invoices as invoice (invoice.id)}
    <button
      class="invoice-entry"
      class:current={Boolean(currentId) && invoice.id === currentId}
      onclick={() => onopen(invoice)}
      disabled={busy}
      title={actionLabel ??
        (currentId && invoice.id === currentId
          ? invoice.status === "draft"
            ? "Entwurf fortsetzen"
            : "Geöffnete Rechnung anzeigen"
          : invoice.status === "draft"
            ? "Entwurf bearbeiten"
            : "Rechnung öffnen")}
    >
      <span class="invoice-entry-title">
        <strong>{invoice.number || "Entwurf"}</strong>
        <small
          ><span class="entry-customer" title={invoice.customerName || undefined}
            >{invoice.customerName || "Ohne Kunde"}</span
          >{#if invoice.sentAt}{" · "}<span
              class="invoice-entry-sent"
              title={invoice.sentVia === "manual"
                ? "Als verschickt markiert"
                : "Per E-Mail versendet"}>Versendet</span
            >{/if}{#if currentId && invoice.id === currentId}{" · "}<span class="entry-current"
              >Geöffnet</span
            >{/if}</small
        >
      </span>
      <span class="invoice-entry-date">{dateLabel(invoice.issueDate)}</span>
      <strong class="invoice-entry-amount">{amountLabel(invoice.gross)}</strong>
      {#if balances.find((b) => b.id === invoice.id && b.direction === "outgoing")}<PaymentStatus
          balance={balances.find((b) => b.id === invoice.id && b.direction === "outgoing")!}
        />{:else}<span class="badge {invoice.status}">{statusLabel(invoice.status)}</span>{/if}
      <Icon name="next" size={16} />
    </button>
  {/each}
</div>

<style>
  /* A long customer name ends in "…" on its own line (full name on hover);
     the sent marker after it stays visible. */
  .entry-customer {
    display: inline-block;
    max-width: 100%;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    vertical-align: bottom;
  }
</style>
