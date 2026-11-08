<script lang="ts">
  import { tick } from "svelte";
  import type {
    Company,
    AppSettings,
  } from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/model/models";
  import ElectronicAddress from "../../components/ElectronicAddress.svelte";
  import Icon from "../../components/Icon.svelte";
  import CompanyTaxSettings from "./CompanyTaxSettings.svelte";
  import CompanyLogoSettings from "./CompanyLogoSettings.svelte";
  import InvoiceTaxDefaults from "./InvoiceTaxDefaults.svelte";
  import ReminderSettings from "./ReminderSettings.svelte";
  import InvoiceNumberSettings from "./InvoiceNumberSettings.svelte";
  import MailSettings from "./MailSettings.svelte";
  import { paymentChoices } from "../../lib/tax";
  import BankAccountsEditor from "../../components/BankAccountsEditor.svelte";
  import { setupMissing } from "../../lib/onboarding";
  import AccountSettings from "./AccountSettings.svelte";
  import AuditSettings from "./AuditSettings.svelte";
  import PlusHint from "../../components/PlusHint.svelte";
  import NewsletterSignup from "../../components/NewsletterSignup.svelte";
  import { optionalSections, sectionVisible } from "../../lib/menuSections";
  let aboutOpen = $state(false);
  const tabs = [
    { id: "general", label: "Allgemein" },
    { id: "company", label: "Unternehmen" },
    { id: "invoice", label: "Rechnung" },
    { id: "mail", label: "E-Mail" },
    { id: "audit", label: "Prüfung" },
    { id: "account", label: "Konto & Abo" },
  ] as const;
  let {
    company = $bindable(),
    settings = $bindable(),
    activeTab = $bindable("general"),
    busy,
    dirty,
    saving = false,
    onsave,
    onsectionpending,
    chooseExportDirectory,
    exportBackup,
    importBackup,
    onopenbanking = () => {},
    shopConnected = false,
  }: {
    company: Company;
    settings: AppSettings;
    activeTab: "general" | "company" | "invoice" | "mail" | "audit" | "account";
    busy: boolean;
    dirty: boolean;
    saving?: boolean;
    onsave: () => void;
    onsectionpending: (id: string, save: (() => Promise<void>) | null) => void;
    /** A step of the way on the account page leads to the bank connection. */
    onopenbanking?: () => void;
    /** Decides whether the shop area shows while the customer did not choose. */
    shopConnected?: boolean;
    chooseExportDirectory: () => void;
    exportBackup: () => void;
    importBackup: () => void;
  } = $props();
  async function switchTab(event: KeyboardEvent, index: number) {
    let next = index;
    if (event.key === "ArrowRight") next = (index + 1) % tabs.length;
    else if (event.key === "ArrowLeft") next = (index + tabs.length - 1) % tabs.length;
    else if (event.key === "Home") next = 0;
    else if (event.key === "End") next = tabs.length - 1;
    else return;
    event.preventDefault();
    activeTab = tabs[next].id;
    await tick();
    document.getElementById(`settings-tab-${activeTab}`)?.focus();
  }
  const missing = $derived(setupMissing(company));
  function showSection(id: string, visible: boolean) {
    settings.sections = { ...(settings.sections ?? {}), [id]: visible };
  }
</script>

<header class="topbar">
  <div>
    <h1>Einstellungen</h1>
  </div>
  <div class="actions">
    <span class="save-state" role="status">{dirty ? "Ungespeichert" : "Gespeichert"}</span>
    <button class="primary" disabled={busy || !dirty} onclick={onsave}
      ><Icon name="save" size={16} /> {saving ? "Wird gespeichert …" : "Speichern"}</button
    >
  </div>
