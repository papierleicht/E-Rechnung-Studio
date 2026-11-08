<script lang="ts">
  import { dismissDialog } from "../../lib/dismissDialog";
  import { onDestroy, onMount, tick, untrack } from "svelte";
  import type {
    InvoiceLine,
    Product,
  } from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/model/models";
  import Icon from "../../components/Icon.svelte";
  import { newLine, isBlankLine, invoiceUnits } from "../../lib/invoiceDraft";
  import { taxChoices, taxReasons } from "../../lib/tax";
  import { grossUnitPrice, netUnitPrice, roundPrice, typedPrice } from "../../lib/priceInput";

  let {
    lines,
    quote = false,
    products,
    standardTax,
    taxTreatment = "standard",
    mixedTax = false,
    defaultLineTax = "standard",
    signedQuantities = false,
    initialIndex = 0,
    onsave,
    ondirtychange,
    oncancel,
  }: {
    lines: InvoiceLine[];
    quote?: boolean;
    products: Product[];
    standardTax: boolean;
    taxTreatment?: string;
    mixedTax?: boolean;
    defaultLineTax?: string;
    signedQuantities?: boolean;
    initialIndex?: number;
    onsave: (lines: InvoiceLine[]) => void;
    ondirtychange: (dirty: boolean) => void;
    oncancel: () => void;
  } = $props();

  let dialog: HTMLDialogElement;
  let working: InvoiceLine[] = $state([]);
  let query = $state("");
  let catalog = $state(false);
  let submissionError = $state("");
  const initial = untrack(() => JSON.stringify(lines));
  let ready = $state(false);
  const matches = $derived(
    products.filter((product) =>
      (product.name + " " + product.description)
        .toLocaleLowerCase("de")
        .includes(query.trim().toLocaleLowerCase("de")),
    ),
  );
  async function focusLine(index: number) {
    await tick();
    dialog.querySelector<HTMLInputElement>(`[data-line-index="${index}"] input`)?.focus();
  }
  async function addFree() {
    let index = working.findIndex(isBlankLine);
    if (index < 0) {
      working = [
        ...working,
        { ...newLine(), ...(mixedTax ? { taxTreatment: defaultLineTax } : {}) },
      ];
      index = working.length - 1;
    }
    catalog = false;
    await focusLine(index);
  }
  async function useProduct(product: Product) {
    const line: InvoiceLine = {
      id: crypto.randomUUID(),
      productId: product.id,
      articleNumber: product.articleNumber,
      name: product.name,
      description: product.description,
      quantity: "1",
      unit: product.unit,
      unitPrice: product.unitPrice,
      taxRate: product.taxRate,
      taxCategory: product.taxCategory,
      ...(mixedTax ? { taxTreatment: defaultLineTax } : {}),
    };
    let index = working.findIndex(isBlankLine);
    if (index >= 0) {
      working[index] = line;
      working = [...working];
    } else {
      working = [...working, line];
      index = working.length - 1;
    }
    catalog = false;
    query = "";
    await focusLine(index);
  }
  function removeLine(index: number) {
    if (working.length <= 1) return;
    working = working.filter((_, i) => i !== index);
  }
  // Keep the user's decimal input intact while typing; invalidate it when net or tax changes.
  let grossInputs: Record<string, { value: string; net: string; rate: string }> = $state({});
  let netInputs: Record<string, string> = $state({});
  function netPrice(line: InvoiceLine) {
    return netInputs[line.id] === line.unitPrice
      ? line.unitPrice
      : roundPrice(line.unitPrice) || line.unitPrice;
  }
  function setNetPrice(line: InvoiceLine, value: string) {
    line.unitPrice = value;
    netInputs[line.id] = value;
    delete grossInputs[line.id];
  }
  function finishNetPrice(line: InvoiceLine) {
    // Merely focusing the rounded display must not overwrite a precise converted price.
    if (netInputs[line.id] === line.unitPrice) {
      const rounded = roundPrice(typedPrice(line.unitPrice));
      if (rounded) line.unitPrice = rounded;
    }
    delete netInputs[line.id];
  }
  function finishGrossPrice(line: InvoiceLine) {
    const entry = grossInputs[line.id];
    if (entry && entry.net === line.unitPrice && entry.rate === effectiveRate(line)) {
      const rounded = roundPrice(typedPrice(entry.value));
      if (rounded) setGrossPrice(line, rounded);
    }
  }
  function effectiveRate(line: InvoiceLine) {
    const treatment = mixedTax ? line.taxTreatment || "standard" : taxTreatment;
    return treatment === "standard" ? line.taxRate : "0";
  }
  function grossPrice(line: InvoiceLine) {
    const entry = grossInputs[line.id];
    const rate = effectiveRate(line);
    return entry && entry.net === line.unitPrice && entry.rate === rate
      ? entry.value
      : grossUnitPrice(line.unitPrice, rate);
  }
  function setGrossPrice(line: InvoiceLine, value: string) {
    const rate = effectiveRate(line);
    delete netInputs[line.id];
    line.unitPrice = netUnitPrice(value, rate);
    grossInputs[line.id] = { value, net: line.unitPrice, rate };
  }
  async function submit() {
    const invalid = working.findIndex((line) => !line.name.trim());
    if (!working.length || invalid >= 0) {
      submissionError = `Gib für Position ${Math.max(0, invalid) + 1} eine Bezeichnung ein.`;
      if (invalid >= 0) await focusLine(invalid);
      return;
    }
    submissionError = "";
    working.forEach((line) => {
      finishNetPrice(line);
      finishGrossPrice(line);
    });
    onsave(working);
  }

  onMount(async () => {
    working = lines.map((line) => ({
      ...line,
      ...(mixedTax && !line.taxTreatment ? { taxTreatment: defaultLineTax } : {}),
    }));
    ready = true;
    dialog.showModal();
    await focusLine(initialIndex);
  });
  $effect(() => {
    if (ready) ondirtychange(JSON.stringify(working) !== initial);
  });
  onDestroy(() => ondirtychange(false));
