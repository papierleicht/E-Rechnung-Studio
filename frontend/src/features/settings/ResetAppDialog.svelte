<script lang="ts">
  import { onMount } from "svelte";
  import { dismissDialog } from "../../lib/dismissDialog";
  import Icon from "../../components/Icon.svelte";
  import * as ResetAPI from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/app/resetservice";
  import type {
    ResetOverview,
    ResetResult,
  } from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/app/models";
  import { serviceErrorMessage } from "../../lib/errors";

  const confirmation = "ZURÜCKSETZEN";
  let {
    exportBackup,
    oncancel,
  }: {
    /** Opens the save dialog for the ZIP backup before anything is deleted. */
    exportBackup: () => void;
    oncancel: () => void;
  } = $props();
  let dialog: HTMLDialogElement;
  let overview = $state<ResetOverview | null>(null),
    typed = $state(""),
    busy = $state(false),
    error = $state(""),
    done = $state<ResetResult | null>(null);
  const matches = $derived(typed.trim() === confirmation);
  const hasRecords = $derived(
    !!overview && overview.issuedInvoices + overview.receivedDocuments > 0,
  );

  onMount(async () => {
    dialog.showModal();
    try {
      overview = await ResetAPI.ResetOverview();
    } catch (e) {
      error = serviceErrorMessage(e, "Es ließ sich nicht ermitteln, was gespeichert ist.");
    }
  });

  function cancel() {
    if (!busy && !done) oncancel();
  }

  async function reset() {
    if (busy || !matches || done) return;
    busy = true;
    error = "";
    try {
      done = await ResetAPI.ResetApp(typed);
      // Remembered views and answers in this window belong to the old data.
      try {
        localStorage.clear();
      } catch {}
    } catch (e) {
      error = serviceErrorMessage(e, "Die App ließ sich nicht zurücksetzen.");
    } finally {
      busy = false;
    }
  }

  async function quit() {
    busy = true;
    try {
      await ResetAPI.QuitAfterReset();
    } catch (e) {
      error = serviceErrorMessage(e, "Die App ließ sich nicht beenden. Beende sie bitte selbst.");
      busy = false;
    }
  }

  function count(n: number, one: string, many: string) {
    return n ? `${n} ${n === 1 ? one : many}` : "";
  }
  const records = $derived(
    overview
      ? [
          count(overview.issuedInvoices, "ausgestellte Rechnung", "ausgestellte Rechnungen"),
          count(overview.receivedDocuments, "empfangener Beleg", "empfangene Belege"),
        ]
          .filter(Boolean)
          .join(" und ")
      : "",
  );
</script>

<dialog
  use:dismissDialog
  aria-labelledby="reset-app-title"
  bind:this={dialog}
  class="invoice-settings-dialog"
  oncancel={(event) => {
    event.preventDefault();
    cancel();
  }}
>
  <form
    class="modal page-settings reset-app-dialog"
    aria-labelledby="reset-app-title"
    onsubmit={(event) => {
      event.preventDefault();
      void (done ? quit() : reset());
    }}
  >
    <header>
      <h2 id="reset-app-title">
        {done ? "App zurückgesetzt" : "App zurücksetzen?"}
      </h2>
      {#if !done}<button
          type="button"
          aria-label="Schließen"
          class="dialog-close"
          title="Schließen (Esc)"
          disabled={busy}
          onclick={cancel}><Icon name="close" /></button
        >{/if}
    </header>
    <div class="page-settings-body">
      {#if done}
        <p role="status">
          Beim Beenden löscht die App alle Daten auf diesem Gerät. Beim nächsten Start beginnt sie
          leer, wie nach der Installation.
        </p>
        {#if done.keychainLeft?.length}
          <p class="reset-warning">
            Einige gespeicherte Zugangsdaten ließen sich nicht entfernen. Lösche die Einträge
            {#each done.keychainLeft as service, i}{i ? ", " : ""}<code>{service}</code>{/each}
            bei Bedarf selbst im Passwortspeicher deines Systems.
          </p>
        {/if}
      {:else}
        <p>
          Das setzt die App auf den Stand nach der Installation zurück, zum Beispiel nachdem du sie
          ausprobiert hast. Gelöscht wird alles, was die App auf diesem Gerät gespeichert hat:
        </p>
        <ul>
          <li>
            Rechnungen, Angebote, Entwürfe, Kunden, Artikel, Belege, Zahlungen und alle
            Einstellungen{#if records}, darunter {records}{/if}
          </li>
          <li>Gespeicherte Passwörter für E-Mail-Versand, Bankzugang und Shops</li>
          {#if overview?.signedIn}<li>Deine Anmeldung bei papierleicht auf diesem Gerät</li>{/if}
        </ul>
        <p>Erhalten bleiben:</p>
        <ul>
          <li>
            Dateien, die du exportiert oder gesichert hast{#if overview?.exportDirectory}, etwa im
              Exportordner <code>{overview.exportDirectory}</code>{/if}
          </li>
          <li>Dein Konto und dein Abo, falls du eines hast</li>
          {#if overview?.backupKey}<li>
              Cloud-Backups und der Wiederherstellungsschlüssel im Schlüsselbund, damit du sie
              wiederherstellen kannst
            </li>{/if}
        </ul>
        {#if hasRecords}
          <div class="reset-warning">
            <p>
              Waren das keine Testdaten? Ausgestellte Rechnungen und Belege musst du in der Regel
              acht Jahre aufbewahren. Exportiere sie vorher als Sicherung.
            </p>
            <button type="button" class="secondary" disabled={busy} onclick={exportBackup}
              >Sicherung exportieren</button
            >
          </div>
        {/if}
        <label class="reset-confirm"
          ><span>Gib zur Bestätigung <strong>{confirmation}</strong> ein</span>
          <input
            bind:value={typed}
            autocomplete="off"
            autocapitalize="characters"
            spellcheck="false"
            disabled={busy}
          /></label
        >
      {/if}
      {#if error}<p class="export-error" role="alert">{error}</p>{/if}
    </div>
    <footer>
      {#if done}
        <button class="primary" disabled={busy}>App beenden</button>
      {:else}
        <button type="button" class="secondary" disabled={busy} onclick={cancel}>Abbrechen</button>
        <button class="danger-text" disabled={busy || !matches || !overview}
          ><Icon name="trash" size={16} /> {busy ? "Wird zurückgesetzt …" : "Alles löschen"}</button
        >
      {/if}
    </footer>
  </form>
</dialog>

<style>
  .reset-app-dialog .page-settings-body {
    display: grid;
    gap: 12px;
  }
  .reset-app-dialog p,
  .reset-app-dialog ul {
    margin: 0;
  }
  .reset-app-dialog ul {
    display: grid;
    gap: 6px;
    padding-left: 20px;
    line-height: 1.45;
  }
  .reset-app-dialog code {
    overflow-wrap: anywhere;
  }
  .reset-warning {
    display: grid;
    gap: 8px;
    justify-items: start;
    padding: 10px 12px;
    border-radius: var(--ui-control-radius);
    background: var(--theme-warning-bg);
    color: var(--theme-warning-text);
    line-height: 1.45;
  }
  .reset-confirm {
    display: grid;
    gap: 4px;
  }
</style>
