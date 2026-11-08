<script lang="ts">
  import SettingSection from "../../components/SettingSection.svelte";
  import { onMount } from "svelte";
  import { Dialogs } from "@wailsio/runtime";
  import * as API from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/app/auditservice";
  import type {
    AuditEntry,
    AuditReport,
    ProcedureProfile,
  } from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/store/models";
  import type {
    AuditExportResult,
    AuditFinding,
    AuditStatus,
  } from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/app/models";
  import { auditActionLabel, auditView, defaultAuditPeriod } from "../../lib/audit";
  import { cached, remember } from "../../lib/cache";
  import { serviceErrorMessage } from "../../lib/errors";
  import { confirmationLine } from "../../lib/securityLevel";
  import { dateLabel, localToday } from "../../lib/format";
  import AuditDataView from "./AuditDataView.svelte";

  let { onplan }: { onplan: () => void } = $props();

  const paperChoices = [
    { id: "none", label: "Es fallen keine Papierbelege an" },
    { id: "keep", label: "Ich bewahre Papierbelege im Original auf" },
    { id: "scan-keep", label: "Ich scanne sie und bewahre das Papier zusätzlich auf" },
    { id: "scan-destroy", label: "Ich scanne sie und vernichte das Papier danach" },
  ];

  const period = defaultAuditPeriod(new Date());
  // The state of the last visit shows right away while it reloads.
  let status = $state<AuditStatus | null>(cached<AuditStatus>("audit:status") ?? null);
  let report = $state<AuditReport | null>(null);
  let entries = $state<AuditEntry[]>([]);
  let logQuery = $state("");
  let findings = $state<AuditFinding[] | null>(null);
  let exported = $state<AuditExportResult | null>(null);
  let showData = $state(false);
  // The form edits the profile in place, so the cache holds a copy of the stored one.
  const copy = (value: ProcedureProfile): ProcedureProfile => ({ ...value });
  const warmProfile = cached<ProcedureProfile>("audit:profile");
  let profile = $state<ProcedureProfile>(
    warmProfile
      ? copy(warmProfile)
      : {
          responsible: "",
          advisor: "",
          paperReceipts: "",
          cash: false,
          otherSystems: "",
          devices: "",
          backupRoutine: "",
        },
  );
  let from = $state(period.from);
  let to = $state(period.to);
  let lockTo = $state("");
  let lockConfirmed = $state(false);
  let busy = $state(false);
  let error = $state("");
  let message = $state("");

  const plan = $derived(status?.plan ?? false);
  const viewOpen = $derived($auditView.open);
  const today = localToday();

  async function load() {
    try {
      status = remember("audit:status", await API.Status());
      auditView.set(status.view);
      const shown = JSON.stringify(profile);
      const stored = remember("audit:profile", await API.ProcedureProfile());
      // Input started on the cached profile stays.
      if (JSON.stringify(profile) === shown) profile = copy(stored);
    } catch (cause) {
      error = serviceErrorMessage(cause, "Der Stand ließ sich nicht laden.");
    }
  }
  onMount(load);

  async function run(work: () => Promise<void>, fallback: string) {
    if (busy) return;
    busy = true;
    error = "";
    message = "";
    try {
      await work();
    } catch (cause) {
      error = serviceErrorMessage(cause, fallback);
    } finally {
      busy = false;
    }
  }

  const verify = () =>
    run(async () => {
      report = await API.Verify();
    }, "Die Prüfung ließ sich nicht ausführen.");

  async function loadLog() {
    entries =
      (await API.Entries({ from: "", to: "", object: "", query: logQuery, limit: 200 })) ?? [];
  }

  const lock = () =>
    run(async () => {
      await API.LockPeriod("", lockTo);
      lockTo = "";
      lockConfirmed = false;
      message = "Der Zeitraum ist festgeschrieben.";
      await load();
    }, "Der Zeitraum ließ sich nicht festschreiben.");

  const check = () =>
    run(async () => {
      findings = (await API.Findings(from, to)) ?? [];
    }, "Die Prüfung ließ sich nicht ausführen.");

  const exportPackage = () =>
    run(async () => {
      const path = await Dialogs.SaveFile({
        Title: "Prüfer-Export speichern",
        Filename: `Pruefer-Export_${from}_${to}.zip`,
      });
      if (!path) return;
      exported = await API.Export(path, from, to);
      await load();
    }, "Der Export ließ sich nicht erstellen.");

  const openView = () =>
    run(async () => {
      auditView.set(await API.OpenView(from, to));
    }, "Die Nur-Lese-Ansicht ließ sich nicht öffnen.");

  const saveProfile = () =>
    run(async () => {
      await API.SaveProcedureProfile(profile);
      remember("audit:profile", copy(profile));
      const version = await API.CreateProcedure();
      message = `Version ${version.version} ist erstellt.`;
      await load();
    }, "Die Verfahrensdokumentation ließ sich nicht erstellen.");

  const saveProcedure = (version: number) =>
    run(async () => {
      const path = await Dialogs.SaveFile({
        Title: "Verfahrensdokumentation speichern",
        Filename: `Verfahrensdokumentation_V${version}.pdf`,
      });
      if (path) await API.SaveProcedure(path, version);
    }, "Die Datei ließ sich nicht speichern.");

  function stamp(value: string) {
    return dateLabel(value.slice(0, 10));
  }
