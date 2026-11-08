import type { DashboardSummary } from "../../bindings/papierleicht.de/e-rechnung-studio/internal/app/models";

export type DashboardTarget = "invoices" | "received" | "banking" | "finance" | "taxes";
export type DashboardTask = {
  id: string;
  title: string;
  detail: string;
  target: DashboardTarget;
  urgent: boolean;
};

const count = (n: number, one: string, many: string) => `${n} ${n === 1 ? one : many}`;

/** Open work across all areas, most urgent first. Empty when nothing is left. */
export function dashboardTasks(summary: DashboardSummary, drafts: number): DashboardTask[] {
  const tasks: DashboardTask[] = [];
  const add = (n: number, task: DashboardTask) => n > 0 && tasks.push(task);
  add(summary.receivableOverdue.count, {
    id: "overdue",
    title: `${count(summary.receivableOverdue.count, "Kundenrechnung", "Kundenrechnungen")} überfällig`,
    detail: "Prüfe den Zahlungseingang oder erinnere deinen Kunden",
    target: "invoices",
    urgent: true,
  });
  add(summary.payableOverdue.count, {
    id: "payable-overdue",
    title: `${count(summary.payableOverdue.count, "Eingangsrechnung", "Eingangsrechnungen")} überfällig`,
    detail: "Überweise den Betrag oder markiere die Rechnung als bezahlt",
    target: "received",
    urgent: true,
  });
  add(summary.taxProfileMissing ? 1 : 0, {
    id: "tax-profile",
    title: "Steuerprofil festlegen",
    detail: "Erst damit zeigt die App deine Voranmeldungen und Fristen",
    target: "taxes",
    urgent: false,
  });
  add(summary.refundsToCredit, {
    id: "refunds",
    title: `${count(summary.refundsToCredit, "Erstattung", "Erstattungen")} ohne Gutschrift`,
    detail: "Bis zur Korrekturgutschrift bleibt die Umsatzsteuer der Rechnung unverändert",
    target: "invoices",
    urgent: false,
  });
  add(summary.plansToReview, {
    id: "plans",
    title: `${count(summary.plansToReview, "Ratenplan", "Ratenpläne")} prüfen`,
    detail: "Eine frühere Zahlung hat sich geändert",
    target: "invoices",
    urgent: true,
  });
  add(summary.unassignedBookings, {
    id: "bookings",
    title: `${count(summary.unassignedBookings, "Bankumsatz", "Bankumsätze")} ohne Zuordnung`,
    detail: "Einer Rechnung zuordnen oder als sonstige Geldbewegung einordnen",
    target: "banking",
    urgent: false,
  });
  add(summary.receivedToReview, {
    id: "received",
    title: `${count(summary.receivedToReview, "Beleg", "Belege")} prüfen`,
    detail:
      "Das Rechnungsdatum fehlt oder die Prüfung hat etwas gefunden, das die Vorsteuer betreffen kann",
    target: "received",
    urgent: false,
  });
  add(drafts, {
    id: "drafts",
    title: `${count(drafts, "Entwurf", "Entwürfe")} noch nicht ausgestellt`,
    detail: "Fertigstellen oder verwerfen",
    target: "invoices",
    urgent: false,
  });
  const issues = summary.financeIssues ?? [];
  add(issues.length, {
    id: "finance",
    title: `${summary.financeLabel}: ${count(issues.length, "offener Punkt", "offene Punkte")} in den Finanzen`,
    detail: issues[0]?.message ?? "",
    target: "finance",
    urgent: false,
  });
  return tasks;
}
