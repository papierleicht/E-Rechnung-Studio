<script lang="ts">
  import { onDestroy, onMount } from "svelte";
  import { Dialogs, Events, Window } from "@wailsio/runtime";
  import * as BackupService from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/app/backupservice";
  import type {
    BackupAccount,
    CloudBackupStatus,
    RestorePreview,
  } from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/app/models";
  import type { BackupVersion } from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/account/models";
  import type { BackupEvent } from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/store/models";
  import { accountErrorMessage, formatBackupSize, formatPlanDate } from "../../lib/planStatus";
  import { cached, remember } from "../../lib/cache";
  import {
    KEY_GROUPS,
    keyGroupsEntered,
    progressLabel,
    recoveryKeyLines,
    restoreLoss,
    storageFull,
    verificationDue,
  } from "../../lib/backup";
  import RecoveryKeyInput from "../../components/RecoveryKeyInput.svelte";
  import SettingSection from "../../components/SettingSection.svelte";
  import { waitForFileDialog, isFileDialogCancellation } from "../../lib/fileDialog";

  // The state of the last visit shows right away while it reloads.
  let status = $state<CloudBackupStatus | null>(cached("backup:status") ?? null);
  let account = $state<BackupAccount | null>(cached("backup:account") ?? null);
  let versions = $state<BackupVersion[]>(cached("backup:versions") ?? []);
  let log = $state<BackupEvent[]>(cached("backup:log") ?? []);
  $effect(() => {
    if (status) remember("backup:status", $state.snapshot(status));
    remember("backup:account", $state.snapshot(account));
    remember("backup:versions", $state.snapshot(versions));
    remember("backup:log", $state.snapshot(log));
  });
  let newKey = $state("");
  let confirmKey = $state("");
  let keyNoted = $state(false);
  let sectionElement = $state<HTMLElement>();
  let confirmRemoveKey = $state(false);
  let checkedKey = $state("");
  let existingKey = $state("");
  let shownKey = $state("");
  let keyMode = $state<"choose" | "import">("choose");
  let working = $state(false);
  let error = $state("");
  let message = $state("");
  let preview = $state<{ id: string; data: RestorePreview } | null>(null);
  let restoreConfirmed = $state(false);
  let restored = $state("");
  let deleteConfirmed = $state(false);

  // Without the plan the stored data can still be fetched, nothing new added.
  const readOnly = $derived(account?.access === "read-only");
  const latestVersion = $derived(
    versions.reduce<BackupVersion | null>(
      (latest, version) => (!latest || version.created > latest.created ? version : latest),
      null,
    ),
  );
  const loss = $derived(preview ? restoreLoss(preview.data.impact) : "");
  const keyComplete = (key: string) => keyGroupsEntered(key) === KEY_GROUPS;

  function dateTime(iso: string) {
    if (!iso) return "";
    const d = new Date(iso);
    return Number.isNaN(d.getTime())
      ? ""
      : d.toLocaleString("de-DE", { dateStyle: "medium", timeStyle: "short" });
  }

  async function run<T>(fn: () => Promise<T>, fallback: string): Promise<T | undefined> {
    if (working) return undefined;
    working = true;
    error = "";
    message = "";
    try {
      return await fn();
    } catch (e) {
      error = accountErrorMessage(e, fallback);
      return undefined;
    } finally {
      working = false;
    }
  }

  async function loadVersions() {
    try {
      versions = (await BackupService.BackupVersions()) ?? [];
    } catch (e) {
      error = accountErrorMessage(e, "Deine Sicherungen konnten nicht geladen werden.");
    }
  }
  async function loadAccount() {
    try {
      account = await BackupService.BackupAccount();
    } catch {
      account = null;
    }
  }
  async function loadLog() {
    try {
      log = (await BackupService.BackupLog()) ?? [];
    } catch {
      log = [];
    }
  }

  async function createKey() {
    const key = await run(
      () => BackupService.CreateBackupKey(),
      "Den Schlüssel konnten wir nicht erstellen.",
    );
    if (key) {
      newKey = key;
      confirmKey = "";
      keyNoted = false;
      status = await BackupService.BackupStatus();
    }
  }
  async function copyKey() {
    try {
      await navigator.clipboard.writeText(newKey || shownKey);
      message = "Schlüssel kopiert. Leg ihn jetzt im Passwortmanager ab oder drucke ihn aus.";
    } catch {
      error = "Das Kopieren hat nicht geklappt. Schreib den Schlüssel bitte ab.";
    }
  }
  // The print sheet sits directly in the body, so the hidden app takes no space
  // on the page. See the print styles at the end.
  function toBody(node: HTMLElement) {
    document.body.appendChild(node);
    return { destroy: () => node.remove() };
  }
  async function printKey() {
    const body = document.body;
    let scroller: Element | null = sectionElement ?? null;
    while (scroller && scroller.scrollTop === 0) scroller = scroller.parentElement;
    const top = scroller?.scrollTop ?? 0;
    body.classList.add("print-recovery-key");
    // The native dialog takes the input, so the next click ends the print mode.
    const done = () => {
      body.classList.remove("print-recovery-key");
      if (scroller) scroller.scrollTop = top;
    };
    window.addEventListener("afterprint", done, { once: true });
    window.addEventListener("pointerdown", done, { once: true, capture: true });
    try {
      await Window.Print();
    } catch {
      window.print();
    }
    if (scroller) scroller.scrollTop = top;
  }
  onDestroy(() => document.body.classList.remove("print-recovery-key"));

  async function confirmNewKey() {
    const next = await run(
      () => BackupService.ConfirmBackupKey(confirmKey),
      "Den Schlüssel konnten wir nicht speichern. Prüfe deine Eingabe.",
    );
    if (next) {
      status = next;
      newKey = confirmKey = "";
      message = "Geschafft. Die erste Sicherung wird jetzt erstellt.";
      await backupNow();
    }
  }
  async function cancelNewKey() {
    status = await BackupService.CancelBackupKey();
    newKey = confirmKey = "";
  }
  async function importKey() {
    const next = await run(
      () => BackupService.ImportBackupKey(existingKey),
      "Den Schlüssel konnten wir nicht speichern. Prüfe deine Eingabe.",
    );
    if (next) {
      status = next;
      existingKey = "";
      keyMode = "choose";
      message = "Schlüssel gespeichert. Du kannst jetzt einen Stand wiederherstellen.";
      await loadVersions();
    }
  }
  async function checkKey() {
    const next = await run(
      () => BackupService.CheckBackupKey(checkedKey),
      "Den Schlüssel konnten wir nicht prüfen.",
    );
    if (next) {
      status = next;
      checkedKey = "";
      message = "Der Schlüssel stimmt. Wir fragen in einem Jahr wieder nach.";
    }
  }
  async function postponeKeyCheck() {
    status = await BackupService.PostponeKeyCheck();
    checkedKey = "";
  }
  async function removeKey() {
    const next = await run(
      () => BackupService.RemoveBackupKey(),
      "Den Schlüssel konnten wir nicht entfernen.",
    );
    if (next) {
      status = next;
      shownKey = "";
      confirmRemoveKey = false;
    }
  }
  async function showKey() {
    const key = await run(
      () => BackupService.ShowBackupKey(),
      "Der Schlüssel ließ sich nicht anzeigen.",
    );
    if (key) shownKey = key;
  }
  async function backupNow() {
    const next = await run(() => BackupService.BackupNow(), "Die Sicherung hat nicht geklappt.");
    if (next) {
      status = next;
      message = "Deine Daten sind gesichert.";
    } else {
      status = await BackupService.BackupStatus();
    }
    await Promise.all([loadVersions(), loadAccount()]);
  }
  async function setAuto(event: Event) {
    status = await BackupService.SetAutoBackup((event.currentTarget as HTMLInputElement).checked);
  }
  async function openPreview(id: string) {
    restoreConfirmed = false;
    const data = await run(
      () => BackupService.PreviewRestore(id),
      "Die Sicherung ließ sich nicht öffnen.",
    );
    preview = data ? { id, data } : null;
  }
  async function restore() {
    if (!preview || !restoreConfirmed) return;
    const result = await run(
      () => BackupService.RestoreVersion(preview!.id),
      "Die Wiederherstellung hat nicht geklappt. Deine Daten sind unverändert.",
    );
    if (result) {
      preview = null;
      restored = result.safetyCopy;
      // Every page must reload the replaced data; saving stale forms would
      // overwrite what was just restored.
      window.setTimeout(() => window.location.reload(), 8000);
    }
  }
  async function verify(id: string) {
    const result = await run(
      () => BackupService.VerifyVersion(id),
      "Die Sicherung ließ sich nicht prüfen.",
    );
    if (result) {
      if (result.ok) message = result.summary;
      else error = result.summary;
      await loadLog();
      status = await BackupService.BackupStatus();
    }
  }
  async function saveVersion(version: BackupVersion) {
    const path = await pickSavePath(`sicherung-${version.created.slice(0, 10)}.zip`);
    if (!path) return;
    const result = await run(
      () => BackupService.SaveVersionArchive(version.id, path),
      "Die Datei ließ sich nicht schreiben.",
    );
    if (result) {
      message = `Sicherung gespeichert: ${result.path}`;
      await loadLog();
    }
  }
  async function pickSavePath(filename: string) {
    try {
      return await waitForFileDialog(
        () =>
          Dialogs.SaveFile({
            Title: "Sicherung speichern",
            Filename: filename,
            CanCreateDirectories: true,
            Filters: [{ DisplayName: "Sicherung (ZIP)", Pattern: "*.zip" }],
          }),
        new AbortController().signal,
      );
    } catch (e) {
      if (!isFileDialogCancellation(e)) {
        error = accountErrorMessage(e, "Die Dateiauswahl hat nicht geklappt.");
      }
      return undefined;
    }
  }
  async function deleteAll() {
    if (!deleteConfirmed) return;
    const at = await run(
      () => BackupService.DeleteAllBackups(),
      "Die Löschung ließ sich nicht vormerken.",
    );
    if (at) {
      deleteConfirmed = false;
      await loadAccount();
    }
  }
  async function cancelDeletion() {
    const done = await run(async () => {
      await BackupService.CancelBackupDeletion();
      return true;
    }, "Die Löschung ließ sich nicht abbrechen.");
    if (done) await loadAccount();
  }

  onMount(() => {
    void (async () => {
      try {
        status = await BackupService.BackupStatus();
        await Promise.all([loadAccount(), loadLog()]);
        if (status.keyConfigured) await loadVersions();
      } catch (e) {
        error = accountErrorMessage(e, "Den Sicherungsstatus konnten wir nicht laden.");
      }
    })();
    return Events.On("backup:status", (event) => {
      const data = (Array.isArray(event.data) ? event.data[0] : event.data) as CloudBackupStatus;
      if (data && typeof data === "object") status = data;
    });
  });
