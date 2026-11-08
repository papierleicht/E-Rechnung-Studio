<script lang="ts">
  import SettingSection from "../../components/SettingSection.svelte";
  import { onDestroy, untrack } from "svelte";
  import type { Customer } from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/model/models";
  import FormDialog from "../../components/FormDialog.svelte";
  import ElectronicAddress from "../../components/ElectronicAddress.svelte";
  import Icon from "../../components/Icon.svelte";
  let {
    customer = $bindable(),
    busy,
    error,
    onsave,
    ondirtychange,
    ondelete,
    onclose,
  }: {
    customer: Customer;
    busy: boolean;
    error: string;
    onsave: () => void;
    ondirtychange: (dirty: boolean) => void;
    ondelete: () => void;
    onclose: () => void;
  } = $props();
  const initial = untrack(() => JSON.stringify(customer));
  $effect(() => ondirtychange(JSON.stringify(customer) !== initial));
  onDestroy(() => ondirtychange(false));
</script>

<FormDialog
  title={customer.id ? "Kundendaten bearbeiten" : "Neuer Kunde"}
  description={customer.id
    ? "Änderungen gelten für neue Rechnungen. Bereits ausgestellte Rechnungen bleiben unverändert."
    : "Speichere einen Kunden, um seine Daten bei neuen Rechnungen wiederzuverwenden."}
  {busy}
  {error}
  {onclose}
  onsubmit={onsave}
>
  <div class="form-grid cols-2">
    <label>Name<input required bind:value={customer.name} /></label>
    <label>Ansprechpartner<input bind:value={customer.contactName} /></label>
    <label>E-Mail<input type="email" bind:value={customer.email} /></label>
    <label>USt-IdNr.<input bind:value={customer.vatId} /></label>
    <label>Straße und Hausnummer<input bind:value={customer.address.street} /></label>
    <label>Adresszusatz<input bind:value={customer.address.additional} /></label>
    <label>Postleitzahl<input bind:value={customer.address.postcode} /></label>
    <label>Ort<input bind:value={customer.address.city} /></label>
    <label>Land (Kürzel)<input maxlength="2" bind:value={customer.address.country} /></label>
    <label
      >Zahlungsziel in Tagen
      <input
        type="number"
        min="0"
        max="3650"
        step="1"
        placeholder="14"
        bind:value={customer.paymentTermDays}
      />
      <small>Ohne Angabe: 14 Tage. Gilt für neue Rechnungen.</small>
    </label>
  </div>
  <SettingSection
    title="Angaben für XRechnung"
    summary={customer.buyerReference || customer.endpoint
      ? [
          customer.buyerReference && `Leitweg-ID ${customer.buyerReference}`,
          customer.endpoint && `Adresse ${customer.endpoint}`,
        ]
          .filter(Boolean)
          .join(" · ")
      : "Leitweg-ID und elektronische Adresse, nur für Behörden nötig"}
  >
    <div class="electronic-settings">
      <label>Käuferreferenz / Leitweg-ID<input bind:value={customer.buyerReference} /></label
      ><ElectronicAddress
        bind:endpoint={customer.endpoint}
        bind:scheme={customer.endpointScheme}
        email={customer.email}
      />
    </div>
  </SettingSection>

  {#snippet footer()}
    {#if customer.id}<button type="button" class="danger-text" disabled={busy} onclick={ondelete}
        ><Icon name="trash" size={16} /> Kunde löschen</button
      >{/if}
    <span class="footer-spacer"></span><button
      type="button"
      class="secondary"
      disabled={busy}
      onclick={onclose}>Abbrechen</button
    ><button class="primary" disabled={busy}
      >{busy ? "Wird gespeichert …" : "Kunde speichern"}</button
    >
  {/snippet}
</FormDialog>
