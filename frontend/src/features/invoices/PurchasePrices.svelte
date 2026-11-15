<script lang="ts">
  import type { InvoiceDraft } from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/model/models";
  import { roundPrice, typedPrice } from "../../lib/priceInput";
  import { amountLabel } from "../../lib/format";
  import { marginLines } from "../../lib/marginScheme";

  let {
    draft,
    busy,
    onsave,
  }: {
    draft: InvoiceDraft;
    busy: boolean;
    /** Saves the purchase prices by line ID; an empty value removes one. */
    onsave: (prices: Record<string, string>) => Promise<void> | void;
  } = $props();
  const lines = $derived(marginLines(draft));
  let typed: Record<string, string> = $state({});
  // The stored prices fill the fields again whenever the invoice reloads.
  $effect(() => {
    typed = Object.fromEntries(
      lines.map((line) => [line.id, roundPrice(line.purchasePrice ?? "")]),
    );
  });
  const missing = $derived(lines.filter((line) => !line.purchasePrice).length);
  const changed = $derived(
    lines.some((line) => (typed[line.id] ?? "").trim() !== roundPrice(line.purchasePrice ?? "")),
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
        Sie erscheinen nicht auf der Rechnung, Änderungen stehen im Änderungsprotokoll.
      </p>
    </div>
    <form
      onsubmit={(event) => {
        event.preventDefault();
        void onsave(
          Object.fromEntries(
            lines.map((line) => [line.id, typedPrice(typed[line.id] ?? "").replace(",", ".")]),
          ),
        );
      }}
    >
      <ul>
        {#each lines as line (line.id)}
          <li>
            <span class="purchase-line"
              >{line.name}<small>{line.quantity} × {amountLabel(line.unitPrice)}</small></span
            >
            <span class="input-suffix"
              ><input
                aria-label={`Einkaufspreis je Einheit für ${line.name}`}
                inputmode="decimal"
                bind:value={typed[line.id]}
                placeholder="Einkaufspreis"
                disabled={busy}
                onblur={() => {
                  const rounded = roundPrice(typedPrice(typed[line.id] ?? ""));
                  if (rounded) typed[line.id] = rounded;
                }}
              /><span>€</span></span
            >
          </li>
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
    gap: 8px;
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
