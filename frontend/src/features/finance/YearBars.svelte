<script lang="ts">
  import type { FinanceYear } from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/app/models";
  import { amountLabel } from "../../lib/format";

  // Income and expenses of every month on one zero line: income points up,
  // expenses point down, so a month's height above and below compares at a
  // glance. As in the bars of the chosen period, the Umsatzsteuer paid to the
  // tax office is hatched inside the expenses, here at their lower end.
  let {
    data,
    chosen = -1,
    onselect,
  }: { data: FinanceYear; chosen?: number; onselect?: (month: number) => void } = $props();

  const pattern = $props.id();
  const names = [
    "Jan",
    "Feb",
    "Mär",
    "Apr",
    "Mai",
    "Jun",
    "Jul",
    "Aug",
    "Sep",
    "Okt",
    "Nov",
    "Dez",
  ];
  const longNames = Array.from({ length: 12 }, (_, i) =>
    new Date(2026, i, 1).toLocaleDateString("de-DE", { month: "long" }),
  );
  let hovered = $state<number | null>(null);

  const num = (value: string) => Number(value) || 0;
  const months = $derived(
    (data.months ?? []).map((m) => ({
      ...m,
      inValue: num(m.income),
      outValue: Math.abs(num(m.expenses)),
      vatValue: Math.abs(num(m.vat)),
      resultValue: num(m.result),
    })),
  );
  const totals = $derived(
    months.reduce(
      (sum, m) => ({
        inValue: sum.inValue + m.inValue,
        outValue: sum.outValue + m.outValue,
        resultValue: sum.resultValue + m.resultValue,
      }),
      { inValue: 0, outValue: 0, resultValue: 0 },
    ),
  );

  // Both halves share one step, so 1.000 € above is as tall as 1.000 € below.
  // An empty year still gets a readable axis of 100 € each way.
  const scale = $derived.by(() => {
    const maxIn = Math.max(0, ...months.map((m) => m.inValue));
    const maxOut = Math.max(0, ...months.map((m) => m.outValue));
    const step = niceStep(Math.max(maxIn + maxOut, 400) / 4);
    return {
      step,
      top: Math.max(step, Math.ceil(maxIn / step) * step),
      bottom: -Math.max(step, Math.ceil(maxOut / step) * step),
    };
  });
  // Whole euros only, so the axis never repeats a rounded label.
  function niceStep(raw: number): number {
    const power = 10 ** Math.floor(Math.log10(Math.max(raw, 1)));
    return ([1, 2, 5, 10].find((k) => k * power >= raw) ?? 10) * power;
  }
  const ticks = $derived.by(() => {
    const list: number[] = [];
    for (let v = scale.bottom; v <= scale.top + 1e-6; v += scale.step) list.push(v);
    return list;
  });

  // The drawing follows the width it gets, so labels keep their size.
  let width = $state(600);
  const plotTop = 12,
    plotBottom = 212,
    height = 240,
    left = 52;
  const slot = $derived((width - left) / 12);
  const barWidth = $derived(Math.max(6, Math.min(28, slot * 0.56)));
  const y = (v: number) =>
    plotTop + ((scale.top - v) / (scale.top - scale.bottom)) * (plotBottom - plotTop);
  const centre = (i: number) => left + slot * i + slot / 2;
  const short = (v: number) =>
    (v < 0 ? "−" : "") + Math.abs(v).toLocaleString("de-DE", { maximumFractionDigits: 0 });
  // A 1px gap on each side of the zero line keeps the two bars of a month apart.
  const gap = 1;

  const signed = (v: number) => (v > 0 ? "+" : "") + amountLabel(v);
  const shown = $derived(hovered === null ? null : months[hovered]);
  const incompleteMonths = $derived(
    months.filter((m) => m.incomplete).map((m) => longNames[m.month - 1]),
  );
  const anyVAT = $derived(months.some((m) => m.vatValue > 0));
</script>

