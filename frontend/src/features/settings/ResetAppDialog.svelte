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
    done = $state<ResetResult | null>(null),
    // The setup's answers stay unless the customer wants a clean slate.
    keepSetup = $state(true);
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
      done = await ResetAPI.ResetApp(typed, keepSetup);
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
          {#if keepSetup}Beim Beenden löscht die App deine Daten auf diesem Gerät. Beim nächsten
            Start sind deine Angaben aus der Einrichtung schon da, alles andere beginnt neu.{:else}Beim
            Beenden löscht die App alle Daten auf diesem Gerät. Beim nächsten Start beginnt sie
            leer, wie nach der Installation.{/if}
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
          Das setzt die App zurück, zum Beispiel nachdem du sie ausprobiert hast. Wähle, womit du
          danach startest:
        </p>
        <fieldset class="reset-mode" disabled={busy} aria-label="Umfang des Zurücksetzens">
          <label
            ><input type="radio" name="reset-mode" value={true} bind:group={keepSetup} />
            <span
              ><strong>Daten löschen, Einrichtung behalten</strong>
              <small
                >Wie eine frische Installation, aber ohne die Einrichtung noch einmal zu
                durchlaufen.</small
              ></span
            ></label
          >
          <label
            ><input type="radio" name="reset-mode" value={false} bind:group={keepSetup} />
            <span
              ><strong>Alles löschen</strong>
              <small>Wie nach der Installation, die Einrichtung beginnt von vorn.</small></span
            ></label
          >
        </fieldset>
        <p>Gelöscht wird:</p>
        <ul>
          <li>
            Rechnungen, Angebote, Entwürfe, Kunden, Artikel, Belege, Zahlungen und Bankumsätze{#if records},
              darunter {records}{/if}
          </li>
          {#if keepSetup}
            <li>
              Weitere Einstellungen wie Nummernkreise, Mahnstufen und DATEV; sie beginnen wieder mit
              den Vorgaben
            </li>
            <li>Gespeicherte Zugänge für Bank und Shops</li>
          {:else}
            <li>Alle Einstellungen, auch deine Angaben aus der Einrichtung</li>
            <li>Gespeicherte Passwörter für E-Mail-Versand, Bankzugang und Shops</li>
            {#if overview?.signedIn}<li>Deine Anmeldung bei papierleicht auf diesem Gerät</li>{/if}
          {/if}
        </ul>
        <p>Erhalten bleiben:</p>
        <ul>
          {#if keepSetup}
            <li>
              Deine Angaben aus der Einrichtung: Unternehmen mit Logo und Bankverbindung,
              steuerliche Angaben und Steuerprofil, E-Mail-Konto und Darstellung{#if overview?.signedIn},
                dazu deine Anmeldung bei papierleicht{/if}
            </li>
          {/if}
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
              acht Jahre aufbewahren. Exportiere sie vorher als Sicherung. Nach dem Zurücksetzen
              beginnen die Rechnungsnummern wieder von vorn; jede Nummer darf aber nur einmal
              vorkommen.
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
          ><Icon name="trash" size={16} />
          {busy ? "Wird zurückgesetzt …" : keepSetup ? "Daten löschen" : "Alles löschen"}</button
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
  .reset-mode {
    display: grid;
    gap: 8px;
    margin: 0;
    padding: 0;
    border: 0;
  }
  .reset-mode label {
    display: flex;
    align-items: flex-start;
    gap: 10px;
    padding: 10px 12px;
    border: 1px solid var(--theme-border);
    border-radius: var(--ui-control-radius);
    cursor: pointer;
  }
  .reset-mode label:has(input:checked) {
    border-color: var(--theme-focus);
  }
  .reset-mode span {
    display: grid;
    gap: 2px;
    min-width: 0;
  }
  .reset-mode small {
    color: var(--theme-muted);
    line-height: 1.4;
  }
  .reset-confirm {
    display: grid;
    gap: 4px;
  }
</style>
