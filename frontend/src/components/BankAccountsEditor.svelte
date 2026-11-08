<script lang="ts">
  import Icon from "./Icon.svelte";
  import Monogram from "./Monogram.svelte";
  import { lookupGermanBank } from "../lib/bankLookup";
  import { bankPresetForIBAN } from "../lib/bankPresets";
  import type { Company } from "../../bindings/papierleicht.de/e-rechnung-studio/internal/model/models";
  // Primary account (company.iban/bic/bankName) plus additional accounts, shown as
  // cards with the bank logo when the IBAN is recognised.
  let { company = $bindable(), field = "company" }: { company: Company; field?: string } = $props();
  function setPrimaryIBAN(iban: string) {
    company.iban = iban;
    const bank = lookupGermanBank(iban);
    if (!bank) return;
    if (bank.bic) company.bic = bank.bic;
    company.bankName = bank.name;
  }
  function setAdditionalIBAN(index: number, iban: string) {
    const account = company.additionalAccounts![index];
    account.iban = iban;
    const bank = lookupGermanBank(iban);
    if (!bank) return;
    if (bank.bic) account.bic = bank.bic;
    if (!account.name.trim()) account.name = bank.name;
  }
  const primaryBank = $derived(bankPresetForIBAN(company.iban));
</script>

<div class="accounts">
  <article class="account-card">
    <header>
      {#if primaryBank || company.bankName}
        <Monogram name={primaryBank?.name ?? company.bankName} />
      {:else}
        <span class="placeholder" aria-hidden="true">€</span>
      {/if}
      <div>
        <strong>Hauptkonto</strong>
        <small
          >{primaryBank?.name ??
            company.bankName ??
            "Erscheint als Erstes auf deinen Rechnungen"}</small
        >
      </div>
    </header>
    <div class="form-grid cols-2">
      <label class="full-width"
        >IBAN<input
          data-field={`${field}.iban`}
          value={company.iban}
          placeholder="DE00 0000 0000 0000 0000 00"
          oninput={(event) => setPrimaryIBAN(event.currentTarget.value)}
        /></label
      >
      <label
        >Bank<input
          data-field={`${field}.bankName`}
          bind:value={company.bankName}
          placeholder="Automatisch"
        /></label
      >
      <label
        >BIC<input
          data-field={`${field}.bic`}
          bind:value={company.bic}
          placeholder="Automatisch"
        /></label
      >
    </div>
  </article>
  {#each company.additionalAccounts ?? [] as account, index}
    {@const bank = bankPresetForIBAN(account.iban)}
    <article class="account-card">
      <header>
        {#if bank}
          <Monogram name={bank.name} />
        {:else}
          <span class="placeholder" aria-hidden="true">€</span>
        {/if}
        <div>
          <strong>Weiteres Konto {index + 1}</strong>
          <small>{bank?.name ?? "Erscheint zusätzlich auf deinen Rechnungen"}</small>
        </div>
        <button
          type="button"
          class="text danger-text"
          aria-label={`Weiteres Konto ${index + 1} entfernen`}
          onclick={() =>
            (company.additionalAccounts = company.additionalAccounts?.filter(
              (_, i) => i !== index,
            ))}><Icon name="trash" size={16} /></button
        >
      </header>
      <div class="form-grid cols-2">
        <label class="full-width"
          >IBAN<input
            data-field={`${field}.additionalAccounts.${index}.iban`}
            value={account.iban}
            oninput={(event) => setAdditionalIBAN(index, event.currentTarget.value)}
          /></label
        >
        <label>Kontoinhaber / Bezeichnung<input bind:value={account.name} /></label>
        <label
          >BIC<input
            data-field={`${field}.additionalAccounts.${index}.bic`}
            bind:value={account.bic}
          /></label
        >
      </div>
    </article>
  {/each}
  <button
    type="button"
    class="secondary add-account"
    onclick={() =>
      (company.additionalAccounts = [
        ...(company.additionalAccounts ?? []),
        { iban: "", bic: "", name: "" },
      ])}><Icon name="plus" size={16} /> Weiteres Konto hinzufügen</button
  >
</div>

<style>
  .accounts {
    display: grid;
    gap: 12px;
    justify-items: start;
  }
  .account-card {
    width: 100%;
    padding: 16px;
    border: 1px solid var(--theme-border);
    border-radius: 10px;
    background: var(--theme-surface);
  }
  .account-card :global(.form-grid) {
    margin: 0;
  }
  header {
    display: flex;
    align-items: center;
    gap: 12px;
    margin-bottom: 14px;
  }
  header div {
    flex: 1;
    min-width: 0;
    display: grid;
  }
  header small {
    color: var(--theme-muted);
    font-size: 12px;
    overflow-wrap: anywhere;
  }
  .placeholder {
    flex: 0 0 40px;
    width: 40px;
    height: 40px;
    border-radius: 9px;
    display: grid;
    place-items: center;
    background: var(--theme-surface-muted);
    border: 1px solid var(--theme-border);
    color: var(--theme-muted);
    font-weight: 700;
  }
</style>
