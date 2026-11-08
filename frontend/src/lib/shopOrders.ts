import { shopProviderByID } from "./shopProviders.ts";

/** Presentation helpers only; order and invoice totals are calculated by the backend. */
export function shopProviderLabel(provider: string): string {
  return shopProviderByID(provider)?.label ?? provider;
}

export function shopOrderState(order: {
  invoiceId: string;
  reviewReason: string;
  conversionError?: string;
}): "invoice" | "review" | "ready" {
  return order.invoiceId ? "invoice" : shopOrderReviewReason(order) ? "review" : "ready";
}

export function shopOrderReviewReason(order: {
  reviewReason: string;
  conversionError?: string;
}): string {
  return order.reviewReason || order.conversionError || "";
}

export function shopMoney(gross: string, currency: string): string {
  if (!/^-?\d+(?:\.\d+)?$/.test(gross)) return "—";
  const amount = Number(gross);
  if (!Number.isFinite(amount)) return "—";
  try {
    return new Intl.NumberFormat("de-DE", { style: "currency", currency }).format(amount);
  } catch {
    return `${gross} ${currency}`;
  }
}

export function shopDate(value: string, withTime = false, sourceCalendar = false): string {
  if (!value) return "Noch nicht synchronisiert";
  const date = new Date(value);
  if (!Number.isFinite(date.getTime())) return "—";
  if (sourceCalendar && !withTime) {
    const day = /^(\d{4})-(\d{2})-(\d{2})T/.exec(value);
    if (day) return `${day[3]}.${day[2]}.${day[1]}`;
  }
  return withTime ? date.toLocaleString("de-DE") : date.toLocaleDateString("de-DE");
}

export function shopPaymentLabel(order: { paid: boolean; reviewReason: string }): string {
  return order.paid ? "Bereits bezahlt" : order.reviewReason ? "Zahlung prüfen" : "Zahlung offen";
}

export function shopImportDate(now = new Date()): string {
  const date = new Date(now);
  date.setDate(date.getDate() - 30);
  return `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, "0")}-${String(date.getDate()).padStart(2, "0")}`;
}
