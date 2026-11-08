<script lang="ts" module>
  export type BankTab = "overview" | "pending" | "accounts" | "hidden" | "history";
</script>

<script lang="ts">
  import SettingSection from "../../components/SettingSection.svelte";
  import { onMount, onDestroy, tick, untrack } from "svelte";
  import FinanceOverview from "../finance/FinanceOverview.svelte";
  import Icon from "../../components/Icon.svelte";
  import LandmarkIcon from "@lucide/svelte/icons/landmark";
  import RefreshCwIcon from "@lucide/svelte/icons/refresh-cw";
  import ListChecksIcon from "@lucide/svelte/icons/list-checks";
  import FileSpreadsheetIcon from "@lucide/svelte/icons/file-spreadsheet";
  import ActionTiles from "../../components/ActionTiles.svelte";
  import Tabs from "../../components/Tabs.svelte";
  import PageHeader from "../../components/PageHeader.svelte";
  import ListHeader from "../../components/ListHeader.svelte";
  import EntryRow from "../../components/EntryRow.svelte";
  import type { ActionTile } from "../../lib/actionTiles";
  import PaymentsPage from "../payments/PaymentsPage.svelte";
  import type { PaymentOverview } from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/store/models";
  import * as InvoiceAPI from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/app/invoiceservice";
  import PaymentStatus from "../payments/PaymentStatus.svelte";
  import AccountPicker from "./AccountPicker.svelte";
  import AccountRemoveDialog from "./AccountRemoveDialog.svelte";
  import Monogram from "../../components/Monogram.svelte";
  import PlusHint from "../../components/PlusHint.svelte";
  import { hintPlanFromProductSource, setHintPlan } from "../../lib/hintPlan";
  import { Browser } from "@wailsio/runtime";
  import * as AccountService from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/app/accountservice";
  import { plan, setPlanStatus } from "../../lib/plan";
  import { plusBookingUrl } from "../../lib/planStatus";
  import { watchActivation, type ActivationWatch } from "../../lib/watchActivation";
  import {
    banking,
    newestFirst,
    type BankConnection,
    type BankOverview,
    type BankOperation,
    type BankBooking,
  } from "../../lib/banking-api";
  import { accountChoices } from "../../lib/bankAccounts";
  import { bankFromCompanyProfile, connectionDefaultsFromProfile } from "../../lib/bankProfile";
  import {
    bankPresets,
    bankPresetByID,
    bankPresetForCode,
    bankPresetForIBAN,
    presetBankCode,
    presetMatchesEndpoint,
    bankRoutingMismatch,
  } from "../../lib/bankPresets";
  import { serviceErrorMessage } from "../../lib/errors";
  // Imported as a namespace: "remember" is the PIN checkbox here.
  import * as cache from "../../lib/cache";
  import { incomingTotals } from "../../lib/bankSummary";
  import type { Company } from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/model/models";
  import type {
    DashboardSummary,
    PaymentBooking,
  } from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/app/models";

  let {
    onchanged = async () => {},
    onbusychange = (_busy: boolean) => {},
    company,
    selected = $bindable(""),
    configuring = $bindable(false),
    initialTab = "overview",
  }: {
    company: Company;
    /** The view to open with; links to unassigned bookings open "pending". */
    initialTab?: BankTab;
    selected?: string;
    configuring?: boolean;
    onchanged?: () => Promise<void>;
    onbusychange?: (busy: boolean) => void;
  } = $props();
  const initialPreset = () => bankPresetForIBAN(company.iban);
  const blankConnection = (): BankConnection => ({
    id: "",
    ...connectionDefaultsFromProfile(company),
    userId: "",
    customerId: "",
    tanMedium: "",
    rememberPin: false,
  });
  const today = new Date();
  const dateInput = (value: Date) =>
    `${value.getFullYear()}-${String(value.getMonth() + 1).padStart(2, "0")}-${String(value.getDate()).padStart(2, "0")}`;
  // The last loaded state shows right away while the page reloads, so opening it
  // neither switches the tiles nor shows empty panels first.
  let overview = $state<BankOverview>(
    cache.cached<BankOverview>("banking:overview") ?? {
      connections: [],
      accounts: [],
      productSource: "",
      csvLabels: {},
    },
  );
  let overviewKnown = $state(Boolean(cache.cached<BankOverview>("banking:overview")));
  let connection = $state<BankConnection>(blankConnection());
  let presetID = $state(initialPreset()?.id ?? "");
  let pickerOpen = $state(false);
  let operation = $state<BankOperation | null>(null);
  let bookingsByAccount = $state<Record<string, BankBooking[]>>(
    cache.cached<Record<string, BankBooking[]>>("banking:bookings") ?? {},
  );
  let bookingErrors = $state<Record<string, boolean>>({});
  let productID = $state("");
  let pin = $state("");
  let syncPIN = $state("");
  let tan = $state("");
  let remember = $state(false);
  let advanced = $state(!initialPreset()?.endpoint);
  let busy = $state(false);
  let loadingBookings = $state(false);
  let error = $state("");
  let notice = $state("");
  let reviewRevision = $state(0);
  let paymentOverview = $state<PaymentOverview | null>(
    cache.cached<PaymentOverview>("payments:overview") ?? null,
  );
  async function refreshPayments() {
    paymentOverview = cache.remember("payments:overview", await InvoiceAPI.Payments());
    await onchanged();
  }
  // Accounts known only from CSV imports: their IBAN, count and latest booking.
  let csvBookings = $state<PaymentBooking[]>([]);
  const csvAccounts = $derived.by(() => {
    const known = new Set(overview.accounts.map((item) => item.data.iban));
    const groups = new Map<string, { id: string; count: number; latest: string }>();
    for (const booking of csvBookings) {
      if (booking.source !== "csv" || known.has(booking.accountId)) continue;
      const group = groups.get(booking.accountId) ?? {
        id: booking.accountId,
        count: 0,
        latest: "",
      };
      group.count++;
      if (booking.data.booking_date > group.latest) group.latest = booking.data.booking_date;
      groups.set(booking.accountId, group);
    }
    return [...groups.values()];
  });
  $effect(() => {
    if (tab !== "accounts" || configuring) return;
    void InvoiceAPI.PaymentBookings()
      .then((rows) => (csvBookings = rows ?? []))
      .catch(() => {});
  });
  function assignedInvoice(id: string) {
    const p = paymentOverview?.payments?.find((p) => p.bookingId === id && !p.revokedAt);
    return p
      ? paymentOverview?.invoices?.find((b) => b.id === p.invoiceId && b.direction === p.direction)
      : undefined;
  }
  let from = $state(dateInput(new Date(today.getFullYear(), today.getMonth() - 1, 1)));
  let to = $state(dateInput(today));
  let query = $state("");
  let disposed = false;
  let timer: ReturnType<typeof setTimeout> | undefined;
  let loadVersion = 0;
  let pollVersion = 0;
  const profileBank = $derived(bankFromCompanyProfile(company));
  const preset = $derived(bankPresetByID(presetID));
  const unavailable = $derived(Boolean(preset && !preset.endpoint));
  const profileMatches = $derived(
    Boolean(
      profileBank.iban &&
      (profileBank.bankCode === connection.bankCode ||
        (preset && bankPresetForIBAN(profileBank.iban)?.id === preset.id)),
    ),
  );
  const endpointPreset = $derived(
    bankPresets.find((bank) => bank.endpoint && bank.endpoint === connection.endpoint),
  );
  const presetMismatch = $derived(
    bankRoutingMismatch(connection.bankCode, connection.endpoint, presetID),
  );
  const active = $derived(
    operation !== null && ["running", "tan", "approval"].includes(operation.status),
  );
  const locked = $derived(busy || active);
  // Without Plus or an own product ID the bank refuses every FinTS request, so
  // connecting and retrieving are offered only once the retrieval is unlocked.
  // Until the first status is known the tiles stay as they were.
  const fintsLocked = $derived(overviewKnown && !overview.productSource);
  const account = $derived(overview.accounts.find((item) => item.id === selected));
  const currentConnection = $derived(
    overview.connections.find((item) => item.id === account?.connectionId),
  );
  // Accounts as the user names them, told apart where the names are equal;
  // named CSV accounts join by their IBAN, unnamed ones by the IBAN alone.
  const choices = $derived([
    ...accountChoices(overview.accounts, overview.connections),
    ...Object.entries(overview.csvLabels ?? {})
      .filter(([id]) => !overview.accounts.some((item) => item.data.iban === id))
      .map(([id, name]) => ({ id, name, detail: id })),
  ]);
  const csvName = (id: string) => overview.csvLabels?.[id] || id;
  // The CSV account being renamed and its draft name.
  let renamingCSV = $state(""),
    csvLabelDraft = $state("");
  async function saveCSVLabel() {
    const id = renamingCSV;
    await act(async () => {
      await banking.setAccountLabel(id, csvLabelDraft);
      await refresh();
      renamingCSV = "";
      notice = csvLabelDraft.trim()
        ? "Der Kontoname ist gespeichert."
        : "Der Kontoname ist zurückgesetzt.";
    });
  }
  const nameOf = (id: string) => choices.find((item) => item.id === id)?.name ?? "Bankkonto";
  // The account the user is about to remove; CSV accounts have no bank connection.
  let removing = $state<{ id: string; name: string; bookings: number; csv: boolean } | null>(null);
  let removeError = $state("");
  async function removeAccount() {
    if (!removing || busy) return;
    const target = removing;
    busy = true;
    removeError = "";
    try {
      const result = await banking.removeAccount(target.id);
      if (disposed) return;
      removing = null;
      selected = "";
      notice = `Das Konto „${target.name}“ und ${result.bookings === 1 ? "sein Umsatz" : `seine ${result.bookings} Umsätze`} sind entfernt.`;
      await refresh();
      await loadAllBookings();
      csvBookings = (await InvoiceAPI.PaymentBookings().catch(() => csvBookings)) ?? [];
      reviewRevision++;
      await refreshPayments();
    } catch (e) {
      if (!disposed)
        removeError = serviceErrorMessage(
          e,
          "Das Konto konnte nicht entfernt werden. Versuche es noch einmal.",
        );
    } finally {
      if (!disposed) busy = false;
    }
  }
  let labelDraft = $state("");
  $effect(() => {
    labelDraft = account?.label ?? "";
  });
  const rows = $derived(bookingsByAccount[selected] ?? []);
  const loadingRows = $derived(loadingBookings && !Object.hasOwn(bookingsByAccount, selected));
  const filteredRows = $derived(
    rows.filter((row) =>
      [row.data.counterparty_name, row.data.description, row.data.booking_date]
        .join(" ")
        .toLocaleLowerCase()
        .includes(query.toLocaleLowerCase()),
    ),
  );
  const challengeImage = $derived.by(() => {
    const c = operation?.challenge;
    return c?.image &&
      ["image/png", "image/jpeg", "image/gif", "image/webp"].includes(c.imageMimeType) &&
      c.image.length < 4_000_000 &&
      /^[A-Za-z0-9+/]*={0,2}$/.test(c.image)
      ? `data:${c.imageMimeType};base64,${c.image}`
      : "";
  });

  function money(value: number, currency: string): string {
    if (!Number.isSafeInteger(value)) return "Betrag außerhalb des sicheren Zahlenbereichs";
    try {
      const formatter = new Intl.NumberFormat("de-DE", { style: "currency", currency });
      return formatter.format(
        value / 10 ** (formatter.resolvedOptions().maximumFractionDigits ?? 2),
      );
    } catch {
      return `${value} ${currency} (kleinste Einheit)`;
    }
  }
  // Bookings waiting for an assignment, as reported by the reconciliation below.
  // Until it reports, the overview's count by the same rule stands in.
  let pendingCount = $state<number | null>(
    cache.cached<DashboardSummary>("dashboard:summary")?.unassignedBookings ?? null,
  );
  const lastSync = $derived(
    overview.accounts
      .map((item) => item.lastSync ?? "")
      .sort()
      .at(-1),
  );
  // The views below the tiles, as on the other pages. The overview answers
  // "Reicht das Geld?" from the same accounts the other views work on.
  let tab = $state<BankTab>(untrack(() => initialTab));
  const bankTabs = $derived<{ id: BankTab; label: string }[]>([
    { id: "overview", label: "Überblick" },
    {
      id: "pending",
      label: pendingCount ? `Umsätze zuordnen (${pendingCount})` : "Umsätze zuordnen",
    },
    { id: "accounts", label: "Konten" },
    { id: "hidden", label: "Ausgeblendet" },
    { id: "history", label: "Zugeordnet" },
  ]);
  let csvOpen = $state(false);
  // Fetching happens per account: open the only account directly, otherwise the list.
  async function openSync() {
    configuring = false;
    tab = "accounts";
    if (overview.accounts.length === 1) selected = overview.accounts[0].id;
    if (!selected) return;
    await tick();
    document
      .querySelector<HTMLElement>(".sync-form input[type=password], .sync-form button.primary")
      ?.focus();
  }
  const assignTile = $derived<ActionTile>({
    id: "assign",
    status: true,
    icon: ListChecksIcon,
    alert: !!pendingCount,
    label: pendingCount
      ? `${pendingCount} ${pendingCount === 1 ? "Umsatz" : "Umsätze"} zuordnen`
      : "Umsätze zuordnen",
    hint:
      pendingCount === null
        ? "Wird abgeglichen …"
        : pendingCount
          ? "Rechnung zuordnen oder einordnen"
          : "Alles zugeordnet",
    title: pendingCount
      ? "Einer Rechnung zuordnen oder als sonstige Geldbewegung einordnen"
      : undefined,
    onclick: () => (tab = "pending"),
  });
  // main.go accepts native CSV drops only on this element (and the open dialog).
  const csvTile = $derived<ActionTile>({
    id: "csv",
    icon: FileSpreadsheetIcon,
    label: "CSV importieren",
    hint: "Kontoauszug einlesen",
    title: "Kontoauszug als CSV-Datei ablegen oder auswählen (bis 5 MB)",
    onclick: () => (csvOpen = true),
    attrs: { id: "bank-csv-drop", "data-file-drop-target": locked ? undefined : "" },
  });
  const plusLockedTitle =
    "Der Bankabruf per FinTS ist mit Plus freigeschaltet. Kontoauszüge liest du ohne Plus über „CSV importieren“ ein.";
  // Community sees the same tiles as Plus; the FinTS ones open the booking page.
  const fints = (tile: ActionTile): ActionTile =>
    fintsLocked
      ? {
          ...tile,
          hint: "Mit Plus freischalten",
          title:
            "Plus auf papierleicht.de buchen: Damit holst du Umsätze direkt von deiner Bank. PIN und TAN bleiben auf deinem Rechner.",
          onclick: () => void bookPlus(),
        }
      : tile;
  const tiles = $derived<ActionTile[]>(
    overview.accounts.length
      ? [
          fints({
            id: "sync",
            primary: true,
            icon: RefreshCwIcon,
            label: "Umsätze abrufen",
            hint: lastSync ? `Zuletzt am ${day(lastSync)}` : "Neue Buchungen holen",
            title: "Neue Buchungen von deiner Bank holen",
            onclick: () => void openSync(),
          }),
          fints({
            id: "connect",
            icon: LandmarkIcon,
            label: "Konto verbinden",
            hint: "Weiteres Konto per FinTS",
            title: "Umsätze eines weiteren Kontos per FinTS abrufen",
            onclick: () => (pickerOpen = true),
          }),
          csvTile,
          assignTile,
        ]
      : [
          fints({
            id: "connect",
            primary: true,
            icon: LandmarkIcon,
            label: "Konto verbinden",
            hint: "Umsätze per FinTS abrufen",
            title: "Umsätze per FinTS abrufen. PIN und TAN bleiben auf deinem Rechner.",
            onclick: () => (pickerOpen = true),
          }),
          csvTile,
          assignTile,
        ],
  );
  // Plus is booked on the website. As on the account page, the app then looks
  // for the new subscription by itself; the effect below unlocks the retrieval.
  let activationWatch: ActivationWatch | null = null;
  async function bookPlus() {
    const status = await AccountService.Status().catch(() => null);
    await openExternal(plusBookingUrl(status?.manageUrl ?? "", "plus", $plan.supportActive));
    if (!status || status.state === "signed-out") return;
    activationWatch?.stop();
    activationWatch = watchActivation(async () => {
      const next = await AccountService.Refresh();
      return setPlanStatus(next).plusActive;
    });
  }
  async function openExternal(url: string) {
    try {
      await Browser.OpenURL(url);
    } catch {
      window.open(url, "_blank", "noopener,noreferrer");
    }
  }
  // A subscription that starts or ends while the page is open changes the
  // product ID source, so the tiles follow without reopening the page.
  let lastPlusActive: boolean | null = null;
  $effect(() => {
    const active = $plan.loaded ? $plan.plusActive : null;
    if (active === null || active === lastPlusActive) return;
    const changed = lastPlusActive !== null;
    lastPlusActive = active;
    if (changed) void refresh().catch(showError);
  });
  function day(value: string): string {
    if (/^\d{4}-\d{2}-\d{2}/.test(value)) return value.slice(0, 10).split("-").reverse().join(".");
    return "—";
  }
  function accountTurnover(id: string, currency: string): string {
    return incomingTotals(bookingsByAccount[id] ?? [], currency || "EUR")
      .map(([code, amount]) => money(amount, code))
      .join(" · ");
  }
  function showError(e: unknown) {
    error = serviceErrorMessage(
      e,
      "Die Bankverbindung konnte nicht aktualisiert werden. Versuche es noch einmal.",
    );
  }
  async function refresh() {
    const data = await banking.overview();
    if (disposed) return;
    overview = cache.remember("banking:overview", {
      ...data,
      connections: data.connections ?? [],
      accounts: data.accounts ?? [],
      csvLabels: data.csvLabels ?? {},
    });
    overviewKnown = true;
    setHintPlan(hintPlanFromProductSource(data.productSource));
    if (selected && !overview.accounts.some((item) => item.id === selected)) selected = "";
  }
  async function loadAllBookings() {
    const version = ++loadVersion;
    const accounts = overview.accounts;
    // Loaded bookings stay visible while they reload.
    loadingBookings = accounts.some((item) => !Object.hasOwn(bookingsByAccount, item.id));
    const results = await Promise.allSettled(accounts.map((item) => banking.bookings(item.id)));
    if (disposed || version !== loadVersion) return;
    const next: Record<string, BankBooking[]> = {};
    const failures: Record<string, boolean> = {};
    results.forEach((result, index) => {
      const id = accounts[index].id;
      if (result.status === "fulfilled") next[id] = newestFirst(result.value);
      else failures[id] = true;
    });
    bookingsByAccount = cache.remember("banking:bookings", next);
    bookingErrors = failures;
    loadingBookings = false;
  }
  async function act(action: () => Promise<void>) {
    if (busy) return;
    busy = true;
    error = "";
    notice = "";
    try {
      await action();
    } catch (e) {
      if (!disposed) showError(e);
    } finally {
      if (!disposed) busy = false;
    }
  }
  async function acceptOperation(result: BankOperation) {
    if (disposed) {
      if (["running", "tan", "approval"].includes(result.status))
        void banking.cancel(result.id).catch(() => {});
      return;
    }
    const previousChallenge = operation?.challenge?.id;
    operation = result;
    if (previousChallenge !== result.challenge?.id) tan = "";
    clearTimeout(timer);
    if (["running", "tan", "approval"].includes(result.status)) {
      timer = setTimeout(() => void poll(result.id), 1000);
    } else {
      if (result.status === "error") {
        error = result.message;
        if ((result.methods?.length ?? 0) || (result.media?.length ?? 0)) {
          configuring = true;
          advanced = true;
        }
      } else if (result.status === "done") {
        notice =
          result.message ||
          `${result.added} neue Umsätze übernommen. Deine Rechnungen sind auf dem neuesten Stand.`;
        reviewRevision++;
        await refreshPayments();
        configuring = false;
      }
      await refresh();
      if (result.status === "error") {
        const saved = overview.connections.find((item) => item.id === result.connectionId);
        if (saved) {
          connection = { ...saved };
          const candidate = bankPresetForCode(saved.bankCode);
          presetID = presetMatchesEndpoint(candidate, saved.endpoint) ? candidate!.id : "";
          if (!presetID) advanced = true;
          remember = saved.rememberPin;
        }
      }
      await loadAllBookings();
    }
  }
  async function poll(id: string) {
    clearTimeout(timer);
    const version = ++pollVersion;
    try {
      const result = await banking.status(id);
      if (!disposed && operation?.id === id && version === pollVersion)
        await acceptOperation(result);
    } catch (e) {
      if (disposed || operation?.id !== id || version !== pollVersion) return;
      showError(e);
      // A temporary bridge failure must not leave a running operation orphaned.
      timer = setTimeout(() => void poll(id), 3000);
    }
  }
  function editConnection(value?: BankConnection) {
    connection = value ? { ...value } : blankConnection();
    const candidate = bankPresetForCode(connection.bankCode);
    presetID = presetMatchesEndpoint(candidate, connection.endpoint) ? candidate!.id : "";
    remember = value?.rememberPin ?? false;
    pin = "";
    advanced = !presetID || Boolean(value?.tanMedium || value?.customerId);
    configuring = true;
    error = "";
  }
  function selectPreset(id: string) {
    const next = bankPresetByID(id);
    // A bank change must never carry credentials to another endpoint.
    connection = {
      id: "",
      label: next?.name ?? "",
      endpoint: next?.endpoint ?? "",
      bankCode: next ? presetBankCode(next, profileBank.bankCode) : profileBank.bankCode,
      userId: "",
      customerId: "",
      securityFunction: next?.defaultTAN ?? "",
      tanMedium: "",
      rememberPin: false,
    };
    presetID = id;
    pin = "";
    remember = false;
    advanced = !next || !connection.bankCode;
    operation = null;
    error = "";
  }
  function chooseAccount(id: string) {
    selectPreset(id);
    selected = "";
    configuring = true;
    pickerOpen = false;
  }
  function editRouting(field: "endpoint" | "bankCode", value: string) {
    connection[field] = value;
    connection.userId = "";
    connection.customerId = "";
    connection.securityFunction = "";
    connection.tanMedium = "";
    pin = "";
    remember = false;
    operation = null;
    const candidate = bankPresetForCode(connection.bankCode);
    presetID = presetMatchesEndpoint(candidate, connection.endpoint) ? candidate!.id : "";
    advanced = true;
  }
  async function connect() {
    if (locked || unavailable || fintsLocked) return;
    if (presetMismatch) {
      advanced = true;
      error =
        "Die Bankleitzahl passt nicht zur gewählten Bank. Prüfe Bankauswahl und BLZ oder richte die Verbindung manuell ein.";
      return;
    }
    await act(async () => {
      const password = pin;
      pin = "";
      const details = { ...connection };
      if (endpointPreset?.loginBankCode) details.bankCode = endpointPreset.loginBankCode;
      if (
        preset?.customerIDFromLogin &&
        !details.customerId &&
        presetMatchesEndpoint(preset, details.endpoint)
      )
        details.customerId = details.userId;
      const result = await banking.connect(details, password, remember);
      connection.id = result.connectionId;
      await acceptOperation(result);
    });
  }
  async function sync() {
    if (locked || fintsLocked || !selected) return;
    await act(async () => {
      const password = syncPIN;
      syncPIN = "";
      await acceptOperation(await banking.sync(selected, password, from, to));
    });
  }
  async function submitTAN() {
    if (!operation?.challenge || busy) return;
    const id = operation.id,
      challengeID = operation.challenge.id,
      answer = tan;
    tan = "";
    await act(async () => {
      await banking.submitTAN(id, challengeID, answer);
      await poll(id);
    });
  }
  async function cancel() {
    const id = operation?.id;
    if (!id) return;
    await act(async () => {
      await banking.cancel(id);
      await poll(id);
    });
  }
  onMount(() => {
    // The first load runs outside act(): going busy would disable the tiles and
    // controls and make the page flash every time it opens.
    void (async () => {
      try {
        await refresh();
        await loadAllBookings();
      } catch (e) {
        if (!disposed) showError(e);
      }
    })();
  });
  onDestroy(() => {
    disposed = true;
    activationWatch?.stop();
    clearTimeout(timer);
    if (operation && ["running", "tan", "approval"].includes(operation.status))
      void banking.cancel(operation.id).catch(() => {});
    pin = "";
    syncPIN = "";
    tan = "";
  });
  $effect(() => {
    if (!configuring) pin = "";
  });
