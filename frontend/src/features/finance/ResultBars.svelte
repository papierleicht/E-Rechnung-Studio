<script lang="ts">
  import type { FinanceFigure } from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/app/models";
  import FigureCard from "./FigureCard.svelte";
  import { amountLabel } from "../../lib/format";

  // Betriebseinnahmen, Betriebsausgaben and Ergebnis of the chosen period as
  // three bars on one baseline. Income and expenses both point up, so their
  // heights compare directly; the result points down when it is a loss. The
  // Umsatzsteuer paid to the tax office is hatched inside the expenses, so a
  // quarter that pays the previous quarter's tax shows why it looks weaker.
  let {
    income,
    expenses,
    result,
  }: { income: FinanceFigure; expenses: FinanceFigure; result: FinanceFigure } = $props();

  const pattern = $props.id();
  type BarId = "income" | "expenses" | "result";
  // The result is the headline, so its details show until another bar is chosen.
  // Hovering a bar shows its details for as long as the pointer stays on it.
  let selected = $state<BarId>("result");
  let hovered = $state<BarId | null>(null);
  const shownId = $derived(hovered ?? selected);

  const num = (value: string) => Number(value) || 0;
  const view = $derived.by(() => {
    // The demo hands expenses over with a minus sign; the bars show amounts.
    const inValue = Math.abs(num(income.amount));
    const outValue = Math.abs(num(expenses.amount));
    const resultValue = num(result.amount);
    const vat = (expenses.entries ?? [])
      .filter((e) => e.kind === "vat")
      .reduce((sum, e) => sum + Math.abs(num(e.amount)), 0);
    // An empty period still gets a readable axis up to 100 €.
    const top = niceCeil(Math.max(inValue, outValue, resultValue, 100));
    const bottom = resultValue < 0 ? -niceCeil(-resultValue, top) : 0;
    return { inValue, outValue, resultValue, vat, top, bottom };
  });

  // A round upper end with a step of 1, 2 or 5 times a power of ten.
  function niceCeil(value: number, against = value): number {
    const step = niceStep(Math.max(value, against) / 4);
    return Math.ceil(value / step) * step;
  }
  // Whole euros only, so the axis never repeats a rounded label.
  function niceStep(raw: number): number {
    const power = 10 ** Math.floor(Math.log10(Math.max(raw, 1)));
    return ([1, 2, 5, 10].find((k) => k * power >= raw) ?? 10) * power;
  }
  const ticks = $derived.by(() => {
    const step = niceStep((view.top - view.bottom) / 4);
    const list: number[] = [];
    for (let v = Math.ceil(view.bottom / step) * step; v <= view.top + 1e-6; v += step)
      list.push(v);
    return list;
  });

  // The drawing follows the width it gets, so labels keep their size instead of
  // shrinking with the picture.
  let width = $state(360);
  const plotTop = 24,
    plotBottom = 188,
    left = 48;
  // A loss writes its amount below the bar; the names move down to make room.
  const height = $derived(plotBottom + (view.resultValue < 0 ? 50 : 32));
  const slot = $derived((width - left) / 3);
  const barWidth = $derived(Math.min(64, slot * 0.5));
  const y = (v: number) =>
    plotTop + ((view.top - v) / (view.top - view.bottom || 1)) * (plotBottom - plotTop);
  const centre = (i: number) => left + slot * i + slot / 2;
  // An amount that would be wider than its column (about 7.6px per character
  // at 13px) drops its cents, so it does not run into the neighbouring one.
  const valueLabel = (v: number, sign = "") => {
    const full = sign + amountLabel(v);
    return full.length * 7.6 > slot - 6
      ? `${sign}${Math.round(v).toLocaleString("de-DE")} €`
      : full;
  };
  const short = (v: number) =>
    (v < 0 ? "−" : "") + Math.abs(v).toLocaleString("de-DE", { maximumFractionDigits: 0 });

  const bars = $derived([
    { id: "income" as const, figure: income, value: view.inValue, label: "Einnahmen" },
    { id: "expenses" as const, figure: expenses, value: view.outValue, label: "Ausgaben" },
    { id: "result" as const, figure: result, value: view.resultValue, label: "Ergebnis" },
  ]);
  const chosen = $derived(bars.find((bar) => bar.id === shownId)!.figure);
</script>

