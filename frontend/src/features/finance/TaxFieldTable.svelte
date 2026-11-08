<script lang="ts">
  import { Clipboard } from "@wailsio/runtime";
  import type { Field } from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/taxforms/models";
  import { amountLabel, dateLabel } from "../../lib/format";
  import { elsterValue } from "../../lib/expenses";

  let {
    fields,
    caption,
    copyable = true,
  }: { fields: Field[] | null | undefined; caption: string; copyable?: boolean } = $props();
  let copied = $state("");

  async function copy(field: Field) {
    const text = elsterValue(field.amount, field.cents);
    try {
      await Clipboard.SetText(text);
    } catch {
      await navigator.clipboard?.writeText(text);
    }
    copied = field.zeile + field.kz;
    setTimeout(() => {
      if (copied === field.zeile + field.kz) copied = "";
    }, 1500);
  }
</script>

<table class="tax-fields">
  <caption class="tax-hidden-caption">{caption}</caption>
  <thead
    ><tr><th>Zeile</th><th>Kennzahl</th><th>Was es ist</th><th class="num">Wert</th><th></th></tr
    ></thead
  >
  <tbody>
    {#each fields ?? [] as f (f.zeile + f.kz)}
      <tr class:info={f.info}>
        <td>{f.zeile}</td>
        <td><strong>{f.kz}</strong></td>
        <td
          >{f.label}{#if f.entries?.length}<details class="link-toggle">
              <summary>{f.entries.length} {f.entries.length === 1 ? "Beleg" : "Belege"}</summary>
              <ul class="tax-entries">
                {#each f.entries as e}<li>
                    {e.date ? dateLabel(e.date) + " · " : ""}{e.label} · {amountLabel(
                      e.amount,
                    )}{e.note ? ` (${e.note})` : ""}
                  </li>{/each}
              </ul>
            </details>{/if}</td
        >
        <td class="num">{elsterValue(f.amount, f.cents)}</td>
        <td
          >{#if copyable && !f.info}<button
              class="text tax-copy"
              aria-label={`Wert für Kennzahl ${f.kz} kopieren`}
              onclick={() => copy(f)}>{copied === f.zeile + f.kz ? "Kopiert" : "Kopieren"}</button
            >{/if}</td
        >
      </tr>
    {/each}
  </tbody>
</table>
