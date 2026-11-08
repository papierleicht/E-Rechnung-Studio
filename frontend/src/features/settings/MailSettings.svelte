<script lang="ts">
  import SettingSection from "../../components/SettingSection.svelte";
  import { onMount } from "svelte";
  import * as MailAPI from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/app/mailservice";
  import type {
    Settings,
    Preset,
  } from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/mail/models";
  import {
    applyMailPreset,
    portForSecurity,
    securityChoices,
    unknownPlaceholders,
  } from "../../lib/mail";
  import { cached, remember } from "../../lib/cache";
  import { serviceErrorMessage } from "../../lib/errors";
  import ProviderList from "../../components/ProviderList.svelte";
  import Monogram from "../../components/Monogram.svelte";
  // compact: embedded in the first-run setup (no heading, no templates).
  let {
    busy = false,
    compact = false,
    onsaved,
  }: { busy?: boolean; compact?: boolean; onsaved?: () => void } = $props();
  let manual = $state(false);
  // Stays open while it is filled in; a server name must not close it again.
  let serverOpen = $state(false);
  // The provider list only shows while choosing; afterwards one summary row.
  let choosing = $state(false);
  type Loaded = { settings: Settings; presets: Preset[]; placeholders: Record<string, string[]> };
  // The stored settings of the last visit show right away while they reload. The form
  // edits them in place, so the cache holds copies and never unsaved input.
  function copy<T>(value: T): T {
    return JSON.parse(JSON.stringify(value));
  }
  const warm = cached<Loaded>("settings:mail");
  let settings = $state<Settings | null>(warm ? copy(warm.settings) : null);
  let presets = $state<Preset[]>(warm?.presets ?? []);
  // Document types with an editable e-mail template.
  const templateKinds = [
    { id: "invoice", label: "Vorlage für Rechnungs-E-Mails", source: "der ausgestellten Rechnung" },
    { id: "quote", label: "Vorlage für Angebots-E-Mails", source: "dem abgeschlossenen Angebot" },
  ] as const;
  let placeholders = $state<Record<string, string[]>>(warm?.placeholders ?? {});
  function keep(value: Settings) {
    value.templates ??= {};
    remember<Loaded>("settings:mail", { settings: copy(value), presets, placeholders });
    return value;
  }
  let password = $state("");
  let working = $state(false);
  let message = $state("");
  let error = $state("");
  let loadError = $state("");
  const preset = $derived(presets.find((item) => item.id === settings?.account.preset));
  const chosen = $derived(Boolean(manual || settings?.account.preset || settings?.account.host));
  const entries = $derived([
    ...presets.map((item) => ({
      id: item.id,
      name: item.label.split(" / ")[0],
      detail: item.label.includes(" / ") ? item.label.split(" / ").slice(1).join(" / ") : "",
    })),
    { id: "", name: "Eigener Server", detail: "SMTP manuell eintragen" },
  ]);
  const listValue = $derived(preset ? preset.id : chosen ? "" : "__none");
  function chooseEntry(id: string) {
    manual = id === "";
    serverOpen = manual && !settings?.account.host;
    choosePreset(id);
    choosing = false;
  }
  const chosenName = $derived(preset ? preset.label.split(" / ")[0] : "Eigener Server");
  const ready = $derived(
    Boolean(settings?.account.host && settings.account.fromAddress && settings.account.username),
  );
  function templateIssues(kind: string): string[] {
    const template = settings?.templates?.[kind];
    return template
      ? unknownPlaceholders(template.subject + "\n" + template.body, placeholders[kind] ?? [])
      : [];
  }
  onMount(async () => {
    const shown = JSON.stringify(settings);
    try {
      const [loaded, presetList, ...names] = await Promise.all([
        MailAPI.Settings(),
        MailAPI.Presets(),
        ...templateKinds.map((kind) => MailAPI.Placeholders(kind.id)),
      ]);
      presets = presetList ?? [];
      placeholders = Object.fromEntries(templateKinds.map((kind, i) => [kind.id, names[i] ?? []]));
      keep(loaded);
      // Input started on the cached settings stays.
      if (JSON.stringify(settings) === shown) settings = loaded;
    } catch (cause) {
      loadError = serviceErrorMessage(
        cause,
        "Deine E-Mail-Einstellungen konnten nicht geladen werden.",
      );
    }
  });
  async function perform(action: () => Promise<void>, fallback: string) {
    if (working) return;
    working = true;
    message = "";
    error = "";
    try {
      await action();
    } catch (cause) {
      error = serviceErrorMessage(cause, fallback);
    } finally {
      working = false;
    }
  }
  function choosePreset(id: string) {
    if (settings) settings.account = applyMailPreset(settings.account, presets, id);
  }
  function chooseSecurity(security: string) {
    if (!settings) return;
    settings.account.security = security;
    settings.account.port = portForSecurity(security, settings.account.port);
  }
  const save = () =>
    perform(async () => {
      settings = keep(await MailAPI.SaveSettings(settings!, password));
      password = "";
      message = "E-Mail-Einstellungen gespeichert.";
      onsaved?.();
    }, "Die E-Mail-Einstellungen ließen sich nicht speichern.");
  const sendTest = () =>
    perform(async () => {
      await MailAPI.SendTestMail(settings!.account, password);
      message = `Testmail an ${settings!.account.fromAddress} gesendet. Schau in deinem Postfach nach, ob sie angekommen ist.`;
    }, "Die Testmail ließ sich nicht senden. Prüfe Zugangsdaten und Servereinstellungen.");
  const forget = () =>
    perform(async () => {
      settings = keep(await MailAPI.ForgetPassword());
      message = "Das Passwort ist aus dem Schlüsselbund entfernt.";
    }, "Das Passwort ließ sich nicht entfernen.");
  function resetTemplate(kind: string) {
    if (!settings) return;
    settings.templates = { ...settings.templates };
    delete settings.templates[kind];
    void save();
  }
