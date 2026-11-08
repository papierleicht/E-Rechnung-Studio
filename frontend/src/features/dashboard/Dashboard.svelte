<script lang="ts">
  import type { InvoiceSummary } from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/model/models";
  import type {
    PaymentBalance,
    ReminderProposal,
  } from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/store/models";
  import {
    DueReminders,
    TaxDeadlines,
  } from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/app/invoiceservice";
  import type { TaxDeadline } from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/app/models";
  import type { DashboardSummary } from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/app/models";
  import PaymentStatus from "../payments/PaymentStatus.svelte";
  import PlusHint from "../../components/PlusHint.svelte";
  import ActionTiles from "../../components/ActionTiles.svelte";
  import ListHeader from "../../components/ListHeader.svelte";
  import EntryRow from "../../components/EntryRow.svelte";
  import StatusLine from "../../components/StatusLine.svelte";
  import {
    addReceiptTile,
    newInvoiceTile,
    nextDeadline,
    openTile,
    summaryTotals,
    taxesTile,
    type ActionTile,
  } from "../../lib/actionTiles";
  import { amountLabel, dateLabel } from "../../lib/format";
  import { cached, remember } from "../../lib/cache";
  import { dashboardTasks, type DashboardTarget } from "../../lib/dashboard";
  import { reminderBasisLine } from "../../lib/reminders";

  let {
    invoices,
    balances = [],
    loadSummary,
    loadReminders = async () => (await DueReminders()) ?? [],
    loadDeadlines = async () => (await TaxDeadlines()) ?? [],
    ontaxes = () => {},
    onopeninvoices,
    onaccount = () => {},
    oncreate,
    onopen,
    onnavigate,
    disabled = false,
  }: {
    invoices: InvoiceSummary[];
    balances?: PaymentBalance[];
    loadSummary: () => Promise<DashboardSummary>;
    loadReminders?: () => Promise<ReminderProposal[]>;
    oncreate: () => unknown;
    onopen: (invoice: InvoiceSummary) => unknown;
    onnavigate: (target: DashboardTarget) => unknown;
    loadDeadlines?: () => Promise<TaxDeadline[]>;
    /** Opens a tax task under Steuern, e.g. "vat:2026-Q3" or "annual:2025". */
    ontaxes?: (task: string) => unknown;
    /** The open customer payments tile leads to the filtered invoice list. */
    onopeninvoices?: () => unknown;
    /** The status line leads to the account page. */
    onaccount?: () => unknown;
    disabled?: boolean;
  } = $props();

  // The last figures show right away on a revisit while they reload, so the tiles do not flicker.
  let summary = $state<DashboardSummary | null>(
    cached<DashboardSummary>("dashboard:summary") ?? null,
  );
  let loadError = $state(false);
  let reminders = $state<ReminderProposal[]>(
    cached<ReminderProposal[]>("dashboard:reminders") ?? [],
  );
  let deadlines = $state<TaxDeadline[]>(cached<TaxDeadline[]>("tax:deadlines") ?? []);
  let loadVersion = 0;
  const drafts = $derived(invoices.filter((item) => item.status === "draft").length);
  const tasks = $derived(summary ? dashboardTasks(summary, drafts) : []);
  const staleBank = $derived(summary?.financeIssues?.some((i) => i.code === "stale_balance"));
  const nextDue = $derived(
    (summary?.nextDue ?? []).flatMap((balance) => {
      const invoice = invoices.find((item) => item.id === balance.id);
      return invoice ? [{ balance, invoice }] : [];
    }),
  );

  const dueReminders = $derived(
    reminders.flatMap((proposal) => {
      const invoice = invoices.find((item) => item.id === proposal.invoiceId);
      return invoice ? [{ proposal, invoice }] : [];
    }),
  );

  async function refresh() {
    const version = ++loadVersion;
    loadError = false;
    // Reminders are an addition; a failure there must not hide the overview.
    void loadReminders().then(
      (result) => version === loadVersion && (reminders = remember("dashboard:reminders", result)),
      () => version === loadVersion && (reminders = []),
    );
    void loadDeadlines().then(
      (result) => version === loadVersion && (deadlines = remember("tax:deadlines", result)),
      () => version === loadVersion && (deadlines = []),
    );
    try {
      const result = await loadSummary();
      if (version === loadVersion) summary = remember("dashboard:summary", result);
    } catch {
      if (version === loadVersion) loadError = true;
    }
  }
  // Reload whenever invoices or payments change elsewhere in the app.
  $effect(() => {
    invoices;
    balances;
    void refresh();
    return () => loadVersion++;
  });

  // Same tiles as in the sections they lead to; the totals come from the summary.
  const deadline = $derived(nextDeadline(deadlines));
  const unavailable = $derived(loadError && !summary ? { hint: "Nicht verfügbar" } : {});
  const tiles = $derived<ActionTile[]>([
    { ...newInvoiceTile(() => void oncreate()), primary: true },
    addReceiptTile(() => void onnavigate("received")),
    taxesTile(
      deadline,
      () => void (deadline ? ontaxes(deadline.task || deadline.periodId) : onnavigate("taxes")),
    ),
    {
      ...openTile(
        "in",
        summary && summaryTotals(summary.receivable, summary.receivableOverdue),
        () => void (onopeninvoices ? onopeninvoices() : onnavigate("invoices")),
      ),
      ...unavailable,
    },
    {
      ...openTile(
        "out",
        summary && summaryTotals(summary.payable, summary.payableOverdue),
        () => void onnavigate("received"),
      ),
      ...unavailable,
    },
  ]);

  function dueLabel(balance: PaymentBalance) {
    if (balance.installments && balance.nextDate)
      return Number(balance.overdueAmount) > 0
        ? `Rate überfällig seit ${dateLabel(balance.nextDate)}`
        : `nächste Rate am ${dateLabel(balance.nextDate)}`;
    if (!balance.dueDate) return "ohne Fälligkeit";
    return balance.dueDate < (summary?.today ?? "")
      ? `überfällig seit ${dateLabel(balance.dueDate)}`
      : `fällig am ${dateLabel(balance.dueDate)}`;
  }
