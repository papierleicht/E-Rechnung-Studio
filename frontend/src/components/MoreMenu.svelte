<script lang="ts" module>
  // Every Lucide icon has this component type; the package root would load all icon types.
  import type LucideIcon from "@lucide/svelte/icons/circle";

  export type MoreMenuItem = {
    id: string;
    label: string;
    /** One short sentence: what happens, so nobody has to try it out. */
    hint?: string;
    icon: typeof LucideIcon;
    onclick: () => void;
    /** Removes or discards something: marked in the error colour. */
    danger?: boolean;
    disabled?: boolean;
  };
</script>

<script lang="ts">
  import { onMount, tick } from "svelte";
  import EllipsisIcon from "@lucide/svelte/icons/ellipsis";
  import ChevronDownIcon from "@lucide/svelte/icons/chevron-down";
  let {
    items,
    label = "Mehr",
    title = "Weitere Aktionen",
    disabled = false,
  }: {
    items: MoreMenuItem[];
    label?: string;
    /** Names the menu for screen readers and the tooltip. */
    title?: string;
    disabled?: boolean;
  } = $props();
  const menuId = $props.id();
  let open = $state(false);
  let host: HTMLDivElement;
  let trigger: HTMLButtonElement;
  let menu = $state<HTMLDivElement>();
  // The menu opens towards the side with room: right-aligned by default, from the
  // button's left edge when a clipping container (a narrow card) would cut it off.
  let fromStart = $state(false);
  function place() {
    if (!menu) return;
    fromStart = false;
    const left = menu.getBoundingClientRect().left;
    let edge = 0;
    for (let el = host.parentElement; el; el = el.parentElement) {
      if (getComputedStyle(el).overflowX !== "visible") {
        edge = el.getBoundingClientRect().left;
        break;
      }
    }
    fromStart = left < edge + 8;
    // Near the bottom of a short window the last entries would sit below the fold.
    menu.scrollIntoView({ block: "nearest" });
  }
  const iconProps = { size: 16, color: "currentColor", "stroke-width": 1.75, "aria-hidden": true };
  const entries = () => [
    ...(menu?.querySelectorAll<HTMLButtonElement>("[role=menuitem]:not(:disabled)") ?? []),
  ];
  async function toggle() {
    open = !open;
    if (open) {
      await tick();
      place();
      entries()[0]?.focus();
    }
  }
  function close(restoreFocus = false) {
    open = false;
    if (restoreFocus) trigger.focus();
  }
  function keydown(event: KeyboardEvent) {
    if (event.key === "Escape" && open) {
      event.preventDefault();
      close(true);
    }
    if (event.key === "Tab") close();
    if (event.key === "ArrowDown" || event.key === "ArrowUp") {
      event.preventDefault();
      if (!open) return void toggle();
      const list = entries();
      const index = list.indexOf(document.activeElement as HTMLButtonElement);
      const step = event.key === "ArrowDown" ? 1 : -1;
      list[(index + step + list.length) % list.length]?.focus();
    }
  }
  $effect(() => {
    if (disabled) open = false;
  });
  onMount(() => {
    const outside = (event: PointerEvent) => {
      if (!host.contains(event.target as Node)) close();
    };
    document.addEventListener("pointerdown", outside);
    return () => document.removeEventListener("pointerdown", outside);
  });
</script>

<!-- Actions that are used less often: one visible button instead of a disclosure,
     each entry says in one line what it does. -->
<div class="more-menu" bind:this={host}>
  <button
    class="secondary more-trigger"
    type="button"
    bind:this={trigger}
    {title}
    aria-haspopup="menu"
    aria-expanded={open}
    aria-controls={menuId}
    {disabled}
    onkeydown={keydown}
    onclick={toggle}
    ><EllipsisIcon {...iconProps} />{label}<span class="chevron" class:open
      ><ChevronDownIcon {...iconProps} size={14} /></span
    ></button
  >
  {#if open}<div
      id={menuId}
      class="more-list"
      class:from-start={fromStart}
      role="menu"
      aria-label={title}
      bind:this={menu}
    >
      {#each items as item (item.id)}<button
          type="button"
          role="menuitem"
          class:danger={item.danger}
          disabled={item.disabled}
          onkeydown={keydown}
          onclick={() => {
            close(true);
            item.onclick();
          }}
          ><span class="item-icon"><item.icon {...iconProps} /></span><span class="item-text"
            ><strong>{item.label}</strong>{#if item.hint}<small>{item.hint}</small>{/if}</span
          ></button
        >{/each}
    </div>{/if}
</div>

<style>
  .more-menu {
    position: relative;
    display: inline-flex;
  }
  .more-trigger {
    gap: 6px;
  }
  .chevron {
    display: inline-grid;
    margin-left: -1px;
    color: var(--theme-muted);
    transition: transform 120ms ease;
  }
  .chevron.open {
    transform: rotate(180deg);
  }
  .more-list {
    position: absolute;
    top: calc(100% + 6px);
    right: 0;
    z-index: 30;
    display: grid;
    gap: 2px;
    width: max-content;
    min-width: 240px;
    max-width: min(320px, calc(100vw - 32px));
    padding: 6px;
    border: 1px solid var(--theme-border);
    border-radius: 10px;
    background: var(--theme-surface);
    box-shadow: 0 12px 35px var(--theme-shadow);
  }
  .more-list.from-start {
    right: auto;
    left: 0;
  }
  .more-list button {
    display: flex;
    align-items: flex-start;
    gap: 10px;
    width: 100%;
    padding: 8px 10px;
    border: 0;
    border-radius: 7px;
    background: transparent;
    color: var(--theme-text);
    font: inherit;
    text-align: left;
    cursor: pointer;
  }
  .more-list button:hover:not(:disabled),
  .more-list button:focus-visible {
    background: var(--theme-hover);
    outline: none;
  }
  .more-list button:disabled {
    cursor: default;
    opacity: 0.55;
  }
  .item-icon {
    flex: none;
    display: grid;
    margin-top: 1px;
    color: var(--theme-accent);
  }
  .item-text {
    display: grid;
    gap: 2px;
    min-width: 0;
  }
  .item-text strong {
    font-size: 13px;
    font-weight: 600;
  }
  .item-text small {
    color: var(--theme-muted);
    font-size: 12px;
    line-height: 1.4;
    overflow-wrap: anywhere;
  }
  .danger .item-icon,
  .danger strong {
    color: var(--theme-error-text);
  }
  @media (prefers-reduced-motion: reduce) {
    .chevron {
      transition: none;
    }
  }
</style>
