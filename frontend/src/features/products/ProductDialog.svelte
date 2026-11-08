<script lang="ts">
  import { onDestroy, untrack } from "svelte";
  import type { Product } from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/model/models";
  import FormDialog from "../../components/FormDialog.svelte";
  import Icon from "../../components/Icon.svelte";
  import { invoiceUnits } from "../../lib/invoiceDraft";
  import { typedPrice } from "../../lib/priceInput";
  let {
    product = $bindable(),
    busy,
    error,
    onsave,
    ondirtychange,
    ondelete,
    onclose,
  }: {
    product: Product;
    busy: boolean;
    error: string;
    onsave: () => void;
    ondirtychange: (dirty: boolean) => void;
    ondelete: () => void;
    onclose: () => void;
  } = $props();
  const initial = untrack(() => JSON.stringify(product));
  $effect(() => ondirtychange(JSON.stringify(product) !== initial));
  onDestroy(() => ondirtychange(false));
</script>

<FormDialog
  title={product.id ? "Artikel bearbeiten" : "Neuer Artikel"}
  description="Gespeicherte Artikel und Leistungen kannst du später zu einer Rechnung hinzufügen."
  {busy}
  {error}
  {onclose}
  onsubmit={onsave}
>
  <div class="form-grid cols-2">
    <label>Bezeichnung<input required bind:value={product.name} /></label><label
      >Artikelnummer (optional)<input bind:value={product.articleNumber} /></label
    ><label>Beschreibung<input bind:value={product.description} /></label><label
      >Einzelpreis netto (€)<input
        placeholder="z. B. 100,00"
        bind:value={product.unitPrice}
        onblur={() => (product.unitPrice = typedPrice(product.unitPrice))}
      /></label
    ><label
      >Einheit<select bind:value={product.unit}
        >{#each invoiceUnits as [value, label]}<option {value}>{label}</option
          >{/each}{#if !invoiceUnits.some(([value]) => value === product.unit)}<option
            value={product.unit}>{product.unit}</option
          >{/if}</select
      ></label
    ><label>USt. %<input bind:value={product.taxRate} /></label><label
      >Steuerkategorie<select bind:value={product.taxCategory}
        ><option value="S">Mit Umsatzsteuer</option><option value="Z">Nullsteuersatz (0 %)</option
        ><option value="E">Von Umsatzsteuer befreit</option><option value="AE"
          >Steuerschuld beim Empfänger</option
        ><option value="K">Innergemeinschaftliche Lieferung</option><option value="G"
          >Ausfuhrlieferung</option
        ><option value="O">Nicht umsatzsteuerpflichtig</option
        >{#if !["S", "Z", "E", "AE", "K", "G", "O"].includes(product.taxCategory)}<option
            value={product.taxCategory}>{product.taxCategory}</option
          >{/if}</select
      ></label
    >
  </div>

  {#snippet footer()}
    {#if product.id}<button type="button" class="danger-text" disabled={busy} onclick={ondelete}
        ><Icon name="trash" size={16} /> Artikel löschen</button
      >{/if}
    <span class="footer-spacer"></span><button
      type="button"
      class="secondary"
      disabled={busy}
      onclick={onclose}>Abbrechen</button
    ><button class="primary" disabled={busy}
      >{busy ? "Wird gespeichert …" : "Artikel speichern"}</button
    >
  {/snippet}
</FormDialog>
