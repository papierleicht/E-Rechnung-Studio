<script lang="ts">
  import { onMount } from "svelte";
  import { dismissDialog } from "../../lib/dismissDialog";
  import Icon from "../../components/Icon.svelte";
  import * as UpdateAPI from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/app/updateservice";
  import type { InstallLocation } from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/app/models";
  import { serviceErrorMessage } from "../../lib/errors";
  import { installProblemText, installTargetName } from "../../lib/installLocation";

  let {
    location,
    unsaved = false,
    onclose,
  }: {
    location: InstallLocation;
    /** Unsaved edits would be lost when the app restarts. */
    unsaved?: boolean;
    onclose: () => void;
  } = $props();
  let dialog: HTMLDialogElement;
  let busy = $state(false),
    moved = $state(false),
    error = $state("");
  const targetName = $derived(installTargetName(location.target ?? ""));

  onMount(() => dialog.showModal());

  function close() {
    if (!busy && !moved) onclose();
  }

  async function move() {
    if (busy || moved || unsaved) return;
    busy = true;
    error = "";
    try {
      await UpdateAPI.MoveApp();
      moved = true;
    } catch (e) {
      error = serviceErrorMessage(e, "Die App ließ sich nicht verschieben.");
    } finally {
      busy = false;
    }
  }
</script>

<dialog
  use:dismissDialog
  aria-labelledby="move-app-title"
  bind:this={dialog}
  class="invoice-settings-dialog"
  oncancel={(event) => {
    event.preventDefault();
    close();
  }}
>
  <form
    class="modal page-settings move-app-dialog"
    aria-labelledby="move-app-title"
    onsubmit={(event) => {
      event.preventDefault();
      void move();
    }}
  >
    <header>
      <h2 id="move-app-title">
        {location.target
          ? `App nach „${targetName}“ verschieben?`
          : "Updates sind hier nicht möglich"}
      </h2>
      <button
        type="button"
        aria-label="Schließen"
        class="dialog-close"
        title="Schließen (Esc)"
        disabled={busy || moved}
        onclick={close}><Icon name="close" /></button
      >
    </header>
    <div class="page-settings-body">
      {#if moved}
        <p role="status">Die App startet gleich aus „{targetName}“ neu.</p>
      {:else}
        <p>{installProblemText(location.problem ?? "")}</p>
        {#if location.target}
          <p>
            Die App kopiert sich nach <code>{location.target}</code> und startet von dort neu. Deine Daten
            bleiben, wo sie sind.
          </p>
          {#if location.targetExists}<p>Die App, die dort schon liegt, wird dabei ersetzt.</p>{/if}
          {#if unsaved}<p class="move-warning">
              Speichere zuerst deine Änderungen. Beim Neustart gingen sie sonst verloren.
            </p>{/if}
        {:else}
          <p>
            Verschiebe die App bitte selbst an einen Ort, an dem du Dateien ändern darfst, und
            starte sie von dort.
          </p>
        {/if}
      {/if}
      {#if error}<p class="export-error" role="alert">{error}</p>{/if}
    </div>
    <footer>
      {#if !moved}
        <button type="button" class="secondary" disabled={busy} onclick={close}>Später</button>
        {#if location.target}<button class="primary" disabled={busy || unsaved}
            >{busy ? "Wird verschoben …" : "Verschieben und neu starten"}</button
          >{/if}
      {/if}
    </footer>
  </form>
</dialog>

<style>
  .move-app-dialog .page-settings-body {
    display: grid;
    gap: 12px;
  }
  .move-app-dialog p {
    margin: 0;
    line-height: 1.45;
  }
  .move-app-dialog code {
    overflow-wrap: anywhere;
  }
  .move-warning {
    padding: 10px 12px;
    border-radius: var(--ui-control-radius);
    background: var(--theme-warning-bg);
    color: var(--theme-warning-text);
  }
</style>
