<script lang="ts">
  import type { InvoiceDraft } from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/model/models";
  import { tick } from "svelte";
  import FormDialog from "../../components/FormDialog.svelte";
  import { taxChoices } from "../../lib/tax";
  import { applyTaxDecision, taxQuestions, taxReviewItems } from "../../lib/taxReview";

  let {
    draft,
    busy,
    onapply,
    onsettings,
  }: {
    draft: InvoiceDraft;
    busy: boolean;
    onapply: (draft: InvoiceDraft) => void;
    onsettings: () => void;
  } = $props();
  const items = $derived(taxReviewItems(draft));
  // Dismissal is local to this invoice and these tax inputs, never a global preference.
  const signature = $derived(
    JSON.stringify([
      draft.id,
      draft.taxTreatment,
      draft.lines?.map((l) => [l.id, l.name, l.taxTreatment, l.taxCategory, l.taxRate]),
    ]),
  );
  let dismissed = $state("");
  let selected = $state<number | null>(null);
  let answer = $state("");
  let step = $state(1);
  let stepTitle = $state<HTMLHeadingElement>();
  async function goToStep(next: number) {
    step = next;
    confirmed = false;
    error = "";
    await tick();
    stepTitle?.focus({ preventScroll: true });
    stepTitle?.closest(".dialog-body")?.scrollTo({ top: 0 });
  }
  let confirmed = $state(false);
  let reason = $state("");
  let rate = $state("19");
  let error = $state("");
  const question = $derived(taxQuestions.find((q) => q.id === answer));
  const choice = $derived(taxChoices.find((c) => c.id === answer));
  function open(index: number) {
    selected = index;
    step = 1;
    answer = "";
    confirmed = false;
    reason = "";
    error = "";
    rate = items.find((item) => item.index === index)?.rate
      ? draft.lines?.[index]?.taxRate || "19"
      : "19";
  }
  function resetAnswerDetails() {
    reason = "";
    confirmed = false;
    error = "";
  }
  function apply() {
    if (step !== 2 || selected === null || !confirmed || !choice || busy) return;
    try {
      const updated = applyTaxDecision(draft, selected, answer, reason, rate);
      onapply(updated);
      selected = null;
    } catch (e) {
      error = e instanceof Error ? e.message : String(e);
    }
  }
</script>

