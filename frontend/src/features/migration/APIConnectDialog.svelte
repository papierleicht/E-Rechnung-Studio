<script lang="ts">
  import { onDestroy } from "svelte";
  import FormDialog from "../../components/FormDialog.svelte";
  import { serviceErrorMessage } from "../../lib/errors";
  import { migration, type APIStatus, type Source } from "../../lib/migration-api";
  let {
    source,
    onclose,
    oncustomers,
    onfinished,
  }: {
    source: Source;
    onclose: () => void;
    /** Opens the customers the run read for review. */
    oncustomers: () => void;
    /** The archive changed: the page reloads its counts. */
    onfinished: () => void;
  } = $props();

  let key = $state("");
  let error = $state("");
  let status = $state<APIStatus | null>(null);
  let timer: ReturnType<typeof setInterval> | undefined;
  const running = $derived(status?.state === "checking" || status?.state === "running");
  const finished = $derived(
    status?.state === "done" || status?.state === "cancelled" || status?.state === "failed",
  );
  const percent = $derived(
    status && status.total ? Math.round((status.done / status.total) * 100) : 0,
  );

  function poll() {
    clearInterval(timer);
    timer = setInterval(async () => {
      try {
        status = await migration.apiStatus();
        if (status.state !== "running" && status.state !== "checking") {
          clearInterval(timer);
          onfinished();
        }
      } catch (cause) {
        clearInterval(timer);
        error = serviceErrorMessage(cause, "Der Stand des Abrufs ließ sich nicht lesen.");
      }
    }, 800);
  }
  onDestroy(() => clearInterval(timer));

  async function connect() {
    error = "";
    try {
      status = await migration.connectAPI(source.id, key);
      key = "";
      poll();
    } catch (cause) {
      error = serviceErrorMessage(cause, "Die Verbindung ist fehlgeschlagen.");
      status = null;
    }
  }

  async function cancel() {
    await migration.cancelAPI();
  }
</script>

<FormDialog
  title={`Direkt mit ${source.name} verbinden`}
  description="Die App liest Kunden und Rechnungen über die offizielle Schnittstelle. Sie ändert in deinem Konto nichts."
  busy={running}
  {error}
  onclose={() => {
    if (!running) onclose();
  }}
  onsubmit={status ? undefined : connect}
>
  {#if !status || status.state === "failed"}
    {#if status?.state === "failed"}<p class="api-failed" role="alert">{status.message}</p>{/if}
    <label class="api-key"
      ><span>Schlüssel (API-Key oder Token)</span><input
        type="password"
        autocomplete="off"
        spellcheck="false"
        required
        bind:value={key}
      /></label
    >
    <ul class="api-notes">
      <li>Wo du den Schlüssel erstellst, steht in der Anleitung unter „In {source.name}“.</li>
      <li>
        Der Schlüssel gibt Zugriff auf dein Konto. papierleicht liest damit nur, behält ihn nur für
        diesen Abruf im Arbeitsspeicher und speichert ihn nicht. Lösche ihn nach dem Umzug in
        {source.name}.
      </li>
      <li>
        Große Konten brauchen Zeit, weil die Programme nur wenige Abrufe pro Sekunde erlauben.
        Brichst du ab, holt ein neuer Abruf nur den Rest.
      </li>
    </ul>
  {:else}
    <p class="api-account">
      Verbunden mit <strong>{status.account || source.name}</strong>
    </p>
    <div
      class="api-progress"
      role="progressbar"
      aria-valuemin="0"
      aria-valuemax="100"
      aria-valuenow={percent}
      aria-label="Fortschritt"
    >
      <span style={`width: ${finished ? 100 : percent}%`}></span>
    </div>
    <p class="api-phase" role="status">
      {#if running}
        {status.phase}
        {#if status.total}{status.done} von {status.total} Belegen{/if}
      {:else if status.state === "done"}
        Fertig.
      {:else}
        {status.message}
      {/if}
    </p>
    <ul class="api-counts">
      <li><strong>{status.imported}</strong> im Altbestand abgelegt</li>
      <li><strong>{status.duplicates}</strong> waren schon da</li>
      <li><strong>{status.failed.length}</strong> ließen sich nicht laden</li>
      <li><strong>{status.customers}</strong> Kunden gelesen</li>
    </ul>
    {#if status.failed.length}
      <ul class="api-failed-list">
        {#each status.failed.slice(0, 50) as item}
          <li><span>{item.origin}</span><small>{item.reason}</small></li>
        {/each}
      </ul>
    {/if}
  {/if}

  {#snippet footer()}
    {#if running}
      <button type="button" class="secondary" onclick={cancel}>Abbrechen</button>
    {:else if status && status.state !== "failed"}
      <button type="button" class="secondary" onclick={onclose}>Schließen</button>
      {#if status.customers}
        <button type="button" class="primary" onclick={oncustomers}>Kunden prüfen</button>
      {/if}
    {:else}
      <button type="button" class="secondary" onclick={onclose}>Abbrechen</button>
      <button type="submit" class="primary" disabled={key.trim().length < 8}
        >Verbinden und abrufen</button
      >
    {/if}
  {/snippet}
</FormDialog>

<style>
  .api-key {
    display: grid;
    gap: 4px;
    font-size: 13px;
  }
  .api-key input {
    width: 100%;
    min-width: 0;
  }
  .api-notes,
  .api-counts {
    margin: 12px 0 0;
    padding-left: 18px;
    font-size: 12px;
    line-height: 1.6;
    color: var(--theme-muted);
  }
  .api-counts {
    display: flex;
    flex-wrap: wrap;
    gap: 6px 18px;
    padding: 0;
    list-style: none;
    font-size: 13px;
  }
  .api-counts strong {
    color: var(--theme-text);
  }
  .api-account {
    margin: 0 0 10px;
    font-size: 13px;
    overflow-wrap: anywhere;
  }
  .api-progress {
    height: 8px;
    border-radius: 4px;
    background: var(--theme-surface-muted);
    overflow: hidden;
  }
  .api-progress span {
    display: block;
    height: 100%;
    background: var(--theme-accent);
    transition: width 0.3s ease;
  }
  .api-phase {
    margin: 8px 0 0;
    font-size: 12px;
    color: var(--theme-muted);
  }
  .api-failed {
    margin: 0 0 12px;
    padding: 10px;
    border-radius: 6px;
    background: var(--theme-error-bg);
    color: var(--theme-error-text);
    font-size: 12px;
  }
  .api-failed-list {
    display: grid;
    gap: 4px;
    max-height: 140px;
    overflow: auto;
    margin: 10px 0 0;
    padding: 0;
    list-style: none;
    font-size: 12px;
  }
  .api-failed-list li {
    display: flex;
    flex-wrap: wrap;
    gap: 2px 10px;
    min-width: 0;
  }
  .api-failed-list span {
    overflow-wrap: anywhere;
  }
  .api-failed-list small {
    color: var(--theme-muted);
  }
</style>
