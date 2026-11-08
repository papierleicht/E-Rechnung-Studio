<script lang="ts">
  import type { FinanceEntry } from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/app/models";
  import FormDialog from "../../components/FormDialog.svelte";
  import { amountLabel, dateLabel } from "../../lib/format";

  // The entries behind a figure, an open point or a part of "Reicht das Geld?",
  // with room for long texts instead of a scrolling list inside the page.
  let {
    title,
    description = "",
    entries,
    onclose,
  }: {
    title: string;
    description?: string;
    entries: FinanceEntry[];
    onclose: () => void;
  } = $props();
</script>

<FormDialog {title} {description} {onclose}>
  {#if entries.length}
    <ul class="entries">
      {#each entries as entry, i (entry.kind + entry.id + i)}
        <li>
          <span class="when">{entry.date ? dateLabel(entry.date) : ""}</span>
          <span class="what"
            ><span class="label">{entry.label}</span>{#if entry.note}<small>{entry.note}</small
              >{/if}</span
          >
          <span class="value">{entry.amount ? amountLabel(entry.amount) : ""}</span>
        </li>
      {/each}
    </ul>
  {:else}
    <p class="empty">Keine Einträge.</p>
  {/if}
  {#snippet footer()}
    <span class="footer-spacer"></span><button type="button" class="secondary" onclick={onclose}
      >Schließen</button
    >
  {/snippet}
</FormDialog>

<style>
  .entries {
    list-style: none;
    margin: 0;
    padding: 0;
    font-size: 13px;
  }
  .entries li {
    display: grid;
    grid-template-columns: 84px minmax(0, 1fr) auto;
    gap: 12px;
    padding: 8px 0;
    border-top: 1px solid var(--theme-border);
  }
  .entries li:first-child {
    border-top: 0;
  }
  .when {
    color: var(--theme-muted);
    white-space: nowrap;
  }
  .what {
    overflow-wrap: anywhere;
    color: var(--theme-text);
  }
  .what small {
    display: block;
    color: var(--theme-muted);
  }
  .value {
    font-variant-numeric: tabular-nums;
    text-align: right;
    white-space: nowrap;
    color: var(--theme-text);
  }
  .empty {
    margin: 0;
    font-size: 13px;
    color: var(--theme-muted);
  }
</style>
