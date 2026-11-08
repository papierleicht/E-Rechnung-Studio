<script lang="ts">
  import { onMount, tick, untrack } from "svelte";
  import { fly } from "svelte/transition";
  import { Browser, Dialogs, Events } from "@wailsio/runtime";
  import * as API from "../bindings/papierleicht.de/e-rechnung-studio/internal/app/invoiceservice";
  import { createInvoiceActions } from "./lib/invoiceActions";
  import { createInvoicePreview } from "./lib/invoicePreview";
  import { createUpdateMonitor } from "./lib/updateMonitor";
  import { scrollPageToTop } from "./lib/scrollPage";
  import type { SetupTaxProfile } from "./lib/onboarding";
  import PapierleichtLogo from "./components/PapierleichtLogo.svelte";
  import PlanLevel from "./components/PlanLevel.svelte";
  import AuditAlert from "./components/AuditAlert.svelte";
  import BackupAlert from "./components/BackupAlert.svelte";
  import * as UpdateAPI from "../bindings/papierleicht.de/e-rechnung-studio/internal/app/updateservice";
  import * as BackupAPI from "../bindings/papierleicht.de/e-rechnung-studio/internal/app/backupservice";
  import type {
    DeletedInvoiceResult,
    UpdateStatus,
  } from "../bindings/papierleicht.de/e-rechnung-studio/internal/app/models";
  import type {
    Bootstrap,
    Customer,
    ImportInspection,
    InvoiceDraft,
    InvoiceLine,
    InvoiceSummary,
    RecurringInvoice,
    InvoiceView,
    Product,
  } from "../bindings/papierleicht.de/e-rechnung-studio/internal/model/models";
  import InvoiceSettingsDialog from "./features/invoices/InvoiceSettingsDialog.svelte";
  import PositionEditor from "./features/invoices/PositionEditor.svelte";
  import Icon from "./components/Icon.svelte";
  import FormDialog from "./components/FormDialog.svelte";
  import { resolveSettingsNavigation, type SettingsDecision } from "./lib/settingsNavigation";
  import InvoiceList from "./features/invoices/InvoiceList.svelte";
  import DocumentTabs from "./features/invoices/DocumentTabs.svelte";
  import ListHeader from "./components/ListHeader.svelte";
  import EntryRow from "./components/EntryRow.svelte";
  import ActionTiles from "./components/ActionTiles.svelte";
  import PageHeader from "./components/PageHeader.svelte";
  import FileCheckIcon from "@lucide/svelte/icons/file-check";
  import UserPlusIcon from "@lucide/svelte/icons/user-plus";
  import PackagePlusIcon from "@lucide/svelte/icons/package-plus";
  import {
    isOpen,
    newInvoiceTile,
    newQuoteTile,
    openTile,
    openTotals,
    type ActionTile,
  } from "./lib/actionTiles";
  import { prefetch } from "./lib/prefetch";
  import QuotesPage, { type QuoteEditor } from "./features/quotes/QuotesPage.svelte";
  import InvoiceDocument from "./features/invoices/InvoiceDocument.svelte";
  import CompanySetup from "./features/settings/CompanySetup.svelte";
  import CustomerChooser from "./features/customers/CustomerChooser.svelte";
  import InvoiceCompletion from "./features/invoices/InvoiceCompletion.svelte";
  import DeleteInvoiceDialog from "./features/invoices/DeleteInvoiceDialog.svelte";
  import DeletedInvoicesDialog from "./features/invoices/DeletedInvoicesDialog.svelte";
  import RecurringDialog from "./features/recurring/RecurringDialog.svelte";
  import RecurringInvoices from "./features/recurring/RecurringInvoices.svelte";
  import { recurringRunMessage } from "./lib/recurring";
  import SettingsPage from "./features/settings/SettingsPage.svelte";
  import { auditView } from "./lib/audit";
  import { CloseView } from "../bindings/papierleicht.de/e-rechnung-studio/internal/app/auditservice";
  import CustomerDialog from "./features/customers/CustomerDialog.svelte";
  import ProductDialog from "./features/products/ProductDialog.svelte";
  import XMLInspectionDialog from "./features/received/XMLInspectionDialog.svelte";
  import Dashboard from "./features/dashboard/Dashboard.svelte";
  import type {
    PaymentBalance,
    PaymentOverview,
  } from "../bindings/papierleicht.de/e-rechnung-studio/internal/store/models";
  import PaymentStatus from "./features/payments/PaymentStatus.svelte";
  import BankingPage, { type BankTab } from "./features/banking/BankingPage.svelte";
  import TaxesPage from "./features/finance/TaxesPage.svelte";
  import ShopsPage from "./features/shops/ShopsPage.svelte";
  import CommunityPage from "./features/community/CommunityPage.svelte";
  import MessagesSquareIcon from "@lucide/svelte/icons/messages-square";
  import { chatUnread, watchChatUnread } from "./lib/chatUnread";
  import { sectionVisible, setupSections } from "./lib/menuSections";
  import { shops } from "./lib/shops-api";
  import LandmarkIcon from "@lucide/svelte/icons/landmark";
  import WalletIcon from "@lucide/svelte/icons/wallet";
  import StoreIcon from "@lucide/svelte/icons/store";
  import ReceivedInvoices from "./features/received/ReceivedInvoices.svelte";
  import { encodeInvoiceFile } from "./lib/received";
  import { amountLabel, statusLabel, dateLabel } from "./lib/format";
  import {
    companyTaxTreatment,
    documentLabel,
    invoiceTaxDefaults,
    initializePositionTaxes,
  } from "./lib/tax";
  import {
    clone,
    DEFAULT_PAYMENT_DAYS,
    newCustomer,
    newProduct,
    newLine,
    dueAfter,
    isBlankLine,
    invoiceFromTemplate,
    defaultProfile,
    fillElectronicAddress,
  } from "./lib/invoiceDraft";
  import { invoiceFormats } from "./lib/formats";
  import { errorDetail, serviceErrorMessage } from "./lib/errors";
  import { normalizeAppearance, resolveAppearance } from "./lib/appearance";
  import { waitForFileDialog } from "./lib/fileDialog";
  import { restoreLoss } from "./lib/backup";

  type Section =
    | "dashboard"
    | "invoices"
    | "received"
    | "shops"
    | "finance"
    | "taxes"
    | "customers"
    | "products"
    | "community"
    | "settings";
  const navigation: { id: Section; label: string }[] = [
    { id: "dashboard", label: "Übersicht" },
    { id: "invoices", label: "Kundenrechnungen" },
    { id: "shops", label: "Shop-Bestellungen" },
    { id: "received", label: "Ausgaben & Belege" },
    { id: "finance", label: "Konto & Finanzen" },
    { id: "taxes", label: "Steuern" },
    { id: "customers", label: "Kunden" },
    { id: "products", label: "Artikel" },
    { id: "community", label: "Chat & Markt" },
    { id: "settings", label: "Einstellungen" },
  ];
  // Without a connected shop or an explicit choice the shop area stays hidden.
  let shopConnected = $state(false);
  // The current section stays, so hiding it in the settings does not jump away.
  const menu = $derived(
    navigation.filter(
      (item) =>
        item.id === section || sectionVisible(item.id, data?.settings?.sections, { shopConnected }),
    ),
  );
  type Page = {
    section: Section;
    invoiceStep: "overview" | "customer" | "document";
    customerId: string;
  };
  let pageStack: Page[] = $state([]);
  let section: Section = $state("dashboard");
  let bankingSelected = $state("");
  let bankingConfiguring = $state(false);
  const localBackPage = $derived(pageStack.at(-1)?.section === section ? pageStack.at(-1) : null);
  let data: Bootstrap | null = $state(null);
  let draft: InvoiceDraft | null = $state(null);
  let unfinishedInvoice = $state(false);
  const isXRechnung = $derived.by(() => draft?.profile.startsWith("xrechnung-") ?? false);
  let view: InvoiceView | null = $state(null);
  let busy = $state(false),
    message = $state(""),
    error = $state("");
  let receivedBusy = $state(false);
  let paymentsBusy = $state(false);
  let shopsBusy = $state(false);
  let shopToConnect = $state("");
  let paymentOverview = $state<PaymentOverview | null>(null);
  const navigationLocked = $derived(busy || receivedBusy || paymentsBusy || shopsBusy);
  let cancelFileDialog = $state<(() => void) | null>(null);
  async function pickFile<T>(open: () => Promise<T>): Promise<T | undefined> {
    const controller = new AbortController();
    const cancel = () => controller.abort();
    cancelFileDialog = cancel;
    try {
      return await waitForFileDialog(open, controller.signal);
    } finally {
      if (cancelFileDialog === cancel) cancelFileDialog = null;
    }
  }

  let editingCustomer: Customer | null = $state(null),
    editingProduct: Product | null = $state(null);
  let dialogDirty = $state(false);
  let repeating = $state<RecurringInvoice | null>(null);
  let inspection: ImportInspection | null = $state(null);
  let invoiceDocument: { editField: (field: string) => void } | undefined = $state();
  let pageSettingsOpen = $state(false),
    settingsFocusField = $state("");
  let invoiceQuery = $state(""),
    customerQuery = $state(""),
    productQuery = $state("");
  let invoiceTab = $state<"invoices" | "quotes" | "recurring">("invoices"),
    quoteDirty = $state(false),
    quoteStartNew = $state(false),
    invoiceFilterOpen = $state(false),
    customerFilterOpen = $state(false);
  // The open quote's step: its document view uses the same desk as an invoice.
  let quoteEditor = $state<QuoteEditor | null>(null);
  let receivedPage: { showList: () => void } | undefined = $state();
  let quotesPage: { showList: () => void } | undefined = $state();
  let needsSetup = $state(false);
  let invoiceStep = $state<"overview" | "customer" | "document">("overview");
  const openQuote = $derived.by(() =>
    section === "invoices" && invoiceStep === "overview" && invoiceTab === "quotes"
      ? quoteEditor
      : null,
  );
  // Links into Konto & Finanzen open a view of it, e.g. the bookings to assign;
  // links to a tax form open that task under Steuern.
  let financeTab = $state<BankTab>("overview");
  let financePeriod = $state("");
  let settingsTab = $state<"general" | "company" | "invoice" | "mail" | "audit" | "account">(
    "general",
  );
  let sidebarOpen = $state(true),
    selectedCustomerId = $state("");
  let systemDark = $state(false);
  // macOS hides the traffic lights in full screen; the sidebar toggle then moves to the edge.
  let fullscreen = $state(false);
  let updateStatus = $state<UpdateStatus>({
    currentVersion: "1.0.0",
    updateAvailable: false,
  });
  let updaterOpening = $state(false);
  let settingsSaving = $state(false),
    navigationPending = $state(false);
  let confirmation = $state<{
    title: string;
    message: string;
    actions: string[];
    resolve: (answer: string) => void;
  } | null>(null);
  function ask(title: string, message: string, actions: string[]): Promise<string> {
    if (confirmation) return Promise.resolve("Abbrechen");
    return new Promise((resolve) => {
      confirmation = { title, message, actions, resolve };
    });
  }
  function answerConfirmation(answer: string) {
    const pending = confirmation;
    confirmation = null;
    pending?.resolve(answer);
  }
  async function leaveSettings(proceed: () => void) {
    if (busy || navigationPending) return;
    navigationPending = true;
    try {
      if (!settingsChanged()) {
        proceed();
        return;
      }
      const answer = await ask(
        "Ungespeicherte Änderungen",
        "Möchtest du die geänderten Einstellungen speichern, bevor du die Seite verlässt?",
        ["Verwerfen", "Speichern"],
      );
      const decision: SettingsDecision =
        answer === "Speichern" ? "save" : answer === "Verwerfen" ? "discard" : "cancel";
      await resolveSettingsNavigation(decision, {
        save: saveCompany,
        discard: restoreSettings,
        navigate: proceed,
      });
    } finally {
      navigationPending = false;
    }
  }
  const nativeMac = /Mac/.test(navigator.platform);
  const nativeWindows = /Win/.test(navigator.platform);
  const selectedCustomer = $derived.by(() =>
    data?.customers?.find((c) => c.id === selectedCustomerId),
  );
  const customerInvoices = $derived.by(() =>
    (data?.invoices ?? []).filter((i) => i.customerId === selectedCustomerId),
  );
  function toggleSidebar() {
    sidebarOpen = !sidebarOpen;
    try {
      localStorage.setItem("e-rechnung-studio-sidebar-open", String(sidebarOpen));
    } catch {}
  }
  async function openPapierleichtWebsite() {
    try {
      await Browser.OpenURL("https://papierleicht.de");
    } catch {
      error = "Die Website ließ sich nicht öffnen: https://papierleicht.de";
    }
  }
  async function openUpdater() {
    if (updaterOpening) return;
    updaterOpening = true;
    try {
      await UpdateAPI.Open();
    } catch {
      error = "Das Update ließ sich nicht öffnen. Versuche es noch einmal.";
    } finally {
      updaterOpening = false;
    }
  }
  function currentPage(): Page {
    return { section, invoiceStep, customerId: selectedCustomerId };
  }
  function pushPage() {
    const page = currentPage();
    const localStack = pageStack.filter((item) => item.section === section);
    if (
      localStack.at(-1)?.invoiceStep === page.invoiceStep &&
      localStack.at(-1)?.customerId === page.customerId
    )
      return;
    pageStack = [...localStack, page];
  }
  function showPage(page: Page) {
    section = page.section;
    invoiceStep = page.invoiceStep;
    selectedCustomerId = page.customerId;
    message = "";
    tick().then(() => scrollPageToTop());
  }
  async function goBack() {
    if (navigationLocked || navigationPending || confirmation || !localBackPage) return;
    const previous = localBackPage;
    const proceed = () => {
      pageStack = pageStack.slice(0, -1);
      showPage(previous);
    };
    if (section === "settings") await leaveSettings(proceed);
    else proceed();
  }
  /** The way back from every invoice page: the list of invoices, not of quotes. */
  async function showInvoiceList() {
    invoiceTab = "invoices";
    await navigate("invoices");
    void runRecurringInvoices();
  }
  /** Creates the drafts that are due. It runs beside the user's work, so it never blocks it. */
  async function runRecurringInvoices() {
    if (!data || $auditView.open) return;
    try {
      const result = await API.RunRecurringInvoices();
      if (result.created) {
        await refreshApplicationLists();
        message = recurringRunMessage(result.created);
      }
      if (result.failed?.length) error = `Kein neuer Entwurf für ${result.failed.join("; ")}`;
    } catch (e) {
      error = serviceErrorMessage(e, "Die Dauerrechnungen ließen sich nicht prüfen.");
    }
  }
  async function navigate(next: Section, financeView: BankTab = "overview") {
    if (navigationLocked || navigationPending || confirmation) return;
    financeTab = financeView;
    if (next === section && (next !== "invoices" || invoiceStep === "overview")) {
      // An open quote returns to the list just like an open invoice.
      if (next === "invoices") quotesPage?.showList();
      if (next === "customers") selectedCustomerId = "";
      if (next === "received") receivedPage?.showList();
      return;
    }
    if (section === "settings") {
      await leaveSettings(() => navigateTo(next));
      return;
    }
    navigateTo(next);
  }
  /** The account page in the settings, where the next tier is unlocked. */
  async function openAccount(tab: "account" | "audit" = "account") {
    settingsTab = tab;
    await navigate("settings");
  }
  function navigateTo(next: Section) {
    pageStack = [];
    if (section === "finance" && next !== "finance") {
      bankingSelected = "";
      bankingConfiguring = false;
    }
    showPage({
      section: next,
      invoiceStep: next === "invoices" ? "overview" : invoiceStep,
      customerId: next === "customers" ? "" : selectedCustomerId,
    });
  }
  async function confirmAction(title: string, message: string, action: string) {
    const result = await ask(title, message, [action]);
    return result === action;
  }
  function confirmDiscard(message: string) {
    return confirmAction("Ungespeicherte Änderungen", message, "Verwerfen");
  }
  async function startCustomerInvoice(customer: Customer) {
    if (await createInvoice()) {
      chooseInvoiceCustomer(customer);
    }
  }
  let draftBaseline = $state("");
  let companyBaseline = $state("");
  let settingsBaseline = $state("");
  // Sections of the settings page that save themselves but share its button.
  let pendingSaves = $state<Record<string, () => Promise<void>>>({});
  let previewPending = $state(false),
    errorDetails = $state("");
  let activeOperations = 0;
  let unsaved = $derived(hasUnsavedChanges());

  $effect(() => {
    API.SetUnsavedChanges(unsaved || quoteDirty).catch(() => {});
  });

  $effect(() => {
    const appearance = normalizeAppearance(data?.settings.appearance);
    const resolved = resolveAppearance(appearance, systemDark);
    document.documentElement.dataset.theme = resolved;
    document.documentElement.style.colorScheme = resolved;
  });

  // Status messages stay long enough to be read and then clear themselves.
  $effect(() => {
    const visibleMessage = message;
    if (!visibleMessage) return;
    const timer = window.setTimeout(() => {
      if (message === visibleMessage) message = "";
    }, 5000);
    return () => window.clearTimeout(timer);
  });

  async function resolveField(field: string) {
    if (field === "lines") {
      editLines(0);
      return;
    }
    const lineMatch = /^lines\.(\d+)/.exec(field);
    if (lineMatch) {
      editLines(Number(lineMatch[1]));
      return;
    }
    if (field.startsWith("company.")) {
      settingsTab = [
        "company.invoiceTaxPreset",
        "company.defaultTaxTreatment",
        "company.exemptionReason",
      ].includes(field)
        ? "invoice"
        : "company";
      pushPage();
      section = "settings";
    } else {
      section = "invoices";
    }
    if (!field.startsWith("company.")) invoiceStep = "document";
    await tick();
    if (
      [
        "kind",
        "taxTreatment",
        "taxExemptionReason",
        "paymentMeansCode",
        "precedingInvoiceNumber",
        "precedingInvoiceDate",
        "profile",
        "businessProcess",
        "deliveryCity",
        "deliveryPostcode",
        "customer.endpoint",
        "customer.endpointScheme",
      ].includes(field) ||
      field.startsWith("adjustments")
    ) {
      settingsFocusField = field;
      pageSettingsOpen = true;
    }
    await tick();
    const element = Array.from(document.querySelectorAll<HTMLElement>("[data-field]")).find(
      (el) => el.dataset.field === field,
    );
    if (!pageSettingsOpen && !field.startsWith("company.")) {
      invoiceDocument?.editField(field);
      return;
    }
    if (element && !pageSettingsOpen) {
      element.closest("details")?.setAttribute("open", "");
      element.scrollIntoView({ behavior: "smooth", block: "center" });
      element.focus({ preventScroll: true });
    }
  }
  $effect(() => {
    const fields = new Set((view?.validation.issues ?? []).map((i) => i.field));
    const currentSection = section;
    tick().then(() => {
      if (section !== currentSection) return;
      document.querySelectorAll<HTMLElement>("[data-field]").forEach((el) => {
        if (fields.has(el.dataset.field)) el.setAttribute("aria-invalid", "true");
        else el.removeAttribute("aria-invalid");
      });
    });
  });

  let prospectiveCustomer: Customer = $state(newCustomer());
  function showCustomerStep() {
    pushPage();
    invoiceStep = "customer";
    tick().then(() => scrollPageToTop());
  }
  async function completeSetup(choices: { shop: string; taxProfile: SetupTaxProfile }) {
    if (!data) return;
    const saved = await run(() => API.CompleteSetup(clone(data!.company)));
    if (saved) {
      data.company = saved;
      // The tax answers of the setup become the tax profile under "Steuern".
      await run(async () =>
        API.SaveFinanceSettings({ ...(await API.FinanceSettings()), ...choices.taxProfile }),
      );
      // The shop answer of the setup decides whether the shop area shows.
      const sections = setupSections(data.settings.sections, choices.shop);
      const settings = await run(() => API.SaveAppSettings({ ...clone(data!.settings), sections }));
      if (settings) data.settings = settings;
      rememberSettings();
      needsSetup = false;
      if (choices.shop) {
        // The shop chosen in the setup opens its connection right away.
        shopToConnect = choices.shop;
        navigateTo("shops");
        message = "Dein Unternehmen ist eingerichtet. Verbinde jetzt deinen Shop.";
        return;
      }
      createInvoice(true);
      message = "Dein Unternehmen ist eingerichtet. Wähle jetzt deinen ersten Kunden.";
    }
  }
  function chooseInvoiceCustomer(customer: Customer) {
    if (!draft) return;
    unfinishedInvoice = false;
    let leitwegNotice = false;
    draft.customer = clone(customer);
    draft.customerId = customer.id;
    draft.buyerReference = customer.buyerReference;
    draft.dueDate = dueAfter(draft.issueDate, customer.paymentTermDays ?? undefined);
    const defaults = customer.invoiceSettings;
    draft.profile = defaults?.profile || defaultProfile(customer);
    if (!defaults?.profile && draft.profile !== "facturx-en16931") {
      fillElectronicAddress(draft.customer);
      leitwegNotice = true;
    }
    draft.syntax = invoiceFormats.find((f) => f.id === draft!.profile)?.syntax || "cii";
    draft.kind = defaults?.kind || "invoice";
    if (data) {
      Object.assign(draft, invoiceTaxDefaults(data.company, defaults));
      if (draft.taxTreatment === "mixed")
        initializePositionTaxes(draft, data.company.smallBusiness ? "small-business" : "standard");
    }
    draft.businessProcess = defaults?.businessProcess || "";
    draft.paymentMeansCode = defaults?.paymentMeansCode || "";
    draft.disablePaymentQR = defaults?.disablePaymentQR ?? false;
    draft.hidePaymentQRDetails = defaults?.hidePaymentQRDetails ?? false;
    pushPage();
    invoiceStep = "document";
    prospectiveCustomer = newCustomer();
    schedulePreview();
    if (leitwegNotice)
      message =
        "Leitweg-ID erkannt: XRechnung ist vorgewählt. Du kannst das in den Rechnungseinstellungen ändern.";
    tick().then(() =>
      document.getElementById("invoice-editor-start")?.scrollIntoView({ block: "start" }),
    );
  }
  async function createInvoiceCustomer(save: boolean) {
    if (!draft || busy) return;
    const customer = clone(prospectiveCustomer);
    customer.name = customer.name.trim();
    customer.address.country = customer.address.country.trim().toUpperCase();
    if (save) {
      const saved = await run(() => API.SaveCustomer(customer));
      if (!saved) return;
      if (data) (data.customers ??= []).push(saved);
      chooseInvoiceCustomer(saved);
    } else {
      chooseInvoiceCustomer(customer);
    }
  }

  onMount(() => {
    watchChatUnread();
    void shops.overview().then(
      (overview) => (shopConnected = (overview.connections?.length ?? 0) > 0),
      () => undefined,
    );
    document.documentElement.dataset.platform = nativeWindows
      ? "windows"
      : nativeMac
        ? "macos"
        : "other";
    const colorScheme = window.matchMedia("(prefers-color-scheme: dark)");
    const syncSystemAppearance = () => (systemDark = colorScheme.matches);
    syncSystemAppearance();
    colorScheme.addEventListener("change", syncSystemAppearance);
    // The window reports it: on a Mac with a notch a full-screen window is lower than the screen.
    const stopFullscreen = Events.On("window:fullscreen", (event) => {
      fullscreen = (Array.isArray(event.data) ? event.data[0] : event.data) === true;
    });
    try {
      sidebarOpen = localStorage.getItem("e-rechnung-studio-sidebar-open") !== "false";
    } catch {}
    load();
    let disposed = false;
    let monitor: ReturnType<typeof createUpdateMonitor> | undefined;
    const poll = () => {
      void monitor?.tick();
    };
    // Give startup and settings loading time before the first network request.
    const startup = window.setTimeout(poll, 30_000);
    const timer = window.setInterval(poll, 60_000);
    // A window left open over several days still gets its due recurring drafts.
    const recurringTimer = window.setInterval(() => void runRecurringInvoices(), 3_600_000);
    window.addEventListener("online", poll);
    void UpdateAPI.Status()
      .then((status) => {
        if (disposed) return;
        updateStatus = status;
        monitor = createUpdateMonitor({
          currentVersion: status.currentVersion,
          enabled: () => Boolean(settingsBaseline && JSON.parse(settingsBaseline).automaticUpdates),
          online: () => navigator.onLine,
          now: () => Date.now(),
          storage: {
            getItem: (key) => localStorage.getItem(key),
            setItem: (key, value) => localStorage.setItem(key, value),
          },
          check: () => UpdateAPI.Check(),
          publish: (status) => {
            updateStatus = status;
          },
        });
      })
      .catch(() => {});
    return () => {
      disposed = true;
      preview.cancel();
      monitor?.stop();
      window.clearTimeout(startup);
      window.clearInterval(timer);
      window.clearInterval(recurringTimer);
      window.removeEventListener("online", poll);
      colorScheme.removeEventListener("change", syncSystemAppearance);
      stopFullscreen();
    };
  });
  // Foreground service calls share one busy/error state; confirmation dialogs do not.
  async function run<T>(op: () => Promise<T>): Promise<T | undefined> {
    activeOperations++;
    busy = true;
    error = "";
    errorDetails = "";
    try {
      return await op();
    } catch (e) {
      const detail = errorDetail(e);
      error = serviceErrorMessage(e, "Das hat leider nicht geklappt. Versuche es noch einmal.");
      errorDetails = detail === error ? "" : detail;
    } finally {
      activeOperations--;
      busy = activeOperations > 0;
    }
  }
  function rememberDraft() {
    preview.cancel();
    draftBaseline = draft ? JSON.stringify(draft) : "";
  }
  function rememberSettings() {
    companyBaseline = data ? JSON.stringify(data.company) : "";
    settingsBaseline = data ? JSON.stringify(data.settings) : "";
  }
  // Called from effects of the sections, so it must not subscribe them to the list.
  function setPendingSave(id: string, save: (() => Promise<void>) | null) {
    untrack(() => {
      const { [id]: previous, ...others } = pendingSaves;
      if (previous !== (save ?? undefined))
        pendingSaves = save ? { ...others, [id]: save } : others;
    });
  }
  function settingsChanged() {
    return Boolean(
      data &&
      (JSON.stringify(data.company) !== companyBaseline ||
        JSON.stringify(data.settings) !== settingsBaseline ||
        Object.keys(pendingSaves).length),
    );
  }
  function restoreSettings() {
    if (!data) return;
    if (companyBaseline) data.company = JSON.parse(companyBaseline);
    if (settingsBaseline) data.settings = JSON.parse(settingsBaseline);
  }
  function hasUnsavedChanges() {
    return (
      (draft?.status === "draft" && JSON.stringify(draft) !== draftBaseline) ||
      JSON.stringify(prospectiveCustomer) !== JSON.stringify(newCustomer()) ||
      settingsChanged() ||
      dialogDirty
    );
  }
  async function load() {
    const r = await run(() => API.Bootstrap());
    if (r) {
      data = r;
      paymentOverview = (await run(() => API.Payments())) ?? null;
      void refreshDeletedCount();
      draft = r.draft;
      needsSetup = !r.company.setupCompleted && !r.company.name.trim();
      invoiceStep = "overview";
      unfinishedInvoice = false;
      pageStack = [];
      prospectiveCustomer = newCustomer();
      rememberDraft();
      rememberSettings();
      tick().then(() => scrollPageToTop());
      view = null;
      void runRecurringInvoices();
      // Links on the overview open sections without a hover over the sidebar; their
      // tiles show counts and amounts, so they load once up front instead of flickering.
      if (!needsSetup) for (const name of ["finance", "received", "invoices"]) prefetch(name);
    }
  }
  function acceptInvoiceView(result: InvoiceView) {
    preview.cancel();
    draft = result.draft;
    view = result;
    return result.draft;
  }
  const preview = createInvoicePreview({
    draft: () => draft,
    clone,
    request: (snapshot) => API.PreviewInvoice(snapshot),
    pending: (pending) => {
      previewPending = pending;
    },
    publish: (result) => {
      view = result;
      if (draft) {
        draft.number = result.draft.number;
        draft.automaticNumber = result.draft.automaticNumber;
        draft.paymentReference = result.draft.paymentReference;
      }
    },
    error: (cause) => {
      view = null;
      error = "Die Rechnung ließ sich nicht prüfen. Starte die Prüfung noch einmal.";
      errorDetails = String(cause);
    },
  });
  function schedulePreview() {
    preview.schedule();
  }
  async function refreshApplicationLists() {
    if (data) {
      const fresh = await API.ApplicationLists();
      data.customers = fresh.customers;
      data.products = fresh.products;
      data.invoices = fresh.invoices;
      paymentOverview = await API.Payments();
    }
  }
  $effect(() => {
    const currentSection = section;
    if (!data || !["dashboard", "invoices"].includes(currentSection)) return;
    // Shop imports can create drafts while another part of the app is open.
    void refreshApplicationLists().catch((cause) => {
      if (section === currentSection)
        error = serviceErrorMessage(
          cause,
          "Die Rechnungsliste ließ sich nicht aktualisieren. Öffne die Übersicht erneut.",
        );
    });
  });
  let issuing = $state(false),
    exportedPDF = $state("");
  let editingLines: InvoiceLine[] | null = $state(null),
    editingLineIndex = $state(0);
  function editLines(index = 0) {
    if (!draft || draft.status !== "draft" || busy) return;
    const lines = clone(draft.lines ?? []);
    if (index < 0) {
      const blank = lines.findIndex(isBlankLine);
      if (blank >= 0) index = blank;
      else {
        lines.push(newLine());
        index = lines.length - 1;
      }
    }
    if (!lines.length) {
      lines.push(newLine());
      index = 0;
    }
    editingLineIndex = Math.max(0, Math.min(index, lines.length - 1));
    editingLines = lines;
  }
  function saveLines(lines: InvoiceLine[]) {
    if (!draft) return;
    draft.lines = clone(lines);
    editingLines = null;
    schedulePreview();
  }
  async function exportPDF() {
    if (!draft?.id || busy) return;
    const id = draft.id,
      number = draft.number;
    const result = await run(async () => {
      const path = await pickFile(() =>
        Dialogs.SaveFile({
          Title: isXRechnung ? "PDF mit XRechnung speichern" : "E-Rechnung als PDF speichern",
          Filename: (number || "rechnung") + ".pdf",
          CanCreateDirectories: true,
          Filters: [{ DisplayName: "E-Rechnung (PDF)", Pattern: "*.pdf" }],
        }),
      );
      if (!path) return;
      return API.ExportInvoicePDF(id, path);
    });
    if (result) {
      exportedPDF = result;
      message = isXRechnung
        ? "PDF mit eingebetteter XRechnung gespeichert"
        : "E-Rechnung als PDF gespeichert";
    }
  }
  async function savePageSettings(working: InvoiceDraft) {
    const r = await run(() => API.SaveInvoice(clone(working)));
    if (!r) return;
    draft = acceptInvoiceView(r);
    rememberDraft();
    pageSettingsOpen = false;
    await run(refreshApplicationLists);
    message = draft.customerId
      ? "Rechnungseinstellungen für diesen Kunden gespeichert"
      : "Rechnungseinstellungen gespeichert";
  }
  async function chooseExportDirectory() {
    const path = await run(() =>
      pickFile(() =>
        Dialogs.OpenFile({
          Title: "Exportordner wählen",
          CanChooseFiles: false,
          CanChooseDirectories: true,
          CanCreateDirectories: true,
          AllowsMultipleSelection: false,
          Directory: data?.settings.exportDirectory,
        }),
      ),
    );
    if (typeof path === "string" && path && data) data.settings.exportDirectory = path;
  }
  async function retryExport() {
    if (!draft?.id || busy) return;
    const result = await run(() => API.ExportIssuedInvoice(draft!.id));
    if (result && view) {
      view.export = result;
      exportedPDF = result.pdfPath;
      message = "Rechnung im Exportordner gespeichert";
    }
  }
  async function openPDF() {
    if (!draft?.id || busy) return;
    const path = await run(() => API.OpenInvoicePDF(draft!.id));
    if (path) {
      exportedPDF = path;
      if (view) view.export.pdfPath = path;
    }
  }
  const invoiceActions = createInvoiceActions({
    state: () => ({ draft, view, busy, issuing, previewPending }),
    api: API,
    run,
    accept: (result) => {
      acceptInvoiceView(result);
      rememberDraft();
    },
    message: (value) => {
      message = value;
    },
    error: (value) => {
      error = value;
    },
    exported: (path) => {
      exportedPDF = path;
    },
    issuing: (value) => {
      issuing = value;
    },
    refresh: refreshApplicationLists,
    showCompletion: async () => {
      await tick();
      document
        .getElementById("invoice-completion")
        ?.scrollIntoView({ block: "center", behavior: "smooth" });
    },
    exportXML,
  });
  const { saveDraft, issue } = invoiceActions;
  async function validate() {
    if (!draft) return;
    // Never leave an earlier successful result actionable after a failed check.
    view = null;
    await preview.check(
      (snapshot) => run(() => API.ValidateInvoice(snapshot)),
      (result) => {
        view = result;
        message =
          result.validation.state === "valid"
            ? "Technische Prüfung bestanden"
            : "Ergänze oder korrigiere die markierten Angaben";
      },
    );
  }

  /** Back into the open invoice as it is, with every unsaved edit kept. */
  function resumeDraft() {
    if (!draft) return;
    pushPage();
    invoiceStep = draft.customer.name ? "document" : "customer";
    section = "invoices";
    // The number settings may have changed while the draft waited.
    if (draft.status === "draft") schedulePreview();
  }
  /** Whether the open invoice is one of the rows shown; then that row resumes it. */
  function draftListed() {
    return Boolean(draft?.id) && visibleInvoices().some((item) => item.id === draft!.id);
  }
  async function openInvoice(item: InvoiceSummary) {
    if (busy) return;
    if (draft && item.id === draft.id) {
      resumeDraft();
      return;
    }
    if (
      hasUnsavedChanges() &&
      !(await confirmDiscard(
        "Ungespeicherte Änderungen verwerfen und eine andere Rechnung öffnen?",
      ))
    )
      return;
    const r = await run(() => API.OpenInvoice(item.id));
    if (r) {
      message = "";
      draft = acceptInvoiceView(r);
      rememberDraft();
      if (draft.status === "draft" && !draft.dueDate && !draft.paymentTerms.trim()) {
        const customer = data?.customers?.find((c) => c.id === draft!.customerId);
        draft.dueDate = dueAfter(draft.issueDate, customer?.paymentTermDays ?? undefined);
        schedulePreview();
      }
      prospectiveCustomer = newCustomer();
      unfinishedInvoice = false;
      restoreSettings();
      exportedPDF = r.export.pdfPath;
      pushPage();
      invoiceStep = "document";
      section = "invoices";
    }
  }
  async function openShopInvoice(id: string) {
    await refreshApplicationLists();
    const invoice = data?.invoices?.find((item) => item.id === id);
    if (!invoice) throw new Error("Die verknüpfte Rechnung konnte nicht gefunden werden.");
    await openInvoice(invoice);
  }
  async function createInvoice(force = false) {
    if (!data || busy) return false;
    if (
      !force &&
      hasUnsavedChanges() &&
      !(await confirmDiscard(
        "Ungespeicherte Änderungen verwerfen und eine neue Rechnung beginnen?",
      ))
    )
      return false;
    if (!force) restoreSettings();
    message = "";
    draft = clone(data.draft);
    draft.issueDate = new Date().toLocaleDateString("sv-SE");
    draft.deliveryDate = draft.issueDate;
    draft.dueDate = dueAfter(draft.issueDate);
    exportedPDF = "";
    draft.taxTreatment = companyTaxTreatment(data.company);
    draft.taxSource = "company";
    draft.taxExemptionReason = data.company.exemptionReason;
    draft.lines = [newLine()];
    if (draft.taxTreatment === "mixed")
      initializePositionTaxes(draft, data.company.smallBusiness ? "small-business" : "standard");
    view = null;
    unfinishedInvoice = true;
    pushPage();
    invoiceStep = "customer";
    section = "invoices";
    prospectiveCustomer = newCustomer();
    rememberDraft();
    return true;
  }

  async function saveCustomer() {
    if (!editingCustomer || !data) return;
    const r = await run(() => API.SaveCustomer(editingCustomer!));
    if (r) {
      const i = data.customers?.findIndex((x) => x.id === r.id) ?? -1;
      if (i >= 0) data.customers![i] = r;
      else (data.customers ??= []).push(r);
      selectedCustomerId = r.id;
      editingCustomer = null;
      message = "Kunde gespeichert";
    }
  }
  async function saveProduct() {
    if (!editingProduct || !data) return;
    const r = await run(() => API.SaveProduct(editingProduct!));
    if (r) {
      const i = data.products?.findIndex((x) => x.id === r.id) ?? -1;
      if (i >= 0) data.products![i] = r;
      else (data.products ??= []).push(r);
      editingProduct = null;
      message = "Artikel gespeichert";
    }
  }
  async function deleteCustomer() {
    if (
      !editingCustomer?.id ||
      !data ||
      !(await confirmAction(
        "Kunde löschen",
        `Kunde „${editingCustomer.name}“ löschen? Bereits ausgestellte Rechnungen bleiben erhalten.`,
        "Kunde löschen",
      ))
    )
      return;
    const id = editingCustomer.id;
    const ok = await run(async () => {
      await API.DeleteCustomer(id);
      return true;
    });
    if (ok) {
      data.customers = (data.customers ?? []).filter((x) => x.id !== id);
      editingCustomer = null;
      message = "Kunde gelöscht";
    }
  }
  async function deleteProduct() {
    if (
      !editingProduct?.id ||
      !data ||
      !(await confirmAction(
        "Artikel löschen",
        `Artikel „${editingProduct.name}“ löschen? Bestehende Rechnungen bleiben unverändert.`,
        "Artikel löschen",
      ))
    )
      return;
    const id = editingProduct.id;
    const ok = await run(async () => {
      await API.DeleteProduct(id);
      return true;
    });
    if (ok) {
      data.products = (data.products ?? []).filter((x) => x.id !== id);
      editingProduct = null;
      message = "Artikel gelöscht";
    }
  }
  async function saveCompany() {
    if (!data || busy) return false;
    const current = data;
    const settings = clone(current.settings),
      company = clone(current.company);
    settingsSaving = true;
    try {
      return (
        (await run(async () => {
          const companyChanged = JSON.stringify(company) !== companyBaseline;
          const saved = await API.SaveSettings(settings, companyChanged ? company : null);
          current.settings = saved.settings;
          current.company = saved.company;
          rememberSettings();
          for (const save of Object.values(pendingSaves)) await save();
          if (companyChanged) schedulePreview();
          message = "Einstellungen gespeichert";
          return true;
        })) === true
      );
    } finally {
      settingsSaving = false;
    }
  }
  async function deleteDraft() {
    if (
      !draft?.id ||
      draft.status !== "draft" ||
      !(await confirmAction(
        "Entwurf löschen",
        "Diesen Entwurf dauerhaft löschen?",
        "Entwurf löschen",
      ))
    )
      return;
    const ok = await run(async () => {
      await API.DeleteDraft(draft!.id);
      return true;
    });
    if (ok) {
      if (data) data.invoices = (data.invoices ?? []).filter((item) => item.id !== draft!.id);
      draft = null;
      view = null;
      exportedPDF = "";
      prospectiveCustomer = newCustomer();
      rememberDraft();
      navigateTo("invoices");
      message = "Entwurf gelöscht";
      await run(refreshApplicationLists);
    }
  }
  // Issued invoices are deleted only through the checked dialog; the log keeps them.
  let deletingInvoice = $state(false),
    deletedLogOpen = $state(false),
    deletedCount = $state(0);
  async function refreshDeletedCount() {
    try {
      deletedCount = (await API.DeletedInvoices())?.length ?? 0;
    } catch {
      deletedCount = 0;
    }
  }
  async function closeAuditView() {
    await CloseView();
    auditView.set({ open: false, from: "", to: "" });
  }
  // Lists show the audited period in the read-only view, so they reload with it.
  let auditViewOpen = false;
  onMount(() =>
    auditView.subscribe((view) => {
      if (view.open === auditViewOpen) return;
      auditViewOpen = view.open;
      void load();
    }),
  );

  async function markInvoiceSent(sent: boolean) {
    const id = draft?.id;
    if (!id || busy) return;
    const result = await run(async () => {
      await API.MarkInvoiceSent(id, sent);
      return API.OpenInvoice(id);
    });
    if (!result) return;
    acceptInvoiceView(result);
    rememberDraft();
    message = sent ? "Als verschickt markiert" : "Markierung zurückgenommen";
    await run(refreshApplicationLists);
  }
  function openDraftView(result: InvoiceView) {
    acceptInvoiceView(result);
    exportedPDF = "";
    rememberDraft();
    prospectiveCustomer = newCustomer();
    restoreSettings();
    invoiceStep = "document";
    section = "invoices";
    scrollPageToTop();
  }
  async function invoiceDeleted(result: DeletedInvoiceResult, editAgain: boolean) {
    deletingInvoice = false;
    const { id, number, kind } = result.invoice;
    if (data) data.invoices = (data.invoices ?? []).filter((item) => item.id !== id);
    deletedCount++;
    const kept = result.keptFiles?.length
      ? ` Die Datei ${result.keptFiles.join(", ")} wurde nach dem Export verändert und bleibt im Ordner.`
      : "";
    const reopened = editAgain ? await run(() => API.DraftFromDeletedInvoice(id)) : undefined;
    if (reopened) {
      openDraftView(reopened);
      message = `${documentLabel(kind)} ${number} gelöscht. Der neue Entwurf hat denselben Inhalt und bekommt beim Ausstellen eine neue Nummer.${kept}`;
    } else {
      draft = null;
      view = null;
      exportedPDF = "";
      prospectiveCustomer = newCustomer();
      rememberDraft();
      navigateTo("invoices");
      message = `${documentLabel(kind)} ${number} gelöscht. Sie steht im Protokoll gelöschter Rechnungen.${kept}`;
    }
    await run(refreshApplicationLists);
  }
  async function draftFromDeleted(id: string) {
    if (
      hasUnsavedChanges() &&
      !(await confirmDiscard(
        "Ungespeicherte Änderungen verwerfen und einen neuen Entwurf anlegen?",
      ))
    )
      return;
    const result = await run(() => API.DraftFromDeletedInvoice(id));
    if (!result) return;
    deletedLogOpen = false;
    pushPage();
    openDraftView(result);
    message =
      "Neuer Entwurf aus der gelöschten Rechnung. Er bekommt beim Ausstellen eine neue Nummer.";
    await run(refreshApplicationLists);
  }
  async function useInvoiceAsTemplate() {
    if (!draft?.id || draft.status === "draft" || busy) return;
    await createTemplateDraft(draft.id);
  }
  async function chooseInvoiceTemplate(item: InvoiceSummary) {
    if (busy || !["issued", "paid"].includes(item.status)) return;
    if (
      hasUnsavedChanges() &&
      !(await confirmDiscard(
        "Ungespeicherte Änderungen verwerfen und diese Rechnung als Vorlage verwenden?",
      ))
    )
      return;
    await createTemplateDraft(item.id);
  }
  async function createTemplateDraft(id: string) {
    const result = await run(async () => {
      const source = await API.OpenInvoice(id);
      return API.SaveInvoice(invoiceFromTemplate(source.draft));
    });
    if (!result) return;
    acceptInvoiceView(result);
    exportedPDF = "";
    rememberDraft();
    invoiceStep = "document";
    prospectiveCustomer = newCustomer();
    restoreSettings();
    message =
      "Neuer Entwurf aus der Vorlage erstellt. Prüfe Leistungsdatum, Leistungszeitraum und Zahlungsbedingungen.";
    await run(refreshApplicationLists);
    scrollPageToTop();
  }
  async function repeatInvoice() {
    if (!draft?.id || draft.status === "draft" || busy) return;
    const id = draft.id;
    repeating = (await run(() => API.RecurringFromInvoice(id))) ?? null;
  }
  async function saveRepeat() {
    if (!repeating) return;
    const input = repeating;
    if (!(await run(() => API.SaveRecurringInvoice(input)))) return;
    repeating = null;
    message = "Dauerrechnung gespeichert. Du findest sie unter „Dauerrechnungen“.";
    await runRecurringInvoices();
  }
  async function createCorrection() {
    if (!draft?.id || draft.status === "draft" || busy) return;
    const result = await run(() => API.CreateCorrectionDraft(draft!.id));
    if (!result) return;
    draft = result.draft;
    view = result;
    exportedPDF = "";
    rememberDraft();
    await run(refreshApplicationLists);
    message =
      "Korrekturentwurf erstellt. Prüfe Positionen und Erstattungsangaben, bevor du ihn ausstellst.";
    scrollPageToTop();
  }
  // The file is written and read by the app itself: with all originals it can
  // be far larger than what passes through the web view.
  async function exportBackup() {
    const result = await run(async () => {
      const path = await pickFile(() =>
        Dialogs.SaveFile({
          Title: "Sicherung speichern",
          Filename: `e-rechnung-studio-sicherung-${new Date().toISOString().slice(0, 10)}.zip`,
          CanCreateDirectories: true,
          Filters: [{ DisplayName: "Sicherung (ZIP)", Pattern: "*.zip" }],
        }),
      );
      if (!path) return;
      return BackupAPI.ExportArchive(path);
    });
    if (result) message = `Sicherung mit ${result.documents} Dokumenten gespeichert`;
  }
  async function importBackup() {
    const path = await run(() =>
      pickFile(() =>
        Dialogs.OpenFile({
          Title: "Sicherung auswählen",
          CanChooseFiles: true,
          CanChooseDirectories: false,
          AllowsMultipleSelection: false,
          Filters: [{ DisplayName: "Sicherung", Pattern: "*.zip;*.json" }],
        }),
      ),
    );
    if (typeof path !== "string" || !path) return;
    const preview = await run(() => BackupAPI.InspectArchive(path));
    if (!preview) return;
    const loss = restoreLoss(preview.impact);
    if (
      !(await confirmAction(
        "Sicherung wiederherstellen",
        `Deine aktuellen lokalen Daten werden durch diese Sicherung ersetzt. Die App prüft vorher jede Datei und sichert den jetzigen Stand im Datenverzeichnis.${loss ? ` ${loss}` : ""}`,
        "Wiederherstellen",
      ))
    )
      return;
    const ok = await run(async () => {
      await BackupAPI.ImportArchive(path);
      return true;
    });
    if (ok) {
      await load();
      section = "invoices";
      message = "Sicherung wiederhergestellt";
    }
  }
  async function inspectXML(event: Event) {
    const input = event.currentTarget as HTMLInputElement,
      file = input.files?.[0];
    input.value = "";
    if (!file) return;
    const r = await run(async () =>
      API.InspectInvoiceFile(file.name, await encodeInvoiceFile(file)),
    );
    if (r) inspection = r;
  }
  async function exportXML(id = draft?.id, number = draft?.number) {
    if (!id || busy) return;
    const result = await run(async () => {
      const path = await pickFile(() =>
        Dialogs.SaveFile({
          Title: "E-Rechnung als XML speichern",
          Filename: (number || "rechnung") + ".xml",
          CanCreateDirectories: true,
          Filters: [{ DisplayName: "E-Rechnung (XML)", Pattern: "*.xml" }],
        }),
      );
      if (!path) return;
      return API.ExportInvoiceXML(id, path);
    });
    if (result) message = "XML-E-Rechnung gespeichert";
  }
  // Links from the overview reach the invoices without a hover over the sidebar.
  $effect(() => {
    if (section === "invoices") prefetch("invoices");
  });
  const openBalances = $derived(
    (paymentOverview?.invoices ?? []).filter(
      (b) => b.direction === "outgoing" && !b.credit && isOpen(b),
    ),
  );
  // The same totals as the overview's tile; null until the payments are loaded.
  const receivables = $derived(
    paymentOverview
      ? openTotals(
          (paymentOverview.invoices ?? []).filter((b) => b.direction === "outgoing" && !b.credit),
        )
      : null,
  );
  function startQuote() {
    quoteStartNew = true;
    invoiceTab = "quotes";
  }
  const invoiceTileItems = $derived<ActionTile[]>([
    { ...newInvoiceTile(() => void createInvoice()), primary: true },
    newQuoteTile(startQuote),
    {
      id: "check",
      icon: FileCheckIcon,
      label: "E-Rechnung prüfen",
      hint: "XML oder PDF testen",
      title: "XML oder PDF auf Gültigkeit testen",
      onclick: () => document.getElementById("invoice-xml")?.click(),
    },
    {
      ...openTile("in", receivables, () => {
        // From the quotes tab the tile always leads to the filtered invoice list.
        invoiceFilterOpen = invoiceTab === "quotes" || !invoiceFilterOpen;
        invoiceTab = "invoices";
      }),
      pressed: invoiceFilterOpen && invoiceTab === "invoices",
      disabled: !receivables?.count && !invoiceFilterOpen,
    },
  ]);
  // Open totals per customer, by the same rule as the tiles.
  const customerOpen = $derived.by(() => {
    const byCustomer = new Map<string, PaymentBalance[]>();
    for (const balance of openBalances) {
      const id = data?.invoices?.find((i) => i.id === balance.id)?.customerId;
      if (id) byCustomer.set(id, [...(byCustomer.get(id) ?? []), balance]);
    }
    return new Map([...byCustomer].map(([id, balances]) => [id, openTotals(balances)]));
  });
  const visibleCustomers = $derived.by(() => {
    const q = customerQuery.trim().toLocaleLowerCase("de");
    return (data?.customers ?? []).filter(
      (c) =>
        (!customerFilterOpen || customerOpen.has(c.id)) &&
        (!q ||
          `${c.name} ${c.contactName} ${c.address.city} ${c.email}`
            .toLocaleLowerCase("de")
            .includes(q)),
    );
  });
  const visibleProducts = $derived.by(() => {
    const q = productQuery.trim().toLocaleLowerCase("de");
    return (data?.products ?? []).filter(
      (p) => !q || `${p.name} ${p.description}`.toLocaleLowerCase("de").includes(q),
    );
  });
  const customerTileItems = $derived<ActionTile[]>([
    {
      id: "customer",
      primary: true,
      icon: UserPlusIcon,
      label: "Neuer Kunde",
      hint: "Kontaktdaten speichern",
      title: "Adresse, E-Mail und Zahlungsziel einmal speichern und immer wieder verwenden",
      onclick: () => {
        error = "";
        editingCustomer = newCustomer();
      },
    },
    newInvoiceTile(() => void createInvoice()),
    {
      ...openTile("in", receivables, () => (customerFilterOpen = !customerFilterOpen)),
      pressed: customerFilterOpen,
      disabled: !receivables?.count && !customerFilterOpen,
    },
  ]);
  const productTileItems = $derived<ActionTile[]>([
    {
      id: "product",
      primary: true,
      icon: PackagePlusIcon,
      label: "Neuer Artikel",
      hint: "Mit Preis und Steuersatz",
      title: "Bezeichnung, Preis und Umsatzsteuer für Leistungen speichern, die du oft brauchst",
      onclick: () => {
        error = "";
        editingProduct = newProduct();
      },
    },
    newInvoiceTile(() => void createInvoice()),
    newQuoteTile(async () => {
      await navigate("invoices");
      if (section === "invoices") startQuote();
    }),
  ]);
  function visibleInvoices() {
    const q = invoiceQuery.trim().toLocaleLowerCase("de");
    const open = new Set(openBalances.map((b) => b.id));
    return (data?.invoices ?? []).filter(
      (i) =>
        (!invoiceFilterOpen || open.has(i.id)) &&
        (!q ||
          `${i.number} ${i.customerName} ${i.issueDate} ${statusLabel(i.status)}`
            .toLocaleLowerCase("de")
            .includes(q)),
    );
  }