<div class="year-bars">
  <div class="readout" aria-live="polite">
    <span class="readout-title"
      >{shown ? `${longNames[shown.month - 1]} ${data.year}` : `Summe ${data.year}`}</span
    >
    <span class="readout-item"
      ><span class="key in" aria-hidden="true"></span>Einnahmen
      <strong>{amountLabel(shown ? shown.inValue : totals.inValue)}</strong></span
    >
    <span class="readout-item"
      ><span class="key out" aria-hidden="true"></span>Ausgaben
      <strong>{amountLabel(shown ? shown.outValue : totals.outValue)}</strong></span
    >
    <span class="readout-item"
      >Ergebnis <strong class:loss={(shown ? shown.resultValue : totals.resultValue) < 0}
        >{signed(shown ? shown.resultValue : totals.resultValue)}</strong
      ></span
    >
  </div>
  <div class="chart" bind:clientWidth={width}>
    <svg
      width="100%"
      {height}
      viewBox="0 0 {width} {height}"
      role="group"
      aria-label="Einnahmen und Ausgaben je Monat {data.year}"
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
        <line class="grid" class:zero={tick === 0} x1={left} x2={width} y1={y(tick)} y2={y(tick)} />
        <text class="tick" x={left - 8} y={y(tick) + 4}>{short(tick)}</text>
      {/each}
      {#each months as m, i (m.month)}
        {@const x = centre(i) - barWidth / 2}
        {@const inHeight = Math.max(y(0) - y(m.inValue) - gap, m.inValue ? 2 : 0)}
        {@const outHeight = Math.max(y(-m.outValue) - y(0) - gap, m.outValue ? 2 : 0)}
        {@const vatHeight = Math.min(outHeight, Math.max(y(-m.vatValue) - y(0), 1))}
        <g
          class="month"
          class:future={m.future}
          class:active={chosen === i}
          role="button"
          tabindex={m.future ? -1 : 0}
          aria-disabled={m.future}
          aria-label="{longNames[i]} {data.year}: Einnahmen {amountLabel(
            m.inValue,
          )}, Ausgaben {amountLabel(m.outValue)}, Ergebnis {signed(m.resultValue)}"
          onclick={() => !m.future && onselect?.(i)}
          onmouseenter={() => (hovered = m.future ? null : i)}
          onmouseleave={() => (hovered = null)}
          onfocus={() => (hovered = m.future ? null : i)}
          onblur={() => (hovered = null)}
          onkeydown={(event) => {
            if (!m.future && (event.key === "Enter" || event.key === " ")) {
              event.preventDefault();
              onselect?.(i);
            }
          }}
        >
          <rect class="hit" x={left + slot * i + 2} y="0" width={slot - 4} {height} rx="6" />
          {#if inHeight}<rect
              class="fill in"
              {x}
              y={y(0) - gap - inHeight}
              width={barWidth}
              height={inHeight}
              rx="3"
            />{/if}
          {#if outHeight}<rect
              class="fill out"
              {x}
              y={y(0) + gap}
              width={barWidth}
              height={outHeight}
              rx="3"
            />{/if}
          {#if m.vatValue > 0 && outHeight}<rect
              {x}
              y={y(0) + gap + outHeight - vatHeight}
              width={barWidth}
              height={vatHeight}
              rx="3"
              fill="url(#{pattern}-vat)"
            />{/if}
          <text class="name" x={centre(i)} y={height - 8}
            >{names[i]}{#if m.incomplete}<tspan class="flag">*</tspan>{/if}</text
          >
        </g>
      {/each}
    </svg>
  </div>
  {#if anyVAT || incompleteMonths.length}
    <div class="bar-notes">
      {#if anyVAT}
        <p>
          <span class="swatch" aria-hidden="true"></span>Schraffiert: Umsatzsteuer ans Finanzamt,
          oft für den Zeitraum davor.
        </p>
      {/if}
      {#if incompleteMonths.length}
        <p class="incomplete">
          * Unvollständig ({incompleteMonths.join(", ")}): Es fehlen Daten, siehe offene Punkte
          oben.
        </p>
      {/if}
    </div>
  {/if}
</div>

<style>
  .year-bars {
    --bar-in: color-mix(in srgb, var(--theme-accent) 60%, var(--theme-surface));
    --bar-out: color-mix(in srgb, var(--theme-error-text) 40%, var(--theme-surface));
    min-width: 0;
  }
  .readout {
    display: flex;
    flex-wrap: wrap;
    align-items: baseline;
    gap: 4px 16px;
    margin-bottom: 12px;
    font-size: 13px;
    color: var(--theme-muted);
    font-variant-numeric: tabular-nums;
  }
  .readout-title {
    min-width: 9em;
    font-weight: 600;
    color: var(--theme-text);
  }
  .readout strong {
    font-weight: 600;
    color: var(--theme-text);
  }
  .readout strong.loss {
    color: var(--theme-error-text);
  }
  .key {
    display: inline-block;
    width: 10px;
    height: 10px;
    margin-right: 6px;
    border-radius: 2px;
    vertical-align: -1px;
  }
  .key.in {
    background: var(--bar-in);
  }
  .key.out {
    background: var(--bar-out);
  }
  .chart {
    min-width: 0;
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
  .month {
    cursor: pointer;
    outline: none;
  }
  .month.future {
    cursor: default;
  }
  .hit {
    fill: transparent;
  }
  .month:not(.future):hover .hit,
  .month:focus-visible .hit {
    fill: var(--theme-hover);
    fill-opacity: 0.25;
  }
  .month.active .hit {
    fill: var(--theme-hover);
    fill-opacity: 0.45;
  }
  .fill.in {
    fill: var(--bar-in);
  }
  .fill.out {
    fill: var(--bar-out);
  }
  .vat-ground {
    fill: var(--bar-out);
  }
  .vat-line {
    stroke: var(--theme-warning-border);
    stroke-width: 2;
  }
  .name {
    font-size: 12px;
    text-anchor: middle;
    fill: var(--theme-muted);
  }
  .month.future .name {
    opacity: 0.5;
  }
  .month.active .name {
    fill: var(--theme-text);
    font-weight: 600;
  }
  .flag {
    fill: var(--theme-warning-text);
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
