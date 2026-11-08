<script lang="ts">
  import type { InputTaxStatus } from "../../bindings/papierleicht.de/e-rechnung-studio/internal/model/models";
  import { amountLabel } from "../lib/format";
  import { inputTaxChoiceLabel } from "../lib/received";
  let { status }: { status?: InputTaxStatus | null } = $props();
</script>

<!-- What the app suggests for the Vorsteuer, or what the user decided. A passed check needs no note. -->
{#if status?.choice}
  <p class="input-tax-notice" role="status">
    <strong>Vorsteuer: deine Entscheidung</strong>
    {inputTaxChoiceLabel(status.choice)}{#if status.choice.note}. Deine Begründung: {status.choice
        .note}{/if}
  </p>
{:else if status?.reason}
  <p class="input-tax-notice" class:blocked={!status.allowed} role="status">
    <strong
      >{status.allowed
        ? `Vorschlag der App: ${status.tax ? `${amountLabel(status.tax)} ` : ""}${status.onPayment ? "mit der Zahlung " : ""}einrechnen`
        : "Vorschlag der App: nicht einrechnen"}</strong
    >
    {status.reason}
  </p>
{/if}

<style>
  .input-tax-notice {
    margin: 0;
    padding: 10px 12px;
    border: 1px solid var(--theme-border);
    border-radius: 8px;
    background: var(--theme-surface-muted);
    font-size: 13px;
    line-height: 1.5;
  }
  .input-tax-notice.blocked {
    border-color: var(--theme-warning-border);
    background: var(--theme-warning-bg);
    color: var(--theme-warning-text);
  }
  strong {
    display: block;
    margin-bottom: 2px;
  }
</style>
