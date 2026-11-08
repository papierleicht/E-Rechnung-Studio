/**
 * In-memory backend of the browser demo. It answers the Wails service calls
 * with the sample data from `data.ts`. Changes live only in this tab: a reload
 * starts over, nothing is stored or sent. Features that need the operating
 * system or a server (files, FinTS, SMTP, account, backup) explain that they
 * work in the desktop app.
 */
import type {
  Company,
  Customer,
  InputTaxChoice,
  InvoiceDraft,
  InvoiceSummary,
  InvoiceView,
  Product,
  Quote,
  QuoteSummary,
  QuoteView,
  ReceivedInvoice,
  RecurringInvoice,
} from "../../bindings/papierleicht.de/e-rechnung-studio/internal/model/models";
import type {
  DeletedInvoice,
  InstallmentPlan,
  Payment,
  PaymentBalance,
  Reminder,
  ReminderProposal,
} from "../../bindings/papierleicht.de/e-rechnung-studio/internal/store/models";
import type {
  BookingReview,
  FinanceEntry,
  FinanceFigure,
  FinanceReport,
  FinanceYear,
  PaymentBooking,
} from "../../bindings/papierleicht.de/e-rechnung-studio/internal/app/models";
import type { Booking } from "../../bindings/papierleicht.de/e-rechnung-studio/internal/banking/models";
import * as seed from "./data";
import { calculate, documentHTML, previewFonts, validate } from "./document";
import { awaitsInputTaxDecision, countsForInputTax } from "../lib/received";
import { receivedAttachments } from "./attachments";
import { desktopOnly } from "./hints";
import { inspectFile } from "./inspect";
import {
  addDays,
  cents,
  day,
  daysBetween,
  fixed,
  germanDate,
  money,
  newID,
  parseAmount,
} from "./money";
import { emit } from "./events";
import { applyStress, stressMode } from "./stress";
import appVersion from "../../../version.txt?raw";

const db = structuredClone({
  settings: { automaticUpdates: false, exportDirectory: "", appearance: "system" },
  company: seed.company,
  customers: seed.customers,
  products: seed.products,
  invoices: seed.invoices,
  quotes: seed.quotes,
  received: seed.received,
  bookings: seed.bookings,
  payments: seed.payments,
  treatments: seed.treatments,
  reminders: seed.reminders,
  reminderSettings: seed.reminderSettings,
  financeSettings: seed.financeSettings,
  datevSettings: seed.datevSettings,
  mail: seed.mailSettings,
  mailLog: seed.mailLog,
  auditView: { open: false, from: "", to: "" },
  locks: [] as { id: string; from: string; to: string; at: string }[],
  procedure: [] as { version: number; at: string; appVersion: string }[],
  procedureProfile: {
    responsible: "",
    advisor: "",
    paperReceipts: "",
    cash: false,
    otherSystems: "",
    devices: "",
    backupRoutine: "",
  },
  plans: [] as InstallmentPlan[],
  deleted: [] as { entry: DeletedInvoice; draft: InvoiceDraft }[],
  counters: seed.counters,
  connection: seed.connection,
  account: seed.account,
  shopConnections: [seed.shopConnection],
  shopOrders: seed.shopOrders,
  synced: false,
});
// `?stress` makes every external text long, for layout checks (see stress.ts).
if (stressMode()) applyStress(db);

const now = () => new Date().toISOString();
const clone = <T>(value: T): T => structuredClone(value);
const yearOf = (iso: string) => Number((iso || day()).slice(0, 4));
const title = (kind: string) =>
  kind === "credit-note"
    ? "Gutschrift"
    : kind === "corrected-invoice"
      ? "Rechnungskorrektur"
      : "Rechnung";

function find<T extends { id: string }>(list: T[], id: string, label: string): T {
  const item = list.find((entry) => entry.id === id);
  if (!item) throw new Error(`Dieser Eintrag (${label}) ist nicht mehr vorhanden.`);
  return item;
}

// ---- Invoices -------------------------------------------------------------

function nextNumber(issueDate: string) {
  return `RE-${yearOf(issueDate)}-${String(db.counters.invoice + 1).padStart(4, "0")}`;
}

function withNumber(d: InvoiceDraft): InvoiceDraft {
  if (d.status === "draft" && d.automaticNumber !== false) d.number = nextNumber(d.issueDate);
  if (!d.paymentReference) d.paymentReference = d.number;
  return d;
}

function invoiceView(d: InvoiceDraft): InvoiceView {
  const company = d.status === "draft" ? db.company : (d.issuerSnapshot ?? db.company);
  const calc = calculate(d);
  return {
    export: { pdfPath: "", xmlPath: "" },
    draft: d,
    company,
    totals: calc.totals,
    lineTotals: calc.lineTotals,
    validation: d.status === "draft" ? validate(d, company) : { state: "valid", issues: [] },
    previewHtml: "",
  };
}

function storeDraft(d: InvoiceDraft): InvoiceDraft {
  const stored = clone(d);
  stored.updatedAt = now();
  if (!stored.id) {
    stored.id = newID("inv");
    stored.createdAt = stored.updatedAt;
    db.invoices.push(stored);
  } else {
    const index = db.invoices.findIndex((item) => item.id === stored.id);
    if (index < 0) db.invoices.push(stored);
    else if (db.invoices[index].status !== "draft")
      throw new Error("Diese Rechnung ist schon ausgestellt und lässt sich nicht mehr ändern.");
    else db.invoices[index] = stored;
  }
  return stored;
}

function summaries(): InvoiceSummary[] {
  return [...db.invoices]
    .sort((a, b) => (b.issueDate + b.number).localeCompare(a.issueDate + a.number))
    .map((d) => {
      const gross = calculate(d).totals.gross;
      const balance = d.status === "draft" ? null : balanceOf("outgoing", d.id);
      const sent = db.mailLog.find((entry) => entry.documentId === d.id && entry.status === "sent");
      return {
        id: d.id,
        number: d.number,
        customerId: d.customerId,
        customerName: d.customer.name,
        issueDate: d.issueDate,
        status: d.status === "draft" ? "draft" : balance?.state === "bezahlt" ? "paid" : "issued",
        gross,
        updatedAt: d.updatedAt,
        ...(sent
          ? { sentAt: sent.sentAt, sentVia: "mail" }
          : d.markedSentAt
            ? { sentAt: d.markedSentAt, sentVia: "manual" }
            : {}),
      };
    });
}

function lists() {
  return { customers: clone(db.customers), products: clone(db.products), invoices: summaries() };
}

function newDraft(): InvoiceDraft {
  const d = seed.blankDraft();
  d.number = nextNumber(d.issueDate);
  return d;
}

function issue(input: InvoiceDraft): InvoiceView {
  const view = invoiceView(withNumber(clone(input)));
  if (view.validation.state !== "valid")
    throw new Error(
      "Die Rechnung hat noch markierte Angaben. Korrigiere sie, bevor du die Rechnung ausstellst.",
    );
  const d = view.draft;
  if (d.automaticNumber === false && d.number) {
    const taken = numberTaken(d.number, d.id);
    if (taken) throw new Error(`Die Rechnungsnummer ${d.number} ist schon vergeben.`);
  } else {
    db.counters.invoice++;
    d.number = `RE-${yearOf(d.issueDate)}-${String(db.counters.invoice).padStart(4, "0")}`;
  }
  d.paymentReference = input.paymentReference || d.number;
  delete d.markedSentAt;
  d.status = "issued";
  d.issuedAt = now();
  d.issuerSnapshot = clone(db.company);
  d.lastValidationState = "valid";
  const stored = storeIssued(d);
  if (stored.quoteId) {
    const quote = db.quotes.find((q) => q.id === stored.quoteId);
    if (quote)
      Object.assign(quote, { status: "converted", invoiceId: stored.id, convertedAt: now() });
  }
  const result = invoiceView(stored);
  result.export = { pdfPath: `Live-Demo/${stored.number}.pdf`, xmlPath: "" };
  return result;
}

function storeIssued(d: InvoiceDraft): InvoiceDraft {
  d.updatedAt = now();
  if (!d.id) {
    d.id = newID("inv");
    d.createdAt = d.updatedAt;
  }
  db.invoices = db.invoices.filter((item) => item.id !== d.id);
  db.invoices.push(clone(d));
  return clone(d);
}

// ---- Invoice numbers and deletion -----------------------------------------
// A small version of the desktop rules: unique numbers, deletion only while
// nobody has the invoice, and a log that keeps every deleted invoice.

const numberPattern = /^[A-Za-z0-9\-/_.:+]{1,36}$/;

function numberTaken(number: string, id = "") {
  const key = number.toLowerCase();
  return (
    db.invoices.find(
      (d) => d.id !== id && d.status !== "draft" && d.number.toLowerCase() === key,
    ) ?? db.deleted.find((x) => x.entry.number.toLowerCase() === key)?.entry
  );
}

function nextFree(number: string) {
  const match = /^(.*?)(\d+)(\D*)$/.exec(number);
  if (!match) return "";
  let n = Number(match[2]);
  let candidate = number;
  do candidate = match[1] + String(++n).padStart(match[2].length, "0") + match[3];
  while (numberTaken(candidate));
  return candidate;
}

function checkNumber(d: InvoiceDraft, input: string) {
  const number = input.trim();
  const result = {
    number,
    state: "ok",
    message: "Die Nummer ist frei.",
    hints: [] as string[],
    series: "",
    pattern: "",
    counter: 0,
    next: number ? nextFree(number) : "",
    canContinue: false,
  };
  if (!numberPattern.test(number)) {
    result.state = "invalid";
    result.message = number
      ? "Verwende Buchstaben ohne Umlaute, Ziffern und „-“, „/“, „_“, „.“, „:“ oder „+“ (höchstens 36 Zeichen)."
      : "Gib eine Rechnungsnummer ein.";
  } else if (numberTaken(number, d.id)) {
    result.state = "taken";
    result.message = "Die Nummer ist schon vergeben.";
  }
  return result;
}

function deletionCheck(id: string) {
  const d = find(db.invoices, id, "Rechnung");
  const blockers: string[] = [];
  if (db.mailLog.some((entry) => entry.documentId === id && entry.status === "sent"))
    blockers.push("Du hast sie per E-Mail verschickt.");
  if (d.markedSentAt) blockers.push("Du hast sie als verschickt markiert.");
  if (db.payments.some((p) => p.invoiceId === id && p.direction === "outgoing"))
    blockers.push("Für sie sind Zahlungen erfasst.");
  if (db.reminders.some((r) => r.invoiceId === id))
    blockers.push("Es gibt schon Zahlungserinnerungen oder Mahnungen dazu.");
  if (db.invoices.some((x) => x.correctsInvoiceId === id))
    blockers.push("Es gibt eine Korrektur dazu.");
  return { allowed: !blockers.length, number: d.number, kind: d.kind, blockers, files: [] };
}