</script>

<dialog
  use:dismissDialog
  aria-labelledby="position-title"
  bind:this={dialog}
  class="position-dialog"
  oncancel={(event) => {
    event.preventDefault();
    oncancel();
  }}
>
  <form
    class="modal position-editor"
    onsubmit={(event) => {
      event.preventDefault();
      submit();
    }}
    aria-labelledby="position-title"
  >
    <header>
      <div>
        <h2 id="position-title">Positionen bearbeiten</h2>
        <p class="heading-subtitle">{quote ? "Angebotsinhalt" : "Rechnungsinhalt"}</p>
        <p>
          Erfasse Leistungen, Mengen und Preise an einem Ort. Netto- und Bruttopreise gelten pro
          Einheit. Nenne in Bezeichnung und Beschreibung Art, Umfang und Inhalt der Leistung, zum
          Beispiel „Webdesign: Relaunch Startseite, 12 Std.“. Bei E-Rechnungen muss die Leistung
          schon in den strukturierten Daten genau genug stehen.
        </p>
        <p>
          Gibst du einen Bruttopreis ein, rechnet die App daraus den Nettopreis. Die E-Rechnung
          rechnet vom Nettobetrag aus, deshalb kann der Gesamtbetrag um wenige Cent vom Bruttopreis
          abweichen. Prüfe ihn in der Vorschau.
        </p>
      </div>
      <button
        type="button"
        aria-label="Schließen"
        class="dialog-close"
        title="Schließen (Esc)"
        onclick={oncancel}><Icon name="close" /></button
      >
    </header>

    <div class="position-editor-toolbar">
      <button class="secondary" type="button" onclick={addFree}
        ><Icon name="plus" size={16} /> Position hinzufügen</button
      >
      {#if products.length}<button
          class="secondary"
          class:active={catalog}
          type="button"
          onclick={() => (catalog = !catalog)}
          ><Icon name="products" size={16} /> Aus Artikeln wählen</button
        >{/if}
      <span>{working.length} {working.length === 1 ? "Position" : "Positionen"}</span>
    </div>

    {#if catalog}
      <section class="position-catalog" aria-label="Gespeicherte Artikel">
        <input
          aria-label="Artikel suchen"
          placeholder="Artikel oder Leistung suchen"
          bind:value={query}
        />
        <div class="catalog-results">
          {#each matches as product}
            <button type="button" onclick={() => useProduct(product)}
              ><span class="catalog-text"
                ><strong title={product.name}>{product.name}</strong>{#if product.description}<small
                    title={product.description}>{product.description}</small
                  >{/if}</span
              ><span class="catalog-price">{product.unitPrice.replace(".", ",")} €</span></button
            >
          {/each}
          {#if !matches.length}<p>Keine passenden Artikel.</p>{/if}
        </div>
      </section>
    {/if}

    <div class="position-table-wrap" oninput={() => (submissionError = "")}>
      {#if signedQuantities}<p class="form-help">
          Rechnungskorrektur: negative Mengen mindern den Betrag. Einzelpreise bleiben positiv.
        </p>{/if}
      <table class="position-edit-table" class:per-position-tax={mixedTax}>
        <thead
          ><tr
            ><th>Pos.</th><th>Bezeichnung</th><th>Menge</th><th>Einheit</th><th
              >{standardTax || mixedTax ? "Preis netto" : "Verkaufspreis"}</th
            ><th>{mixedTax ? "Steuerfall / USt." : "USt."}</th><th>Preis brutto</th><th
              aria-label="Aktion"
            ></th></tr
          ></thead
        >
        <tbody>
          {#each working as line, index (line.id)}
            <tr data-line-index={index}>
              <td class="position-number">{index + 1}</td>
              <td class="position-description"
                ><input
                  required
                  aria-label={`Bezeichnung Position ${index + 1}`}
                  bind:value={line.name}
                  placeholder="Leistung oder Artikel"
                /><textarea
                  aria-label={`Beschreibung Position ${index + 1}`}
                  bind:value={line.description}
                  placeholder="Beschreibung (optional)"></textarea>
                {#if (mixedTax ? line.taxTreatment || "standard" : taxTreatment).startsWith("margin-")}
                  <p class="form-help">
                    {taxReasons[mixedTax ? line.taxTreatment! : taxTreatment]}
                  </p>
                {/if}
                {#if mixedTax && line.taxTreatment && !["standard", "zero-rated", "margin-goods", "margin-art", "margin-collectibles"].includes(line.taxTreatment)}
                  <div class="position-tax-reason">
                    <label
                      >Befreiungsgrund<input
                        data-field={`lines.${index}.exemptionReason`}
                        required={line.taxTreatment === "exempt" && !line.exemptionCode}
                        bind:value={line.exemptionReason}
                        placeholder="Standardhinweis automatisch verwenden"
                      /></label
                    >
                    {#if line.taxTreatment === "exempt"}<label
                        >VATEX-Code (optional)<input
                          data-field={`lines.${index}.exemptionCode`}
                          bind:value={line.exemptionCode}
                          placeholder="z. B. VATEX-EU-132"
                        /></label
                      >{/if}
                  </div>
                {/if}</td
              >
              <td
                ><input
                  required
                  aria-label={`Menge Position ${index + 1}`}
                  inputmode="decimal"
                  bind:value={line.quantity}
                /></td
              >
              <td
                ><select aria-label={`Einheit Position ${index + 1}`} bind:value={line.unit}
                  >{#each invoiceUnits as unit}<option value={unit[0]}>{unit[1]}</option
                    >{/each}{#if !invoiceUnits.some((unit) => unit[0] === line.unit)}<option
                      value={line.unit}>{line.unit}</option
                    >{/if}</select
                ></td
              >
              <td data-label={standardTax || mixedTax ? "Preis netto" : "Verkaufspreis"}
                ><span class="input-suffix"
                  ><input
                    required
                    aria-label={`Nettopreis Position ${index + 1}`}
                    inputmode="decimal"
                    bind:value={() => netPrice(line), (value) => setNetPrice(line, value)}
                    onblur={() => finishNetPrice(line)}
                  /><span>€</span></span
                ></td
              >
              <td class="position-tax-cell">
                {#if mixedTax}<select
                    class="position-tax-select"
                    aria-label={`Steuerfall Position ${index + 1}`}
                    title={taxChoices.find(
                      (choice) => choice.id === (line.taxTreatment || "standard"),
                    )?.label}
                    data-field={`lines.${index}.taxTreatment`}
                    value={line.taxTreatment || "standard"}
                    onchange={(event) => {
                      line.taxTreatment = event.currentTarget.value;
                      line.taxCategory = "S";
                      line.exemptionCode = "";
                      line.exemptionReason = "";
                    }}
                    >{#each taxChoices as choice}<option value={choice.id}>{choice.label}</option
                      >{/each}</select
                  >{/if}
                {#if standardTax || (mixedTax && (!line.taxTreatment || line.taxTreatment === "standard"))}<span
                    class="input-suffix"
                    ><input
                      required
                      aria-label={`Umsatzsteuer Position ${index + 1}`}
                      inputmode="decimal"
                      bind:value={line.taxRate}
                    /><span>%</span></span
                  >{:else}<span class="fixed-tax"
                    >{taxTreatment.startsWith("margin-") ||
                    (mixedTax && line.taxTreatment?.startsWith("margin-"))
                      ? "—"
                      : (mixedTax ? line.taxTreatment : taxTreatment) === "zero-rated"
                        ? "0 %"
                        : "Nicht ausgewiesen"}</span
                  >{/if}</td
              >
              <td class="position-amount">
                <span class="input-suffix"
                  ><input
                    required
                    aria-label={`Bruttopreis Position ${index + 1}`}
                    inputmode="decimal"
                    pattern="[0-9]+([.,][0-9]+)?"
                    title="Bruttopreis pro Einheit, z. B. 119,00"
                    bind:value={() => grossPrice(line), (value) => setGrossPrice(line, value)}
                    onblur={() => finishGrossPrice(line)}
                  /><span>€</span></span
                >
              </td>
              <td
                ><button
                  class="remove-position"
                  type="button"
                  aria-label={`Position ${index + 1} entfernen`}
                  title="Position entfernen"
                  disabled={working.length <= 1}
                  onclick={() => removeLine(index)}><Icon name="trash" size={16} /></button
                ></td
              >
            </tr>
          {/each}
        </tbody>
      </table>
    </div>

    {#if submissionError}<p class="dialog-error position-error" role="alert">
        {submissionError}
      </p>{/if}

    <footer>
      <span class="footer-spacer"></span><button class="secondary" type="button" onclick={oncancel}
        >Abbrechen</button
      ><button class="primary">Positionen übernehmen</button>
    </footer>
  </form>
</dialog>

<style>
  /* Saved articles with long names or descriptions stay two lines each
     (full text on hover); the price never breaks. In a low window the list
     gets shorter so the dialog footer stays visible. */
  .catalog-results {
    max-height: min(170px, 22vh);
    overscroll-behavior: contain;
  }
  .catalog-text {
    min-width: 0;
  }
  .catalog-text strong,
  .catalog-text small {
    display: -webkit-box;
    -webkit-box-orient: vertical;
    -webkit-line-clamp: 2;
    line-clamp: 2;
    overflow: hidden;
    overflow-wrap: anywhere;
  }
  .catalog-price {
    flex: none;
    white-space: nowrap;
    font-variant-numeric: tabular-nums;
  }
</style>
