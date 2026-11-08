<script lang="ts">
  import type { Company } from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/model/models";
  import { chooseCompanyTaxStatus } from "../../lib/tax";
  let { company = $bindable() }: { company: Company } = $props();
</script>

<section class="company-tax-settings" aria-label="Umsatzsteuer im Unternehmensprofil">
  <h3>Umsatzsteuer</h3>
  <label class="small-business-toggle">
    <input
      type="checkbox"
      data-field="company.taxStatusUnclear"
      checked={company.smallBusiness}
      onchange={(event) =>
        chooseCompanyTaxStatus(
          company,
          event.currentTarget.checked ? "small-business" : "standard",
        )}
    />
    <span>Ich nutze die Kleinunternehmerregelung (§ 19 UStG).</span>
  </label>
  <p class="form-help">
    Das beschreibt deinen Unternehmensstatus. Steuerbefreiungen und andere Besonderheiten einzelner
    Leistungen legst du auf der Rechnung fest. Optionale Voreinstellungen findest du unter
    „Rechnungsvorgaben“.
  </p>
  {#if company.taxStatusUnclear}<p class="form-help">
      Du kannst die Einrichtung fortsetzen und Entwürfe speichern. Bevor du eine Rechnung
      ausstellst, musst du diesen Status klären.
      <button
        type="button"
        class="text"
        onclick={() =>
          chooseCompanyTaxStatus(company, company.smallBusiness ? "small-business" : "standard")}
        >Aktuelle Auswahl bestätigen</button
      >
    </p>{/if}
</section>

<style>
  .company-tax-settings > .small-business-toggle {
    display: flex;
    align-items: center;
    gap: 10px;
  }
  .small-business-toggle input {
    width: 16px;
    height: 16px;
    min-height: 0;
    flex: 0 0 auto;
    margin: 0;
  }
</style>
