// Every Lucide icon has this component type; the package root would load all icon types.
import type LucideIcon from "@lucide/svelte/icons/circle";
import FilePlusIcon from "@lucide/svelte/icons/file-plus";
import FilePenLineIcon from "@lucide/svelte/icons/file-pen-line";
import FileUpIcon from "@lucide/svelte/icons/file-up";
import CalculatorIcon from "@lucide/svelte/icons/calculator";
import ArrowDownLeftIcon from "@lucide/svelte/icons/arrow-down-left";
import ArrowUpRightIcon from "@lucide/svelte/icons/arrow-up-right";
import type { PaymentBalance } from "../../bindings/papierleicht.de/e-rechnung-studio/internal/store/models";
import type {
  DashboardBucket,
  TaxDeadline,
} from "../../bindings/papierleicht.de/e-rechnung-studio/internal/app/models";
import { amountLabel, dateLabel } from "./format";
import { collectable } from "./paymentWorkflow";

export type ActionTile = {
  id: string;
  label: string;
  hint?: string;
  icon: typeof LucideIcon;
  onclick: () => void;
  /** The section's main task: the large tile on the left. Exactly one per group. */
  primary?: boolean;
  /** Shows a state (count, amount); with `alert` it is marked as needing attention. */
  status?: boolean;
  alert?: boolean;
  /** Tiles that filter the list below toggle like a button. */
  pressed?: boolean;
  /** Tiles that switch the view below mark the view currently shown. */
  current?: boolean;
  disabled?: boolean;
  title?: string;
  /** Extra attributes, e.g. to make a tile a native file drop target. */
  attrs?: Record<string, string | undefined>;
};

// Tasks that appear in several sections are defined once here, so a tile
// reads, looks and counts the same wherever it appears. Every hint fits one
// line of a small tile at the 900px minimum window; details go to the tooltip.

export const newInvoiceTile = (onclick: () => void): ActionTile => ({
  id: "invoice",
  icon: FilePlusIcon,
  label: "Neue Rechnung",
  hint: "Als E-Rechnung an Kunden",
  onclick,
});

export const newQuoteTile = (onclick: () => void): ActionTile => ({
  id: "quote",
  icon: FilePenLineIcon,
  label: "Angebot erstellen",
  hint: "Später zur Rechnung",
  onclick,
});

export const addReceiptTile = (onclick: () => void): ActionTile => ({
  id: "receipt",
  icon: FileUpIcon,
  label: "Beleg hinzufügen",
  hint: "E-Rechnung oder PDF",
  title: "E-Rechnung oder PDF ablegen oder auswählen (bis 20 MB)",
  onclick,
});

/** Overdue deadlines first, then the earliest one. */
export const nextDeadline = (deadlines: TaxDeadline[]) =>
  [...deadlines].sort(
    (a, b) => Number(b.overdue) - Number(a.overdue) || a.due.localeCompare(b.due),
  )[0];

export const taxesTile = (deadline: TaxDeadline | undefined, onclick: () => void): ActionTile => ({
  id: "taxes",
  status: true,
  icon: CalculatorIcon,
  alert: deadline?.overdue,
  label: "Steuern erledigen",
  hint: deadline
    ? `${deadline.overdue ? "Überfällig seit" : "Fällig am"} ${dateLabel(deadline.due)}`
    : "Werte für ELSTER ablesen",
  title: deadline
    ? `${deadline.label}: ${deadline.overdue ? "überfällig seit" : "fällig am"} ${dateLabel(deadline.due)}`
    : "Werte für Umsatzsteuer und EÜR Schritt für Schritt ablesen",
  onclick,
});

export type OpenTotals = {
  count: number;
  amount: number;
  overdueCount: number;
  overdueAmount: number;
  /** Counted, but not added up: amounts in other currencies. */
  foreign: number;
};

/** A balance counts as open while something remains to be paid (as on the server). */
export const isOpen = (balance: PaymentBalance) => collectable(balance) > 0;

/** Open totals by the dashboard's rule: overdue once the due date has passed. */
export function openTotals(
  balances: PaymentBalance[],
  today = new Date().toLocaleDateString("sv-SE"),
): OpenTotals {
  const totals = { count: 0, amount: 0, overdueCount: 0, overdueAmount: 0, foreign: 0 };
  for (const balance of balances.filter(isOpen)) {
    const missed = Number(balance.overdueAmount ?? 0);
    const overdue = balance.installments
      ? missed > 0
      : balance.dueDate !== "" && balance.dueDate < today;
    totals.count++;
    if (overdue) totals.overdueCount++;
    if (balance.currency && balance.currency !== "EUR") {
      totals.foreign++;
      continue;
    }
    totals.amount += collectable(balance);
    if (overdue)
      totals.overdueAmount += balance.installments
        ? Math.min(missed, collectable(balance))
        : collectable(balance);
  }
  return totals;
}

/** The same totals from the server's dashboard summary. */
export const summaryTotals = (total: DashboardBucket, overdue: DashboardBucket): OpenTotals => ({
  count: total.count,
  amount: Number(total.amount),
  overdueCount: overdue.count,
  overdueAmount: Number(overdue.amount),
  foreign: total.foreign,
});

/** Money customers still owe ("in") or that is still to be paid ("out"). */
export function openTile(
  direction: "in" | "out",
  totals: OpenTotals | null,
  onclick: () => void,
): ActionTile {
  const [one, many] =
    direction === "in"
      ? ["Kundenrechnung", "Kundenrechnungen"]
      : ["Eingangsrechnung", "Eingangsrechnungen"];
  const tile = {
    id: `open-${direction}`,
    status: true,
    icon: direction === "in" ? ArrowDownLeftIcon : ArrowUpRightIcon,
    onclick,
  };
  if (!totals)
    return { ...tile, label: direction === "in" ? "Offen" : "Zu zahlen", hint: "Wird geladen …" };
  if (!totals.count) return { ...tile, label: "Alles bezahlt", hint: "Nichts offen" };
  const count = `${totals.count} ${totals.count === 1 ? one : many}`;
  const foreign = totals.foreign ? `, ${totals.foreign} in Fremdwährung` : "";
  return {
    ...tile,
    alert: totals.overdueCount > 0,
    label: `${amountLabel(totals.amount)} ${direction === "in" ? "offen" : "zu zahlen"}`,
    hint: totals.overdueCount
      ? `${amountLabel(totals.overdueAmount)} überfällig`
      : "Nichts überfällig",
    title: `${count} offen${totals.overdueCount ? `, davon ${totals.overdueCount} überfällig` : ""}${foreign}`,
  };
}