function draftFromDeleted(id: string) {
  const source = db.deleted.find((x) => x.entry.id === id);
  if (!source) throw new Error("Die gelöschte Rechnung wurde nicht gefunden.");
  const d = clone(source.draft);
  Object.assign(d, {
    id: "",
    number: "",
    automaticNumber: true,
    status: "draft",
    issueDate: day(),
    paymentReference: d.paymentReference === d.number ? "" : d.paymentReference,
    issuerSnapshot: null,
    issuedAt: "",
    paidAt: "",
  });
  delete d.markedSentAt;
  return invoiceView(storeDraft(withNumber(d)));
}

// ---- Payments -------------------------------------------------------------

function partyOf(direction: string, id: string) {
  if (direction === "outgoing") {
    const d = find(db.invoices, id, "Rechnung");
    const gross = calculate(d).totals.gross;
    const negative = d.kind === "credit-note";
    return {
      number: d.number,
      partner: d.customer.name,
      iban: "",
      currency: d.currency,
      dueDate: d.dueDate,
      gross: negative ? fixed(-cents(gross)) : gross,
    };
  }
  const r = find(db.received, id, "Eingangsrechnung");
  return {
    number: r.inspection.invoiceNumber || r.filename,
    partner: r.inspection.sellerName || r.filename,
    iban: r.inspection.iban,
    currency: r.inspection.currency || "EUR",
    dueDate: r.inspection.dueDate,
    gross: r.inspection.totals.gross || "0.00",
  };
}

function balanceOf(direction: string, id: string): PaymentBalance {
  const party = partyOf(direction, id);
  const paid = db.payments
    .filter((p) => p.invoiceId === id && p.direction === direction && !p.revokedAt)
    .reduce((sum, p) => sum + cents(p.amount), 0);
  const remaining = cents(party.gross) - paid;
  let state = remaining <= 0 ? "bezahlt" : paid > 0 ? "teilbezahlt" : "offen";
  if (remaining > 0 && party.dueDate && party.dueDate < day()) state = "überfällig";
  const plan = db.plans.find((p) => p.invoiceId === id && p.active);
  const next = plan?.rates?.find((rate) => rate.date >= day());
  const overdue = (plan?.rates ?? [])
    .filter((rate) => rate.date < day())
    .reduce((sum, rate) => sum + cents(rate.amount), 0);
  return {
    id,
    direction,
    credit: false,
    ...party,
    paid: fixed(paid),
    remaining: fixed(Math.max(remaining, 0)),
    overpaid: fixed(Math.max(-remaining, 0)),
    state,
    installments: Boolean(plan),
    nextDate: next?.date ?? "",
    nextAmount: next?.amount ?? "",
    overdueAmount: plan
      ? fixed(Math.max(0, Math.min(overdue - paid + cents(plan.baselinePaid), remaining)))
      : "",
    planNeedsReview: false,
  };
}

function overview() {
  const invoices = [
    ...db.invoices.filter((d) => d.status !== "draft").map((d) => balanceOf("outgoing", d.id)),
    ...db.received.filter((r) => r.status !== "misimport").map((r) => balanceOf("incoming", r.id)),
  ];
  return { invoices, payments: clone(db.payments) };
}

function paymentBooking(b: Booking): PaymentBooking {
  return {
    id: b.id,
    accountId: b.accountId,
    source: "fints",
    data: clone(b.data),
    duplicate: false,
  };
}

function reconcile() {
  const open = overview().invoices.filter((b) => b.state !== "bezahlt");
  const used = new Set(
    db.payments.filter((p) => !p.revokedAt && p.bookingId).map((p) => p.bookingId),
  );
  const treated = new Map(db.treatments.filter((t) => t.kind).map((t) => [t.bookingId, t]));
  const pending: BookingReview[] = [];
  for (const b of db.bookings) {
    if (used.has(b.id) || treated.has(b.id)) continue;
    const direction = b.data.amount > 0 ? "outgoing" : "incoming";
    const amount = Math.abs(b.data.amount); // FinTS amounts are in cents
    const text = `${b.data.description ?? ""} ${b.data.counterparty_name ?? ""}`.toLowerCase();
    const suggestions = open
      .filter((invoice) => invoice.direction === direction)
      .map((invoice) => {
        const reasons: string[] = [];
        if (invoice.number && text.includes(invoice.number.toLowerCase()))
          reasons.push("Rechnungsnummer steht im Verwendungszweck");
        if (cents(invoice.remaining) === amount) reasons.push("Betrag entspricht dem offenen Rest");
        if (invoice.partner && text.includes(invoice.partner.toLowerCase().split(" ")[0]))
          reasons.push("Name passt zum Zahlenden");
        return { invoice, reasons, certain: reasons.length >= 2 };
      })
      .filter((s) => s.reasons.length > 0)
      .sort((a, b) => b.reasons.length - a.reasons.length);
    pending.push({
      booking: paymentBooking(b),
      remaining: fixed(Math.sign(b.data.amount) * amount),
      suggestions,
      expenses: [],
    });
  }
  const other = db.bookings
    .filter((b) => treated.has(b.id))
    .map((b) => ({ booking: paymentBooking(b), treatment: treated.get(b.id)! }));
  return { other, automatic: 0, pending, overview: overview() };
}

function recordPayment(p: Payment): Payment {
  const amount = parseAmount(p.amount);
  if (!Number.isFinite(amount) || amount === 0)
    throw new Error("Der Betrag ist ungültig. Gib eine Zahl ungleich 0 ein.");
  const booking = db.bookings.find((b) => b.id === p.bookingId);
  const stored: Payment = {
    id: newID("pay"),
    invoiceId: p.invoiceId,
    direction: p.direction,
    amount: amount.toFixed(2),
    currency: p.currency || "EUR",
    date: p.date || booking?.data.booking_date || day(),
    source: p.source || "manual",
    bookingId: p.bookingId || "",
    reason: p.reason || "",
    createdAt: now(),
    revokedAt: "",
    revocationReason: "",
  };
  db.payments.push(stored);
  return clone(stored);
}

// ---- Reminders ------------------------------------------------------------

function fillTemplate(text: string, values: Record<string, string>) {
  return text.replace(/\{([^{}]+)\}/g, (match, name: string) => values[name.trim()] ?? match);
}

function proposal(id: string): ReminderProposal {
  const d = find(db.invoices, id, "Rechnung");
  const balance = balanceOf("outgoing", id);
  const history = db.reminders.filter((r) => r.invoiceId === id);
  const last = history.at(-1) ?? null;
  const level = Math.min(history.length + 1, 3);
  const settings = db.reminderSettings.levels![level - 1];
  const today = day();
  const daysOverdue = d.dueDate ? Math.max(0, daysBetween(d.dueDate, today)) : 0;
  const nextDate = last
    ? addDays(last.paymentDue, settings.waitDays)
    : d.dueDate
      ? addDays(d.dueDate, settings.waitDays)
      : "";
  const fee = cents(settings.fee) || 0;
  const eligible = d.status !== "draft" && cents(balance.remaining) > 0 && history.length < 3;
  return {
    today,
    invoiceId: id,
    number: d.number,
    partner: d.customer.name,
    currency: d.currency,
    eligible,
    reason: eligible
      ? ""
      : history.length >= 3
        ? "Alle drei Mahnstufen sind schon erstellt."
        : "Die Rechnung ist komplett bezahlt.",
    due: eligible && nextDate !== "" && nextDate <= today,
    complete: cents(balance.remaining) <= 0,
    level,
    title: settings.title,
    amount: balance.gross,
    remaining: balance.remaining,
    fee: fixed(fee),
    interest: "0.00",
    total: fixed(cents(balance.remaining) + fee),
    dueSince: d.dueDate,
    daysOverdue,
    nextDate,
    paymentDue: addDays(today, settings.paymentDays),
    installment: false,
    last,
  };
}

function reminderTexts(p: ReminderProposal) {
  const d = find(db.invoices, p.invoiceId, "Rechnung");
  const level = db.reminderSettings.levels![p.level - 1];
  const values = {
    rechnungsnummer: d.number,
    rechnungsdatum: germanDate(d.issueDate),
    betrag: money(cents(p.total)) + " €",
    faelligkeit: germanDate(d.dueDate),
    frist: germanDate(p.paymentDue),
    firma: db.company.name,
    kunde: d.customer.name,
  };
  return { subject: fillTemplate(level.subject, values), text: fillTemplate(level.text, values) };
}

function recordReminder(id: string, channel: string): Reminder {
  const p = proposal(id);
  if (!p.eligible) throw new Error("Für diese Rechnung gibt es keine weitere Erinnerung.");
  const texts = reminderTexts(p);
  const reminder: Reminder = {
    id: newID("rem"),
    invoiceId: id,
    number: p.number,
    level: p.level,
    title: p.title,
    date: p.today,
    currency: p.currency,
    amount: p.remaining,
    fee: p.fee,
    interest: p.interest,
    total: p.total,
    dueSince: p.dueSince,
    paymentDue: p.paymentDue,
    installment: false,
    channel,
    ...texts,
    fileName: `${p.number}-${p.title.replace(/\W+/g, "-")}.pdf`,
    documentSha256: "",
    createdAt: now(),
  };
  db.reminders.push(reminder);
  return clone(reminder);
}

// ---- Quotes ---------------------------------------------------------------

function quoteView(q: Quote): QuoteView {
  const calc = calculate(q.document);
  const invoice = q.invoiceId ? db.invoices.find((d) => d.id === q.invoiceId) : undefined;
  const validation =
    q.status === "draft"
      ? validate({ ...q.document, dueDate: q.document.dueDate || q.validUntil }, db.company)
      : { state: "valid", issues: [] };
  return {
    quote: q,
    company: db.company,
    totals: calc.totals,
    lineTotals: calc.lineTotals,
    validation,
    ...(invoice ? { invoiceNumber: invoice.number } : {}),
  };
}

function quoteNumber(q: Quote) {
  if (
    q.status === "draft" &&
    q.automaticNumber !== false &&
    !db.quotes.some((x) => x.id === q.id && x.number === q.number && q.number)
  )
    q.number = `AN-${yearOf(q.document.issueDate)}-${String(db.counters.quote + 1).padStart(4, "0")}`;
  q.document.number = q.number;
  return q;
}

