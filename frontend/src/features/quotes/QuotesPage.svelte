<script module lang="ts">
  import DocumentMailButton from "../invoices/DocumentMailButton.svelte";
  import type {
    Quote,
    QuoteView,
  } from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/model/models";
  /** Which step of an open quote shows, so the app can lay out the document view. */
  export type QuoteEditor = {
    step: "customer" | "document";
  };
  // Like the open invoice draft, an open quote survives switching sections.
  let kept: {
    quote: Quote;
    baseline: string;
    view: QuoteView | null;
    step: QuoteEditor["step"];
  } | null = null;
</script>

<script lang="ts">
  import { onDestroy, onMount, tick, untrack, type Snippet } from "svelte";
  import { scrollPageToTop } from "../../lib/scrollPage";
  import { cached, remember } from "../../lib/cache";
  import { Dialogs } from "@wailsio/runtime";
  import * as API from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/app/invoiceservice";
  import type {
    Company,
    Customer,
    InvoiceDraft,
    InvoiceLine,
    Product,
    QuoteSummary,
  } from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/model/models";
  import Icon from "../../components/Icon.svelte";
  import PageHeader from "../../components/PageHeader.svelte";
  import ValidationReport from "../../components/ValidationReport.svelte";
  import DocumentTabs from "../invoices/DocumentTabs.svelte";
  import ListHeader from "../../components/ListHeader.svelte";
  import InvoiceDocument from "../invoices/InvoiceDocument.svelte";
  import InvoiceSettingsDialog from "../invoices/InvoiceSettingsDialog.svelte";
  import PositionEditor from "../invoices/PositionEditor.svelte";
  import CustomerChooser from "../customers/CustomerChooser.svelte";
  import * as MailAPI from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/app/mailservice";
  import type { LogEntry } from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/mail/models";
  import { sentAtLabel } from "../../lib/mail";
  import QuoteList from "./QuoteList.svelte";
  import { clone, isBlankLine, newCustomer, newLine } from "../../lib/invoiceDraft";
  import { amountLabel, dateLabel } from "../../lib/format";
  import {
    applyQuoteCustomer,
    canConvertQuote,
    filterQuotes,
    newQuote,
    quoteFromTemplate,
    quoteBadgeClass,
    quoteStatusLabel,
  } from "../../lib/quotes";

  let {
    company,
    customers,
    products,
    template,
    busy,
    error = "",
    run,
    confirm,
    notify,
    pickFile,
    onshowtab,
    onopeninvoice,
    oneditcompany,
    oncustomersaved,
    ondirtychange,
    onmailsettings = () => {},
    startNew = false,
    onstartnew = () => {},
    editor = $bindable(null),
    tiles,
  }: {
    company: Company;
    customers: Customer[];
    products: Product[];
    /** The bootstrap invoice draft; quotes start from the same defaults. */
    template: InvoiceDraft;
    busy: boolean;
    /** The last failure, shown inside the settings dialog. */
    error?: string;
    run: <T>(op: () => Promise<T>) => Promise<T | undefined>;
    confirm: (title: string, message: string, action: string) => Promise<boolean>;
    notify: (message: string) => void;
    pickFile: <T>(open: () => Promise<T>) => Promise<T | undefined>;
    onshowtab: (tab: "invoices" | "recurring") => void;
    onopeninvoice: (id: string) => void;
    oneditcompany: () => void;
    /** A customer created while choosing the recipient joins the customer list. */
    oncustomersaved: (customer: Customer) => void;
    ondirtychange: (dirty: boolean) => void;
    onmailsettings?: () => void;
    /** Opened from the "Angebot erstellen" tile: start a new quote right away. */
    startNew?: boolean;
    onstartnew?: () => void;
    editor?: QuoteEditor | null;
    /** The section's action tiles, shared with the invoice overview. */
    tiles?: Snippet;
  } = $props();

  let quotes = $state<QuoteSummary[]>(cached<QuoteSummary[]>("quotes:list") ?? []);
  let listLoaded = $state(cached("quotes:list") !== undefined);
  let query = $state("");
  let quote = $state<Quote | null>(kept?.quote ?? null);
  let view = $state<QuoteView | null>(kept?.view ?? null);
  let baseline = $state(kept?.baseline ?? "");
  let step = $state<QuoteEditor["step"]>(kept?.step ?? "document");
  // As with an invoice draft, the list can show while a quote stays open; coming
  // back to the section starts at the list, with a button to continue the quote.
  let listing = $state(kept !== null);
  let pending = $state(false);
  let prospective = $state(newCustomer());
  let settingsOpen = $state(false),
    settingsFocus = $state("");
  let editingLines = $state<InvoiceLine[] | null>(null),
    editingLineIndex = $state(0);
  let dialogDirty = $state(false);
  let quoteDocument: { editField: (field: string) => void } | undefined = $state();

  const editable = $derived(quote?.status === "draft");
  let mailLog = $state<LogEntry[]>([]);
  const lastMailed = $derived(mailLog.find((entry) => entry.status === "sent"));
  async function loadMailLog() {
    const id = quote?.id;
    if (!id || quote?.status === "draft") mailLog = [];
    else mailLog = (await MailAPI.QuoteMailLog(id).catch(() => [])) ?? [];
  }
  $effect(() => {
    void [quote?.id, quote?.status];
    void loadMailLog();
  });
  // The previewed number is informational and never makes a quote "unsaved".
  const fingerprint = (value: Quote) =>
    JSON.stringify({ ...value, number: "", automaticNumber: false });
  const dirty = $derived(Boolean(quote && editable && fingerprint(quote) !== baseline));
  const visibleQuotes = $derived(filterQuotes(quotes, query));
  const resumeLabel = $derived(
    !quote
      ? ""
      : quote.document.customer.name
        ? `${editable ? "Entwurf fortsetzen" : "Geöffnetes Angebot anzeigen"} · ${quote.document.customer.name}`
        : "Neues Angebot fortsetzen · Kunde auswählen",
  );

  $effect(() => ondirtychange(dirty || dialogDirty));
  $effect(() => {
    editor = quote && !listing ? { step } : null;
  });
  // Also reacts while the page is open: the tile above the quote list sets it again.
  $effect(() => {
    if (!startNew) return;
    untrack(() => {
      onstartnew();
      void createQuote();
    });
  });
  onMount(() => {
    if (quote?.status === "draft" && !untrack(() => startNew)) schedulePreview();
    // The first load runs outside run(): going busy would disable the controls
    // and make the page flash every time the tab opens.
    void (async () => {
      try {
        quotes = remember("quotes:list", (await API.ListQuotes()) ?? []);
        listLoaded = true;
      } catch {
        await refreshList();
      }
    })();
  });
  onDestroy(() => {
    clearTimeout(timer);
    kept = quote ? { quote: clone(quote), baseline, view: view ? clone(view) : null, step } : null;
  });

  async function refreshList() {
    const result = await run(() => API.ListQuotes());
    if (result !== undefined) quotes = remember("quotes:list", result ?? []);
    listLoaded = true;
  }

  let timer: ReturnType<typeof setTimeout> | undefined;
  let revision = 0;
  function schedulePreview() {
    if (!quote || quote.status !== "draft") return;
    clearTimeout(timer);
    const current = ++revision;
    pending = true;
    timer = setTimeout(async () => {
      try {
        const result = await API.PreviewQuote(clone(quote!));
        if (current !== revision || !quote) return;
        view = result;
        quote.number = result.quote.number;
        quote.automaticNumber = result.quote.automaticNumber;
      } catch {
        if (current === revision) view = null;
      } finally {
        if (current === revision) pending = false;
      }
    }, 350);
  }
  function accept(result: QuoteView) {
    clearTimeout(timer);
    revision++;
    pending = false;
    view = result;
    quote = clone(result.quote);
    baseline = fingerprint(quote);
  }
  async function discardAllowed(message: string) {
    return !dirty || (await confirm("Ungespeicherte Änderungen", message, "Verwerfen"));
  }
  async function scrollTop() {
    await tick();
    scrollPageToTop();
  }

  async function createQuote() {
    if (
      busy ||
      !(await discardAllowed("Ungespeicherte Änderungen verwerfen und ein neues Angebot beginnen?"))
    )
      return;
    quote = newQuote(template, company);
    baseline = fingerprint(quote);
    view = null;
    prospective = newCustomer();
    step = "customer";
    listing = false;
    await scrollTop();
  }
  async function useQuoteTemplate(item: QuoteSummary) {
    const result = await run(() => API.OpenQuote(item.id));
    if (!result) return;
    quote = quoteFromTemplate(result.quote);
    baseline = fingerprint(quote);
    view = null;
    prospective = newCustomer();
    step = "document";
    schedulePreview();
    await scrollTop();
  }
  function chooseCustomer(customer: Customer) {
    if (!quote) return;
    applyQuoteCustomer(quote, customer, company);
    prospective = newCustomer();
    step = "document";
    schedulePreview();
    void scrollTop();
  }
  async function createCustomer(save: boolean) {
    if (!quote || busy) return;
    const customer = clone(prospective);
    customer.name = customer.name.trim();
    customer.address.country = customer.address.country.trim().toUpperCase();
    if (!save) return chooseCustomer(customer);
    const saved = await run(() => API.SaveCustomer(customer));
    if (!saved) return;
    oncustomersaved(saved);
    chooseCustomer(saved);
  }
  /** Whether the open quote is one of the rows shown; then that row resumes it. */
  const quoteListed = $derived(
    Boolean(quote?.id) && visibleQuotes.some((item) => item.id === quote!.id),
  );
  async function openQuote(item: QuoteSummary) {
    if (busy) return;
    if (quote && item.id === quote.id) {
      resume();
      return;
    }
    if (
      !(await discardAllowed("Ungespeicherte Änderungen verwerfen und ein anderes Angebot öffnen?"))
    )
      return;
    const result = await run(() => API.OpenQuote(item.id));
    if (!result) return;
    accept(result);
    step = "document";
    listing = false;
    if (result.quote.status === "draft") schedulePreview();
    await scrollTop();
  }
  /** Back to the list without discarding the open quote, like "Alle Rechnungen". */
  export function showList() {
    if (!quote || listing) return;
    listing = true;
    void refreshList();
    void scrollTop();
  }
  function resume() {
    listing = false;
    schedulePreview();
    void scrollTop();
  }
  async function save() {
    if (!quote || busy) return;
    const result = await run(() => API.SaveQuote(clone(quote!)));
    if (!result) return;
    accept(result);
    notify("Angebot gespeichert");
    await refreshList();
  }
  async function saveSettings(working: InvoiceDraft) {
    if (!quote || busy) return;
    const result = await run(() => API.SaveQuote({ ...clone(quote!), document: clone(working) }));
    if (!result) return;
    accept(result);
    settingsOpen = false;
    schedulePreview();
    notify("Angebotseinstellungen gespeichert");
    await refreshList();
  }
  async function finish() {
    if (!quote || busy) return;
    const result = await run(() => API.MarkQuoteSent(clone(quote!)));
    if (!result) return;
    accept(result);
    notify(`Angebot ${result.quote.number} abgeschlossen. Du kannst die PDF jetzt versenden.`);
    await refreshList();
  }
  async function reopenQuote() {
    if (
      lastMailed &&
      !(await confirm(
        "Angebot wurde schon versendet",
        "Dein Kunde hat die bisherige Fassung bekommen. Änderungen erreichen ihn erst mit einem neuen Versand.",
        "Wieder bearbeiten",
      ))
    )
      return;
    await changeStatus("draft", "Angebot wieder in Bearbeitung");
  }
  async function changeStatus(status: string, text: string) {
    if (!quote?.id || busy) return;
    const result = await run(() => API.SetQuoteStatus(quote!.id, status));
    if (!result) return;
    accept(result);
    if (status === "draft") schedulePreview();
    notify(text);
    await refreshList();
  }
  async function convert() {
    if (!quote?.id || busy) return;
    if (
      !(await confirm(
        "In Rechnung umwandeln",
        "Die App erstellt einen Rechnungsentwurf mit allen Positionen und einem Hinweis auf dieses Angebot. Das Angebot gilt danach als umgewandelt.",
        "Rechnungsentwurf erstellen",
      ))
    )
      return;
    const id = quote.id;
    const invoice = await run(() => API.ConvertQuoteToInvoice(id));
    if (!invoice) return;
    const reopened = await run(() => API.OpenQuote(id));
    if (reopened) accept(reopened);
    await refreshList();
    notify("Rechnungsentwurf erstellt. Prüfe noch Leistungsdatum und Zahlungsziel.");
    onopeninvoice(invoice.draft.id);
  }
  async function remove() {
    if (!quote?.id || busy) return;
    if (
      !(await confirm(
        "Entwurf löschen",
        "Diesen Angebotsentwurf dauerhaft löschen?",
        "Entwurf löschen",
      ))
    )
      return;
    const id = quote.id;
    const ok = await run(async () => {
      await API.DeleteQuote(id);
      return true;
    });
    if (!ok) return;
    quote = null;
    view = null;
    notify("Angebotsentwurf gelöscht");
    await refreshList();
  }
  async function exportPDF() {
    if (!quote?.id || busy) return;
    const { id, number } = quote;
    const path = await run(async () => {
      const target = await pickFile(() =>
        Dialogs.SaveFile({
          Title: "Angebot als PDF speichern",
          Filename: `Angebot-${number || "Entwurf"}.pdf`,
          CanCreateDirectories: true,
          Filters: [{ DisplayName: "Angebot (PDF)", Pattern: "*.pdf" }],
        }),
      );
      if (!target) return;
      return API.ExportQuotePDF(id, target);
    });
    if (path) notify("Angebot als PDF gespeichert");
  }
  async function openPDF() {
    if (!quote?.id || busy) return;
    const id = quote.id;
    await run(() => API.OpenQuotePDF(id));
  }

  function editLines(index = 0) {
    if (!quote || !editable || busy) return;
    const lines = clone(quote.document.lines ?? []);
    if (index < 0 || !lines.length) {
      const blank = lines.findIndex(isBlankLine);
      if (blank >= 0) index = blank;
      else {
        lines.push(newLine());
        index = lines.length - 1;
      }
    }
    editingLineIndex = Math.max(0, Math.min(index, lines.length - 1));
    editingLines = lines;
  }
  function openSettings(field: string) {
    if (!editable || busy) return;
    settingsFocus = field;
    settingsOpen = true;
  }
  // Same routing as on an invoice: tax and discounts live in the settings dialog.
  function resolve(field: string) {
    const line = /^lines\.(\d+)/.exec(field);
    if (field === "lines" || line) editLines(line ? Number(line[1]) : 0);
    else if (field.startsWith("company.")) oneditcompany();
    else if (
      ["taxTreatment", "taxExemptionReason"].includes(field) ||
      field.startsWith("adjustments")
    )
      openSettings(field);
    else quoteDocument?.editField(field);
  }
