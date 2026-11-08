<script lang="ts">
  import * as API from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/app/invoiceservice";
  import type {
    EURView,
    TaxSetup,
    VATReturnView,
  } from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/app/models";
  import { serviceErrorMessage } from "../../lib/errors";
  import { amountLabel, dateLabel } from "../../lib/format";
  import { yearsLabel } from "../../lib/expenses";
  import { openElster } from "../../lib/elster";
  import DatevExportDialog from "./DatevExportDialog.svelte";
  import TaxFieldTable from "./TaxFieldTable.svelte";
  import TaxIssueList from "./TaxIssueList.svelte";
  import FilingMark from "./FilingMark.svelte";
  import TaxHelpNote from "./TaxHelpNote.svelte";

  let {
    year,
    setup,
    onchanged,
  }: { year: number; setup: TaxSetup; onchanged: () => Promise<void> } = $props();
  let eur = $state<EURView | null>(null);
  let vatYear = $state<VATReturnView | null>(null);
  let error = $state("");
  let datevOpen = $state(false);
  // Answers to the questions only steer which hints show; they stay on this device.
  let answers = $state<Record<string, "yes" | "no">>({});
  const advisor = $derived(setup.filing === "advisor");
  const trade = $derived(setup.activity === "trade");
  // Gewerbe goes into the Anlage G, freelance work into the Anlage S.
  const annex = $derived(
    setup.activity === "trade" ? "Anlage G" : setup.activity === "freelance" ? "Anlage S" : "",
  );
  const returns = $derived(
    [
      `Einkommensteuer mit Anlage EÜR${annex ? ` und ${annex}` : ""}`,
      setup.smallBusiness ? "" : "Umsatzsteuer-Jahreserklärung",
      trade
        ? "Gewerbesteuererklärung bei mehr als 24.500 € Gewerbeertrag"
        : setup.activity
          ? ""
          : "bei einem Gewerbe Gewerbesteuererklärung",
    ]
      .filter(Boolean)
      .join(", "),
  );
  const answered = $derived(Object.keys(answers).length);

  const answersKey = (y: number) => `tax-eur-questions:${y}`;
  function readAnswers(y: number) {
    try {
      return JSON.parse(localStorage.getItem(answersKey(y)) ?? "{}");
    } catch {
      return {};
    }
  }
  function answer(id: string, value: "yes" | "no") {
    answers = { ...answers, [id]: value };
    try {
      localStorage.setItem(answersKey(year), JSON.stringify(answers));
    } catch {
      // Without storage the answers last for this view.
    }
  }
  async function load(y: number) {
    error = "";
    answers = readAnswers(y);
    try {
      eur = await API.EUR(y);
      vatYear = setup.smallBusiness ? null : await API.VATYear(y);
    } catch (cause) {
      error = serviceErrorMessage(cause, "Die EÜR konnte nicht berechnet werden.");
    }
  }
  // The deadline depends on who files, so a changed profile reloads the view.
  $effect(() => {
    void setup.filing;
    void load(year);
  });
  async function mark(submittedOn: string) {
    if (!eur) return;
    error = "";
    try {
      if (submittedOn) await API.MarkAnnualFiled(year, submittedOn, eur.eur.profit);
      else await API.UnmarkAnnualFiled(year);
      await load(year);
      await onchanged();
    } catch (cause) {
      error = serviceErrorMessage(cause, "Der Vermerk konnte nicht gespeichert werden.");
    }
  }
</script>

