<script lang="ts" generics="Id extends string">
  let {
    tabs,
    active,
    label,
    disabled = false,
    onchange,
  }: {
    tabs: readonly { id: Id; label: string }[];
    active: Id;
    /** Accessible name of the tab list, e.g. "Kundenbelege". */
    label: string;
    disabled?: boolean;
    onchange: (tab: Id) => void;
  } = $props();
  function step(by: number) {
    const index = tabs.findIndex((tab) => tab.id === active);
    onchange(tabs[(index + by + tabs.length) % tabs.length].id);
  }
</script>

<!-- The views of a section, below its tiles: the same switch on every page. -->
<div class="tabs no-print" role="tablist" aria-label={label}>
  {#each tabs as tab (tab.id)}
    <button
      type="button"
      role="tab"
      aria-selected={active === tab.id}
      tabindex={active === tab.id ? 0 : -1}
      {disabled}
      onclick={() => onchange(tab.id)}
      onkeydown={(event) => {
        if (event.key === "ArrowLeft" || event.key === "ArrowRight") {
          event.preventDefault();
          step(event.key === "ArrowLeft" ? -1 : 1);
        }
      }}>{tab.label}</button
    >
  {/each}
</div>

<style>
  /* Many or long tab names wrap to a second row instead of leaving the page. */
  .tabs {
    display: flex;
    flex-wrap: wrap;
    gap: 4px;
    width: fit-content;
    max-width: 100%;
    margin: 0 0 16px;
    padding: 4px;
    border: 1px solid var(--theme-border);
    border-radius: 9px;
    background: var(--theme-surface-muted);
  }
  .tabs button {
    padding: 6px 14px;
    border: 1px solid transparent;
    border-radius: 6px;
    background: transparent;
    color: var(--theme-muted);
    font: inherit;
    font-size: var(--ui-control-font-size);
    font-weight: 600;
    white-space: nowrap;
    cursor: pointer;
  }
  .tabs button[aria-selected="true"] {
    background: var(--theme-surface);
    border-color: var(--theme-border);
    color: var(--theme-accent);
  }
  .tabs button:hover:not(:disabled) {
    color: var(--theme-text);
  }
  .tabs button:focus-visible {
    outline: 2px solid var(--theme-focus);
    outline-offset: 2px;
  }
</style>
