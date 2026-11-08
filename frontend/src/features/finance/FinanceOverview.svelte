<script lang="ts">
  import { untrack, onMount, tick } from "svelte";
  import * as API from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/app/invoiceservice";
  import type {
    FinanceReport,
    FinanceYear,
  } from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/app/models";
  import type {
    AccountBalance,
    FinanceSettings,
  } from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/store/models";
  import FigureCard from "./FigureCard.svelte";
  import CoverageBar from "./CoverageBar.svelte";
  import OpenTasks from "./OpenTasks.svelte";
  import ResultBars from "./ResultBars.svelte";
  import YearBars from "./YearBars.svelte";
  import PeriodPicker from "./PeriodPicker.svelte";
  import PlusHint from "../../components/PlusHint.svelte";
  import SettingSection from "../../components/SettingSection.svelte";
  import { dateLabel, localToday } from "../../lib/format";
  import { serviceErrorMessage } from "../../lib/errors";
  import { periodLabel, periodRange, type PeriodKind } from "../../lib/financePeriod";
  import { cached, remember } from "../../lib/cache";

  // The overview of Konto & Finanzen. Three reports: the chosen period for
  // income and expenses; January 1 to today for everything valued today
  // (balance, open items, tax pot); and the last years for the open points, so
  // the list of what is left to do does not change with the period. Below the
  // period, the months of a year of its own choice.
  let { onassign }: { onassign?: () => void } = $props();
  const now = new Date();
  let kind = $state<PeriodKind>("quarter");
  let year = $state(now.getFullYear());
  let index = $state(Math.floor(now.getMonth() / 3));
  const range = $derived(periodRange(kind, year, index));

  const todayDate = localToday();
  const currentKey = `finance:current:${todayDate}`;
  let current = $state<FinanceReport | null>(
    untrack(() => cached<FinanceReport>(currentKey)) ?? null,
  );
  let currentFailed = $state(false);
  const todoKey = `finance:todo:${todayDate}`;
  let todo = $state<FinanceReport | null>(untrack(() => cached<FinanceReport>(todoKey)) ?? null);
  const openIssues = $derived(todo?.issues ?? (todo ? [] : null));

  type Loaded = { report: FinanceReport; settings: FinanceSettings };
  const warm = untrack(() => cached<Loaded>(`finance:${range.from}:${range.to}`));
  let report = $state<FinanceReport | null>(warm?.report ?? null);
  let settings = $state<FinanceSettings | null>(warm?.settings ?? null);
  let balances = $state<AccountBalance[]>((warm?.settings.balances ?? []).map((b) => ({ ...b })));
  let reservePercent = $state(warm?.settings.reservePercent ?? "");
  // The year view keeps its own year; a click on a month shows it in the period above.
  let chartYear = $state(now.getFullYear());
  const chartYears = Array.from({ length: 6 }, (_, i) => now.getFullYear() - i);
  let yearData = $state<FinanceYear | null>(
    untrack(() => cached<FinanceYear>(`finance:year:${chartYear}`)) ?? null,
  );
  let yearLoading = $state(false),
    yearError = $state("");
  let yearVersion = 0;
  let loading = $state(true),
    saving = $state(false),
    error = $state(""),
    notice = $state("");
  let loadVersion = 0;
  // The first reload of cached figures stays invisible; later period changes dim them.
  let firstLoad = $state(true);

  // Accounts with bookings but without a balance are offered for entry.
  const missingAccounts = $derived(
    (openIssues ?? [])
      .find((issue) => issue.code === "missing_balance")
      ?.entries?.map((entry) => entry.id)
      .filter((id) => id && !balances.some((b) => accountKey(b.accountId) === accountKey(id))) ??
      [],
  );
  const accountKey = (id: string) => id.replaceAll(" ", "").toUpperCase();
  const balanceOutdated = $derived(
    (openIssues ?? []).some((i) => i.code === "missing_balance" || i.code === "stale_balance"),
  );
  // Balances and reserve rate show what is stored; the form opens on "Ändern" and by
  // itself once, when a balance is missing or old or no reserve rate is set.
  let moneyOpen = $state(false);
  let moneyOpenDecided = false;
  $effect(() => {
    if (moneyOpenDecided || !settings || !todo) return;
    moneyOpenDecided = true;
    moneyOpen = balanceOutdated || !settings.reservePercent;
  });
  const moneySummary = $derived.by(() => {
    if (!settings) return "";
    const stored = settings.balances ?? [];
    const latest = stored.reduce((date, b) => (b.date > date ? b.date : date), "");
    const accounts = stored.length
      ? `${stored.length} ${stored.length === 1 ? "Kontostand" : "Kontostände"} von Hand, Stand ${dateLabel(latest)}${balanceOutdated ? " (nicht aktuell)" : ""}`
      : "Kein Kontostand von Hand eingetragen";
    const reserve = settings.reservePercent
      ? `Rücklage ${settings.reservePercent.replace(".", ",")} %`
      : "Rücklagensatz nicht festgelegt";
    return `${accounts} · ${reserve}`;
  });

  async function loadCurrent() {
    try {
      current = remember(
        currentKey,
        await API.FinanceReport(`${todayDate.slice(0, 4)}-01-01`, todayDate),
      );
      currentFailed = false;
    } catch {
      currentFailed = !current;
    }
  }

  async function loadTodo() {
    try {
      todo = remember(
        todoKey,
        await API.FinanceReport(`${Number(todayDate.slice(0, 4)) - 5}-01-01`, todayDate),
      );
    } catch {
      // The page still works; the list shows again with the next load.
    }
  }

  async function loadYear() {
    const version = ++yearVersion;
    const key = `finance:year:${chartYear}`;
    yearData = cached<FinanceYear>(key) ?? yearData;
    yearLoading = true;
    yearError = "";
    try {
      const next = await API.FinanceYear(chartYear);
      if (version === yearVersion) yearData = remember(key, next);
    } catch (e) {
      if (version === yearVersion)
        yearError = serviceErrorMessage(e, "Der Jahresverlauf konnte nicht geladen werden.");
    } finally {
      if (version === yearVersion) yearLoading = false;
    }
  }

  async function showMonth(month: number) {
    kind = "month";
    year = chartYear;
    index = month;
    await tick();
    document
      .getElementById("result-title")
      ?.closest("section")
      ?.scrollIntoView({ behavior: "smooth", block: "start" });
  }

  async function load() {
    const version = ++loadVersion;
    loading = true;
    error = "";
    try {
      const [nextReport, nextSettings] = await Promise.all([
        API.FinanceReport(range.from, range.to),
        API.FinanceSettings(),
      ]);
      if (version !== loadVersion) return;
      report = nextReport;
      settings = nextSettings;
      remember(`finance:${range.from}:${range.to}`, { report: nextReport, settings: nextSettings });
      balances = (nextSettings.balances ?? []).map((b) => ({ ...b }));
      reservePercent = nextSettings.reservePercent;
    } catch (e) {
      if (version === loadVersion)
        error = serviceErrorMessage(e, "Die Finanzübersicht konnte nicht geladen werden.");
    } finally {
      if (version === loadVersion) {
        loading = false;
        firstLoad = false;
      }
    }
  }

  async function save(patch: Partial<FinanceSettings>, message: string) {
    if (!settings) return;
    saving = true;
    error = "";
    notice = "";
    try {
      settings = await API.SaveFinanceSettings({
        ...settings,
        balances,
        reservePercent,
        ...patch,
      });
      notice = message;
      await Promise.all([load(), loadCurrent(), loadTodo()]);
    } catch (e) {
      error = serviceErrorMessage(e, "Die Einstellungen konnten nicht gespeichert werden.");
    } finally {
      saving = false;
    }
  }

  function addBalance(accountId = "") {
    balances = [...balances, { accountId, amount: "", date: localToday() }];
  }

  function removeBalance(i: number) {
    balances = balances.filter((_, j) => j !== i);
  }

  $effect(() => {
    range.from;
    range.to;
    void load();
  });
  $effect(() => {
    chartYear;
    void loadYear();
  });
  onMount(() => {
    void loadCurrent();
    void loadTodo();
    return () => {
      loadVersion = -1;
      yearVersion = -1;
    };
  });

  async function showMoney() {
    moneyOpen = true;
    await tick();
    document
      .getElementById("money-settings")
      ?.scrollIntoView({ behavior: "smooth", block: "start" });
  }
