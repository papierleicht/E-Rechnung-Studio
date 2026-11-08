export const recurringIntervals = [
  ["monthly", "monatlich"],
  ["quarterly", "vierteljährlich"],
  ["half-yearly", "halbjährlich"],
  ["yearly", "jährlich"],
] as const;

export function recurringIntervalLabel(interval: string): string {
  return recurringIntervals.find(([id]) => id === interval)?.[1] ?? interval;
}

export function recurringStatusLabel(status: string): string {
  if (status === "paused") return "Pausiert";
  if (status === "ended") return "Beendet";
  return "Aktiv";
}

export function recurringBadgeClass(status: string): string {
  return status === "active" ? "paid" : "draft";
}

/** The waiting drafts of one recurring invoice, "" when none waits. */
export function recurringDraftsLabel(count: number): string {
  if (!count) return "";
  return count === 1 ? "1 Entwurf wartet" : `${count} Entwürfe warten`;
}

export function recurringRunMessage(created: number): string {
  return created === 1
    ? "Neuer Entwurf aus einer Dauerrechnung. Prüfe ihn und stelle ihn aus."
    : `${created} neue Entwürfe aus Dauerrechnungen. Prüfe sie und stelle sie aus.`;
}

/** Billing before the period starts is the case where the supply is still open. */
export function billsInAdvance(nextDate: string, periodStart: string): boolean {
  return Boolean(nextDate && periodStart) && periodStart >= nextDate;
}
