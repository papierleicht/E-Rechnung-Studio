// The offers of the setup wizard. They follow the plan cards on
// papierleicht.de/konto, where the booking happens; `id` is its ?tarif= value.

export type SetupPlusTier = "plus" | "backup";

export const setupPlusTiers: readonly {
  id: SetupPlusTier;
  name: string;
  level: number;
  badge?: string;
  lead: string;
  items: readonly string[];
}[] = [
  {
    id: "plus",
    name: "Plus",
    level: 2,
    lead: "Spart dir Zeit und lässt dein Protokoll extern bestätigen.",
    items: [
      "Bank automatisch verbinden: Umsätze und Kontostand per Klick, ohne CSV-Export",
      "Änderungsprotokoll täglich extern bestätigt, die Nachweise bewahren wir für dich auf",
      "Tägliche automatische Prüfung deines Protokolls",
      "Software-Support per E-Mail",
      "Bis zu 3 Geräte",
    ],
  },
  {
    id: "backup",
    name: "Plus mit Backup",
    level: 3,
    badge: "Komplett",
    lead: "Macht deinen Schutz komplett: Deine Daten sichern sich von selbst.",
    items: [
      "Alles aus Plus",
      "Automatische Sicherung, die nur du lesen kannst",
      "Alle Belege im Original, bis 25 GB",
      "Fällt dein Gerät aus, holst du deine Daten zurück",
    ],
  },
];

export const setupSupportOffer = {
  name: "Community Support",
  lead: "Ohne Plus, aber mit Hilfe, wenn du sie brauchst.",
  items: [
    "Software-Support per E-Mail bei Fehlern, Einrichtung und Bedienung",
    "Die App bleibt die kostenlose Community-Version, ohne Anmeldung",
    "Später auf Plus wechseln, die bezahlte Zeit rechnen wir an",
  ],
} as const;

/** "" while unanswered, "none" without a shop, otherwise the provider ID. */
export type SetupShop = string;

/** The line of the review step for the shop answer. */
export function setupShopSummary(shop: SetupShop, label: (id: string) => string | undefined) {
  if (!shop) return "Übersprungen, Shop-Bestellungen ausgeblendet";
  if (shop === "none") return "Kein Online-Shop, Shop-Bestellungen ausgeblendet";
  return `${label(shop) ?? shop}, verbinden direkt nach dem Start`;
}

/** The line of the review step for the plan answer. */
export function setupPlanSummary(booked: "" | SetupPlusTier) {
  if (!booked) return "Community, kostenlos";
  return `${setupPlusTiers.find((tier) => tier.id === booked)?.name}: Buchung im Browser geöffnet`;
}
