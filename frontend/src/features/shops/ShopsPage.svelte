<script lang="ts">
  import { onMount, onDestroy } from "svelte";
  import StoreIcon from "@lucide/svelte/icons/store";
  import Icon from "../../components/Icon.svelte";
  import ShopConnectionDialog from "./ShopConnectionDialog.svelte";
  import {
    shops,
    type ShopConnection,
    type ShopOrder,
    type ShopOverview,
  } from "../../lib/shops-api";
  import {
    shopDate,
    shopMoney,
    shopOrderState,
    shopOrderReviewReason,
    shopProviderLabel,
    shopPaymentLabel,
  } from "../../lib/shopOrders";
  import { serviceErrorMessage } from "../../lib/errors";
  import { cached, remember } from "../../lib/cache";

  let {
    connectProvider = "",
    onconnectopened = () => {},
    onopeninvoice,
    onchanged,
    onbusychange,
    confirmDisconnect,
  }: {
    /** Opens a new connection to this provider, e.g. the shop of the setup. */
    connectProvider?: string;
    onconnectopened?: () => void;
    onopeninvoice: (id: string) => Promise<void>;
    onchanged: () => Promise<void>;
    onbusychange: (value: boolean) => void;
    confirmDisconnect: (label: string) => Promise<boolean>;
  } = $props();
  // The last overview shows right away on a revisit while it reloads.
  const warm = cached<ShopOverview>("shops:overview");
  let overview = $state<ShopOverview>(warm ?? { connections: [], orders: [], syncing: false });
  let loading = $state(!warm);
  let busy = $state(false);
  let error = $state("");
  let notice = $state("");
  let configuring = $state(false);
  let editing = $state<ShopConnection | null>(null);
  let query = $state("");
  let filter = $state("all");
  let selectedShop = $state("");
  let disposed = false;
  let refreshing = false;
  let invoiceSnapshot = "";
  const locked = $derived(busy || loading || overview.syncing);
  const reviewCount = $derived(
    overview.orders.filter((order) => !!shopOrderReviewReason(order)).length,
  );
  const filtered = $derived(
    overview.orders.filter((order) => {
      const connection = overview.connections.find((item) => item.id === order.connectionId);
      return (
        (!selectedShop || order.connectionId === selectedShop) &&
        (filter === "all" ||
          (filter === "review"
            ? !!shopOrderReviewReason(order)
            : shopOrderState(order) === filter)) &&
        [order.number, order.customer.name, order.customer.email, connection?.label]
          .join(" ")
          .toLocaleLowerCase("de-DE")
          .includes(query.trim().toLocaleLowerCase("de-DE"))
      );
    }),
  );

  onDestroy(() => {
    disposed = true;
    onbusychange(false);
  });
  onMount(() => {
    if (connectProvider) {
      configure(null);
      onconnectopened();
    }
    void refresh().catch(showError);
    const timer = window.setInterval(() => {
      if (!busy && !configuring) void refresh().catch(showError);
    }, 15000);
    return () => window.clearInterval(timer);
  });

  function showError(cause: unknown) {
    if (!disposed)
      error = serviceErrorMessage(
        cause,
        "Die Shopdaten ließen sich nicht aktualisieren. Versuch es erneut.",
      );
  }
  async function refresh() {
    if (refreshing) return;
    refreshing = true;
    try {
      const result = await shops.overview();
      if (disposed) return;
      overview = remember("shops:overview", result);
      const snapshot = JSON.stringify(result.orders.map((order) => order.invoiceId).sort());
      if (snapshot !== invoiceSnapshot) {
        await onchanged();
        if (!disposed) invoiceSnapshot = snapshot;
      }
      if (!disposed) error = "";
    } finally {
      refreshing = false;
      if (!disposed) loading = false;
    }
  }
  async function perform(action: () => Promise<void>) {
    if (busy) return;
    busy = true;
    onbusychange(true);
    error = "";
    notice = "";
    try {
      await action();
    } catch (cause) {
      showError(cause);
    } finally {
      busy = false;
      if (!disposed) onbusychange(false);
    }
  }
  let newProvider = $state("");
  function configure(connection: ShopConnection | null = null) {
    newProvider = connection ? "" : connectProvider;
    editing = connection;
    configuring = true;
    error = "";
  }
  async function connected(connection: ShopConnection) {
    try {
      await refresh();
    } catch (cause) {
      showError(cause);
    }
    configuring = false;
    editing = null;
    notice = `${connection.label} ist verbunden. Neue Bestellungen werden automatisch abgerufen.`;
  }
  async function sync(connection: ShopConnection) {
    await perform(async () => {
      const result = await shops.sync(connection.id);
      await refresh();
      notice = `${result.imported} Bestellungen importiert · ${result.drafts} Entwürfe erstellt · ${result.review} zur Prüfung`;
    });
  }
  async function toggle(connection: ShopConnection) {
    await perform(async () => {
      await shops.setEnabled(connection.id, !connection.enabled);
      await refresh();
      notice = connection.enabled
        ? "Automatischer Abruf pausiert."
        : "Automatischer Abruf aktiviert.";
    });
  }
  async function disconnect(connection: ShopConnection) {
    if (!(await confirmDisconnect(connection.label)) || disposed) return;
    await perform(async () => {
      await shops.disconnect(connection.id);
      if (selectedShop === connection.id) selectedShop = "";
      await refresh();
      notice = "Shop getrennt. Importierte Bestellungen und Rechnungen bleiben erhalten.";
    });
  }
  async function createInvoice(order: ShopOrder) {
    await perform(async () => {
      const invoice = await shops.createInvoice(order.id);
      await refresh();
      await onopeninvoice(invoice.id);
    });
  }
  async function openInvoice(id: string) {
    await perform(async () => {
      await onchanged();
      await onopeninvoice(id);
    });
  }
  function connectionLabel(id: string) {
    return (
      overview.connections.find((connection) => connection.id === id)?.label ?? "Getrennter Shop"
    );
  }
  function orderDate(order: ShopOrder) {
    const provider = overview.connections.find(
      (connection) => connection.id === order.connectionId,
    )?.provider;
    return shopDate(order.createdAt, false, provider === "gambio" || provider === "prestashop");
  }