</script>

<div class="audit-settings" aria-busy={busy}>
  {#if error}<p class="audit-message error" role="alert">{error}</p>{/if}
  {#if message}<p class="audit-message" role="status">{message}</p>{/if}

  <section class="settings-group" aria-labelledby="audit-log-title">
    <h2 id="audit-log-title">Änderungsprotokoll</h2>
    <p class="settings-help">
      Die App hält jede Änderung an Belegen, Rechnungen, Zahlungen und steuerlichen Einstellungen
      fest, mit dem Inhalt davor und danach. Einträge lassen sich nicht ändern, und jeder enthält
      einen Fingerabdruck des vorherigen.
    </p>
    {#if status}
      <dl class="audit-facts">
        <dt>Einträge</dt>
        <dd>{status.entries}</dd>
        <dt>Externe Bestätigung</dt>
        <dd>
          {confirmationLine(
            {
              plan,
              firstAnchor: status.firstAnchor,
              lastAnchor: status.lastAnchor,
              comparedAt: status.health.comparedAt,
              conflicts: status.health.conflicts,
            },
            new Date(),
          )}
        </dd>
      </dl>
      {#if status.health.conflicts > 0}
        <div class="audit-message warning" role="alert">
          <p><strong>Das Protokoll passt nicht zur externen Liste</strong></p>
          <p>
            Bestätigte Stände stimmen nicht mehr mit diesem Protokoll überein. Das kann nach einer
            Wiederherstellung vorkommen oder wenn sich die Daten außerhalb der App geändert haben.
            Schreib uns, damit wir es gemeinsam ansehen.
          </p>
        </div>
      {:else if status.health.stale}
        <div class="audit-message warning" role="status">
          <p><strong>Seit über einer Woche keine neue Bestätigung</strong></p>
          <p>Prüfe deine Internetverbindung und unter „Konto & Abo“, ob dein Abo aktiv ist.</p>
        </div>
      {:else if status.health.restored > 0}
        <p class="audit-message" role="status">
          {status.health.restored === 1
            ? "1 Nachweis wurde aus der externen Liste zurückgeholt."
            : `${status.health.restored} Nachweise wurden aus der externen Liste zurückgeholt.`}
        </p>
      {/if}
    {/if}
    <div class="audit-actions">
      <button type="button" class="secondary" disabled={busy} onclick={verify}
        >{busy && !report ? "Wird geprüft …" : "Jetzt prüfen"}</button
      >
    </div>
    {#if report}
      {#if report.ok}<p class="audit-message" role="status">
          Alles in Ordnung: {report.entries} Einträge, {report.objects} Datensätze und {report.files}
          Dateien passen zusammen.
        </p>
      {:else}<div class="audit-message error" role="alert">
          <p>Die Prüfung hat Abweichungen gefunden. Sichere deine Daten und kläre sie:</p>
          <ul>
            {#each report.problems ?? [] as p}<li>{p.label}: {p.message}</li>{/each}
          </ul>
        </div>{/if}
    {/if}
    <SettingSection
      title="Protokolleinträge"
      action="Ansehen"
      summary={status ? `${status.entries} Einträge, durchsuchbar` : ""}
      onopen={() => void loadLog()}
    >
      <label class="audit-search"
        >Suchen<input bind:value={logQuery} type="search" oninput={loadLog} /></label
      >
      <ul class="audit-log">
        {#each entries as e (e.seq)}
          <li>
            <span class="audit-log-date">{stamp(e.at)}</span>
            <span><strong>{auditActionLabel(e.action)}</strong> {e.label}</span>
            {#if e.reason}<small>{e.reason}</small>{/if}
          </li>
        {/each}
      </ul>
    </SettingSection>
  </section>

  <section class="settings-group" aria-labelledby="audit-lock-title">
    <h2 id="audit-lock-title">Festschreibung</h2>
    <p class="settings-help">
      Ist ein Zeitraum festgeschrieben, lassen sich seine Belege nicht mehr ändern, nur noch
      stornieren. Vermerkst du eine Steuermeldung als übermittelt, geschieht das für ihren Zeitraum
      automatisch. Von Hand brauchst du es, wenn du deine Daten an die Steuerkanzlei gibst.
    </p>
    {#if status?.locks?.length}<ul class="audit-locks">
        {#each status.locks as l (l.id)}<li>
            Bis {dateLabel(l.to)}, festgeschrieben am {stamp(l.at)}
          </li>{/each}
      </ul>{/if}
    <div class="audit-row">
      <label>Festschreiben bis<input type="date" max={today} bind:value={lockTo} /></label>
      <label class="audit-check"
        ><input type="checkbox" bind:checked={lockConfirmed} /> Mir ist klar, dass sich das nicht zurücknehmen
        lässt.</label
      >
      <button
        type="button"
        class="secondary"
        disabled={busy || !lockTo || !lockConfirmed || viewOpen}
        onclick={lock}>Festschreiben</button
      >
    </div>
  </section>

  <section class="settings-group" aria-labelledby="audit-exam-title">
    <h2 id="audit-exam-title">Betriebsprüfung</h2>
    <p class="settings-help">
      Das Finanzamt entscheidet, wie es auf deine Daten zugreift: Es liest selbst, lässt sich
      Auswertungen geben oder bekommt die Daten als Datei. Du gibst die Daten heraus, und zwar nur
      den Prüfungszeitraum.
    </p>
    <div class="audit-row">
      <label>Prüfungszeitraum von<input type="date" bind:value={from} /></label>
      <label>bis<input type="date" bind:value={to} /></label>
    </div>
    <div class="audit-actions">
      <button type="button" class="secondary" disabled={busy} onclick={check}
        >Vollständigkeit prüfen</button
      >
      <button type="button" class="primary" disabled={busy || viewOpen} onclick={exportPackage}
        >Prüfer-Export erstellen</button
      >
      <button type="button" class="secondary" onclick={() => (showData = !showData)}
        >{showData ? "Datenansicht schließen" : "Daten ansehen und auswerten"}</button
      >
      <button type="button" class="secondary" disabled={busy || viewOpen} onclick={openView}
        >Nur-Lese-Ansicht öffnen</button
      >
    </div>
    {#if findings}
      {#if findings.length === 0}<p class="audit-message" role="status">
          Für diesen Zeitraum ist nichts aufgefallen.
        </p>{/if}
      {#each findings as f}
        <div class="audit-message" class:warning={f.level === "warning"}>
          <p><strong>{f.title}</strong></p>
          <p>{f.detail}</p>
          <ul>
            {#each (f.items ?? []).slice(0, 12) as item}<li>{item}</li>{/each}
            {#if (f.items ?? []).length > 12}<li>
                und {(f.items ?? []).length - 12} weitere
              </li>{/if}
          </ul>
        </div>
      {/each}
    {/if}
    {#if exported}
      <div class="audit-message" role="status">
        <p>
          <strong>Export erstellt:</strong>
          {exported.files} Dokumente, {Math.ceil(exported.bytes / 1024 / 1024)} MB.
        </p>
        <p class="audit-path">{exported.path}</p>
        <p>
          Lade die Datei über den Link hoch, den dir die prüfende Person schickt, oder gib sie
          deiner Steuerkanzlei. Das Archiv ist unverschlüsselt, weil die Plattformen der
          Finanzverwaltung verschlüsselte Archive abweisen. Auf einem USB-Stick verschlüsselst du
          den Stick selbst.
        </p>
        <p class="audit-path">Prüfsumme: {exported.sum}</p>
      </div>
    {/if}
    {#if showData}<AuditDataView {from} {to} />{/if}
    {#if !plan}
      <p class="settings-help">
        Mit Plus wird dein Änderungsprotokoll täglich extern bestätigt und geprüft, und wir bewahren
        die Nachweise zusätzlich für dich auf.
      </p>
      <button type="button" class="secondary" onclick={onplan}>Zu Konto & Abo</button>
    {/if}
  </section>

  <section class="settings-group" aria-labelledby="audit-procedure-title">
    <h2 id="audit-procedure-title">Verfahrensdokumentation</h2>
    <p class="settings-help">
      Sie beschreibt, wie du mit Belegen umgehst. Die App schreibt sie aus deinen Angaben und dem,
      was sie über sich weiß. Lies sie und ergänze, was bei dir anders läuft. Ändert sich dein
      Ablauf, erstellst du eine neue Version; alte Versionen bleiben erhalten.
    </p>
    <form
      class="audit-form"
      onsubmit={(event) => {
        event.preventDefault();
        void saveProfile();
      }}
    >
      <label class="wide"
        >Wer erfasst und prüft die Belege?<input
          bind:value={profile.responsible}
          required
          placeholder="z. B. Erika Beispiel, Inhaberin"
        /></label
      >
      <label class="wide"
        >Wie gehst du mit Papierbelegen um?<select bind:value={profile.paperReceipts} required>
          <option value="" disabled>Auswählen …</option>
          {#each paperChoices as c (c.id)}<option value={c.id}>{c.label}</option>{/each}
        </select></label
      >
      <label>Steuerkanzlei (falls vorhanden)<input bind:value={profile.advisor} /></label>
      <label>Rechner mit der App<input bind:value={profile.devices} /></label>
      <label class="wide"
        >Weitere Systeme mit steuerlichen Daten<input
          bind:value={profile.otherSystems}
          placeholder="z. B. Onlineshop, Kassensystem"
        /></label
      >
      <label class="wide"
        >Wie sicherst du deine Daten?<input
          bind:value={profile.backupRoutine}
          placeholder="z. B. verschlüsselte Sicherung der App, zusätzlich jährlich auf einer Festplatte"
        /></label
      >
      <label class="audit-check wide"
        ><input type="checkbox" bind:checked={profile.cash} /> Ich habe Bareinnahmen</label
      >
      <div class="audit-actions wide">
        <button class="primary" disabled={busy || viewOpen}>Neue Version erstellen</button>
      </div>
    </form>
    {#if status?.procedure?.length}<ul class="audit-versions">
        {#each status.procedure as v (v.version)}<li>
            <span>Version {v.version} vom {stamp(v.at)}</span>
            <button type="button" class="text" onclick={() => saveProcedure(v.version)}
              >Als PDF speichern</button
            >
          </li>{/each}
      </ul>{/if}
  </section>
</div>

<style>
  .audit-settings {
    display: grid;
    gap: 8px;
    min-width: 0;
  }
  .audit-settings > :global(.settings-group:first-of-type) {
    margin-top: 0;
    padding-top: 0;
    border-top: 0;
  }
  .audit-message {
    overflow-wrap: anywhere;
    margin: 8px 0;
    padding: 10px 12px;
    border-radius: 8px;
    border: 1px solid var(--theme-info-border);
    background: var(--theme-info-bg);
    color: var(--theme-info-text);
    line-height: 1.45;
  }
  .audit-message p {
    margin: 0 0 6px;
  }
  .audit-message p:last-child {
    margin-bottom: 0;
  }
  .audit-message ul {
    margin: 4px 0 0;
    padding-left: 18px;
  }
  .audit-message.warning {
    border-color: var(--theme-warning-border);
    background: var(--theme-warning-bg);
    color: var(--theme-warning-text);
  }
  .audit-message.error {
    border-color: var(--theme-error-border);
    background: var(--theme-error-bg);
    color: var(--theme-error-text);
  }
  .audit-path {
    overflow-wrap: anywhere;
    font-size: 12px;
  }
  /* Same scale as the other settings tabs: values 13px, disclosures 14px. */
  .audit-facts {
    display: grid;
    grid-template-columns: max-content minmax(0, 1fr);
    gap: 6px 18px;
    margin: 0 0 12px;
    font-size: 13px;
  }
  .audit-facts dt {
    color: var(--theme-muted);
  }
  .audit-facts dd {
    margin: 0;
    overflow-wrap: anywhere;
  }
  .audit-actions {
    display: flex;
    flex-wrap: wrap;
    gap: 8px;
    margin: 10px 0;
  }
  .audit-row {
    display: flex;
    flex-wrap: wrap;
    align-items: end;
    gap: 12px;
    margin-top: 10px;
  }
  .audit-row label {
    display: grid;
    gap: 4px;
  }
  .audit-check {
    display: flex !important;
    align-items: center;
    gap: 8px;
    font-weight: 400;
  }
  .audit-check input {
    flex-shrink: 0;
  }
  .audit-search {
    display: grid;
    gap: 4px;
    margin: 10px 0;
    max-width: 320px;
  }
  .audit-log {
    list-style: none;
    margin: 0;
    padding: 0;
    max-height: 340px;
    overflow: auto;
    overscroll-behavior: contain;
    border: 1px solid var(--theme-border);
    border-radius: 8px;
  }
  .audit-log li {
    display: grid;
    grid-template-columns: 84px minmax(0, 1fr);
    gap: 2px 12px;
    overflow-wrap: anywhere;
    padding: 7px 10px;
    border-top: 1px solid var(--theme-border);
    font-size: 12px;
  }
  .audit-log li:first-child {
    border-top: 0;
  }
  .audit-log small {
    grid-column: 2;
    color: var(--theme-muted);
  }
  .audit-log-date {
    color: var(--theme-muted);
    overflow-wrap: normal;
  }
  .audit-locks,
  .audit-versions {
    list-style: none;
    margin: 8px 0;
    padding: 0;
  }
  .audit-versions li {
    display: flex;
    flex-wrap: wrap;
    gap: 12px;
    padding: 6px 0;
    border-top: 1px solid var(--theme-border);
  }
  .audit-versions li > button {
    margin-left: auto;
  }
  .audit-form {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 12px;
    margin-top: 10px;
  }
  .audit-form label {
    display: grid;
    gap: 4px;
    min-width: 0;
  }
  .audit-form .wide {
    grid-column: 1 / -1;
  }
  @media (max-width: 640px) {
    .audit-form {
      grid-template-columns: 1fr;
    }
  }
</style>
