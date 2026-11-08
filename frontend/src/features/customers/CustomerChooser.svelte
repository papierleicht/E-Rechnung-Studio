<script lang="ts">
  import SettingSection from "../../components/SettingSection.svelte";
  import Icon from "../../components/Icon.svelte";
  import Tabs from "../../components/Tabs.svelte";
  import ListHeader from "../../components/ListHeader.svelte";
  import EntryRow from "../../components/EntryRow.svelte";
  import InvoiceList from "../invoices/InvoiceList.svelte";
  import QuoteList from "../quotes/QuoteList.svelte";
  import { filterQuotes } from "../../lib/quotes";
  import { untrack } from "svelte";
  import type {
    InvoiceSummary,
    Customer,
    QuoteSummary,
  } from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/model/models";
  type Mode = "existing" | "new" | "template";
  let {
    kind = "invoice",
    customers,
    customer = $bindable(),
    busy,
    onchoose,
    oncreate,
    invoices = [],
    ontemplate,
    quotes = [],
    onquotetemplate,
  }: {
    /** Quotes start the same way, from an earlier quote instead of an invoice. */
    kind?: "invoice" | "quote";
    customers: Customer[];
    customer: Customer;
    busy: boolean;
    onchoose: (customer: Customer) => void;
    oncreate: (save: boolean) => void;
    invoices?: InvoiceSummary[];
    ontemplate?: (invoice: InvoiceSummary) => void;
    quotes?: QuoteSummary[];
    onquotetemplate?: (quote: QuoteSummary) => void;
  } = $props();
  const tabs = $derived([
    { id: "existing", label: "Gespeicherte Kunden" },
    { id: "new", label: "Neuer Kunde" },
    ...(ontemplate || onquotetemplate ? [{ id: "template", label: "Aus Vorlage" }] : []),
  ] as { id: Mode; label: string }[]);
  let mode = $state<Mode>(untrack(() => (customers.length ? "existing" : "new")));
  const write = $derived(kind === "invoice" ? "Rechnung schreiben" : "Angebot schreiben");
  let query = $state(""),
    templateQuery = $state(""),
    saveForLater = $state(true);
  const matches = $derived(
    customers.filter((c) =>
      `${c.name} ${c.address.city} ${c.email}`
        .toLocaleLowerCase("de")
        .includes(query.toLocaleLowerCase("de").trim()),
    ),
  );
  const templates = $derived(
    invoices.filter(
      (invoice) =>
        ["issued", "paid"].includes(invoice.status) &&
        `${invoice.number} ${invoice.customerName} ${invoice.issueDate}`
          .toLocaleLowerCase("de")
          .includes(templateQuery.trim().toLocaleLowerCase("de")),
    ),
  );
  const quoteTemplates = $derived(
    filterQuotes(
      quotes.filter((quote) => quote.status !== "draft"),
      templateQuery,
    ),
  );
  const address = (c: Customer) =>
    [c.address.street, [c.address.postcode, c.address.city].filter(Boolean).join(" ")]
      .filter(Boolean)
      .join(", ");
</script>

<!-- The first step of a new invoice or quote, laid out like the overviews:
     tabs, then count and search, then the rows to choose from. -->
