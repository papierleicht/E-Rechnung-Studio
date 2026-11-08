<script lang="ts">
  import type { Snippet } from "svelte";
  import ArrowDownUpIcon from "@lucide/svelte/icons/arrow-down-up";
  import Icon from "./Icon.svelte";
  import type { BookingOrder } from "../lib/bankAccounts";
  let {
    count,
    query = $bindable(""),
    placeholder = "",
    filter = "",
    onclearfilter = () => {},
    sort,
    onsort = () => {},
    aside,
  }: {
    /** The number of entries shown, e.g. "8 Rechnungen". */
    count: string;
    query?: string;
    /** Also the accessible name of the search field, e.g. "Rechnungen suchen";
        short lists without a placeholder have no search. */
    placeholder?: string;
    /** An active filter takes the place of the count until it is cleared. */
    filter?: string;
    onclearfilter?: () => void;
    /** The order of the list; given, the header offers to switch it. */
    sort?: BookingOrder;
    onsort?: (next: BookingOrder) => void;
    /** Shown on the right instead of a search, e.g. a link to the full list. */
    aside?: Snippet;
  } = $props();
</script>

<!-- Count and search above every list, as on the invoice overview. -->
<div class="section-title list-header">
  <div>
    {#if filter}<button class="filter-chip" onclick={onclearfilter} title="Filter aufheben"
        >{filter} <Icon name="close" size={14} /></button
      >{:else}<span>{count}</span>{/if}
  </div>
  {#if sort}<div class="list-tools">
      <button
        type="button"
        class="secondary sort-button"
        title="Sortierung wechseln"
        onclick={() => onsort(sort === "newest" ? "oldest" : "newest")}
        ><ArrowDownUpIcon size={14} aria-hidden="true" />{sort === "newest"
          ? "Neueste zuerst"
          : "Älteste zuerst"}</button
      >{#if placeholder}<input
          class="invoice-search"
          type="search"
          aria-label={placeholder}
          {placeholder}
          bind:value={query}
        />{/if}
    </div>
  {:else if placeholder}<input
      class="invoice-search"
      type="search"
      aria-label={placeholder}
      {placeholder}
      bind:value={query}
    />{:else if aside}{@render aside()}{/if}
</div>

<style>
  /* Lists start at the same height whether or not they offer a search. A long
     filter (e.g. an account name) pushes the search to the next row, still on the right. */
  .list-header {
    flex-wrap: wrap;
    gap: 8px 16px;
    min-height: 36px;
  }
  .list-header > div {
    min-width: 0;
  }
  .list-header > :global(:last-child:not(:first-child)) {
    margin-left: auto;
  }
  .list-tools {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    justify-content: flex-end;
    gap: 8px;
    min-width: 0;
    max-width: 100%;
  }
  .sort-button {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    padding: 8px 12px;
    white-space: nowrap;
  }
</style>
