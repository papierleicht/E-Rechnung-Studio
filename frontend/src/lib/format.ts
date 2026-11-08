/** German display formatting. These helpers never calculate invoice totals. */
const euroFormatter = new Intl.NumberFormat("de-DE", { style: "currency", currency: "EUR" });

export function amountLabel(value: string | number): string {
  if (value === "") return "—";
  const amount = typeof value === "string" ? Number(value.replace(",", ".")) : value;
  return Number.isFinite(amount) ? euroFormatter.format(amount) : "—";
}

/** Today in the user's time zone as YYYY-MM-DD; toISOString would give the UTC date. */
export function localToday(now = new Date()): string {
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${now.getFullYear()}-${pad(now.getMonth() + 1)}-${pad(now.getDate())}`;
}

export function dateLabel(value: string): string {
  const [year, month, day] = value.split("-");
  return year && month && day ? `${day}.${month}.${year}` : value || "—";
}

export function statusLabel(status: string): string {
  const labels: Record<string, string> = { draft: "Entwurf", issued: "Offen", paid: "Bezahlt" };
  return labels[status] ?? status;
}

/** A typed German amount as the plain decimal the services expect: 1.234,50 → 1234.50. */
export function plainDecimal(value: string): string {
  const text = value.trim();
  return text.includes(",") ? text.replaceAll(".", "").replace(",", ".") : text;
}
/** A stored decimal for an input field: 1234.50 → 1234,50. */
export const decimalInput = (value: string) => value.replace(".", ",");
