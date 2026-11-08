/** UI helpers for receipts and tax forms. Go stores the authoritative values. */

export const taxRates = [
  { id: "19", label: "19 % Umsatzsteuer" },
  { id: "7", label: "7 % Umsatzsteuer" },
  { id: "mixed", label: "19 % und 7 % gemischt" },
  { id: "0", label: "0 % (z. B. Solaranlage)" },
  { id: "none", label: "Keine Umsatzsteuer ausgewiesen" },
  {
    id: "reverse-charge-eu",
    label: "Steuerschuld bei dir – Anbieter in der EU (z. B. Google, Meta)",
  },
  { id: "reverse-charge", label: "Steuerschuld bei dir – Anbieter außerhalb der EU" },
] as const;

/** Receipts without VAT whose tax the business owes itself (§ 13b UStG). */
export function isReverseCharge(rate: string): boolean {
  return rate === "reverse-charge-eu" || rate === "reverse-charge";
}

/** What a reverse-charge receipt means for the Voranmeldung. */
export function reverseChargeHint(net: number, smallBusiness: boolean): string {
  const tax = (Math.round(net * 19) / 100).toLocaleString("de-DE", {
    style: "currency",
    currency: "EUR",
  });
  return smallBusiness
    ? `Mit dieser Wahl rechnet die App auch als Kleinunternehmer mit 19 % Umsatzsteuer (${tax}). Für diesen Zeitraum erscheint deshalb eine Umsatzsteuer-Voranmeldung unter „Steuern erledigen“.`
    : `Bei Reverse Charge enthält die Rechnung keine Umsatzsteuer. Mit dieser Wahl meldet die App ${tax} in der Voranmeldung an und zieht denselben Betrag als Vorsteuer ab.`;
}

export const paidOptions = [
  { id: "bank", label: "Per Bank oder Karte" },
  { id: "cash", label: "Bar" },
  { id: "private", label: "Privat bezahlt" },
  { id: "open", label: "Noch nicht bezahlt" },
] as const;

export const paidLabels: Record<string, string> = {
  bank: "per Bank",
  cash: "bar",
  private: "privat bezahlt",
};

/**
 * Useful-life suggestions for purchases, in years: computer hardware with
 * peripherals and software per BMF-Schreiben vom 22.02.2022, the others per
 * AfA-Tabelle. There is no default; the user picks one deliberately.
 */
export const usefulLives = [
  { years: 1, label: "Computer, Laptop, Tablet, Drucker, Software – 1 Jahr" },
  { years: 5, label: "Smartphone – 5 Jahre" },
  { years: 6, label: "Auto – 6 Jahre" },
  { years: 7, label: "Kamera, Foto- und Videotechnik – 7 Jahre" },
  { years: 13, label: "Büromöbel – 13 Jahre" },
];

export const GWG_LIMIT = 800;

export function yearsLabel(years: number): string {
  return years === 1 ? "1 Jahr" : `${years} Jahre`;
}

function parseAmount(value: string): number {
  const text = String(value).trim();
  // "1.190,00" uses dots for thousands; "10.70" uses a dot as decimal separator.
  const normalized = text.includes(",") ? text.replace(/\./g, "").replace(",", ".") : text;
  const amount = Number(normalized);
  return text && Number.isFinite(amount) ? amount : NaN;
}

/** Amount as the Go side expects it ("1190.00"), or "" if not a number. */
export function normalizeAmount(value: string): string {
  const amount = parseAmount(value);
  return Number.isFinite(amount) ? amount.toFixed(2) : "";
}

/**
 * Net and tax of a gross amount, rounded to cents like the Go side. For mixed
 * receipts `reduced` is the part taxed at 7 %; the rest is taxed at 19 %.
 */
export function splitGross(
  gross: string,
  rate: string,
  reduced = "",
): { net: number; tax: number } | null {
  const amount = parseAmount(gross);
  if (!(amount > 0)) return null;
  if (rate === "mixed") {
    const part = parseAmount(reduced);
    if (!(part > 0 && part < amount)) return null;
    const high = splitAt(Math.round((amount - part) * 100) / 100, 19);
    const low = splitAt(part, 7);
    return {
      net: Math.round((high.net + low.net) * 100) / 100,
      tax: Math.round((high.tax + low.tax) * 100) / 100,
    };
  }
  if (rate !== "19" && rate !== "7") return { net: amount, tax: 0 };
  return splitAt(amount, Number(rate));
}

function splitAt(amount: number, rate: number) {
  const net = Math.round((amount * 100 * 100) / (100 + rate)) / 100;
  return { net, tax: Math.round((amount - net) * 100) / 100 };
}

/** How a purchase is deducted: at once (GWG) or spread over its useful life. */
export function assetTreatment(net: number, years: number): string {
  if (net <= GWG_LIMIT)
    return "Bis 800 € netto wird der Betrag in der App im Jahr des Kaufs voll abgesetzt.";
  if (!years) return "Wähle, wie lange du es nutzt. Danach richtet sich die Abschreibung.";
  if (years === 1) return "In der App wird der Betrag im Jahr des Kaufs voll abgeschrieben.";
  const perYear = net / years;
  return `Die App schreibt über ${years} Jahre ab, etwa ${perYear.toLocaleString("de-DE", { style: "currency", currency: "EUR" })} pro Jahr (im ersten Jahr anteilig ab dem Kaufmonat).`;
}

export const attachmentTypes: Record<string, string> = {
  pdf: "application/pdf",
  jpg: "image/jpeg",
  jpeg: "image/jpeg",
  png: "image/png",
  heic: "image/heic",
};

export function attachmentType(name: string): string {
  const ext = name.split(".").pop()?.toLowerCase() ?? "";
  return attachmentTypes[ext] ?? "";
}

/** Value as typed into ELSTER: German separators, no currency sign. */
export function elsterValue(amount: string, cents: boolean): string {
  const value = Number(amount);
  if (!Number.isFinite(value)) return amount;
  return value.toLocaleString("de-DE", {
    minimumFractionDigits: cents ? 2 : 0,
    maximumFractionDigits: cents ? 2 : 0,
    useGrouping: false,
  });
}

export const frequencyLabels: Record<string, string> = {
  monthly: "monatlich",
  quarterly: "vierteljährlich",
  yearly: "nur jährlich",
};
