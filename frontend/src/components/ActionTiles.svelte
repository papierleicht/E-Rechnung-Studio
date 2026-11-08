<script lang="ts">
  import { tick } from "svelte";
  import { cubicOut } from "svelte/easing";
  import { fly } from "svelte/transition";
  import type { ActionTile } from "../lib/actionTiles";
  let {
    tiles,
    label = "Was möchtest du tun?",
    disabled = false,
  }: {
    tiles: ActionTile[];
    label?: string;
    disabled?: boolean;
  } = $props();
  const main = $derived(tiles.find((tile) => tile.primary));
  const others = $derived(tiles.filter((tile) => !tile.primary));
  // Every section uses the same two rows of fixed height, so switching sections
  // only changes the tile contents. A lone tile in a row spans both columns.
  const rows = $derived(Math.max(2, Math.ceil(others.length / 2)));
  const wide = (index: number) =>
    others.length <= 2 || (others.length % 2 === 1 && index === others.length - 1);
  const iconProps = { size: 20, color: "currentColor", "stroke-width": 1.65, "aria-hidden": true };
  const stripIconProps = { ...iconProps, size: 15 };

  // While the tiles are scrolled out of view, they stay at hand as small buttons
  // in the page's top row.
  let grid = $state<HTMLElement>();
  let header = $state<HTMLElement>();
  let revealed = $state(false);
  const reducedMotion =
    typeof matchMedia === "function" && matchMedia("(prefers-reduced-motion: reduce)").matches;

  /** The page head before the tiles; while pages switch, the old one may still be there. */
  function headerBefore(node: HTMLElement, workspace: HTMLElement) {
    for (let el: Element | null = node; el && el !== workspace; el = el.parentElement)
      for (let sib = el.previousElementSibling; sib; sib = sib.previousElementSibling)
        if (sib.matches(":is(.topbar, header:has(h1))")) return sib as HTMLElement;
  }

  $effect(() => {
    const workspace = grid?.closest<HTMLElement>(".app-shell .workspace");
    header = grid && workspace ? headerBefore(grid, workspace) : undefined;
    if (!grid || !workspace || !header) return;
    const observer = new IntersectionObserver(
      ([entry]) => {
        revealed =
          !entry.isIntersecting &&
          !!entry.rootBounds &&
          entry.boundingClientRect.bottom <= entry.rootBounds.top;
      },
      { root: workspace, rootMargin: `-${header.offsetHeight}px 0px 0px 0px` },
    );
    observer.observe(grid);
    return () => observer.disconnect();
  });

  // Where the row is too narrow for every label, buttons show only their icon:
  // tasks from the right first, amounts last, the main task never.
  let strip = $state<HTMLElement>();
  let collapsed = $state(0);
  const collapseOrder = $derived([
    ...others.filter((tile) => !tile.status).reverse(),
    ...others.filter((tile) => tile.status).reverse(),
  ]);
  const iconOnly = (tile: ActionTile) => {
    const index = collapseOrder.indexOf(tile);
    return index >= 0 && index < collapsed;
  };
  // Layout widths, not scrollWidth: the buttons flying in would count as overflow.
  const overflows = (node: HTMLElement) => {
    const set = node.firstElementChild;
    if (!set) return false;
    const buttons = [...set.children] as HTMLElement[];
    const style = getComputedStyle(node);
    const needed =
      buttons.reduce((sum, button) => sum + button.offsetWidth, 0) +
      parseFloat(getComputedStyle(set).columnGap) * Math.max(0, buttons.length - 1) +
      parseFloat(style.paddingLeft) +
      parseFloat(style.paddingRight);
    return needed > node.clientWidth + 0.5;
  };
  async function fit() {
    collapsed = 0;
    await tick();
    while (strip && overflows(strip) && collapsed < collapseOrder.length) {
      collapsed++;
      await tick();
    }
  }
  $effect(() => {
    if (!revealed || !header) return;
    void tiles.map((tile) => tile.label);
    void fit();
    const observer = new ResizeObserver(() => void fit());
    observer.observe(header);
    return () => observer.disconnect();
  });

  /** Moves the strip into the top row, after the title. */
  const intoHeader = (target: HTMLElement) => (node: HTMLElement) => {
    target.append(node);
    return () => node.remove();
  };

  /** The buttons come in from the right, the rightmost first. */
  const slideIn = (node: Element, { index, count }: { index: number; count: number }) =>
    fly(node, {
      x: 16,
      duration: reducedMotion ? 0 : 180,
      delay: reducedMotion ? 0 : (count - 1 - index) * 40,
      easing: cubicOut,
    });