<section class="invoice-dashboard customer-chooser">
  <Tabs {tabs} active={mode} label="Empfänger" disabled={busy} onchange={(id) => (mode = id)} />
  {#if mode === "existing"}
    <ListHeader
      count="{matches.length} {matches.length === 1 ? 'Kunde' : 'Kunden'}"
      placeholder={customers.length ? "Kunden suchen" : ""}
      bind:query
    />
    {#if matches.length}<div class="invoice-rows customer-rows">
        {#each matches as c (c.id)}<EntryRow
            title={c.name}
            subtitle={address(c)}
            tooltip="{write}: {c.name}"
            disabled={busy}
            onclick={() => onchoose(c)}
          >
            {#snippet status()}<span class="choose-label">{write}</span>{/snippet}
          </EntryRow>{/each}
      </div>{:else}<div class="empty-state">
        <h2>{customers.length ? "Keine Treffer" : "Noch keine Kunden gespeichert"}</h2>
        <p>
          {customers.length
            ? "Versuche es mit einem anderen Namen oder Ort – oder lege den Kunden neu an."
            : "Lege deinen ersten Kunden an. Beim nächsten Mal reicht dann ein Klick."}
        </p>
        <button class="secondary" disabled={busy} onclick={() => (mode = "new")}
          ><Icon name="plus" size={16} /> Neuen Kunden anlegen</button
        >
      </div>{/if}
  {:else if mode === "new"}
    <form
      class="customer-form"
      onsubmit={(event) => {
        event.preventDefault();
        oncreate(saveForLater);
      }}
    >
      <fieldset disabled={busy}>
        <div class="form-grid cols-2">
          <label class="full-width"
            >Name oder Unternehmen *<input
              required
              bind:value={customer.name}
              autocomplete="organization"
              placeholder={kind === "invoice"
                ? "Name des Rechnungsempfängers"
                : "Name des Angebotsempfängers"}
            /></label
          >
          <label class="full-width"
            >Straße und Hausnummer *<input
              required
              bind:value={customer.address.street}
              autocomplete="address-line1"
            /></label
          >
          <label
            >Postleitzahl<input
              bind:value={customer.address.postcode}
              autocomplete="postal-code"
            /></label
          ><label
            >Ort *<input
              required
              bind:value={customer.address.city}
              autocomplete="address-level2"
            /></label
          >
          <label
            >Land (Kürzel) *<input
              required
              minlength="2"
              maxlength="2"
              bind:value={customer.address.country}
              placeholder="DE"
            /></label
          ><label
            >E-Mail (optional)<input
              type="email"
              bind:value={customer.email}
              autocomplete="email"
            /></label
          >
        </div>
        <SettingSection
          title="Weitere Kundenangaben (optional)"
          action="Ergänzen"
          summary={[
            customer.vatId && `USt-IdNr. ${customer.vatId}`,
            customer.buyerReference && `Referenz ${customer.buyerReference}`,
            customer.contactName,
          ]
            .filter(Boolean)
            .join(" · ") || "USt-IdNr., Kundenreferenz, Ansprechpartner"}
        >
          <div class="form-grid cols-2">
            <label>USt-IdNr.<input bind:value={customer.vatId} /></label><label
              >Kundenreferenz<input
                bind:value={customer.buyerReference}
                placeholder="Vom Kunden mitgeteilte Referenz"
              /></label
            ><label>Ansprechpartner<input bind:value={customer.contactName} /></label>
          </div>
        </SettingSection>
        <label class="remember-customer"
          ><input type="checkbox" bind:checked={saveForLater} /><span
            >In der Kundenliste speichern</span
          ></label
        >
      </fieldset>
      <footer>
        <span>* Pflichtangaben</span><button class="primary" disabled={busy}
          >{busy
            ? "Wird übernommen …"
            : saveForLater
              ? `Kunden speichern & ${write}`
              : kind === "invoice"
                ? "Weiter zur Rechnung"
                : "Weiter zum Angebot"}<Icon name="next" size={16} /></button
        >
      </footer>
    </form>
  {:else if kind === "quote"}
    <ListHeader
      count="{quoteTemplates.length} {quoteTemplates.length === 1 ? 'Angebot' : 'Angebote'}"
      placeholder="Angebote suchen"
      bind:query={templateQuery}
    />
    {#if quoteTemplates.length && onquotetemplate}<QuoteList
        quotes={quoteTemplates}
        onopen={onquotetemplate}
        {busy}
      />{:else}<div class="empty-state">
        <h2>{templateQuery ? "Keine Treffer" : "Noch keine Vorlage"}</h2>
        <p>
          {templateQuery
            ? "Kein Angebot passt zu deiner Suche."
            : "Sobald du ein Angebot abgeschlossen hast, kannst du es hier als Vorlage verwenden."}
        </p>
      </div>{/if}
  {:else}
    <ListHeader
      count="{templates.length} {templates.length === 1 ? 'Rechnung' : 'Rechnungen'}"
      placeholder="Rechnungen suchen"
      bind:query={templateQuery}
    />
    {#if templates.length && ontemplate}<InvoiceList
        invoices={templates}
        onopen={ontemplate}
        actionLabel="Als Vorlage verwenden"
        {busy}
      />{:else}<div class="empty-state">
        <h2>{templateQuery ? "Keine Treffer" : "Noch keine Vorlage"}</h2>
        <p>
          {templateQuery
            ? "Keine ausgestellte Rechnung passt zu deiner Suche."
            : "Sobald du eine Rechnung ausgestellt hast, kannst du sie hier als Vorlage für eine neue verwenden."}
        </p>
      </div>{/if}
  {/if}
</section>

<style>
  .customer-form {
    padding: 20px;
    border: 1px solid var(--theme-border);
    border-radius: 10px;
    background: var(--theme-surface);
  }
  fieldset {
    min-width: 0;
    margin: 0;
    padding: 0;
    border: 0;
  }
  .customer-form input:not([type="checkbox"]) {
    min-height: 40px;
  }
  .customer-form footer {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 12px 16px;
    margin-top: 20px;
    padding-top: 16px;
    border-top: 1px solid var(--theme-border);
  }
  .customer-form footer > span {
    color: var(--theme-muted);
    font-size: 11px;
  }
  .customer-form footer button {
    margin-left: auto;
    display: inline-flex;
    align-items: center;
    gap: 6px;
  }
  /* Long names and addresses stop after two lines each; the row tooltip
     carries the full name. */
  .customer-rows :global(.invoice-entry-title > :is(strong, small)) {
    display: -webkit-box;
    -webkit-box-orient: vertical;
    -webkit-line-clamp: 2;
    line-clamp: 2;
    overflow: hidden;
  }
  .choose-label {
    justify-self: end;
    color: var(--theme-accent);
    font-size: 12px;
    font-weight: 600;
    white-space: nowrap;
  }
</style>