function saveQuote(input: Quote): QuoteView {
  const q = clone(input);
  q.updatedAt = now();
  if (!q.id) {
    db.counters.quote++;
    q.id = newID("quo");
    q.number =
      q.automaticNumber === false && q.number
        ? q.number
        : `AN-${yearOf(q.document.issueDate)}-${String(db.counters.quote).padStart(4, "0")}`;
    q.status = "draft";
    q.createdAt = q.updatedAt;
    db.quotes.push(q);
  } else {
    const index = db.quotes.findIndex((x) => x.id === q.id);
    if (index >= 0 && db.quotes[index].status !== "draft")
      throw new Error("Dieses Angebot ist schon versendet und lässt sich nicht mehr ändern.");
    if (index < 0) db.quotes.push(q);
    else db.quotes[index] = q;
  }
  q.document.number = q.number;
  return quoteView(clone(q));
}

function quoteSummaries(): QuoteSummary[] {
  return [...db.quotes]
    .sort((a, b) => b.number.localeCompare(a.number))
    .map((q) => ({
      id: q.id,
      number: q.number,
      customerId: q.document.customerId,
      customerName: q.document.customer.name,
      issueDate: q.document.issueDate,
      validUntil: q.validUntil,
      status: q.status,
      gross: calculate(q.document).totals.gross,
      invoiceId: q.invoiceId ?? "",
      invoiceNumber: db.invoices.find((d) => d.id === q.invoiceId)?.number ?? "",
      updatedAt: q.updatedAt,
    }));
}

// ---- Dashboard & finance --------------------------------------------------

function bucket(items: PaymentBalance[]) {
  return {
    count: items.length,
    amount: fixed(items.reduce((sum, b) => sum + cents(b.remaining), 0)),
    foreign: 0,
  };
}

function quarter() {
  const today = new Date();
  const q = Math.floor(today.getMonth() / 3);
  const from = new Date(today.getFullYear(), q * 3, 1, 12).toLocaleDateString("sv-SE");
  const to = new Date(today.getFullYear(), q * 3 + 3, 0, 12).toLocaleDateString("sv-SE");
  return { label: `Q${q + 1} ${today.getFullYear()}`, from, to };
}

function dashboard() {
  const all = overview().invoices.filter((b) => b.state !== "bezahlt");
  const receivable = all.filter((b) => b.direction === "outgoing");
  const payable = all.filter((b) => b.direction === "incoming");
  const q = quarter();
  return {
    today: day(),
    receivable: bucket(receivable),
    receivableOverdue: bucket(receivable.filter((b) => b.state === "überfällig")),
    payable: bucket(payable),
    payableOverdue: bucket(payable.filter((b) => b.state === "überfällig")),
    nextDue: [...all]
      .filter((b) => b.dueDate)
      .sort((a, b) => a.dueDate.localeCompare(b.dueDate))
      .slice(0, 5),
    unassignedBookings: reconcile().pending.length,
    receivedToReview: db.received.filter(
      (r) => r.status === "active" && awaitsInputTaxDecision(r.inspection),
    ).length,
    plansToReview: 0,
    refundsToCredit: 0,
    taxProfileMissing: false,
    financeLabel: q.label,
    financeFrom: q.from,
    financeTo: q.to,
    financeIssues: [],
  };
}

function figure(
  label: string,
  area: string,
  explanation: string,
  items: (Omit<FinanceEntry, "date"> & { date?: string })[],
  amount?: number,
): FinanceFigure {
  const entries = items.map((e) => ({ date: "", ...e }));
  const sum = amount ?? entries.reduce((total, e) => total + cents(e.amount), 0);
  return {
    label,
    area,
    explanation,
    amount: fixed(sum),
    available: true,
    incomplete: false,
    entries,
  };
}

function financeReport(from: string, to: string): FinanceReport {
  if (!from || !to || to < from)
    throw new Error("Der Zeitraum ist ungültig. Wähle einen gültigen Zeitraum.");
  const inRange = (date: string) => date >= from && date <= to;
  const label = (p: Payment) => {
    const party = partyOf(p.direction, p.invoiceId);
    return `Zahlung zu ${party.number} · ${party.partner}`;
  };
  const active = db.payments.filter((p) => !p.revokedAt && inRange(p.date));
  const inflows = active
    .filter((p) => p.direction === "outgoing")
    .map((p) => ({ kind: "payment", id: p.id, date: p.date, label: label(p), amount: p.amount }));
  const outflows = active
    .filter((p) => p.direction === "incoming")
    .map((p) => ({
      kind: "payment",
      id: p.id,
      date: p.date,
      label: label(p),
      amount: fixed(-cents(p.amount)),
    }));
  const other = db.treatments
    .filter((t) => t.kind)
    .map((t) => db.bookings.find((b) => b.id === t.bookingId)!)
    .filter((b) => b && inRange(b.data.booking_date))
    .map((b) => ({
      kind: "booking",
      id: b.id,
      date: b.data.booking_date,
      label: b.data.counterparty_name || b.data.description || "Bankumsatz",
      amount: fixed(b.data.amount),
    }));
  // As in the backend, payments to the tax office count as expenses, refunds as income.
  const vatMovements = db.treatments
    .filter((t) => t.kind === "vat")
    .map((t) => db.bookings.find((b) => b.id === t.bookingId)!)
    .filter((b) => b && inRange(b.data.booking_date))
    .map((b) => ({
      kind: "vat",
      id: b.id,
      date: b.data.booking_date,
      label: b.data.counterparty_name || b.data.description || "Finanzamt",
      amount: fixed(b.data.amount),
      note: "Umsatzsteuer (Finanzamt)",
    }));
  const open = overview().invoices.filter((b) => b.state !== "bezahlt");
  const receivables = open
    .filter((b) => b.direction === "outgoing")
    .map((b) => ({
      kind: "invoice",
      id: b.id,
      date: b.dueDate,
      label: `${b.number} · ${b.partner}`,
      amount: b.remaining,
    }));
  const payables = open
    .filter((b) => b.direction === "incoming")
    .map((b) => ({
      kind: "received",
      id: b.id,
      date: b.dueDate,
      label: `${b.number} · ${b.partner}`,
      amount: b.remaining,
    }));
  const issued = db.invoices.filter((d) => d.status !== "draft" && inRange(d.issueDate));
  const rates = new Map<string, { category: string; percent: string; net: number; tax: number }>();
  const outputEntries: FinanceEntry[] = [];
  for (const d of issued) {
    const calc = calculate(d);
    outputEntries.push({
      kind: "invoice",
      id: d.id,
      date: d.issueDate,
      label: `${d.number} · ${d.customer.name}`,
      amount: calc.totals.tax,
    });
    for (const g of calc.groups) {
      const key = `${g.category}/${g.rate}`;
      const line = rates.get(key) ?? {
        category: g.category,
        percent: String(g.rate),
        net: 0,
        tax: 0,
      };
      line.net += g.basis;
      line.tax += g.tax;
      rates.set(key, line);
    }
  }
  const countable = db.received.filter(
    (r) =>
      r.status !== "misimport" &&
      r.inspection.issueDate &&
      r.inspection.totals.tax &&
      countsForInputTax(r.inspection),
  );
  const paidEntries = countable.filter(countsOnPayment).flatMap((r) =>
    active
      .filter((p) => p.direction === "incoming" && p.invoiceId === r.id)
      .map((p) => ({
        kind: "payment",
        id: p.id,
        date: p.date,
        label: label(p),
        amount: fixed(
          (cents(r.inspection.totals.tax) * cents(p.amount)) / cents(r.inspection.totals.gross),
        ),
        note: "Mit der Zahlung",
      })),
  );
  const inputEntries = countable
    .filter((r) => !countsOnPayment(r) && inRange(r.inspection.issueDate))
    .map<FinanceEntry>((r) => ({
      kind: "received",
      id: r.id,
      date: r.inspection.issueDate,
      label: `${r.inspection.invoiceNumber} · ${r.inspection.sellerName}`,
      amount: countedInputTax(r),
    }))
    .concat(paidEntries);
  const income = figure(
    "Betriebseinnahmen",
    "euer",
    "Alle Zahlungseingänge im Zeitraum (Zuflussprinzip), brutto inklusive Umsatzsteuer. Ob jeder Eingang steuerlich eine Betriebseinnahme ist, prüft die App nicht.",
    inflows.concat(vatMovements.filter((e) => cents(e.amount) > 0)),
  );
  const expenses = figure(
    "Betriebsausgaben",
    "euer",
    "Alle bezahlten Eingangsrechnungen im Zeitraum (Abflussprinzip), brutto inklusive Vorsteuer, dazu Umsatzsteuer-Zahlungen ans Finanzamt. Ob und in welcher Höhe du sie abziehen kannst, prüft die App nicht.",
    outflows.concat(vatMovements.filter((e) => cents(e.amount) < 0)),
  );
  const result = cents(income.amount) + cents(expenses.amount);
  const outputTax = figure(
    "Umsatzsteuer",
    "ust",
    db.financeSettings.taxBasis === "ist"
      ? "Umsatzsteuer aus den Kundenrechnungen, die du im Zeitraum bezahlt bekommen hast (Ist-Versteuerung)."
      : "Umsatzsteuer aus deinen Kundenrechnungen, berechnet nach Rechnungsdatum (Soll-Versteuerung).",
    outputEntries,
  );
  const inputTax = figure(
    "Vorsteuer",
    "ust",
    "Steuer deiner Eingangsrechnungen und erfassten Belege nach Rechnungsdatum, Anzahlungen nach Zahlung, wie die App sie vorschlägt oder du sie im Beleg festgelegt hast. Ob du sie abziehen darfst, prüft die App nicht.",
    inputEntries,
  );
  const vatPayable = cents(outputTax.amount) - cents(inputTax.amount);
  const balance = db.account.balance?.booked.amount ?? 0;
  const bankBalance = figure(
    "Kontostand",
    "liquiditaet",
    "Je Konto der neueste Stand, entweder von der Bank abgerufen oder von dir eingetragen. Er zeigt das Guthaben auf dem Konto, nicht dein Ergebnis.",
    [
      {
        kind: "balance",
        id: db.account.id,
        date: db.account.balance?.booked.date ?? "",
        label: `${db.connection.label} · Geschäftskonto`,
        amount: fixed(balance),
      },
    ],
  );
  const vatReserve = Math.max(vatPayable, 0);
  const percent = parseAmount(db.financeSettings.reservePercent) || 0;
  const incomeReserve = Math.max(0, Math.round((result * percent) / 100));
  const payablesFigure = figure(
    "Anstehende Zahlungen",
    "zahlungen",
    "Offene Restbeträge aus deinen Eingangsrechnungen und auszuzahlende Gutschriften, Stand heute.",
    payables,
  );
  const available = balance - cents(payablesFigure.amount) - vatReserve - incomeReserve;
  // A rough stand-in for the backend's estimate: the reserve rate on the net of
  // open customer invoices less open supplier invoices, taken at 19 %.
  const openNet = (list: { amount: string }[]) =>
    list.reduce((sum, e) => sum + cents(e.amount), 0) / (db.company.smallBusiness ? 1 : 1.19);
  const pendingReserve = Math.max(
    0,
    Math.round(((openNet(receivables) - openNet(payables)) * percent) / 100),
  );
  // As in the backend: bank bookings of the period without invoice or treatment.
  const used = new Set(db.payments.filter((p) => !p.revokedAt).map((p) => p.bookingId));
  const treated = new Set(db.treatments.filter((t) => t.kind).map((t) => t.bookingId));
  const unassigned = db.bookings
    .filter((b) => !used.has(b.id) && !treated.has(b.id) && inRange(b.data.booking_date))
    .map((b) => ({
      kind: "booking",
      id: b.id,
      date: b.data.booking_date,
      label: b.data.counterparty_name || b.data.description || "Bankumsatz",
      amount: fixed(b.data.amount),
      note: "nicht zugeordnet",
    }));
  const issues = unassigned.length
    ? [
        {
          code: "unassigned_bookings",
          area: "zahlungen",
          message:
            "Bankumsätze ohne Rechnung oder Einordnung. Sie fehlen in den Zahlungen und in der EÜR.",
          entries: unassigned,
        },
      ]
    : [];
  return {
    from,
    to,
    today: day(),
    taxBasis: db.financeSettings.taxBasis,
    smallBusiness: db.company.smallBusiness,
    complete: issues.length === 0,
    inflows: figure(
      "Zahlungseingänge",
      "zahlungen",
      "Zahlungen auf deine Kundenrechnungen, nach Zahlungsdatum. Auszahlungen zu Gutschriften sind abgezogen.",
      inflows,
    ),
    outflows: figure(
      "Zahlungsausgänge",
      "zahlungen",
      "Zahlungen auf deine Eingangsrechnungen, nach Zahlungsdatum.",
      outflows,
    ),
    other: figure(
      "Sonstige Geldbewegungen",
      "zahlungen",
      "Bankumsätze ohne Rechnung, die du eingeordnet hast, z. B. Privatentnahmen, Umbuchungen oder Steuerzahlungen. Sie zählen nicht zu Einnahmen oder Ausgaben.",
      other,
    ),
    receivables: figure(
      "Offene Forderungen",
      "zahlungen",
      "Restbeträge aus deinen Kundenrechnungen, Stand heute. Teilzahlungen sind abgezogen.",
      receivables,
    ),
    payables: payablesFigure,
    pendingTax: figure(
      "Steuern auf offene Rechnungen",
      "liquiditaet",
      "Was die offenen Rechnungen dem Steuer-Topf hinzufügen, sobald sie bezahlt sind. Eine Schätzung mit deinem Rücklagensatz, keine Steuerberechnung.",
      pendingReserve
        ? [
            {
              kind: "figure",
              id: "pendingReserve",
              label: `${percent} % Rücklage Einkommensteuer`,
              amount: fixed(pendingReserve),
              note: "auf die offenen Rechnungen netto, offene Ausgaben abgezogen",
            },
          ]
        : [],
    ),
    income,
    expenses,
    result: figure(
      "Ergebnis",
      "euer",
      "",
      [
        { kind: "figure", id: "income", label: "Betriebseinnahmen", amount: income.amount },
        { kind: "figure", id: "expenses", label: "Betriebsausgaben", amount: expenses.amount },
      ],
      result,
    ),
    outputTax,
    outputTaxRates: [...rates.values()].map((r) => ({
      category: r.category,
      percent: r.percent,
      net: fixed(r.net),
      tax: fixed(r.tax),
    })),
    inputTax,
    vatPayable: figure(
      "Zahllast",
      "ust",
      "",
      [
        { kind: "figure", id: "outputTax", label: "Umsatzsteuer", amount: outputTax.amount },
        {
          kind: "figure",
          id: "inputTax",
          label: "Vorsteuer",
          amount: fixed(-cents(inputTax.amount)),
        },
      ],
      vatPayable,
    ),
    bankBalance,
    vatReserve: figure(
      "Offene Umsatzsteuer",
      "liquiditaet",
      "Umsatzsteuer, die noch ans Finanzamt geht. In der Demo: die Zahllast des Zeitraums.",
      [{ kind: "figure", id: "vatPayable", label: "Zahllast", amount: fixed(vatPayable) }],
      vatReserve,
    ),
    incomeTaxReserve: figure(
      "Rücklage Einkommensteuer",
      "liquiditaet",
      "",
      percent
        ? [
            {
              kind: "figure",
              id: "result",
              label: "Ergebnis",
              amount: fixed(result),
              note: `${percent} %`,
            },
          ]
        : [],
      incomeReserve,
    ),
    available: figure(
      "Frei verfügbar",
      "liquiditaet",
      "Kontostand abzüglich anstehender Zahlungen, offener Umsatzsteuer und Rücklage. Offene Forderungen zählen nicht mit, weil das Geld noch nicht da ist.",
      [
        { kind: "figure", id: "bankBalance", label: "Kontostand", amount: fixed(balance) },
        {
          kind: "figure",
          id: "payables",
          label: "Anstehende Zahlungen",
          amount: fixed(-cents(payablesFigure.amount)),
        },
        {
          kind: "figure",
          id: "vatReserve",
          label: "Offene Umsatzsteuer",
          amount: fixed(-vatReserve),
        },
        {
          kind: "figure",
          id: "incomeTaxReserve",
          label: "Rücklage Einkommensteuer",
          amount: fixed(-incomeReserve),
        },
      ],
      available,
    ),
    notCovered: [
      "Abschreibungen auf Anlagegüter und andere Buchungen ohne Zahlung",
      "Regelmäßig wiederkehrende Zahlungen um den Jahreswechsel (10-Tage-Regel)",
      "Einkommensteuer-Prognose: Dafür fehlen ein geprüftes Modell und deine persönlichen Angaben",
    ],
    issues,
  };
}

