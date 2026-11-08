<script lang="ts">
  import { onMount, onDestroy, tick } from "svelte";
  import { scrollPageToTop } from "../../lib/scrollPage";
  import { Dialogs, Events } from "@wailsio/runtime";
  import * as API from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/app/invoiceservice";
  import type {
    ReceivedInvoice,
    ReceivedPreview,
  } from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/model/models";
  import { paymentMoney } from "../../lib/paymentWorkflow";
  import InvoicePayments from "../payments/InvoicePayments.svelte";
  import PaymentStatus from "../payments/PaymentStatus.svelte";
  import type {
    Expense,
    PaymentOverview,
  } from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/store/models";
  import ReceivedAttachments from "./ReceivedAttachments.svelte";
  import ReceivedImportDialog from "./ReceivedImportDialog.svelte";
  import ReceivedManageDialog from "./ReceivedManageDialog.svelte";
  import Icon from "../../components/Icon.svelte";
  import PageHeader from "../../components/PageHeader.svelte";
  import ValidationReport from "../../components/ValidationReport.svelte";
  import ExpenseList from "../expenses/ExpenseList.svelte";
  import ReceivedTaxBlock from "../expenses/ReceivedTaxBlock.svelte";
  import {
    awaitsInputTaxDecision,
    documentKindLabel,
    encodeInvoiceFile,
    inspectionLabel,
    originalOf,
    precedingLabel,
    receiptSubtitle,
  } from "../../lib/received";
  import { serviceErrorMessage } from "../../lib/errors";
  import { cached, remember } from "../../lib/cache";
  import ActionTiles from "../../components/ActionTiles.svelte";
  import Tabs from "../../components/Tabs.svelte";
  import ListHeader from "../../components/ListHeader.svelte";
  import EntryRow from "../../components/EntryRow.svelte";
  import { dateLabel } from "../../lib/format";
  import ReceiptIcon from "@lucide/svelte/icons/receipt";
  import FileWarningIcon from "@lucide/svelte/icons/file-warning";
  import RefreshCwIcon from "@lucide/svelte/icons/refresh-cw";
  import ArchiveIcon from "@lucide/svelte/icons/archive";
  import ArchiveRestoreIcon from "@lucide/svelte/icons/archive-restore";
  import FileXIcon from "@lucide/svelte/icons/file-x";
  import TrashIcon from "@lucide/svelte/icons/trash";
  import MoreMenu, { type MoreMenuItem } from "../../components/MoreMenu.svelte";
  import {
    addReceiptTile,
    isOpen,
    openTile,
    openTotals,
    type ActionTile,
  } from "../../lib/actionTiles";

  let {
    onbusychange,
    pickFile,
  }: {
    onbusychange: (busy: boolean) => void;
    pickFile: <T>(open: () => Promise<T>) => Promise<T | undefined>;
  } = $props();
  let expenseRevision = $state(0);
  let disposed = false;
  onDestroy(() => {
    disposed = true;
    onbusychange(false);
  });

  let paymentOverview = $state<PaymentOverview | null>(
    cached<PaymentOverview>("payments:overview") ?? null,
  );
  async function refreshPayments() {
    paymentOverview = remember("payments:overview", await API.Payments());
  }
  let items: ReceivedInvoice[] = $state(cached<ReceivedInvoice[]>("received:list") ?? []);
  let selected: ReceivedInvoice | null = $state(null);
  // The expense captured for a PDF without invoice data shows in its place.
  let linked = $state<Expense | null>(null);
  let pending: { filename: string; encoded: string; preview: ReceivedPreview } | null =
    $state(null);
  let management: "misimport" | "delete" | null = $state(null);
  // The views of the page; the receipt statuses plus the captured expenses.
  type View = "active" | "expenses" | "archived" | "misimport";
  let tab = $state<View>("active");
  const statusLabels: Record<string, string> = {
    active: "Aktuelle Belege",
    archived: "Archiv",
    misimport: "Fehlimporte",
    imported: "Hinzugefügt",
    rechecked: "Neu geprüft",
  };
  let busy = $state(false),
    error = $state(""),
    message = $state(""),
    query = $state("");
  let input: HTMLInputElement;
  let expenseList = $state<ExpenseList>();
  // The status tiles narrow the list to receipts that need attention.
  let focus = $state<"" | "open" | "review">("");
  // PDFs whose amounts were captured as a receipt; the receipt carries the date.
  const capturedIds = (expenses: Expense[]) =>
    new Set(expenses.filter((x) => x.receivedId && !x.cancelledAt).map((x) => x.receivedId!));
  // The expense list warmed by the prefetch, so the review count does not jump.
  let captured = $state(
    capturedIds(cached<{ expenses: Expense[] }>("expenses:data")?.expenses ?? []),
  );
  async function refreshCaptured() {
    captured = capturedIds((await API.Expenses()) ?? []);
  }
  const needsReview = (item: ReceivedInvoice) =>
    item.status !== "misimport" &&
    !captured.has(item.id) &&
    (!item.inspection.issueDate || awaitsInputTaxDecision(item.inspection));
  const incoming = $derived(
    (paymentOverview?.invoices ?? []).filter((b) => b.direction === "incoming"),
  );
  const openIds = $derived(new Set(incoming.filter(isOpen).map((b) => b.id)));
  // The same totals as the overview's tile; null until the payments are loaded.
  const payables = $derived(paymentOverview ? openTotals(incoming) : null);
  const reviewCount = $derived(items.filter(needsReview).length);
  // The received invoice that the open receipt corrects, if both are in the list.
  const original = $derived(selected ? originalOf(selected, items) : undefined);
  function toggleFocus(next: "open" | "review") {
    focus = focus === next ? "" : next;
    selected = null;
    tab = "active";
  }
  const misimports = $derived(items.filter((item) => item.status === "misimport").length);
  const tabs = $derived<{ id: View; label: string }[]>([
    { id: "active", label: "Belege" },
    { id: "expenses", label: "Ausgaben" },
    { id: "archived", label: "Archiv" },
    // Rare: only offered while there are any, or while it is open.
    ...(misimports || tab === "misimport"
      ? [{ id: "misimport" as const, label: "Fehlimporte" }]
      : []),
  ]);
  async function showExpenses() {
    tab = "expenses";
    focus = "";
    await tick();
    expenseList?.create();
  }
  const tiles = $derived<ActionTile[]>([
    {
      ...addReceiptTile(() => input.click()),
      primary: true,
      disabled: !!pending || !!management,
      // main.go accepts native file drops only on this element.
      attrs: {
        id: "received-file-drop",
        "data-file-drop-target": busy || pending || management ? undefined : "",
      },
    },
    {
      id: "expense",
      icon: ReceiptIcon,
      label: "Ausgabe erfassen",
      hint: "Kassenbon oder Quittung",
      title: "Kassenbon oder Ausgabe ohne E-Rechnung eintragen",
      onclick: () => void showExpenses(),
    },
    {
      id: "review",
      status: true,
      icon: FileWarningIcon,
      alert: reviewCount > 0,
      pressed: focus === "review",
      disabled: !reviewCount && focus !== "review",
      label: reviewCount
        ? `${reviewCount} ${reviewCount === 1 ? "Beleg" : "Belege"} prüfen`
        : "Keine offenen Belege",
      hint: reviewCount ? "Datum oder Prüfbefund klären" : "Nichts zu klären",
      title: reviewCount
        ? "Das Rechnungsdatum fehlt oder die Prüfung hat etwas gefunden, das die Vorsteuer betreffen kann"
        : undefined,
      onclick: () => toggleFocus("review"),
    },
    {
      ...openTile("out", payables, () => toggleFocus("open")),
      pressed: focus === "open",
      disabled: !payables?.count && focus !== "open",
    },
  ]);

  $effect(() => {
    const visibleMessage = message;
    if (!visibleMessage) return;
    const timer = window.setTimeout(() => {
      if (message === visibleMessage) message = "";
    }, 5000);
    return () => window.clearTimeout(timer);
  });

  const filtered = $derived(
    items.filter(
      (item) =>
        (focus === "review"
          ? needsReview(item)
          : focus === "open"
            ? openIds.has(item.id)
            : (item.status || "active") === tab) &&
        [item.filename, item.inspection.invoiceNumber, item.inspection.sellerName]
          .join(" ")
          .toLocaleLowerCase()
          .includes(query.toLocaleLowerCase()),
    ),
  );
  async function perform(action: () => Promise<void>) {
    if (busy) return;
    busy = true;
    onbusychange(true);
    error = "";
    message = "";
    try {
      await action();
    } catch (e) {
      error = serviceErrorMessage(e, "Das hat leider nicht geklappt. Versuche es noch einmal.");
    } finally {
      busy = false;
      if (!disposed) onbusychange(false);
    }
  }
  // The first load runs outside perform(): going busy would disable the
  // controls and flash "Datei wird gelesen …" every time the page opens.
  let loaded = $state(cached("received:list") !== undefined);
  onMount(() => {
    void (async () => {
      try {
        items = remember("received:list", (await API.ListReceivedInvoices()) ?? []);
        await Promise.all([refreshPayments(), refreshCaptured()]);
      } catch (e) {
        error = serviceErrorMessage(e, "Das hat leider nicht geklappt. Versuche es noch einmal.");
      } finally {
        loaded = true;
      }
    })();
    return Events.On("received:file-drop", (event) => {
      if (busy || pending || management) return;
      const file = event.data as { filename: string; encoded: string; error: string };
      if (file.error) {
        error = file.error;
        message = "";
        return;
      }
      void perform(async () => {
        const preview = await API.PrepareReceivedInvoice(file.filename, file.encoded);
        pending = { filename: file.filename, encoded: file.encoded, preview };
      });
    });
  });
  async function addFile(event: Event) {
    const element = event.currentTarget as HTMLInputElement;
    const file = element.files?.[0];
    element.value = "";
    if (!file) return;
    await perform(async () => {
      const encoded = await encodeInvoiceFile(file);
      const preview = await API.PrepareReceivedInvoice(file.name, encoded);
      pending = { filename: file.name, encoded, preview };
    });
  }
  async function confirmImport() {
    if (!pending) return;
    const file = pending;
    await perform(async () => {
      const result = await API.ImportReceivedInvoice(file.filename, file.encoded);
      selected = result.invoice;
      tab = (selected.status || "active") as View;
      query = "";
      pending = null;
      items = (await API.ListReceivedInvoices()) ?? [];
      await refreshPayments();
      message = result.duplicate
        ? "Dieser Beleg ist schon gespeichert."
        : "Original gespeichert. Das Prüfergebnis findest du unten.";
    });
  }
  function openExisting() {
    if (!pending?.preview.duplicate) return;
    selected = pending.preview.duplicate;
    tab = (selected.status || "active") as View;
    query = "";
    pending = null;
  }
  async function changeStatus(status: string, reason = "") {
    if (!selected) return;
    const id = selected.id;
    await perform(async () => {
      selected = await API.ChangeReceivedStatus(id, status, reason);
      items = items.map((item) => (item.id === id ? selected! : item));
      management = null;
      tab = status as View;
      await refreshPayments();
      message =
        status === "active"
          ? "Beleg wiederhergestellt."
          : status === "archived"
            ? "Beleg archiviert. Das Original bleibt erhalten."
            : "Fehlimport gekennzeichnet. Das Original bleibt erhalten.";
    });
  }
  async function confirmManagement(reason: string, filename: string, confirmed: boolean) {
    if (management === "misimport") {
      await changeStatus("misimport", reason);
      return;
    }
    if (!selected) return;
    const id = selected.id;
    await perform(async () => {
      await API.DeleteReceivedMisimport(id, filename, confirmed);
      items = items.filter((item) => item.id !== id);
      selected = null;
      management = null;
      await refreshPayments();
      message =
        "Fehlimport endgültig aus der App gelöscht. Vorhandene Sicherungen und Exporte bleiben bestehen.";
    });
  }
  async function recheck() {
    if (!selected) return;
    const item = selected;
    await perform(async () => {
      const inspection = await API.RecheckReceivedInvoice(item.id);
      selected = { ...item, inspection };
      items = items.map((row) => (row.id === item.id ? selected! : row));
      message = "Prüfung aktualisiert. Die Originaldatei bleibt unverändert.";
    });
  }
  async function saveOriginal() {
    if (!selected) return;
    const item = selected;
    await perform(async () => {
      const path = await pickFile(() =>
        Dialogs.SaveFile({ Title: "Original speichern", Filename: item.filename }),
      );
      if (!path) return;
      await API.SaveReceivedOriginal(item.id, path);
      message = "Originaldatei gespeichert.";
    });
  }
  // Less frequent receipt actions sit in one "Mehr" menu next to the main buttons.
  const moreItems = $derived.by((): MoreMenuItem[] => {
    if (!selected) return [];
    const item = selected;
    const manage = (action: "misimport" | "delete") => () => {
      error = "";
      management = action;
    };
    return [
      {
        id: "recheck",
        icon: RefreshCwIcon,
        label: "Erneut prüfen",
        hint: "Prüft die Rechnungsdaten neu. Die Datei bleibt unverändert.",
        onclick: recheck,
      },
      (item.status || "active") === "active"
        ? {
            id: "archive",
            icon: ArchiveIcon,
            label: "Archivieren",
            hint: "Nimmt den Beleg aus der Liste. Das Original bleibt erhalten.",
            onclick: () => changeStatus("archived"),
          }
        : {
            id: "restore",
            icon: ArchiveRestoreIcon,
            label: "Wiederherstellen",
            hint: "Holt den Beleg zurück in die Liste.",
            onclick: () => changeStatus("active"),
          },
      item.status !== "misimport"
        ? {
            id: "misimport",
            icon: FileXIcon,
            label: "Als Fehlimport kennzeichnen",
            hint: "Für doppelte oder falsche Dateien. Sie zählen dann nicht mehr mit.",
            onclick: manage("misimport"),
          }
        : {
            id: "delete",
            icon: TrashIcon,
            danger: true,
            label: "Endgültig löschen …",
            hint: "Entfernt den Fehlimport aus der App.",
            onclick: manage("delete"),
          },
    ];
  });
  // Same two-digit form as the invoice dates next to it.
  const importedLabel = (value: string) =>
    new Date(value).toLocaleDateString("de-DE", {
      day: "2-digit",
      month: "2-digit",
      year: "numeric",
    });
  export function showList() {
    selected = null;
    error = "";
  }
  function open(item: ReceivedInvoice) {
    selected = item;
    message = "";
    error = "";
    void tick().then(() => scrollPageToTop());
  }
