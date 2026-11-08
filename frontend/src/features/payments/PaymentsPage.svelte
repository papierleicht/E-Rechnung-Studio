<script lang="ts">
  import SettingSection from "../../components/SettingSection.svelte";
  import { onDestroy, tick, untrack } from "svelte";
  import * as API from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/app/invoiceservice";
  import type {
    Reconciliation,
    BookingReview,
    PaymentBooking,
  } from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/app/models";
  import type { Payment } from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/store/models";
  import FormDialog from "../../components/FormDialog.svelte";
  import PlusHint from "../../components/PlusHint.svelte";
  import FilePicker from "../../components/FilePicker.svelte";
  import InvoicePayments from "./InvoicePayments.svelte";
  import ExpenseDialog from "../expenses/ExpenseDialog.svelte";
  import { decodeCSV } from "../../lib/paymentWorkflow";
  import { Events } from "@wailsio/runtime";
  import type { Expense } from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/store/models";
  import type { Category } from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/taxforms/models";
  import { paymentMoney as money } from "../../lib/paymentWorkflow";
  import { dateLabel } from "../../lib/format";
  import { serviceErrorMessage } from "../../lib/errors";
  import { cached, remember } from "../../lib/cache";
  import ListHeader from "../../components/ListHeader.svelte";
  import EntryRow from "../../components/EntryRow.svelte";
  import PaymentStatus from "./PaymentStatus.svelte";
  import AccountFilter from "./AccountFilter.svelte";
  import {
    saveBookingOrder,
    savedBookingOrder,
    sortByDate,
    type AccountChoice,
    type BookingOrder,
  } from "../../lib/bankAccounts";
  let {
    revision = 0,
    view = "pending",
    csvOpen = $bindable(false),
    accountID = "",
    accountLabel = "",
    accounts = [],
    onchanged = async () => {},
    onallaccounts = () => {},
    onbusychange = (_busy: boolean) => {},
    onpendingchange = (_count: number) => {},
  }: {
    revision?: number;
    /** The list the bank page shows; null keeps only the dialogs (e.g. on the accounts tab). */
    view?: "pending" | "hidden" | "history" | null;
    csvOpen?: boolean;
    accountID?: string;
    /** Names the selected account in the filter chip. */
    accountLabel?: string;
    /** The retrieved accounts, named for the filter; CSV accounts join by their IBAN. */
    accounts?: AccountChoice[];
    onchanged?: () => Promise<void>;
    onallaccounts?: () => void;
    onbusychange?: (busy: boolean) => void;
    /** Reports the bookings still waiting for an assignment once they are known. */
    onpendingchange?: (count: number) => void;
  } = $props();
  // The last result shows right away while the page reconciles again.
  let result = $state<Reconciliation | null>(
      cached<Reconciliation>("payments:reconciliation") ?? null,
    ),
    busy = $state(false),
    error = $state(""),
    notice = $state("");
  let activeBooking = $state(""),
    chosen = $state(""),
    allocation = $state(""),
    historyInvoice = $state(""),
    historyDirection = $state("outgoing");
  let csv = $state(""),
    csvFilename = $state(""),
    account = $state(""),
    preview = $state<PaymentBooking[] | null>(null),
    previewCSV = $state(""),
    importedRows = $state(0),
    previewAccount = $state("");
  let treatmentKind = $state("other"),
    treatmentNote = $state(""),
    hideBooking = $state(""),
    hiddenSearch = $state("");
  const treatmentLabels: Record<string, string> = {
    other: "Keine Rechnungszuordnung nötig",
    reserve: "Rücklagen",
    transfer: "Umbuchung zwischen eigenen Konten",
    private: "Private Zahlung",
    vat: "Umsatzsteuer an oder vom Finanzamt",
    tradetax: "Gewerbesteuer an oder von der Gemeinde",
    tax: "Private Steuer (z. B. Einkommensteuer)",
  };
  // Outgoing bookings without invoice can become a receipt (bank fee,
  // subscription, card payment).
  let expenseDraft = $state<Partial<Expense> | null>(null),
    expenseCategories = $state<Category[]>([]),
    smallBusiness = $state(false);
  async function captureExpense(review: BookingReview) {
    error = "";
    try {
      const [cats, setup] = await Promise.all([API.ExpenseCategories(), API.TaxSetup()]);
      expenseCategories = cats ?? [];
      smallBusiness = setup.smallBusiness;
      const date = review.booking.data.booking_date;
      expenseDraft = {
        bookingId: review.booking.id,
        supplier: review.booking.data.counterparty_name || review.booking.data.description || "",
        note: review.booking.data.description || "",
        gross: review.remaining.replace("-", ""),
        date,
        paidDate: date,
        paidWith: "bank",
      };
    } catch (cause) {
      error = serviceErrorMessage(
        cause,
        "Der Beleg konnte nicht vorbereitet werden. Versuche es noch einmal.",
      );
    }
  }
  let disposed = false,
    request = 0;
  // The account the lists are limited to: the bank page's choice, switched here in one click.
  let filterID = $state(untrack(() => accountID));
  $effect(() => {
    filterID = accountID;
  });
  let order = $state<BookingOrder>(savedBookingOrder());
  function setOrder(next: BookingOrder) {
    order = next;
    saveBookingOrder(next);
  }
  const accountChoices = $derived.by(() => {
    const known = new Set(accounts.map((item) => item.id));
    const others = new Set<string>();
    for (const r of [...(result?.pending ?? []), ...(result?.other ?? [])])
      if (!known.has(r.booking.accountId)) others.add(r.booking.accountId);
    return [...accounts, ...[...others].sort().map((id) => ({ id, name: id, detail: "" }))].filter(
      (item) => item.id,
    );
  });
  const filterNames = $derived(
    accountChoices.length > 1 && (view === "pending" || view === "hidden"),
  );
  // A filter chosen here on an account that is gone, e.g. just removed, would
  // show an empty list as if everything were assigned.
  $effect(() => {
    if (
      result &&
      filterID &&
      filterID !== accountID &&
      !accountChoices.some((item) => item.id === filterID)
    )
      filterID = "";
  });
  const accountCounts = $derived.by(() => {
    const counts: Record<string, number> = {};
    for (const item of accountChoices) counts[item.id] = 0;
    for (const r of (view === "hidden" ? result?.other : result?.pending) ?? [])
      counts[r.booking.accountId] = (counts[r.booking.accountId] ?? 0) + 1;
    return counts;
  });
  const pending = $derived(
    result?.pending?.filter((r) => !filterID || r.booking.accountId === filterID) ?? [],
  );
  $effect(() => {
    if (result) onpendingchange(result.pending?.length ?? 0);
  });
  let pendingSearch = $state(""),
    historySearch = $state(""),
    restoringId = $state("");
  const matches = (query: string, ...values: (string | undefined)[]) =>
    values.join(" ").toLocaleLowerCase("de-DE").includes(query.trim().toLocaleLowerCase("de-DE"));
  const visiblePending = $derived(
    sortByDate(
      pending.filter((r) =>
        matches(
          pendingSearch,
          r.booking.data.counterparty_name,
          r.booking.data.description,
          dateLabel(r.booking.data.booking_date),
          money(r.remaining, r.booking.data.currency),
        ),
      ),
      (r) => r.booking.data.booking_date,
      order,
    ),
  );
  const active = $derived(pending.find((r) => r.booking.id === activeBooking));
  const hiding = $derived(pending.find((r) => r.booking.id === hideBooking));
  const hidden = $derived(
    result?.other?.filter((r) => !filterID || r.booking.accountId === filterID) ?? [],
  );
  const filteredHidden = $derived(
    sortByDate(
      hidden.filter((r) =>
        [
          r.booking.data.counterparty_name,
          r.booking.data.description,
          r.treatment.note,
          treatmentLabels[r.treatment.kind],
          dateLabel(r.booking.data.booking_date),
          bookingMoney(r.booking),
        ]
          .join(" ")
          .toLocaleLowerCase("de-DE")
          .includes(hiddenSearch.trim().toLocaleLowerCase("de-DE")),
      ),
      (r) => r.booking.data.booking_date,
      order,
    ),
  );
  const filterLabel = $derived(
    filterID && !filterNames
      ? `Nur ${accountChoices.find((item) => item.id === filterID)?.name || accountLabel || "dieses Konto"}`
      : "",
  );
  const restoring = $derived(hidden.find((r) => r.booking.id === restoringId));
  const invoiceOf = (p: Payment) =>
    result?.overview.invoices?.find((b) => b.id === p.invoiceId && b.direction === p.direction);
  const payments = $derived(
    (result?.overview.payments ?? []).filter((p) =>
      matches(
        historySearch,
        invoiceOf(p)?.number,
        invoiceOf(p)?.partner,
        dateLabel(p.date),
        money(p.amount, p.currency),
      ),
    ),
  );
  const availableInvoices = $derived(
    // A booking only settles what is still open, in its own direction.
    result?.overview.invoices?.filter(
      (b) =>
        b.currency === active?.booking.data.currency &&
        Number(b.remaining) > 0 &&
        (active?.remaining.startsWith("-")
          ? b.direction === "incoming" || b.credit
          : b.direction === "outgoing" && !b.credit),
    ) ?? [],
  );
  const scale = (currency: string) =>
    new Intl.NumberFormat("de-DE", { style: "currency", currency }).resolvedOptions()
      .maximumFractionDigits ?? 2;
  const bookingMoney = (b: PaymentBooking) =>
    money(b.data.amount / 10 ** scale(b.data.currency), b.data.currency);
  async function refresh() {
    const v = ++request;
    try {
      const r = await API.ReconcilePayments();
      if (disposed || v !== request) return;
      result = remember("payments:reconciliation", r);
      if (r.automatic)
        notice = `${r.automatic} ${r.automatic === 1 ? "Zahlung" : "Zahlungen"} automatisch zugeordnet.`;
      await onchanged();
    } catch (e) {
      if (!disposed && v === request)
        error = serviceErrorMessage(
          e,
          "Der Bankabgleich hat nicht geklappt. Versuche es gleich noch einmal.",
        );
    }
  }
  $effect(() => {
    const r = revision;
    void r;
    void refresh();
  });
  onDestroy(() => {
    disposed = true;
    request++;
    onbusychange(false);
  });
  async function run(action: () => Promise<unknown>) {
    if (busy) return;
    busy = true;
    onbusychange(true);
    error = "";
    notice = "";
    try {
      await action();
      await refresh();
    } catch (e) {
      error = serviceErrorMessage(
        e,
        "Die Zuordnung konnte nicht gespeichert werden. Versuche es noch einmal.",
      );
    } finally {
      busy = false;
      if (!disposed) onbusychange(false);
    }
  }
  function selectAccount(id: string) {
    filterID = id;
    // "All accounts" also leaves the account the bank page has open.
    if (!id) onallaccounts();
  }
  let reviewTrigger: HTMLButtonElement | null = null;
  function closeReview() {
    if (busy) return;
    activeBooking = "";
    hideBooking = "";
    void tick().then(() => {
      if (reviewTrigger?.isConnected) reviewTrigger.focus({ preventScroll: true });
    });
  }
  function choose(r: BookingReview, trigger: HTMLButtonElement) {
    reviewTrigger = trigger;
    error = "";
    activeBooking = r.booking.id;
    chosen = "";
    allocation = String(Math.abs(Number(r.remaining)));
  }
  function chooseHide(r: BookingReview, trigger?: HTMLButtonElement) {
    if (trigger) reviewTrigger = trigger;
    error = "";
    treatmentKind = "other";
    treatmentNote = "";
    activeBooking = "";
    hideBooking = r.booking.id;
  }
  async function hide() {
    if (!hiding) return;
    await run(async () => {
      await API.SetBookingTreatment(hiding.booking.id, treatmentKind, treatmentNote);
      hideBooking = "";
      hiddenSearch = "";
      notice = "Umsatz ausgeblendet. Du findest ihn im Tab „Ausgeblendet“.";
    });
  }
  async function restore() {
    if (!restoring) return;
    const id = restoring.booking.id;
    await run(async () => {
      await API.SetBookingTreatment(id, "", "Ausgeblendeten Umsatz wieder angezeigt");
      restoringId = "";
      notice = "Umsatz wieder angezeigt. Du kannst ihn jetzt im Tab „Zuordnen“ prüfen.";
    });
  }
  // Takes the CSV the user picked or dropped; the preview starts over.
  function takeCSV(name: string, data: ArrayBuffer) {
    csv = decodeCSV(data);
    csvFilename = name;
    preview = null;
  }
  // A bank statement dropped on the "CSV importieren" tile or into the open
  // dialog: main.go reads the file and reports it here.
  $effect(() =>
    Events.On("payments:csv-drop", (event) => {
      if (busy) return;
      const file = event.data as { filename: string; encoded: string; error: string };
      csvOpen = true;
      if (file.error) {
        error = file.error;
        return;
      }
      error = "";
      takeCSV(file.filename, Uint8Array.from(atob(file.encoded), (c) => c.charCodeAt(0)).buffer);
    }),
  );
  async function checkCSV() {
    await run(async () => {
      preview = await API.PreviewPaymentCSV(account, csv);
      previewCSV = csv;
      previewAccount = account;
    });
  }
  async function importCSV() {
    if (!preview || csv !== previewCSV || account !== previewAccount) return;
    await run(async () => {
      const n = await API.ImportPaymentCSV(account, csv);
      onallaccounts();
      preview = null;
      csv = "";
      csvFilename = "";
      csvOpen = false;
      notice = `${n} Bankumsätze importiert. Ordne sie jetzt zu.`;
      importedRows = n;
    });
  }
  async function assign() {
    if (!active || !chosen) return;
    if (chosen.startsWith("expense:")) {
      const expenseId = chosen.slice("expense:".length);
      await run(async () => {
        await API.AssignBookingToExpense(active.booking.id, expenseId);
        activeBooking = "";
        notice = "Zuordnung gespeichert. Die Ausgabe ist als bezahlt vermerkt.";
      });
      return;
    }
    const target = result?.overview.invoices?.find((b) => `${b.direction}:${b.id}` === chosen);
    if (!target) return;
    await run(async () => {
      await API.RecordPayment({
        invoiceId: target.id,
        direction: target.direction,
        bookingId: active.booking.id,
        source: active.booking.source,
        amount: allocation.replace(",", "."),
        reason: "Bankumsatz nach Prüfung zugeordnet",
      } as Payment);
      activeBooking = "";
      notice = "Zuordnung gespeichert. Die Rechnung ist aktualisiert.";
    });
  }
