<script lang="ts">
  import type { FinanceFigure } from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/app/models";
  import EntriesDialog from "./EntriesDialog.svelte";
  import { amountLabel } from "../../lib/format";

  // plain drops the card frame where the figure sits inside another panel.
  let {
    figure,
    emphasis = false,
    plain = false,
  }: { figure: FinanceFigure; emphasis?: boolean; plain?: boolean } = $props();
  const entries = $derived(figure.entries ?? []);
  // The entries open in a dialog with room for them, instead of a scrolling
  // list inside the card.
  let entriesOpen = $state(false);
  const countLabel = $derived(`${entries.length} ${entries.length === 1 ? "Eintrag" : "Einträge"}`);
</script>

<article class="figure" class:emphasis class:plain class:incomplete={figure.incomplete}>
  <header>
    <h3>{figure.label}</h3>
    {#if figure.incomplete}<span class="flag" title="Es fehlen Daten, siehe offene Punkte"
        >unvollständig</span
      >{/if}
  </header>
  <strong class="amount">{figure.available ? amountLabel(figure.amount) : "—"}</strong>
  <p>{figure.explanation}</p>
  {#if entries.length}
    <button type="button" class="entries-link" onclick={() => (entriesOpen = true)}
      >{countLabel}</button
    >
  {/if}
</article>

{#if entriesOpen}
  <EntriesDialog
    title={figure.label}
    description="{figure.available ? amountLabel(figure.amount) : '—'} · {countLabel}"
    {entries}
    onclose={() => (entriesOpen = false)}
  />
{/if}

<style>
  .figure {
    display: flex;
    flex-direction: column;
    gap: 6px;
    min-width: 0;
    padding: 16px;
    background: var(--theme-surface);
    border: 1px solid var(--theme-border);
    border-radius: 12px;
  }
  .figure.plain {
    padding: 0;
    background: none;
    border: 0;
  }
  .figure.emphasis {
    border-color: var(--theme-accent);
  }
  .figure.incomplete {
    border-style: dashed;
  }
  header {
    display: flex;
    flex-wrap: wrap;
    justify-content: space-between;
    align-items: baseline;
    gap: 4px 8px;
  }
  h3 {
    margin: 0;
    font-size: 13px;
    font-weight: 600;
    color: var(--theme-muted);
  }
  .flag {
    font-size: 11px;
    padding: 1px 7px;
    border-radius: 999px;
    background: var(--theme-warning-bg);
    color: var(--theme-warning-text);
    white-space: nowrap;
  }
  .amount {
    font-size: 22px;
    white-space: nowrap;
    font-variant-numeric: tabular-nums;
    color: var(--theme-text);
  }
  p {
    margin: 0;
    font-size: 12px;
    line-height: 1.45;
    color: var(--theme-muted);
  }
  /* Looks like the "3 Einträge" toggles elsewhere, with a chevron that points
     to the dialog it opens. */
  .entries-link {
    align-self: flex-start;
    display: inline-flex;
    align-items: center;
    gap: 5px;
    margin-top: 4px;
    padding: 0;
    border: 0;
    background: none;
    color: var(--theme-accent);
    font: inherit;
    font-size: 12px;
    font-weight: 600;
    cursor: pointer;
  }
  .entries-link::after {
    content: "";
    width: 5px;
    height: 5px;
    border: solid currentColor;
    border-width: 0 1.5px 1.5px 0;
    transform: rotate(-45deg);
  }
  .entries-link:hover {
    text-decoration: underline;
    text-underline-offset: 3px;
  }
</style>
