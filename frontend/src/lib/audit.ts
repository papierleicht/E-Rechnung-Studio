/**
 * UI helpers for the change log and the tools for a tax audit. Go keeps the log
 * and decides what is allowed; this file only words it.
 */
import { writable } from "svelte/store";

/** The read-only view for an auditor, shown as a banner over every page. */
export const auditView = writable({ open: false, from: "", to: "" });

const actionLabels: Record<string, string> = {
  created: "Erfasst",
  changed: "Geändert",
  cancelled: "Storniert",
  deleted: "Gelöscht",
  issued: "Ausgestellt",
  sent: "Als verschickt markiert",
  unsent: "Markierung zurückgenommen",
  recorded: "Zahlung erfasst",
  revoked: "Zahlung aufgehoben",
  filed: "Als übermittelt vermerkt",
  unfiled: "Vermerk zurückgenommen",
  locked: "Festgeschrieben",
  restored: "Wiederhergestellt",
  baseline: "Übernommen",
  version: "Programmversion",
  exported: "Exportiert",
};

export function auditActionLabel(action: string): string {
  return actionLabels[action] ?? action;
}

/** The last three calendar years up to yesterday: a usual audit period. */
export function defaultAuditPeriod(today: Date): { from: string; to: string } {
  const year = today.getFullYear();
  return { from: `${year - 3}-01-01`, to: `${year - 1}-12-31` };
}

/** Rows that contain the search text in any column. */
export function filterRows(rows: string[][], query: string): string[][] {
  const needle = query.trim().toLocaleLowerCase("de");
  if (!needle) return rows;
  return rows.filter((row) => row.join(" ").toLocaleLowerCase("de").includes(needle));
}

function numericValue(value: string): number {
  return Number(value.replace(",", "."));
}

function germanDate(value: string): string | null {
  const m = /^(\d{2})\.(\d{2})\.(\d{4})$/.exec(value);
  return m ? `${m[3]}-${m[2]}-${m[1]}` : null;
}

/** Sorts by a column: numbers by value, dates by day, text by the alphabet. */
export function sortRows(
  rows: string[][],
  column: number,
  numeric: boolean,
  ascending: boolean,
): string[][] {
  const sign = ascending ? 1 : -1;
  return [...rows].sort((a, b) => {
    const left = a[column] ?? "";
    const right = b[column] ?? "";
    if (numeric) return sign * (numericValue(left) - numericValue(right));
    const [dateLeft, dateRight] = [germanDate(left), germanDate(right)];
    if (dateLeft !== null && dateRight !== null) return sign * dateLeft.localeCompare(dateRight);
    return sign * left.localeCompare(right, "de");
  });
}
