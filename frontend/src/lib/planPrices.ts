/** A bookable plan as the license server charges it. */
export type PlanPrice = { plan: string; interval: string; currency: string; value: string };

/** "4,90 €" or "49 €" for a plan and interval; "" when the server names none. */
export function priceOf(prices: readonly PlanPrice[], plan: string, interval: string): string {
  const price = prices.find((item) => item.plan === plan && item.interval === interval);
  const value = Number(price?.value);
  if (!price || !Number.isFinite(value) || value <= 0) return "";
  return new Intl.NumberFormat("de-DE", {
    style: "currency",
    currency: price.currency || "EUR",
    minimumFractionDigits: Number.isInteger(value) ? 0 : 2,
  }).format(value);
}
