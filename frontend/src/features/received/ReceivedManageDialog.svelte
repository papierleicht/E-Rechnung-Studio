<script lang="ts">
  import type { ReceivedInvoice } from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/model/models";
  import FormDialog from "../../components/FormDialog.svelte";
  let {
    invoice,
    action,
    busy,
    error,
    onclose,
    onconfirm,
  }: {
    invoice: ReceivedInvoice;
    action: "misimport" | "delete";
    busy: boolean;
    error: string;
    onclose: () => void;
    onconfirm: (reason: string, filename: string, confirmed: boolean) => void;
  } = $props();
  let reason = $state(""),
    filename = $state(""),
    confirmed = $state(false);
  const canConfirm = $derived(
    action === "misimport" ? !!reason.trim() : confirmed && filename === invoice.filename,
  );
</script>

<FormDialog
  title={action === "delete" ? "Fehlimport endgültig löschen" : "Als Fehlimport kennzeichnen"}
  description={invoice.filename}
  {busy}
  {error}
  {onclose}
>
  {#if action === "misimport"}
    <p>
      Der Beleg verschwindet aus dem aktiven Eingang. Original und Verlauf bleiben erhalten, du
      kannst ihn später wiederherstellen.
    </p>
    <label
      >Grund<textarea
        rows="3"
        maxlength="2000"
        bind:value={reason}
        placeholder="Zum Beispiel: Testdatei oder falsches Unternehmen"
        required></textarea></label
    >
  {:else}
    <p>
      Wir löschen den Beleg und seinen Verlauf aus dieser App. Das lässt sich hier nicht rückgängig
      machen. Bereits exportierte Dateien und vorhandene Datensicherungen bleiben bestehen.
    </p>
    <label class="confirmation"
      ><input type="checkbox" bind:checked={confirmed} />Ich bestätige, dass dieser Fehlimport nicht
      aufbewahrungspflichtig ist. Eine fehlgeschlagene Prüfung oder eine fehlende Buchung allein ist
      kein Löschgrund.</label
    >
    <label
      >Gib zur Bestätigung den Dateinamen ein<input
        bind:value={filename}
        autocomplete="off"
        spellcheck="false"
      /></label
    >
  {/if}
  {#snippet footer()}
    <button type="button" class="secondary" disabled={busy} onclick={onclose}>Abbrechen</button>
    <button
      type="button"
      class={action === "delete" ? "secondary danger-text" : "primary"}
      disabled={busy || !canConfirm}
      onclick={() => onconfirm(reason, filename, confirmed)}
      >{action === "delete" ? "Endgültig löschen" : "Fehlimport kennzeichnen"}</button
    >
  {/snippet}
</FormDialog>

<style>
  p,
  label {
    font-size: 13px;
    line-height: 1.5;
  }
  label {
    display: grid;
    gap: 8px;
    margin-top: 16px;
  }
  textarea,
  input:not([type="checkbox"]) {
    width: 100%;
    padding: 9px;
    border: 1px solid var(--theme-input-border);
    border-radius: 7px;
  }
  .confirmation {
    display: flex;
    align-items: start;
    gap: 10px;
  }
  .confirmation input {
    margin-top: 4px;
    flex-shrink: 0;
  }
</style>