</script>

<section class="mail-settings" aria-labelledby="mail-settings-title">
  {#if compact}
    <h2 id="mail-settings-title" class="visually-hidden">E-Mail-Versand</h2>
  {:else}
    <h2 id="mail-settings-title">E-Mail-Versand</h2>
    <p class="settings-help">
      Verschicke Rechnungen, Angebote und Zahlungserinnerungen direkt über dein eigenes
      E-Mail-Konto. Das Passwort speichern wir nur im Schlüsselbund deines Betriebssystems, nicht in
      der Datenbank und nicht in der Datensicherung.
    </p>
  {/if}
  {#if loadError}<p class="mail-error" role="alert">{loadError}</p>{/if}
  {#if settings}
    <fieldset class="mail-fields" disabled={busy || working}>
      <h3 class="mail-step">Wo liegt dein E-Mail-Konto?</h3>
      {#if chosen && !choosing}
        <div class="mail-provider-row">
          <Monogram name={chosenName} size={28} />
          <strong>{chosenName}</strong>
          <button type="button" class="text" onclick={() => (choosing = true)}>Ändern</button>
        </div>
      {:else}
        <ProviderList {entries} value={listValue} label="E-Mail-Anbieter" onchoose={chooseEntry} />
      {/if}
      {#if chosen}
        {#if preset?.hint}<p class="mail-hint" role="note">{preset.hint}</p>{/if}
        <h3 class="mail-step">Zugangsdaten</h3>
        <div class="form-grid cols-2">
          <label
            >Absenderadresse<input
              type="email"
              bind:value={settings.account.fromAddress}
              placeholder="rechnung@deine-firma.de"
            /></label
          ><label
            >Absendername<input
              bind:value={settings.account.fromName}
              placeholder="Deine Firma"
            /></label
          ><label
            >Benutzername<input
              bind:value={settings.account.username}
              autocomplete="username"
              placeholder="Meist die E-Mail-Adresse"
            /></label
          ><label
            >Passwort<input
              type="password"
              bind:value={password}
              autocomplete="new-password"
              placeholder={settings.hasPassword
                ? "Im Schlüsselbund gespeichert"
                : "Passwort oder App-Passwort"}
            />{#if settings.hasPassword}<small class="form-help"
                >Leer lassen, um das gespeicherte Passwort zu behalten.</small
              >{/if}</label
          >
        </div>
        <SettingSection
          title="Servereinstellungen"
          summary={settings.account.host
            ? `${settings.account.host}:${settings.account.port}`
            : "Noch kein Server eingetragen"}
          bind:open={serverOpen}
        >
          <div class="form-grid cols-2">
            <label
              >SMTP-Server<input
                bind:value={settings.account.host}
                placeholder="smtp.example.de"
              /></label
            ><label
              >Port<input
                type="number"
                min="1"
                max="65535"
                bind:value={settings.account.port}
              /></label
            ><label
              >Verschlüsselung<select
                value={settings.account.security}
                onchange={(event) => chooseSecurity(event.currentTarget.value)}
                >{#each securityChoices as choice}<option value={choice.id}>{choice.label}</option
                  >{/each}</select
              ></label
            >
          </div>
        </SettingSection>
        <SettingSection
          title="Weitere Optionen"
          summary={`Antworten an ${settings.account.replyTo || "deine Absenderadresse"} · Kopie an dich ${settings.account.bccSelf ? "an" : "aus"}`}
        >
          <div class="form-grid cols-2">
            <label
              >Antwort an (optional)<input
                type="email"
                bind:value={settings.account.replyTo}
              /></label
            ><label class="mail-check"
              ><input type="checkbox" bind:checked={settings.account.bccSelf} /> Kopie (BCC) an mich selbst</label
            >
          </div>
        </SettingSection>
        <div class="mail-actions">
          <button type="button" class="primary" onclick={save}
            >{working ? "Einen Moment …" : "Speichern"}</button
          ><button type="button" class="secondary" disabled={!ready} onclick={sendTest}
            >Testmail senden</button
          >{#if settings.hasPassword}<button type="button" class="text danger-text" onclick={forget}
              >Passwort entfernen</button
            >{/if}
        </div>
      {:else}
        <p class="form-help">
          Wähle deinen Anbieter. Server und Port füllen wir dann für dich aus.
        </p>
      {/if}
      {#if message}<p class="mail-message" role="status">{message}</p>{/if}
      {#if error}<p class="mail-error" role="alert">{error}</p>{/if}
      {#if !compact}
        {#each templateKinds as kind (kind.id)}
          {@const template = settings.templates?.[kind.id]}
          {#if template}
            <SettingSection title={kind.label} summary={`Betreff: ${template.subject}`}>
              <p class="form-help">
                Platzhalter: {(placeholders[kind.id] ?? []).map((name) => `{${name}}`).join(", ")}.
                Die Werte setzt die App beim Versand aus {kind.source} ein.
              </p>
              <label class="mail-template">Betreff<input bind:value={template.subject} /></label
              ><label class="mail-template"
                >Text<textarea rows="9" bind:value={template.body}></textarea></label
              >
              {#if templateIssues(kind.id).length}<p class="mail-error" role="alert">
                  Unbekannter Platzhalter: {templateIssues(kind.id).join(", ")}
                </p>{/if}
              <div class="mail-actions">
                <button type="button" class="secondary" onclick={save}>Vorlage speichern</button
                ><button type="button" class="text" onclick={() => resetTemplate(kind.id)}
                  >Standardtext wiederherstellen</button
                >
              </div>
            </SettingSection>
          {/if}
        {/each}
      {/if}
    </fieldset>
  {:else if !loadError}<p class="form-help">Wird geladen …</p>{/if}
</section>

<style>
  .visually-hidden {
    position: absolute;
    width: 1px;
    height: 1px;
    overflow: hidden;
    clip-path: inset(50%);
    white-space: nowrap;
  }
  .mail-fields {
    border: 0;
    padding: 0;
    margin: 0;
    min-width: 0;
  }
  .mail-actions {
    display: flex;
    flex-wrap: wrap;
    gap: 10px;
    align-items: center;
    margin: 20px 0 12px;
  }
  .mail-step {
    margin: 20px 0 10px;
  }
  .mail-step:first-child {
    margin-top: 0;
  }
  .mail-provider-row {
    display: flex;
    align-items: center;
    gap: 10px;
    min-height: 40px;
  }
  .mail-provider-row strong {
    min-width: 0;
    font-size: 13px;
    font-weight: 600;
    overflow-wrap: anywhere;
  }
  .mail-provider-row button {
    flex-shrink: 0;
    padding: 4px 6px;
  }
  /* Server names and SMTP replies can be one long word. */
  .mail-hint,
  .mail-message,
  .mail-error {
    overflow-wrap: anywhere;
  }
  .mail-hint {
    margin: 14px 0 0;
    padding: 10px 12px;
    border-left: 3px solid var(--theme-accent);
    border-radius: 0 6px 6px 0;
    background: var(--theme-surface-muted);
    font-size: 12px;
    line-height: 1.5;
  }
  .mail-check input {
    flex-shrink: 0;
  }
  .mail-check {
    flex-direction: row;
    align-items: center;
    gap: 8px;
    align-self: end;
  }
  .mail-template {
    display: flex;
    flex-direction: column;
    gap: 6px;
    margin-bottom: 12px;
  }
  .mail-template textarea {
    font: inherit;
    resize: vertical;
  }
  .mail-message {
    color: var(--theme-success-text);
  }
  .mail-error {
    color: var(--theme-error-text);
  }
</style>