{#if error}<div class="alert error" role="alert">{error}</div>{/if}
{#if eur}
  <section class="tax-card" aria-labelledby="annual-task-title">
    <h2 id="annual-task-title">Steuererklärungen {year}</h2>
    <p class="tax-muted">
      {eur.filing
        ? `Übermittelt am ${dateLabel(eur.filing.submittedOn)}`
        : `Abgabe bis ${dateLabel(eur.deadline)}${advisor ? " mit Steuerkanzlei" : ""}`} · {returns}
    </p>
    <div class="tax-step">
      <span class="tax-step-no">1</span>
      <div>
        <h3>Vollständig?</h3>
        <TaxIssueList issues={eur.eur.issues} />
        <h3 class="questions-title">Was die App nicht wissen kann</h3>
        <p class="tax-muted">
          {answered} von {eur.questions?.length ?? 0} Fragen beantwortet. Bei „Ja“ steht da, was du ergänzt.
        </p>
        <ul class="questions">
          {#each eur.questions ?? [] as q (q.id)}
            <li>
              <span>{q.question}</span>
              <span class="choice" role="radiogroup" aria-label={q.question}>
                <button
                  class="secondary"
                  role="radio"
                  aria-checked={answers[q.id] === "yes"}
                  onclick={() => answer(q.id, "yes")}>Ja</button
                >
                <button
                  class="secondary"
                  role="radio"
                  aria-checked={answers[q.id] === "no"}
                  onclick={() => answer(q.id, "no")}>Nein</button
                >
              </span>
              {#if answers[q.id] === "yes"}<p class="answer">{q.answer}</p>{/if}
            </li>
          {/each}
        </ul>
      </div>
    </div>
    <div class="tax-step">
      <span class="tax-step-no">2</span>
      <div>
        <h3>Ergebnis</h3>
        <div class="tax-result">
          <span>Gewinn {year} laut EÜR</span>
          <strong>{amountLabel(eur.eur.profit)}</strong>
          <small
            >Einnahmen {amountLabel(eur.eur.incomeTotal)} · Ausgaben {amountLabel(
              eur.eur.expenseTotal,
            )}
            · {eur.small
              ? "als Kleinunternehmer mit Bruttobeträgen"
              : "Beträge netto, Steuern in eigenen Zeilen"}</small
          >
        </div>
      </div>
    </div>
    <div class="tax-step">
      <span class="tax-step-no">3</span>
      <div>
        {#if advisor}
          <h3>An die Steuerkanzlei geben</h3>
          <p>
            Deine Kanzlei erstellt die Erklärungen. Gib ihr die Daten des Jahres als DATEV-Export
            und die Antworten auf die Fragen oben.
          </p>
          <div class="tax-actions">
            <button class="secondary" onclick={() => (datevOpen = true)}>DATEV-Export {year}</button
            >
          </div>
        {:else}
          <h3>In Mein ELSTER übertragen</h3>
        {/if}
        <div class="detail-list">
          <details open={!advisor}>
            <summary
              ><span class="detail-title">Anlage EÜR: Werte Zeile für Zeile</span><span
                class="detail-state">Zum Übertragen in ELSTER</span
              ></summary
            >
            <h4>Betriebseinnahmen</h4>
            <TaxFieldTable
              fields={eur.eur.income}
              caption="Betriebseinnahmen der Anlage EÜR"
              copyable={!advisor}
            />
            <h4>Betriebsausgaben</h4>
            <TaxFieldTable
              fields={eur.eur.expenses}
              caption="Betriebsausgaben der Anlage EÜR"
              copyable={!advisor}
            />
            {#if eur.eur.assets?.length}
              <h4>Anschaffungen (für die Anlage AVEÜR)</h4>
              <table class="tax-fields">
                <thead
                  ><tr
                    ><th>Gekauft</th><th>Was</th><th class="num">Kosten</th><th class="num"
                      >Abschreibung {year}</th
                    ><th class="num">Restwert</th></tr
                  ></thead
                >
                <tbody>
                  {#each eur.eur.assets as a (a.entry.id)}<tr>
                      <td>{dateLabel(a.acquired)}</td>
                      <td
                        >{a.entry.label}<small class="tax-muted"
                          >{a.gwg
                            ? " · geringwertig, sofort abgesetzt"
                            : ` · ${yearsLabel(a.years)} Nutzung`}</small
                        ></td
                      >
                      <td class="num">{amountLabel(a.cost)}</td>
                      <td class="num">{amountLabel(a.depreciation)}</td>
                      <td class="num">{amountLabel(a.bookValue)}</td>
                    </tr>{/each}
                </tbody>
              </table>
            {/if}
          </details>
        </div>
        {#if !advisor}
          <ol class="tax-steps">
            <li>
              Öffne in Mein ELSTER die „Einkommensteuererklärung {year}“ und füge die Anlage EÜR{annex
                ? ` und die ${annex}`
                : ""} hinzu.
            </li>
            <li>
              Trage die Werte in die Zeilen mit derselben Kennzahl ein, dazu deine Antworten auf die
              Fragen.
            </li>
            {#if eur.eur.assets?.length}<li>
                Fülle für Anschaffungen mit Abschreibung die Anlage AVEÜR aus.
              </li>{/if}
            {#if !setup.smallBusiness}<li>
                Gib die Umsatzsteuer-Jahreserklärung ab; sie fasst deine Voranmeldungen des Jahres
                zusammen.
              </li>{/if}
            <li>Vergleiche Summen und Gewinn mit den Werten hier.</li>
          </ol>
          {#if vatYear && !setup.smallBusiness}<div class="detail-list annual-vat">
              <details>
                <summary
                  ><span class="detail-title">Jahreswerte für die Umsatzsteuer-Jahreserklärung</span
                  ><span class="detail-state">Kennzahlen</span></summary
                >
                <p>
                  Rechnerisch nach den Zahlen in der App, mit den Kennzahlen der Voranmeldung. Das
                  Formular der Jahreserklärung hat eigene Zeilen. Summe des Jahres:
                  {amountLabel(vatYear.return.payable)}.
                </p>
                <TaxFieldTable
                  fields={vatYear.return.fields}
                  caption="Jahreswerte der Umsatzsteuer"
                  copyable={false}
                />
              </details>
            </div>{/if}
          {#if trade}
            <p>
              Die Gewerbesteuererklärung ist abzugeben, wenn der Gewerbeertrag {year} mehr als 24.500
              € beträgt oder das Finanzamt sie verlangt (§ 25 GewStDV). Ausgangspunkt ist der Gewinn laut
              EÜR, hier {amountLabel(eur.eur.profit)}; Hinzurechnungen und Kürzungen rechnet die App
              nicht vor.
            </p>
          {:else if !setup.activity}
            <p>
              Die Gewerbesteuererklärung rechnet die App nicht vor. Ausgangspunkt ist der Gewinn aus
              der EÜR. Ob du ein Gewerbe hast, legst du im Steuerprofil fest.
            </p>
          {/if}
          <div class="tax-actions">
            <button class="secondary" onclick={openElster}>Mein ELSTER öffnen</button>
          </div>
        {/if}
        <FilingMark
          filing={eur.filing}
          changed={eur.changed}
          until={`${year}-12-31`}
          onmark={mark}
          onunmark={() => mark("")}
        />
        <TaxHelpNote />
      </div>
    </div>
  </section>
  {#if datevOpen}
    <DatevExportDialog
      from={`${year}-01-01`}
      to={`${year}-12-31`}
      onclose={() => (datevOpen = false)}
    />
  {/if}
{/if}

<style>
  h4 {
    margin: 16px 0 6px;
    font-size: 14px;
  }
  .questions-title {
    margin-top: 16px;
  }
  .annual-vat {
    margin: 16px 0;
  }
  .questions {
    display: grid;
    gap: 10px;
    margin: 8px 0 0;
    padding: 0;
    list-style: none;
  }
  .questions li {
    display: grid;
    grid-template-columns: minmax(0, 1fr) auto;
    gap: 6px 12px;
    align-items: center;
  }
  .choice {
    display: inline-flex;
    gap: 6px;
  }
  .choice button[aria-checked="true"] {
    border-color: var(--theme-accent);
    box-shadow: 0 0 0 1px var(--theme-accent);
  }
  .answer {
    grid-column: 1 / -1;
    margin: 0;
    padding: 8px 10px;
    border-radius: 6px;
    background: var(--theme-info-bg);
    color: var(--theme-info-text);
  }
</style>
