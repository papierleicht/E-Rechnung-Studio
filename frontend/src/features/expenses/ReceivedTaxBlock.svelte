<script lang="ts">
  import * as API from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/app/invoiceservice";
  import type {
    ImportInspection,
    ReceivedInvoice,
  } from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/model/models";
  import type { Expense } from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/store/models";
  import type { Category } from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/taxforms/models";
  import ExpenseDialog from "./ExpenseDialog.svelte";
  import InputTaxDecision from "./InputTaxDecision.svelte";
  import InputTaxNotice from "../../components/InputTaxNotice.svelte";
  import { serviceErrorMessage } from "../../lib/errors";
  import { amountLabel } from "../../lib/format";
  import { assetTreatment, usefulLives } from "../../lib/expenses";

  let {
    item,
    linked = $bindable(null),
    onexpense = () => {},
    ondecision = () => {},
  }: {
    item: ReceivedInvoice;
    /** The expense captured for this receipt, if any. */
    linked?: Expense | null;
    /** Called after amounts were captured for a PDF receipt. */
    onexpense?: () => void;
    /** Called after the user decided about the Vorsteuer. */
    ondecision?: (inspection: ImportInspection) => void;
  } = $props();

  let categories = $state<Category[]>([]);
  let smallBusiness = $state(false);
  let category = $state("");
  let years = $state(0);
  let capturing = $state(false);
  let error = $state("");
  let saved = $state(false);
  let pending = $state(false);

  // E-invoices carry amounts; plain PDFs need them typed in once.
  const structured = $derived(!!item.inspection.totals?.gross && !!item.inspection.issueDate);
  // Only a document with Vorsteuer leaves something to decide, and only for those who deduct it.
  const decidable = $derived(
    structured && !smallBusiness && !linked && Number(item.inspection.totals.tax) > 0,
  );
  // Small businesses depreciate the gross price, others the net price.
  const cost = $derived(
    Number((smallBusiness ? item.inspection.totals?.gross : item.inspection.totals?.net) ?? 0),
  );

  async function load(id: string) {
    error = "";
    saved = false;
    pending = false;
    try {
      const [cats, assigned, expenses, setup] = await Promise.all([
        API.ExpenseCategories(),
        API.ReceivedCategories(),
        API.Expenses(),
        API.TaxSetup(),
      ]);
      categories = cats ?? [];
      smallBusiness = setup.smallBusiness;
      const current = assigned?.[id];
      category = current?.category ?? "";
      years = current?.years || 0;
      linked = (expenses ?? []).find((e) => e.receivedId === id) ?? null;
    } catch (cause) {
      error = serviceErrorMessage(cause, "Die Einordnung konnte nicht geladen werden.");
    }
  }
  $effect(() => {
    void load(item.id);
  });

  async function saveCategory() {
    if (!category) return;
    error = "";
    saved = false;
    // A purchase counts only once its useful life is chosen.
    pending = category === "asset" && !years;
    if (pending) return;
    try {
      await API.SaveReceivedCategory({
        receivedId: item.id,
        category,
        years: category === "asset" ? years : 0,
      });
      saved = true;
    } catch (cause) {
      error = serviceErrorMessage(cause, "Die Kategorie konnte nicht gespeichert werden.");
    }
  }
  const hint = $derived(categories.find((c) => c.id === category)?.hint ?? "");
</script>

{#if item.status !== "misimport"}
  <section class="received-tax" aria-label="Steuerliche Einordnung">
    <h3>Für Steuer und EÜR</h3>
    {#if error}<p class="received-error" role="alert">{error}</p>{/if}
    {#if !linked && !smallBusiness}<InputTaxNotice status={item.inspection.inputTax} />{/if}
    {#if structured}
      <label
        >Wofür war die Ausgabe?<select bind:value={category} onchange={saveCategory}>
          <option value="" disabled>Auswählen …</option>
          {#each categories as c (c.id)}<option value={c.id}>{c.label}</option>{/each}
        </select></label
      >
      {#if hint}<p class="form-help">{hint}</p>{/if}
      {#if category === "asset"}<label
          >Wie lange nutzt du es?<select bind:value={years} onchange={saveCategory}>
            <option value={0} disabled>Auswählen …</option>
            {#each usefulLives as l (l.years)}<option value={l.years}>{l.label}</option>{/each}
            {#if years && !usefulLives.some((l) => l.years === years)}<option value={years}
                >{years} Jahre</option
              >{/if}
          </select></label
        >
        <p class="form-help">{assetTreatment(cost, years)}</p>{/if}
      {#if pending}<p class="form-help" role="status">
          Wähle die Nutzungsdauer. Erst dann ist die Einordnung gespeichert.
        </p>{/if}
      {#if saved}<p class="form-help" role="status">Gespeichert.</p>{/if}
      {#if linked}<p>
          Erfasste Beträge: {amountLabel(linked.gross)} bei {linked.supplier}. Du kannst die Angaben
          unter „Ausgaben“ ändern.
        </p>{:else if decidable}<InputTaxDecision {item} {ondecision} />{/if}
    {:else if linked}
      <p>
        Erfasste Beträge: {amountLabel(linked.gross)} bei {linked.supplier}. Du kannst die Angaben
        unter „Ausgaben“ ändern.
      </p>
    {:else}
      <p>
        Diese Datei enthält keine Rechnungsdaten. Erfasst du Betrag und Kategorie einmal, rechnet
        die App sie in Umsatzsteuer und EÜR ein.
      </p>
      <button class="primary" onclick={() => (capturing = true)}>Beträge erfassen</button>
    {/if}
  </section>
{/if}

{#if capturing}
  <ExpenseDialog
    initial={{
      receivedId: item.id,
      supplier: item.inspection.sellerName || item.filename.replace(/\.[^.]+$/, ""),
      date: item.inspection.issueDate || undefined,
      gross: structured ? item.inspection.totals.gross : undefined,
    }}
    {categories}
    {smallBusiness}
    onclose={() => (capturing = false)}
    onsaved={async () => {
      capturing = false;
      await load(item.id);
      onexpense();
    }}
  />
{/if}

<style>
  .received-tax {
    margin: 16px 0;
    padding: 14px 16px;
    border: 1px solid var(--theme-border);
    border-radius: 8px;
    display: grid;
    grid-template-columns: minmax(0, 1fr);
    gap: 8px;
  }
  h3 {
    margin: 0;
    font-size: 0.95rem;
  }
  .received-tax p {
    margin: 0;
    overflow-wrap: anywhere;
  }
  label {
    display: flex;
    flex-direction: column;
    gap: 4px;
  }
  .primary {
    justify-self: start;
  }
</style>
