<script lang="ts">
  import { untrack } from "svelte";
  import * as API from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/app/invoiceservice";
  import type { TaxOverview } from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/app/models";
  import ChevronRightIcon from "@lucide/svelte/icons/chevron-right";
  import { cached, remember } from "../../lib/cache";
  import { serviceErrorMessage } from "../../lib/errors";
  import { dateLabel, localToday } from "../../lib/format";
  import { stateLabels, taxTasks, type TaxTaskGroups } from "../../lib/taxTasks";
  import AnnualTask from "./AnnualTask.svelte";
  import SmallBusinessCard from "./SmallBusinessCard.svelte";
  import TaxProfile from "./TaxProfile.svelte";
  import TaxYearOverview from "./TaxYearOverview.svelte";
  import VATTask from "./VATTask.svelte";
  import ZMTask from "./ZMTask.svelte";

  let {
    initialTask = "",
    year = $bindable(new Date().getFullYear()),
    onchanged = async () => {},
  }: {
    /** A task to open, e.g. "zm:2026-Q3"; a bare period like "2026-Q3" is a Voranmeldung. */
    initialTask?: string;
    /** The year shown; the running year lists what is open now, others list everything. */
    year?: number;
    /** Runs after a filing note changed, so tiles and figures around follow. */
    onchanged?: () => Promise<void>;
  } = $props();
  const today = localToday();
  const thisYear = new Date().getFullYear();
  const years = [thisYear, thisYear - 1, thisYear - 2];
  const current = $derived(year === thisYear);
  // The tasks of the last visit show right away while they are worked out again.
  let overview = $state<TaxOverview | null>(
    cached<TaxOverview>(`tax:overview:${thisYear}`) ?? null,
  );
  let error = $state("");
  let selected = $state(
    untrack(() => (initialTask && !initialTask.includes(":") ? `vat:${initialTask}` : initialTask)),
  );
  const groups = $derived<TaxTaskGroups | null>(
    overview
      ? taxTasks(
          {
            year,
            vat: overview.vat ?? [],
            zm: overview.zm ?? [],
            annual: overview.annual ?? [],
            since: overview.since,
          },
          today,
          !current,
        )
      : null,
  );
  const task = $derived.by(() => {
    if (!selected) return null;
    const [kind, periodId] = selected.split(":");
    return { kind, periodId };
  });
  // The next Voranmeldung to file, for the overview.
  const nextVAT = $derived(
    current
      ? ([...(groups?.now ?? []), ...(groups?.soon ?? [])].find((t) => t.kind === "vat") ?? null)
      : null,
  );
  const sections = [
    ["now", "Jetzt erledigen"],
    ["soon", "Demnächst"],
    ["done", "Erledigt"],
  ] as const;

  async function load() {
    error = "";
    const loading = year;
    try {
      overview = remember(`tax:overview:${loading}`, await API.TaxOverview(loading));
    } catch (cause) {
      error = serviceErrorMessage(cause, "Die Steuer-Aufgaben konnten nicht geladen werden.");
    }
  }
  $effect(() => {
    void year;
    void load();
  });
  async function changed() {
    await Promise.all([load(), onchanged()]);
  }
</script>

