<script lang="ts">
  import type {
    FinanceEntry,
    FinanceReport,
  } from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/app/models";
  import EntriesDialog from "./EntriesDialog.svelte";
  import { amountLabel, dateLabel, localToday } from "../../lib/format";

  // One bar like an election result, valued today and independent of the chosen
  // period: what the business has and will receive on the left, what still goes
  // out on the right. Paid expenses are already part of the balance and do not
  // appear again. When the left side is longer than the right, the money
  // covers everything that is still to pay. The report runs from January 1 to
  // today, because the income tax reserve follows the result of the year so far.
  let {
    report,
    failed = false,
    onaddbalance,
  }: { report: FinanceReport | null; failed?: boolean; onaddbalance?: () => void } = $props();
  const today = localToday();

  type Part = {
    id: string;
    label: string;
    amount: number;
    hint: string;
    tone: string;
    // What a click on the row lists: the invoices, balances or tax lines behind it.
    entries: FinanceEntry[];
  };
  let shown = $state<Part | null>(null);
  const num = (value: string | undefined) => Math.abs(Number(value ?? 0)) || 0;

  const view = $derived.by(() => {
    if (!report) return null;
    const balance = Number(report.bankBalance.amount) || 0;
    const hasBalance = report.bankBalance.available;
    const receivables = num(report.receivables.amount);
    const payables = num(report.payables.amount);
    const taxes = num(report.vatReserve.amount) + num(report.incomeTaxReserve.amount);
    // Older reports without the field count no tax on open invoices.
    const pending = num(report.pendingTax?.amount);
    const overdue = (report.receivables.entries ?? []).filter((e) => e.date && e.date < today);
    const latest = (report.bankBalance.entries ?? []).reduce(
      (date, e) => (e.date > date ? e.date : date),
      "",
    );
    const has: Part[] = [];
    const owes: Part[] = [];
    if (hasBalance && balance > 0)
      has.push({
        id: "balance",
        label: "Kontostand",
        amount: balance,
        hint: latest ? `Stand ${dateLabel(latest)}` : "",
        tone: "in-sure",
        entries: report.bankBalance.entries ?? [],
      });
    has.push({
      id: "receivables",
      label: "Kommt noch rein",
      amount: receivables,
      hint: overdue.length
        ? `${report.receivables.entries?.length ?? 0} offene Rechnungen, ${overdue.length} überfällig`
        : `${report.receivables.entries?.length ?? 0} offene Rechnungen`,
      tone: "in-open",
      entries: report.receivables.entries ?? [],
    });
    // An overdrawn account is money owed to the bank.
    if (hasBalance && balance < 0)
      owes.push({
        id: "overdraft",
        label: "Konto im Minus",
        amount: -balance,
        hint: latest ? `Stand ${dateLabel(latest)}` : "",
        tone: "out-sure",
        entries: report.bankBalance.entries ?? [],
      });
    owes.push({
      id: "payables",
      label: "Muss noch bezahlt werden",
      amount: payables,
      hint: `${report.payables.entries?.length ?? 0} offene Eingangsrechnungen`,
      tone: "out-sure",
      entries: report.payables.entries ?? [],
    });
    owes.push({
      id: "taxes",
      label: "Steuer-Topf",
      amount: taxes,
      hint: report.incomeTaxReserve.available
        ? "Offene Umsatzsteuer und Rücklage Einkommensteuer"
        : "Offene Umsatzsteuer, ohne Rücklage Einkommensteuer",
      tone: "out-open",
      entries: [...(report.vatReserve.entries ?? []), ...(report.incomeTaxReserve.entries ?? [])],
    });
    // Only once customers pay: it lowers what is expected to remain, not what
    // is free today, and sits in the middle like the open invoices.
    owes.push({
      id: "pending",
      label: "Steuern auf offene Rechnungen",
      amount: pending,
      hint: "Fällt erst an, wenn Kunden zahlen",
      tone: "out-later",
      entries: report.pendingTax?.entries ?? [],
    });
    const total = [...has, ...owes].reduce((sum, p) => sum + p.amount, 0);
    const expected = balance + receivables - payables - taxes - pending;
    const free = balance - payables - taxes;
    return { hasBalance, has, owes, total, expected, free };
  });

  const share = (amount: number) => (view && view.total > 0 ? (amount / view.total) * 100 : 0);
  // The inner, pale parts sit in the middle; certain money sits outside.
  const bar = $derived(view ? [...view.has, ...[...view.owes].reverse()] : []);
  const visible = $derived(bar.filter((part) => part.amount > 0));
  const outgoing = (part: Part) => view?.owes.some((p) => p.id === part.id) ?? false;

  // Hovering or focusing a part names it above the bar; a click lists its entries.
  // By id: a part kept in state is a proxy and never identical to the original.
  let hoveredId = $state("");
  const hovered = $derived(visible.find((part) => part.id === hoveredId) ?? null);
  const tipAt = $derived.by(() => {
    if (!hovered) return 50;
    let before = 0;
    for (const part of visible) {
      if (part.id === hoveredId) return before + share(part.amount) / 2;
      before += share(part.amount);
    }
    return 50;
  });
