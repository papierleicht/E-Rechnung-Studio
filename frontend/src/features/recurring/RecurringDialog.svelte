<script lang="ts">
  import * as API from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/app/invoiceservice";
  import type { RecurringInvoice } from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/model/models";
  import FormDialog from "../../components/FormDialog.svelte";
  import Icon from "../../components/Icon.svelte";
  import { amountLabel, dateLabel } from "../../lib/format";
  import { billsInAdvance, recurringIntervals } from "../../lib/recurring";
  let {
    plan = $bindable(),
    busy,
    error,
    takesOverInvoice = false,
    onsave,
    onpause,
    ondelete,
    onclose,
  }: {
    plan: RecurringInvoice;
    busy: boolean;
    error: string;
    /** Opened from an invoice of this recurring invoice: its content comes along. */
    takesOverInvoice?: boolean;
    onsave: () => void;
    onpause?: () => void;
    ondelete?: () => void;
    onclose: () => void;
  } = $props();
  let periodEnd = $state("");
  $effect(() => {
    const input = plan;
    const key = [input.interval, input.nextDate, input.periodStart];
    let current = true;
    periodEnd = "";
    if (key.every(Boolean))
      API.RecurringPreview(input)
        .then((preview) => {
          if (current) periodEnd = preview.periodEnd;
        })
        .catch(() => {});
    return () => {
      current = false;
    };
  });
  const description = $derived(
    takesOverInvoice
      ? "Die Positionen und Preise dieser Rechnung gelten ab dem nächsten Entwurf."
      : "Zu jedem Termin legt die App einen Entwurf an. Du prüfst ihn und stellst ihn selbst aus. Ausgestellt oder versendet wird nichts automatisch.",
  );
</script>

<FormDialog
  title={plan.id ? "Dauerrechnung bearbeiten" : "Rechnung regelmäßig wiederholen"}
  {description}
  {busy}
  {error}
  {onclose}
  onsubmit={onsave}
>
  <div class="form-grid cols-2">
    <label>Name<input required bind:value={plan.name} placeholder="z. B. Hosting" /></label><label
      >Rhythmus<select bind:value={plan.interval}
        >{#each recurringIntervals as [value, label]}<option {value}>{label}</option>{/each}</select
      ></label
    ><label>Nächste Rechnung am<input type="date" required bind:value={plan.nextDate} /></label
    ><label
      >Leistungszeitraum beginnt am<input
        type="date"
        required
        bind:value={plan.periodStart}
      /></label
    ><label
      >Letzte Rechnung spätestens am (optional)<input
        type="date"
        bind:value={plan.endDate}
      /></label
    >
  </div>
  <p class="recurring-summary">
    Rechnung an <strong>{plan.template.customer.name}</strong>{plan.gross
      ? ` über ${amountLabel(plan.gross)}`
      : ""}.
    {#if periodEnd}Leistungszeitraum {dateLabel(plan.periodStart)} bis {dateLabel(periodEnd)}.{/if}
  </p>
  {#if billsInAdvance(plan.nextDate, plan.periodStart)}<p class="recurring-summary">
      Die Rechnung liegt vor dem Ende des Zeitraums. Steht die Leistung beim Ausstellen noch aus,
      kannst du den Entwurf als Anzahlungsrechnung kennzeichnen. Das entscheidest du.
    </p>{/if}

  {#snippet footer()}
    {#if ondelete}<button type="button" class="danger-text" disabled={busy} onclick={ondelete}
        ><Icon name="trash" size={16} /> Löschen</button
      >{/if}
    <span class="footer-spacer"></span>
    {#if onpause && plan.status !== "ended"}<button
        type="button"
        class="secondary"
        disabled={busy}
        onclick={onpause}>{plan.paused ? "Fortsetzen" : "Pausieren"}</button
      >{/if}
    <button type="button" class="secondary" disabled={busy} onclick={onclose}>Abbrechen</button
    ><button class="primary" disabled={busy}>{busy ? "Wird gespeichert …" : "Speichern"}</button>
  {/snippet}
</FormDialog>

<style>
  .recurring-summary {
    margin: 12px 0 0;
    color: var(--theme-muted);
    font-size: 13px;
    line-height: 1.5;
  }
</style>
