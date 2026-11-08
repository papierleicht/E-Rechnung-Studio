<script lang="ts">
  import { onDestroy, untrack } from "svelte";
  import { Browser } from "@wailsio/runtime";
  import FormDialog from "../../components/FormDialog.svelte";
  import { shopImportDate } from "../../lib/shopOrders";
  import { shops, type ShopConnection } from "../../lib/shops-api";
  import { serviceErrorMessage } from "../../lib/errors";
  import {
    emptyShopCredentials,
    shopProviders,
    shopProviderByID,
    shopifyClientFields,
    wixSiteIdPattern,
  } from "../../lib/shopProviders";

  let {
    existing,
    provider: initialProvider = "",
    onclose,
    onconnected,
    onbusychange,
  }: {
    existing: ShopConnection | null;
    /** Preselects the shop system of a new connection. */
    provider?: string;
    onclose: () => void;
    onconnected: (connection: ShopConnection) => Promise<void>;
    onbusychange: (value: boolean) => void;
  } = $props();

  let connection = $state<ShopConnection>(
    untrack(() =>
      existing
        ? { ...existing }
        : {
            id: "",
            provider: shopProviderByID(initialProvider) ? initialProvider : "shopify",
            label: "",
            url: "",
            siteId: "",
            enabled: true,
            autoCreateDrafts: true,
            importFrom: shopImportDate(),
            lastSync: "",
            lastError: "",
            hasCredentials: false,
          },
    ),
  );
  let credentials = $state(emptyShopCredentials());
  let shopifyAuth = $state("token");
  const provider = $derived(shopProviderByID(connection.provider));
  const credentialFields = $derived(
    connection.provider === "shopify" && shopifyAuth === "client"
      ? shopifyClientFields
      : (provider?.fields ?? []),
  );
  let busy = $state(false);
  let error = $state("");
  let disposed = false;
  function clearCredentials() {
    credentials = emptyShopCredentials();
  }
  function changeProvider() {
    clearCredentials();
    connection.siteId = "";
    shopifyAuth = "token";
    error = "";
  }
  onDestroy(() => {
    disposed = true;
    clearCredentials();
    onbusychange(false);
  });

  async function connect() {
    if (busy) return;
    busy = true;
    onbusychange(true);
    error = "";
    try {
      const result = await shops.connect({ ...connection }, { ...credentials });
      clearCredentials();
      if (!disposed) await onconnected(result);
    } catch (cause) {
      if (!disposed)
        error = serviceErrorMessage(
          cause,
          "Die Shopverbindung ließ sich nicht herstellen. Prüf Adresse und Zugangsdaten.",
        );
    } finally {
      busy = false;
      if (!disposed) onbusychange(false);
    }
  }
  async function openSetupGuide() {
    if (!provider) return;
    try {
      await Browser.OpenURL(provider.guide);
    } catch {
      error = "Die Anleitung konnte nicht im Browser geöffnet werden.";
    }
  }
</script>

<FormDialog
  title={existing ? "Shop erneut verbinden" : "Shop verbinden"}
  description="Bestellungen automatisch importieren und Rechnungsentwürfe vorbereiten."
  {busy}
  {error}
  {onclose}
  onsubmit={connect}
