<script lang="ts">
  import type { ImportInspection } from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/model/models";
  import FormDialog from "../../components/FormDialog.svelte";
  import InputTaxNotice from "../../components/InputTaxNotice.svelte";
  import ValidationReport from "../../components/ValidationReport.svelte";
  import { dateLabel } from "../../lib/format";
  import { paymentMoney } from "../../lib/paymentWorkflow";
  let { inspection, onclose }: { inspection: ImportInspection; onclose: () => void } = $props();
</script>

<FormDialog
  title="Rechnungsprüfung"
  description="Prüfergebnis der eingelesenen Originaldatei."
  {onclose}
>
  <div class="inspection-grid">
    <span>Nummer<strong>{inspection.invoiceNumber || "—"}</strong></span><span
      >Datum<strong>{inspection.issueDate ? dateLabel(inspection.issueDate) : "—"}</strong></span
    ><span>Verkäufer<strong>{inspection.sellerName || "—"}</strong></span><span
      >Käufer<strong>{inspection.buyerName || "—"}</strong></span
    ><span>Syntax<strong>{inspection.syntax || "Unbekannt"}</strong></span><span
      >Gesamt<strong
        >{inspection.totals.gross
          ? paymentMoney(inspection.totals.gross, inspection.currency || "EUR")
          : "—"}</strong
      ></span
    >
  </div>
  <ValidationReport result={inspection.validation} received />
  <InputTaxNotice status={inspection.inputTax} />
  <p>
    Geprüft werden die strukturierten Rechnungsdaten. PDF/A-Konformität, Signaturen und die
    Übereinstimmung mit dem sichtbaren PDF-Inhalt gehören nicht zur Prüfung.
  </p>
  <small class="profile-id">{inspection.profile}</small>

  {#snippet footer()}<button type="button" class="primary" onclick={onclose}>Schließen</button
    >{/snippet}
</FormDialog>

<style>
  /* Long supplier names and invoice numbers wrap inside their tile. */
  .inspection-grid {
    grid-template-columns: repeat(3, minmax(0, 1fr));
  }
  .inspection-grid span {
    min-width: 0;
  }
  .inspection-grid strong {
    overflow-wrap: anywhere;
  }
</style>
