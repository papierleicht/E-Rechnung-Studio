import type {
  Reminder,
  ReminderProposal,
} from "../../bindings/papierleicht.de/e-rechnung-studio/internal/store/models";
import { dateLabel } from "./format.ts";
import { paymentMoney } from "./paymentWorkflow.ts";

export const reminderPlaceholders = [
  ["{kunde}", "Kundenname"],
  ["{rechnungsnummer}", "Rechnungsnummer"],
  ["{rechnungsdatum}", "Rechnungsdatum"],
  ["{faelligkeit}", "fällig seit"],
  ["{betrag}", "zu zahlender Betrag inkl. Gebühr/Zinsen"],
  ["{offen}", "offener Betrag ohne Gebühr/Zinsen"],
  ["{frist}", "neue Zahlungsfrist"],
  ["{firma}", "dein Firmenname"],
] as const;

export function reminderChannelLabel(channel: string): string {
  if (channel === "manual-export") return "manuell exportiert";
  if (channel === "email") return "per E-Mail versendet";
  return channel;
}

/** One line for a recorded reminder in the invoice history. */
export function reminderHistoryLine(reminder: Reminder): string {
  return `${reminder.title} · ${dateLabel(reminder.date)} · ${paymentMoney(reminder.total, reminder.currency)} · ${reminderChannelLabel(reminder.channel)}`;
}

/** Explains what is open and since when, distinguishing installment plans. */
export function reminderBasisLine(p: ReminderProposal): string {
  const what = p.installment ? "Fällige Raten" : "Offen";
  return `${what}: ${paymentMoney(p.amount, p.currency)} · fällig seit ${dateLabel(p.dueSince)} (${p.daysOverdue} ${p.daysOverdue === 1 ? "Tag" : "Tage"})`;
}

/** When the proposed level is due; empty once all levels are used. */
export function reminderTiming(p: ReminderProposal): string {
  if (p.complete)
    return "Alle Erinnerungsstufen sind erstellt. Alles Weitere musst du selbst prüfen und veranlassen.";
  if (p.due) return `${p.title} empfohlen`;
  return p.nextDate ? `${p.title} empfohlen ab ${dateLabel(p.nextDate)}` : "";
}

export function hasHintAmounts(p: Pick<ReminderProposal, "fee" | "interest">): boolean {
  return Number(p.fee) > 0 || Number(p.interest) > 0;
}
