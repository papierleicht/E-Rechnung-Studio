<script lang="ts">
  import { onMount } from "svelte";
  import { Browser } from "@wailsio/runtime";
  import RefreshIcon from "@lucide/svelte/icons/refresh-cw";
  import ShieldIcon from "@lucide/svelte/icons/shield-check";
  import { banking } from "../lib/banking-api";
  import { plan } from "../lib/plan";
  import { hintPlan, loadHintPlan, withAccount } from "../lib/hintPlan";
  import {
    PLUS_INFO_URL,
    dismissPlusHint,
    dismissedPlusHints,
    plusHintRelevant,
    plusHintVisible,
    type PlusHintTopic,
  } from "../lib/plusHints";

  let {
    id,
    topic,
    text,
    dismissible = true,
  }: {
    /** Stable, unique key under which "Nicht mehr anzeigen" is remembered. */
    id: string;
    topic: PlusHintTopic;
    /** One short, clear line describing the benefit. */
    text: string;
    /** False only where the line explains a disabled function. */
    dismissible?: boolean;
  } = $props();

  let dismissed = $state(dismissedPlusHints());
  const status = $derived(withAccount($hintPlan, $plan));
  const visible = $derived(
    dismissible ? plusHintVisible(id, topic, status, dismissed) : plusHintRelevant(topic, status),
  );

  onMount(() => {
    void loadHintPlan(() => banking.overview());
  });

  async function openInfo() {
    try {
      await Browser.OpenURL(PLUS_INFO_URL);
    } catch {
      window.open(PLUS_INFO_URL, "_blank", "noopener,noreferrer");
    }
  }
</script>

{#if visible}
  <p class="plus-hint">
    <span class="plus-hint-icon" aria-hidden="true"
      >{#if topic === "backup"}<ShieldIcon size={14} />{:else}<RefreshIcon size={14} />{/if}</span
    >
    <span class="plus-hint-body"
      >{text}
      <button type="button" class="plus-hint-link" onclick={openInfo}>Mehr zu Plus</button
      >{#if dismissible}<button
          type="button"
          class="plus-hint-link plus-hint-dismiss"
          onclick={() => (dismissed = dismissPlusHint(id))}>Nicht mehr anzeigen</button
        >{/if}</span
    >
  </p>
{/if}

<style>
  .plus-hint {
    display: flex;
    align-items: flex-start;
    gap: 8px;
    margin: 10px 0;
    color: var(--theme-muted);
    font-size: 12px;
    line-height: 1.5;
  }
  .plus-hint-icon {
    display: inline-flex;
    flex-shrink: 0;
    /* Centre the icon on the first text line. */
    padding-top: 2px;
    color: var(--theme-accent);
  }
  .plus-hint-body {
    min-width: 0;
  }
  .plus-hint-link {
    margin-left: 10px;
    border: 0;
    padding: 0;
    background: transparent;
    color: var(--theme-accent);
    font: inherit;
    white-space: nowrap;
    cursor: pointer;
  }
  .plus-hint-dismiss {
    color: var(--theme-muted);
  }
  .plus-hint-link:hover {
    text-decoration: underline;
  }
</style>
