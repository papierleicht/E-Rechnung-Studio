/** Cent arithmetic for the demo. The desktop app uses exact decimals in Go. */
export function parseAmount(value: string | number | undefined | null): number {
  if (typeof value === "number") return value;
  const n = Number(
    String(value ?? "")
      .trim()
      .replace(",", "."),
  );
  return Number.isFinite(n) ? n : NaN;
}

export function cents(value: string | number): number {
  return Math.round(parseAmount(value) * 100);
}

export function fixed(centsValue: number): string {
  return (centsValue / 100).toFixed(2);
}

/** "1234.5" → "1.234,50" as on the printed invoice. */
export function money(centsValue: number): string {
  return (centsValue / 100).toLocaleString("de-DE", {
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  });
}

export function plainNumber(value: string | number): string {
  return String(parseAmount(value)).replace(".", ",");
}

/** Local calendar date with an offset in days, as YYYY-MM-DD. */
export function day(offset = 0, from = new Date()): string {
  const date = new Date(from.getFullYear(), from.getMonth(), from.getDate() + offset, 12);
  return date.toLocaleDateString("sv-SE");
}

export function addDays(iso: string, days: number): string {
  const [y, m, d] = iso.split("-").map(Number);
  return new Date(y, m - 1, d + days, 12).toLocaleDateString("sv-SE");
}

export function daysBetween(from: string, to: string): number {
  return Math.round((Date.parse(to) - Date.parse(from)) / 86_400_000);
}

export function germanDate(iso: string): string {
  const [y, m, d] = (iso || "").slice(0, 10).split("-");
  return y && m && d ? `${d}.${m}.${y}` : "";
}

/** ISO timestamp for a local date (noon avoids time-zone date shifts). */
export function stamp(iso: string, hour = 10): string {
  const [y, m, d] = iso.split("-").map(Number);
  return new Date(y, m - 1, d, hour, 12).toISOString();
}

export function displayIBAN(iban: string): string {
  return iban
    .replace(/\s+/g, "")
    .replace(/(.{4})/g, "$1 ")
    .trim();
}

let counter = 0;
export function newID(prefix: string): string {
  counter++;
  const random =
    globalThis.crypto?.randomUUID?.().slice(0, 8) ?? Math.random().toString(16).slice(2, 10);
  return `${prefix}-${random}${counter}`;
}
