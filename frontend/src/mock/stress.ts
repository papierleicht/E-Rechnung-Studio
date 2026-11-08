/**
 * Stress data for layout checks: open the demo with `?stress` and every text
 * a customer, supplier, bank or shop can send gets long, including words and
 * links without spaces. IDs, numbers, amounts, dates and IBANs stay as they
 * are, so all calculations still work. Use it to check that nothing overflows
 * at the 900px minimum window (defensive CSS).
 */

const longWord = "Donaudampfschifffahrtsgesellschaftskapitänsmütze";
const longLink =
  "https://www.beispiel-unternehmen.example/rechnungen/2026/november/ohne-leerzeichen";

const extend: Record<string, (value: string) => string> = {
  name: (v) => `${v} – Zweigniederlassung Nordrhein-Westfalen und Rheinland-Pfalz ${longWord}`,
  contactName: (v) => (v ? `${v} von und zu Hohenzollern-Sigmaringen-Wittelsbach` : v),
  sellerName: (v) => `${v} Internationale Handelsgesellschaft mit beschränkter Haftung & Co. KG`,
  counterparty_name: (v) => `${v} Treuhand- und Vermögensverwaltungsgesellschaft mbH`,
  description: (v) =>
    v ? `${v} · EREF+20261102123456789012345678901234 MREF+KD-0000012345678 ${longLink}` : v,
  city: (v) => (v ? `${v}-Südvorstadt-Connewitz-Lößnig-Dölitz-Dösen` : v),
  street: (v) => (v ? `Großer Straßenname mit Hinterhausaufgang links ${v}` : v),
  email: (v) => (v ? `sehr.lange.adresse.fuer.die.buchhaltung.${v}` : v),
  filename: (v) => `eingangsrechnung-${longWord.toLowerCase()}-ohne-leerzeichen-${v}`,
  title: (v) => (v ? `${v} – ${longWord}` : v),
  note: (v) => (v ? `${v} ${longLink}` : v),
};

/** True when the demo runs with `?stress`. */
export function stressMode(): boolean {
  try {
    return new URLSearchParams(globalThis.location?.search ?? "").has("stress");
  } catch {
    return false;
  }
}

/** Lengthens the known text fields of the demo data in place. */
export function applyStress(value: unknown, seen = new Set<unknown>()): void {
  if (!value || typeof value !== "object" || seen.has(value)) return;
  seen.add(value);
  if (Array.isArray(value)) {
    for (const item of value) applyStress(item, seen);
    return;
  }
  const record = value as Record<string, unknown>;
  for (const [key, item] of Object.entries(record)) {
    if (typeof item === "string" && extend[key]) record[key] = extend[key](item);
    else applyStress(item, seen);
  }
}