// Like the backend: each month is the regular report for that month.
function financeYear(year: number): FinanceYear {
  const today = day();
  const months = Array.from({ length: 12 }, (_, i) => {
    const from = `${year}-${String(i + 1).padStart(2, "0")}-01`;
    const to = `${year}-${String(i + 1).padStart(2, "0")}-${new Date(year, i + 1, 0).getDate()}`;
    if (from > today)
      return {
        month: i + 1,
        from,
        to,
        income: "0.00",
        expenses: "0.00",
        vat: "0.00",
        result: "0.00",
        incomplete: false,
        future: true,
      };
    const r = financeReport(from, to);
    const vat = (r.expenses.entries ?? [])
      .filter((e) => e.kind === "vat")
      .reduce((sum, e) => sum + Math.abs(cents(e.amount)), 0);
    return {
      month: i + 1,
      from,
      to,
      income: r.income.amount,
      expenses: fixed(Math.abs(cents(r.expenses.amount))),
      vat: fixed(vat),
      result: r.result.amount,
      incomplete: r.income.incomplete || r.expenses.incomplete,
      future: false,
    };
  });
  return { year, today, months };
}

const datevCharts: Record<string, typeof seed.datevSettings.accounts> = {
  SKR03: seed.datevSettings.accounts,
  SKR04: {
    revenue: {
      standard19: "4400",
      reduced7: "4300",
      zeroRated: "4290",
      exempt: "4100",
      export: "4120",
      intraCommunity: "4125",
      reverseCharge: "4337",
      reverseChargeEU: "4336",
      outOfScope: "4338",
      smallBusiness: "4184",
      margin: "4138",
    },
    expense: "6850",
    bank: "1800",
    debtor: "10000",
    creditor: "70000",
  },
};

function datevAccounts(chart: string, length: number) {
  const base = datevCharts[chart];
  if (!base) throw new Error("Wähle SKR03 oder SKR04.");
  if (length < 4 || length > 8)
    throw new Error("Die Sachkontenlänge muss zwischen 4 und 8 liegen.");
  const pad = (s: string) => s.padEnd(length, "0");
  const revenue = Object.fromEntries(
    Object.entries(base.revenue).map(([k, v]) => [k, pad(v)]),
  ) as unknown as typeof base.revenue;
  return {
    revenue,
    expense: pad(base.expense),
    bank: pad(base.bank),
    debtor: "1" + "0".repeat(length),
    creditor: "7" + "0".repeat(length),
  };
}

function datevPreview(from: string, to: string) {
  const inRange = (date: string) => date >= from && date <= to;
  const invoices = db.invoices.filter((d) => d.status !== "draft" && inRange(d.issueDate));
  const received = db.received.filter(
    (r) => r.status === "active" && r.inspection.issueDate && inRange(r.inspection.issueDate),
  );
  const payments = db.payments.filter((p) => !p.revokedAt && inRange(p.date)).length;
  const settings = clone(db.datevSettings);
  const issues =
    settings.consultantNumber && settings.clientNumber
      ? []
      : [
          {
            code: "numbers",
            severity: "blocking",
            message:
              "Trag deine Berater- und Mandantennummer ein. Beide bekommst du von deiner Steuerkanzlei.",
            entries: [],
          },
        ];
  return {
    from,
    to,
    fiscalYearStart: `${from.slice(0, 4)}-${String(settings.fiscalYearStartMonth).padStart(2, "0")}-01`,
    ready: issues.length === 0,
    bookings: invoices.length + received.length + (settings.includePayments ? payments : 0),
    invoices: invoices.filter((d) => d.kind !== "credit-note").length,
    creditNotes: invoices.filter((d) => d.kind === "credit-note").length,
    received: received.length,
    payments: settings.includePayments ? payments : 0,
    documents: settings.includeDocuments ? invoices.length + received.length : 0,
    newAccounts: 0,
    issues,
    notCovered: [
      "Anlagevermögen und Abschreibungen",
      "Lohnbuchhaltung",
      "Bankumsätze ohne Rechnung",
    ],
    settings,
  };
}

// ---- Received invoices ----------------------------------------------------

function receivedID(filename: string, encoded: string) {
  let hash = 0;
  for (let i = 0; i < encoded.length; i += 7) hash = (hash * 31 + encoded.charCodeAt(i)) | 0;
  return `rcv-${filename.length}-${(hash >>> 0).toString(16)}`;
}

/** Whether the Vorsteuer of a receipt follows its payments instead of its date. */
function countsOnPayment(r: ReceivedInvoice) {
  const status = r.inspection.inputTax;
  return status?.choice ? status.choice.mode === "payment" : !!status?.onPayment;
}

/** The Vorsteuer of a receipt in the figures: the user's decision, or else the app's suggestion. */
function countedInputTax(r: ReceivedInvoice) {
  const status = r.inspection.inputTax;
  if (status?.choice?.mode === "amount") return status.choice.amount ?? "0.00";
  if (status?.choice) return r.inspection.totals.tax;
  return status?.tax ?? r.inspection.totals.tax;
}

