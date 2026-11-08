<script lang="ts">
  import { onMount } from "svelte";
  import Icon from "../../components/Icon.svelte";
  import Monogram from "../../components/Monogram.svelte";
  import { dismissDialog } from "../../lib/dismissDialog";
  import { bankPresets } from "../../lib/bankPresets";

  let {
    onselect,
    onclose,
  }: {
    onselect: (id: string) => void;
    onclose: () => void;
  } = $props();
  let dialog: HTMLDialogElement;
  let search = $state("");
  const groups = [
    {
      title: "Banken mit FinTS",
      entries: bankPresets
        .filter((bank) => bank.endpoint)
        .map((bank) => ({
          id: bank.id,
          name: bank.name,
        })),
    },
    {
      title: "Weitere Banken",
      entries: bankPresets
        .filter((bank) => !bank.endpoint)
        .map((bank) => ({
          id: bank.id,
          name: bank.name,
        })),
    },
  ];
  const filteredGroups = $derived(
    groups
      .map((group) => ({
        title: group.title,
        entries: group.entries.filter((item) =>
          item.name.toLocaleLowerCase("de").includes(search.trim().toLocaleLowerCase("de")),
        ),
      }))
      .filter((group) => group.entries.length),
  );

  onMount(() => {
    dialog.showModal();
    dialog.querySelector<HTMLInputElement>("input")?.focus();
  });
</script>

<dialog
  use:dismissDialog
  bind:this={dialog}
  class="account-picker-dialog"
  aria-labelledby="account-picker-title"
  oncancel={(event) => {
    event.preventDefault();
    onclose();
  }}
>
  <div class="account-picker-content">
    <header>
      <div>
        <h2 id="account-picker-title">Anbieter auswählen</h2>
        <p>
          Banken mit FinTS kannst du direkt verbinden. Bei den übrigen liest du Kontoauszüge als
          CSV-Datei ein.
        </p>
      </div>
      <button type="button" class="picker-close" aria-label="Schließen" onclick={onclose}>
        <Icon name="close" size={18} />
      </button>
    </header>
    <div class="picker-body">
      <label class="picker-search">
        <span>Anbieter suchen</span>
        <input type="search" bind:value={search} placeholder="Name eingeben …" autocomplete="off" />
      </label>
      {#each filteredGroups as group}
        <section class="picker-group">
          <h3>{group.title}</h3>
          <div class="picker-grid">
            {#each group.entries as item}
              <button type="button" class="picker-card" onclick={() => onselect(item.id)}>
                <Monogram name={item.name} size={28} />
                <span class="picker-name">{item.name}</span>
              </button>
            {/each}
          </div>
        </section>
      {/each}
      {#if !filteredGroups.length}
        <p class="picker-empty">Kein Anbieter gefunden.</p>
      {/if}
    </div>
    <footer>
      <button type="button" class="picker-manual" onclick={() => onselect("")}
        >Andere Bank manuell einrichten</button
      >
    </footer>
  </div>
</dialog>

<style>
  .account-picker-dialog {
    width: min(760px, calc(100vw - 32px));
    max-height: calc(100dvh - 32px);
    padding: 0;
    border: 1px solid var(--theme-border);
    border-radius: 12px;
    color: var(--theme-text);
    background: var(--theme-surface);
  }
  .account-picker-dialog::backdrop {
    background: var(--theme-backdrop);
  }
  .account-picker-content {
    display: flex;
    flex-direction: column;
    max-height: calc(100dvh - 34px);
  }
  header,
  footer {
    padding: 16px 20px;
    flex: 0 0 auto;
  }
  header {
    display: flex;
    justify-content: space-between;
    gap: 16px;
    border-bottom: 1px solid var(--theme-border);
  }
  header > div {
    min-width: 0;
  }
  header h2 {
    margin: 0;
    font-size: 18px;
  }
  header p {
    margin: 4px 0 0;
    font-size: 12px;
    color: var(--theme-muted);
  }
  .picker-close {
    display: grid;
    place-items: center;
    width: 32px;
    height: 32px;
    border: 0;
    border-radius: 6px;
    background: transparent;
    color: var(--theme-text);
    cursor: pointer;
  }
  .picker-body {
    padding: 18px 20px;
    overflow-y: auto;
    overscroll-behavior: contain;
    min-height: 0;
  }
  .picker-search {
    display: grid;
    gap: 6px;
    font-size: 12px;
    font-weight: 600;
  }
  .picker-search input {
    width: 100%;
    min-height: 38px;
    padding: 8px 11px;
    border: 1px solid var(--theme-border);
    border-radius: 6px;
    background: var(--theme-surface);
    color: var(--theme-text);
    font: inherit;
  }
  .picker-search input:focus {
    outline: 2px solid var(--theme-primary);
    outline-offset: 1px;
  }
  .picker-group {
    margin-top: 16px;
  }
  .picker-group h3 {
    margin: 0 0 8px;
    font-size: 13px;
  }
  .picker-grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(min(230px, 100%), 1fr));
    gap: 6px;
  }
  .picker-card {
    display: flex;
    align-items: center;
    gap: 8px;
    min-width: 0;
    min-height: 40px;
    padding: 5px 10px 5px 6px;
    border: 1px solid var(--theme-border);
    border-radius: 8px;
    background: var(--theme-surface);
    color: var(--theme-text);
    text-align: left;
    cursor: pointer;
  }
  .picker-card > :global(.monogram) {
    margin-right: 2px;
  }
  .picker-card:hover,
  .picker-card:focus-visible {
    border-color: var(--theme-primary);
    outline: 2px solid var(--theme-primary);
    outline-offset: 1px;
  }
  .picker-name {
    font-size: 13px;
    font-weight: 650;
    overflow-wrap: anywhere;
  }
  .picker-empty {
    margin: 26px 0;
    text-align: center;
    color: var(--theme-muted);
  }
  footer {
    border-top: 1px solid var(--theme-border);
    background: var(--theme-surface-muted);
  }
  .picker-manual {
    padding: 8px 10px;
    border: 0;
    background: transparent;
    color: var(--theme-primary);
    cursor: pointer;
    font: inherit;
  }
  @media (max-width: 520px) {
    .picker-grid {
      grid-template-columns: minmax(0, 1fr);
    }
  }
</style>