</script>

<svelte:head><title>E-Rechnung-Studio</title></svelte:head>
{#snippet invoiceTiles()}
  <input
    id="invoice-xml"
    class="hidden-file"
    type="file"
    accept="application/xml,text/xml,application/pdf,.xml,.pdf"
    onchange={inspectXML}
  />
  <ActionTiles tiles={invoiceTileItems} disabled={busy} />
{/snippet}
{#if data && needsSetup}
  <div class="window-toolbar setup-window-toolbar no-print" class:native-mac={nativeMac}>
    <span>E-Rechnung-Studio · Einrichtung</span>
  </div>
  <CompanySetup
    company={data.company}
    {busy}
    {error}
    oncomplete={completeSetup}
    onrestore={importBackup}
  />
{:else}
  <div
    class="shell app-shell"
    class:sidebar-collapsed={!sidebarOpen}
    class:traffic-lights={nativeMac && !fullscreen}
    style:--sidebar-width={sidebarOpen ? "208px" : "0px"}
  >
    <!-- Stays in the window corner, so it is in the same place with the sidebar open or closed. -->
    <button
      class="sidebar-toggle no-print"
      class:native-mac={nativeMac && !fullscreen}
      aria-label={sidebarOpen ? "Seitenleiste ausblenden" : "Seitenleiste einblenden"}
      aria-expanded={sidebarOpen}
      aria-controls="app-sidebar"
      title="Seitenleiste ein-/ausblenden"
      onclick={toggleSidebar}><Icon name="sidebar" size={21} /></button
    >
    <aside
      id="app-sidebar"
      class="sidebar no-print"
      inert={!sidebarOpen}
      aria-hidden={!sidebarOpen}
    >
      <div class="sidebar-titlebar"></div>
      <nav aria-label="Hauptnavigation">
        {#each menu as item}<button
            class:active={section === item.id}
            aria-current={section === item.id ? "page" : undefined}
            disabled={navigationLocked || navigationPending}
            onpointerenter={() => prefetch(item.id)}
            onfocus={() => prefetch(item.id)}
            onclick={() => (item.id === "invoices" ? showInvoiceList() : navigate(item.id))}
            >{#if item.id === "taxes"}<LandmarkIcon
                size={18}
                stroke-width="1.65"
                aria-hidden="true"
              />{:else if item.id === "finance"}<WalletIcon
                size={18}
                stroke-width="1.65"
                aria-hidden="true"
              />{:else if item.id === "shops"}<StoreIcon
                size={18}
                stroke-width="1.65"
                aria-hidden="true"
              />{:else if item.id === "community"}<MessagesSquareIcon
                size={18}
                stroke-width="1.65"
                aria-hidden="true"
              />{:else}<Icon name={item.id} />{/if}<span>{item.label}</span
            >{#if item.id === "community" && $chatUnread > 0 && section !== "community"}<span
                class="nav-badge"
                aria-label={`${$chatUnread} neue Nachrichten`}
                >{$chatUnread > 98 ? "99+" : $chatUnread}</span
              >{/if}</button
          >{/each}
      </nav>
      <div class="app-version">
        <AuditAlert onselect={() => openAccount("audit")} />
        <BackupAlert onselect={() => openAccount()} />
        <PlanLevel onselect={() => openAccount()} />
        <div class="version-brand">
          <button
            type="button"
            class="version-logo-badge"
            aria-label="papierleicht-Website im Browser öffnen"
            title="papierleicht.de öffnen"
            onclick={openPapierleichtWebsite}><PapierleichtLogo /></button
          >
          <span class="version-number">Version {updateStatus.currentVersion}</span>
        </div>
        {#if updateStatus.updateAvailable}
          <button
            class="update-available"
            disabled={updaterOpening}
            onclick={openUpdater}
            title={`Update auf Version ${updateStatus.availableVersion} öffnen`}
          >
            <Icon name="download" size={16} />
            <span>Update {updateStatus.availableVersion}</span>
          </button>
        {/if}
      </div>
    </aside>
    <main
      class="workspace"
      class:invoice-workspace={section === "invoices" &&
        (invoiceStep === "document" || openQuote?.step === "document")}
    >
      {#if $auditView.open}<div class="audit-view-banner no-print" role="status">
          <span
            ><strong>Nur-Lese-Ansicht</strong> für die Betriebsprüfung: {dateLabel($auditView.from)} bis
            {dateLabel($auditView.to)}. Änderungen sind gesperrt.</span
          >
          <button type="button" class="secondary" onclick={closeAuditView}>Beenden</button>
        </div>{/if}
      {#if !data}<div class="loading">E-Rechnung-Studio wird geladen …</div>
      {:else if section === "dashboard"}<Dashboard
          invoices={data.invoices ?? []}
          loadSummary={API.DashboardSummary}
          oncreate={() => createInvoice()}
          onopen={openInvoice}
          onnavigate={(target) =>
            target === "banking" ? navigate("finance", "pending") : navigate(target)}
          onaccount={() => openAccount()}
          onopeninvoices={async () => {
            await navigate("invoices");
            if (section !== "invoices") return;
            invoiceTab = "invoices";
            invoiceFilterOpen = true;
          }}
          ontaxes={(task) => {
            financePeriod = task;
            void navigate("taxes");
          }}
          balances={paymentOverview?.invoices ?? []}
          disabled={navigationLocked}
        />
      {:else if section === "taxes"}<TaxesPage initialTask={financePeriod} />
      {:else if section === "finance"}<BankingPage
          company={data.company}
          initialTab={financeTab}
          bind:selected={bankingSelected}
          bind:configuring={bankingConfiguring}
          onchanged={refreshApplicationLists}
          onbusychange={(value) => (paymentsBusy = value)}
        />
      {:else if section === "shops"}<ShopsPage
          connectProvider={shopToConnect}
          onconnectopened={() => (shopToConnect = "")}
          onopeninvoice={openShopInvoice}
          onchanged={refreshApplicationLists}
          onbusychange={(value) => (shopsBusy = value)}
          confirmDisconnect={(label) =>
            confirmAction(
              "Shop trennen",
              `„${label}“ trennen? Zugangsdaten werden entfernt. Importierte Bestellungen und Rechnungen bleiben erhalten.`,
              "Shop trennen",
            )}
        />
      {:else if section === "community"}<CommunityPage
          postalCode={data.company.address?.postcode ?? ""}
          city={data.company.address?.city ?? ""}
          version={updateStatus.currentVersion}
          onaccount={() => openAccount()}
          confirm={confirmAction}
        />
      {:else if section === "received"}<ReceivedInvoices
          bind:this={receivedPage}
          onbusychange={(value) => (receivedBusy = value)}
          {pickFile}
        />
      {:else if section === "invoices" && invoiceStep === "overview" && invoiceTab === "quotes"}<QuotesPage
          bind:this={quotesPage}
          bind:editor={quoteEditor}
          company={data.company}
          customers={data.customers ?? []}
          products={data.products ?? []}
          template={data.draft}
          startNew={quoteStartNew}
          onstartnew={() => (quoteStartNew = false)}
          tiles={invoiceTiles}
          {busy}
          {error}
          {run}
          confirm={confirmAction}
          notify={(text) => (message = text)}
          {pickFile}
          onshowtab={(tab) => (invoiceTab = tab)}
          onopeninvoice={async (id) => {
            await run(refreshApplicationLists);
            await openInvoice({ id } as InvoiceSummary);
          }}
          oneditcompany={() => resolveField("company.name")}
          oncustomersaved={(customer) => (data!.customers ??= []).push(customer)}
          ondirtychange={(dirty) => (quoteDirty = dirty)}
          onmailsettings={() => {
            settingsTab = "mail";
            void navigate("settings");
          }}
        />
      {:else if section === "invoices"}
        {#if invoiceStep === "overview"}
          <header class="topbar">
            <div><h1>Kundenrechnungen</h1></div>
          </header>
          <section class="invoice-dashboard">
            {@render invoiceTiles()}
            <DocumentTabs
              active={invoiceTab}
              disabled={busy}
              onchange={(tab) => (invoiceTab = tab)}
            />
            {#if invoiceTab === "recurring"}<RecurringInvoices
                {busy}
                {error}
                {run}
                confirm={confirmAction}
                notify={(text) => (message = text)}
                onchanged={runRecurringInvoices}
              />{:else}
              <ListHeader
                count="{data.invoices?.length ?? 0} {data.invoices?.length === 1
                  ? 'Rechnung'
                  : 'Rechnungen'}"
                placeholder="Rechnungen suchen"
                bind:query={invoiceQuery}
                filter={invoiceFilterOpen ? "Nur offene Rechnungen" : ""}
                onclearfilter={() => (invoiceFilterOpen = false)}
              />
              {#if draft && invoiceStep === "overview" && (draft.customer.name || unfinishedInvoice) && !draftListed()}<button
                  class="resume-invoice"
                  onclick={resumeDraft}
                  ><Icon name="invoices" />
                  {draft.customer.name
                    ? `${draft.status === "draft" ? "Entwurf fortsetzen" : "Geöffnete Rechnung anzeigen"} · ${draft.customer.name}`
                    : "Neue Rechnung fortsetzen · Kunde auswählen"}</button
                >{/if}
              {#if visibleInvoices().length}<InvoiceList
                  invoices={visibleInvoices()}
                  balances={paymentOverview?.invoices ?? []}
                  currentId={draft?.id}
                  onopen={openInvoice}
                  {busy}
                />{:else}<div class="empty-state">
                  <h2>
                    {invoiceQuery ? "Keine Treffer" : "Deine erste Rechnung"}
                  </h2>
                  <p>
                    {invoiceQuery
                      ? "Versuche es mit einer anderen Nummer oder einem anderen Kundennamen."
                      : "Erstelle eine Rechnung. Hier erscheinen dann deine Rechnungen."}
                  </p>
                </div>{/if}
              {#if deletedCount}<button
                  class="text deleted-log-link"
                  disabled={busy}
                  onclick={() => (deletedLogOpen = true)}
                  >Gelöschte Rechnungen ({deletedCount})</button
                >{/if}
              {#if deletedLogOpen}<DeletedInvoicesDialog
                  company={data.company}
                  {busy}
                  ondraft={draftFromDeleted}
                  onclose={() => (deletedLogOpen = false)}
                />{/if}
            {/if}
          </section>
        {:else if invoiceStep === "customer"}
          <PageHeader
            back={draft?.customer.name ? "Zurück zur Rechnung" : "Alle Rechnungen"}
            onback={() => {
              if (draft?.customer.name) invoiceStep = "document";
              else void showInvoiceList();
            }}
            title={draft?.customer.name ? "Anderen Kunden wählen" : "Neue Rechnung"}
            description="An wen geht die Rechnung? Wähle einen gespeicherten Kunden, lege einen neuen an oder nimm eine frühere Rechnung als Vorlage."
            disabled={navigationLocked || navigationPending}
          />
          <CustomerChooser
            customers={data.customers ?? []}
            bind:customer={prospectiveCustomer}
            {busy}
            onchoose={chooseInvoiceCustomer}
            oncreate={createInvoiceCustomer}
            invoices={data.invoices ?? []}
            ontemplate={chooseInvoiceTemplate}
          />
        {:else if draft}
          <div id="invoice-editor-start"></div>
          <PageHeader
            back="Alle Rechnungen"
            onback={showInvoiceList}
            title={draft.number
              ? `${documentLabel(draft.kind)} ${draft.number}`
              : documentLabel(draft.kind)}
            badge={statusLabel(draft.status)}
            badgeClass={draft.status}
            disabled={navigationLocked || navigationPending}
          >
            {#snippet actions()}
              {#if draft?.status === "draft"}<span class="save-state" role="status"
                  >{unsaved ? "Ungespeichert" : "Gespeichert"}</span
                ><button class="primary" disabled={busy || !unsaved} onclick={saveDraft}
                  ><Icon name="save" size={16} />
                  {busy ? "Wird gespeichert …" : "Speichern"}</button
                >{/if}
            {/snippet}
          </PageHeader>
          <InvoiceDocument
            bind:this={invoiceDocument}
            {previewPending}
            bind:draft
            company={view?.company ?? data.company}
            disabled={busy || draft.status !== "draft"}
            onchange={schedulePreview}
            onsettings={() => {
              error = "";
              settingsFocusField = "";
              pageSettingsOpen = true;
            }}
            onedittax={() => resolveField("taxTreatment")}
            oneditcompany={() => resolveField("company.name")}
            onchangecustomer={showCustomerStep}
            oneditline={editLines}
            oneditorstatechange={(dirty) => (dialogDirty = dirty)}
          />
          <InvoiceCompletion
            invoices={data.invoices ?? []}
            onpaymentschange={async () => {
              await refreshApplicationLists();
              if (draft?.id) acceptInvoiceView(await API.OpenInvoice(draft.id));
            }}
            onpaymentbusy={(value) => (paymentsBusy = value)}
            onmailsent={(text) => {
              message = text;
              void run(refreshApplicationLists);
            }}
            onmailsettings={() => {
              settingsTab = "mail";
              void navigate("settings");
            }}
            {draft}
            {view}
            {busy}
            {previewPending}
            {issuing}
            {exportedPDF}
            {resolveField}
            {deleteDraft}
            {validate}
            {issue}
            {retryExport}
            {openPDF}
            {exportPDF}
            {exportXML}
            {createCorrection}
            {useInvoiceAsTemplate}
            {repeatInvoice}
            applyTaxReview={(updated) => {
              draft = updated;
              schedulePreview();
            }}
            markSent={markInvoiceSent}
            deleteIssued={() => (deletingInvoice = true)}
          />
          {#if deletingInvoice && draft.id}<DeleteInvoiceDialog
              invoiceId={draft.id}
              kind={draft.kind}
              ondeleted={invoiceDeleted}
              oncorrection={() => {
                deletingInvoice = false;
                void createCorrection();
              }}
              oncancel={() => (deletingInvoice = false)}
            />{/if}
        {/if}
      {:else if section === "customers"}
        {#if selectedCustomer}
          <PageHeader
            back="Alle Kunden"
            onback={() => {
              if (localBackPage) void goBack();
              else selectedCustomerId = "";
            }}
            title={selectedCustomer.name}
            disabled={navigationLocked || navigationPending}
          >
            {#snippet actions()}
              <button
                class="secondary"
                onclick={() => {
                  error = "";
                  editingCustomer = clone(selectedCustomer!);
                }}
                disabled={busy}>Kundendaten bearbeiten</button
              ><button
                class="primary"
                onclick={() => startCustomerInvoice(selectedCustomer!)}
                disabled={busy}><Icon name="plus" size={16} /> Neue Rechnung</button
              >
            {/snippet}
          </PageHeader>
          <section class="customer-summary">
            <div>
              <span>Adresse</span>
              <p>
                {selectedCustomer.address.street}<br />
                {#if selectedCustomer.address.additional}{selectedCustomer.address.additional}<br
                  />{/if}
                {selectedCustomer.address.postcode}
                {selectedCustomer.address.city} · {selectedCustomer.address.country}
              </p>
            </div>
            <div>
              <span>Kontakt</span>
              <p>
                {selectedCustomer.contactName || "—"}<br />{selectedCustomer.email ||
                  "Keine E-Mail hinterlegt"}
              </p>
            </div>
            <div>
              <span>Zahlungsziel</span>
              <p>{selectedCustomer.paymentTermDays ?? DEFAULT_PAYMENT_DAYS} Tage</p>
            </div>
          </section>
          <section class="customer-history">
            <ListHeader
              count="{customerInvoices.length} {customerInvoices.length === 1
                ? 'Rechnung'
                : 'Rechnungen'}"
            />
            {#if customerInvoices.length}<InvoiceList
                invoices={customerInvoices}
                balances={paymentOverview?.invoices ?? []}
                onopen={openInvoice}
                {busy}
              />{:else}<div class="empty-state">
                <h2>Noch keine Rechnungen</h2>
                <p>
                  Die Rechnungen dieses Kunden erscheinen hier, sobald du einen Entwurf speicherst.
                </p>
              </div>{/if}
          </section>
        {:else}
          <header class="topbar">
            <div>
              <h1>Kunden</h1>
            </div>
          </header>
          <section class="invoice-dashboard">
            <ActionTiles tiles={customerTileItems} disabled={busy} />
            <ListHeader
              count="{visibleCustomers.length} {visibleCustomers.length === 1 ? 'Kunde' : 'Kunden'}"
              placeholder="Kunden suchen"
              bind:query={customerQuery}
              filter={customerFilterOpen ? "Nur Kunden mit offenen Rechnungen" : ""}
              onclearfilter={() => (customerFilterOpen = false)}
            />
            {#if visibleCustomers.length}<div class="invoice-rows">
                {#each visibleCustomers as c (c.id)}
                  {@const invoices = (data.invoices ?? []).filter(
                    (i) => i.customerId === c.id,
                  ).length}
                  {@const open = customerOpen.get(c.id)}
                  <EntryRow
                    title={c.name}
                    subtitle={[c.address.city, c.address.country].filter(Boolean).join(", ")}
                    meta="{invoices} {invoices === 1 ? 'Rechnung' : 'Rechnungen'}"
                    tooltip="Kunde öffnen"
                    disabled={busy}
                    onclick={() => {
                      pushPage();
                      selectedCustomerId = c.id;
                    }}
                  >
                    {#snippet status()}
                      {#if open}<PaymentStatus
                          summary={{
                            title: open.overdueCount ? "Überfällig" : "Offen",
                            description: `${amountLabel(open.amount)} offen`,
                            overdue: open.overdueCount > 0,
                          }}
                        />{:else}<span></span>{/if}
                    {/snippet}
                  </EntryRow>
                {/each}
              </div>{:else}<div class="empty-state">
                <h2>{customerQuery ? "Keine Treffer" : "Alle deine Kunden an einem Ort"}</h2>
                <p>
                  {customerQuery
                    ? "Versuche es mit einem anderen Namen oder Ort."
                    : "Lege einen Kunden an, um Rechnungen und Kontaktdaten gemeinsam zu verwalten."}
                </p>
              </div>{/if}
          </section>
        {/if}
      {:else if section === "products"}<header class="topbar">
          <div>
            <h1>Artikel & Leistungen</h1>
          </div>
        </header>
        <section class="invoice-dashboard">
          <ActionTiles tiles={productTileItems} disabled={busy} />
          <ListHeader
            count="{visibleProducts.length} Artikel"
            placeholder="Artikel suchen"
            bind:query={productQuery}
          />
          {#if visibleProducts.length}<div class="invoice-rows">
              {#each visibleProducts as p (p.id)}
                <EntryRow
                  title={p.name}
                  subtitle={p.description}
                  meta="{p.taxRate} % USt."
                  amount={amountLabel(p.unitPrice)}
                  tooltip="Artikel bearbeiten"
                  disabled={busy}
                  onclick={() => {
                    error = "";
                    editingProduct = clone(p);
                  }}
                />
              {/each}
            </div>{:else}<div class="empty-state">
              <h2>{productQuery ? "Keine Treffer" : "Leistungen einfach wiederverwenden"}</h2>
              <p>
                {productQuery
                  ? "Versuche es mit einer anderen Bezeichnung."
                  : "Speichere Bezeichnung, Preis und Umsatzsteuer für Positionen, die du oft brauchst. Einzelne Leistungen kannst du auch direkt auf der Rechnung eingeben."}
              </p>
            </div>{/if}
        </section>
      {:else}<SettingsPage
          bind:activeTab={settingsTab}
          bind:company={data.company}
          bind:settings={data.settings}
          {shopConnected}
          {busy}
          dirty={settingsChanged()}
          saving={settingsSaving}
          onsave={saveCompany}
          onsectionpending={setPendingSave}
          {chooseExportDirectory}
          {exportBackup}
          {importBackup}
          onopenbanking={() => navigate("finance", "accounts")}
        />{/if}
    </main>
  </div>
{/if}
{#if pageSettingsOpen && draft && data}<InvoiceSettingsDialog
    {draft}
    company={data.company}
    {busy}
    {error}
    focusField={settingsFocusField}
    onsave={savePageSettings}
    ondirtychange={(dirty) => (dialogDirty = dirty)}
    oncancel={() => (pageSettingsOpen = false)}
    oncompany={() => {
      pageSettingsOpen = false;
      resolveField("company.endpoint");
    }}
  />{/if}
{#if editingLines}<PositionEditor
    lines={editingLines}
    products={data?.products ?? []}
    standardTax={draft?.taxTreatment === "standard"}
    taxTreatment={draft?.taxTreatment}
    mixedTax={draft?.taxTreatment === "mixed"}
    defaultLineTax={data?.company.smallBusiness ? "small-business" : "standard"}
    signedQuantities={draft?.kind === "corrected-invoice"}
    initialIndex={editingLineIndex}
    onsave={saveLines}
    ondirtychange={(dirty) => (dialogDirty = dirty)}
    oncancel={() => (editingLines = null)}
  />{/if}
{#if editingCustomer}<CustomerDialog
    bind:customer={editingCustomer}
    {busy}
    {error}
    onsave={saveCustomer}
    ondirtychange={(dirty) => (dialogDirty = dirty)}
    ondelete={deleteCustomer}
    onclose={() => (editingCustomer = null)}
  />{/if}
{#if editingProduct}<ProductDialog
    bind:product={editingProduct}
    {busy}
    {error}
    onsave={saveProduct}
    ondirtychange={(dirty) => (dialogDirty = dirty)}
    ondelete={deleteProduct}
    onclose={() => (editingProduct = null)}
  />{/if}
{#if inspection}<XMLInspectionDialog {inspection} onclose={() => (inspection = null)} />{/if}
{#if repeating}<RecurringDialog
    bind:plan={repeating}
    {busy}
    {error}
    takesOverInvoice={Boolean(repeating.id)}
    onsave={saveRepeat}
    onclose={() => (repeating = null)}
  />{/if}
{#if confirmation}
  <FormDialog title={confirmation.title} onclose={() => answerConfirmation("Abbrechen")}>
    <p>{confirmation.message}</p>
    {#snippet footer()}
      <button type="button" class="secondary" onclick={() => answerConfirmation("Abbrechen")}
        >Abbrechen</button
      >
      {#each confirmation?.actions ?? [] as action}
        <button
          type="button"
          class={/verwerfen|löschen|wiederherstellen/i.test(action)
            ? "secondary danger-text"
            : "primary"}
          onclick={() => answerConfirmation(action)}>{action}</button
        >
      {/each}
    {/snippet}
  </FormDialog>
{/if}
{#if navigationLocked || cancelFileDialog}<div class="busy" role="status">
    {cancelFileDialog ? "Dateiauswahl geöffnet …" : "Wird verarbeitet …"}
    {#if cancelFileDialog}<button type="button" class="secondary" onclick={cancelFileDialog}
        >Dateiauswahl abbrechen</button
      >{/if}
  </div>{/if}{#if message}<div
    class="toast"
    role="status"
    transition:fly={{ y: 8, duration: nativeWindows ? 120 : 180 }}
  >
    <Icon name="check" size={17} /><span>{message}</span>
    <button aria-label="Hinweis schließen" onclick={() => (message = "")}
      ><Icon name="close" size={17} /></button
    >
  </div>{/if}{#if error && !editingCustomer && !editingProduct && !pageSettingsOpen}<aside
    class="error-notice no-print"
    role="alert"
  >
    <div>
      <strong>{error}</strong>{#if errorDetails}<details class="chevron-toggle">
          <summary>Technische Details</summary>
          <pre>{errorDetails}</pre>
        </details>{/if}
    </div>
    <button aria-label="Fehlermeldung schließen" onclick={() => (error = "")}
      ><Icon name="close" size={18} /></button
    >
  </aside>{/if}
