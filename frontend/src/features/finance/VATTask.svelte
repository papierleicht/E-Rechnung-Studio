<script lang="ts">
  import * as API from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/app/invoiceservice";
  import type {
    TaxSetup,
    VATReturnView,
  } from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/app/models";
  import { serviceErrorMessage } from "../../lib/errors";
  import { amountLabel, dateLabel } from "../../lib/format";
  import DatevExportDialog from "./DatevExportDialog.svelte";
  import TaxFieldTable from "./TaxFieldTable.svelte";
  import TaxIssueList from "./TaxIssueList.svelte";
  import FilingMark from "./FilingMark.svelte";
  import TaxHelpNote from "./TaxHelpNote.svelte";
  import { openElster } from "../../lib/elster";

  let {
    periodId,
    setup,
    onchanged,
  }: { periodId: string; setup: TaxSetup; onchanged: () => Promise<void> } = $props();
  let vat = $state<VATReturnView | null>(null);
  let error = $state("");
  let datevOpen = $state(false);
  const payable = $derived(Number(vat?.return.payable ?? 0));
  const advisor = $derived(setup.filing === "advisor");
  const changed = $derived(
    !!vat?.return.issues?.some((issue) => issue.code === "changed_after_filing"),
  );

  async function load(id: string) {
    error = "";
    try {
      vat = await API.VATReturn(id);
    } catch (cause) {
      error = serviceErrorMessage(cause, "Die Voranmeldung konnte nicht berechnet werden.");
    }
  }
  $effect(() => {
    void load(periodId);
  });
  async function mark(submittedOn: string) {
    if (!vat) return;
    error = "";
    try {
      if (submittedOn) await API.MarkVATFiled(vat.period.id, submittedOn, vat.return.payable);
      else await API.UnmarkVATFiled(vat.period.id);
      await load(vat.period.id);
      await onchanged();
    } catch (cause) {
      error = serviceErrorMessage(cause, "Der Vermerk konnte nicht gespeichert werden.");
    }
  }
</script>

{#if error}<div class="alert error" role="alert">{error}</div>{/if}
{#if vat}
  <section class="tax-card" aria-labelledby="vat-task-title">
    <h2 id="vat-task-title">Umsatzsteuer-Voranmeldung {vat.period.label}</h2>
    <p class="tax-muted">
      {vat.filing
        ? `Übermittelt am ${dateLabel(vat.filing.submittedOn)}`
        : `${payable > 0 ? "Übermitteln und zahlen" : "Übermitteln"} bis ${dateLabel(vat.period.due)}`}
      {setup.smallBusiness ? " · nur Steuer aus Rechnungen mit „Steuerschuld bei dir“" : ""}
    </p>
    <div class="tax-step">
      <span class="tax-step-no">1</span>
      <div>
        <h3>Vollständig?</h3>
        <TaxIssueList issues={vat.return.issues} />
      </div>
    </div>
    <div class="tax-step">
      <span class="tax-step-no">2</span>
      <div>
        <h3>Ergebnis</h3>
        <div class="tax-result">
          <strong
            >{payable < 0
              ? `Rechnerisch bekommst du ${amountLabel(String(-payable))} zurück`
              : `Rechnerisch zahlst du ${amountLabel(vat.return.payable)}`}</strong
          >
          <small>Kennzahl 83, gerechnet wie ELSTER mit Bemessungsgrundlagen in vollen Euro.</small>
        </div>
      </div>
    </div>
    <div class="tax-step">
      <span class="tax-step-no">3</span>
      <div>
        {#if advisor}
          <h3>An die Steuerkanzlei geben</h3>
          <p>
            Deine Kanzlei übermittelt die Voranmeldung. Gib ihr die Daten des Zeitraums als
            DATEV-Export und markiere den Zeitraum, sobald sie ihn übermittelt hat.
          </p>
          <div class="tax-actions">
            <button class="secondary" onclick={() => (datevOpen = true)}
              >DATEV-Export erstellen</button
            >
          </div>
          <div class="detail-list">
            <details>
              <summary
                ><span class="detail-title">Werte zur Kontrolle</span><span class="detail-state"
                  >Alle Kennzahlen mit Belegen</span
                ></summary
              >
              <TaxFieldTable
                fields={vat.return.fields}
                caption="Kennzahlen der Umsatzsteuer-Voranmeldung"
                copyable={false}
              />
            </details>
          </div>
        {:else}
          <h3>In Mein ELSTER übertragen</h3>
          {#if (vat.return.fields?.length ?? 0) > 1}
            <TaxFieldTable
              fields={vat.return.fields}
              caption="Kennzahlen der Umsatzsteuer-Voranmeldung"
            />
          {:else}
            <p>
              In der App gibt es für diesen Zeitraum keine Umsätze und keine Vorsteuer. Abzugeben
              ist die Voranmeldung in der Regel trotzdem – mit einer 0 in Kennzahl 83.
            </p>
          {/if}
          <ol class="tax-steps">
            <li>
              Öffne Mein ELSTER → Formulare & Leistungen → „Umsatzsteuer-Voranmeldung {vat.period.from.slice(
                0,
                4,
              )}“ und wähle den Zeitraum.
            </li>
            <li>
              Trage die Werte in die gleichnamigen Kennzahlen ein. Bei 81 und 86 rechnet ELSTER die
              Steuer selbst.
            </li>
            <li>Prüfe, ob ELSTER bei Kennzahl 83 denselben Betrag zeigt, und sende ab.</li>
            {#if payable > 0}
              <li>
                Zahle fristgerecht und ordne die Abbuchung unter „Konto & Finanzen“ als
                „Umsatzsteuer an oder vom Finanzamt“ ein – dann zählt sie in der EÜR.
              </li>
            {:else if payable < 0}
              <li>
                Das Finanzamt erstattet den Überschuss, sofern es ihn nicht mit anderen Steuern
                verrechnet. Ordne den Zahlungseingang unter „Konto & Finanzen“ als „Umsatzsteuer an
                oder vom Finanzamt“ ein – dann zählt er in der EÜR.
              </li>
            {/if}
          </ol>
          <div class="tax-actions">
            <button class="secondary" onclick={openElster}>Mein ELSTER öffnen</button>
          </div>
        {/if}
        <FilingMark
          filing={vat.filing}
          {changed}
          until={vat.period.to}
          onmark={mark}
          onunmark={() => mark("")}
        />
        <TaxHelpNote />
      </div>
    </div>
  </section>
  {#if datevOpen}
    <DatevExportDialog
      from={vat.period.from}
      to={vat.period.to}
      onclose={() => (datevOpen = false)}
    />
  {/if}
{/if}