</script>

{#if !quote || listing}
  <header class="topbar">
    <div><h1>Kundenrechnungen</h1></div>
    {#if !tiles}<div class="actions">
        <button class="primary" onclick={createQuote} disabled={busy}
          ><Icon name="plus" size={16} /> Neues Angebot</button
        >
      </div>{/if}
  </header>
  <section class="invoice-dashboard">
    {@render tiles?.()}
    <DocumentTabs
      active="quotes"
      disabled={busy}
      onchange={(tab) => tab !== "quotes" && onshowtab(tab)}
    />
    <ListHeader
      count={listLoaded ? `${quotes.length} ${quotes.length === 1 ? "Angebot" : "Angebote"}` : ""}
      placeholder="Angebote suchen"
      bind:query
    />
    {#if quote && !quoteListed}<button class="resume-invoice" disabled={busy} onclick={resume}
        ><Icon name="invoices" /> {resumeLabel}</button
      >{/if}
    {#if visibleQuotes.length}<QuoteList
        quotes={visibleQuotes}
        currentId={quote?.id}
        onopen={openQuote}
        {busy}
      />{:else if listLoaded}<div class="empty-state">
        <h2>{query ? "Keine Treffer" : "Dein erstes Angebot"}</h2>
        <p>
          {query
            ? "Versuche es mit einer anderen Nummer oder einem anderen Kundennamen."
            : "Erstelle ein Angebot mit denselben Kunden, Artikeln und Steuerfällen wie bei deinen Rechnungen. Angenommene Angebote wandelst du mit einem Klick in eine Rechnung um."}
        </p>
      </div>{/if}
  </section>
{:else if step === "customer"}
  <PageHeader
    back={quote.document.customer.name ? "Zurück zum Angebot" : "Alle Angebote"}
    onback={() => {
      if (quote?.document.customer.name) step = "document";
      else showList();
    }}
    title={quote.document.customer.name ? "Anderen Kunden wählen" : "Neues Angebot"}
    description="An wen geht das Angebot? Wähle einen gespeicherten Kunden oder lege einen neuen an. Steuerfall und Rabatte des Kunden übernimmt die App."
    disabled={busy}
  />
  <CustomerChooser
    kind="quote"
    {customers}
    bind:customer={prospective}
    {busy}
    onchoose={chooseCustomer}
    oncreate={createCustomer}
    {quotes}
    onquotetemplate={useQuoteTemplate}
  />
{:else}
  <PageHeader
    back="Alle Angebote"
    onback={showList}
    title={quote.number ? `Angebot ${quote.number}` : "Angebot"}
    badge={quoteStatusLabel(quote.status)}
    badgeClass={quoteBadgeClass(quote.status)}
    disabled={busy}
  >
    {#snippet actions()}
      {#if editable}<span class="save-state" role="status"
          >{dirty || !quote?.id ? "Ungespeichert" : "Gespeichert"}</span
        ><button class="primary" disabled={busy || !(dirty || !quote?.id)} onclick={save}
          ><Icon name="save" size={16} /> {busy ? "Wird gespeichert …" : "Speichern"}</button
        >{:else}<button class="secondary" disabled={busy} onclick={openPDF}
          ><Icon name="open" size={16} /> PDF öffnen</button
        >{/if}
    {/snippet}
  </PageHeader>
  <InvoiceDocument
    bind:this={quoteDocument}
    kind="quote"
    bind:draft={quote.document}
    bind:validUntil={quote.validUntil}
    {editable}
    {company}
    disabled={busy || !editable}
    previewPending={pending}
    onchange={schedulePreview}
    onsettings={() => openSettings("")}
    onedittax={() => openSettings("taxTreatment")}
    {oneditcompany}
    onchangecustomer={() => {
      step = "customer";
      void scrollTop();
    }}
    oneditline={editLines}
    oneditorstatechange={(value) => (dialogDirty = value)}
    load={(document) =>
      API.QuoteDocumentHTML({ ...clone(quote!), document: { ...document, status: "draft" } })}
    renderKey={JSON.stringify([quote.id, quote.number, quote.validUntil, quote.status])}
  />

  <section class="invoice-completion no-print">
    <header class="completion-heading">
      <div>
        <h2>
          {editable
            ? "Angebot prüfen und abschließen"
            : quote.status === "sent"
              ? "Angebot abgeschlossen"
              : `Angebot ${quoteStatusLabel(quote.status).toLocaleLowerCase("de")}`}
        </h2>
        <p class="heading-subtitle">
          {editable ? "Abschluss" : "Antwort des Kunden"}
        </p>
        <p>
          {#if editable}
            Die angezeigte Angebotsnummer ist vorläufig. Beim Abschließen vergibt die App sie
            verbindlich; danach kannst du die PDF öffnen, speichern und versenden.
          {:else if quote.status === "converted"}
            In Rechnung {view?.invoiceNumber || ""} übernommen. Das Angebot bleibt unverändert.
          {:else if quote.status === "expired"}
            Die Gültigkeit ist abgelaufen. Nimmt dein Kunde das Angebot trotzdem an, kannst du es
            weiterhin als angenommen markieren oder in eine Rechnung umwandeln.
          {:else}
            {quote.number} · gültig bis {dateLabel(quote.validUntil)}{view?.totals.gross
              ? ` · Gesamt ${amountLabel(view.totals.gross)}`
              : ""}. Ein Angebot ist keine Rechnung und enthält keine E-Rechnungsdaten. Halte hier
            die Antwort deines Kunden fest.
          {/if}
        </p>
      </div>
      <span class="badge {quoteBadgeClass(quote.status)}">{quoteStatusLabel(quote.status)}</span>
    </header>
    {#if editable}<ValidationReport result={view?.validation} {pending} onresolve={resolve} />{/if}
    <footer class="completion-actions">
      <div class="draft-actions">
        {#if editable && quote.id}<button class="danger-text" onclick={remove} disabled={busy}
            >Entwurf löschen</button
          >{/if}
        {#if editable}<button class="secondary" onclick={schedulePreview} disabled={busy}
            ><Icon name="checkFile" size={16} /> Erneut prüfen</button
          >{/if}
        {#if quote.status === "sent" || quote.status === "expired"}
          <button class="secondary" disabled={busy} onclick={reopenQuote}
            ><Icon name="edit" size={16} /> Wieder bearbeiten</button
          >
          <button
            class="secondary"
            disabled={busy}
            onclick={() => changeStatus("rejected", "Angebot als abgelehnt markiert")}
            >Abgelehnt</button
          >
          <button
            class="secondary"
            disabled={busy}
            onclick={() => changeStatus("accepted", "Angebot als angenommen markiert")}
            ><Icon name="check" size={16} /> Angenommen</button
          >
        {:else if quote.status === "accepted" || quote.status === "rejected"}
          <button
            class="secondary"
            disabled={busy}
            onclick={() => changeStatus("sent", "Angebot wieder offen")}
            >Wieder auf offen setzen</button
          >
        {/if}
        {#if !editable}<button class="secondary" disabled={busy} onclick={exportPDF}
            ><Icon name="download" size={16} /> PDF speichern …</button
          >{/if}
        {#if !editable && quote.id && (quote.status === "sent" || quote.status === "expired")}<DocumentMailButton
            documentType="quote"
            documentId={quote.id}
            {busy}
            onsent={(text) => {
              notify(text);
              void loadMailLog();
              void refreshList();
            }}
            onerror={notify}
            onsettings={onmailsettings}
          />{/if}
        {#if lastMailed}<small class="quote-mailed" role="status"
            >Versendet {sentAtLabel(lastMailed.sentAt)} an {(lastMailed.to ?? []).join(", ")}</small
          >{/if}
      </div>
      <div class="issue-action">
        {#if editable}
          <span
            >{pending
              ? "Änderungen werden geprüft …"
              : view?.validation.state === "valid"
                ? "Nach dem Abschließen kannst du die PDF öffnen und versenden."
                : "Ergänze die oben genannten Angaben. Einen Entwurf kannst du jederzeit speichern."}</span
          >
          <button
            class="primary issue-button"
            disabled={busy || pending || view?.validation.state !== "valid"}
            onclick={finish}>Angebot abschließen</button
          >
        {:else if canConvertQuote(quote.status)}
          <button class="primary" disabled={busy} onclick={convert}
            ><Icon name="invoices" size={16} /> In Rechnung umwandeln</button
          >
        {:else if quote.status === "converted" && quote.invoiceId}
          <button class="primary" disabled={busy} onclick={() => onopeninvoice(quote!.invoiceId!)}
            ><Icon name="invoices" size={16} /> Rechnung öffnen</button
          >
        {/if}
      </div>
    </footer>
  </section>
{/if}

{#if settingsOpen && quote}<InvoiceSettingsDialog
    kind="quote"
    draft={quote.document}
    {company}
    {busy}
    {error}
    focusField={settingsFocus}
    onsave={saveSettings}
    ondirtychange={(value) => (dialogDirty = value)}
    oncancel={() => (settingsOpen = false)}
  />{/if}
{#if editingLines && quote}<PositionEditor
    quote
    lines={editingLines}
    {products}
    standardTax={quote.document.taxTreatment === "standard"}
    taxTreatment={quote.document.taxTreatment}
    mixedTax={quote.document.taxTreatment === "mixed"}
    defaultLineTax={company.smallBusiness ? "small-business" : "standard"}
    initialIndex={editingLineIndex}
    onsave={(lines) => {
      quote!.document.lines = clone(lines);
      editingLines = null;
      schedulePreview();
    }}
    ondirtychange={(value) => (dialogDirty = value)}
    oncancel={() => (editingLines = null)}
  />{/if}

<style>
  .quote-mailed {
    color: var(--theme-muted);
    font-size: 12px;
  }
</style>