</script>

{#if pickerOpen && !fintsLocked}
  <AccountPicker onselect={chooseAccount} onclose={() => (pickerOpen = false)} />
{/if}

{#if removing}
  <AccountRemoveDialog
    name={removing.name}
    bookings={removing.bookings}
    csv={removing.csv}
    {busy}
    error={removeError}
    onclose={() => (removing = null)}
    onconfirm={() => void removeAccount()}
  />
{/if}

<div class="banking-page">
  <!-- Connecting a bank and a single account open as sub-pages with their own head. -->
  {#if configuring}
    <PageHeader
      back={selected ? "Zurück zum Konto" : "Konto & Finanzen"}
      onback={() => {
        configuring = false;
        pin = "";
      }}
      title={connection.id ? "Bankverbindung bearbeiten" : "Konto verbinden"}
      disabled={locked}
    />
  {:else if tab === "accounts" && account}
    <PageHeader
      back="Alle Konten"
      onback={() => {
        selected = "";
        syncPIN = "";
      }}
      title={nameOf(account.id)}
      description={account.data.iban || account.data.account_number}
      disabled={locked}
    >
      {#snippet actions()}
        <button
          class="secondary"
          disabled={locked || fintsLocked}
          title={fintsLocked ? plusLockedTitle : undefined}
          onclick={() => editConnection(currentConnection)}>Verbindung bearbeiten</button
        >
        <button
          class="secondary danger-text"
          disabled={locked}
          onclick={() => {
            removeError = "";
            removing = {
              id: account!.id,
              name: nameOf(account!.id),
              bookings: rows.length,
              csv: false,
            };
          }}>Konto entfernen</button
        >
      {/snippet}
    </PageHeader>
  {:else}
    <header class="banking-header">
      <div>
        <h1>Konto & Finanzen</h1>
      </div>
    </header>
    <ActionTiles {tiles} disabled={locked} />
  {/if}

  {#if error}<div class="bank-alert error" role="alert">{error}</div>{/if}
  {#if notice}<div class="bank-alert success" role="status">{notice}</div>{/if}
  {#if configuring}
    <section class="bank-panel">
      <form
        onsubmit={(event) => {
          event.preventDefault();
          void connect();
        }}
      >
        <fieldset disabled={locked}>
          <div class="bank-picker">
            {#if preset}<Monogram name={preset.name} size={42} />
            {:else}<div class="bank-placeholder">
                <LandmarkIcon size={22} aria-hidden="true" />
              </div>{/if}
            <div class="bank-choice">
              <span>Anbieter</span>
              <button
                type="button"
                class="bank-choice-button"
                disabled={Boolean(connection.id)}
                onclick={() => (pickerOpen = true)}
              >
                {preset?.name ?? "Andere Bank · manuell einrichten"}
                {#if !connection.id}<span aria-hidden="true">Ändern …</span>{/if}
              </button>
            </div>
          </div>
          {#if profileMatches}
            <p class="profile-account">
              Aus deinem Profil · {profileBank.iban}{company.bic ? ` · ${company.bic}` : ""}
            </p>
          {/if}
          {#if preset}
            <p class="bank-hint">{preset.hint}</p>
          {:else}
            <p class="bank-hint">
              Deine Bank ist noch nicht hinterlegt. Trage die FinTS-Adresse und BLZ aus der
              Anleitung deiner Bank ein.
            </p>
          {/if}
          {#if unavailable}
            <p class="unavailable-note">
              Hier ist noch keine direkte Verbindung möglich. Wir fragen keine Zugangsdaten ab.
              Bietet deine Bank eine CSV-Datei an, liest du sie über „CSV importieren“ ein. <a
                href={preset?.source}
                target="_blank"
                rel="noreferrer">Hinweise des Anbieters ↗</a
              >
            </p>
            <button class="text-button" type="button" onclick={() => selectPreset("")}
              >Eigene FinTS-Verbindungsdaten eingeben</button
            >
          {:else}
            <div class="form-grid">
              <label
                >{preset?.loginLabel ?? "Benutzerkennung"}<input
                  bind:value={connection.userId}
                  autocomplete="off"
                  required
                /></label
              >
              <label
                >Online-Banking-PIN<input
                  type="password"
                  bind:value={pin}
                  autocomplete="off"
                  required
                  placeholder="PIN eingeben"
                /></label
              >
            </div>
            {#if preset?.loginHelp}
              <details class="login-help link-toggle">
                <summary>{preset.loginHelp.question}</summary>
                <p>{preset.loginHelp.answer}</p>
              </details>
            {/if}
            <SettingSection
              title="Verbindungsdaten und TAN-Verfahren"
              summary={[
                connection.label,
                connection.bankCode && `BLZ ${connection.bankCode}`,
                connection.securityFunction
                  ? `TAN-Verfahren ${connection.securityFunction}`
                  : "TAN-Verfahren automatisch",
              ]
                .filter(Boolean)
                .join(" · ")}
              bind:open={advanced}
            >
              <div class="form-grid advanced-fields">
                <label
                  >Bezeichnung<input
                    bind:value={connection.label}
                    placeholder="Mein Geschäftskonto"
                    required
                  /></label
                >
                <label
                  >Bankleitzahl<input
                    value={connection.bankCode}
                    oninput={(event) => editRouting("bankCode", event.currentTarget.value)}
                    inputmode="numeric"
                    pattern={"[0-9]{8}"}
                    maxlength="8"
                    required
                    oninvalid={() => (advanced = true)}
                  /></label
                >
                {#if presetMismatch}<small class="wide mismatch" role="status"
                    >Die BLZ passt nicht zur gewählten Bank. Prüfe die Bankauswahl.</small
                  >{/if}
                <label class="wide"
                  >FinTS-Adresse der Bank<input
                    type="url"
                    value={connection.endpoint}
                    oninput={(event) => editRouting("endpoint", event.currentTarget.value)}
                    placeholder="https://…"
                    required
                    autocomplete="off"
                    oninvalid={() => (advanced = true)}
                  />
                  <small
                    >Die Adresse der FinTS-Schnittstelle, nicht die Online-Banking-Website.{#if preset}
                      <a href={preset.source} target="_blank" rel="noreferrer">Bankhinweise ↗</a
                      >{/if}</small
                  >
                </label>
                <label
                  >Kunden-ID (optional)<input
                    bind:value={connection.customerId}
                    autocomplete="off"
                    placeholder={preset?.customerIDFromLogin ? "Wie Benutzerkennung" : "Bei Bedarf"}
                  /></label
                >
                <label
                  >TAN-Verfahren<input
                    bind:value={connection.securityFunction}
                    list="bank-tan-methods"
                    maxlength="3"
                    placeholder="Automatisch"
                  />
                  <datalist id="bank-tan-methods"
                    >{#each operation?.methods ?? [] as method}<option
                        value={method.security_function}>{method.name}</option
                      >{/each}</datalist
                  >
                </label>
                <label class="wide"
                  >TAN-Gerät (optional)<input
                    bind:value={connection.tanMedium}
                    list="bank-tan-media"
                    placeholder="Name des freigegebenen Geräts"
                  />
                  <datalist id="bank-tan-media"
                    >{#each operation?.media ?? [] as medium}<option value={medium.name}
                      ></option>{/each}</datalist
                  >
                </label>
              </div>
            </SettingSection>
            <div class="form-footer">
              <label class="checkbox"
                ><input type="checkbox" bind:checked={remember} /> PIN im Schlüsselbund dieses Rechners
                speichern</label
              ><button class="primary" disabled={!overview.productSource}
                >{connection.id ? "Erneut verbinden" : "Verbinden"}</button
              >
            </div>
            <PlusHint
              id="bank-connect"
              topic="fints"
              dismissible={false}
              text="Mit Plus rufst du deine Umsätze ohne eigene Produktkennung ab. PIN und TAN bleiben auf deinem Rechner."
            />
          {/if}
        </fieldset>
      </form>
    </section>
  {/if}

  {#if active && operation}
    <section class="bank-panel challenge-panel" aria-label="Bankfreigabe">
      <div class="panel-heading">
        <h2>
          {operation.status === "tan"
            ? "TAN eingeben"
            : operation.status === "approval"
              ? "In deiner Banking-App freigeben"
              : "Bank wird aktualisiert …"}
        </h2>
        <button class="secondary" disabled={busy} onclick={() => void cancel()}>Abbrechen</button>
      </div>
      <p role="status">
        {operation.challenge?.text ||
          operation.message ||
          "Verbindung zu deiner Bank wird hergestellt."}
      </p>
      {#if challengeImage}<img
          class="tan-image"
          src={challengeImage}
          alt="TAN-Code deiner Bank für dein Freigabegerät"
        />{/if}
      {#if operation.status === "tan" && operation.challenge}
        <form
          class="inline-form"
          onsubmit={(event) => {
            event.preventDefault();
            void submitTAN();
          }}
        >
          <label
            >TAN<input
              bind:value={tan}
              autocomplete="one-time-code"
              spellcheck="false"
              required
            /></label
          ><button class="primary" disabled={busy || !tan}>Bestätigen</button>
        </form>
      {/if}
    </section>
  {/if}

  {#if !configuring && !(tab === "accounts" && account)}<Tabs
      tabs={bankTabs}
      active={tab}
      label="Bereiche von Konto & Finanzen"
      disabled={locked}
      onchange={(next) => (tab = next)}
    />{/if}
  {#if !configuring && tab === "overview"}<FinanceOverview
      onassign={() => (tab = "pending")}
    />{/if}
  {#if !configuring && tab === "accounts"}
    {#if !overview.productSource}
      <section class="bank-panel">
        <h2>Bankabruf freischalten</h2>
        <p>
          Mit Plus rufst du Umsätze per FinTS ab. Bereits abgerufene Umsätze und der CSV-Import
          bleiben auch ohne Plus verfügbar.
        </p>
        {#if !configuring}<PlusHint
            id="bank-setup"
            topic="fints"
            dismissible={false}
            text="Mit Plus ist der Bankabruf ohne eigene Kennung freigeschaltet. PIN und TAN bleiben auf deinem Rechner."
          />{/if}
        <SettingSection
          title="Eigene FinTS-Produktkennung"
          action="Eintragen"
          summary="Für Fortgeschrittene, die selbst bei der Deutschen Kreditwirtschaft registriert sind"
        >
          <p class="own-product-id-help">
            Nur nötig, wenn du selbst bei der Deutschen Kreditwirtschaft registriert bist.
          </p>
          <form
            class="inline-form"
            onsubmit={(event) => {
              event.preventDefault();
              void act(async () => {
                await banking.setProductID(productID.trim());
                productID = "";
                await refresh();
              });
            }}
          >
            <label
              >Produktkennung<input
                bind:value={productID}
                maxlength="25"
                required
                autocomplete="off"
              /></label
            >
            <button class="primary" disabled={locked || !productID.trim()}>Speichern</button>
          </form>
        </SettingSection>
      </section>
    {/if}
  {/if}
  {#if !configuring && tab === "accounts" && overview.accounts.length && !selected}
    <ListHeader
      count="{overview.accounts.length} {overview.accounts.length === 1 ? 'Konto' : 'Konten'}"
    />
    <div class="invoice-rows">
      {#each overview.accounts as item (item.id)}
        <EntryRow
          title={nameOf(item.id)}
          subtitle={item.data.iban || item.data.account_number || "Kontonummer unbekannt"}
          meta={item.lastSync ? day(item.lastSync) : "Noch nie"}
          amount={loadingBookings || bookingErrors[item.id]
            ? ""
            : accountTurnover(item.id, item.data.currency ?? "EUR")}
          tooltip="Konto öffnen"
          disabled={locked}
          onclick={() => {
            selected = item.id;
            configuring = false;
            query = "";
          }}
        >
          {#snippet status()}
            <PaymentStatus
              summary={{
                title: "Geldeingänge",
                description: loadingBookings
                  ? "Wird geladen …"
                  : bookingErrors[item.id]
                    ? "Nicht verfügbar"
                    : "Summe der gespeicherten Umsätze",
              }}
            />
          {/snippet}
        </EntryRow>
      {/each}
    </div>
  {:else if !configuring && tab === "accounts" && account}
    <section class="bank-panel">
      <form
        class="sync-form"
        onsubmit={(event) => {
          event.preventDefault();
          void sync();
        }}
      >
        <label>Von<input type="date" bind:value={from} max={to} required disabled={locked} /></label
        >
        <label>Bis<input type="date" bind:value={to} min={from} required disabled={locked} /></label
        >
        {#if !currentConnection?.rememberPin}<label
            >Online-Banking-PIN<input
              type="password"
              bind:value={syncPIN}
              required
              autocomplete="off"
              disabled={locked}
            /></label
          >{/if}
        <button
          class="primary"
          disabled={locked || fintsLocked}
          title={fintsLocked ? plusLockedTitle : undefined}
          ><Icon name="refresh" size={16} /> Aktualisieren</button
        >
      </form>
      <form
        class="inline-form label-form"
        onsubmit={(event) => {
          event.preventDefault();
          void act(async () => {
            await banking.setAccountLabel(account!.id, labelDraft);
            await refresh();
            notice = labelDraft.trim()
              ? "Der Kontoname ist gespeichert."
              : "Der Kontoname ist zurückgesetzt.";
          });
        }}
      >
        <label
          >Name oder Beschreibung des Kontos<input
            bind:value={labelDraft}
            maxlength="60"
            autocomplete="off"
            placeholder={`Standard: ${nameOf(account.id)}`}
            disabled={locked}
          /></label
        >
        <button class="secondary" disabled={locked || labelDraft.trim() === (account.label ?? "")}
          >Namen speichern</button
        >
      </form>
      <div class="account-meta">
        <small
          >{account?.lastSync
            ? `Zuletzt abgerufen: ${day(account.lastSync)}`
            : "Noch keine Umsätze abgerufen"}{#if account?.balance}{" · "}Kontostand laut Bank: {money(
              account.balance.booked.amount,
              account.balance.booked.currency,
            )} (Stand {day(account.balance.booked.date)}){/if}</small
        >
        {#if currentConnection?.rememberPin}<button
            class="text-button"
            disabled={locked}
            onclick={() =>
              void act(async () => {
                await banking.forgetPIN(currentConnection!.id);
                await refresh();
                notice = "Die gespeicherte PIN ist entfernt.";
              })}>Gespeicherte PIN entfernen</button
          >{/if}
      </div>
    </section>
    <details class="bookings-panel detail-row">
      <summary
        ><span class="detail-title">Alle Kontoumsätze</span><span class="detail-state"
          >{rows.length} {rows.length === 1 ? "Umsatz" : "Umsätze"}</span
        ></summary
      >
      <div class="panel-heading">
        <div>
          <h2>Gebuchte Umsätze <span class="count">{rows.length}</span></h2>
          <p>Ein- und Auszahlungen mit Zuordnung zu deinen Rechnungen</p>
        </div>
        <label class="search-label"
          ><span class="sr-label">Umsätze durchsuchen</span><input
            type="search"
            bind:value={query}
            placeholder="Umsätze durchsuchen"
          /></label
        >
      </div>
      {#if loadingRows}<p class="empty-state" role="status">Umsätze werden geladen …</p>
      {:else if bookingErrors[selected]}<p class="empty-state" role="alert">
          Die Umsätze dieses Kontos konnten nicht geladen werden.
        </p>
      {:else if !filteredRows.length}<p class="empty-state">
          {query
            ? "Keine passenden Umsätze."
            : "Noch keine Umsätze vorhanden. Wähle einen Zeitraum und rufe deine Umsätze ab."}
        </p>
      {:else}<div class="table-scroll">
          <table>
            <thead
              ><tr
                ><th>Datum</th><th>Zahlung</th><th class="amount">Betrag</th><th>Zuordnung</th></tr
              ></thead
            ><tbody
              >{#each filteredRows as row (row.id)}<tr
                  ><td class="booking-date">{day(row.data.booking_date)}</td><td
                    ><strong>{row.data.counterparty_name || "Bankbuchung"}</strong><span
                      class="booking-purpose"
                      >{row.data.description ||
                        row.data.bank_reference ||
                        "Ohne Verwendungszweck"}</span
                    ></td
                  ><td class="amount" class:positive={row.data.amount > 0}
                    >{money(row.data.amount, row.data.currency)}</td
                  ><td
                    >{#if assignedInvoice(row.id)}<small>{assignedInvoice(row.id)!.number}</small
                      ><PaymentStatus balance={assignedInvoice(row.id)!} />{:else}<small
                        >Noch offen</small
                      >{/if}</td
                  ></tr
                >{/each}</tbody
            >
          </table>
        </div>{/if}
    </details>
  {:else if !configuring && tab === "accounts" && !overview.accounts.length && !csvAccounts.length}<div
      class="empty-state"
    >
      <h2>Noch kein Konto verbunden</h2>
      <p>
        Verbinde dein Bankkonto über „Konto verbinden“ oder lies einen Kontoauszug mit „CSV
        importieren“ ein.
      </p>
    </div>{/if}
  {#if !configuring && tab === "accounts" && csvAccounts.length && !selected}
    <ListHeader
      count="{csvAccounts.length} {csvAccounts.length === 1
        ? 'Konto aus CSV-Importen'
        : 'Konten aus CSV-Importen'}"
    />
    <ul class="csv-accounts">
      {#each csvAccounts as item (item.id)}
        <li>
          {#if renamingCSV === item.id}
            <form
              class="csv-rename"
              onsubmit={(event) => {
                event.preventDefault();
                void saveCSVLabel();
              }}
            >
              <label
                >Name oder Beschreibung für {item.id}<input
                  bind:value={csvLabelDraft}
                  maxlength="60"
                  autocomplete="off"
                  placeholder="z. B. Tagesgeld"
                  disabled={locked}
                /></label
              >
              <button class="secondary" disabled={locked}>Namen speichern</button>
              <button type="button" class="text-button" onclick={() => (renamingCSV = "")}
                >Abbrechen</button
              >
            </form>
          {:else}
            <span class="csv-name"
              ><strong>{csvName(item.id)}</strong>{#if overview.csvLabels?.[item.id]}<small
                  >{item.id}</small
                >{/if}</span
            >
            <span
              >{item.count}
              {item.count === 1 ? "Umsatz" : "Umsätze"}, zuletzt vom {day(item.latest)}</span
            >
            <span class="csv-actions"
              ><button
                class="text-button"
                disabled={locked}
                onclick={() => {
                  renamingCSV = item.id;
                  csvLabelDraft = overview.csvLabels?.[item.id] ?? "";
                }}>Umbenennen</button
              ><button
                class="text-button danger-text"
                disabled={locked}
                onclick={() => {
                  removeError = "";
                  removing = {
                    id: item.id,
                    name: csvName(item.id),
                    bookings: item.count,
                    csv: true,
                  };
                }}>Entfernen</button
              ></span
            >
          {/if}
        </li>
      {/each}
    </ul>
  {/if}
  {#if !configuring}<PaymentsPage
      revision={reviewRevision}
      view={tab === "accounts" || tab === "overview" ? null : tab}
      bind:csvOpen
      accountID={selected}
      accountLabel={account ? nameOf(account.id) : "Bankkonto"}
      accounts={choices}
      onpendingchange={(count) => (pendingCount = count)}
      onallaccounts={() => (selected = "")}
      onchanged={refreshPayments}
      {onbusychange}
    />{/if}
</div>

<style>
  .bookings-panel {
    margin-bottom: 16px;
  }
  .own-product-id-help {
    margin: 8px 0 12px;
    font-size: 12px;
    color: var(--theme-muted);
  }
  .banking-page {
    max-width: var(--ui-page-width);
    margin: 0 auto;
    padding: 0 0 24px;
    color: var(--theme-text);
  }
  .banking-header,
  .panel-heading,
  .form-footer,
  .account-meta {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    justify-content: space-between;
    gap: 16px;
  }
  /* When a row wraps, its trailing control stays on the right. */
  .panel-heading > :last-child:not(:first-child),
  .form-footer > :last-child:not(:first-child),
  .account-meta > :last-child:not(:first-child) {
    margin-left: auto;
  }
  .panel-heading > :first-child,
  .account-meta > small {
    min-width: 0;
  }
  .banking-header {
    min-height: 36px;
    margin-bottom: 24px;
  }
  h1 {
    font-size: var(--ui-title-size);
    font-weight: 650;
    line-height: 1.25;
    letter-spacing: -0.4px;
    margin: 0;
  }
  h2 {
    font-size: 16px;
    margin: 0;
  }
  p {
    color: var(--theme-muted);
    font-size: 13px;
    line-height: 1.5;
    margin: 8px 0 0;
  }
  .bank-panel {
    padding: var(--ui-card-padding);
    border: 1px solid var(--theme-border);
    border-radius: 12px;
    background: var(--theme-surface);
    margin-bottom: 16px;
    min-width: 0;
  }
  .bank-picker {
    display: flex;
    align-items: end;
    gap: 12px;
  }
  .bank-choice {
    flex: 1;
    display: grid;
    gap: 6px;
    min-width: 0;
    font-size: 12px;
    font-weight: 600;
  }
  .bank-choice-button {
    display: flex;
    justify-content: space-between;
    align-items: center;
    gap: 12px;
    width: 100%;
    min-height: 42px;
    padding: 8px 11px;
    border: 1px solid var(--theme-border);
    border-radius: 6px;
    background: var(--theme-surface);
    color: var(--theme-text);
    text-align: left;
    white-space: normal;
    overflow-wrap: anywhere;
    cursor: pointer;
  }
  .bank-choice-button span {
    color: var(--theme-muted);
    font-weight: 400;
    white-space: nowrap;
  }
  .bank-placeholder {
    width: 42px;
    height: 42px;
    flex-shrink: 0;
    object-fit: contain;
    border-radius: 9px;
    background: var(--paper-white);
  }
  .bank-placeholder {
    display: grid;
    place-items: center;
    background: var(--theme-canvas);
    color: var(--theme-muted);
  }
  .profile-account {
    font-size: 11px;
    overflow-wrap: anywhere;
  }
  .bank-hint {
    margin: 12px 0 18px;
  }
  .login-help {
    margin: 10px 0 0;
    font-size: 13px;
  }
  .login-help p {
    margin: 6px 0 0;
    color: var(--theme-muted);
    line-height: 1.5;
  }
  .unavailable-note {
    margin-bottom: 12px;
  }
  .mismatch {
    color: var(--theme-error-text);
  }
  a {
    color: var(--theme-accent);
    text-underline-offset: 3px;
  }
  .panel-heading {
    margin-bottom: 18px;
  }
  label {
    display: flex;
    flex-direction: column;
    gap: 7px;
    font-size: 12px;
    font-weight: 600;
    min-width: 0;
  }
  input {
    width: 100%;
    min-width: 0;
    box-sizing: border-box;
    padding: 10px 11px;
    border: 1px solid var(--theme-input-border);
    border-radius: 8px;
    background: var(--theme-input);
    color: var(--theme-text);
    font: inherit;
    font-size: 13px;
  }
  small {
    color: var(--theme-muted);
    font-weight: 400;
    line-height: 1.5;
  }
  button {
    display: inline-flex;
    gap: 7px;
    align-items: center;
    justify-content: center;
    white-space: nowrap;
  }
  .text-button {
    border: 0;
    background: none;
    color: var(--theme-accent);
    padding: 5px 0;
    font-size: 12px;
    cursor: pointer;
  }
  button:disabled {
    cursor: default;
    opacity: 0.5;
  }
  fieldset {
    padding: 0;
    margin: 0;
    border: 0;
    min-width: 0;
  }
  .form-grid {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 16px;
  }
  .wide {
    grid-column: 1 / -1;
  }
  .inline-form,
  .sync-form {
    display: flex;
    align-items: end;
    gap: 12px;
    margin-top: 18px;
    flex-wrap: wrap;
  }
  .inline-form label {
    flex: 1;
    max-width: 420px;
  }
  .sync-form label {
    flex: 1;
    min-width: 120px;
  }
  .sync-form label:has(input[type="password"]) {
    flex: 1.4;
  }
  details {
    margin-top: 20px;
  }
  summary {
    cursor: pointer;
    color: var(--theme-muted);
    font-size: 12px;
  }
  .advanced-fields {
    margin-top: 16px;
  }
  .form-footer {
    margin-top: 22px;
  }
  .checkbox {
    flex-direction: row;
    align-items: center;
    font-weight: 400;
  }
  .checkbox input {
    width: auto;
  }
  .account-meta {
    margin-top: 15px;
  }
  .bank-alert {
    padding: 13px 16px;
    border: 1px solid var(--theme-border);
    border-radius: 10px;
    margin-bottom: 16px;
    font-size: 13px;
    line-height: 1.5;
    overflow-wrap: anywhere;
  }
  .bank-alert.error {
    color: var(--theme-error-text);
    border-color: var(--theme-error-border);
  }
  .bank-alert.success {
    background: var(--theme-success-bg);
    color: var(--theme-success-text);
  }
  .challenge-panel {
    border-color: var(--theme-accent);
  }
  .challenge-panel p {
    overflow-wrap: anywhere;
  }
  .tan-image {
    display: block;
    max-width: min(100%, 320px);
    max-height: 320px;
    object-fit: contain;
    margin: 18px 0;
  }
  .count {
    font-size: 12px;
    color: var(--theme-muted);
    font-weight: 400;
    margin-left: 6px;
  }
  .search-label {
    max-width: 250px;
  }
  .sr-label {
    position: absolute;
    width: 1px;
    height: 1px;
    overflow: hidden;
    clip-path: inset(50%);
  }
  .empty-state {
    text-align: center;
    padding: 30px 16px;
    line-height: 1.6;
  }
  .table-scroll {
    overflow-x: auto;
    overscroll-behavior-x: contain;
  }
  table {
    width: 100%;
    border-collapse: collapse;
    font-size: 13px;
  }
  th {
    font-size: 11px;
    color: var(--theme-muted);
    font-weight: 500;
    text-align: left;
  }
  td,
  th {
    padding: 13px 10px;
    border-bottom: 1px solid var(--theme-border);
    vertical-align: top;
  }
  td strong {
    font-weight: 500;
    overflow-wrap: anywhere;
  }
  /* Invoice numbers of received invoices can be long file names. */
  td small {
    overflow-wrap: anywhere;
  }
  .booking-purpose {
    display: block;
    margin-top: 5px;
    color: var(--theme-muted);
    font-size: 12px;
    line-height: 1.5;
    overflow-wrap: anywhere;
  }
  .booking-date {
    white-space: nowrap;
    width: 95px;
  }
  .amount {
    text-align: right;
    white-space: nowrap;
    font-variant-numeric: tabular-nums;
  }
  .positive {
    color: var(--theme-accent);
  }
  @media (max-width: 1000px) {
    .banking-page {
      padding: 0 0 24px;
    }
    .bank-panel {
      padding: 18px;
    }
    .search-label {
      max-width: 190px;
    }
  }
  .csv-accounts {
    display: grid;
    gap: 6px;
    margin: 0 0 20px;
    padding: 0;
    list-style: none;
  }
  .csv-accounts li {
    display: flex;
    flex-wrap: wrap;
    gap: 4px 16px;
    justify-content: space-between;
    padding: 12px 16px;
    border: 1px solid var(--theme-border);
    border-radius: 10px;
    background: var(--theme-surface);
    font-size: 13px;
  }
  .csv-accounts strong {
    min-width: 0;
    overflow-wrap: anywhere;
  }
  .csv-name {
    display: flex;
    flex-direction: column;
    min-width: 0;
  }
  .csv-name small {
    color: var(--theme-muted);
    overflow-wrap: anywhere;
  }
  .csv-actions {
    display: inline-flex;
    gap: 12px;
  }
  .csv-rename {
    display: flex;
    flex-wrap: wrap;
    align-items: flex-end;
    gap: 8px 12px;
    width: 100%;
  }
  .csv-rename label {
    flex: 1 1 240px;
    min-width: 0;
  }
</style>