</script>

{#if error}<div class="alert error" role="alert">{error}</div>{/if}
{#if notice}<div class="alert success" role="status">{notice}</div>{/if}

<div class="finance" class:refreshing={loading && !firstLoad}>
  <OpenTasks issues={openIssues} loading={!todo} {onassign} onsettings={() => void showMoney()} />
  <CoverageBar report={current} failed={currentFailed} onaddbalance={() => void showMoney()} />
  {#if !report}
    <div class="loading">{loading ? "Finanzübersicht wird berechnet …" : ""}</div>
  {:else}
    <section class="block finance-panel" aria-labelledby="result-title">
      <header class="block-head">
        <h2 id="result-title">Einnahmen & Ausgaben · {periodLabel(kind, year, index)}</h2>
        <PeriodPicker bind:kind bind:year bind:index />
      </header>
      <p class="hint">
        Was im gewählten Zeitraum geflossen ist. Den steuerlichen Gewinn mit Abschreibungen und
        10-Tage-Regel zeigt die EÜR unter „Steuern“.
      </p>
      <ResultBars income={report.income} expenses={report.expenses} result={report.result} />
      <div class="detail-list payments-detail">
        <details>
          <summary
            ><span class="detail-title">Zahlungen im Einzelnen</span><span class="detail-state"
              >Zuflüsse, Abflüsse, Sonstiges</span
            ></summary
          >
          <div class="grid">
            <FigureCard figure={report.inflows} />
            <FigureCard figure={report.outflows} />
            <FigureCard figure={report.other} />
          </div>
        </details>
      </div>
    </section>

    <section
      class="block finance-panel"
      class:year-refreshing={yearLoading && yearData?.year !== chartYear}
      aria-labelledby="year-title"
    >
      <header class="block-head">
        <h2 id="year-title">Jahresverlauf · {chartYear}</h2>
        <select class="year-select" bind:value={chartYear} aria-label="Jahr">
          {#each chartYears as y}<option value={y}>{y}</option>{/each}
        </select>
      </header>
      <p class="hint">
        Einnahmen nach oben, Ausgaben nach unten, je Monat nach Zahlungsdatum. Ein Klick auf einen
        Monat zeigt ihn oben im Einzelnen.
      </p>
      {#if yearError}<p class="hint year-error" role="alert">{yearError}</p>{/if}
      {#if yearData}<YearBars
          data={yearData}
          chosen={kind === "month" && year === yearData.year ? index : -1}
          onselect={(month) => void showMonth(month)}
        />{:else if !yearError}<div class="loading">Jahresverlauf wird berechnet …</div>{/if}
    </section>

    <section
      class="block finance-panel settings-panel"
      id="money-settings"
      aria-label="Grundlagen der Übersicht"
    >
      <SettingSection
        title="Kontostände und Rücklagensatz"
        summary={moneySummary}
        bind:open={moneyOpen}
      >
        <form
          class="money-form"
          onsubmit={(event) => {
            event.preventDefault();
            void save({}, "Kontostände und Rücklagensatz gespeichert.");
          }}
        >
          <fieldset>
            <legend>Kontostände</legend>
            <p class="hint">
              Beim Bankabruf per FinTS übernimmt papierleicht den Kontostand automatisch aus den
              Umsatzdaten. Bei CSV-Konten trägst du den Kontostand einmal mit Datum ein; später
              importierte Umsätze rechnet papierleicht dazu. Es gilt jeweils der neuere Stand. Jede
              Änderung speichert papierleicht mit Zeitstempel.
            </p>
            {#if balanceOutdated}<PlusHint
                id="finance-balance"
                topic="fints"
                text="Mit Plus übernimmt papierleicht den Kontostand bei jedem Bankabruf direkt von deiner Bank."
              />{/if}
            {#each balances as balance, i}
              <div class="balance-row">
                <label
                  >Konto<input
                    bind:value={balance.accountId}
                    required
                    maxlength="100"
                    placeholder="IBAN oder Name"
                  /></label
                >
                <label
                  >Kontostand in €<input
                    bind:value={balance.amount}
                    required
                    inputmode="decimal"
                    placeholder="0,00"
                  /></label
                >
                <label>Stand vom<input type="date" bind:value={balance.date} required /></label>
                <button type="button" class="text-button" onclick={() => removeBalance(i)}
                  >Entfernen</button
                >
              </div>
            {/each}
            <div class="balance-actions">
              {#each missingAccounts as account}
                <button type="button" class="secondary" onclick={() => addBalance(account)}
                  >Kontostand für {account}</button
                >
              {/each}
              <button type="button" class="secondary" onclick={() => addBalance()}
                >Weiteres Konto</button
              >
            </div>
          </fieldset>
          <fieldset>
            <legend>Rücklage Einkommensteuer</legend>
            <label class="percent"
              >Anteil vom Ergebnis in %<input
                bind:value={reservePercent}
                inputmode="decimal"
                placeholder="z. B. 30"
              /></label
            >
            <p class="hint">
              Ein fester Satz, den du selbst festlegst. papierleicht berechnet keine
              Einkommensteuer, weil dafür ein geprüftes Modell und deine persönlichen Angaben
              fehlen.
            </p>
          </fieldset>
          <button class="primary" disabled={saving}>Speichern</button>
        </form>
      </SettingSection>
    </section>

    <details class="chevron-toggle not-covered">
      <summary>Was diese Übersicht nicht enthält</summary>
      <ul>
        {#each report.notCovered ?? [] as item}<li>{item}</li>{/each}
      </ul>
      <p class="hint">
        Diese Übersicht bereitet deine Unterlagen vor. Sie ersetzt weder eine Steuererklärung noch
        die Beratung durch deine Steuerkanzlei.
      </p>
    </details>
  {/if}
</div>

<style>
  /* As tall as the quarter and year selects next to it. */
  .alert {
    padding: 10px 14px;
    border-radius: 8px;
    margin-bottom: 16px;
    font-size: 13px;
  }
  .alert.error {
    background: var(--theme-error-bg);
    color: var(--theme-error-text);
  }
  .alert.success {
    background: var(--theme-success-bg);
    color: var(--theme-success-text);
  }
  .finance {
    display: flex;
    flex-direction: column;
    gap: var(--ui-section-gap);
    padding-bottom: 32px;
    transition: opacity 0.15s;
  }
  .finance.refreshing {
    opacity: 0.6;
  }
  /* Framed like "Reicht das Geld?" above, so the two answers read as a pair.
     Not .panel or .settings: the app styles both globally (shadow, padding). */
  .finance-panel {
    padding: var(--ui-card-padding);
    background: var(--theme-surface);
    border: 1px solid var(--theme-border);
    border-radius: 12px;
    min-width: 0;
    /* Jumps to a section stop below the sticky page head (52px). */
    scroll-margin-top: 68px;
  }
  .year-refreshing {
    opacity: 0.6;
    transition: opacity 0.15s;
  }
  .year-select {
    margin-left: auto;
  }
  .year-error {
    color: var(--theme-error-text);
  }
  .payments-detail {
    margin-top: 16px;
  }
  .block-head {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    justify-content: space-between;
    gap: 8px 16px;
    margin-bottom: 6px;
  }
  .block-head h2 {
    margin: 0;
  }
  .block h2 {
    margin: 0 0 6px;
    color: var(--theme-text);
  }
  .grid {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(min(220px, 100%), 1fr));
    gap: 12px;
    align-items: start;
  }
  .hint {
    margin: 0 0 12px;
    font-size: 12px;
    color: var(--theme-muted);
    line-height: 1.5;
  }
  .money-form {
    margin-top: 16px;
    display: flex;
    flex-direction: column;
    gap: 16px;
    align-items: flex-start;
  }
  fieldset {
    border: 1px solid var(--theme-border);
    border-radius: 12px;
    padding: 14px 16px;
    margin: 0;
    width: 100%;
    box-sizing: border-box;
  }
  /* Below the section's own title, so one step smaller than a section. */
  legend {
    font-size: var(--ui-subsection-title-size);
    font-weight: 600;
    padding: 0 4px;
  }
  label {
    display: flex;
    flex-direction: column;
    gap: 4px;
    font-size: 12px;
    color: var(--theme-muted);
  }
  .balance-row {
    display: grid;
    grid-template-columns: minmax(0, 2fr) minmax(0, 1fr) minmax(0, 1fr) auto;
    gap: 10px;
    align-items: end;
    margin-bottom: 10px;
  }
  .balance-actions {
    display: flex;
    flex-wrap: wrap;
    gap: 8px;
  }
  /* Account names come from the bank import and can be long. */
  .balance-actions button {
    max-width: 100%;
    overflow-wrap: anywhere;
  }
  .percent {
    max-width: 200px;
    margin-bottom: 8px;
  }
  /* The section's own top line and gap are not needed inside the frame. */
  .settings-panel :global(.setting-section) {
    margin-top: 0;
    padding-top: 0;
    border-top: 0;
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
    color: var(--theme-text);
  }
  .text-button {
    border: 0;
    padding: 4px 0;
    background: transparent;
    color: var(--theme-accent);
    font-size: 12px;
    cursor: pointer;
  }
  .text-button:hover {
    text-decoration: underline;
  }
  /* The sidebar takes ~210px, so a 900px window leaves little room. */
  @media (max-width: 1100px) {
    .balance-row {
      grid-template-columns: minmax(0, 1fr) minmax(0, 1fr) auto;
    }
    .balance-row label:first-child {
      grid-column: 1 / -1;
    }
  }
</style>
