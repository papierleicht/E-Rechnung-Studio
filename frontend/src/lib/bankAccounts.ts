import type { BankAccount, BankConnection } from "./banking-api";

export interface AccountChoice {
  id: string;
  name: string;
  /** IBAN or account number, for the line below the name. */
  detail: string;
}

/** What the user named the account, else its bank and the bank's own product name. */
export function accountName(account: BankAccount, connection?: BankConnection): string {
  return account.label?.trim() || connection?.label || account.data.product_name || "Bankkonto";
}

/** The accounts as the user tells them apart; equal names get the end of the IBAN. */
export function accountChoices(
  accounts: readonly BankAccount[],
  connections: readonly BankConnection[],
): AccountChoice[] {
  const choices = accounts.map((account) => ({
    id: account.id,
    name: accountName(
      account,
      connections.find((item) => item.id === account.connectionId),
    ),
    detail: account.data.iban || account.data.account_number || "",
  }));
  const uses = new Map<string, number>();
  for (const choice of choices) uses.set(choice.name, (uses.get(choice.name) ?? 0) + 1);
  return choices.map((choice) =>
    (uses.get(choice.name) ?? 0) > 1 && choice.detail
      ? { ...choice, name: `${choice.name} (…${choice.detail.replace(/\s+/g, "").slice(-4)})` }
      : choice,
  );
}

export type BookingOrder = "newest" | "oldest";

/** Orders by ISO date; bookings of one day keep their order. */
export function sortByDate<T>(
  rows: readonly T[],
  date: (row: T) => string,
  order: BookingOrder,
): T[] {
  const sign = order === "newest" ? -1 : 1;
  return [...rows].sort((a, b) => {
    const left = date(a),
      right = date(b);
    return left < right ? -sign : left > right ? sign : 0;
  });
}

const ORDER_KEY = "e-rechnung-studio-booking-order";

/** The order chosen last, newest first until then. */
export function savedBookingOrder(): BookingOrder {
  try {
    return localStorage.getItem(ORDER_KEY) === "oldest" ? "oldest" : "newest";
  } catch {
    return "newest";
  }
}

export function saveBookingOrder(order: BookingOrder): void {
  try {
    localStorage.setItem(ORDER_KEY, order);
  } catch {
    // The choice then holds until the page closes.
  }
}
