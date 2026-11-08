import type { PaymentBalance } from "../../bindings/papierleicht.de/e-rechnung-studio/internal/store/models";
import { dateLabel } from "./format.ts";

export function paymentMoney(amount: string | number, currency = "EUR"): string {
  return new Intl.NumberFormat("de-DE", { style: "currency", currency }).format(Number(amount));
}
/** Banks export UTF-8 or Windows-1252; only valid UTF-8 is read as such. */
export function decodeCSV(data: ArrayBuffer): string {
  try {
    return new TextDecoder("utf-8", { fatal: true }).decode(data);
  } catch {
    return new TextDecoder("windows-1252").decode(data);
  }
}
/** What the customer still owes: refunds waiting for a credit note are not asked for again. */
export function collectable(balance: PaymentBalance): number {
  const cents = Number(balance.remaining) * 100 - Number(balance.awaitingCredit ?? 0) * 100;
  return Math.round(cents) / 100;
}
export function paymentTitle(balance: PaymentBalance): string {
  if (balance.planNeedsReview) return "Ratenplan prüfen";
  if (balance.state === "bezahlt") return "Bezahlt";
  if (balance.state === "verrechnet") return "Verrechnet";
  if (balance.state === "gutschrift-offen") return "Gutschrift fehlt";
  if (balance.credit) return "Auszuzahlen";
  if (balance.state === "überfällig")
    return balance.installments ? "Rate überfällig" : "Überfällig";
  if (balance.installments) return "Ratenzahlung";
  return balance.state === "teilbezahlt" ? "Teilweise bezahlt" : "Offen";
}
export function paymentDescription(balance: PaymentBalance): string {
  if (balance.planNeedsReview)
    return "Eine frühere Zahlung hat sich geändert. Aktualisiere die Vereinbarung.";
  if (balance.state === "bezahlt")
    return Number(balance.overpaid) > 0
      ? `Überzahlung: ${paymentMoney(balance.overpaid, balance.currency)}`
      : "";
  if (balance.state === "verrechnet")
    return `${paymentMoney(balance.credited ?? 0, balance.currency)} mit Gutschrift ausgeglichen`;
  if (balance.state === "gutschrift-offen")
    return `${paymentMoney(balance.awaitingCredit ?? 0, balance.currency)} erstattet`;
  if (balance.credit) return `${paymentMoney(balance.remaining, balance.currency)} an den Kunden`;
  if (balance.installments && balance.nextDate)
    return `${paymentMoney(balance.nextAmount, balance.currency)} ${balance.state === "überfällig" ? "fällig seit" : "am"} ${dateLabel(balance.nextDate)}`;
  return `${paymentMoney(collectable(balance), balance.currency)} offen`;
}
