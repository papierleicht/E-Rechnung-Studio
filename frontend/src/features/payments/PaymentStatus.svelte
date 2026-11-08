<script lang="ts">
  import type { PaymentBalance } from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/store/models";
  import { paymentTitle, paymentDescription } from "../../lib/paymentWorkflow";
  let {
    balance,
    summary,
  }: {
    balance?: PaymentBalance;
    /** A combined state in the same look, e.g. all open invoices of a customer. */
    summary?: { title: string; description?: string; overdue?: boolean; paid?: boolean };
  } = $props();
  const state = $derived(
    balance
      ? {
          title: paymentTitle(balance),
          description: paymentDescription(balance),
          overdue: balance.state === "überfällig" || balance.planNeedsReview,
          paid: balance.state === "bezahlt" || balance.state === "verrechnet",
        }
      : (summary ?? { title: "" }),
  );
</script>

<span class="payment-state" class:overdue={state.overdue} class:paid={state.paid}
  ><strong>{state.title}</strong>{#if state.description}<small>{state.description}</small
    >{/if}</span
>

<style>
  .payment-state {
    display: grid;
    gap: 4px;
    font-size: 12px;
    color: var(--theme-muted);
    line-height: 1.4;
  }
  .payment-state strong {
    font-weight: 600;
    color: var(--theme-text);
  }
  .payment-state.overdue strong {
    color: var(--theme-error-text);
  }
  .payment-state.paid strong {
    color: var(--theme-success-text);
  }
  small {
    font-size: 11px;
  }
</style>
