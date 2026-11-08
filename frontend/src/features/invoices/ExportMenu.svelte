<script lang="ts">
  import { onMount, tick } from "svelte";
  import Icon from "../../components/Icon.svelte";
  let { disabled, onxml }: { disabled: boolean; onxml: () => void } = $props();
  const menuId = $props.id();
  let open = $state(false);
  let host: HTMLDivElement;
  let trigger: HTMLButtonElement;
  let item = $state<HTMLButtonElement>();
  async function toggle() {
    open = !open;
    if (open) {
      await tick();
      item?.focus();
    }
  }
  function close(restoreFocus = false) {
    open = false;
    if (restoreFocus) trigger.focus();
  }
  function keydown(event: KeyboardEvent) {
    if (event.key === "Escape") {
      event.preventDefault();
      close(true);
    }
    if (event.key === "Tab") close(true);
    if (event.key === "ArrowDown" || event.key === "ArrowUp") {
      event.preventDefault();
      if (open) item?.focus();
      else toggle();
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

<div class="export-menu" bind:this={host} role="group" aria-label="Weitere Exportoptionen">
  <button
    class="primary issue-more-button"
    type="button"
    bind:this={trigger}
    aria-label="Weitere Exportoptionen"
    title="Weitere Exportoptionen"
    aria-haspopup="menu"
    aria-expanded={open}
    aria-controls={menuId}
    {disabled}
    onkeydown={keydown}
    onclick={toggle}><Icon name="down" size={16} /></button
  >
  {#if open}<div
      id={menuId}
      class="issue-export-menu"
      role="menu"
      aria-label="Weitere Exportoptionen"
    >
      <button
        bind:this={item}
        type="button"
        role="menuitem"
        onkeydown={keydown}
        onclick={() => {
          close();
          onxml();
        }}
        ><Icon name="download" size={18} /><span
          ><strong>Zusätzlich XML speichern</strong><span
            >Stelle die Rechnung aus und speichere die XML-Datei zusätzlich an einem anderen Ort.</span
          ></span
        ></button
      >
    </div>{/if}
</div>
