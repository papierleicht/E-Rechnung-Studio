<script lang="ts">
  import FormDialog from "../../components/FormDialog.svelte";
  import PlusHint from "../../components/PlusHint.svelte";
  import catalog from "../../lib/license-catalog.json";
  import notices from "../../../../docs/third-party-notices.md?raw";
  import ReleaseNotesDialog from "../updates/ReleaseNotesDialog.svelte";
  import * as UpdateAPI from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/app/updateservice";
  import type { VersionNotes } from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/app/models";
  let { onclose }: { onclose: () => void } = $props();
  let licenses = $state(false);
  // The release notes the app ships with, once the customer opens them.
  let releaseNotes = $state<VersionNotes[] | null>(null);
  let notesError = $state("");
  async function showReleaseNotes() {
    notesError = "";
    try {
      releaseNotes = (await UpdateAPI.ReleaseNotes()) ?? [];
    } catch {
      notesError = "Die Versionshinweise ließen sich nicht laden.";
    }
  }
  let query = $state("");
  const filtered = $derived(
    catalog.filter((entry) =>
      `${entry.name} ${entry.group}`.toLowerCase().includes(query.trim().toLowerCase()),
    ),
  );
</script>

<FormDialog
  title={licenses ? "Lizenzen & Drittanbieterhinweise" : "Über papierleicht"}
  error={notesError}
  {onclose}
>
  {#if licenses}
    <p>
      Lizenztexte und Hinweise zu Bibliotheken, Schriften und Validierungsdaten. Die
      Entwicklungswerkzeuge sind separat aufgeführt. Alle Texte funktionieren auch offline.
    </p>
    <label class="license-search"
      >Komponente suchen
      <input type="search" bind:value={query} placeholder="Zum Beispiel Wails, Lucide oder PDF" />
    </label>
    <div class="detail-list">
      <details>
        <summary
          ><span class="detail-title"
            >Ergänzende Hinweise zu eingebundenen Daten und Komponenten</span
          ></summary
        >
        <pre>{notices}</pre>
      </details>
    </div>
    <p aria-live="polite">{filtered.length} Einträge</p>
    {#if filtered.length}<div class="detail-list license-list">
        {#each filtered as entry}
          <details>
            <summary
              ><span class="detail-title">{entry.name}</span><span class="detail-state"
                >{entry.version} · {entry.group}</span
              ></summary
            >
            <pre>{entry.text}</pre>
          </details>
        {/each}
      </div>{/if}
    {#if filtered.length === 0}<p>Keine passende Komponente gefunden.</p>{/if}
  {:else}
    <p>Rechnungen erstellen, prüfen und verwalten – lokal auf deinem Computer.</p>
    <PlusHint
      id="about-plus"
      topic="plus"
      text="Mit Plus kannst du optional Konten per FinTS abrufen, dein Änderungsprotokoll extern bestätigen lassen und bekommst Software-Support per E-Mail."
    />
    <div class="about-actions">
      <button type="button" class="secondary" onclick={showReleaseNotes}>Versionshinweise</button>
      <button type="button" class="secondary" onclick={() => (licenses = true)}
        >Lizenzen & Drittanbieterhinweise</button
      >
    </div>
  {/if}
  {#snippet footer()}
    {#if licenses}<button type="button" class="secondary" onclick={() => (licenses = false)}
        >Zurück</button
      >{/if}
    <button type="button" class="secondary" onclick={onclose}>Schließen</button>
  {/snippet}
</FormDialog>
{#if releaseNotes}<ReleaseNotesDialog
    versions={releaseNotes}
    title="Versionshinweise"
    onclose={() => (releaseNotes = null)}
  />{/if}

<style>
  .about-actions {
    display: flex;
    flex-wrap: wrap;
    gap: 8px;
  }
  .license-search {
    display: grid;
    gap: 6px;
    margin: 16px 0;
  }
  pre {
    white-space: pre-wrap;
    overflow-wrap: anywhere;
    font-size: 12px;
    line-height: 1.6;
    font-family: inherit;
  }
</style>