</script>

<div class="dashboard">
  <header>
    <h1>Übersicht</h1>
  </header>

  {#if loadError}
    <div class="notice" role="alert">
      <span>Die Übersicht konnte nicht geladen werden.</span><button
        class="text-button"
        onclick={refresh}>Erneut versuchen</button
      >
    </div>
  {/if}

  <ActionTiles {tiles} {disabled} />

  <!-- The same list header and rows as on the other pages. -->
  <section class="list" aria-label="Zu erledigen">
    <ListHeader count="Zu erledigen" />
    {#if !summary && !deadlines.length}
      <div class="empty-state"><p>{loadError ? "" : "Wird geladen …"}</p></div>
    {:else if deadlines.length || tasks.length}
      <div class="invoice-rows task-rows">
        {#each deadlines as d (d.task || d.periodId)}
          <EntryRow
            title={d.label}
            subtitle={d.overdue
              ? "Werte für ELSTER ansehen und übermitteln"
              : "Werte ansehen und an ELSTER übermitteln"}
            meta={dateLabel(d.due)}
            {disabled}
            onclick={() => void ontaxes(d.task || d.periodId)}
          >
            {#snippet status()}
              <PaymentStatus
                summary={{ title: d.overdue ? "Überfällig" : "Bald fällig", overdue: d.overdue }}
              />
            {/snippet}
          </EntryRow>
        {/each}
        {#each tasks as task (task.id)}
          <EntryRow
            title={task.title}
            subtitle={task.detail}
            {disabled}
            onclick={() => void (task.id === "tax-profile" ? ontaxes("") : onnavigate(task.target))}
          >
            {#snippet status()}
              {#if task.urgent}<PaymentStatus
                  summary={{ title: "Dringend", overdue: true }}
                />{:else}<span></span>{/if}
            {/snippet}
          </EntryRow>
        {/each}
      </div>
    {:else}
      <div class="empty-state">
        <h2>Alles erledigt</h2>
        <p>Gerade ist nichts offen.</p>
      </div>
    {/if}
    {#if staleBank}<PlusHint
        id="dashboard-stale-bank"
        topic="fints"
        text="Mit Plus rufst du Umsätze und Kontostand per FinTS ab – ohne CSV-Export."
      />{/if}
    {#if invoices.length >= 5}<PlusHint
        id="dashboard-backup"
        topic="backup"
        text={`Deine ${invoices.length} Rechnungen liegen auf diesem Gerät. Mit dem Backup-Zusatz sind sie zusätzlich außer Haus gesichert, ohne dass du etwas tun musst.`}
      />{/if}
  </section>

  {#if dueReminders.length}
    <section class="list" aria-label="Zahlungserinnerungen fällig">
      <ListHeader count="Zahlungserinnerungen fällig" />
      <div class="invoice-rows task-rows">
        {#each dueReminders as { proposal, invoice } (proposal.invoiceId)}
          <EntryRow
            title="{proposal.partner || invoice.customerName || 'Ohne Kunde'} · {proposal.title}"
            subtitle="{proposal.number || invoice.number} · {reminderBasisLine(proposal)}"
            tooltip="Rechnung öffnen und Erinnerung erstellen"
            {disabled}
            onclick={() => void onopen(invoice)}
          >
            {#snippet status()}
              <PaymentStatus summary={{ title: "Erinnerung fällig", overdue: true }} />
            {/snippet}
          </EntryRow>
        {/each}
      </div>
    </section>
  {/if}

  {#if nextDue.length}
    <section class="list" aria-label="Fällig als Nächstes">
      <ListHeader count="Fällig als Nächstes">
        {#snippet aside()}
          <button class="text-button" onclick={() => onnavigate("invoices")} {disabled}
            >Alle Kundenrechnungen →</button
          >
        {/snippet}
      </ListHeader>
      <div class="invoice-rows">
        {#each nextDue as { balance, invoice } (balance.id)}
          <EntryRow
            title={balance.partner || invoice.customerName || "Ohne Kunde"}
            subtitle="{balance.number || invoice.number} · {dueLabel(balance)}"
            meta={(balance.installments && balance.nextDate) || balance.dueDate
              ? dateLabel((balance.installments && balance.nextDate) || balance.dueDate)
              : ""}
            amount={amountLabel(invoice.gross)}
            {disabled}
            onclick={() => void onopen(invoice)}
          >
            {#snippet status()}
              <PaymentStatus {balance} />
            {/snippet}
          </EntryRow>
        {/each}
      </div>
    </section>
  {/if}
  <StatusLine onselect={onaccount} />
</div>

<style>
  /* Tasks and reminders carry no amount: the title takes that column, so it
     stays on one line down to the 900px minimum window. */
  .task-rows :global(.invoice-entry) {
    grid-template-columns: minmax(0, 1fr) 84px minmax(110px, max-content) 16px;
  }
  .task-rows :global(.invoice-entry-amount) {
    display: none;
  }
  .dashboard {
    max-width: var(--ui-page-width);
    margin: 0 auto;
    padding: 0 0 24px;
  }
  header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 20px;
    min-height: 36px;
    margin-bottom: var(--ui-section-gap);
  }
  .dashboard h1 {
    font-size: var(--ui-title-size);
    font-weight: 650;
    line-height: 1.25;
    letter-spacing: -0.4px;
    margin: 0;
  }
  .list + .list {
    margin-top: var(--ui-section-gap);
  }
  .text-button {
    border: 0;
    padding: 4px 0;
    background: transparent;
    color: var(--theme-accent);
    font-size: 12px;
  }
  .text-button:hover {
    text-decoration: underline;
  }
  .notice {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 4px 10px;
    margin-bottom: 16px;
    font-size: 12px;
    color: var(--theme-muted);
  }
</style>