function kindOf(filename: string) {
  return /\.pdf$/i.test(filename) ? "pdf" : "xml";
}

// ---- Mail -----------------------------------------------------------------

function mailDraft(documentType: string, id: string, level?: number) {
  const account = db.mail.account;
  const templates: Record<string, { subject: string; body: string }> = {
    invoice: {
      subject: "Rechnung {Rechnungsnummer} von {Firma}",
      body: "Guten Tag,\n\nanbei erhalten Sie unsere Rechnung {Rechnungsnummer} über {Betrag}.\nSie ist am {Fälligkeit} fällig.\n\nBei Fragen antworten Sie einfach auf diese E-Mail.\n\nMit freundlichen Grüßen\n{Firma}",
    },
    quote: {
      subject: "Angebot {Angebotsnummer} von {Firma}",
      body: "Guten Tag,\n\nanbei erhalten Sie unser Angebot {Angebotsnummer} über {Betrag}.\nDas Angebot gilt bis {Gültig bis}.\n\nBei Fragen antworten Sie einfach auf diese E-Mail.\n\nMit freundlichen Grüßen\n{Firma}",
    },
  };
  let customer: Customer, values: Record<string, string>, filename: string, docTitle: string;
  let template = {
    ...(templates[documentType] ?? templates.invoice),
    ...(db.mail.templates?.[documentType] ?? {}),
  };
  if (documentType === "quote") {
    const q = find(db.quotes, id, "Angebot");
    customer = q.document.customer;
    values = {
      Angebotsnummer: q.number,
      Betrag: money(cents(calculate(q.document).totals.gross)) + " €",
      "Gültig bis": germanDate(q.validUntil),
      Kunde: customer.name,
      Firma: db.company.name,
    };
    filename = `${q.number}.pdf`;
    docTitle = `Angebot ${q.number}`;
  } else if (documentType === "reminder") {
    const d = find(db.invoices, id, "Rechnung");
    const p = proposal(id);
    const texts = reminderTexts({ ...p, level: level || p.level });
    customer = d.customer;
    template = { subject: texts.subject, body: texts.text };
    values = {};
    filename = `${d.number}-mahnung.pdf`;
    docTitle = `${p.title} zu ${d.number}`;
  } else {
    const d = find(db.invoices, id, "Rechnung");
    customer = d.customer;
    values = {
      Rechnungsnummer: d.number,
      Betrag: money(cents(calculate(d).totals.gross)) + " €",
      Fälligkeit: germanDate(d.dueDate),
      Kunde: customer.name,
      Firma: db.company.name,
    };
    filename = `${d.number}.pdf`;
    docTitle = `${title(d.kind)} ${d.number}`;
  }
  const attachment = { filename, contentType: "application/pdf", size: 48_000 };
  return {
    customer,
    attachment,
    title: docTitle,
    subject: fillTemplate(template.subject, values),
    body: fillTemplate(template.body, values),
    from: account.fromAddress
      ? `${account.fromName || db.company.name} <${account.fromAddress}>`
      : "",
    replyTo: account.replyTo,
    bccSelf: account.bccSelf,
    configured: Boolean(account.host && account.fromAddress),
    hasPassword: db.mail.hasPassword,
    log: db.mailLog.filter((entry) => entry.documentId === id),
  };
}

let recurring: RecurringInvoice[] = [];
const newRecurringID = () => `rec-${Date.now()}`;
const stored = (id?: string) => recurring.find((item) => item.id === id);
const recurringMonths: Record<string, number> = {
  monthly: 1,
  quarterly: 3,
  "half-yearly": 6,
  yearly: 12,
};
function addMonths(date: string, months: number) {
  const [year, month, dayOfMonth] = date.split("-").map(Number);
  const last = new Date(Date.UTC(year, month + months, 0)).getUTCDate();
  return new Date(Date.UTC(year, month - 1 + months, Math.min(dayOfMonth, last)))
    .toISOString()
    .slice(0, 10);
}
function withRecurringPeriod(r: RecurringInvoice) {
  const end = new Date(addMonths(r.periodStart, recurringMonths[r.interval] ?? 1) + "T00:00:00Z");
  end.setUTCDate(end.getUTCDate() - 1);
  r.periodEnd = end.toISOString().slice(0, 10);
  return r;
}
const mailHint = () => desktopOnly("der Versand über dein eigenes E-Mail-Postfach (SMTP)");
const shopHint = () => desktopOnly("die Anbindung deines Online-Shops");

// ---- Service table --------------------------------------------------------

type Handler = (...args: any[]) => unknown;