</script>

<header class="topbar shops-topbar">
  <div>
    <h1>Shop-Bestellungen</h1>
  </div>
  <button class="primary" disabled={locked} onclick={() => configure()}>
    <Icon name="plus" size={16} /> Shop verbinden
  </button>
</header>
<p class="page-intro">
  Bestellungen übernehmen und Rechnungsentwürfe für deine Kunden vorbereiten.
</p>

<section class="shops-workspace" aria-busy={busy || loading || overview.syncing}>
  {#if error}
    <div class="shop-feedback shop-error" role="alert">
      <p>{error}</p>
      <button class="text" disabled={busy || loading} onclick={() => refresh().catch(showError)}
        >Erneut laden</button
      >
    </div>
  {/if}
  {#if notice}<p class="shop-feedback shop-success" role="status">{notice}</p>{/if}
  {#if loading}
    <p class="shop-loading" role="status">Shopverbindungen werden geladen …</p>
  {:else}
    {#if overview.connections.length}
      <div class="shop-connections" aria-label="Verbundene Shops">
        {#each overview.connections as connection (connection.id)}
          <article class="shop-card">
            <div class="shop-card-heading">
              <div class="shop-provider-icon">
                <StoreIcon size={20} stroke-width="1.65" aria-hidden="true" />
              </div>
              <div class="shop-card-name">
                <h2>{connection.label}</h2>
                <p>{shopProviderLabel(connection.provider)}</p>
              </div>
              <span class="shop-badge" class:shop-warning={!connection.hasCredentials}>
                {!connection.hasCredentials
                  ? "Zugang benötigt"
                  : connection.enabled
                    ? "Aktiv"
                    : "Pausiert"}
              </span>
            </div>
            <p class="shop-url">{connection.url}</p>
            <dl class="shop-card-details">
              <div>
                <dt>Letzter Abruf</dt>
                <dd>{shopDate(connection.lastSync, true)}</dd>
              </div>
              <div>
                <dt>Entwürfe</dt>
                <dd>{connection.autoCreateDrafts ? "Automatisch" : "Manuell erstellen"}</dd>
              </div>
            </dl>
            {#if !connection.hasCredentials}
              <p class="shop-card-warning">
                Gib die Zugangsdaten erneut ein, zum Beispiel nach dem Wiederherstellen einer
                Sicherung.
              </p>
            {/if}
            {#if connection.lastError && connection.hasCredentials}<p class="shop-card-error">
                {connection.lastError}
              </p>{/if}
            <div class="shop-card-actions">
              {#if connection.hasCredentials}
                <button class="secondary" disabled={locked} onclick={() => sync(connection)}>
                  <Icon name="refresh" size={14} /> Abrufen
                </button>
                <button class="text" disabled={locked} onclick={() => toggle(connection)}>
                  {connection.enabled ? "Pausieren" : "Aktivieren"}
                </button>
              {:else}
                <button class="secondary" disabled={locked} onclick={() => configure(connection)}
                  >Zugang erneuern</button
                >
              {/if}
              <button
                class="text danger-text"
                disabled={locked}
                onclick={() => disconnect(connection)}>Trennen</button
              >
            </div>
          </article>
        {/each}
      </div>
      <p class="shop-automation-note">
        {overview.syncing
          ? "Bestellungen werden abgerufen …"
          : "Aktive Shops ruft die App alle fünf Minuten ab, solange sie geöffnet ist."}
        Geeignete Bestellungen bereitet sie als Entwurf vor. Prüfen und ausstellen kannst du die Rechnungen
        anschließend wie gewohnt.
      </p>
    {:else}
      <div class="shop-empty">
        <div class="shop-provider-icon">
          <StoreIcon size={28} stroke-width="1.5" aria-hidden="true" />
        </div>
        <h2>Deine Bestellungen, direkt als Rechnungsentwurf</h2>
        <p>
          Verbinde Shopify, WooCommerce, Shopware 6, Gambio, Wix oder PrestaShop. Neue Bestellungen
          übernimmt die App beim Abruf und bereitet sie für die Rechnung vor.
        </p>
        <button class="secondary" disabled={locked} onclick={() => configure()}
          >Ersten Shop verbinden</button
        >
      </div>
    {/if}

    <section class="shop-orders" aria-labelledby="shop-orders-heading">
      <div class="shop-orders-heading">
        <div>
          <h2 id="shop-orders-heading">Bestelleingang <span>{overview.orders.length}</span></h2>
          {#if reviewCount}<p>
              {reviewCount === 1
                ? "Eine Bestellung braucht"
                : `${reviewCount} Bestellungen brauchen`} deine Prüfung.
            </p>{/if}
        </div>
        <button class="text" disabled={locked} onclick={() => refresh().catch(showError)}
          ><Icon name="refresh" size={14} /> Aktualisieren</button
        >
      </div>
      <div class="shop-order-filters">
        <input
          class="invoice-search shop-search"
          type="search"
          aria-label="Bestellungen durchsuchen"
          placeholder="Bestellung oder Kunde suchen"
          bind:value={query}
        />
        <label class="shop-select"
          ><span>Status</span><select bind:value={filter}
            ><option value="all">Alle Bestellungen</option><option value="ready"
              >Bereit für Entwurf</option
            ><option value="review">Prüfung nötig</option><option value="invoice"
              >Mit Rechnung</option
            ></select
          ></label
        >
        {#if overview.connections.length > 1}
          <label class="shop-select"
            ><span>Shop</span><select bind:value={selectedShop}
              ><option value="">Alle Shops</option>{#each overview.connections as connection}<option
                  value={connection.id}>{connection.label}</option
                >{/each}</select
            ></label
          >
        {/if}
      </div>
      {#if filtered.length}
        <!-- Keyboard users need to reach the horizontally scrolling table. -->
        <!-- svelte-ignore a11y_no_noninteractive_tabindex -->
        <div
          class="shop-table-wrap"
          tabindex="0"
          role="region"
          aria-label="Importierte Bestellungen"
        >
          <table class="shop-table">
            <thead
              ><tr
                ><th>Bestellung</th><th>Kunde</th><th class="shop-amount">Gesamt</th><th>Status</th
                ><th><span class="shop-sr-only">Aktion</span></th></tr
              ></thead
            >
            <tbody>
              {#each filtered as order (order.id)}
                {@const reviewReason = shopOrderReviewReason(order)}
                <tr>
                  <td
                    ><strong>{order.number || order.externalId}</strong><small
                      >{orderDate(order)} · {connectionLabel(order.connectionId)}</small
                    ></td
                  >
                  <td
                    ><strong>{order.customer.name || "Kundendaten fehlen"}</strong><small
                      >{order.customer.email}</small
                    ></td
                  >
                  <td class="shop-amount"
                    ><strong>{shopMoney(order.gross, order.currency)}</strong><small
                      >{shopPaymentLabel(order)}</small
                    ></td
                  >
                  <td class="shop-order-status">
                    <span class="shop-badge" class:shop-warning={!!reviewReason}
                      >{reviewReason
                        ? "Prüfung nötig"
                        : order.invoiceId
                          ? "Rechnung verknüpft"
                          : "Bereit für Entwurf"}</span
                    >
                    {#if reviewReason}<small class="shop-review-reason">{reviewReason}</small>{/if}
                  </td>
                  <td class="shop-order-action">
                    {#if order.invoiceId}
                      <button
                        class="secondary"
                        disabled={locked}
                        onclick={() => openInvoice(order.invoiceId)}>Rechnung öffnen</button
                      >
                    {:else if !order.reviewReason}
                      <button
                        class="secondary"
                        disabled={locked}
                        onclick={() => createInvoice(order)}
                        >{order.conversionError ? "Erneut prüfen" : "Entwurf erstellen"}</button
                      >
                    {:else}
                      <span class="shop-muted">Daten im Shop prüfen</span>
                    {/if}
                  </td>
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
      {:else}
        <p class="shop-no-orders">
          {overview.orders.length
            ? "Keine Bestellungen für diese Auswahl."
            : "Noch keine Bestellungen importiert. Nach dem Verbinden wird der erste Abruf automatisch gestartet."}
        </p>
      {/if}
    </section>
  {/if}
</section>

{#if configuring}
  <ShopConnectionDialog
    existing={editing}
    provider={newProvider}
    onclose={() => {
      configuring = false;
      editing = null;
    }}
    onconnected={connected}
    {onbusychange}
  />
{/if}

<style>
  .shops-workspace {
    display: grid;
    gap: 20px;
    min-width: 0;
  }
  .shops-topbar {
    gap: 20px;
  }
  .shops-topbar > button {
    flex-shrink: 0;
  }
  .shop-connections {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(min(17.5rem, 100%), 1fr));
    gap: 16px;
  }
  .shop-card {
    min-width: 0;
    border: 1px solid var(--theme-border);
    border-radius: 11px;
    padding: 18px;
    background: var(--theme-surface);
  }
  .shop-card-heading {
    display: flex;
    align-items: center;
    gap: 10px;
  }
  .shop-provider-icon {
    display: grid;
    place-items: center;
    flex-shrink: 0;
    width: 40px;
    height: 40px;
    border-radius: 9px;
    background: var(--theme-surface-muted);
    color: var(--theme-accent);
  }
  .shop-card-name {
    min-width: 0;
    flex: 1;
  }
  .shop-card-name h2 {
    margin: 0;
    font-size: 14px;
    overflow-wrap: anywhere;
  }
  .shop-card-name p,
  .shop-url {
    color: var(--theme-muted);
    font-size: 12px;
    margin: 3px 0 0;
  }
  .shop-url {
    margin: 15px 0;
    overflow-wrap: anywhere;
  }
  .shop-badge {
    display: inline-block;
    width: fit-content;
    border-radius: 5px;
    padding: 4px 6px;
    font-size: 10px;
    font-weight: 600;
    line-height: 1.3;
    color: var(--theme-muted);
    background: var(--theme-surface-muted);
  }
  .shop-warning {
    color: var(--theme-warning-text);
    background: var(--theme-warning-bg);
  }
  .shop-card-details {
    display: grid;
    gap: 8px;
    font-size: 12px;
    margin: 0;
  }
  .shop-card-details div {
    display: flex;
    gap: 12px;
    justify-content: space-between;
  }
  .shop-card-details dt {
    flex-shrink: 0;
    color: var(--theme-muted);
  }
  .shop-card-details dd {
    min-width: 0;
    margin: 0;
    text-align: right;
    overflow-wrap: anywhere;
  }
  .shop-card-actions {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: 12px;
    margin-top: 18px;
  }
  .shop-card-actions > :last-child {
    margin-left: auto;
  }
  .shop-card-warning,
  .shop-card-error {
    padding: 10px;
    margin: 12px 0 0;
    font-size: 12px;
    line-height: 1.5;
    border-radius: 6px;
    overflow-wrap: anywhere;
  }
  .shop-card-warning {
    background: var(--theme-warning-bg);
    color: var(--theme-warning-text);
  }
  .shop-card-error {
    background: var(--theme-error-bg);
    color: var(--theme-error-text);
    border: 1px solid var(--theme-error-border);
  }
  .shop-automation-note {
    margin: -4px 0 0;
    color: var(--theme-muted);
    font-size: 12px;
    line-height: 1.6;
  }
  .shop-empty {
    display: grid;
    justify-items: start;
    gap: 14px;
    padding: 24px;
    background: var(--theme-surface);
    border: 1px solid var(--theme-border);
    border-radius: 11px;
  }
  .shop-empty h2,
  .shop-empty p {
    margin: 0;
  }
  .shop-empty p {
    max-width: 550px;
    color: var(--theme-muted);
    font-size: 13px;
    line-height: 1.6;
  }
  .shop-empty .shop-provider-icon {
    width: 50px;
    height: 50px;
  }
  .shop-orders {
    min-width: 0;
  }
  .shop-orders-heading {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 16px;
    margin-bottom: 16px;
  }
  .shop-orders-heading > div {
    min-width: 0;
  }
  .shop-orders-heading > button {
    margin-left: auto;
  }
  .shop-orders-heading h2 {
    margin: 0;
  }
  .shop-orders-heading h2 span {
    color: var(--theme-muted);
    font-size: 12px;
    font-weight: 400;
    margin-left: 8px;
  }
  .shop-orders-heading p {
    color: var(--theme-muted);
    font-size: 12px;
    margin: 5px 0 0;
  }
  .shop-order-filters {
    display: flex;
    flex-wrap: wrap;
    align-items: end;
    gap: 12px;
    margin-bottom: 16px;
  }
  .shop-search {
    flex: 1 1 220px;
    min-width: 0;
  }
  .shop-select {
    display: grid;
    gap: 5px;
    color: var(--theme-muted);
    font-size: 12px;
  }
  .shop-select select {
    max-width: 220px;
  }
  .shop-table-wrap {
    width: 100%;
    overflow-x: auto;
    border: 1px solid var(--theme-border);
    border-radius: 9px;
    background: var(--theme-surface);
  }
  .shop-table {
    width: 100%;
    min-width: 670px;
    border-collapse: collapse;
    table-layout: auto;
    font-size: 12px;
  }
  .shop-table th {
    color: var(--theme-muted);
    font-size: 11px;
    font-weight: 600;
    text-align: left;
    background: var(--theme-surface-muted);
  }
  .shop-table th,
  .shop-table td {
    padding: 12px;
    border-bottom: 1px solid var(--theme-border);
    vertical-align: top;
  }
  .shop-table tr:last-child td {
    border-bottom: 0;
  }
  .shop-table strong {
    display: block;
    font-size: 12px;
    font-weight: 600;
    overflow-wrap: anywhere;
  }
  .shop-table small {
    display: block;
    margin-top: 5px;
    color: var(--theme-muted);
    font-size: 11px;
    line-height: 1.5;
    overflow-wrap: anywhere;
  }
  .shop-table .shop-amount {
    text-align: right;
    white-space: nowrap;
  }
  .shop-order-status {
    max-width: 250px;
    min-width: 100px;
  }
  .shop-table .shop-review-reason {
    color: var(--theme-warning-text);
  }
  .shop-order-action {
    text-align: right;
    width: 145px;
  }
  .shop-order-action button {
    white-space: nowrap;
  }
  .shop-muted,
  .shop-no-orders,
  .shop-loading {
    font-size: 12px;
    color: var(--theme-muted);
    line-height: 1.6;
  }
  .shop-no-orders {
    padding: 25px 20px;
    text-align: center;
    background: var(--theme-surface);
    border: 1px solid var(--theme-border);
    border-radius: 9px;
  }
  .shop-feedback {
    margin: 0;
    overflow-wrap: anywhere;
    padding: 12px 14px;
    border: 1px solid var(--theme-border);
    border-radius: 7px;
    font-size: 12px;
    line-height: 1.5;
  }
  .shop-feedback p {
    min-width: 0;
    margin: 0;
  }
  .shop-error {
    display: flex;
    justify-content: space-between;
    gap: 12px;
    color: var(--theme-error-text);
    border-color: var(--theme-error-border);
    background: var(--theme-error-bg);
  }
  .shop-error > button {
    flex-shrink: 0;
  }
  .shop-success {
    color: var(--theme-success-text);
    border-color: var(--theme-success-border);
    background: var(--theme-success-bg);
  }
  .shop-sr-only {
    position: absolute;
    width: 1px;
    height: 1px;
    padding: 0;
    margin: -1px;
    overflow: hidden;
    clip: rect(0, 0, 0, 0);
    white-space: nowrap;
    border: 0;
  }
  @media (max-width: 1050px) {
    .shops-topbar {
      align-items: start;
    }
    .shop-connections {
      grid-template-columns: minmax(0, 1fr);
    }
    .shop-table {
      display: block;
      min-width: 0;
    }
    .shop-table thead {
      position: absolute;
      width: 1px;
      height: 1px;
      overflow: hidden;
      clip-path: inset(50%);
    }
    .shop-table tbody {
      display: block;
    }
    .shop-table tr {
      display: grid;
      grid-template-columns: minmax(0, 1fr) minmax(0, 1fr) 140px;
      gap: 14px;
      padding: 16px;
      border-bottom: 1px solid var(--theme-border);
    }
    .shop-table tr:last-child {
      border-bottom: 0;
    }
    .shop-table td {
      padding: 0;
      border: 0;
    }
    .shop-table td:nth-child(1)::before,
    .shop-table td:nth-child(2)::before {
      display: block;
      margin-bottom: 5px;
      font-size: 10px;
      color: var(--theme-muted);
    }
    .shop-table td:nth-child(1)::before {
      content: "Bestellung";
    }
    .shop-table td:nth-child(2)::before {
      content: "Kunde";
    }
    .shop-order-status {
      grid-column: 1 / 3;
      max-width: none;
    }
    .shop-order-action {
      grid-column: 3;
      width: auto;
      align-self: end;
    }
  }
</style>
