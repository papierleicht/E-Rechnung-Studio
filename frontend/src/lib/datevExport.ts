import type { RevenueAccounts } from "../../bindings/papierleicht.de/e-rechnung-studio/internal/datev/models";

/** Revenue account fields of the DATEV settings, in the order of the tax cases. */
export const revenueFields: { key: keyof RevenueAccounts; label: string }[] = [
  { key: "standard19", label: "Erlöse 19 % USt" },
  { key: "reduced7", label: "Erlöse 7 % USt" },
  { key: "zeroRated", label: "Erlöse 0 % (Nullsteuersatz)" },
  { key: "exempt", label: "Steuerfreie Umsätze (§ 4 Nr. 8 ff.)" },
  { key: "export", label: "Ausfuhrlieferungen" },
  { key: "intraCommunity", label: "Innergemeinschaftliche Lieferungen" },
  { key: "reverseCharge", label: "§ 13b, Empfänger im Inland" },
  { key: "reverseChargeEU", label: "§ 13b, Empfänger im übrigen EU-Gebiet" },
  { key: "outOfScope", label: "Nicht steuerbare Umsätze" },
  { key: "smallBusiness", label: "Kleinunternehmer (§ 19)" },
  { key: "margin", label: "Differenzbesteuerung (§ 25a)" },
];

/** Checklist severities as reported by the backend. */
export const severityLabels: Record<string, string> = {
  blocking: "Erforderlich:",
  excluded: "Nicht enthalten:",
  review: "Kanzlei prüfen:",
};
