import type { BankBooking } from "./banking-api";

export function incomingTotals(bookings: BankBooking[], fallbackCurrency: string) {
  const totals = new Map<string, number>();
  for (const booking of bookings) {
    if (booking.data.amount <= 0) continue;
    const currency = booking.data.currency || fallbackCurrency;
    totals.set(currency, (totals.get(currency) ?? 0) + booking.data.amount);
  }
  if (!totals.size) totals.set(fallbackCurrency, 0);
  return [...totals].sort(([left], [right]) => left.localeCompare(right));
}
