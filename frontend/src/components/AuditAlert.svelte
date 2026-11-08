<script lang="ts">
  import Icon from "./Icon.svelte";
  import { auditHealth } from "../lib/security";

  /** Opens the audit tab of the settings. */
  let { onselect }: { onselect: () => void } = $props();

  const text = $derived.by(() => {
    const h = $auditHealth;
    if (!h?.checkedAt) return "";
    if (!h.ok || h.conflicts > 0) return "Änderungsprotokoll prüfen";
    return h.stale ? "Keine neue Bestätigung" : "";
  });
</script>

<!-- A broken or unconfirmed log is the failure nobody notices. -->
{#if text}
  <button type="button" class="audit-alert" onclick={onselect}>
    <Icon name="refresh" size={15} />
    <span>{text}</span>
  </button>
{/if}

<style>
  .audit-alert {
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
