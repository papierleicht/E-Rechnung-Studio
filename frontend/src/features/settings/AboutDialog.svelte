<script lang="ts">
  import FormDialog from "../../components/FormDialog.svelte";
  import PlusHint from "../../components/PlusHint.svelte";
  import catalog from "../../lib/license-catalog.json";
  import notices from "../../../../docs/third-party-notices.md?raw";
  let { onclose }: { onclose: () => void } = $props();
  let licenses = $state(false);
  let query = $state("");
  const filtered = $derived(
    catalog.filter((entry) =>
      `${entry.name} ${entry.group}`.toLowerCase().includes(query.trim().toLowerCase()),
    ),
  );
</script>

<FormDialog
  title={licenses ? "Lizenzen & Drittanbieterhinweise" : "Über E-Rechnung-Studio"}
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
    <button type="button" class="secondary" onclick={() => (licenses = true)}
      >Lizenzen & Drittanbieterhinweise</button
    >
  {/if}
  {#snippet footer()}
    {#if licenses}<button type="button" class="secondary" onclick={() => (licenses = false)}
        >Zurück</button
      >{/if}
    <button type="button" class="secondary" onclick={onclose}>Schließen</button>
  {/snippet}
</FormDialog>

<style>
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
