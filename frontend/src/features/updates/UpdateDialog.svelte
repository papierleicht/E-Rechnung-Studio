<script lang="ts">
  import { onDestroy } from "svelte";
  import FormDialog from "../../components/FormDialog.svelte";
  import ReleaseNotes from "./ReleaseNotes.svelte";
  import * as UpdateAPI from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/app/updateservice";
  import type {
    UpdateProgress,
    UpdateStatus,
  } from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/app/models";
  import { serviceErrorMessage } from "../../lib/errors";
  let {
    status,
    onclose,
    onskipped,
  }: {
    status: UpdateStatus;
    onclose: () => void;
    /** The customer skipped this version. */
    onskipped: () => void;
  } = $props();

  let step = $state<"offer" | "working" | "ready" | "failed">("offer");
  let progress = $state<UpdateProgress>({ phase: "idle", written: 0, total: 0 });
  let error = $state("");
  let timer: ReturnType<typeof setInterval> | undefined;
  onDestroy(() => clearInterval(timer));

  const megabytes = (bytes: number) =>
    (bytes / 1_000_000).toLocaleString("de-DE", { maximumFractionDigits: 1 });
  const percent = $derived(
    progress.total > 0 ? Math.min(100, Math.round((progress.written / progress.total) * 100)) : 0,
  );
  const phaseText = $derived(
    progress.phase === "verifying"
      ? "Die Signatur wird geprüft …"
      : progress.phase === "installing"
        ? "Das Update wird vorbereitet …"
        : progress.total > 0
          ? `Wird heruntergeladen: ${megabytes(progress.written)} von ${megabytes(progress.total)} MB`
          : "Wird heruntergeladen …",
  );

  async function install() {
    error = "";
    step = "working";
    progress = { phase: "downloading", written: 0, total: 0 };
    timer = setInterval(async () => {
      try {
        progress = await UpdateAPI.Progress();
      } catch {
        /* the next poll tries again */
      }
    }, 400);
    try {
      await UpdateAPI.Install();
      step = "ready";
    } catch (cause) {
      error = serviceErrorMessage(cause, "Das Update ließ sich nicht installieren.");
      step = "failed";
    } finally {
      clearInterval(timer);
    }
  }

  async function restart() {
    error = "";
    try {
      await UpdateAPI.Restart();
    } catch (cause) {
      error = serviceErrorMessage(cause, "Die App ließ sich nicht neu starten.");
    }
  }

  async function skip() {
    try {
      await UpdateAPI.Skip(status.availableVersion ?? "");
      onskipped();
    } catch (cause) {
      error = serviceErrorMessage(cause, "Das hat nicht geklappt.");
    }
  }
</script>

<FormDialog
  title={step === "ready" ? "Update bereit" : `Version ${status.availableVersion} ist da`}
  description={step === "offer"
    ? `Du hast Version ${status.currentVersion}. Deine Daten bleiben beim Update unverändert.`
    : ""}
  busy={step === "working"}
  {error}
  {onclose}
>
  {#if step === "offer" || step === "failed"}
    <h3 class="update-heading">Was ist neu?</h3>
    <div class="update-notes"><ReleaseNotes notes={status.notes ?? ""} /></div>
  {:else if step === "working"}
    <div
      class="update-progress"
      role="progressbar"
      aria-label="Fortschritt des Updates"
      aria-valuemin="0"
      aria-valuemax="100"
      aria-valuenow={percent}
    >
      <span
        class:indeterminate={progress.total === 0 || progress.phase !== "downloading"}
        style={`width: ${progress.phase === "downloading" && progress.total > 0 ? percent : 100}%`}
      ></span>
    </div>
    <p class="update-phase" role="status">{phaseText}</p>
    <p class="update-hint">Du kannst weiterarbeiten. Schließe die App währenddessen nicht.</p>
  {:else}
    <p>
      Version {status.availableVersion} ist geprüft und installiert. Starte die App neu, damit sie die
      neue Version verwendet. Sichere vorher, woran du gerade arbeitest.
    </p>
  {/if}

  {#snippet footer()}
    {#if step === "offer"}
      <button type="button" class="text" onclick={skip}>Diese Version überspringen</button>
      <button type="button" class="secondary" onclick={onclose}>Später</button>
      <button type="button" class="primary" onclick={install}>Jetzt aktualisieren</button>
    {:else if step === "failed"}
      <button type="button" class="secondary" onclick={onclose}>Schließen</button>
      <button type="button" class="primary" onclick={install}>Erneut versuchen</button>
    {:else if step === "ready"}
      <button type="button" class="secondary" onclick={onclose}>Später neu starten</button>
      <button type="button" class="primary" onclick={restart}>Jetzt neu starten</button>
    {/if}
  {/snippet}
</FormDialog>

<style>
  .update-heading {
    margin: 0 0 8px;
    font-size: 14px;
  }
  .update-notes {
    max-height: 320px;
    overflow: auto;
    padding: 12px 14px;
    border: 1px solid var(--theme-border);
    border-radius: 8px;
    background: var(--theme-surface-muted);
  }
  .update-progress {
    height: 8px;
    border-radius: 4px;
    background: var(--theme-surface-muted);
    overflow: hidden;
  }
  .update-progress span {
    display: block;
    height: 100%;
    background: var(--theme-accent);
    transition: width 0.3s ease;
  }
  .update-progress span.indeterminate {
    animation: update-pulse 1.4s ease-in-out infinite;
  }
  @keyframes update-pulse {
    50% {
      opacity: 0.45;
    }
  }
  .update-phase {
    margin: 10px 0 0;
    font-size: 13px;
  }
  .update-hint {
    margin: 6px 0 0;
    font-size: 12px;
    color: var(--theme-muted);
  }
</style>
