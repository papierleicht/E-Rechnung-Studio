<script lang="ts">
  // Compact, searchable provider rows shared by the e-mail and bank setup. The
  // app bundles no third-party logos, so a letter tile and the name identify
  // each entry; rows stay small enough to scan a dozen providers at a glance.
  import Monogram from "./Monogram.svelte";
  import Icon from "./Icon.svelte";
  export interface ProviderEntry {
    id: string;
    name: string;
    detail?: string;
  }
  let {
    entries,
    value,
    label,
    searchable = entries.length > 5,
    onchoose,
  }: {
    entries: ProviderEntry[];
    value: string;
    label: string;
    searchable?: boolean;
    onchoose: (id: string) => void;
  } = $props();
  let search = $state("");
  const filtered = $derived.by(() => {
    const needle = search.trim().toLocaleLowerCase("de");
    if (!needle) return entries;
    return entries.filter((entry) =>
      `${entry.name} ${entry.detail ?? ""}`.toLocaleLowerCase("de").includes(needle),
    );
  });
</script>

<div class="provider-list">
  {#if searchable}
    <label class="provider-search">
      <span class="visually-hidden">{label} suchen</span>
      <Icon name="search" size={16} />
      <input
        type="search"
        bind:value={search}
        placeholder="Anbieter suchen …"
        autocomplete="off"
        aria-label={`${label} suchen`}
      />
    </label>
  {/if}
  <div class="rows" role="radiogroup" aria-label={label}>
    {#each filtered as entry (entry.id)}
      <button
        type="button"
        role="radio"
        aria-checked={value === entry.id}
        class="row"
        class:selected={value === entry.id}
        onclick={() => onchoose(entry.id)}
      >
        <Monogram name={entry.name} size={28} />
        <span class="row-text">
          <strong>{entry.name}</strong>
          {#if entry.detail}<small>{entry.detail}</small>{/if}
        </span>
        {#if value === entry.id}<Icon name="check" size={16} />{/if}
      </button>
    {/each}
    {#if !filtered.length}
      <p class="empty">Kein Anbieter gefunden. Wähle „Eigener Server“.</p>
    {/if}
  </div>
</div>

<style>
  .provider-list {
    display: grid;
    gap: 8px;
  }
  .provider-search {
    display: flex;
    align-items: center;
    gap: 8px;
    max-width: 320px;
    padding: 0 10px;
    border: 1px solid var(--theme-border);
    border-radius: 8px;
    background: var(--theme-surface);
    color: var(--theme-muted);
  }
  .provider-search:focus-within {
    outline: 2px solid var(--theme-accent);
    outline-offset: 1px;
  }
  .provider-search input {
    flex: 1;
    min-width: 0;
    min-height: 34px;
    padding: 0;
    border: 0;
    background: transparent;
    color: var(--theme-text);
    font: inherit;
    font-size: 13px;
  }
  .provider-search input:focus {
    outline: none;
  }
  .visually-hidden {
    position: absolute;
    width: 1px;
    height: 1px;
    overflow: hidden;
    clip-path: inset(50%);
    white-space: nowrap;
  }
  .rows {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(min(230px, 100%), 1fr));
    gap: 6px;
  }
  .row {
    display: flex;
    align-items: center;
    gap: 10px;
    min-width: 0;
    min-height: 40px;
    padding: 5px 10px 5px 6px;
    border: 1px solid var(--theme-border);
    border-radius: 8px;
    background: var(--theme-surface);
    color: var(--theme-text);
    font: inherit;
    text-align: left;
    cursor: pointer;
  }
  .row:hover {
    border-color: var(--theme-accent);
  }
  .row:focus-visible {
    outline: 2px solid var(--theme-accent);
    outline-offset: 2px;
  }
  .row.selected {
    border-color: var(--theme-accent);
    background: var(--theme-surface-muted);
    color: var(--theme-accent);
  }
  .row-text {
    display: flex;
    flex: 1;
    flex-wrap: wrap;
    align-items: baseline;
    gap: 0 6px;
    min-width: 0;
    overflow-wrap: anywhere;
  }
  strong {
    font-size: 13px;
    font-weight: 600;
    color: var(--theme-text);
  }
  small {
    color: var(--theme-muted);
    font-size: 11px;
  }
  .empty {
    grid-column: 1 / -1;
    margin: 6px 0;
    color: var(--theme-muted);
    font-size: 12px;
  }
</style>
