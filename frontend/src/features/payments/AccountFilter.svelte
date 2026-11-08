<script lang="ts">
  import type { AccountChoice } from "../../lib/bankAccounts";
  let {
    accounts,
    selected = "",
    counts = {},
    onselect,
  }: {
    accounts: readonly AccountChoice[];
    /** The id of the account the list is limited to; empty shows all. */
    selected?: string;
    /** Entries per account id, shown next to the name. */
    counts?: Record<string, number>;
    onselect: (id: string) => void;
  } = $props();
</script>

<!-- One click limits the list to an account. -->
<div class="account-filter" role="group" aria-label="Nach Konto filtern">
  <button type="button" aria-pressed={!selected} onclick={() => onselect("")}>Alle Konten</button>
  {#each accounts as item (item.id)}
    <button
      type="button"
      aria-pressed={selected === item.id}
      title={item.detail || undefined}
      onclick={() => onselect(item.id)}
      >{item.name}{#if counts[item.id] !== undefined}<span class="count">{counts[item.id]}</span
        >{/if}</button
    >
  {/each}
</div>

<style>
  .account-filter {
    display: flex;
    flex-wrap: wrap;
    gap: 6px;
    margin: 0 0 12px;
  }
  .account-filter button {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    min-width: 0;
    max-width: 100%;
    padding: 4px 12px;
    border: 1px solid var(--theme-border);
    border-radius: 999px;
    background: var(--theme-surface);
    color: var(--theme-text);
    font: inherit;
    font-size: 13px;
    text-align: left;
    overflow-wrap: anywhere;
    cursor: pointer;
  }
  .account-filter button[aria-pressed="true"] {
    border-color: var(--theme-accent);
    background: var(--theme-selected);
    font-weight: 600;
  }
  .account-filter button:focus-visible {
    outline: 2px solid var(--theme-focus);
    outline-offset: 2px;
  }
  .count {
    color: var(--theme-muted);
    font-size: 12px;
    font-weight: 400;
  }
</style>