const handlers: Record<string, Handler> = {
  // InvoiceService: master data and invoices
  "InvoiceService.Bootstrap": () => ({
    settings: clone(db.settings),
    company: clone(db.company),
    ...lists(),
    draft: newDraft(),
  }),
  "InvoiceService.ApplicationLists": lists,
  "InvoiceService.SetUnsavedChanges": () => undefined,
  "InvoiceService.CompleteSetup": (c: Company) => (db.company = { ...c, setupCompleted: true }),
  "InvoiceService.SaveCompany": (c: Company) => {
    if (!c.name.trim()) throw new Error("Der Name deines Unternehmens fehlt. Trag ihn ein.");
    return (db.company = c);
  },
  "InvoiceService.ValidateCompanyStep": (c: Company, step: string) => {
    const issues =
      validate({ ...seed.blankDraft(), customer: { ...seed.customers[0] } }, c).issues ?? [];
    return issues.filter(
      (i) =>
        i.field?.startsWith("company.") &&
        (step !== "company" || !/contactName|phone|endpoint/.test(i.field)),
    );
  },
  "InvoiceService.SaveAppSettings": (v) => (db.settings = { ...v, automaticUpdates: false }),
  "InvoiceService.SaveSettings": (v, company: Company | null) => {
    db.settings = { ...v, automaticUpdates: false };
    if (company) db.company = company;
    return { settings: db.settings, company: db.company };
  },
  "InvoiceService.PrepareCompanyLogo": (encoded: string) => ({
    logo: encoded,
    width: 600,
    height: 240,
    warning: "",
  }),
  "InvoiceService.SaveCustomer": (c: Customer) => {
    if (!c.name.trim()) throw new Error("Der Name des Kunden fehlt. Trag ihn ein.");
    const stored = { ...c, updatedAt: now() };
    if (!stored.id) {
      stored.id = newID("cus");
      stored.createdAt = stored.updatedAt;
      db.customers.push(stored);
    } else db.customers = db.customers.map((x) => (x.id === stored.id ? stored : x));
    db.customers.sort((a, b) => a.name.localeCompare(b.name, "de"));
    return stored;
  },
  "InvoiceService.DeleteCustomer": (id: string) => {
    if (db.invoices.some((d) => d.customerId === id && d.status !== "draft"))
      throw new Error(
        "Dieser Kunde hat ausgestellte Rechnungen. Du kannst ihn deshalb nicht löschen.",
      );
    db.customers = db.customers.filter((c) => c.id !== id);
  },
  "InvoiceService.SaveProduct": (p: Product) => {
    if (!p.name.trim()) throw new Error("Die Bezeichnung des Artikels fehlt. Trag sie ein.");
    const stored = { ...p, updatedAt: now() };
    if (!stored.id) {
      stored.id = newID("prd");
      stored.createdAt = stored.updatedAt;
      db.products.push(stored);
    } else db.products = db.products.map((x) => (x.id === stored.id ? stored : x));
    return stored;
  },
  "InvoiceService.DeleteProduct": (id: string) => {
    db.products = db.products.filter((p) => p.id !== id);
  },
  "InvoiceService.PreviewInvoice": (d: InvoiceDraft) => invoiceView(withNumber(d)),
  "InvoiceService.ValidateInvoice": (d: InvoiceDraft) => invoiceView(withNumber(d)),
  "InvoiceService.InvoiceDocumentHTML": (d: InvoiceDraft) => {
    const stored = d.id ? db.invoices.find((x) => x.id === d.id) : undefined;
    if (stored && stored.status !== "draft")
      return documentHTML(stored, stored.issuerSnapshot ?? db.company);
    return documentHTML(withNumber(d), db.company);
  },
  "InvoiceService.InvoicePreviewFonts": () => previewFonts,
  "InvoiceService.SaveInvoice": (d: InvoiceDraft) => invoiceView(storeDraft(withNumber(d))),
  "InvoiceService.IssueInvoice": issue,
  "InvoiceService.OpenInvoice": (id: string) => {
    const d = clone(find(db.invoices, id, "Rechnung"));
    return invoiceView(d.status === "draft" ? withNumber(d) : d);
  },
  "InvoiceService.DeleteDraft": (id: string) => {
    const d = find(db.invoices, id, "Rechnung");
    if (d.status !== "draft")
      throw new Error("Diese Rechnung ist ausgestellt. Du kannst sie nicht mehr löschen.");
    db.invoices = db.invoices.filter((x) => x.id !== id);
  },
  "InvoiceService.CreateCorrectionDraft": (id: string) => {
    const source = find(db.invoices, id, "Rechnung");
    const d = clone(source);
    Object.assign(d, {
      id: "",
      number: "",
      automaticNumber: true,
      kind: "credit-note",
      status: "draft",
      issueDate: day(),
      correctsInvoiceId: id,
      precedingInvoiceNumber: source.number,
      precedingInvoiceDate: source.issueDate,
      paymentReference: "",
      issuerSnapshot: null,
      issuedAt: "",
      paidAt: "",
    });
    return invoiceView(storeDraft(withNumber(d)));
  },
  "InvoiceService.InvoiceNumberOptions": (d: InvoiceDraft) => {
    const last = [...db.invoices]
      .filter((x) => x.status !== "draft")
      .sort((a, b) => b.number.localeCompare(a.number))[0];
    return {
      options: [
        {
          value: "",
          kind: "standard",
          label: "Standard",
          series: "RE-{JJJJ}-{NNNN}",
          pattern: "RE-JJJJ-NNNN",
          next: nextNumber(d.issueDate),
          ...(last ? { last: { number: last.number, issueDate: last.issueDate } } : {}),
        },
      ],
      selected: d.automaticNumber === false && d.number ? "manual" : "",
      customerId: d.customerId,
      customerName: d.customer.name,
      customerSeries: "",
    };
  },
  "InvoiceService.CheckInvoiceNumber": checkNumber,
  "InvoiceService.StartNumberSeries": () => {
    throw desktopOnly("das Anlegen eigener Nummernkreise");
  },
  "InvoiceService.InvoiceNumberSettings": () => ({
    series: "RE-{JJJJ}-{NNNN}",
    pattern: "RE-JJJJ-NNNN",
    next: nextNumber(day()),
    nextValue: db.counters.invoice + 1,
    minValue: db.counters.invoice + 1,
    customers: [],
  }),
  "InvoiceService.PreviewNumberSeries": () => {
    throw desktopOnly("ein eigenes Format für Rechnungsnummern");
  },
  "InvoiceService.SaveInvoiceNumberSettings": () => {
    throw desktopOnly("ein eigenes Format für Rechnungsnummern");
  },
  "InvoiceService.RemoveCustomerNumberSeries": () => {
    throw desktopOnly("eigene Nummernkreise für Kunden");
  },
  "InvoiceService.MarkInvoiceSent": (id: string, sent: boolean) => {
    const d = find(db.invoices, id, "Rechnung");
    if (d.status === "draft")
      throw new Error("Nur ausgestellte Rechnungen kannst du als verschickt markieren.");
    if (sent) d.markedSentAt = now();
    else delete d.markedSentAt;
  },
  "InvoiceService.InvoiceDeletionCheck": deletionCheck,
  "InvoiceService.DeleteIssuedInvoice": (id: string, reason: string) => {
    const check = deletionCheck(id);
    if (!check.allowed)
      throw new Error(
        `Die Rechnung kann nicht gelöscht werden. ${check.blockers.join(" ")} Hebe sie mit einer Stornorechnung auf.`,
      );
    if (!reason.trim()) throw new Error("Gib an, warum du die Rechnung löschst.");
    const d = find(db.invoices, id, "Rechnung");
    const entry: DeletedInvoice = {
      id,
      number: d.number,
      kind: d.kind,
      customerName: d.customer.name,
      issueDate: d.issueDate,
      issuedAt: d.issuedAt,
      gross: calculate(d).totals.gross,
      currency: d.currency,
      reason: reason.trim(),
      deletedAt: now(),
    };
    db.deleted.unshift({ entry, draft: clone(d) });
    db.invoices = db.invoices.filter((x) => x.id !== id);
    for (const quote of db.quotes)
      if (quote.invoiceId === id)
        Object.assign(quote, { status: "accepted", invoiceId: "", convertedAt: "" });
    return { invoice: clone(entry), removedFiles: [], keptFiles: [] };
  },
  "InvoiceService.DeletedInvoices": () => db.deleted.map((x) => clone(x.entry)),
  "InvoiceService.DeletedInvoiceHTML": (id: string) => {
    const source = db.deleted.find((x) => x.entry.id === id);
    if (!source) throw new Error("Die gelöschte Rechnung wurde nicht gefunden.");
    return documentHTML(source.draft, source.draft.issuerSnapshot ?? db.company);
  },
  "InvoiceService.DraftFromDeletedInvoice": draftFromDeleted,
  "InvoiceService.SetPaid": (id: string, paid: boolean) => {
    const balance = balanceOf("outgoing", id);
    if (paid && cents(balance.remaining) > 0)
      recordPayment({
        invoiceId: id,
        direction: "outgoing",
        amount: balance.remaining,
        date: day(),
        source: "manual",
        reason: "Als bezahlt markiert",
      } as Payment);
    if (!paid)
      db.payments.forEach((p) => {
        if (p.invoiceId === id && p.direction === "outgoing" && !p.revokedAt) {
          p.revokedAt = now();
          p.revocationReason = "Wieder als offen markiert";
        }
      });
  },
  "InvoiceService.DashboardSummary": dashboard,

  // Files and the operating system
  "InvoiceService.ExportInvoicePDF": () => {
    throw desktopOnly("das Speichern der PDF-Datei");
  },
  "InvoiceService.ExportInvoiceXML": () => {
    throw desktopOnly("das Speichern der XML-Datei");
  },
  "InvoiceService.ExportIssuedInvoice": (id: string) => ({
    pdfPath: `Live-Demo/${find(db.invoices, id, "Rechnung").number}.pdf`,
    xmlPath: "",
  }),
  "InvoiceService.OpenInvoicePDF": () => {
    throw desktopOnly("das Öffnen der fertigen PDF-Datei");
  },
  "InvoiceService.ExportQuotePDF": () => {
    throw desktopOnly("das Speichern des Angebots als PDF");
  },
  "InvoiceService.OpenQuotePDF": () => {
    throw desktopOnly("das Öffnen des Angebots als PDF");
  },
  "InvoiceService.OpenReceivedPDF": () => {
    throw desktopOnly("das Öffnen der Originaldatei");
  },
  "InvoiceService.SaveReceivedOriginal": () => {
    throw desktopOnly("das Speichern der Originaldatei");
  },
  "InvoiceService.ExportReminderPDF": (id: string) => recordReminder(id, "manual-export"),
  "InvoiceService.ExportStoredReminderPDF": () => {
    throw desktopOnly("das erneute Speichern des Schreibens");
  },
  "InvoiceService.ExportDatev": () => {
    throw desktopOnly("der DATEV-Export als Datei");
  },
  "InvoiceService.OpenDatevExportFolder": () => {
    throw desktopOnly("das Öffnen des Exportordners");
  },

  // Recurring invoices. The demo keeps them, but its dates never come due.
  "InvoiceService.ListRecurringInvoices": () => clone(recurring).map(withRecurringPeriod),
  "InvoiceService.RecurringFromInvoice": (id: string) => {
    const d = clone(find(db.invoices, id, "Rechnung"));
    const planned = stored(d.recurringId);
    if (planned) return withRecurringPeriod({ ...clone(planned), template: d });
    const next = addMonths(d.issueDate, 1);
    return withRecurringPeriod({
      id: "",
      name: d.lines?.[0]?.name || d.customer.name,
      interval: "monthly",
      nextDate: next,
      periodStart: d.periodStart || `${d.issueDate.slice(0, 7)}-01`,
      endDate: "",
      paused: false,
      template: d,
      gross: invoiceView(d).totals.gross,
      status: "active",
      openDrafts: 0,
    } as RecurringInvoice);
  },
  "InvoiceService.RecurringPreview": (r: RecurringInvoice) => withRecurringPeriod(clone(r)),
  "InvoiceService.SaveRecurringInvoice": (r: RecurringInvoice) => {
    const saved = withRecurringPeriod({ ...clone(r), id: r.id || newRecurringID() });
    recurring = [...recurring.filter((item) => item.id !== saved.id), saved];
    return clone(saved);
  },
  "InvoiceService.SetRecurringPaused": (id: string, paused: boolean) => {
    const plan = find(recurring, id, "Dauerrechnung");
    plan.paused = paused;
    plan.status = paused ? "paused" : "active";
    return clone(plan);
  },
  "InvoiceService.DeleteRecurringInvoice": (id: string) => {
    recurring = recurring.filter((item) => item.id !== id);
  },
  "InvoiceService.RunRecurringInvoices": () => ({ created: 0, failed: [] }),
  // Quotes
  "InvoiceService.ListQuotes": quoteSummaries,
  "InvoiceService.OpenQuote": (id: string) => quoteView(clone(find(db.quotes, id, "Angebot"))),
  "InvoiceService.PreviewQuote": (q: Quote) => quoteView(quoteNumber(q)),
  "InvoiceService.SaveQuote": saveQuote,
  "InvoiceService.QuoteDocumentHTML": (q: Quote) =>
    documentHTML(quoteNumber(q).document, db.company, { quoteValidUntil: q.validUntil }),
  "InvoiceService.MarkQuoteSent": (input: Quote) => {
    const view = input.status === "draft" ? saveQuote(input) : quoteView(input);
    const q = find(db.quotes, view.quote.id, "Angebot");
    Object.assign(q, { status: "sent", sentAt: now(), updatedAt: now() });
    return quoteView(clone(q));
  },
  "InvoiceService.SetQuoteStatus": (id: string, status: string) => {
    const q = find(db.quotes, id, "Angebot");
    Object.assign(q, {
      status,
      decidedAt: status === "accepted" || status === "rejected" ? now() : q.decidedAt,
      updatedAt: now(),
    });
    return quoteView(clone(q));
  },
  "InvoiceService.DeleteQuote": (id: string) => {
    db.quotes = db.quotes.filter((q) => q.id !== id);
  },
  "InvoiceService.ConvertQuoteToInvoice": (id: string) => {
    const q = find(db.quotes, id, "Angebot");
    const d = clone(q.document);
    Object.assign(d, {
      id: "",
      number: "",
      automaticNumber: true,
      status: "draft",
      issueDate: day(),
      deliveryDate: day(),
      dueDate: addDays(day(), d.customer.paymentTermDays ?? 14),
      quoteId: q.id,
      quoteNumber: q.number,
      notes:
        d.notes || `Gemäß unserem Angebot ${q.number} vom ${germanDate(q.document.issueDate)}.`,
    });
    const stored = storeDraft(withNumber(d));
    Object.assign(q, { status: "converted", invoiceId: stored.id, convertedAt: now() });
    return invoiceView(stored);
  },

  // Received invoices
  "InvoiceService.ListReceivedInvoices": () =>
    [...db.received].sort((a, b) => b.importedAt.localeCompare(a.importedAt)),
  "InvoiceService.InspectInvoiceFile": async (filename: string, encoded: string) =>
    (await inspectFile(filename, encoded)).inspection,
  "InvoiceService.PrepareReceivedInvoice": async (filename: string, encoded: string) => {
    const { inspection } = await inspectFile(filename, encoded);
    const id = receivedID(filename, encoded);
    return {
      inspection,
      pdf: kindOf(filename) === "pdf" ? encoded : "",
      previewError: "",
      duplicate: db.received.find((r) => r.id === id) ?? null,
    };
  },
  "InvoiceService.ImportReceivedInvoice": async (filename: string, encoded: string) => {
    const id = receivedID(filename, encoded);
    const existing = db.received.find((r) => r.id === id);
    if (existing) return { invoice: existing, duplicate: true };
    const { inspection } = await inspectFile(filename, encoded);
    const item: ReceivedInvoice = {
      id,
      filename,
      kind: kindOf(filename),
      importedAt: now(),
      size: Math.floor((encoded.length * 3) / 4),
      status: "active",
      history: [],
      inspection,
    };
    db.received.push(item);
    return { invoice: item, duplicate: false };
  },
  "InvoiceService.RecheckReceivedInvoice": (id: string) =>
    find(db.received, id, "Eingangsrechnung").inspection,
  "InvoiceService.ChangeReceivedStatus": (id: string, status: string, reason: string) => {
    const r = find(db.received, id, "Eingangsrechnung");
    r.status = status;
    r.history = [...(r.history ?? []), { action: status, reason, at: now() }];
    return r;
  },
  "InvoiceService.DeleteReceivedMisimport": (id: string) => {
    db.received = db.received.filter((r) => r.id !== id);
  },
  "InvoiceService.ExpenseCategories": () => [
    { id: "buero", label: "Büro und Material", hint: "", zeile: "", kz: "" },
    { id: "software", label: "Software und Lizenzen", hint: "", zeile: "", kz: "" },
    { id: "sonstige", label: "Sonstige Betriebsausgaben", hint: "", zeile: "", kz: "" },
  ],
  "InvoiceService.ReceivedCategories": () => ({}),
  "InvoiceService.Expenses": () => [],
  "InvoiceService.TaxSetup": () => ({
    smallBusiness: db.company.smallBusiness,
    vatPeriod: "month",
    vatExtension: false,
    specialPrepayment: "0.00",
    taxBasis: db.financeSettings.taxBasis,
    filing: "self",
    activity: db.financeSettings.activity ?? "",
  }),
  "InvoiceService.SaveInputTaxChoice": (c: InputTaxChoice) => {
    const r = find(db.received, c.receivedId, "Eingangsrechnung");
    const amount = c.mode === "amount" ? (c.amount ?? "").replace(",", ".") : "";
    if (c.mode === "amount" && !(Number(amount) >= 0 && amount !== "")) {
      throw new Error("Gib die Vorsteuer als Betrag ab 0 Euro an, zum Beispiel 94,20.");
    }
    r.inspection.inputTax = {
      allowed: false,
      ...r.inspection.inputTax,
      choice: { ...c, amount: amount && Number(amount).toFixed(2), decidedAt: now() },
    };
    return r.inspection;
  },
  "InvoiceService.ClearInputTaxChoice": (id: string) => {
    const r = find(db.received, id, "Eingangsrechnung");
    if (r.inspection.inputTax) delete r.inspection.inputTax.choice;
    return r.inspection;
  },
  "InvoiceService.ReceivedAttachments": (id: string) => receivedAttachments[id] ?? [],
  "InvoiceService.OpenReceivedAttachment": () => {
    throw desktopOnly("das Öffnen einer Anlage");
  },
  "InvoiceService.SaveReceivedAttachment": () => {
    throw desktopOnly("das Speichern einer Anlage");
  },

  // Payments and bank reconciliation
  "InvoiceService.Payments": overview,
  "InvoiceService.ReconcilePayments": reconcile,
  "InvoiceService.PaymentBookings": () => db.bookings.map(paymentBooking),
  "InvoiceService.RecordPayment": recordPayment,
  "InvoiceService.RevokePayment": (id: string, reason: string) => {
    const p = find(db.payments, id, "Zahlung");
    p.revokedAt = now();
    p.revocationReason = reason;
  },
  "InvoiceService.SetBookingTreatment": (id: string, kind: string, note: string) => {
    db.treatments = db.treatments.filter((t) => t.bookingId !== id);
    if (kind) db.treatments.push({ bookingId: id, kind, note, at: now() });
  },
  "InvoiceService.PreviewPaymentCSV": () => {
    throw desktopOnly("der Import von Kontoauszügen als CSV-Datei");
  },
  "InvoiceService.ImportPaymentCSV": () => {
    throw desktopOnly("der Import von Kontoauszügen als CSV-Datei");
  },
  "InvoiceService.InstallmentPlan": (id: string) =>
    db.plans.find((p) => p.invoiceId === id) ?? {
      invoiceId: id,
      currency: "EUR",
      baselinePaid: "0.00",
      rates: [],
      note: "",
      at: "",
      active: false,
    },
  "InvoiceService.PreviewInstallments": (id: string, rate: string, first: string) => {
    const balance = balanceOf("outgoing", id);
    const step = cents(rate);
    if (!(step > 0)) throw new Error("Die Rate muss größer als 0 sein. Gib einen Betrag ein.");
    let rest = cents(balance.remaining);
    const rates = [];
    for (let i = 0; rest > 0 && i < 60; i++) {
      const amount = Math.min(step, rest);
      const [y, m, d] = (first || day()).split("-").map(Number);
      rates.push({
        date: new Date(y, m - 1 + i, d, 12).toLocaleDateString("sv-SE"),
        amount: fixed(amount),
      });
      rest -= amount;
    }
    return {
      invoiceId: id,
      currency: balance.currency,
      baselinePaid: balance.paid,
      rates,
      note: "",
      at: "",
      active: true,
    };
  },
  "InvoiceService.SaveInstallmentPlan": (p: InstallmentPlan) => {
    db.plans = db.plans.filter((x) => x.invoiceId !== p.invoiceId);
    if (p.active) db.plans.push({ ...p, at: now() });
  },

  // Reminders
  "InvoiceService.InvoiceReminders": (id: string) => {
    const p = proposal(id);
    return {
      proposal: p,
      history: db.reminders.filter((r) => r.invoiceId === id),
      ...reminderTexts(p),
    };
  },
  "InvoiceService.DueReminders": () =>
    db.invoices
      .filter((d) => d.status !== "draft")
      .map((d) => proposal(d.id))
      .filter((p) => p.due),
  "InvoiceService.ReminderSettings": () => db.reminderSettings,
  "InvoiceService.DefaultReminderSettings": () => seed.reminderSettings,
  "InvoiceService.SaveReminderSettings": (v) => (db.reminderSettings = v),

  // Finance and DATEV
  "InvoiceService.FinanceReport": financeReport,
  "InvoiceService.FinanceYear": financeYear,
  "InvoiceService.FinanceSettings": () => db.financeSettings,
  "InvoiceService.SaveFinanceSettings": (f) => (db.financeSettings = { ...f, updatedAt: now() }),
  "InvoiceService.DatevSettings": () => db.datevSettings,
  "InvoiceService.SaveDatevSettings": (v) => (db.datevSettings = { ...v, updatedAt: now() }),
  "InvoiceService.DatevDefaultAccounts": datevAccounts,
  "InvoiceService.DatevExportPreview": datevPreview,

  // MailService
  "MailService.Settings": () => db.mail,
  "MailService.Presets": () => [
    {
      id: "gmail",
      label: "Gmail / Google Workspace",
      host: "smtp.gmail.com",
      port: 587,
      security: "starttls",
      hint: "Du brauchst die Bestätigung in zwei Schritten und ein App-Passwort.",
    },
    {
      id: "microsoft365",
      label: "Microsoft 365 / Exchange Online",
      host: "smtp.office365.com",
      port: 587,
      security: "starttls",
      hint: "Die Anmeldung per Passwort (SMTP AUTH) muss für dein Postfach freigeschaltet sein.",
    },
    {
      id: "ionos",
      label: "IONOS",
      host: "smtp.ionos.de",
      port: 587,
      security: "starttls",
      hint: "Benutzername ist die vollständige E-Mail-Adresse.",
    },
    {
      id: "strato",
      label: "Strato",
      host: "smtp.strato.de",
      port: 465,
      security: "tls",
      hint: "Benutzername ist die vollständige E-Mail-Adresse.",
    },
    {
      id: "gmx",
      label: "GMX",
      host: "mail.gmx.net",
      port: 587,
      security: "starttls",
      hint: "Aktiviere in den GMX-Einstellungen den Zugriff über externe Programme.",
    },
    {
      id: "webde",
      label: "web.de",
      host: "smtp.web.de",
      port: 587,
      security: "starttls",
      hint: "Aktiviere in den WEB.DE-Einstellungen den Zugriff über externe Programme.",
    },
  ],
  "MailService.Placeholders": (type: string) =>
    type === "quote"
      ? ["Angebotsnummer", "Betrag", "Gültig bis", "Kunde", "Firma"]
      : ["Rechnungsnummer", "Betrag", "Fälligkeit", "Kunde", "Firma"],
  // The password is never kept; the demo only remembers that one was entered.
  "MailService.SaveSettings": (v, password: string) =>
    (db.mail = { ...v, hasPassword: db.mail.hasPassword || Boolean(password) }),
  "MailService.ForgetPassword": () => (db.mail = { ...db.mail, hasPassword: false }),
  "MailService.SendTestMail": () => {
    throw mailHint();
  },
  "AuditService.Status": () => ({
    entries: auditEntries().length,
    plan: true,
    anchors: 1,
    firstAnchor: "2026-10-29T08:00:00Z",
    lastAnchor: "2026-10-29T08:00:00Z",
    health: {
      checkedAt: "2026-10-29T08:00:00Z",
      ok: true,
      problems: 0,
      comparedAt: "2026-10-29T08:00:00Z",
      matched: 1,
      restored: 0,
      conflicts: 0,
      stale: false,
    },
    view: { ...db.auditView },
    locks: db.locks,
    procedure: db.procedure.map(({ version, at, appVersion }) => ({ version, at, appVersion })),
  }),
  "AuditService.Verify": () => ({
    ok: true,
    entries: auditEntries().length,
    objects: db.invoices.length,
    files: 0,
    head: "",
    anchors: 1,
    checkedAt: now(),
    lastAnchor: "2026-10-29T08:00:00Z",
    problems: [],
  }),
  "AuditService.Entries": (f: { query: string }) =>
    auditEntries().filter((e) => e.label.toLowerCase().includes((f.query ?? "").toLowerCase())),
  "AuditService.LockPeriod": (from: string, to: string) => {
    const lock = { id: `lock-${db.locks.length + 1}`, from, to, at: now() };
    db.locks.push(lock);
    return lock;
  },
  "AuditService.Findings": () => [
    {
      level: "warning",
      title: "Ausgaben ohne Belegdatei",
      detail: "Jede Buchung braucht einen Beleg.",
      items: ["B-2026-0003 Tankstelle"],
    },
  ],
  "AuditService.Tables": () => [
    { name: "ausgangsrechnungen", title: "Ausgangsrechnungen", rows: db.invoices.length },
  ],
  "AuditService.Table": () => ({
    name: "ausgangsrechnungen",
    title: "Ausgangsrechnungen",
    columns: ["Rechnungsnummer", "Rechnungsdatum", "Kunde", "Brutto"],
    numeric: [false, false, false, true],
    rows: db.invoices
      .filter((d) => d.status !== "draft")
      .map((d) => [
        d.number,
        d.issueDate.split("-").reverse().join("."),
        d.customer.name,
        calculate(d).totals.gross.replace(".", ","),
      ]),
  }),
  "AuditService.SaveTable": () => {
    throw desktopOnly("das Speichern von Dateien");
  },
  "AuditService.Export": () => {
    throw desktopOnly("der Prüfer-Export");
  },
  "AuditService.OpenView": (from: string, to: string) => (db.auditView = { open: true, from, to }),
  "AuditService.CloseView": () => {
    db.auditView = { open: false, from: "", to: "" };
  },
  "AuditService.ProcedureProfile": () => db.procedureProfile,
  "AuditService.SaveProcedureProfile": (p: typeof db.procedureProfile) => {
    db.procedureProfile = p;
  },
  "AuditService.CreateProcedure": () => {
    const version = { version: db.procedure.length + 1, at: now(), appVersion: "1.0.0" };
    db.procedure.unshift(version);
    return version;
  },
  "AuditService.SaveProcedure": () => {
    throw desktopOnly("das Speichern von Dateien");
  },
  "MailService.InvoiceMailLog": (id: string) =>
    db.mailLog.filter((entry) => entry.documentId === id),
  "MailService.PrepareInvoiceMail": (id: string) => {
    const m = mailDraft("invoice", id);
    const d = find(db.invoices, id, "Rechnung");
    return {
      invoiceId: id,
      number: d.number,
      customerId: m.customer.id,
      customerName: m.customer.name,
      to: m.customer.email,
      canSaveEmail: Boolean(m.customer.id),
      subject: m.subject,
      body: m.body,
      from: m.from,
      replyTo: m.replyTo,
      bccSelf: m.bccSelf,
      xrechnung: d.profile.startsWith("xrechnung-"),
      includePdf: true,
      attachments: [m.attachment],
      pdfAttachment: m.attachment,
      configured: m.configured,
      hasPassword: m.hasPassword,
      log: m.log,
    };
  },
  "MailService.PrepareDocumentMail": (type: string, id: string, level: number) => {
    const m = mailDraft(type, id, level);
    return {
      documentType: type,
      documentId: id,
      level,
      title: m.title,
      customerId: m.customer.id,
      customerName: m.customer.name,
      to: m.customer.email,
      canSaveEmail: Boolean(m.customer.id),
      subject: m.subject,
      body: m.body,
      from: m.from,
      replyTo: m.replyTo,
      bccSelf: m.bccSelf,
      attachments: [m.attachment],
      configured: m.configured,
      hasPassword: m.hasPassword,
      log: m.log,
    };
  },
  "MailService.SendInvoiceMail": () => {
    throw mailHint();
  },
  "MailService.SendDocumentMail": () => {
    throw mailHint();
  },

  // ShopService: the demo shows one connected shop; changes need the desktop app.
  "ShopService.Overview": () => ({
    connections: db.shopConnections,
    orders: db.shopOrders,
    syncing: false,
  }),
  "ShopService.Connect": () => {
    throw shopHint();
  },
  "ShopService.Sync": () => {
    throw shopHint();
  },
  "ShopService.SetEnabled": () => {
    throw shopHint();
  },
  "ShopService.Disconnect": () => {
    throw shopHint();
  },
  "ShopService.CreateInvoice": () => {
    throw shopHint();
  },

  // BankingService: the demo account is read-only; a sync adds one booking.
  "AccountService.Prices": () => [
    { plan: "support", interval: "monthly", currency: "EUR", value: "4.90" },
    { plan: "support", interval: "yearly", currency: "EUR", value: "49.00" },
    { plan: "plus", interval: "monthly", currency: "EUR", value: "9.40" },
    { plan: "plus", interval: "yearly", currency: "EUR", value: "94.00" },
  ],
  "BankingService.Overview": () => ({
    connections: [db.connection],
    accounts: [db.account],
    productSource: "",
    csvLabels: {},
  }),
  "BankingService.Bookings": (id: string) => db.bookings.filter((b) => !id || b.accountId === id),
  "BankingService.RemoveAccount": () => {
    throw desktopOnly("das Entfernen eines Bankkontos");
  },
  "BankingService.SetAccountLabel": (id: string, label: string) => {
    if (db.account.id === id) db.account.label = label.trim();
  },
  "BankingService.Connect": () => {
    throw desktopOnly("die Verbindung zu deiner Bank über FinTS");
  },
  "BankingService.Sync": (accountID: string) => {
    const op = {
      id: newID("op"),
      connectionId: db.connection.id,
      status: "done",
      message: "",
      methods: [],
      media: [],
      added: 0,
    };
    if (!db.synced) {
      db.synced = true;
      const target = db.invoices.find((d) => d.id === "inv-4");
      const amount = target ? cents(calculate(target).totals.gross) : 10_000;
      db.bookings.push({
        id: "bk-sync",
        accountId: accountID || db.account.id,
        data: {
          booking_date: day(),
          value_date: day(),
          amount,
          currency: "EUR",
          counterparty_name: "Lea Brandt",
          counterparty_iban: "DE75512108001245126199",
          description: `Cafe Morgenrot ${target?.number ?? ""}`,
        },
      });
      db.account.balance!.booked = {
        amount: db.account.balance!.booked.amount + amount,
        currency: "EUR",
        date: day(),
      };
      op.added = 1;
    }
    db.account.lastSync = now();
    op.message = op.added
      ? "Live-Demo: 1 neuer Umsatz simuliert. In der Desktop-App ruft die App deine Umsätze per FinTS bei der Bank ab."
      : "Live-Demo: keine neuen Umsätze.";
    return op;
  },
  "BankingService.Status": (id: string) => ({
    id,
    connectionId: db.connection.id,
    status: "done",
    message: "",
    methods: [],
    media: [],
    added: 0,
  }),
  "BankingService.SubmitTAN": () => undefined,
  "BankingService.Cancel": () => undefined,
  "BankingService.ForgetPIN": () => undefined,
  "BankingService.SetProductID": () => {
    throw desktopOnly("die eigene FinTS-Produktregistrierung");
  },

  // AccountService and BackupService (papierleicht Plus)
  "AccountService.Status": () => accountStatus(),
  "AccountService.Refresh": () => accountStatus(),
  "AccountService.Devices": () => [],
  "AccountService.RequestCode": () => {
    throw desktopOnly("die Anmeldung bei papierleicht Plus");
  },
  "AccountService.SignIn": () => {
    throw desktopOnly("die Anmeldung bei papierleicht Plus");
  },
  "AccountService.CancelCode": () => accountStatus(),
  "AccountService.SignOut": () => accountStatus(),
  "AccountService.SignOutEverywhere": () => accountStatus(),
  "AccountService.RemoveDevice": () => accountStatus(),
  // The demo sends nothing; the sign-up only works in the desktop app.
  "AccountService.Newsletter": () => ({ email: "", requestedAt: "" }),
  "AccountService.SubscribeNewsletter": () => {
    throw desktopOnly("die Anmeldung zum Newsletter");
  },
  // ChatService: the demo shows the showcase of "Chat & Markt"; the
  // community itself runs on the server for members.
  "ChatService.Preview": () => ({
    members: 0,
    messagesWeek: 0,
    online: 0,
    offers: 0,
    requests: 0,
    categories: [],
  }),
  "ChatService.State": () => ({ access: "signed-out", guidelines: "" }),
  "ChatService.Join": () => {
    throw desktopOnly("die Community");
  },
  "ChatService.Messages": () => {
    throw desktopOnly("der Community-Chat");
  },
  "ChatService.Unread": () => {
    throw desktopOnly("der Community-Chat");
  },
  "ChatService.Send": () => {
    throw desktopOnly("der Community-Chat");
  },
  "ChatService.Delete": () => {
    throw desktopOnly("der Community-Chat");
  },
  "ChatService.Report": () => {
    throw desktopOnly("der Community-Chat");
  },
  "ChatService.Listings": () => ({ items: [], total: 0, page: 1, pageSize: 24 }),
  "ChatService.CreateListing": () => {
    throw desktopOnly("Angebote & Gesuche");
  },
  "ChatService.RenewListing": () => {
    throw desktopOnly("Angebote & Gesuche");
  },
  "ChatService.DeleteListing": () => {
    throw desktopOnly("Angebote & Gesuche");
  },
  "ChatService.ContactListing": () => {
    throw desktopOnly("Angebote & Gesuche");
  },
  "ChatService.ReportListing": () => {
    throw desktopOnly("Angebote & Gesuche");
  },
  "BackupService.BackupStatus": () => backupStatus(),
  "BackupService.BackupAccount": () => ({
    access: "none",
    readableUntil: "",
    deletionAt: "",
    usedBytes: 0,
    quotaBytes: 0,
    versions: 0,
  }),
  "BackupService.BackupLog": () => [],
  "BackupService.BackupVersions": () => [],
  "BackupService.ExportArchive": () => {
    throw desktopOnly("die lokale Datensicherung");
  },
  "BackupService.InspectArchive": () => {
    throw desktopOnly("das Wiederherstellen einer Datensicherung");
  },
  "BackupService.ImportArchive": () => {
    throw desktopOnly("das Wiederherstellen einer Datensicherung");
  },
  "BackupService.BackupNow": () => {
    throw desktopOnly("das verschlüsselte Online-Backup");
  },
  "BackupService.PostponeKeyCheck": () => backupStatus(),
  "BackupService.CheckBackupKey": () => backupStatus(),
  "BackupService.CreateBackupKey": () => {
    throw desktopOnly("das verschlüsselte Online-Backup");
  },
  "BackupService.ImportBackupKey": () => {
    throw desktopOnly("das verschlüsselte Online-Backup");
  },
  "BackupService.SetAutoBackup": () => {
    throw desktopOnly("das verschlüsselte Online-Backup");
  },

  // UpdateService: the demo always shows the published version.
  "UpdateService.Status": () => ({ currentVersion: appVersion.trim(), updateAvailable: false }),
  "UpdateService.Check": () => ({ currentVersion: appVersion.trim(), updateAvailable: false }),
  "UpdateService.Open": () => {
    window.open("https://papierleicht.de/#download", "_blank", "noopener,noreferrer");
  },
};