{#if items.length}
  <aside class="guidance-report tax-review" aria-label="Hinweise zur Umsatzsteuer">
    {#if dismissed !== signature}
      <header>
        <strong>Passt die Umsatzsteuer zu deiner Rechnung?</strong>
        <button type="button" class="text" disabled={busy} onclick={() => (dismissed = signature)}
          >Ausblenden</button
        >
      </header>
      <p>
        Schau dir {items.length === 1 ? "diese Position" : "diese Positionen"} bitte noch einmal an. Mit
        ein paar einfachen Fragen helfen wir dir dabei.
      </p>
      <ul>
        {#each items as item}
          <li>
            <div class="review-copy">
              <strong
                >Position {item.index + 1}: {draft.lines?.[item.index]?.name || "Ohne Bezeichnung"} ·
                {String(item.rate).replace(".", ",")} %</strong
              >
              <p>{item.message}</p>
              {#if item.rate === 0}<details class="review-note link-toggle">
                  <summary>Was passiert, wenn ich nichts ändere?</summary>
                  <p>
                    Bei der üblichen Einstellung behandelt das Programm 0 % als gesetzlichen
                    Nullsteuersatz. Wenn du zum Beispiel als Kleinunternehmer abrechnest, musst du
                    einen anderen Grund wählen. Ausblenden ändert deine Angaben nicht.
                  </p>
                </details>{/if}
            </div>
            <button type="button" class="secondary" disabled={busy} onclick={() => open(item.index)}
              >Jetzt prüfen</button
            >
          </li>
        {/each}
      </ul>
    {:else}
      <header>
        <strong>Hinweise zur Umsatzsteuer ausgeblendet</strong><button
          type="button"
          class="text"
          disabled={busy}
          onclick={() => (dismissed = "")}>Wieder anzeigen ({items.length})</button
        >
      </header>
    {/if}
  </aside>
{/if}

<details class="tax-reference detail-row">
  <summary
    ><span class="detail-title">Gut zu wissen: Umsatzsteuer auf deiner Rechnung</span><span
      class="detail-state">Steuersätze und Sonderfälle</span
    ></summary
  >
  <div class="reference-content">
    <p>
      <strong>Meist sind es 19 % oder 7 %.</strong> Welcher Satz gilt, hängt von deiner Leistung ab. Bei
      verschiedenen Leistungen kann eine Aufteilung nötig sein.
    </p>
    <p>
      <strong>Keine Umsatzsteuer? Der Grund zählt.</strong> Zum Beispiel gelten für Kleinunternehmer andere
      Regeln als für bestimmte Solaranlagen. Auch Lieferungen ins Ausland solltest du gesondert prüfen.
    </p>
    <p>
      <strong>Andere Steuersätze sind möglich.</strong> 5,5 % und 7,8 % gibt es für bestimmte Betriebe
      in der Land- und Forstwirtschaft. 16 % und 5 % können bei Leistungen aus dem zweiten Halbjahr 2020
      passen.
    </p>
    <p>
      Für gebrauchte Waren und Reiseleistungen gelten besondere Regeln. Die Steuer auf den
      Gewinnanteil berechnet das Programm nicht. Für Reiseleistungen gibt es noch keine eigene
      Auswahl.
    </p>
    <details class="link-toggle">
      <summary>Rechtsgrundlagen und Angaben in der E-Rechnung</summary>
      <div class="reference-content">
        <p>
          Die technischen Steuerkennzeichen setzt die App automatisch: S für positive Steuersätze, Z
          für den gesetzlichen Nullsteuersatz, E für Befreiungen, AE für Reverse Charge, K für
          innergemeinschaftliche Lieferungen, G für Ausfuhrlieferungen und O für nicht steuerbare
          Vorgänge. S darf nicht mit 0 % verwendet werden.
        </p>
        <p>
          Stand: 16.09.2026 · <a
            href="https://www.gesetze-im-internet.de/ustg_1980/"
            target="_blank"
            rel="noreferrer">Umsatzsteuergesetz</a
          >
          ·
          <a
            href="https://docs.peppol.eu/poacc/billing/3.0/codelist/UNCL5305/"
            target="_blank"
            rel="noreferrer">Technische Steuerkennzeichen</a
          >
        </p>
      </div>
    </details>
    <button type="button" class="text settings-link" disabled={busy} onclick={onsettings}
      >Rechnungseinstellungen öffnen</button
    >
  </div>
</details>

{#if selected !== null}
  <FormDialog
    title="Umsatzsteuer Schritt für Schritt prüfen"
    description={`Position ${selected + 1}: ${draft.lines?.[selected]?.name || "Ohne Bezeichnung"}`}
    {busy}
    {error}
    onclose={() => (selected = null)}
    onsubmit={() => {
      if (step === 1 && answer) goToStep(2);
      else apply();
    }}
  >
    <div class="tax-help">
      <div class="step-heading">
        <p class="step-caption">
          Schritt {step} von 2 · {step === 1 ? "Grund auswählen" : "Angaben prüfen"}
        </p>
        <h3 bind:this={stepTitle} tabindex="-1">
          {step === 1
            ? "Welche Aussage passt zu dieser Position?"
            : answer === "unknown"
              ? "Du musst jetzt nichts festlegen"
              : "Passt das zu deiner Leistung?"}
        </h3>
      </div>
      {#if step === 1}
        <fieldset class="answer-list">
          <legend class="answer-legend"
            >Wähle die passende Aussage. Im nächsten Schritt erklären wir, worauf du achten
            solltest.</legend
          >
          {#each taxQuestions.slice(0, 4) as q}
            <label class="answer-option" class:chosen={answer === q.id}
              ><input
                type="radio"
                name="tax-answer"
                value={q.id}
                bind:group={answer}
                onchange={resetAnswerDetails}
              /><span>{q.question}</span></label
            >
          {/each}
          <details
            class="more-options link-toggle"
            open={taxQuestions.slice(4).some((q) => q.id === answer)}
          >
            <summary>Ausland und weitere Sonderfälle</summary>
            <div class="answer-list">
              {#each taxQuestions.slice(4) as q}<label
                  class="answer-option"
                  class:chosen={answer === q.id}
                  ><input
                    type="radio"
                    name="tax-answer"
                    value={q.id}
                    bind:group={answer}
                    onchange={resetAnswerDetails}
                  /><span>{q.question}</span></label
                >{/each}
            </div>
          </details>
          <label class="answer-option" class:chosen={answer === "unknown"}
            ><input
              type="radio"
              name="tax-answer"
              value="unknown"
              bind:group={answer}
              onchange={resetAnswerDetails}
            /><span>Ich bin mir nicht sicher</span></label
          >
        </fieldset>
      {:else if question && choice}
        <div class="recommendation">
          <strong>{question.question}</strong>
          <p>{question.explanation}</p>
        </div>
        {#if answer === "standard" || answer === "exempt"}
          <div class="form-grid">
            {#if answer === "standard"}<label
                >Wie viel Umsatzsteuer berechnest du?<input
                  required
                  inputmode="decimal"
                  bind:value={rate}
                  placeholder="z. B. 19 oder 7"
                /><small>Steuersatz in Prozent</small></label
              >{/if}
            {#if answer === "exempt"}<label
                >Warum ist deine Leistung steuerfrei?<input
                  required
                  bind:value={reason}
                  placeholder="z. B. Heilbehandlung nach § 4 Nr. 14 UStG"
                /><small>Dieser Hinweis erscheint auf deiner Rechnung.</small></label
              >{/if}
          </div>
        {/if}
        <details class="technical-details link-toggle">
          <summary>Rechtsgrundlage und technische Angaben</summary>
          <div class="reference-content">
            <a href={question.source} target="_blank" rel="noreferrer"
              >Regelung und Voraussetzungen nachlesen</a
            >
            <p>
              In der E-Rechnung steht das Steuerkennzeichen <strong>{choice.category}</strong>
              ({choice.label}). Den Code musst du nicht selbst eingeben.
            </p>
          </div>
        </details>
        {#if !draft.adjustments?.length}
          <label class="confirmation"
            ><input type="checkbox" bind:checked={confirmed} /><span
              >Ja, die beschriebenen Voraussetzungen treffen auf diese Position zu.</span
            ></label
          >
          <p class="quiet-copy">
            Wir ändern nur diese Position und berechnen die Rechnung neu. Die anderen Positionen
            bleiben unverändert.
          </p>
        {/if}
      {:else}
        <p>
          Kläre zuerst, welche Regel für deine Leistung gilt, zum Beispiel mit deiner
          Steuerberatung. Deine bisherigen Angaben bleiben erhalten.
        </p>
        <p class="quiet-copy">
          Du kannst den Entwurf weiter bearbeiten und diese Hilfe später noch einmal öffnen.
        </p>
      {/if}
      {#if step === 2 && draft.adjustments?.length}<div class="recommendation">
          <strong>Prüfe auch die Rabatte und Zuschläge</strong>
          <p>
            Damit die Steuer überall zusammenpasst, bearbeitest du diese Rechnung am besten in den
            Rechnungseinstellungen.
          </p>
          <button
            type="button"
            class="text settings-link"
            disabled={busy}
            onclick={() => {
              selected = null;
              onsettings();
            }}>Rechnungseinstellungen öffnen</button
          >
        </div>{/if}
      {#if step === 2 && answer === "margin-goods" && !draft.adjustments?.length}<button
          type="button"
          class="text settings-link"
          disabled={busy}
          onclick={() => {
            selected = null;
            onsettings();
          }}>Kunst oder Sammlerstücke? Rechnungseinstellungen öffnen</button
        >{/if}
    </div>
    {#snippet footer()}
      {#if step === 1}
        <button type="button" class="secondary" disabled={busy} onclick={() => (selected = null)}
          >Abbrechen</button
        >
        <button class="primary" disabled={busy || !answer}>Weiter</button>
      {:else}
        <button type="button" class="secondary" disabled={busy} onclick={() => goToStep(1)}
          >Zurück</button
        >
        {#if choice && !draft.adjustments?.length}<button
            class="primary"
            disabled={busy || !confirmed}>Angaben übernehmen</button
          >{:else}<button
            type="button"
            class="primary"
            disabled={busy}
            onclick={() => (selected = null)}>Schließen</button
          >{/if}
      {/if}
    {/snippet}
  </FormDialog>
{/if}

<style>
  .tax-review > header {
    flex-wrap: wrap;
  }
  .tax-review li {
    align-items: center;
  }
  .tax-review li > button {
    text-align: center;
    white-space: nowrap;
    font-size: var(--ui-control-font-size);
  }
  .review-copy {
    display: grid;
    gap: 8px;
    min-width: 0;
    font-size: 13px;
    line-height: 1.5;
  }
  .review-copy p {
    margin: 0;
  }
  .review-note {
    font-size: 12px;
  }
  .review-note p {
    margin-top: 8px;
  }
  summary {
    cursor: pointer;
    font-size: 12px;
    line-height: 1.5;
  }
  .tax-reference {
    margin: 0 24px 16px;
    color: var(--theme-muted);
  }
  .reference-content {
    display: grid;
    gap: 12px;
    padding-top: 12px;
  }
  .reference-content p {
    margin: 0;
    font-size: 13px;
    line-height: 1.5;
  }
  .tax-help {
    display: grid;
    gap: var(--ui-field-gap);
    font-size: 13px;
    line-height: 1.5;
  }
  .tax-help p,
  .tax-help h3 {
    margin: 0;
  }
  .step-heading {
    display: grid;
    gap: 6px;
  }
  .step-caption,
  .quiet-copy {
    color: var(--theme-muted);
    font-size: 12px;
  }
  .step-heading h3:focus {
    outline: none;
  }
  .answer-list {
    display: grid;
    gap: 8px;
    border: 0;
    padding: 0;
    margin: 0;
    min-width: 0;
  }
  .answer-legend {
    padding: 0 0 12px;
    color: var(--theme-muted);
    font-size: 13px;
  }
  .answer-option {
    display: flex;
    align-items: center;
    gap: 12px;
    padding: 12px;
    border: 1px solid var(--theme-border);
    border-radius: var(--ui-control-radius);
    cursor: pointer;
    font-size: 13px;
    line-height: 1.5;
    color: var(--theme-text);
    background: var(--theme-surface);
  }
  .answer-option:hover,
  .answer-option.chosen {
    background: var(--theme-surface-muted);
  }
  .answer-option.chosen {
    border-color: var(--theme-focus);
  }
  .answer-option input,
  .confirmation input {
    width: 16px;
    height: 16px;
    min-height: 0;
    padding: 0;
    margin: 0;
    flex: 0 0 16px;
    accent-color: var(--theme-primary);
  }
  .more-options {
    padding: 4px 0;
  }
  .more-options .answer-list {
    padding-top: 12px;
  }
  .recommendation {
    display: grid;
    gap: 8px;
    border: 1px solid var(--theme-border);
    border-radius: var(--ui-control-radius);
    padding: 16px;
    background: var(--theme-surface-muted);
  }
  .technical-details {
    color: var(--theme-muted);
  }
  .confirmation {
    display: flex;
    align-items: flex-start;
    gap: 10px;
    font-size: 13px;
    line-height: 1.5;
    cursor: pointer;
  }
  .confirmation input {
    margin-top: 2px;
  }
  .settings-link {
    justify-self: start;
    text-align: left;
    padding: 0;
  }
  @media (max-width: 600px) {
    .tax-review li {
      flex-direction: column;
      align-items: stretch;
      gap: 12px;
    }
    .tax-review li > button {
      align-self: flex-start;
    }
  }
</style>