<div class="tax-page">
  {#if error}<div class="alert error" role="alert">{error}</div>{/if}
  {#if overview}
    <TaxProfile setup={overview.setup} onsaved={load} />
    {#if task}
      <button class="text back" onclick={() => (selected = "")}>← Alle Steuern</button>
      {#if task.kind === "zm"}
        <ZMTask periodId={task.periodId} setup={overview.setup} onchanged={changed} />
      {:else if task.kind === "annual"}
        <AnnualTask year={Number(task.periodId)} setup={overview.setup} onchanged={changed} />
      {:else}
        <VATTask periodId={task.periodId} setup={overview.setup} onchanged={changed} />
      {/if}
    {:else}
      {#if overview.smallBusiness}<SmallBusinessCard status={overview.smallBusiness} />{/if}
      <TaxYearOverview
        {overview}
        bind:year
        {years}
        {current}
        {nextVAT}
        onopen={(id) => (selected = id)}
      />
      <section class="tax-card" aria-labelledby="tax-tasks-title">
        <h2 id="tax-tasks-title">
          {current ? "Meldungen und Erklärungen" : `Meldungen und Erklärungen ${year}`}
        </h2>
        <p class="tax-muted">
          {current
            ? "Was jetzt ansteht und was du schon übermittelt hast."
            : `Alle Zeiträume aus ${year}.`} Jede Aufgabe in drei Schritten: Vollständig? Ergebnis. Übertragen.
        </p>
        {#if groups}
          {#each sections as [key, label] (key)}
            {#if groups[key].length}
              <h3>{label}</h3>
              <ul class="tasks">
                {#each groups[key] as t (t.id)}
                  <li>
                    <button class={`task ${t.state}`} onclick={() => (selected = t.id)}>
                      <span class="what">{t.title}</span>
                      <span class="when"
                        >{stateLabels[t.state]} · {t.filedOn
                          ? `übermittelt am ${dateLabel(t.filedOn)}`
                          : `fällig am ${dateLabel(t.due)}`}</span
                      >
                      <ChevronRightIcon size={16} aria-hidden="true" />
                    </button>
                  </li>
                {/each}
              </ul>
            {/if}
          {/each}
          {#if !groups.now.length && !groups.soon.length && !groups.done.length}
            <p class="tax-muted">
              Hier steht gerade nichts an. Sobald ein Zeitraum endet, erscheint seine Meldung mit
              Frist.
            </p>
          {/if}
          {#if overview.since && overview.since < today}
            <p class="tax-muted since">
              Die Liste beginnt mit deinem ersten Eintrag in der App vom {dateLabel(
                overview.since,
              )}. Zeiträume davor kennt die App nicht.
            </p>
          {/if}
        {/if}
      </section>
      <details class="chevron-toggle not-covered">
        <summary>Was diese Seite nicht enthält</summary>
        <ul>
          <li>
            Einkommen- und Gewerbesteuer selbst: Dafür fehlen der App deine persönlichen Angaben und
            der Hebesatz deiner Gemeinde. Die Beträge stehen in deinen Bescheiden.
          </li>
          <li>Lohnsteuer für Angestellte: Die Anmeldung übernimmt dein Lohnprogramm.</li>
          <li>
            Kfz-, Grund- und Grunderwerbsteuer: Sie kommen per Bescheid; betriebliche Anteile
            erfasst du als Beleg.
          </li>
        </ul>
        <p class="tax-muted">
          Die Seite bereitet deine Unterlagen vor. Sie ersetzt keine Steuerberatung.
        </p>
      </details>
    {/if}
  {:else if !error}
    <p class="tax-muted">Steuern werden berechnet …</p>
  {/if}
</div>

<style>
  .tax-page {
    display: grid;
    gap: var(--ui-section-gap);
    padding-bottom: 32px;
    font-size: 13px;
  }
  .back {
    justify-self: start;
  }
  h3 {
    margin: 16px 0 6px;
  }
  .tasks {
    display: grid;
    gap: 6px;
    margin: 0;
    padding: 0;
    list-style: none;
  }
  .task {
    display: grid;
    grid-template-columns: minmax(0, 1fr) auto 16px;
    align-items: center;
    gap: 12px;
    width: 100%;
    padding: 10px 12px;
    border: 1px solid var(--theme-border);
    border-radius: 8px;
    background: var(--theme-surface-muted);
    color: var(--theme-text);
    font: inherit;
    text-align: left;
    cursor: pointer;
  }
  .task:hover {
    border-color: var(--theme-accent);
  }
  .task .what {
    font-weight: 600;
    overflow-wrap: anywhere;
  }
  .task .when {
    color: var(--theme-muted);
  }
  .task.overdue .when,
  .task.due .when,
  .task.changed .when {
    color: var(--theme-warning-text);
    font-weight: 600;
  }
  .task.done {
    background: var(--theme-surface);
  }
  .since {
    margin: 14px 0 0;
  }
  .not-covered {
    padding: 0 2px;
  }
  .not-covered > summary {
    font-size: 12px;
    color: var(--theme-muted);
  }
  .not-covered ul {
    margin: 8px 0 10px;
    padding-left: 18px;
    font-size: 13px;
    line-height: 1.5;
  }
  @media (max-width: 760px) {
    .task {
      grid-template-columns: minmax(0, 1fr) 16px;
    }
    .task .when {
      grid-row: 2;
    }
  }
</style>
