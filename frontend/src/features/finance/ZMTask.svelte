<script lang="ts">
  import { Clipboard } from "@wailsio/runtime";
  import * as API from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/app/invoiceservice";
  import type {
    TaxSetup,
    ZMReturnView,
  } from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/app/models";
  import { serviceErrorMessage } from "../../lib/errors";
  import { amountLabel, dateLabel } from "../../lib/format";
  import { openElster } from "../../lib/elster";
  import TaxIssueList from "./TaxIssueList.svelte";
  import FilingMark from "./FilingMark.svelte";
  import TaxHelpNote from "./TaxHelpNote.svelte";

  let {
    periodId,
    setup,
    onchanged,
  }: { periodId: string; setup: TaxSetup; onchanged: () => Promise<void> } = $props();
  let zm = $state<ZMReturnView | null>(null);
  let error = $state("");
  let copied = $state("");
  const changed = $derived(
    !!zm?.report.issues?.some((issue) => issue.code === "changed_after_filing"),
  );

  async function load(id: string) {
    error = "";
    try {
      zm = await API.ZMReturn(id);
    } catch (cause) {
      error = serviceErrorMessage(
        cause,
        "Die Zusammenfassende Meldung konnte nicht erstellt werden.",
      );
    }
  }
  $effect(() => {
    void load(periodId);
  });
  async function mark(submittedOn: string) {
    if (!zm) return;
    error = "";
    try {
      if (submittedOn) await API.MarkZMFiled(zm.period.id, submittedOn, zm.report.total);
      else await API.UnmarkZMFiled(zm.period.id);
      await load(zm.period.id);
      await onchanged();
    } catch (cause) {
      error = serviceErrorMessage(cause, "Der Vermerk konnte nicht gespeichert werden.");
    }
  }
  async function copy(text: string, key: string) {
    try {
      await Clipboard.SetText(text);
    } catch {
      await navigator.clipboard?.writeText(text);
    }
    copied = key;
    setTimeout(() => {
      if (copied === key) copied = "";
    }, 1500);
  }
</script>

{#if error}<div class="alert error" role="alert">{error}</div>{/if}
{#if zm}
  <section class="tax-card" aria-labelledby="zm-task-title">
    <h2 id="zm-task-title">Zusammenfassende Meldung {zm.period.label}</h2>
    <p class="tax-muted">
      {zm.filing
        ? `Übermittelt am ${dateLabel(zm.filing.submittedOn)}`
        : `Übermitteln bis ${dateLabel(zm.period.due)} – ohne Dauerfristverlängerung`}
    </p>
    <p>
      Wer Waren steuerfrei an Unternehmen in anderen EU-Ländern liefert oder dort Leistungen für
      Unternehmen erbringt, meldet die Summen je Kunde an das Bundeszentralamt für Steuern (§ 18a
      UStG). Die Summen müssen zu den Kennzahlen 41 und 21 der Voranmeldung passen.
    </p>
    <div class="tax-step">
      <span class="tax-step-no">1</span>
      <div>
        <h3>Vollständig?</h3>
        <TaxIssueList issues={zm.report.issues} />
      </div>
    </div>
    <div class="tax-step">
      <span class="tax-step-no">2</span>
      <div>
        <h3>Ergebnis</h3>
        <div class="tax-result">
          <strong>{amountLabel(zm.report.total)}</strong>
          <small
            >{zm.report.lines?.length ?? 0}
            {(zm.report.lines?.length ?? 0) === 1 ? "Zeile" : "Zeilen"} · Beträge in vollen Euro</small
          >
        </div>
      </div>
    </div>
    <div class="tax-step">
      <span class="tax-step-no">3</span>
      <div>
        <h3>
          {setup.filing === "advisor"
            ? "Mit der Steuerkanzlei abstimmen"
            : "In Mein ELSTER übertragen"}
        </h3>
        <table class="tax-fields">
          <caption class="tax-hidden-caption">Zeilen der Zusammenfassenden Meldung</caption>
          <thead
            ><tr><th>Land</th><th>USt-IdNr.</th><th>Art</th><th class="num">Summe</th><th></th></tr
            ></thead
          >
          <tbody>
            {#each zm.report.lines ?? [] as line (line.vatId + line.services)}
              <tr>
                <td>{line.country}</td>
                <td
                  ><strong>{line.vatId}</strong>
                  <small class="tax-muted">· {line.partner}</small
                  >{#if line.entries?.length}<details class="link-toggle">
                      <summary
                        >{line.entries.length}
                        {line.entries.length === 1 ? "Rechnung" : "Rechnungen"}</summary
                      >
                      <ul class="tax-entries">
                        {#each line.entries as e}<li>
                            {e.date ? dateLabel(e.date) + " · " : ""}{e.label} · {amountLabel(
                              e.amount,
                            )}
                          </li>{/each}
                      </ul>
                    </details>{/if}</td
                >
                <td>{line.services ? "Sonstige Leistung" : "Warenlieferung"}</td>
                <td class="num">{line.amount}</td>
                <td
                  ><button
                    class="text tax-copy"
                    aria-label={`USt-IdNr. ${line.vatId} kopieren`}
                    onclick={() => copy(line.vatId.slice(2), line.vatId + line.services)}
                    >{copied === line.vatId + line.services ? "Kopiert" : "ID kopieren"}</button
                  ></td
                >
              </tr>
            {/each}
          </tbody>
        </table>
        {#if setup.filing !== "advisor"}
          <ol class="tax-steps">
            <li>
              Öffne Mein ELSTER → Formulare & Leistungen → „Zusammenfassende Meldung“ und wähle den
              Meldezeitraum.
            </li>
            <li>
              Trage je Zeile das Land, die USt-IdNr. ohne Länderkürzel, die Summe und die Art ein.
            </li>
            <li>
              Sende die Meldung ab. Erkennst du später einen Fehler, ist sie innerhalb eines Monats
              zu berichtigen (§ 18a Abs. 10 UStG).
            </li>
          </ol>
          <div class="tax-actions">
            <button class="secondary" onclick={openElster}>Mein ELSTER öffnen</button>
          </div>
        {/if}
        <FilingMark
          filing={zm.filing}
          {changed}
          until={zm.period.to}
          onmark={mark}
          onunmark={() => mark("")}
        />
        <TaxHelpNote />
      </div>
    </div>
  </section>
{/if}
