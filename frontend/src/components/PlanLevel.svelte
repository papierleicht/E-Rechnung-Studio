<script lang="ts">
  import Icon from "./Icon.svelte";
  import SecurityMeter from "./SecurityMeter.svelte";
  import { plan } from "../lib/plan";
  import { planLevel, planTiers } from "../lib/planStatus";
  import { lastBackupAt, next, security } from "../lib/security";
  import { SECURITY_STEPS, lastSavedLabel } from "../lib/securityLevel";

  /** Opens the account page with the security level and the next step. */
  let { onselect }: { onselect: () => void } = $props();

  const label = $derived(
    $plan.backupActive
      ? `${planTiers[planLevel($plan).level].label} + Backup`
      : $plan.supportActive
        ? "Community Support"
        : planTiers[planLevel($plan).level].label,
  );
  const lastSaved = $derived(lastSavedLabel($lastBackupAt, new Date()));
  const stage = $derived(`Sicherheitsstufe ${$security.level} von ${SECURITY_STEPS}`);
  // Once everything is safe, the sidebar stays calm.
  const action = $derived($security.level < SECURITY_STEPS ? ($next?.label ?? "") : "");
  const title = $derived(
    action
      ? `${label}, ${stage}: ${$security.title} – ${action}`
      : `${label}, ${stage}: ${$security.title}`,
  );
</script>

{#if $plan.loaded && $plan.configured}
  <button
    type="button"
    class="plan-card"
    aria-label={`${title}. Konto & Abo öffnen`}
    {title}
    onclick={onselect}
  >
    <span class="plan-card-status">
      <SecurityMeter level={$security.level} />
      <strong>{label}</strong>
    </span>
    {#if action}
      <span class="plan-card-action">
        <span>{action}</span>
        <Icon name="next" size={14} />
      </span>
    {:else if lastSaved}
      <span class="plan-card-action calm"><span>{lastSaved}</span></span>
    {/if}
  </button>
{/if}
