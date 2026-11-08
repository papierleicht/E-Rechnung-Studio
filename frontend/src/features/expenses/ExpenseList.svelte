<script lang="ts">
  import { onMount } from "svelte";
  import * as API from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/app/invoiceservice";
  import type { Expense } from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/store/models";
  import type { Category } from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/taxforms/models";
  import ExpenseDialog from "./ExpenseDialog.svelte";
  import { serviceErrorMessage } from "../../lib/errors";
  import { amountLabel, dateLabel } from "../../lib/format";
  import { paidLabels, yearsLabel } from "../../lib/expenses";
  import { cached, remember } from "../../lib/cache";
  import FormDialog from "../../components/FormDialog.svelte";
  import ListHeader from "../../components/ListHeader.svelte";
  import EntryRow from "../../components/EntryRow.svelte";
  import PaymentStatus from "../payments/PaymentStatus.svelte";

  let {
    revision = 0,
  }: {
    /** Changes when receipts were added elsewhere (e.g. from a document). */
    revision?: number;
  } = $props();

  const warm = cached<{ expenses: Expense[]; categories: Category[]; smallBusiness: boolean }>(
    "expenses:data",
  );
  let expenses = $state<Expense[]>(warm?.expenses ?? []);
  let categories = $state<Category[]>(warm?.categories ?? []);
  let smallBusiness = $state(warm?.smallBusiness ?? false);
  let editing = $state<Partial<Expense> | null>(null);
  let cancelling = $state<Expense | null>(null);
  let cancelReason = $state("");
  let cancelError = $state("");
  let busy = $state(false);
  let error = $state("");
  let message = $state("");
  let query = $state("");
  let loaded = $state(!!warm);

  const label = (id: string) => categories.find((c) => c.id === id)?.label ?? id;
  const shown = $derived(
    expenses.filter((e) =>
      [e.supplier, e.note, label(e.category), e.number, e.documentNumber]
        .join(" ")
        .toLocaleLowerCase()
        .includes(query.toLocaleLowerCase()),
    ),
  );

  async function load() {
    try {
      const [list, cats, setup] = await Promise.all([
        API.Expenses(),
        API.ExpenseCategories(),
        API.TaxSetup(),
      ]);
      expenses = list ?? [];
      categories = cats ?? [];
      smallBusiness = setup.smallBusiness;
      remember("expenses:data", { expenses, categories, smallBusiness });
    } catch (cause) {
      error = serviceErrorMessage(cause, "Die erfassten Belege konnten nicht geladen werden.");
    } finally {
      loaded = true;
    }
  }
  onMount(load);
  /** Opens the dialog for a new receipt, e.g. from the tile above the page. */
  export function create() {
    editing = {};
  }
  $effect(() => {
    if (revision) void load();
  });

  async function cancelReceipt() {
    if (!cancelling || busy) return;
    busy = true;
    cancelError = "";
    try {
      await API.CancelExpense(cancelling.id, cancelReason);
      cancelling = null;
      editing = null;
      cancelReason = "";
      message = "Beleg storniert. Er zählt nicht mehr und bleibt nachvollziehbar erhalten.";
      await load();
    } catch (cause) {
      cancelError = serviceErrorMessage(cause, "Der Beleg konnte nicht storniert werden.");
    } finally {
      busy = false;
    }
  }

  async function openFile(e: Expense) {
    try {
      await API.OpenExpenseAttachment(e.id);
    } catch (cause) {
      error = serviceErrorMessage(cause, "Die Datei konnte nicht geöffnet werden.");
    }
  }
</script>

<!-- Same list as on the invoice overview: count and search, rows that open the receipt. -->
<ListHeader
  count="{shown.length} {shown.length === 1 ? 'Ausgabe' : 'Ausgaben'}"
  placeholder="Ausgaben suchen"
  bind:query
/>
{#if error}<p class="received-error" role="alert">{error}</p>{/if}
{#if message}<p class="expense-message" role="status">{message}</p>{/if}
{#if shown.length}
  <div class="invoice-rows">
    {#each shown as e (e.id)}
      <EntryRow
        title={e.supplier}
        subtitle="{e.number ? `${e.number} · ` : ''}{label(e.category)}{e.years
          ? ` · ${yearsLabel(e.years)}`
          : ''}"
        meta={dateLabel(e.date)}
        amount={amountLabel(e.gross)}
        tooltip="Ausgabe bearbeiten"
        onclick={() => (editing = e)}
      >
        {#snippet status()}
          <PaymentStatus
            summary={e.cancelledAt
              ? { title: "Storniert", description: e.cancelReason }
              : e.paidDate
                ? {
                    title: "Bezahlt",
                    description:
                      `${paidLabels[e.paidWith] ?? ""} am ${dateLabel(e.paidDate)}`.trim(),
                    paid: true,
                  }
                : { title: "Offen", description: "noch nicht bezahlt" }}
          />
        {/snippet}
      </EntryRow>
    {/each}
  </div>
{:else if loaded}
  <div class="empty-state">
    <h2>{query ? "Keine Treffer" : "Noch keine Ausgaben erfasst"}</h2>
    <p>
      {query
        ? "Versuche es mit einem anderen Lieferanten oder einer anderen Kategorie."
        : "Kassenbons, PDF-Rechnungen ohne E-Rechnungsdaten und Ausgaben ohne Beleg-Datei legst du mit „Ausgabe erfassen“ mit wenigen Angaben an."}
    </p>
  </div>
{/if}
{#if editing}
  <ExpenseDialog
    initial={editing}
    {categories}
    {smallBusiness}
    ondelete={editing.id && !editing.cancelledAt
      ? () => (cancelling = editing as Expense)
      : undefined}
    onopenfile={editing.attachmentName ? () => openFile(editing as Expense) : undefined}
    onclose={() => (editing = null)}
    onsaved={async () => {
      message = editing?.id ? "Beleg gespeichert." : "Beleg erfasst.";
      editing = null;
      await load();
    }}
  />
{/if}

{#if cancelling}
  <FormDialog
    title="Beleg {cancelling.number} stornieren?"
    description="Der Beleg von {cancelling.supplier} über {amountLabel(
      cancelling.gross,
    )} fehlt dann in Umsatzsteuer und EÜR, bleibt aber sichtbar."
    {busy}
    error={cancelError}
    onclose={() => (cancelling = null)}
    onsubmit={cancelReceipt}
  >
    <label class="cancel-reason"
      >Warum stornierst du den Beleg?<input
        bind:value={cancelReason}
        required
        maxlength="500"
        placeholder="z. B. doppelt erfasst"
      /></label
    >
    {#snippet footer()}
      <span class="footer-spacer"></span>
      <button type="button" class="secondary" disabled={busy} onclick={() => (cancelling = null)}
        >Abbrechen</button
      >
      <button class="primary" disabled={busy || !cancelReason.trim()}>Stornieren</button>
    {/snippet}
  </FormDialog>
{/if}

<style>
  .cancel-reason {
    display: grid;
    gap: 6px;
  }
  .expense-message {
    margin: 0 0 12px;
    color: var(--theme-muted);
  }
</style>
