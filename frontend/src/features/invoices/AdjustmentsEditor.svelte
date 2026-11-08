<script lang="ts">
  import SettingSection from "../../components/SettingSection.svelte";
  import type {
    InvoiceDraft,
    InvoiceAdjustment,
  } from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/model/models";
  import { taxGroups } from "../../lib/tax";
  import { typedPrice } from "../../lib/priceInput";
  import Icon from "../../components/Icon.svelte";
  let { draft = $bindable() }: { draft: InvoiceDraft } = $props();
  const groups = $derived(taxGroups(draft));
  function add(charge: boolean) {
    const group = groups[0];
    if (!group) return;
    draft.adjustments = [
      ...(draft.adjustments ?? []),
      {
        id: crypto.randomUUID(),
        charge,
        amount: "",
        reason: "",
        taxCategory: group.category,
        taxRate: group.rate,
      },
    ];
  }
  function selectGroup(adjustment: InvoiceAdjustment, key: string) {
    const group = groups.find((group) => group.key === key);
    if (group) {
      adjustment.taxCategory = group.category;
      adjustment.taxRate = group.rate;
    }
  }
</script>

<SettingSection
  title="Rabatte und Zuschläge"
  field="adjustments"
  action={draft.adjustments?.length ? "Ändern" : "Hinzufügen"}
  summary={draft.adjustments?.length
    ? draft.adjustments
        .map((a) => `${a.charge ? "Zuschlag" : "Rabatt"} ${a.amount || "0,00"} €`)
        .join(" · ")
    : "Keine. Nettobeträge für die ganze Rechnung"}
  open={Boolean(draft.adjustments?.length)}
>
  <p class="form-help">
    Nettobeträge für die gesamte Rechnung. Bei mehreren Steuersätzen legst du für jede Steuergruppe
    einen eigenen Eintrag an.
  </p>
  {#each draft.adjustments ?? [] as adjustment, index (adjustment.id)}
    <div class="adjustment-card">
      <div class="adjustment-heading">
        <strong>{adjustment.charge ? "Zuschlag" : "Rabatt"} {index + 1}</strong><button
          type="button"
          class="text danger-text"
          aria-label={`Eintrag ${index + 1} entfernen`}
          onclick={() => (draft.adjustments = draft.adjustments?.filter((_, i) => i !== index))}
          ><Icon name="trash" size={16} /></button
        >
      </div>
      <div class="form-grid cols-2">
        <label
          >Grund<input
            required
            data-field={`adjustments.${index}.reason`}
            bind:value={adjustment.reason}
            placeholder={adjustment.charge ? "z. B. Versandkosten" : "z. B. Treuerabatt"}
          /></label
        >
        <label
          >Betrag netto ({draft.currency})<input
            required
            inputmode="decimal"
            data-field={`adjustments.${index}.amount`}
            bind:value={adjustment.amount}
            onblur={() => (adjustment.amount = typedPrice(adjustment.amount))}
            placeholder="0,00"
          /></label
        >
        <label class="wide-setting"
          >Steuergruppe<select
            data-field={`adjustments.${index}.taxCategory`}
            value={`${adjustment.taxCategory}|${Number(adjustment.taxRate.replace(",", "."))}`}
            onchange={(event) => selectGroup(adjustment, event.currentTarget.value)}
          >
            {#if !groups.some((g) => g.category === adjustment.taxCategory && Number(g.rate) === Number(adjustment.taxRate.replace(",", ".")))}<option
                value={`${adjustment.taxCategory}|${Number(adjustment.taxRate.replace(",", "."))}`}
                >Steuergruppe neu auswählen</option
              >{/if}
            {#each groups as group}<option value={group.key}>{group.label}</option>{/each}
          </select></label
        >
      </div>
    </div>
  {/each}
  <div class="document-actions">
    <button type="button" class="secondary" disabled={!groups.length} onclick={() => add(false)}
      ><Icon name="plus" size={16} /> Rabatt</button
    ><button type="button" class="secondary" disabled={!groups.length} onclick={() => add(true)}
      ><Icon name="plus" size={16} /> Zuschlag</button
    >
  </div>
</SettingSection>
