<script lang="ts">
  import { dismissDialog } from "../lib/dismissDialog";
  import { onMount, tick, type Snippet } from "svelte";
  import Icon from "./Icon.svelte";
  let {
    title,
    description = "",
    busy = false,
    error = "",
    onclose,
    onsubmit,
    children,
    footer,
  }: {
    title: string;
    description?: string;
    busy?: boolean;
    error?: string;
    onclose: () => void;
    onsubmit?: () => void;
    children: Snippet;
    footer: Snippet;
  } = $props();
  const titleId = $props.id();
  let dialog: HTMLDialogElement;
  function close() {
    if (!busy) onclose();
  }
  // Keep server feedback visible even when the compact dialog body has scrolled.
  $effect(() => {
    if (error)
      tick().then(() =>
        dialog.querySelector<HTMLElement>(".dialog-error")?.scrollIntoView({ block: "nearest" }),
      );
  });
  onMount(() => {
    dialog.showModal();
    // Without a field, focus starts on the first harmless answer (e.g. "Abbrechen"),
    // not on the close icon.
    (
      dialog.querySelector<HTMLElement>("input, select, textarea") ??
      dialog.querySelector<HTMLElement>("footer button:not(.danger-text)")
    )?.focus();
    const revealInvalid = (event: Event) => {
      let parent = (event.target as HTMLElement)?.parentElement;
      while (parent && parent !== dialog) {
        if (parent instanceof HTMLDetailsElement) parent.open = true;
        parent = parent.parentElement;
      }
    };
    dialog.addEventListener("invalid", revealInvalid, true);
    return () => dialog.removeEventListener("invalid", revealInvalid, true);
  });
</script>

<dialog
  use:dismissDialog
  aria-labelledby={titleId}
  class="app-dialog"
  bind:this={dialog}
  oncancel={(event) => {
    event.preventDefault();
    close();
  }}
>
  <form
    class="modal form-dialog"
    aria-labelledby={titleId}
    onsubmit={(event) => {
      event.preventDefault();
      if (!busy) onsubmit?.();
    }}
  >
    <header>
      <div>
        <h2 id={titleId}>{title}</h2>
        {#if description}<p class="heading-subtitle">{description}</p>{/if}
      </div>
      <button
        class="dialog-close"
        type="button"
        aria-label="Dialog schließen"
        title="Schließen (Esc)"
        disabled={busy}
        onclick={close}><Icon name="close" /></button
      >
    </header>
    <div class="dialog-body">
      <fieldset disabled={busy} class="dialog-fields">{@render children()}</fieldset>
      {#if error}<p class="dialog-error" role="alert">{error}</p>{/if}
    </div>
    <footer>{@render footer()}</footer>
  </form>
</dialog>