function accountStatus() {
  return {
    configured: true,
    state: "signed-out",
    email: "",
    plan: "",
    features: [],
    expiresAt: "",
    refreshAfter: "",
    notice: "",
    noticeKind: "",
    codePending: false,
    pendingEmail: "",
    manageUrl: "",
    deviceName: "Live-Demo",
  };
}

function backupStatus() {
  return {
    available: false,
    keyConfigured: false,
    keyPending: false,
    auto: false,
    running: false,
    pending: false,
    lastBackupAt: "",
    lastError: "",
    lastErrorAt: "",
    lastVerifiedAt: "",
    keyCheckDue: false,
  };
}

function auditEntries() {
  return db.invoices
    .filter((d) => d.status !== "draft")
    .map((d, index) => ({
      seq: index + 1,
      at: `${d.issueDate}T09:00:00Z`,
      action: "issued",
      object: "invoice",
      objectId: d.id,
      label: `Rechnung ${d.number}`,
      version: "1.0.0",
      previous: "",
      hash: "",
    }));
}

export async function call(method: string, args: unknown[]): Promise<unknown> {
  const handler = handlers[method];
  if (!handler) throw desktopOnly("dieser Schritt");
  const result = await handler(...args);
  if (method === "AccountService.SignOut" || method === "AccountService.SignOutEverywhere") {
    emit("account:status", result);
  }
  return result;
}