<div class="result-bars">
  <div class="layout">
    <div class="chart" bind:clientWidth={width}>
      <svg
        width="100%"
        {height}
        viewBox="0 0 {width} {height}"
        role="group"
        aria-label="Betriebseinnahmen, Betriebsausgaben und Ergebnis im Zeitraum"
      >
        <defs>
          <pattern
            id="{pattern}-vat"
            width="6"
            height="6"
            patternUnits="userSpaceOnUse"
            patternTransform="rotate(45)"
          >
            <rect width="6" height="6" class="vat-ground" />
            <line x1="0" y1="0" x2="0" y2="6" class="vat-line" />
          </pattern>
        </defs>
        {#each ticks as tick (tick)}
          <line
            class="grid"
            class:zero={tick === 0}
            x1={left}
            x2={width}
            y1={y(tick)}
            y2={y(tick)}
          />
          <text class="tick" x={left - 8} y={y(tick) + 4}>{short(tick)}</text>
        {/each}
        {#each bars as bar, i (bar.id)}
          {@const x = centre(i) - barWidth / 2}
          {@const top = Math.min(y(bar.value), y(0))}
          {@const size = Math.max(Math.abs(y(bar.value) - y(0)), bar.value ? 2 : 0)}
          <g
            class="bar {bar.id}"
            class:loss={bar.id === "result" && bar.value < 0}
            class:active={shownId === bar.id}
            role="button"
            tabindex="0"
            aria-pressed={selected === bar.id}
            aria-label="{bar.figure.label}: {amountLabel(bar.figure.amount)}"
            onclick={() => (selected = bar.id)}
            onmouseenter={() => (hovered = bar.id)}
            onmouseleave={() => (hovered = null)}
            onkeydown={(event) => {
              if (event.key === "Enter" || event.key === " ") {
                event.preventDefault();
                selected = bar.id;
              }
            }}
          >
            <rect class="hit" x={left + slot * i + 4} y="0" width={slot - 8} {height} rx="8" />
            <rect class="fill" {x} y={top} width={barWidth} height={size} rx="4" />
            {#if bar.id === "expenses" && view.vat > 0}
              <rect
                {x}
                y={y(bar.value)}
                width={barWidth}
                height={Math.max(y(bar.value - view.vat) - y(bar.value), 1)}
                rx="4"
                fill="url(#{pattern}-vat)"
              />
            {/if}
            <text
              class="value"
              x={centre(i)}
              y={bar.value < 0 ? y(bar.value) + 16 : y(bar.value) - 8}
              >{valueLabel(bar.value, bar.id === "result" && bar.value > 0 ? "+" : "")}</text
            >
            <text class="name" x={centre(i)} y={height - 10}>{bar.label}</text>
          </g>
        {/each}
      </svg>
      {#if view.vat > 0 || income.incomplete || expenses.incomplete}
        <div class="bar-notes">
          {#if view.vat > 0}
            <p>
              <span class="swatch" aria-hidden="true"></span>Davon {amountLabel(view.vat)}
              Umsatzsteuer ans Finanzamt, oft für den Zeitraum davor.
            </p>
          {/if}
          {#if income.incomplete || expenses.incomplete}
            <p class="incomplete">Unvollständig: Es fehlen Daten, siehe offene Punkte oben.</p>
          {/if}
        </div>
      {/if}
    </div>
    <div class="detail" aria-live="polite">
      <FigureCard figure={chosen} plain />
    </div>
  </div>
</div>

<style>
  .result-bars {
    --bar-in: color-mix(in srgb, var(--theme-accent) 60%, var(--theme-surface));
    --bar-out: color-mix(in srgb, var(--theme-error-text) 40%, var(--theme-surface));
    container-type: inline-size;
    min-width: 0;
  }
  /* Chart and details side by side; the details move below only where the
     section is too narrow for both (below the 900px window, e.g. zoomed in). */
  .layout {
    display: grid;
    grid-template-columns: minmax(0, 1fr);
    gap: 16px;
  }
  .chart {
    min-width: 0;
  }
  .detail {
    min-width: 0;
    padding-top: 16px;
    border-top: 1px solid var(--theme-border);
  }
  @container (min-width: 560px) {
    .layout {
      grid-template-columns: minmax(0, 1.2fr) minmax(0, 1fr);
      gap: 24px;
    }
    .detail {
      padding: 0 0 0 24px;
      border-top: 0;
      border-left: 1px solid var(--theme-border);
    }
  }
  svg {
    display: block;
    overflow: visible;
  }
  .grid {
    stroke: var(--theme-border);
    stroke-width: 1;
  }
  .grid.zero {
    stroke: var(--theme-border-strong);
  }
  .tick {
    font-size: 11px;
    text-anchor: end;
    fill: var(--theme-muted);
    font-variant-numeric: tabular-nums;
  }
  .bar {
    cursor: pointer;
    outline: none;
  }
  .hit {
    fill: transparent;
  }
  .bar:hover .hit,
  .bar:focus-visible .hit {
    fill: var(--theme-hover);
    fill-opacity: 0.25;
  }
  .bar.active .hit {
    fill: var(--theme-hover);
    fill-opacity: 0.45;
  }
  .income .fill {
    fill: var(--bar-in);
  }
  .expenses .fill {
    fill: var(--bar-out);
  }
  .result .fill {
    fill: var(--theme-accent);
  }
  .result.loss .fill {
    fill: var(--theme-error-text);
  }
  .vat-ground {
    fill: var(--bar-out);
  }
  .vat-line {
    stroke: var(--theme-warning-border);
    stroke-width: 2;
  }
  .value {
    font-size: 13px;
    font-weight: 600;
    text-anchor: middle;
    fill: var(--theme-text);
    font-variant-numeric: tabular-nums;
  }
  .result.loss .value {
    fill: var(--theme-error-text);
  }
  .name {
    font-size: 12px;
    text-anchor: middle;
    fill: var(--theme-muted);
  }
  .bar.active .name {
    fill: var(--theme-text);
    font-weight: 600;
  }
  .bar-notes {
    margin-top: 8px;
  }
  .bar-notes p {
    margin: 0;
    font-size: 12px;
    line-height: 1.5;
    color: var(--theme-muted);
  }
  .bar-notes .incomplete {
    color: var(--theme-warning-text);
  }
  .swatch {
    display: inline-block;
    width: 10px;
    height: 10px;
    margin-right: 6px;
    border-radius: 2px;
    vertical-align: -1px;
    background: repeating-linear-gradient(
      45deg,
      var(--theme-warning-border) 0 2px,
      var(--bar-out) 2px 5px
    );
  }
</style>
