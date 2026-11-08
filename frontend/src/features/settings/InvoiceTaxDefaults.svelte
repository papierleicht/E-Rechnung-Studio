<script lang="ts">
  import { untrack } from "svelte";
  import type { Company } from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/model/models";
  import {
    invoiceTaxPreset,
    chooseInvoiceTaxPreset,
    companyTaxTreatment,
    taxChoices,
    taxReasons,
  } from "../../lib/tax";
  import { setupTaxGuidance } from "../../lib/setupGuidance";
  let { company = $bindable() }: { company: Company } = $props();
  const preset = $derived(invoiceTaxPreset(company));
  let uniformPreset = $state(
    untrack(() => (invoiceTaxPreset(company) === "mixed" ? "" : invoiceTaxPreset(company))),
  );
  let uniformReason = $state(untrack(() => company.exemptionReason));
  function togglePositionTax(enabled: boolean) {
    if (enabled) {
      uniformPreset = preset;
      uniformReason = company.exemptionReason;
      chooseInvoiceTaxPreset(company, "mixed");
    } else {
      chooseInvoiceTaxPreset(company, uniformPreset);
      company.exemptionReason = uniformReason;
    }
  }
  const effective = $derived(companyTaxTreatment(company));
  const reason = $derived(
    ["standard", "zero-rated"].includes(effective)
      ? ""
      : company.exemptionReason.trim() || taxReasons[effective] || "",
  );
</script>

<section class="company-tax-settings" aria-label="Steuerliche Rechnungsvorgaben">
  <h2>Rechnungsvorgaben</h2>
  <h3>Steuerliche Voreinstellung</h3>
  <label class="position-tax-toggle">
    <input
      type="checkbox"
      data-field="company.invoiceTaxPreset"
      checked={preset === "mixed"}
      onchange={(event) => togglePositionTax(event.currentTarget.checked)}
    />
    <span>Umsatzsteuer je Position festlegen</span>
  </label>
  <label class:tax-muted={preset === "mixed"}
    >Wie sollen neue Rechnungen vorbereitet werden?
    <select
      disabled={preset === "mixed"}
      value={preset === "mixed" ? uniformPreset : preset}
      onchange={(event) => chooseInvoiceTaxPreset(company, event.currentTarget.value)}
    >
      <option value="">Je Rechnung festlegen – mit Unternehmensstandard beginnen</option>
      <optgroup label="Optionale Vorgabe für überwiegend gleiche Leistungen">
        {#each taxChoices.filter((choice) => choice.id !== "small-business") as choice}
          <option value={choice.id}>{choice.label}</option>
        {/each}
      </optgroup>
    </select>
  </label>
  <p class="form-help">
    Rechnest du überwiegend gleichartige Leistungen ab? Dann kannst du hier eine Behandlung gezielt
    vorbelegen. Deinen Kleinunternehmerstatus ändert das nicht.
  </p>
  {#if preset === "mixed"}
    <p class="form-help">
      Neue Rechnungen verwenden die Steuerbehandlung je Position. Der einheitliche Steuerfall ist
      deaktiviert. Steuersatz und gegebenenfalls Befreiungsgrund wählst du bei der jeweiligen
      Position. Die gesetzlichen Voraussetzungen müssen weiterhin erfüllt sein.
    </p>
  {:else if preset}
    <p class="form-help">{setupTaxGuidance[preset]?.meaning} {setupTaxGuidance[preset]?.effect}</p>
    <p class="form-help">
      Die Auswahl verändert die Steuerberechnung und die elektronischen Rechnungsdaten – nicht nur
      den Hinweistext. Prüfe die Voraussetzungen für jede Rechnung.
    </p>
    {#if company.smallBusiness}<p class="form-help">
        Diese ausdrückliche Rechnungsvorgabe hat Vorrang vor der üblichen
        Kleinunternehmer-Behandlung. Wähle sie nur, wenn die abweichende Behandlung für deine
        Leistungen geklärt ist.
      </p>{/if}
    {#if !["standard", "zero-rated"].includes(preset)}
      <label
        >Steuerlicher Hinweis für neue Rechnungen
        <input
          data-field="company.exemptionReason"
          bind:value={company.exemptionReason}
          placeholder={taxReasons[preset] || "Trage hier den passenden Befreiungsgrund ein"}
        />
      </label>
    {/if}
  {:else}
    <p class="form-help">
      Neue Rechnungen beginnen mit {company.smallBusiness
        ? "der Kleinunternehmer-Behandlung"
        : "der regulären Umsatzsteuer-Behandlung"}. Besonderheiten wählst du auf der Rechnung oder
      je Position. Eine pauschale Sonderbehandlung wird nicht übernommen.
    </p>
  {/if}
  <div class="tax-preview" aria-live="polite">
    <strong>So bereiten wir die Rechnung vor</strong>
    <p>
      {effective === "mixed"
        ? "Steuerberechnung und Hinweise ergeben sich aus den einzelnen Positionen."
        : ["standard", "zero-rated"].includes(effective)
          ? effective === "standard"
            ? "Umsatzsteuer nach dem Steuersatz der jeweiligen Position."
            : "Nullsteuersatz (0 %), keine Steuerbefreiung."
          : "Ohne gesonderten Umsatzsteuerausweis."}
    </p>
    <p>
      Hinweis auf der Rechnung: {effective === "mixed"
        ? "Abhängig von der Steuerbehandlung der Positionen; kein einheitlicher Hinweis."
        : reason
          ? `„${reason}“`
          : effective === "exempt"
            ? "Noch nicht festgelegt – ergänze den passenden Befreiungsgrund."
            : "Kein zusätzlicher Steuerhinweis."}
    </p>
  </div>
  <p class="form-help">
    <strong>Geltungsbereich:</strong> Nur für künftig neu angelegte Rechnungen. Bestehende Entwürfe und
    ausgestellte Rechnungen bleiben unverändert. Abweichende Kundenvorgaben haben Vorrang. Auf jeder Rechnung
    kannst du die Behandlung ändern.
  </p>
</section>

<style>
  .company-tax-settings > .position-tax-toggle {
    display: flex;
    align-items: center;
    gap: 10px;
    margin-bottom: 14px;
  }
  .position-tax-toggle input {
    width: 16px;
    height: 16px;
    min-height: 0;
    margin: 0;
    padding: 0;
    flex: 0 0 auto;
  }
  .tax-muted {
    color: var(--theme-muted);
  }
  .tax-muted select {
    background: var(--theme-surface-muted);
    color: var(--theme-muted);
    cursor: not-allowed;
  }
  .tax-preview {
    margin: 16px 0;
    padding: 14px;
    border: 1px solid var(--theme-border);
    border-radius: 6px;
    background: var(--theme-surface-muted);
    color: var(--theme-text);
    font-size: 13px;
    overflow-wrap: anywhere;
  }
  .tax-preview p {
    margin: 8px 0 0;
  }
  input:not([type="checkbox"]) {
    width: 100%;
    min-width: 0;
    padding: 9px 10px;
    border: 1px solid var(--theme-input-border);
    border-radius: 5px;
    background: var(--theme-input);
    color: var(--theme-text);
  }
</style>
