<script lang="ts">
  import SettingSection from "../../components/SettingSection.svelte";
  import { onMount } from "svelte";
  import * as API from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/app/invoiceservice";
  import type { ReminderSettings } from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/store/models";
  import { cached, remember } from "../../lib/cache";
  import { serviceErrorMessage } from "../../lib/errors";
  import { reminderPlaceholders } from "../../lib/reminders";

  let {
    disabled = false,
    onpendingchange = () => {},
  }: {
    disabled?: boolean;
    /** Hands the page a way to save, so its own button covers unsaved texts too. */
    onpendingchange?: (save: (() => Promise<void>) | null) => void;
  } = $props();
  let settings = $state<ReminderSettings | null>(null),
    saved = $state(""),
    busy = $state(false),
    error = $state(""),
    notice = $state("");
  const dirty = $derived(settings !== null && JSON.stringify(settings) !== saved);
  // The stored levels in a few words, e.g. "3 Stufen: Zahlungserinnerung, 1. Mahnung, …".
  const reminderSummary = $derived.by(() => {
    if (!saved) return "";
    const levels: { title: string }[] = JSON.parse(saved).levels ?? [];
    return `${levels.length} ${levels.length === 1 ? "Stufe" : "Stufen"}: ${levels
      .map((level) => level.title)
      .join(", ")}`;
  });
  $effect(() => {
    onpendingchange(dirty ? persist : null);
    return () => onpendingchange(null);
  });

  function accept(value: ReminderSettings) {
    settings = value;
    saved = JSON.stringify(value);
    // A copy: the form edits the settings in place, unsaved edits stay out of the cache.
    remember("settings:reminders", JSON.parse(saved));
  }
  // The stored texts of the last visit show right away; a reload leaves started edits alone.
  const warm = cached<ReminderSettings>("settings:reminders");
  if (warm) accept(JSON.parse(JSON.stringify(warm)));
  onMount(async () => {
    try {
      const fresh = await API.ReminderSettings();
      if (!dirty) accept(fresh);
    } catch (e) {
      error = serviceErrorMessage(
        e,
        "Deine Einstellungen für Erinnerungen konnten nicht geladen werden.",
      );
    }
  });
  async function persist() {
    busy = true;
    error = "";
    notice = "";
    try {
      accept(await API.SaveReminderSettings($state.snapshot(settings) as ReminderSettings));
      notice = "Einstellungen für Erinnerungen gespeichert.";
    } catch (e) {
      error = serviceErrorMessage(
        e,
        "Die Einstellungen für Erinnerungen ließen sich nicht speichern.",
      );
      throw e;
    } finally {
      busy = false;
    }
  }
  async function save() {
    if (!settings || busy) return;
    await persist().catch(() => {});
  }
  async function restoreDefaults() {
    if (!settings) return;
    const defaults = await API.DefaultReminderSettings();
    settings = { ...defaults, interestRate: settings.interestRate };
    notice = "Standardtexte eingesetzt. Speichere, um sie zu übernehmen.";
  }
</script>

<SettingSection title="Zahlungserinnerungen und Mahnungen" summary={reminderSummary}>
  <div class="reminder-settings">
    <p class="form-help">
      Bei überfälligen Kundenrechnungen schlägt die App dir die nächste Stufe vor. Grundlage ist der
      tatsächliche Restbetrag, bei einer Ratenvereinbarung nur die bereits fälligen Raten. Die
      „Wartezeit“ zählt ab Fälligkeit bzw. ab Ablauf der Frist der vorherigen Stufe.
    </p>
    {#if settings}
      {#each settings.levels ?? [] as level, index}
        <section class="settings-group" aria-labelledby={`reminder-level-${index}`}>
          <h3 id={`reminder-level-${index}`}>Stufe {index + 1}: {level.title}</h3>
          <div class="form-grid cols-4">
            <label class="span-1"
              >Bezeichnung<input bind:value={level.title} maxlength="80" /></label
            >
            <label class="span-1"
              >Wartezeit (Tage)<input
                type="number"
                min="0"
                max="365"
                bind:value={level.waitDays}
              /></label
            >
            <label class="span-1"
              >Frist in Tagen<input
                type="number"
                min="1"
                max="90"
                bind:value={level.paymentDays}
              /></label
            >
            <label class="span-1"
              >Mahngebühr<input bind:value={level.fee} inputmode="decimal" /></label
            >
            <label class="wide">Betreff<input bind:value={level.subject} maxlength="200" /></label>
            <label class="wide"
              >Text<textarea bind:value={level.text} rows="8" maxlength="5000"></textarea></label
            >
          </div>
        </section>
      {/each}
      <div class="form-grid cols-4">
        <label
          >Verzugszinsen in % p. a. (optional)<input
            bind:value={settings.interestRate}
            inputmode="decimal"
            placeholder="leer = keine"
          /></label
        >
      </div>
      <p class="form-help">
        Platzhalter: {#each reminderPlaceholders as [key, label], i}<code>{key}</code>
          {label}{i < reminderPlaceholders.length - 1 ? ", " : ""}{/each}.
      </p>
      <p class="form-help">
        Mahngebühr und Verzugszinsen erscheinen nur als Hinweisbetrag auf der Erinnerung. Ob und in
        welcher Höhe du sie verlangen darfst, musst du selbst prüfen. Die App bietet keine
        Rechtsberatung und kein Inkasso.
      </p>
      {#if error}<p class="error" role="alert">{error}</p>{/if}
      {#if notice}<p role="status">{notice}</p>{/if}
      <div class="actions">
        <button type="button" class="primary" disabled={disabled || busy || !dirty} onclick={save}
          >Erinnerungen speichern</button
        ><button
          type="button"
          class="secondary"
          disabled={disabled || busy}
          onclick={restoreDefaults}>Standardtexte einsetzen</button
        >
      </div>
    {:else if error}<p class="error" role="alert">{error}</p>{/if}
  </div>
</SettingSection>

<style>
  .reminder-settings :global(.form-grid.cols-4 > .wide) {
    grid-column: 1 / -1;
  }
  .reminder-settings h3 {
    overflow-wrap: anywhere;
  }
  .reminder-settings textarea {
    width: 100%;
    min-height: 140px;
    font: inherit;
    resize: vertical;
    overscroll-behavior: contain;
  }
  .reminder-settings .actions {
    display: flex;
    gap: 8px;
    flex-wrap: wrap;
    margin-top: 8px;
  }
  .reminder-settings code {
    font-size: 11px;
  }
  .error {
    color: var(--theme-error-text);
  }
</style>
