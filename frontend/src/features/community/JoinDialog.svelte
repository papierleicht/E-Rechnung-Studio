<script lang="ts">
  import { untrack } from "svelte";
  import FormDialog from "../../components/FormDialog.svelte";
  import GuidelinesText from "./GuidelinesText.svelte";
  import { GUIDELINES_VERSION } from "../../lib/community";
  import { errorDetail } from "../../lib/errors";

  let {
    serverVersion,
    currentName = "",
    rename = false,
    onjoin,
    onclose,
  }: {
    /** The guidelines version the server asks for. */
    serverVersion: string;
    currentName?: string;
    /** Members only change their name; the guidelines stay accepted. */
    rename?: boolean;
    onjoin: (name: string) => Promise<void>;
    onclose: () => void;
  } = $props();
  let name = $state(untrack(() => currentName));
  let accepted = $state(untrack(() => rename));
  let busy = $state(false);
  let error = $state("");
  const outdated = $derived(!!serverVersion && serverVersion !== GUIDELINES_VERSION);

  async function join() {
    if (outdated) return;
    if (!accepted) {
      error = "Bitte stimme den Community-Richtlinien zu.";
      return;
    }
    busy = true;
    error = "";
    try {
      await onjoin(name.trim());
    } catch (err) {
      error = errorDetail(err);
    } finally {
      busy = false;
    }
  }
</script>

<FormDialog
  title={rename ? "Anzeigenamen ändern" : "Willkommen in der Community"}
  description={rename
    ? "Der neue Name erscheint auch an deinen bisherigen Beiträgen."
    : "Lies die Richtlinien und wähle, wie dich die anderen Mitglieder sehen."}
  {busy}
  {error}
  {onclose}
  onsubmit={join}
>
  {#if outdated}
    <p class="join-outdated" role="alert">
      Die Community-Richtlinien wurden aktualisiert. Bitte aktualisiere die App, damit du die neue
      Fassung lesen und ihr zustimmen kannst.
    </p>
  {/if}
  {#if !rename}
    <div class="join-guidelines" tabindex="-1">
      <GuidelinesText />
    </div>
  {/if}
  <div class="form-grid join-fields">
    <label
      >Anzeigename<input
        bind:value={name}
        required
        minlength="2"
        maxlength="30"
        autocomplete="nickname"
        placeholder="z. B. Lena Grafik oder Murat Elektro"
      /></label
    >
    <p class="form-help">
      2 bis 30 Zeichen. Gern Vorname und Gewerk, damit man dich findet. Deine E-Mail-Adresse sieht
      niemand.
    </p>
    {#if !rename}
      <label
        ><input type="checkbox" bind:checked={accepted} />Ich habe die Community-Richtlinien gelesen
        und halte mich daran.</label
      >
    {/if}
  </div>
  {#snippet footer()}
    <button type="button" class="secondary" disabled={busy} onclick={onclose}>Abbrechen</button>
    <button type="submit" class="primary" disabled={busy || outdated}
      >{busy ? "Wird gespeichert …" : rename ? "Namen speichern" : "Community beitreten"}</button
    >
  {/snippet}
</FormDialog>

<style>
  .join-guidelines {
    max-height: 280px;
    overflow: auto;
    margin-bottom: 14px;
    padding: 12px 14px;
    border: 1px solid var(--theme-border);
    border-radius: 10px;
    background: var(--theme-surface-muted);
  }
  .form-grid.join-fields {
    grid-template-columns: minmax(0, 1fr);
  }
  .join-fields .form-help {
    margin: 0;
  }
  .join-outdated {
    margin: 0 0 12px;
    padding: 10px 12px;
    border-radius: 8px;
    background: var(--theme-warning-bg);
    color: var(--theme-warning-text);
    font-size: 13px;
  }
</style>