</script>

<input
  bind:this={input}
  type="file"
  class="hidden-file"
  accept=".xml,.pdf,application/pdf,application/xml,text/xml"
  onchange={addFile}
  disabled={busy}
/>
{#if selected}
  <!-- A receipt opens as its own page, like a customer or an invoice. -->
  <PageHeader
    back="Alle Belege"
    onback={() => {
      selected = null;
      error = "";
    }}
    title={selected.inspection.sellerName || linked?.supplier || selected.filename}
    description="{selected.receiptNumber ? `${selected.receiptNumber} · ` : ''}{selected.inspection
      .invoiceNumber || 'Ohne Rechnungsnummer'} · {selected.filename} · {Math.ceil(
      selected.size / 1024,
    )} KB"
    badge={selected.status === "archived"
      ? "Archiviert"
      : selected.status === "misimport"
        ? "Fehlimport"
        : ""}
    disabled={busy}
  >
    {#snippet actions()}
      {#if selected!.kind === "pdf"}<button
          class="secondary"
          disabled={busy}
          onclick={() =>
            perform(async () => {
              await API.OpenReceivedPDF(selected!.id);
            })}><Icon name="open" size={16} /> PDF öffnen</button
        >{/if}
      <button class="secondary" disabled={busy} onclick={saveOriginal}
        ><Icon name="download" size={16} /> Original speichern</button
      >
      <MoreMenu items={moreItems} disabled={busy} title="Weitere Aktionen für diesen Beleg" />
    {/snippet}
  </PageHeader>
  <section
    class="invoice-dashboard received-detail"
    aria-busy={busy}
    aria-label="Beleg und Prüfergebnis"
  >
    {#if error}<p class="received-error" role="alert">{error}</p>{/if}
    <p class="received-status" role="status">{busy ? "Wird verarbeitet …" : message}</p>
    <dl class="received-metadata">
      <div>
        <dt>Lieferant</dt>
        <dd>{selected.inspection.sellerName || linked?.supplier || "—"}</dd>
      </div>
      <div>
        <dt>Empfänger</dt>
        <dd>{selected.inspection.buyerName || "—"}</dd>
      </div>
      <div>
        <dt>Rechnungsdatum</dt>
        <dd>
          {selected.inspection.issueDate || linked?.date
            ? dateLabel(selected.inspection.issueDate || linked!.date)
            : "—"}
        </dd>
      </div>
      <div>
        <dt>Gesamtbetrag</dt>
        <dd>
          {selected.inspection.totals.gross || linked?.gross
            ? paymentMoney(
                selected.inspection.totals.gross || linked!.gross,
                selected.inspection.currency || "EUR",
              )
            : "—"}
        </dd>
      </div>
      {#if precedingLabel(selected.inspection)}
        <div>
          <dt>{documentKindLabel(selected.inspection) || "Bezug"} zu Rechnung</dt>
          <dd>
            {precedingLabel(selected.inspection)}
            {#if original}<button class="text" disabled={busy} onclick={() => open(original)}
                >Rechnung öffnen</button
              >{/if}
          </dd>
        </div>
      {/if}
      {#if selected.inspection.totals.prepaid}
        <div>
          <dt>Bereits gezahlt (abgesetzt)</dt>
          <dd>
            {paymentMoney(
              selected.inspection.totals.prepaid,
              selected.inspection.currency || "EUR",
            )}
          </dd>
        </div>
        <div>
          <dt>Zu zahlen</dt>
          <dd>
            {paymentMoney(selected.inspection.totals.due, selected.inspection.currency || "EUR")}
          </dd>
        </div>
      {/if}
    </dl>
    {#if selected.inspection.totals.prepaid}<p class="form-help">
        Diese Rechnung setzt bereits gezahlte Beträge ab. Der offene Betrag richtet sich nach dem
        Zahlbetrag.
      </p>{/if}
    {#if original && !selected.inspection.inputTax?.choice && !original.inspection.inputTax?.choice}<p
        class="form-help"
      >
        Eine Berichtigung kann die Ursprungsrechnung ersetzen oder ergänzen. Ersetzt sie sie, steht
        deren Vorsteuer doppelt in der Übersicht. Prüfe, welche Rechnung gilt, und entscheide es
        unten oder in der Ursprungsrechnung.
      </p>{/if}
    <ReceivedAttachments id={selected.id} {pickFile} />
    <ReceivedTaxBlock
      bind:linked
      item={selected}
      onexpense={() => {
        expenseRevision++;
        void refreshCaptured().catch(() => {});
      }}
      ondecision={(inspection) => {
        const id = selected!.id;
        selected = { ...selected!, inspection };
        items = items.map((row) => (row.id === id ? selected! : row));
      }}
    />
    {#if selected.status !== "misimport"}<InvoicePayments
        invoiceID={selected.id}
        direction="incoming"
        onchanged={refreshPayments}
        {onbusychange}
      />{/if}
    <div class="detail-list">
      <details>
        <summary
          ><span class="detail-title">Rechnungsprüfung</span><span
            class="detail-state"
            class:good={selected.inspection.validation.state === "valid"}
            class:bad={selected.inspection.validation.state !== "valid"}
            >{selected.inspection.validation.state === "valid"
              ? "Bestanden"
              : inspectionLabel(selected.inspection)}</span
          ></summary
        ><ValidationReport
          result={selected.inspection.validation}
          received
          harmless={selected.inspection.inputTax?.allowed}
        />
        <p class="received-note">
          Geprüft werden die strukturierten Rechnungsdaten. PDF/A-Konformität, Signaturen und die
          Übereinstimmung mit dem sichtbaren PDF-Inhalt werden nicht geprüft. Kontrolliere Inhalt
          und Zahlungsempfänger bitte selbst.
        </p>
      </details>
      {#if selected.inspection.profile}<details>
          <summary
            ><span class="detail-title">Technisches Format</span><span class="detail-state"
              >{selected.inspection.syntax.toUpperCase()}</span
            ></summary
          >
          <p class="received-profile">
            {selected.inspection.syntax.toUpperCase()} · {selected.inspection.profile}
          </p>
        </details>{/if}

      {#if selected.history?.length}<details>
          <summary
            ><span class="detail-title">Verlauf</span><span class="detail-state"
              >{selected.history.length}
              {selected.history.length === 1 ? "Eintrag" : "Einträge"}</span
            ></summary
          >
          <ul class="received-history">
            {#each selected.history as event}<li>
                <strong>{statusLabels[event.action] || event.action}</strong> · {new Date(
                  event.at,
                ).toLocaleString("de-DE")}{#if event.reason}<p>{event.reason}</p>{/if}
              </li>{/each}
          </ul>
        </details>{/if}
    </div>
  </section>
{:else}
  <header class="topbar">
    <div>
      <h1>Ausgaben & Belege</h1>
    </div>
  </header>
  <section class="invoice-dashboard received-workspace" aria-busy={busy}>
    <ActionTiles {tiles} disabled={busy} />
    {#if error}<p class="received-error" role="alert">{error}</p>{/if}
    <p class="received-status" role="status">{busy ? "Datei wird gelesen …" : message}</p>
    <Tabs
      {tabs}
      active={tab}
      label="Belegansichten"
      disabled={busy}
      onchange={(next) => {
        tab = next;
        focus = "";
      }}
    />
    {#if tab === "expenses"}
      <ExpenseList bind:this={expenseList} revision={expenseRevision} />
    {:else}
      <ListHeader
        count="{filtered.length} {filtered.length === 1 ? 'Beleg' : 'Belege'}"
        placeholder="Belege suchen"
        bind:query
        filter={focus === "review"
          ? "Nur Belege zum Prüfen"
          : focus === "open"
            ? "Nur offene Rechnungen"
            : ""}
        onclearfilter={() => (focus = "")}
      />
      {#if filtered.length}<div class="invoice-rows">
          {#each filtered as item (item.id)}
            {@const balance = paymentOverview?.invoices?.find(
              (b) => b.id === item.id && b.direction === "incoming",
            )}
            <EntryRow
              title={item.inspection.sellerName || item.filename}
              subtitle="{item.receiptNumber ? `${item.receiptNumber} · ` : ''}{receiptSubtitle(
                item,
              )}"
              meta={item.inspection.issueDate
                ? dateLabel(item.inspection.issueDate)
                : importedLabel(item.importedAt)}
              amount={item.inspection.totals.gross
                ? paymentMoney(item.inspection.totals.gross, item.inspection.currency || "EUR")
                : ""}
              tooltip="Beleg öffnen"
              disabled={busy}
              onclick={() => open(item)}
            >
              {#snippet status()}
                {#if balance}<PaymentStatus {balance} />{:else}<span></span>{/if}
              {/snippet}
            </EntryRow>
          {/each}
        </div>{:else if loaded}<div class="empty-state">
          <h2>
            {query
              ? "Keine Treffer"
              : tab === "active"
                ? "Noch keine Belege"
                : "Keine Belege in dieser Ansicht"}
          </h2>
          <p>
            {query
              ? "Versuche es mit einem anderen Lieferanten, einer Rechnungsnummer oder einem Dateinamen."
              : tab === "active"
                ? "Lege eine E-Rechnung oder ein PDF auf „Beleg hinzufügen“ oder klicke darauf (bis 20 MB)."
                : "Archivierte Belege und Fehlimporte erscheinen hier."}
          </p>
        </div>{/if}
    {/if}
  </section>
{/if}

{#if pending}<ReceivedImportDialog
    {...pending}
    {busy}
    {error}
    onclose={() => {
      pending = null;
      error = "";
    }}
    onconfirm={confirmImport}
    onexisting={openExisting}
  />{/if}
{#if management && selected}<ReceivedManageDialog
    invoice={selected}
    action={management}
    {busy}
    {error}
    onclose={() => {
      management = null;
      error = "";
    }}
    onconfirm={confirmManagement}
  />{/if}

<style>
  .received-history {
    padding-left: 18px;
    font-size: 12px;
  }
  .received-history li {
    margin: 12px 0;
    overflow-wrap: anywhere;
  }
  .received-history p {
    margin: 4px 0;
  }
  .received-status:empty {
    display: none;
  }
  .received-workspace {
    padding: 0 0 24px;
  }
  .received-status {
    min-height: 1.4em;
    margin: 0 0 12px;
    color: var(--theme-muted);
  }
  .received-error {
    color: var(--theme-error-text);
  }
  .received-detail {
    min-width: 0;
    padding: 20px;
    border: 1px solid var(--theme-border);
    border-radius: 12px;
    background: var(--theme-surface);
    font-size: 13px;
  }
  h2 {
    margin: 0;
    font-size: 18px;
  }
  .received-profile {
    overflow-wrap: anywhere;
  }
  .received-metadata {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 16px;
    margin: 0 0 20px;
  }
  .received-metadata > div {
    min-width: 0;
  }
  dt {
    color: var(--theme-muted);
    font-size: 12px;
    margin-bottom: 4px;
  }
  dd {
    margin: 0;
    overflow-wrap: anywhere;
  }
  .received-note {
    margin: 16px 0 0;
    font-size: 12px;
    color: var(--theme-muted);
    line-height: 1.5;
  }
</style>
