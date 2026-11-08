<script lang="ts">
  import type {
    FinanceFigure,
    TaxOverview,
  } from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/app/models";
  import ChevronRightIcon from "@lucide/svelte/icons/chevron-right";
  import EntriesDialog from "./EntriesDialog.svelte";
  import { amountLabel, dateLabel } from "../../lib/format";
  import type { TaxTask } from "../../lib/taxTasks";

  // The year by kind of tax: what went out, what is still open or set aside,
  // and what comes next. Framed like the panels under "Konto & Finanzen".
  let {
    overview,
    year = $bindable(),
    years,
    current,
    nextVAT,
    onopen,
  }: {
    overview: TaxOverview;
    year: number;
    years: number[];
    /** The running year: only then do "open" and "next" values apply. */
    current: boolean;
    /** The next Voranmeldung still to file, if any. */
    nextVAT: TaxTask | null;
    onopen: (taskId: string) => void;
  } = $props();

  // The Freibetrag of § 11 Abs. 1 Satz 3 Nr. 1 GewStG; above it a
  // Gewerbesteuererklärung is due (§ 25 Abs. 1 Nr. 1 GewStDV).
  const allowance = 24500;
  const taxes = $derived(overview.taxes);
  const setup = $derived(overview.setup);
  const annual = $derived(overview.annual?.find((a) => a.year === year));
  const trade = $derived(setup.activity !== "freelance");
  const profit = $derived(Number(taxes.profit) || 0);
  const value = (f: FinanceFigure) => Number(f.amount) || 0;
  const paid = $derived(
    value(taxes.vatPaid) + value(taxes.incomeTaxPaid) + (trade ? value(taxes.tradeTaxPaid) : 0),
  );
  const open = $derived(
    current
      ? Math.max(value(taxes.vatOpen), 0) +
          (taxes.incomeTaxReserve.available ? value(taxes.incomeTaxReserve) : 0)
      : 0,
  );
  let shown = $state<FinanceFigure | null>(null);
</script>

