<script lang="ts">
  import type { VATFiling } from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/store/models";
  import { dateLabel, localToday } from "../../lib/format";

  let {
    filing,
    changed = false,
    until,
    onmark,
    onunmark,
  }: {
    filing?: VATFiling | null;
    /** The result differs from the transmitted one: a correction is due. */
    changed?: boolean;
    /** Last day of the period; before it ends there is nothing to transmit. */
    until: string;
    onmark: (submittedOn: string) => Promise<void>;
    onunmark: () => Promise<void>;
  } = $props();
  const today = localToday();
  let filedOn = $state(today);
</script>

<!-- The transmission note of a filing: set it, renew it after a correction or remove it. -->
<div class="tax-actions">
  {#if !filing && until >= today}
    <p class="tax-muted">
      Der Zeitraum läuft noch bis {dateLabel(until)}. Übermitteln kannst du danach.
    </p>
  {:else if filing && !changed}
    <button class="text" onclick={onunmark}>Vermerk „übermittelt“ entfernen</button>
  {:else}
    <label
      >{changed ? "Berichtigung übermittelt am" : "Übermittelt am"}<input
        type="date"
        bind:value={filedOn}
        max={today}
      /></label
    >
    <button class="primary" onclick={() => onmark(filedOn)}
      >{changed ? "Berichtigung als übermittelt markieren" : "Als übermittelt markieren"}</button
    >
    {#if filing}<button class="text" onclick={onunmark}>Vermerk entfernen</button>{/if}
  {/if}
</div>
