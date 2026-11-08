<script lang="ts">
  import Icon from "./Icon.svelte";
  import SecurityMeter from "./SecurityMeter.svelte";
  import { next, security } from "../lib/security";
  import { SECURITY_STEPS } from "../lib/securityLevel";

  /** Opens the account page. */
  let { onselect }: { onselect: () => void } = $props();
</script>

<!-- A quiet look at the state, never a call to action. -->
<button type="button" class="status-line" onclick={onselect}>
  <SecurityMeter level={$security.level} />
  <span>Stufe {$security.level} von {SECURITY_STEPS}: {$security.title}</span>
  {#if $next}<span class="status-next">Nächster Schritt: {$next.label}</span>{/if}
  <Icon name="next" size={14} />
</button>

<style>
  .status-line {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 8px 12px;
    width: 100%;
    margin-top: 20px;
    padding: 10px 12px;
    border: 0;
    border-top: 1px solid var(--theme-border);
    background: none;
    color: var(--theme-muted);
    font: inherit;
    font-size: 13px;
    text-align: left;
    cursor: pointer;
  }
  .status-next {
    color: var(--theme-accent);
    font-weight: 600;
  }
  .status-line:hover {
    color: var(--theme-text);
  }
</style>