<section class="tax-year" aria-labelledby="tax-year-title">
  <header class="tax-year-head">
    <h2 id="tax-year-title">Steuern {year}</h2>
    <select class="year-select" bind:value={year} aria-label="Jahr">
      {#each years as y}<option value={y}>{y}</option>{/each}
    </select>
  </header>
  <p class="headline">
    {current ? "Bisher gezahlt" : "Gezahlt"} <strong>{amountLabel(paid)}</strong>{#if current}{" "}·
      offen und zurückgelegt <strong>{amountLabel(open)}</strong>{/if}
  </p>
  <p class="hint">
    Nach den Bankumsätzen, die du als Steuer eingeordnet hast, und deinem Rücklagensatz. Die App
    berechnet keine Einkommen- oder Gewerbesteuer.
  </p>

  <div class="tax-rows" role="table" aria-label="Steuern {year} nach Art">
    <div class="tax-row tax-row-head" role="row">
      <span role="columnheader">Steuer</span>
      <span role="columnheader" class="num">Gezahlt</span>
      <span role="columnheader" class="num">{current ? "Offen / Rücklage" : ""}</span>
      <span role="columnheader">Als Nächstes</span>
    </div>

    <div class="tax-row" role="row">
      <span class="what" role="cell"
        ><strong>Umsatzsteuer</strong><small
          >{setup.smallBusiness
            ? "Als Kleinunternehmer nur bei Steuerschuld als Empfänger"
            : "Auf deine Umsätze, minus Vorsteuer, ans Finanzamt"}</small
        ></span
      >
      <span class="num" role="cell"
        ><button type="button" class="amount" onclick={() => (shown = taxes.vatPaid)}
          >{amountLabel(taxes.vatPaid.amount)}</button
        ><small class="narrow-label">gezahlt</small></span
      >
      <span class="num" role="cell"
        >{#if current}<button type="button" class="amount" onclick={() => (shown = taxes.vatOpen)}
            >{amountLabel(taxes.vatOpen.amount)}</button
          ><small>offen</small>{/if}</span
      >
      <span class="next" role="cell">
        {#if nextVAT}<button type="button" class="next-task" onclick={() => onopen(nextVAT.id)}
            ><span
              >{nextVAT.title.replace("Umsatzsteuer-", "")}<small
                >bis {dateLabel(nextVAT.due)}</small
              ></span
            ><ChevronRightIcon size={16} aria-hidden="true" /></button
          >{:else}<small>Nichts offen</small>{/if}
      </span>
    </div>

    <div class="tax-row" role="row">
      <span class="what" role="cell"
        ><strong>Einkommensteuer</strong><small
          >Auf deinen Gewinn, mit Soli und ggf. Kirchensteuer, privat gezahlt</small
        ></span
      >
      <span class="num" role="cell"
        ><button type="button" class="amount" onclick={() => (shown = taxes.incomeTaxPaid)}
          >{amountLabel(taxes.incomeTaxPaid.amount)}</button
        ><small class="narrow-label">gezahlt</small></span
      >
      <span class="num" role="cell"
        >{#if current}{#if taxes.incomeTaxReserve.available}<button
              type="button"
              class="amount"
              onclick={() => (shown = taxes.incomeTaxReserve)}
              >{amountLabel(taxes.incomeTaxReserve.amount)}</button
            ><small>Rücklage</small>{:else}<small class="missing"
              >Rücklagensatz fehlt, unter „Konto & Finanzen“</small
            >{/if}{/if}</span
      >
      <span class="next" role="cell">
        {#if current && taxes.incomeTaxNext}<small
            >Vorauszahlung {dateLabel(taxes.incomeTaxNext)}, falls festgesetzt</small
          >{/if}
        {#if annual}<button
            type="button"
            class="next-task"
            onclick={() => onopen(`annual:${annual.year}`)}
            ><span
              >Erklärung {annual.year}<small
                >{annual.filing
                  ? `übermittelt am ${dateLabel(annual.filing.submittedOn)}`
                  : `bis ${dateLabel(annual.due)}`}</small
              ></span
            ><ChevronRightIcon size={16} aria-hidden="true" /></button
          >{/if}
      </span>
    </div>

    {#if trade}
      <div class="tax-row" role="row">
        <span class="what" role="cell"
          ><strong>Gewerbesteuer</strong><small
            >{setup.activity === "trade"
              ? "Auf den Gewerbeertrag über 24.500 €, an die Gemeinde"
              : "Nur bei einem Gewerbe; lege die Tätigkeit im Steuerprofil fest"}</small
          ></span
        >
        <span class="num" role="cell"
          ><button type="button" class="amount" onclick={() => (shown = taxes.tradeTaxPaid)}
            >{amountLabel(taxes.tradeTaxPaid.amount)}</button
          ><small class="narrow-label">gezahlt</small></span
        >
        <span class="num" role="cell"
          >{#if current}<small>rechnet die App nicht</small>{/if}</span
        >
        <span class="next" role="cell">
          {#if current && taxes.tradeTaxNext}<small
              >Vorauszahlung {dateLabel(taxes.tradeTaxNext)}, falls festgesetzt</small
            >{/if}
          <small
            >Gewinn laut EÜR {amountLabel(taxes.profit)}: {profit > allowance
              ? "über dem Freibetrag, Erklärung abgeben"
              : "unter dem Freibetrag von 24.500 €"}</small
          >
        </span>
      </div>
    {/if}
  </div>
</section>

{#if shown}
  <EntriesDialog
    title={shown.label}
    description={shown.explanation}
    entries={shown.entries ?? []}
    onclose={() => (shown = null)}
  />
{/if}

<style>
  /* Framed like the panels under "Konto & Finanzen". */
  .tax-year {
    padding: var(--ui-card-padding);
    background: var(--theme-surface);
    border: 1px solid var(--theme-border);
    border-radius: 12px;
    min-width: 0;
    /* The rows follow the panel's width, not the window's. */
    container-type: inline-size;
  }
  .tax-year-head {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 8px 16px;
  }
  /* Size and weight come from the app's section title. */
  .tax-year-head h2 {
    margin: 0;
    color: var(--theme-text);
  }
  .year-select {
    width: auto;
    margin-left: auto;
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
  .hint {
    margin: 0 0 14px;
    font-size: 12px;
    line-height: 1.5;
    color: var(--theme-muted);
  }
  .tax-rows {
    border: 1px solid var(--theme-border);
    border-radius: 10px;
    overflow: hidden;
  }
  .tax-row {
    display: grid;
    grid-template-columns: minmax(0, 1.6fr) minmax(7rem, 0.8fr) minmax(8rem, 0.9fr) minmax(
        0,
        1.5fr
      );
    gap: 4px 16px;
    align-items: start;
    padding: 12px 14px;
    border-top: 1px solid var(--theme-border);
    font-size: 13px;
  }
  .tax-row:first-child {
    border-top: 0;
  }
  .tax-row-head {
    padding-block: 8px;
    background: var(--theme-surface-muted);
    font-size: 12px;
    font-weight: 600;
    color: var(--theme-muted);
  }
  .what,
  .num,
  .next {
    display: flex;
    flex-direction: column;
    gap: 2px;
    min-width: 0;
  }
  .what small,
  .num small,
  .next small {
    font-size: 12px;
    line-height: 1.45;
    color: var(--theme-muted);
    overflow-wrap: anywhere;
  }
  .num {
    align-items: flex-end;
    text-align: right;
  }
  .narrow-label {
    display: none;
  }
  .missing {
    color: var(--theme-warning-text);
  }
  .amount {
    padding: 0;
    border: 0;
    background: none;
    color: var(--theme-text);
    font: inherit;
    font-weight: 600;
    font-variant-numeric: tabular-nums;
    cursor: pointer;
    text-decoration: underline dotted var(--theme-muted);
    text-underline-offset: 3px;
  }
  .amount:hover {
    color: var(--theme-accent);
  }
  .next-task {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 8px;
    width: 100%;
    padding: 6px 8px;
    margin: -2px 0 0 -8px;
    border: 0;
    border-radius: 6px;
    background: none;
    color: var(--theme-text);
    font: inherit;
    text-align: left;
    cursor: pointer;
  }
  .next-task span {
    display: flex;
    flex-direction: column;
    gap: 2px;
    min-width: 0;
    font-weight: 600;
  }
  .next-task small {
    font-weight: 400;
  }
  .next-task:hover {
    background: var(--theme-surface-muted);
  }
  @container (max-width: 760px) {
    .tax-row {
      grid-template-columns: minmax(0, 1fr) auto;
    }
    .tax-row-head {
      display: none;
    }
    .tax-row .what {
      grid-row: span 2;
    }
    .narrow-label {
      display: block;
    }
    .tax-row .num {
      grid-column: 2;
    }
    .tax-row .next {
      grid-column: 1 / -1;
    }
  }
</style>
