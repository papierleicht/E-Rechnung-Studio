<script lang="ts">
  import { onMount } from "svelte";
  import { Events } from "@wailsio/runtime";
  import * as BackupService from "../../bindings/papierleicht.de/e-rechnung-studio/internal/app/backupservice";
  import type { CloudBackupStatus } from "../../bindings/papierleicht.de/e-rechnung-studio/internal/app/models";
  import Icon from "./Icon.svelte";
  import { backupActive } from "../lib/plan";
  import { backupOverdueDays } from "../lib/backup";

  /** Opens the account page with the backup section. */
  let { onselect }: { onselect: () => void } = $props();

  let status = $state<CloudBackupStatus | null>(null);
  const days = $derived(status && $backupActive ? backupOverdueDays(status, new Date()) : 0);

  onMount(() => {
    void BackupService.BackupStatus().then(
      (next) => (status = next),
      () => undefined,
    );
    return Events.On("backup:status", (event) => {
      const data = (Array.isArray(event.data) ? event.data[0] : event.data) as CloudBackupStatus;
      if (data && typeof data === "object") status = data;
    });
  });
</script>

<!-- A backup that silently stopped is the failure nobody notices. -->
{#if days > 0}
  <button type="button" class="backup-alert" onclick={onselect}>
    <Icon name="refresh" size={15} />
    <span
      >{status?.lastError ? "Sicherung fehlgeschlagen" : `Seit ${days} Tagen keine Sicherung`}</span
    >
  </button>
{/if}

<style>
  .backup-alert {
    display: flex;
    align-items: center;
    gap: 8px;
    width: 100%;
    margin-bottom: 8px;
    padding: 8px 10px;
    border: 1px solid var(--theme-warning-border);
    border-radius: 8px;
    background: var(--theme-warning-bg);
    color: var(--theme-warning-text);
    font: inherit;
    font-size: 12px;
    text-align: left;
    cursor: pointer;
  }
</style>
