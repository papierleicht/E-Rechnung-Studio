<script lang="ts">
  import type {
    InvoiceDraft,
    PurchaseAssignment,
  } from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/model/models";
  import { roundPrice, typedPrice } from "../../lib/priceInput";
  import { amountLabel } from "../../lib/format";
  import { marginLines } from "../../lib/marginScheme";
  import PurchaseSourceField from "./PurchaseSourceField.svelte";

  let {
    draft,
    busy,
    onsave,
  }: {
    draft: InvoiceDraft;
    busy: boolean;
    /** Saves price and document per line ID; empty values remove them. */
    onsave: (prices: Record<string, PurchaseAssignment>) => Promise<void> | void;
  } = $props();
  const lines = $derived(marginLines(draft));
  type Entry = { price: string; source: string; sourceLabel: string };
  let typed: Record<string, Entry> = $state({});
  const stored = (line: (typeof lines)[number]): Entry => ({
    price: roundPrice(line.purchasePrice ?? ""),
    source: line.purchaseSource ?? "",
    sourceLabel: line.purchaseSourceLabel ?? "",
  });
  // The stored values fill the fields again whenever the invoice reloads.
  $effect(() => {
    typed = Object.fromEntries(lines.map((line) => [line.id, stored(line)]));
  });
  const missing = $derived(lines.filter((line) => !line.purchasePrice).length);
  const changed = $derived(
    lines.some((line) => {
      const now = typed[line.id],
        was = stored(line);
      return !!now && (now.price.trim() !== was.price || now.source !== was.source);
    }),
  );
</script>

{#if lines.length}
  <section class="purchase-prices" aria-labelledby="purchase-prices-title">
    <div>
      <h3 id="purchase-prices-title">Einkaufspreise für die Differenzbesteuerung</h3>
      <p>
        {missing
          ? "Ohne Einkaufspreis fehlt die Umsatzsteuer auf deine Marge in Finanzen, Voranmeldung und EÜR."
          : "Die App rechnet die Umsatzsteuer auf deine Marge mit diesen Preisen aus."}
        Trage den Preis ein oder übernimm ihn aus dem Ankaufbeleg. Er erscheint nicht auf der Rechnung,
        Änderungen stehen im Änderungsprotokoll.
      </p>
    </div>
    <form
      onsubmit={(event) => {
        event.preventDefault();
        void onsave(
          Object.fromEntries(
            lines.map((line) => {
              const e = typed[line.id] ?? stored(line);
              return [
                line.id,
                {
                  price: typedPrice(e.price),
                  source: e.source,
                  sourceLabel: e.sourceLabel,
                },
              ];
            }),
          ),
        );
      }}
    >
      <ul>
        {#each lines as line (line.id)}
          {#if typed[line.id]}
            <li>
              <span class="purchase-line"
                >{line.name}<small>{line.quantity} × {amountLabel(line.unitPrice)}</small>
                <PurchaseSourceField
                  source={typed[line.id].source}
                  label={typed[line.id].sourceLabel}
                  quantity={line.quantity}
                  invoiceNumber={draft.number}
                  disabled={busy}
                  onchange={(choice) => {
                    typed[line.id].source = choice?.source ?? "";
                    typed[line.id].sourceLabel = choice?.label ?? "";
                    if (choice?.price) typed[line.id].price = choice.price;
                  }}
                /></span
              >
              <span class="input-suffix"
                ><input
                  aria-label={`Einkaufspreis je Einheit für ${line.name}`}
                  inputmode="decimal"
                  bind:value={typed[line.id].price}
                  placeholder="Einkaufspreis"
                  disabled={busy}
                  onblur={() => {
                    const rounded = roundPrice(typedPrice(typed[line.id].price));
                    if (rounded) typed[line.id].price = rounded;
                  }}
                /><span>€</span></span
              >
            </li>
          {/if}
        {/each}
      </ul>
      <button class="secondary" disabled={busy || !changed}>Einkaufspreise speichern</button>
    </form>
  </section>
{/if}

<style>
  .purchase-prices {
    display: grid;
    gap: 12px;
    margin: 20px 0;
    padding: 20px;
    border: 1px solid var(--theme-border);
    border-radius: 10px;
    background: var(--theme-surface);
    font-size: 13px;
    line-height: 1.5;
  }
  .purchase-prices h3,
  .purchase-prices p {
    margin: 0;
  }
  .purchase-prices h3 {
    font-size: 14px;
    font-weight: 600;
  }
  .purchase-prices form {
    display: grid;
    gap: 10px;
    justify-items: start;
  }
  .purchase-prices ul {
    display: grid;
    gap: 12px;
    width: 100%;
    margin: 0;
    padding: 0;
    list-style: none;
  }
  .purchase-prices li {
    display: flex;
    flex-wrap: wrap;
    gap: 8px 16px;
    align-items: center;
    justify-content: space-between;
  }
  .purchase-line {
    display: grid;
    flex: 1 1 220px;
    gap: 2px;
    min-width: 0;
    overflow-wrap: anywhere;
  }
  .purchase-line small {
    color: var(--theme-muted);
  }
  .purchase-prices input {
    width: 9em;
  }
</style>
