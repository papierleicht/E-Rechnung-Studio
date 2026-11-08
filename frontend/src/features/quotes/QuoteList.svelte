<script lang="ts">
  import type { QuoteSummary } from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/model/models";
  import Icon from "../../components/Icon.svelte";
  import { amountLabel, dateLabel } from "../../lib/format";
  import { quoteBadgeClass, quoteStatusLabel } from "../../lib/quotes";
  let {
    quotes,
    onopen,
    busy = false,
    currentId,
  }: {
    quotes: QuoteSummary[];
    onopen: (quote: QuoteSummary) => void;
    busy?: boolean;
    /** The quote open in the editor: its row resumes it instead of reloading. */
    currentId?: string;
  } = $props();
</script>

<div class="invoice-rows">
  {#each quotes as quote (quote.id)}
    <button
      class="invoice-entry"
      class:current={Boolean(currentId) && quote.id === currentId}
      onclick={() => onopen(quote)}
      disabled={busy}
      title={currentId && quote.id === currentId
        ? quote.status === "draft"
          ? "Entwurf fortsetzen"
          : "Geöffnetes Angebot anzeigen"
        : quote.status === "draft"
          ? "Entwurf bearbeiten"
          : "Angebot öffnen"}
    >
      <span class="invoice-entry-title">
        <strong>{quote.status === "draft" ? "Entwurf" : quote.number}</strong>
        <small
          ><span class="entry-customer" title={quote.customerName || undefined}
            >{quote.customerName || "Ohne Kunde"}</span
          >
          · gültig bis {dateLabel(quote.validUntil)}{#if quote.mailedAt}{" · "}versendet
            {dateLabel(quote.mailedAt.slice(0, 10))}{/if}{#if quote.invoiceNumber}{" · "}Rechnung
            {quote.invoiceNumber}{/if}{#if currentId && quote.id === currentId}{" · "}<span
              class="entry-current">Geöffnet</span
            >{/if}</small
        >
      </span>
      <span class="invoice-entry-date">{dateLabel(quote.issueDate)}</span>
      <strong class="invoice-entry-amount">{amountLabel(quote.gross)}</strong>
      <span class="badge {quoteBadgeClass(quote.status)}">{quoteStatusLabel(quote.status)}</span>
      <Icon name="next" size={16} />
    </button>
  {/each}
</div>

<style>
  /* A long customer name ends in "…" on its own line (full name on hover);
     validity, mail and invoice notes after it stay visible. */
  .entry-customer {
    display: inline-block;
    max-width: 100%;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    vertical-align: bottom;
  }
</style>