</header>
<div class="settings-tabs" role="tablist" aria-label="Einstellungskategorien">
  {#each tabs as tab, index}
    <button
      type="button"
      role="tab"
      id={`settings-tab-${tab.id}`}
      aria-controls={`settings-panel-${tab.id}`}
      aria-selected={activeTab === tab.id}
      tabindex={activeTab === tab.id ? 0 : -1}
      onclick={() => (activeTab = tab.id)}
      onkeydown={(event) => switchTab(event, index)}
    >
      {tab.label}
    </button>
  {/each}
</div>
<section class="settings panel">
  {#if missing.length}
    <aside class="setup-banner" aria-labelledby="setup-banner-title">
      <strong id="setup-banner-title">Noch zu ergänzen für deine erste Rechnung</strong>
      <ul>
        {#each missing as item}<li>
            <button type="button" class="text" onclick={() => (activeTab = "company")}
              >{item.message}</button
            >
          </li>{/each}
      </ul>
    </aside>
  {/if}
  <fieldset class="settings-fields" disabled={busy}>
    <div
      id="settings-panel-general"
      role="tabpanel"
      aria-labelledby="settings-tab-general"
      tabindex="0"
      hidden={activeTab !== "general"}
    >
      <section class="appearance-settings" aria-labelledby="appearance-title">
        <div>
          <h2 id="appearance-title">Darstellung</h2>
          <p>Standardmäßig richtet sich die Darstellung nach den Einstellungen deines Systems.</p>
        </div>
        <label for="appearance">
          Erscheinungsbild
          <select id="appearance" bind:value={settings.appearance}>
            <option value="system">System</option>
            <option value="light">Hell</option>
            <option value="dark">Dunkel</option>
          </select>
        </label>
      </section>
      <section class="appearance-settings update-settings" aria-labelledby="updates-title">
        <div>
          <h2 id="updates-title">Automatische Updates</h2>
          <p>
            Bei geöffneter App und Internetverbindung sucht die App einmal täglich nach Updates.
            Verfügbare Updates siehst du unten in der Seitenleiste. Die Installation startest du
            selbst.
          </p>
        </div>
        <label class="update-toggle">
          <input type="checkbox" bind:checked={settings.automaticUpdates} />
          Automatische Updates aktivieren
        </label>
      </section>
      <section class="appearance-settings update-settings" aria-labelledby="menu-title">
        <div>
          <h2 id="menu-title">Bereiche im Menü</h2>
          <p>
            Blende aus, was du nicht brauchst. Deine Daten bleiben erhalten, und du kannst die
            Bereiche jederzeit wieder einblenden.
          </p>
        </div>
        <div class="menu-sections">
          {#each optionalSections as item}
            <label class="update-toggle" title={item.hint}>
              <input
                type="checkbox"
                checked={sectionVisible(item.id, settings.sections, { shopConnected })}
                onchange={(event) => showSection(item.id, event.currentTarget.checked)}
              />
              {item.label} anzeigen
            </label>
          {/each}
        </div>
      </section>
      <NewsletterSignup email={company.email} source="app-settings" />
      <div class="backup">
        <div>
          <h2>Datensicherung</h2>
          <p>
            Speichert alle Daten und Belege im Original als ZIP-Datei. Lege sie an einem anderen Ort
            ab als auf diesem Gerät, damit sie einen Ausfall übersteht.
          </p>
          <PlusHint
            id="settings-backup"
            topic="backup"
            text="Mit dem Backup-Zusatz zu Plus werden deine Daten automatisch und verschlüsselt gesichert."
          />
        </div>
        <div class="actions">
          <button class="secondary" disabled={busy} onclick={exportBackup}
            >Sicherung exportieren</button
          ><button class="secondary" disabled={busy} onclick={importBackup}
            >Sicherung importieren</button
          >
        </div>
      </div>
    </div>
    <div
      id="settings-panel-company"
      role="tabpanel"
      aria-labelledby="settings-tab-company"
      tabindex="0"
      hidden={activeTab !== "company"}
    >
      <h2>Unternehmen</h2>
      <p class="settings-help">
        Diese Angaben erscheinen auf deinen Rechnungen. Ergänze Anschrift, steuerliche Kennungen und
        Bankverbindung und speichere anschließend deine Änderungen.
      </p>
      <CompanyLogoSettings bind:company disabled={busy} />
      <section class="settings-group" aria-labelledby="company-contact-title">
        <h3 id="company-contact-title">Unternehmen und Kontakt</h3>
        <div class="form-grid cols-4">
          <label>Firmenname<input data-field="company.name" bind:value={company.name} /></label>
          <label
            >Ansprechpartner<input
              data-field="company.contactName"
              bind:value={company.contactName}
            /></label
          >
          <label
            >E-Mail<input
              data-field="company.email"
              type="email"
              bind:value={company.email}
            /></label
          >
          <label>Telefon<input data-field="company.phone" bind:value={company.phone} /></label>
        </div>
      </section>
      <section class="settings-group" aria-labelledby="electronic-address-title">
        <h3 id="electronic-address-title">Elektronische Absenderadresse</h3>
        <p class="settings-help">
          Für E-Rechnungen erforderlich. Standardmäßig übernehmen wir deine Kontakt-E-Mail-Adresse.
        </p>
        <div class="form-grid cols-4">
          <ElectronicAddress
            bind:endpoint={company.endpoint}
            bind:scheme={company.endpointScheme}
            email={company.email}
            field="company"
            followEmail
          />
        </div>
      </section>
      <section class="settings-group" aria-labelledby="company-address-title">
        <h3 id="company-address-title">Anschrift</h3>
        <div class="form-grid cols-4">
          <label
            >Straße und Hausnummer<input
              data-field="company.address.street"
              bind:value={company.address.street}
            /></label
          >
          <label
            >Adresszusatz<input
              data-field="company.address.additional"
              bind:value={company.address.additional}
            /></label
          >
          <label class="span-1"
            >Postleitzahl<input
              data-field="company.address.postcode"
              bind:value={company.address.postcode}
            /></label
          >
          <label
            >Ort<input data-field="company.address.city" bind:value={company.address.city} /></label
          >
          <label class="span-1"
            >Land<input
              data-field="company.address.country"
              maxlength="2"
              placeholder="DE"
              bind:value={company.address.country}
            /></label
          >
        </div>
      </section>
      <section class="settings-group" aria-labelledby="company-tax-ids-title">
        <h3 id="company-tax-ids-title">Steuerliche Kennungen</h3>
        <div class="form-grid cols-4">
          <label>USt-IdNr.<input data-field="company.vatId" bind:value={company.vatId} /></label>
          <label
            >Steuernummer<input
              data-field="company.taxNumber"
              bind:value={company.taxNumber}
            /></label
          >
        </div>
      </section>
      <section class="settings-group" aria-labelledby="company-bank-title">
        <h3 id="company-bank-title">Bankverbindung</h3>
        <p class="settings-help">
          Bei Überweisungen erscheinen alle hinterlegten Konten auf der Rechnung.
        </p>
        <BankAccountsEditor bind:company />
      </section>
      <CompanyTaxSettings bind:company />
      <section class="settings-group" aria-labelledby="company-payment-title">
        <h3 id="company-payment-title">Standard-Zahlungsart</h3>
        <div class="form-field form-field-compact">
          <label for="company-payment-means">Zahlungsart für neue Rechnungen</label>
          <select
            id="company-payment-means"
            class="control-compact"
            data-field="company.paymentMeansCode"
            value={company.paymentMeansCode || "58"}
            onchange={(event) => (company.paymentMeansCode = event.currentTarget.value)}
            >{#each paymentChoices as choice}<option value={choice.id}>{choice.label}</option
              >{/each}</select
          >
        </div>
        <p class="form-help">
          In den Rechnungseinstellungen kannst du pro Rechnung eine andere Zahlungsart wählen.
        </p>
      </section>
    </div>
    <div
      id="settings-panel-invoice"
      role="tabpanel"
      aria-labelledby="settings-tab-invoice"
      tabindex="0"
      hidden={activeTab !== "invoice"}
    >
      <InvoiceTaxDefaults bind:company />
      <InvoiceNumberSettings
        disabled={busy}
        onpendingchange={(save) => onsectionpending("numbers", save)}
      />
      <section class="export-folder-settings" aria-labelledby="export-folder-title">
        <h2 id="export-folder-title">Rechnungsexport</h2>
        <p>
          Beim Ausstellen speichern wir die PDF hier. Bei XRechnung kommt zusätzlich die XML-Datei
          dazu.
        </p>
        <label for="export-directory">Exportordner</label>
        <div class="export-directory-row">
          <input id="export-directory" bind:value={settings.exportDirectory} /><button
            class="secondary"
            disabled={busy}
            onclick={chooseExportDirectory}><Icon name="folder" size={16} /> Ordner wählen …</button
          >
        </div>
        <div class="export-folder-footer">
          <span
            >Standard: Dokumente/Rechnungen. Klicke oben rechts auf „Speichern“, um Änderungen zu
            übernehmen.</span
          >
        </div>
      </section>
      <ReminderSettings
        disabled={busy}
        onpendingchange={(save) => onsectionpending("reminders", save)}
      />
    </div>
    <div
      id="settings-panel-mail"
      role="tabpanel"
      aria-labelledby="settings-tab-mail"
      tabindex="0"
      hidden={activeTab !== "mail"}
    >
      <MailSettings {busy} />
    </div>
  </fieldset>
  <div
    id="settings-panel-audit"
    role="tabpanel"
    aria-labelledby="settings-tab-audit"
    tabindex="0"
    hidden={activeTab !== "audit"}
  >
    {#if activeTab === "audit"}<AuditSettings onplan={() => (activeTab = "account")} />{/if}
  </div>
  <div
    id="settings-panel-account"
    role="tabpanel"
    aria-labelledby="settings-tab-account"
    tabindex="0"
    hidden={activeTab !== "account"}
  >
    {#if activeTab === "account"}<AccountSettings
        onopen={(target) => (target === "audit" ? (activeTab = "audit") : onopenbanking())}
      />{/if}
  </div>
  <footer class="settings-about" hidden={activeTab !== "general"}>
    <button type="button" onclick={() => (aboutOpen = true)}>Über E-Rechnung-Studio</button>
  </footer>
</section>
{#if aboutOpen}
  {#await import("./AboutDialog.svelte") then module}
    <module.default onclose={() => (aboutOpen = false)} />
  {:catch}
    <p role="alert">Die Lizenzhinweise ließen sich nicht laden.</p>
    <button type="button" onclick={() => (aboutOpen = false)}>Schließen</button>
  {/await}
{/if}

<style>
  .menu-sections {
    display: grid;
    gap: 10px;
  }
  .setup-banner {
    margin: 0 0 20px;
    padding: 14px 16px;
    border: 1px solid var(--theme-warning-text);
    border-radius: 10px;
    background: var(--theme-warning-bg);
    color: var(--theme-warning-text);
    font-size: 13px;
  }
  .setup-banner ul {
    overflow-wrap: anywhere;
    margin: 8px 0 0;
    padding-left: 18px;
  }
  .setup-banner li + li {
    margin-top: 4px;
  }
  .setup-banner button {
    padding: 0;
    color: inherit;
    text-align: left;
    text-decoration: underline;
  }
  .settings-tabs {
    display: flex;
    gap: 6px;
    margin: 0 0 20px;
    padding: 5px;
    width: fit-content;
    max-width: 100%;
    overflow-x: auto;
    overscroll-behavior-x: contain;
    border: 1px solid var(--theme-border);
    border-radius: 10px;
    background: var(--theme-surface-muted);
  }
  .settings-tabs button {
    flex-shrink: 0;
    padding: 10px 18px;
    border: 1px solid transparent;
    border-radius: 7px;
    background: transparent;
    color: var(--theme-muted);
    font: inherit;
    font-size: var(--ui-control-font-size);
    line-height: 20px;
    font-weight: 600;
    cursor: pointer;
  }
  .settings-tabs button[aria-selected="true"] {
    background: var(--theme-surface);
    border-color: var(--theme-border);
    color: var(--theme-accent);
  }
  .settings-tabs button:hover {
    color: var(--theme-text);
  }
  .settings-tabs button:focus-visible {
    outline: 2px solid var(--theme-accent);
    outline-offset: 2px;
  }
  [role="tabpanel"][hidden],
  .settings-about[hidden] {
    display: none;
  }
  [role="tabpanel"] {
    min-width: 0;
  }
  /* Every tab starts at the same distance from the card edge. */
  [role="tabpanel"] > :global(:first-child),
  [role="tabpanel"] > :global(:first-child > h2:first-child) {
    margin-top: 0;
  }
  @media (max-width: 1050px) {
    .settings-tabs button {
      padding-inline: 12px;
    }
  }
  @media (max-width: 600px) {
    .settings-tabs {
      width: 100%;
    }
    .settings-tabs button {
      flex: 1;
      padding: 10px;
    }
  }
  .settings-about {
    margin-top: 24px;
    text-align: center;
  }
  .settings-about button {
    border: 0;
    background: transparent;
    color: var(--theme-muted);
    font-size: 12px;
    padding: 8px;
  }
  .settings-about button:hover {
    color: var(--theme-text);
    text-decoration: underline;
  }
</style>