>
  <div class="form-grid cols-2 shop-connection-fields">
    <label
      >Shopsystem<select
        aria-label="Shopsystem"
        bind:value={connection.provider}
        disabled={!!existing}
        onchange={changeProvider}
        >{#each shopProviders as option}<option value={option.id}>{option.label}</option
          >{/each}</select
      ></label
    >
    <label
      >Bezeichnung<input
        required
        maxlength="120"
        bind:value={connection.label}
        placeholder="Mein Onlineshop"
      /></label
    >
    <label class="full-width"
      >Shop-Adresse<input
        required
        type="url"
        pattern="https://.*"
        title="Bitte eine vollständige HTTPS-Adresse eingeben."
        placeholder={connection.provider === "shopify"
          ? "https://mein-shop.myshopify.com"
          : "https://mein-shop.de"}
        bind:value={connection.url}
        disabled={!!existing}
        autocomplete="url"
      /></label
    >
    {#if connection.provider === "wix"}
      <label class="full-width"
        >Wix-Website-ID (Site-ID)<input
          aria-label="Wix-Website-ID (Site-ID)"
          required
          type="text"
          pattern={wixSiteIdPattern}
          title="Bitte die Website-ID als UUID mit Bindestrichen eingeben."
          placeholder="00000000-0000-0000-0000-000000000000"
          bind:value={connection.siteId}
          disabled={!!existing}
          autocomplete="off"
          spellcheck="false"
        /><small class="setup-help"
          >Die Site-ID deiner Wix-Website als UUID. Verwende die ID der Website, für die dein
          API-Schlüssel freigeschaltet ist.</small
        ></label
      >
    {/if}
    {#if provider}
      <p class="setup-help full-width">{provider.setup}</p>
      {#if provider.readResources}
        <p class="setup-help full-width">
          Benötigte GET-Leserechte: {provider.readResources.join(", ")}.
        </p>
      {/if}
    {/if}
    {#if connection.provider === "shopify"}
      <label class="full-width"
        >Anmeldung<select
          aria-label="Anmeldung"
          bind:value={shopifyAuth}
          onchange={clearCredentials}
          ><option value="token">Admin-API-Zugriffstoken</option><option value="client"
            >Client-ID und Client-Secret</option
          ></select
        ></label
      >
      {#if shopifyAuth === "client"}
        <p class="setup-help full-width">
          Für Apps der eigenen Shopify-Organisation mit Client-Credentials-Zugriff.
        </p>
      {/if}
    {/if}
    {#each credentialFields as field (field.key)}
      <label class:full-width={credentialFields.length === 1}
        >{field.label}<input
          required
          type="password"
          autocomplete="off"
          spellcheck="false"
          bind:value={credentials[field.key]}
        /></label
      >
    {/each}
    <label
      >Bestellungen importieren ab<input
        type="date"
        required
        bind:value={connection.importFrom}
        disabled={!!existing}
      /></label
    >
    <label class="full-width shop-auto-drafts">
      <input type="checkbox" bind:checked={connection.autoCreateDrafts} />
      <span
        >Rechnungsentwürfe automatisch erstellen<small
          >Für geeignete Bestellungen. Prüfen und ausstellen kannst du die Rechnung anschließend wie
          gewohnt.</small
        ></span
      >
    </label>
  </div>
  <p class="setup-help credentials-note">
    Die Zugangsdaten liegen im Schlüsselbund deines Betriebssystems. Beim Verbinden prüft die App
    den Lesezugriff auf deine Bestellungen.
  </p>
  <button type="button" class="text setup-link" disabled={busy} onclick={openSetupGuide}
    >Offizielle Anleitung öffnen</button
  >
  {#snippet footer()}
    <button type="button" class="secondary" disabled={busy} onclick={onclose}>Abbrechen</button>
    <button class="primary" disabled={busy}
      >{busy ? "Verbindung wird geprüft …" : "Shop verbinden"}</button
    >
  {/snippet}
</FormDialog>

<style>
  .full-width {
    grid-column: 1 / -1;
  }
  .setup-help {
    color: var(--theme-muted);
    font-size: 12px;
    line-height: 1.55;
    margin: 0;
    overflow-wrap: anywhere;
  }
  .credentials-note {
    margin-top: 16px;
  }
  .setup-link {
    margin-top: 8px;
  }
  .shop-auto-drafts small {
    display: block;
    margin-top: 4px;
    color: var(--theme-muted);
    font-size: 12px;
  }
  @media (max-width: 600px) {
    .shop-connection-fields {
      grid-template-columns: minmax(0, 1fr);
    }
  }
</style>
