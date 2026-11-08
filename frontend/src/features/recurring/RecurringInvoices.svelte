<script lang="ts">
  import { onMount } from "svelte";
  import * as API from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/app/invoiceservice";
  import type { RecurringInvoice } from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/model/models";
  import Icon from "../../components/Icon.svelte";
  import { amountLabel, dateLabel } from "../../lib/format";
  import { clone } from "../../lib/invoiceDraft";
  import {
    recurringBadgeClass,
    recurringDraftsLabel,
    recurringIntervalLabel,
    recurringStatusLabel,
  } from "../../lib/recurring";
  import RecurringDialog from "./RecurringDialog.svelte";
  let {
    busy,
    error,
    run,
    confirm,
    notify,
    onchanged,
  }: {
    busy: boolean;
    error: string;
    run: <T>(op: () => Promise<T>) => Promise<T | undefined>;
    confirm: (title: string, message: string, action: string) => Promise<boolean>;
    notify: (message: string) => void;
    /** Saved dates can be due at once, so the run follows every change. */
    onchanged: () => Promise<void>;
  } = $props();
  let plans = $state<RecurringInvoice[]>([]);
  let loaded = $state(false);
  let editing = $state<RecurringInvoice | null>(null);
  async function load() {
    plans = (await run(() => API.ListRecurringInvoices())) ?? [];
    loaded = true;
  }
  onMount(load);
  async function save() {
    if (!editing) return;
    const input = editing;
    if (!(await run(() => API.SaveRecurringInvoice(input)))) return;
    editing = null;
    notify("Dauerrechnung gespeichert");
    await onchanged();
    await load();
  }
  async function togglePause() {
    if (!editing) return;
    const { id, paused } = editing;
    if (!(await run(() => API.SetRecurringPaused(id, !paused)))) return;
    editing = null;
    notify(paused ? "Dauerrechnung fortgesetzt" : "Dauerrechnung pausiert");
    await load();
  }
  async function remove() {
    if (!editing) return;
    const { id, name } = editing;
    if (
      !(await confirm(
        "Dauerrechnung löschen?",
        `„${name}“ legt danach keine Entwürfe mehr an. Vorhandene Entwürfe und Rechnungen bleiben.`,
        "Löschen",
      ))
    )
      return;
    const removed = await run(async () => {
      await API.DeleteRecurringInvoice(id);
      return true;
    });
    if (!removed) return;
    editing = null;
    notify("Dauerrechnung gelöscht");
    await load();
  }
</script>

{#if plans.length}<div class="invoice-rows">
    {#each plans as plan (plan.id)}
      {@const details = [
        recurringIntervalLabel(plan.interval),
        recurringDraftsLabel(plan.openDrafts),
      ]
        .filter(Boolean)
        .join(" · ")}
      <button
        class="invoice-entry"
        disabled={busy}
        title="Dauerrechnung bearbeiten"
        onclick={() => (editing = clone(plan))}
      >
        <span class="invoice-entry-title">
          <strong class="plan-name" title={plan.name}>{plan.name}</strong>
          <small
            >{#if plan.template.customer.name}<span
                class="entry-customer"
                title={plan.template.customer.name}>{plan.template.customer.name}</span
              >{#if details}{" · "}{/if}{/if}{details}</small
          >
        </span>
        <span class="invoice-entry-date" title="Nächste Rechnung"
          >{plan.status === "ended" ? "" : dateLabel(plan.nextDate)}</span
        >
        <strong class="invoice-entry-amount">{amountLabel(plan.gross)}</strong>
        <span class="badge {recurringBadgeClass(plan.status)}"
          >{recurringStatusLabel(plan.status)}</span
        >
        <Icon name="next" size={16} />
      </button>
    {/each}
  </div>{:else if loaded}<div class="empty-state">
    <h2>Noch keine Dauerrechnung</h2>
    <p>
      Öffne eine ausgestellte Rechnung und wähle unter „Weitere Rechnungsaktionen“ den Punkt
      „Regelmäßig wiederholen“. Die App legt dann zu jedem Termin einen Entwurf an.
    </p>
  </div>{/if}
{#if editing}<RecurringDialog
    bind:plan={editing}
    {busy}
    {error}
    onsave={save}
    onpause={togglePause}
    ondelete={remove}
    onclose={() => (editing = null)}
  />{/if}

<style>
  /* Long plan names stop after two lines and long customer names after one,
     with the full text on hover, so the interval stays visible. */
  .plan-name {
    display: -webkit-box;
    -webkit-box-orient: vertical;
    -webkit-line-clamp: 2;
    line-clamp: 2;
    overflow: hidden;
  }
  .entry-customer {
    display: inline-block;
    max-width: 100%;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    vertical-align: bottom;
  }
</style>