</script>

<section class="coverage" aria-labelledby="coverage-title">
  <header>
    <h2 id="coverage-title">Reicht das Geld?</h2>
    <span class="today">Stand heute</span>
  </header>
  {#if failed}
    <p class="hint">Der Überblick konnte nicht berechnet werden.</p>
  {:else if view}
    {#if !view.hasBalance}
      <p class="missing">
        Ohne Kontostand fehlt die wichtigste Zahl.
        {#if onaddbalance}<button type="button" class="text-button" onclick={onaddbalance}
            >Kontostand eintragen</button
          >{/if}
      </p>
    {/if}
    <p class="headline" class:short={view.expected < 0}>
      {#if view.expected >= 0}
        Voraussichtlich bleiben <strong>{amountLabel(view.expected)}</strong> übrig
      {:else}
        Voraussichtlich fehlen <strong>{amountLabel(-view.expected)}</strong>
      {/if}
    </p>
    {#if view.hasBalance}
      <p class="hint">
        {#if view.free >= 0}
          Auch ohne offene Kundenrechnungen ist alles gedeckt: {amountLabel(view.free)} frei.
        {:else}
          Ohne die offenen Kundenrechnungen fehlen {amountLabel(-view.free)}. Erst wenn Kunden
          zahlen, ist alles gedeckt.
        {/if}
      </p>
    {/if}

    {#if view.total > 0}
      <div class="track" role="group" aria-label="Geld und Eingänge gegen Ausgaben und Steuern">
        <div class="bar">
          {#each visible as part (part.id)}
            <button
              type="button"
              class="segment {part.tone}"
              class:active={hoveredId === part.id}
              style:flex-basis="{share(part.amount)}%"
              aria-label="{part.label}: {outgoing(part) ? '−' : ''}{amountLabel(
                part.amount,
              )}. {part.hint}"
              onmouseenter={() => (hoveredId = part.id)}
              onmouseleave={() => (hoveredId = "")}
              onfocus={() => (hoveredId = part.id)}
              onblur={() => (hoveredId = "")}
              onclick={() => (shown = part)}
            ></button>
          {/each}
        </div>
        {#if hovered}
          <div class="tip" style:--at="{tipAt}%" aria-hidden="true">
            <span class="tip-head"><span class="swatch {hovered.tone}"></span>{hovered.label}</span>
            <strong>{outgoing(hovered) ? "− " : ""}{amountLabel(hovered.amount)}</strong>
            {#if hovered.hint}<span class="tip-note">{hovered.hint}</span>{/if}
            <span class="tip-note">Klicken für die Einträge</span>
          </div>
        {/if}
      </div>
      <div class="scale" aria-hidden="true">
        <span>Geld und Eingänge</span>
        <span>Ausgaben und Steuern</span>
      </div>
    {/if}
  {:else}
    <p class="hint">Wird berechnet …</p>
  {/if}
</section>

{#if shown}
  <EntriesDialog
    title={shown.label}
    description="{amountLabel(shown.amount)} · {shown.hint}"
    entries={shown.entries}
    onclose={() => (shown = null)}
  />
{/if}

<style>
  .coverage {
    --in-sure: var(--theme-accent);
    --in-open: color-mix(in srgb, var(--theme-accent) 38%, var(--theme-surface));
    --out-sure: var(--theme-error-text);
    --out-open: color-mix(in srgb, var(--theme-error-text) 38%, var(--theme-surface));
    background: var(--theme-surface);
    border: 1px solid var(--theme-border);
    border-radius: 12px;
    padding: var(--ui-card-padding);
    min-width: 0;
  }
  header {
    display: flex;
    flex-wrap: wrap;
    align-items: baseline;
    justify-content: space-between;
    gap: 4px 12px;
  }
  /* Size and weight come from the app's section title. */
  h2 {
    margin: 0;
    color: var(--theme-text);
  }
  .today,
  .hint,
  .scale {
    font-size: 12px;
    color: var(--theme-muted);
  }
  .headline {
    margin: 8px 0 2px;
    font-size: 15px;
    color: var(--theme-text);
  }
  .headline strong {
    font-size: 22px;
    font-variant-numeric: tabular-nums;
  }
  .headline.short strong {
    color: var(--theme-error-text);
  }
  .hint {
    margin: 0;
    line-height: 1.5;
  }
  .missing {
    margin: 8px 0 0;
    font-size: 13px;
    color: var(--theme-error-text);
  }
  .track {
    position: relative;
    margin: 12px 0 4px;
    padding: 6px 0;
  }
  .bar {
    display: flex;
    gap: 2px;
    height: 22px;
  }
  .segment {
    padding: 0;
    border: 0;
    cursor: pointer;
    min-width: 2px;
    flex-grow: 0;
    flex-shrink: 1;
  }
  .segment:first-child {
    border-radius: 4px 0 0 4px;
  }
  .segment:last-child {
    border-radius: 0 4px 4px 0;
  }
  .segment:hover,
  .segment:focus-visible,
  .segment.active {
    outline: 2px solid var(--theme-text);
    outline-offset: 1px;
    position: relative;
    z-index: 1;
  }
  /* Above the bar at the hovered part, kept inside the section's width. */
  .tip {
    position: absolute;
    bottom: calc(100% + 4px);
    left: clamp(125px, var(--at), calc(100% - 125px));
    transform: translateX(-50%);
    z-index: 3;
    display: flex;
    flex-direction: column;
    gap: 2px;
    min-width: 220px;
    max-width: 250px;
    padding: 8px 12px;
    border: 1px solid var(--theme-border);
    border-radius: 8px;
    background: var(--theme-surface);
    box-shadow: 0 6px 18px var(--theme-shadow);
    pointer-events: none;
  }
  .tip-head {
    white-space: nowrap;
    display: flex;
    align-items: center;
    gap: 8px;
    font-size: 13px;
    color: var(--theme-text);
  }
  .tip strong {
    font-size: 15px;
    font-variant-numeric: tabular-nums;
    color: var(--theme-text);
  }
  .tip-note {
    font-size: 12px;
    color: var(--theme-muted);
  }
  .scale {
    display: flex;
    justify-content: space-between;
    gap: 12px;
  }
  /* Money that is certain is solid; money that is still expected is hatched. */
  .in-sure {
    background: var(--in-sure);
  }
  .in-open {
    background: repeating-linear-gradient(
      135deg,
      var(--in-open) 0 5px,
      color-mix(in srgb, var(--in-sure) 55%, var(--theme-surface)) 5px 7px
    );
  }
  .out-sure {
    background: var(--out-sure);
  }
  .out-open {
    background: repeating-linear-gradient(
      45deg,
      var(--out-open) 0 5px,
      color-mix(in srgb, var(--out-sure) 55%, var(--theme-surface)) 5px 7px
    );
  }
  /* Tax that arises only with a customer payment: the diagonal of the tax pot
     beside it, in a red turned slightly towards the warning colour. */
  .out-later {
    --later: color-mix(in srgb, var(--out-sure) 60%, var(--theme-warning-border));
    background: repeating-linear-gradient(
      45deg,
      color-mix(in srgb, var(--later) 38%, var(--theme-surface)) 0 5px,
      color-mix(in srgb, var(--later) 55%, var(--theme-surface)) 5px 7px
    );
  }
  .swatch {
    flex: none;
    width: 12px;
    height: 12px;
    border-radius: 3px;
  }
</style>