</script>

<section class="bank-review" aria-label="Bankabgleich">
  {#if error && !active && !hiding && !restoring && !csvOpen}<p role="alert" class="error">
      {error}
    </p>{/if}{#if notice}<p role="status" class="success">
      {notice}
    </p>{/if}{#if importedRows}<PlusHint
      id="bank-csv-done"
      topic="fints"
      text={`Das waren ${importedRows} Umsätze von Hand. Mit Plus holst du sie künftig per Klick von deiner Bank.`}
    />{/if}
  {#if filterNames}<AccountFilter
      accounts={accountChoices}
      selected={filterID}
      counts={accountCounts}
      onselect={selectAccount}
    />{/if}
  {#if view === "pending"}
    <ListHeader
      count="{visiblePending.length} {visiblePending.length === 1
        ? 'Umsatz ohne Zuordnung'
        : 'Umsätze ohne Zuordnung'}"
      placeholder="Umsätze suchen"
      bind:query={pendingSearch}
      filter={filterLabel}
      onclearfilter={() => selectAccount("")}
      sort={order}
      onsort={setOrder}
    />
    {#if !result && !error}<p role="status" class="hint">Zahlungen werden abgeglichen …</p>
    {:else if visiblePending.length}<div class="invoice-rows">
        {#each visiblePending as review (review.booking.id)}
          <EntryRow
            title={review.booking.data.counterparty_name || "Bankumsatz"}
            subtitle={review.booking.data.description || "Ohne Verwendungszweck"}
            meta={dateLabel(review.booking.data.booking_date)}
            amount={money(review.remaining, review.booking.data.currency)}
            tooltip="Umsatz prüfen und zuordnen"
            disabled={busy}
            onclick={(event) => choose(review, event.currentTarget)}
          >
            {#snippet status()}
              <PaymentStatus
                summary={review.suggestions?.length
                  ? { title: "Rechnung gefunden", description: "bitte prüfen" }
                  : { title: "Ohne Zuordnung", description: "Rechnung wählen oder ausblenden" }}
              />
            {/snippet}
          </EntryRow>
        {/each}
      </div>
    {:else if result}<div class="empty-state">
        <h2>{pendingSearch ? "Keine Treffer" : "Alles zugeordnet"}</h2>
        <p>
          {pendingSearch
            ? "Versuche es mit einem anderen Namen, Datum oder Betrag."
            : "Eindeutige Zahlungseingänge ordnet die App automatisch zu. Nur unklare Umsätze musst du selbst prüfen."}
        </p>
      </div>{/if}
  {:else if view === "hidden"}
    <ListHeader
      count="{filteredHidden.length} {filteredHidden.length === 1
        ? 'ausgeblendeter Umsatz'
        : 'ausgeblendete Umsätze'}"
      placeholder="Ausgeblendete suchen"
      bind:query={hiddenSearch}
      filter={filterLabel}
      onclearfilter={() => selectAccount("")}
      sort={order}
      onsort={setOrder}
    />
    {#if filteredHidden.length}<div class="invoice-rows">
        {#each filteredHidden as r (r.booking.id)}
          <EntryRow
            title={r.booking.data.counterparty_name || "Bankumsatz"}
            subtitle="{treatmentLabels[r.treatment.kind]} · {r.booking.data.description ||
              'Ohne Verwendungszweck'}"
            meta={dateLabel(r.booking.data.booking_date)}
            amount={bookingMoney(r.booking)}
            tooltip="Ausgeblendeten Umsatz ansehen"
            disabled={busy}
            onclick={(event) => {
              reviewTrigger = event.currentTarget;
              error = "";
              restoringId = r.booking.id;
            }}
          />
        {/each}
      </div>{:else}<div class="empty-state">
        <h2>{hiddenSearch ? "Keine Treffer" : "Nichts ausgeblendet"}</h2>
        <p>
          {hiddenSearch
            ? "Versuche es mit einem anderen Namen, Datum oder Betrag."
            : "Umbuchungen, private Zahlungen und andere Umsätze ohne Rechnung blendest du beim Prüfen aus. Sie bleiben gespeichert."}
        </p>
      </div>{/if}
  {:else if view === "history"}
    <ListHeader
      count="{payments.length} {payments.length === 1 ? 'Zuordnung' : 'Zuordnungen'}"
      placeholder="Zuordnungen suchen"
      bind:query={historySearch}
    />
    {#if payments.length}<div class="invoice-rows">
        {#each payments as p (p.id)}
          {@const invoice = invoiceOf(p)}
          <EntryRow
            title={invoice?.number || "Rechnung"}
            subtitle="{invoice?.partner || 'Ohne Partner'}{p.revokedAt ? ' · Aufgehoben' : ''}"
            meta={dateLabel(p.date)}
            amount={money(p.amount, p.currency)}
            tooltip="Zahlungen dieser Rechnung ansehen"
            disabled={busy}
            onclick={() => {
              historyInvoice = p.invoiceId;
              historyDirection = p.direction;
            }}
          >
            {#snippet status()}
              {#if invoice}<PaymentStatus balance={invoice} />{:else}<span></span>{/if}
            {/snippet}
          </EntryRow>
        {/each}
      </div>{:else if result}<div class="empty-state">
        <h2>{historySearch ? "Keine Treffer" : "Noch keine Zuordnungen"}</h2>
        <p>
          {historySearch
            ? "Versuche es mit einer anderen Rechnungsnummer oder einem anderen Namen."
            : "Hier erscheinen alle Zahlungen, die einer Rechnung zugeordnet sind."}
        </p>
      </div>{/if}
    {#if historyInvoice}<InvoicePayments
        invoiceID={historyInvoice}
        direction={historyDirection}
        onchanged={refresh}
        {onbusychange}
      />{/if}
  {/if}
  {#if expenseDraft}<ExpenseDialog
      initial={expenseDraft}
      categories={expenseCategories}
      {smallBusiness}
      onclose={() => (expenseDraft = null)}
      onsaved={async (saved) => {
        expenseDraft = null;
        notice = `Als Ausgabe erfasst: ${saved.supplier}. Der Umsatz ist damit geklärt.`;
        await onchanged();
        await refresh();
      }}
    />{/if}
  {#if active}<FormDialog
      title="Umsatz prüfen"
      description={`${dateLabel(active.booking.data.booking_date)} · ${bookingMoney(active.booking)} · ${active.booking.data.counterparty_name || "Bankumsatz"}`}
      {busy}
      {error}
      onclose={closeReview}
      onsubmit={() => void assign()}
    >
      <h3>Wozu gehört dieser Umsatz?</h3>
      <p>{active.booking.data.description || "Ohne Verwendungszweck"}</p>
      {#each active.suggestions ?? [] as suggestion}<button
          type="button"
          class="secondary suggestion"
          disabled={busy}
          onclick={() => (chosen = `${suggestion.invoice.direction}:${suggestion.invoice.id}`)}
          >{suggestion.invoice.number} · {suggestion.invoice.partner}<small
            >Passend: {suggestion.reasons?.join(", ")}</small
          ></button
        >{/each}
      {#each (active.expenses ?? []).filter((x) => x.reasons?.length) as x}<button
          type="button"
          class="secondary suggestion"
          disabled={busy}
          onclick={() => (chosen = `expense:${x.id}`)}
          >{x.documentNumber || x.number} · {x.supplier}<small
            >Passend: {x.reasons?.join(", ")}</small
          ></button
        >{/each}

      <label
        >Rechnung oder Ausgabe<select bind:value={chosen} required disabled={busy}
          ><option value="">Auswählen …</option>{#each availableInvoices as b}<option
              value={`${b.direction}:${b.id}`}
              >{b.direction === "outgoing" ? "Kundenrechnung" : "Ausgabe"} · {b.number} · {b.partner}
              · {money(b.remaining, b.currency)} offen</option
            >{/each}{#each active.expenses ?? [] as x}<option value={`expense:${x.id}`}
              >Ausgabe · {x.documentNumber || x.number} · {x.supplier}
              · {money(x.gross, "EUR")} offen</option
            >{/each}</select
        ></label
      >
      <div hidden={chosen.startsWith("expense:")}>
        <SettingSection
          title="Zuordnungsbetrag"
          action="Teilbetrag"
          summary={`${allocation || "0"} ${active.booking.data.currency}, du kannst auch nur einen Teil zuordnen`}
          ><label
            >Zuordnungsbetrag in {active.booking.data.currency}<input
              bind:value={allocation}
              inputmode="decimal"
              required
              disabled={busy}
            /></label
          >
        </SettingSection>
      </div>
      {#if active.remaining.startsWith("-") && active.booking.data.currency === "EUR"}<button
          class="secondary"
          type="button"
          disabled={busy}
          onclick={() => {
            const review = active!;
            activeBooking = "";
            void captureExpense(review);
          }}>Als Ausgabe erfassen</button
        >{/if}
      <button class="text" type="button" disabled={busy} onclick={() => chooseHide(active)}
        >Keine Rechnungszuordnung nötig? Umsatz ausblenden</button
      >
      <p class="hint">
        Gibt es keinen passenden Beleg, kannst du ihn unter „Ausgaben & Belege“ ergänzen.
        Umbuchungen und private Zahlungen brauchen keine Rechnungszuordnung.
      </p>
      {#snippet footer()}
        <button class="secondary" type="button" disabled={busy} onclick={closeReview}
          >Abbrechen</button
        >
        <button class="primary" type="submit" disabled={busy || !chosen}
          >Zuordnung bestätigen</button
        >
      {/snippet}
    </FormDialog>{/if}
  {#if hiding}<FormDialog
      title="Umsatz ausblenden"
      description={`${dateLabel(hiding.booking.data.booking_date)} · ${bookingMoney(hiding.booking)} · ${hiding.booking.data.counterparty_name || "Bankumsatz"}`}
      {busy}
      {error}
      onclose={closeReview}
      onsubmit={() => void hide()}
    >
      <p>{hiding.booking.data.description || "Ohne Verwendungszweck"}</p>
      <p class="hint">
        Der Umsatz bleibt gespeichert und wird beim Rechnungsabgleich übersprungen. Unter
        „Ausgeblendete Umsätze“ kannst du ihn jederzeit wieder anzeigen.
      </p>
      <label
        >Grund<select bind:value={treatmentKind} disabled={busy}>
          {#each Object.entries(treatmentLabels) as [value, label]}<option {value}>{label}</option
            >{/each}
        </select></label
      >
      <label
        >Notiz (optional)<input
          bind:value={treatmentNote}
          maxlength="2000"
          disabled={busy}
        /></label
      >
      {#snippet footer()}
        <button class="secondary" type="button" disabled={busy} onclick={closeReview}
          >Abbrechen</button
        >
        <button class="primary" type="submit" disabled={busy}>Umsatz ausblenden</button>
      {/snippet}
    </FormDialog>{/if}
  {#if restoring}<FormDialog
      title="Ausgeblendeter Umsatz"
      description={`${dateLabel(restoring.booking.data.booking_date)} · ${bookingMoney(restoring.booking)} · ${restoring.booking.data.counterparty_name || "Bankumsatz"}`}
      {busy}
      {error}
      onclose={() => {
        restoringId = "";
        closeReview();
      }}
      onsubmit={() => void restore()}
    >
      <p>{restoring.booking.data.description || "Ohne Verwendungszweck"}</p>
      <p class="hint">
        Ausgeblendet als „{treatmentLabels[
          restoring.treatment.kind
        ]}“{#if restoring.treatment.note}:
          {restoring.treatment.note}{/if}. Wieder angezeigt, kannst du den Umsatz einer Rechnung
        zuordnen.
      </p>
      {#snippet footer()}
        <button
          class="secondary"
          type="button"
          disabled={busy}
          onclick={() => {
            restoringId = "";
            closeReview();
          }}>Abbrechen</button
        >
        <button class="primary" type="submit" disabled={busy}>Wieder anzeigen</button>
      {/snippet}
    </FormDialog>{/if}
  {#if csvOpen}<FormDialog
      title="Bankumsätze aus CSV importieren"
      description="Für Konten ohne FinTS-Abruf: Umsätze deiner Bank als CSV-Datei einlesen."
      {busy}
      {error}
      onclose={() => (csvOpen = false)}
      onsubmit={() => void importCSV()}
    >
      <p class="hint">
        Lade die Umsätze im Online-Banking als CSV herunter und wähle die Datei aus. Die App erkennt
        Spalten, Datum und Beträge selbst. Prüfe die Vorschau, bevor du den Import bestätigst.
        Verwende für dasselbe Konto immer dieselbe Kennung und Importquelle; ein Abgleich zwischen
        FinTS und CSV erfolgt nicht automatisch.
      </p>
      <PlusHint
        id="bank-csv"
        topic="fints"
        text="Mit Plus rufst du Umsätze per FinTS ab – ohne CSV-Export."
      />
      <label>Kontokennung (IBAN)<input bind:value={account} required disabled={busy} /></label>
      <label
        id="bank-csv-dialog-drop"
        class="csv-field"
        data-file-drop-target={busy ? undefined : ""}
        >CSV-Datei<FilePicker
          accept=".csv,.txt,text/csv"
          disabled={busy}
          filename={csvFilename}
          empty="Keine Datei ausgewählt – du kannst sie auch hier ablegen."
          onpick={async (f) => {
            if (f.size > 5_000_000) {
              error = "Die CSV-Datei ist zu groß (maximal 5 MB).";
              return;
            }
            takeCSV(f.name, await f.arrayBuffer());
          }}
        /></label
      >
      <label>CSV-Inhalt<textarea bind:value={csv} rows="5" disabled={busy}></textarea></label>
      <button
        class="secondary"
        type="button"
        disabled={busy || !csv || !account}
        onclick={() => void checkCSV()}>Vorschau & Duplikatprüfung</button
      >
      {#if preview}<p>
          {preview.filter((b) => !b.duplicate).length} neue Umsätze · {preview.filter(
            (b) => b.duplicate,
          ).length} Duplikate
        </p>
        <div class="table-wrap">
          <table>
            <thead
              ><tr><th>Datum</th><th>Betrag</th><th>Partner / Zweck</th><th>Prüfung</th></tr></thead
            ><tbody
              >{#each preview as b}<tr
                  ><td class="nowrap">{dateLabel(b.data.booking_date)}</td><td class="nowrap"
                    >{bookingMoney(b)}</td
                  ><td class="purpose">{b.data.counterparty_name} · {b.data.description}</td><td
                    >{b.duplicate ? "Bereits vorhanden" : "Neu"}</td
                  ></tr
                >{/each}</tbody
            >
          </table>
        </div>{/if}
      {#snippet footer()}
        <button class="secondary" type="button" disabled={busy} onclick={() => (csvOpen = false)}
          >Abbrechen</button
        >
        <button
          class="primary"
          type="submit"
          disabled={busy || !preview || csv !== previewCSV || account !== previewAccount}
          >Import bestätigen</button
        >
      {/snippet}
    </FormDialog>{/if}
</section>

<style>
  .hint {
    font-size: 12px;
    color: var(--theme-muted);
    line-height: 1.5;
  }
  /* The file field is a native drop target; the runtime marks it while a file hovers. */
  .csv-field {
    border-radius: 8px;
  }
  .csv-field:global(.file-drop-target-active) {
    outline: 2px solid var(--theme-focus);
    outline-offset: 4px;
    background: var(--theme-hover);
  }
  h3 {
    font-size: 14px;
  }
  small {
    display: block;
    color: var(--theme-muted);
    font-size: 12px;
    margin-top: 4px;
    overflow-wrap: anywhere;
  }
  label {
    display: grid;
    gap: 6px;
    margin: 14px 0;
  }
  input,
  select,
  textarea {
    width: 100%;
    min-width: 0;
  }
  textarea {
    resize: vertical;
  }
  /* Suggested matches carry long partner names and reasons: a left-aligned
     card per match instead of centred two-column text. */
  .suggestion {
    display: grid;
    justify-items: start;
    width: 100%;
    margin: 8px 0;
    text-align: left;
    white-space: normal;
    overflow-wrap: anywhere;
  }
  .table-wrap {
    overflow: auto;
    overscroll-behavior: contain;
  }
  table {
    width: 100%;
    border-collapse: collapse;
  }
  th,
  td {
    text-align: left;
    padding: 8px;
    border-bottom: 1px solid var(--theme-border);
    vertical-align: top;
  }
  td.nowrap {
    white-space: nowrap;
  }
  td.purpose {
    overflow-wrap: anywhere;
  }
  .error {
    color: var(--theme-error-text);
  }
  .success {
    color: var(--theme-success-text);
    font-size: 13px;
  }
  .error,
  .success {
    overflow-wrap: anywhere;
  }
</style>