</script>

{#snippet keyBlock(key: string)}
  <code class="recovery-key" aria-label="Dein Schlüssel">
    {#each recoveryKeyLines(key) as line}<span>{line}</span>{/each}
  </code>
{/snippet}

{#if newKey || shownKey}
  <div class="key-print" use:toBody>
    <h1>Dein Schlüssel für die Sicherung</h1>
    <p>E-Rechnung-Studio von papierleicht, erstellt am {new Date().toLocaleDateString("de-DE")}</p>
    {@render keyBlock(newKey || shownKey)}
    <p>
      Bewahre diesen Zettel sicher und getrennt von deinem Rechner auf. Mit diesem Schlüssel kommst
      du an deine Sicherung, auch wenn dein Rechner ausfällt. Niemand kann ihn ersetzen, auch wir
      nicht. Wer ihn hat, kann deine Sicherung lesen, wenn er auch an dein Konto kommt.
    </p>
  </div>
{/if}

<section
  bind:this={sectionElement}
  class="settings-group cloud-backup"
  aria-labelledby="cloud-backup-title"
  aria-busy={working}
>
  <h3 id="cloud-backup-title">Sicherung</h3>
  <p class="settings-help">
    Deine Daten werden automatisch außer Haus gesichert. Nur du kannst sie lesen, auch wir nicht.
  </p>

  {#if error}<p class="backup-message error" role="alert">{error}</p>{/if}
  {#if message}<p class="backup-message info" role="status">{message}</p>{/if}
  {#if status?.progress}
    <p class="backup-message info" role="status">{progressLabel(status.progress)}</p>
  {/if}
  {#if account?.deletionAt}
    <div class="backup-message warning" role="status">
      <p>
        Alle Sicherungen werden am {formatPlanDate(account.deletionAt)} gelöscht. Bis dahin ruht die automatische
        Sicherung.
      </p>
      <button class="secondary" type="button" disabled={working} onclick={cancelDeletion}
        >Löschung abbrechen</button
      >
    </div>
  {:else if readOnly}
    <p class="backup-message warning" role="status">
      Dein Backup-Zusatz ist beendet. Deine Sicherungen bleiben bis {formatPlanDate(
        account?.readableUntil ?? "",
      )} abrufbar, danach löschen wir sie. Speichere sie vorher als Datei oder stelle sie wieder her.
    </p>
  {/if}

  {#if restored}
    <div class="backup-message info" role="status">
      <p><strong>Wiederherstellung abgeschlossen.</strong></p>
      <p>
        Deine bisherigen Daten haben wir vorher lokal gesichert unter:<br /><code>{restored}</code>
      </p>
      <p>Die App lädt die Daten in wenigen Sekunden neu.</p>
      <button class="primary" type="button" onclick={() => window.location.reload()}
        >Jetzt neu laden</button
      >
    </div>
  {:else if !status}
    <p class="settings-help">Wird geladen …</p>
  {:else if !status.keyConfigured}
    {#if newKey && !keyNoted}
      <div class="key-step">
        <h4>Schritt 1 von 2: Schlüssel notieren</h4>
        <p>
          Mit diesem Schlüssel kommst du an deine Sicherung, auch wenn dein Rechner ausfällt.
          Niemand kann ihn ersetzen, auch wir nicht.
        </p>
        {@render keyBlock(newKey)}
        <div class="actions">
          <button class="secondary" type="button" onclick={printKey}>Drucken</button>
          <button class="secondary" type="button" onclick={copyKey}>Kopieren</button>
        </div>
        <ul class="key-tips">
          <li>Drucke ihn aus oder speichere ihn in einem Passwortmanager.</li>
          <li>Bewahre ihn getrennt von diesem Gerät auf.</li>
        </ul>
        <div class="actions">
          <button class="primary" type="button" onclick={() => (keyNoted = true)}
            >Ich habe ihn sicher aufbewahrt</button
          >
          <button class="text" type="button" disabled={working} onclick={cancelNewKey}
            >Abbrechen</button
          >
        </div>
      </div>
    {:else if newKey}
      <form
        class="key-step"
        onsubmit={(event) => {
          event.preventDefault();
          if (confirmKey === newKey) void confirmNewKey();
        }}
      >
        <h4>Schritt 2 von 2: Schlüssel bestätigen</h4>
        <p>Tippe den Schlüssel von deiner Notiz ab. So stellen wir sicher, dass er stimmt.</p>
        <RecoveryKeyInput bind:value={confirmKey} label="Dein Schlüssel" expected={newKey} />
        <div class="actions">
          <button class="primary" type="submit" disabled={working || confirmKey !== newKey}
            >Sicherung starten</button
          >
          <button
            class="secondary"
            type="button"
            disabled={working}
            onclick={() => (keyNoted = false)}>Zurück</button
          >
        </div>
      </form>
    {:else if keyMode === "import"}
      <form
        class="key-step"
        onsubmit={(event) => {
          event.preventDefault();
          void importKey();
        }}
      >
        <h4>Vorhandenen Schlüssel eingeben</h4>
        <p>Du hast den Schlüssel notiert, als du die Sicherung eingerichtet hast.</p>
        <RecoveryKeyInput bind:value={existingKey} label="Dein Schlüssel" />
        <div class="actions">
          <button class="primary" type="submit" disabled={working || !keyComplete(existingKey)}
            >Schlüssel speichern</button
          >
          <button class="secondary" type="button" onclick={() => (keyMode = "choose")}
            >Zurück</button
          >
        </div>
      </form>
    {:else}
      <div class="actions">
        {#if status.available}<button
            class="primary"
            type="button"
            disabled={working}
            onclick={createKey}>Sicherung einrichten</button
          >{/if}
        <button
          class="secondary"
          type="button"
          disabled={working}
          onclick={() => (keyMode = "import")}>Ich habe schon einen Schlüssel</button
        >
      </div>
    {/if}
  {:else}
    <dl class="facts">
      <dt>Letzte Sicherung</dt>
      <dd>{status.lastBackupAt ? dateTime(status.lastBackupAt) : "noch keine"}</dd>
      <dt>Letzte Prüfung</dt>
      <dd>{status.lastVerifiedAt ? dateTime(status.lastVerifiedAt) : "noch keine"}</dd>
      {#if account && account.quotaBytes}
        <dt>Speicher</dt>
        <dd class:warning-text={storageFull(account.usedBytes, account.quotaBytes)}>
          {formatBackupSize(account.usedBytes)} von {formatBackupSize(account.quotaBytes)}
          {#if storageFull(account.usedBytes, account.quotaBytes)}
            · Der Speicher wird knapp. Schreib uns, bevor er voll ist.
          {/if}
        </dd>
      {/if}
      {#if status.pending}
        <dt>Ausstehend</dt>
        <dd>Änderungen werden spätestens nach einer Stunde und beim Beenden gesichert.</dd>
      {/if}
      {#if status.lastError}
        <dt>Letzter Fehler</dt>
        <dd class="error-text">{status.lastError} ({dateTime(status.lastErrorAt)})</dd>
      {/if}
    </dl>
    {#if status.available}
      <label class="check">
        <input type="checkbox" checked={status.auto} onchange={setAuto} />
        <span>Automatisch sichern</span>
      </label>
    {/if}
    <div class="actions">
      {#if status.available}<button
          class="primary"
          type="button"
          disabled={working || status.running}
          onclick={backupNow}>{status.running ? "Sicherung läuft …" : "Jetzt sichern"}</button
        >{/if}
      {#if versions.length}<button
          class="secondary"
          type="button"
          disabled={working}
          onclick={() => verify(versions[0].id)}>Letzte Sicherung prüfen</button
        >{/if}
    </div>
    {#if verificationDue(status.lastVerifiedAt, new Date()) && versions.length}
      <p class="settings-help">
        Prüfe deine Sicherung einmal im Jahr, damit du weißt, dass sie sich wiederherstellen lässt.
      </p>
    {/if}

    {#if status.keyCheckDue}
      <form
        class="key-step"
        onsubmit={(event) => {
          event.preventDefault();
          void checkKey();
        }}
      >
        <h4>Findest du deinen Schlüssel noch?</h4>
        <p>
          Gib ihn zur Probe ein, dann weißt du, dass du im Ernstfall an deine Daten kommst. Das ist
          freiwillig, deine Sicherung läuft in jedem Fall weiter.
        </p>
        <RecoveryKeyInput bind:value={checkedKey} label="Dein Schlüssel" />
        <div class="actions">
          <button class="secondary" type="submit" disabled={working || !keyComplete(checkedKey)}
            >Schlüssel prüfen</button
          >
          <button class="text" type="button" disabled={working} onclick={postponeKeyCheck}
            >Später erinnern</button
          >
        </div>
      </form>
    {/if}

    {#if preview}
      <div class="restore-preview" role="region" aria-label="Vorschau der Wiederherstellung">
        <h4>Diesen Stand wiederherstellen?</h4>
        <p>
          Stand vom {dateTime(preview.data.backup.createdAt)}{preview.data.backup.deviceName
            ? ` (${preview.data.backup.deviceName})`
            : ""}:
          {preview.data.backup.customers} Kunden, {preview.data.backup.products} Artikel, {preview
            .data.backup.invoices} Rechnungen und Entwürfe, {preview.data.backup.received} Eingangsbelege,
          {preview.data.backup.expenses} Ausgaben, insgesamt {preview.data.backup.documents.toLocaleString(
            "de-DE",
          )} Dateien ({formatBackupSize(preview.data.backup.bytes)}).
        </p>
        {#if loss}<p class="backup-message warning">{loss}</p>{/if}
        <p class="backup-message warning">
          Die aktuellen Daten auf diesem Gerät werden komplett ersetzt. Den jetzigen Stand legt die
          App vorher im Datenverzeichnis unter „Sicherungen“ ab. Bank-PIN und E-Mail-Passwörter
          musst du danach neu eingeben.
        </p>
        <label class="check">
          <input type="checkbox" bind:checked={restoreConfirmed} />
          <span>Ich will die aktuellen Daten durch diesen Stand ersetzen.</span>
        </label>
        <div class="actions">
          <button
            class="primary"
            type="button"
            disabled={working || !restoreConfirmed}
            onclick={restore}>Wiederherstellen</button
          >
          <button class="secondary" type="button" onclick={() => (preview = null)}>Abbrechen</button
          >
        </div>
      </div>
    {/if}

    <SettingSection
      title="Gespeicherte Stände"
      action="Ansehen"
      summary={latestVersion
        ? `${versions.length} ${versions.length === 1 ? "Stand" : "Stände"}, zuletzt ${dateTime(latestVersion.created)}`
        : "Noch keine Sicherung vorhanden"}
    >
      {#if versions.length === 0}
        <p class="settings-help">Noch keine Sicherung vorhanden.</p>
      {:else}
        <ul class="backup-versions">
          {#each versions as version (version.id)}
            <li>
              <span
                ><strong>{dateTime(version.created)}</strong>
                <small
                  >{version.deviceName || "Unbekanntes Gerät"} · {formatBackupSize(
                    version.logicalSize,
                  )}</small
                ></span
              >
              <span class="row-actions">
                <button
                  class="text"
                  type="button"
                  disabled={working}
                  onclick={() => verify(version.id)}>Prüfen</button
                >
                <button
                  class="text"
                  type="button"
                  disabled={working}
                  onclick={() => saveVersion(version)}>Als Datei</button
                >
                <button
                  class="secondary"
                  type="button"
                  disabled={working}
                  onclick={() => openPreview(version.id)}>Wiederherstellen …</button
                >
              </span>
            </li>
          {/each}
        </ul>
      {/if}
    </SettingSection>

    <SettingSection
      title="Dein Schlüssel"
      action="Verwalten"
      summary="Anzeigen, drucken oder von diesem Gerät entfernen"
    >
      <p class="settings-help">
        Lass ihn dir anzeigen, wenn du deine Notiz nicht mehr findest. Das geht, solange dieses
        Gerät läuft.
      </p>
      {#if shownKey}
        {@render keyBlock(shownKey)}
        <div class="actions">
          <button class="secondary" type="button" onclick={printKey}>Drucken</button>
          <button class="secondary" type="button" onclick={copyKey}>Kopieren</button>
          <button class="secondary" type="button" onclick={() => (shownKey = "")}>Ausblenden</button
          >
        </div>
      {:else}
        <div class="actions">
          <button class="secondary" type="button" disabled={working} onclick={showKey}
            >Schlüssel anzeigen</button
          >
        </div>
      {/if}
      <p class="settings-help">
        Entfernst du ihn von diesem Gerät, bleiben deine Sicherungen erhalten. Neue entstehen erst
        wieder, wenn du den Schlüssel erneut eingibst.
      </p>
      {#if confirmRemoveKey}
        <p class="backup-message warning">
          Hast du den Schlüssel notiert? Ohne ihn kommst du nicht mehr an deine Sicherungen.
        </p>
        <div class="actions">
          <button class="secondary" type="button" disabled={working} onclick={removeKey}
            >Ja, entfernen</button
          >
          <button class="text" type="button" onclick={() => (confirmRemoveKey = false)}
            >Abbrechen</button
          >
        </div>
      {:else}
        <div class="actions">
          <button
            class="text"
            type="button"
            disabled={working}
            onclick={() => (confirmRemoveKey = true)}>Schlüssel von diesem Gerät entfernen</button
          >
        </div>
      {/if}
    </SettingSection>

    <SettingSection
      title="Protokoll"
      action="Ansehen"
      summary={log.length
        ? `${log.length} ${log.length === 1 ? "Eintrag" : "Einträge"}, zuletzt ${dateTime(log[0].at)}`
        : "Noch keine Prüfung oder Wiederherstellung"}
    >
      {#if log.length === 0}
        <p class="settings-help">Noch keine Prüfung oder Wiederherstellung.</p>
      {:else}
        <ul class="backup-log">
          {#each log.slice(0, 50) as event (event.id)}
            <li class:failed={!event.ok}>
              <small>{dateTime(event.at)}</small>
              <span>{event.summary}</span>
            </li>
          {/each}
        </ul>
      {/if}
    </SettingSection>

    <SettingSection
      title="Alle Sicherungen löschen"
      action="Löschen …"
      summary={account?.deletionAt
        ? "Löschung ist vorgemerkt"
        : "Mit 14 Tagen Wartezeit, die Daten auf diesem Gerät bleiben"}
    >
      <p class="settings-help">
        Wir löschen alles nach einer Wartezeit von 14 Tagen von unserem Server. Du bekommst dazu
        eine E-Mail und kannst die Löschung bis dahin abbrechen. Die Daten auf diesem Gerät bleiben
        erhalten.
      </p>
      <label class="check">
        <input type="checkbox" bind:checked={deleteConfirmed} disabled={!!account?.deletionAt} />
        <span>Ich will alle Sicherungen löschen.</span>
      </label>
      <div class="actions">
        <button
          class="secondary"
          type="button"
          disabled={working || !deleteConfirmed || !!account?.deletionAt}
          onclick={deleteAll}>Löschung vormerken</button
        >
      </div>
    </SettingSection>
  {/if}

  <SettingSection
    title="Was gesichert wird"
    action="Lesen"
    summary="Rechnungen, Belege, Kunden, Zahlungen, Bankumsätze und Einstellungen"
  >
    <p class="settings-help">
      Alles, was du für deine Aufbewahrung brauchst: Rechnungen, Belege im Original, Kunden,
      Zahlungen, Bankumsätze und Einstellungen. Bank-PIN und E-Mail-Passwörter sichern wir bewusst
      nicht. Ältere Stände bleiben bis zu elf Jahre erhalten, ein gesicherter Beleg wird nie
      überschrieben.
    </p>
    <p class="settings-help">
      Die Sicherung schützt vor Datenverlust. Für die Aufbewahrung deiner Unterlagen bleibst du
      selbst verantwortlich, auch nach einem Geräte- oder Programmwechsel.
    </p>
  </SettingSection>
</section>

<style>
  .actions {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 8px;
    margin: 12px 0 0;
  }
  .recovery-key {
    display: block;
    width: fit-content;
    max-width: 100%;
    margin: 12px 0 0;
    padding: 12px 14px;
    border: 1px solid var(--theme-border);
    border-radius: 8px;
    background: var(--theme-surface);
    font-family: ui-monospace, "SF Mono", Menlo, monospace;
    font-size: 18px;
    line-height: 1.7;
    user-select: all;
    overflow-wrap: anywhere;
  }
  .recovery-key span {
    display: block;
  }
  .key-print {
    display: none;
  }
  @media print {
    :global(html:has(body.print-recovery-key)),
    :global(body.print-recovery-key) {
      height: auto !important;
      min-height: 0 !important;
      overflow: visible !important;
    }
    :global(body.print-recovery-key > *) {
      display: none !important;
    }
    :global(body.print-recovery-key > .key-print) {
      display: block !important;
      color: var(--paper-text);
      font-size: 13pt;
      line-height: 1.5;
    }
    .key-print h1 {
      margin: 0 0 4pt;
      font-size: 20pt;
    }
    .key-print p {
      max-width: 15cm;
    }
    .key-print .recovery-key {
      margin: 18pt 0;
      padding: 12pt 16pt;
      border: 1pt solid currentColor;
      background: none;
      color: inherit;
      font-size: 17pt;
    }
  }
  .key-step {
    margin-top: 14px;
    padding: 16px;
    border: 1px solid var(--theme-border);
    border-radius: 10px;
  }
  .key-step h4 {
    margin: 0 0 4px;
  }
  .key-step p {
    margin: 0;
    color: var(--theme-muted);
    line-height: 1.6;
  }
  .key-tips {
    margin: 12px 0 0;
    padding-left: 18px;
    color: var(--theme-muted);
    line-height: 1.6;
  }
  .check {
    display: flex;
    align-items: center;
    gap: 8px;
    margin-top: 12px;
    cursor: pointer;
  }
  .check input {
    flex-shrink: 0;
  }
  .backup-message {
    overflow-wrap: anywhere;
    margin: 12px 0 0;
    padding: 10px 12px;
    border-radius: 8px;
    border: 1px solid var(--theme-info-border);
    background: var(--theme-info-bg);
    color: var(--theme-info-text);
  }
  .backup-message p {
    margin: 0 0 6px;
  }
  .backup-message.warning {
    border-color: var(--theme-warning-border);
    background: var(--theme-warning-bg);
    color: var(--theme-warning-text);
  }
  .backup-message.error {
    border-color: var(--theme-error-border);
    background: var(--theme-error-bg);
    color: var(--theme-error-text);
  }
  .error-text {
    color: var(--theme-error-text);
  }
  .warning-text {
    color: var(--theme-warning-text);
  }
  .facts {
    display: grid;
    grid-template-columns: max-content minmax(0, 1fr);
    gap: 6px 18px;
    margin: 12px 0 0;
  }
  .facts dt {
    color: var(--theme-muted);
  }
  .facts dd {
    margin: 0;
    overflow-wrap: anywhere;
  }
  .backup-versions {
    list-style: none;
    margin: 8px 0 0;
    padding: 0;
  }
  .backup-versions li {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: 8px;
    padding: 8px 0;
    border-bottom: 1px solid var(--theme-border);
  }
  .backup-versions li > span:first-child {
    display: flex;
    flex-direction: column;
    min-width: 0;
    overflow-wrap: anywhere;
  }
  .backup-versions strong {
    font-weight: 500;
  }
  .row-actions {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 8px;
    margin-left: auto;
  }
  .restore-preview {
    margin-top: 14px;
    padding: 16px;
    border: 1px solid var(--theme-border);
    border-radius: 10px;
  }
  .restore-preview h4 {
    margin: 0 0 6px;
  }
  .restore-preview p {
    margin: 0;
    line-height: 1.6;
    overflow-wrap: anywhere;
  }
  .backup-log {
    list-style: none;
    margin: 8px 0 0;
    padding: 0;
  }
  .backup-log li {
    display: flex;
    flex-direction: column;
    gap: 2px;
    overflow-wrap: anywhere;
    padding: 6px 0;
    border-bottom: 1px solid var(--theme-border);
  }
  .backup-log small {
    color: var(--theme-muted);
  }
  .backup-log .failed span {
    color: var(--theme-error-text);
  }
</style>
