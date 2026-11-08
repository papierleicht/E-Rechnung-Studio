<script lang="ts">
  import type { PeriodKind } from "../../lib/financePeriod";

  // Month, quarter or year. It sits next to the figures it changes, not in the
  // page head: balances and open items are valued today and do not follow it.
  const today = new Date();
  let {
    kind = $bindable<PeriodKind>("quarter"),
    year = $bindable(today.getFullYear()),
    index = $bindable(Math.floor(today.getMonth() / 3)),
  }: { kind?: PeriodKind; year?: number; index?: number } = $props();

  const years = Array.from({ length: 6 }, (_, i) => today.getFullYear() - i);
  const monthNames = Array.from({ length: 12 }, (_, i) =>
    new Date(2026, i, 1).toLocaleDateString("de-DE", { month: "long" }),
  );

  function changeKind(next: PeriodKind) {
    kind = next;
    index =
      next === "month"
        ? today.getMonth()
        : next === "quarter"
          ? Math.floor(today.getMonth() / 3)
          : 0;
  }
</script>

<div class="period" role="group" aria-label="Zeitraum">
  <div class="segmented" aria-label="Zeitraumart">
    {#each [["month", "Monat"], ["quarter", "Quartal"], ["year", "Jahr"]] as [value, label]}
      <button
        type="button"
        class:chosen={kind === value}
        aria-pressed={kind === value}
        onclick={() => changeKind(value as PeriodKind)}>{label}</button
      >
    {/each}
  </div>
  {#if kind === "month"}
    <select bind:value={index} aria-label="Monat">
      {#each monthNames as name, i}<option value={i}>{name}</option>{/each}
    </select>
  {:else if kind === "quarter"}
    <select bind:value={index} aria-label="Quartal">
      {#each [0, 1, 2, 3] as q}<option value={q}>Q{q + 1}</option>{/each}
    </select>
  {/if}
  <select bind:value={year} aria-label="Jahr">
    {#each years as y}<option value={y}>{y}</option>{/each}
  </select>
</div>

<style>
  .period {
    display: flex;
    flex-wrap: wrap;
    gap: 8px;
    align-items: center;
  }
  .segmented {
    display: inline-flex;
    border: 1px solid var(--theme-border);
    border-radius: 8px;
    overflow: hidden;
  }
  /* As tall as the quarter and year selects next to it. */
  .segmented button {
    min-height: 34px;
    border: 0;
    border-radius: 0;
    background: var(--theme-surface);
    color: var(--theme-text);
    padding: 6px 12px;
    font: inherit;
    font-size: 13px;
    cursor: pointer;
  }
  .segmented button + button {
    border-left: 1px solid var(--theme-border);
  }
  .segmented button.chosen {
    background: var(--theme-selected);
    font-weight: 600;
  }
</style>