</script>

{#snippet tileButton(tile: ActionTile)}
  <button
    {...tile.attrs}
    type="button"
    class="tile"
    class:primary-tile={tile.primary}
    class:status-tile={tile.status}
    class:alert={tile.alert}
    class:current={tile.current}
    title={tile.title ?? (tile.hint ? `${tile.label}: ${tile.hint}` : undefined)}
    aria-pressed={tile.pressed}
    aria-current={tile.current ? "true" : undefined}
    disabled={disabled || tile.disabled}
    onclick={tile.onclick}
  >
    <span class="tile-icon"><tile.icon {...iconProps} /></span>
    <span class="tile-text"
      ><strong>{tile.label}</strong><small>{tile.hint || "\u00a0"}</small></span
    >
  </button>
{/snippet}

<!-- Entry points for the tasks of a section, so new users see what it offers. -->
{#if header}
  <div class="tile-strip no-print" bind:this={strip} {@attach intoHeader(header)}>
    <!-- The way out plays only when the tiles come back, not when the page closes. -->
    {#if revealed}<div class="strip-set" out:fly={{ x: 16, duration: reducedMotion ? 0 : 120 }}>
        {#each tiles as tile, index (tile.id)}
          <button
            type="button"
            class="strip-button"
            class:primary-strip={tile.primary}
            class:icon-only={iconOnly(tile)}
            class:alert={tile.status && tile.alert}
            class:current={tile.current}
            title={tile.title ?? (tile.hint ? `${tile.label}: ${tile.hint}` : tile.label)}
            aria-pressed={tile.pressed}
            aria-current={tile.current ? "true" : undefined}
            disabled={disabled || tile.disabled}
            onclick={tile.onclick}
            in:slideIn|global={{ index, count: tiles.length }}
          >
            <tile.icon {...stripIconProps} /><span class:visually-hidden={iconOnly(tile)}
              >{tile.label}</span
            >
          </button>
        {/each}
      </div>{/if}
  </div>
{/if}

<div class="action-tiles-host no-print">
  <div
    class="action-tiles"
    bind:this={grid}
    style:--tile-rows={rows}
    role="group"
    aria-label={label}
  >
    {#if main}{@render tileButton(main)}{/if}
    {#each others as tile, index (tile.id)}
      <div class="tile-slot" class:wide={wide(index)}>
        {@render tileButton(tile)}
      </div>
    {/each}
  </div>
</div>

<style>
  /* Right of the title. When the row gets tight the strip gives way first (see
     fit); the tooltip has the full text. */
  :global(.app-shell .workspace :is(.topbar, header):has(> .tile-strip)) {
    flex-wrap: nowrap;
  }
  .tile-strip {
    flex: 0 1000 auto;
    align-self: center;
    display: flex;
    justify-content: flex-end;
    min-width: 0;
    margin: -3px -3px -3px auto;
    padding: 3px;
    overflow: hidden;
  }
  .tile-strip:empty {
    display: none;
  }
  .strip-set {
    display: flex;
    gap: 6px;
  }
  .strip-button {
    flex: none;
    display: inline-flex;
    align-items: center;
    gap: 6px;
    min-width: 32px;
    height: 30px;
    padding: 0 10px;
    overflow: hidden;
    border: 1px solid var(--theme-border);
    border-radius: 7px;
    background: var(--theme-surface);
    color: var(--theme-text);
    font: inherit;
    font-size: 13px;
    white-space: nowrap;
    cursor: pointer;
  }
  .strip-button :global(svg) {
    flex: none;
    color: var(--theme-accent);
  }
  .strip-button.icon-only {
    padding: 0 8px;
  }
  .strip-button span {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .strip-button .visually-hidden {
    position: absolute;
    width: 1px;
    height: 1px;
    overflow: hidden;
    clip-path: inset(50%);
  }
  .strip-button:hover:not(:disabled) {
    border-color: var(--theme-border-strong, var(--theme-border));
    background: var(--theme-hover);
  }
  .strip-button:focus-visible {
    outline: 2px solid var(--theme-focus);
    outline-offset: 1px;
  }
  .strip-button:disabled {
    cursor: default;
    opacity: 0.6;
  }
  .primary-strip {
    border-color: transparent;
    background: var(--theme-primary);
    color: var(--theme-on-primary);
  }
  .primary-strip :global(svg) {
    color: inherit;
  }
  .primary-strip:hover:not(:disabled) {
    border-color: transparent;
    background: var(--theme-primary-hover);
  }
  .strip-button[aria-pressed="true"],
  .strip-button.current:not(.primary-strip) {
    border-color: var(--theme-accent);
    background: var(--theme-selected);
  }
  .strip-button.alert:not(.primary-strip) :global(svg) {
    color: var(--theme-warning-text);
  }
  .action-tiles-host {
    --tile-row: 62px;
    container-type: inline-size;
    margin: 0 0 20px;
  }
  .action-tiles {
    display: grid;
    grid-template-columns: 1.25fr 1fr 1fr;
    grid-template-rows: repeat(var(--tile-rows), var(--tile-row));
    gap: 10px;
  }
  .tile-slot {
    display: grid;
    min-width: 0;
  }
  .tile-slot.wide {
    grid-column: span 2;
  }
  /* One structure for every tile: the icon in front of title and hint, exactly as
     high as both lines together, resting on the bottom edge. */
  .tile {
    display: flex;
    align-items: flex-end;
    gap: 10px;
    min-width: 0;
    min-height: 0;
    overflow: hidden;
    padding: 12px 14px;
    border: 1px solid var(--theme-border);
    border-radius: 12px;
    background: var(--theme-surface);
    color: var(--theme-text);
    font: inherit;
    text-align: left;
    cursor: pointer;
  }
  /* 36px = title line (18px) + gap (2px) + hint line (16px). */
  .tile-icon {
    flex: none;
    display: grid;
    place-items: center;
    width: 36px;
    height: 36px;
    border-radius: 9px;
    background: var(--theme-surface-muted);
    color: var(--theme-accent);
  }
  .tile-text {
    flex: 1;
    min-width: 0;
  }
  /* Title and hint keep one line each, so they sit at the same place in every tile
     and section; the tooltip has the full text. */
  .tile strong,
  .tile small {
    display: block;
    overflow: hidden;
    white-space: nowrap;
    text-overflow: ellipsis;
  }
  /* 600 like section subtitles and buttons; the browser's bold (700) made the
     tiles the heaviest text on every page, above the page and section titles. */
  .tile strong {
    font-size: 14px;
    font-weight: 600;
    line-height: 18px;
  }
  .tile small {
    margin-top: 2px;
    color: var(--theme-muted);
    font-size: 12px;
    line-height: 16px;
  }
  .tile:hover:not(:disabled),
  .tile:global(.file-drop-target-active) {
    border-color: var(--theme-border-strong, var(--theme-border));
    background: var(--theme-hover);
  }
  .tile:global(.file-drop-target-active) {
    outline: 2px solid var(--theme-focus);
    outline-offset: 2px;
  }
  .tile:focus-visible {
    outline: 2px solid var(--theme-focus);
    outline-offset: 2px;
  }
  .tile:disabled {
    cursor: default;
    opacity: 0.6;
  }
  .primary-tile {
    grid-row: span var(--tile-rows);
    border-color: transparent;
    background: var(--theme-primary);
    color: var(--theme-on-primary);
  }
  .primary-tile .tile-icon {
    background: color-mix(in srgb, var(--theme-on-primary) 16%, transparent);
    color: inherit;
  }
  .primary-tile strong {
    font-size: 16px;
  }
  .primary-tile small {
    color: inherit;
    opacity: 0.85;
  }
  .primary-tile:hover:not(:disabled),
  .primary-tile:global(.file-drop-target-active) {
    border-color: transparent;
    background: var(--theme-primary-hover);
  }
  .tile[aria-pressed="true"],
  .tile.current:not(.primary-tile) {
    border-color: var(--theme-accent);
    background: var(--theme-selected);
  }
  .primary-tile.current {
    box-shadow:
      0 0 0 2px var(--theme-canvas),
      0 0 0 4px var(--theme-primary);
  }
  .status-tile.alert:not(.primary-tile) .tile-icon {
    background: var(--theme-warning-bg);
    color: var(--theme-warning-text);
  }
  .status-tile.alert:not(.primary-tile) small {
    color: var(--theme-warning-text);
  }
  /* Near the 900px minimum window the small tiles need their full width for text. */
  @container (max-width: 780px) {
    .tile-slot .tile-icon {
      display: none;
    }
  }
  @container (max-width: 560px) {
    .action-tiles {
      grid-template-columns: 1fr 1fr;
      grid-template-rows: var(--tile-row) repeat(var(--tile-rows), var(--tile-row));
    }
    .primary-tile {
      grid-column: 1 / -1;
      grid-row: auto;
    }
  }
</style>
