// "Chat & Markt": the community chat and "Angebote & Gesuche" for
// customers with Community Support or Plus, plus the contact for everyone.
// The license server decides who takes part; these helpers only shape what
// the page shows.

export const CONTACT_EMAIL = "kontakt@papierleicht.de";

/** The version the server asks for; a new text needs a new version there. */
export const GUIDELINES_VERSION = "2026-11-02";

export type GuidelineSection = { title: string; items: readonly string[] };

export const guidelines: readonly GuidelineSection[] = [
  {
    title: "Respektvoll miteinander",
    items: [
      "Wir duzen uns und bleiben freundlich, auch bei unterschiedlichen Meinungen. Kritik ist willkommen, wenn sie sachlich ist.",
      "Keine Beleidigungen, Drohungen, Belästigung oder Diskriminierung, etwa wegen Herkunft, Geschlecht, Religion, Behinderung, Alter oder sexueller Identität.",
      "Keine extremistischen, pornografischen, gewaltverherrlichenden oder anderweitig rechtswidrigen Inhalte.",
    ],
  },
  {
    title: "Erfahrungen teilen, nicht beraten",
    items: [
      "Hier helfen Unternehmer einander mit Erfahrungen, Tipps und Wegen durch die App.",
      "Antworten aus der Community sind persönliche Erfahrungen, keine Steuer- oder Rechtsberatung. Für deinen Einzelfall bleibt deine Steuerberatung oder Kanzlei zuständig.",
      "Gib keine geschäftsmäßige Hilfe in Steuersachen für andere, wenn du dazu nicht befugt bist.",
    ],
  },
  {
    title: "Daten schützen",
    items: [
      "Teile keine Daten von Kunden oder anderen Dritten: keine Rechnungen mit Namen, keine Adressen, IBAN oder Kundennummern. Schwärze Beispiele vorher.",
      "Teile nie Passwörter, PINs, TANs, Zugangsdaten oder Wiederherstellungsschlüssel. Das papierleicht-Team fragt niemals danach.",
      "Deine eigenen Kontaktdaten gibst du nur bewusst an, etwa in einem Eintrag unter Angebote & Gesuche.",
      "Adressen aus Anfragen nutzt du nur für die Antwort auf diese Anfrage, nicht für Werbung oder Newsletter.",
    ],
  },
  {
    title: "Angebote & Gesuche",
    items: [
      "Biete nur eigene, echte Leistungen und Waren an und suche nur, was du wirklich brauchst. Beschreibe Preise und Bedingungen ehrlich.",
      "Werbung gehört in Angebote & Gesuche, nicht in den Chat. Keine Massenwerbung, Kettenbriefe, Schneeballsysteme oder Partnerlinks.",
      "Nichts Verbotenes: keine Schwarzarbeit, keine verbotenen Waren, keine Leistungen ohne nötige Zulassung.",
      "Anfragen zu einem Eintrag gehen per E-Mail an den Ersteller, nicht in den Chat. Wer anfragt, gibt dafür seine E-Mail-Adresse weiter; die Adresse des Erstellers sieht man erst mit seiner Antwort.",
      "papierleicht vermittelt nicht und ist nicht Vertragspartner. Prüfe dein Gegenüber selbst und sei bei Vorkasse vorsichtig.",
    ],
  },
  {
    title: "Kein Spam, keine Fallen",
    items: [
      "Keine Links zu Schadsoftware, Phishing oder unseriösen Angeboten. Wiederhole keine Nachrichten.",
      "Achte Urheberrechte: Teile nur Texte und Bilder, die du teilen darfst.",
    ],
  },
  {
    title: "Melden und Moderation",
    items: [
      "Verstößt ein Beitrag gegen diese Richtlinien, melde ihn. Nach drei Meldungen wird er ausgeblendet, bis das Team entscheidet.",
      "Das Team kann Beiträge löschen, das Schreiben zeitweise sperren und bei schweren oder wiederholten Verstößen vom Chat ausschließen. Das Abo selbst bleibt davon unberührt.",
      `Fragen zur Moderation beantworten wir unter ${CONTACT_EMAIL}.`,
    ],
  },
  {
    title: "Sichtbarkeit und Speicherung",
    items: [
      "Alle Mitglieder sehen deine Beiträge mit deinem Anzeigenamen, nie deine E-Mail-Adresse.",
      "Eigene Nachrichten löschst du jederzeit. Nachrichten werden nach zwölf Monaten automatisch gelöscht, Einträge enden nach 30 Tagen und lassen sich verlängern.",
    ],
  },
];

export type MarketCategory = { id: string; label: string };

/** Same IDs as the server. */
export const marketCategories: readonly MarketCategory[] = [
  { id: "handwerk", label: "Handwerk" },
  { id: "bau", label: "Bau & Renovierung" },
  { id: "it", label: "IT & Software" },
  { id: "design", label: "Design & Medien" },
  { id: "marketing", label: "Marketing & Vertrieb" },
  { id: "text", label: "Text & Übersetzung" },
  { id: "beratung", label: "Beratung & Coaching" },
  { id: "buero", label: "Büro & Verwaltung" },
  { id: "finanzen", label: "Finanzen & Buchhaltung" },
  { id: "transport", label: "Transport & Logistik" },
  { id: "gastro", label: "Gastronomie & Events" },
  { id: "handel", label: "Handel & Waren" },
  { id: "gesundheit", label: "Gesundheit & Pflege" },
  { id: "bildung", label: "Bildung & Kurse" },
  { id: "foto", label: "Foto & Video" },
  { id: "sonstiges", label: "Sonstiges" },
];

export function categoryLabel(id: string) {
  return marketCategories.find((c) => c.id === id)?.label ?? "Sonstiges";
}

export type AreaOption = { value: string; label: string };

/**
 * The radius filter works on postal codes: German postal codes that start
 * with the same two digits form a region (e.g. 80… for Munich), the first
 * digit a larger zone.
 */
export function areaOptions(postalCode: string): AreaOption[] {
  const options: AreaOption[] = [{ value: "", label: "Ganz Deutschland" }];
  const code = (postalCode ?? "").trim();
  if (!/^[0-9]{5}$/.test(code)) return options;
  options.push(
    { value: code.slice(0, 2), label: `Meine Region (PLZ ${code.slice(0, 2)}…)` },
    { value: code.slice(0, 1), label: `Mein PLZ-Gebiet (${code.slice(0, 1)}…)` },
  );
  return options;
}

/** How near a listing is, measured by the shared start of the postal codes. */
export function nearness(own: string, other: string, remote: boolean): string {
  if (/^[0-9]{5}$/.test(own) && /^[0-9]{5}$/.test(other)) {
    if (own.slice(0, 3) === other.slice(0, 3)) return "ganz in deiner Nähe";
    if (own.slice(0, 2) === other.slice(0, 2)) return "in deiner Region";
  }
  return remote ? "auch online" : "";
}

export function daysLeftLabel(expiresAt: string, now = new Date()): string {
  const end = new Date(expiresAt).getTime();
  if (!Number.isFinite(end)) return "";
  const days = Math.ceil((end - now.getTime()) / 86_400_000);
  if (days <= 0) return "Beendet";
  if (days === 1) return "Endet morgen";
  return `Noch ${days} Tage`;
}

export function timeLabel(iso: string, now = new Date()): string {
  const at = new Date(iso);
  if (Number.isNaN(at.getTime())) return "";
  const time = at.toLocaleTimeString("de-DE", { hour: "2-digit", minute: "2-digit" });
  const sameDay = at.toDateString() === now.toDateString();
  if (sameDay) return time;
  const yesterday = new Date(now);
  yesterday.setDate(now.getDate() - 1);
  if (at.toDateString() === yesterday.toDateString()) return `Gestern, ${time}`;
  return `${at.toLocaleDateString("de-DE", { day: "2-digit", month: "2-digit", year: "numeric" })}, ${time}`;
}

export type ChatLine = {
  id: string;
  seq: number;
  author: string;
  role: string;
  mine: boolean;
  body: string;
  created: string;
  removed: boolean;
};

/** Applies a page of changes: adds new messages, drops removed ones, keeps order. */
export function mergeMessages<T extends ChatLine>(current: T[], changes: T[]): T[] {
  const byId = new Map(current.map((m) => [m.id, m]));
  for (const change of changes) {
    if (change.removed) byId.delete(change.id);
    else byId.set(change.id, change);
  }
  return [...byId.values()].sort((a, b) => a.seq - b.seq);
}

/**
 * The menu badge: what one answer of the server means: on the first check the app only remembers the
 * newest message, so a new device starts without a full badge.
 */
export function unreadStep(mark: number, answer: { count: number; latest: number }) {
  if (mark <= 0 || answer.latest < mark) return { mark: answer.latest, count: 0 };
  return { mark, count: answer.count };
}

/** True when the text addresses the member, e.g. "@Anna". */
export function mentions(body: string, displayName: string): boolean {
  if (!displayName) return false;
  return body.toLocaleLowerCase("de-DE").includes(`@${displayName.toLocaleLowerCase("de-DE")}`);
}

/** Initials for the avatar of a display name. */
export function initials(name: string): string {
  const parts = name
    .trim()
    .split(/[\s._-]+/)
    .filter(Boolean);
  if (parts.length === 0) return "?";
  if (parts.length === 1) return parts[0].slice(0, 2).toLocaleUpperCase("de-DE");
  return (parts[0][0] + parts[parts.length - 1][0]).toLocaleUpperCase("de-DE");
}

/** A stable colour role per name (0 to 3), so avatars are told apart at a glance. */
export function avatarTone(name: string): number {
  let hash = 0;
  for (const ch of name) hash = (hash * 31 + ch.codePointAt(0)!) % 997;
  return hash % 4;
}

// The showcase for customers without the chat. Everything here is a made-up
// example and marked as such on the page; real messages and listings stay
// with the members.

export const sampleMessages: readonly Omit<ChatLine, "created" | "removed" | "seq">[] = [
  {
    id: "s1",
    author: "Lena Grafik",
    role: "member",
    mine: false,
    body: "Hat jemand schon eine XRechnung an eine Behörde geschickt? Welche Leitweg-ID habt ihr genommen?",
  },
  {
    id: "s2",
    author: "Murat Elektro",
    role: "member",
    mine: false,
    body: "Ja, die steht im Auftrag. In der App unter Kunde → Elektronische Adresse eintragen, dann passt das XML.",
  },
  {
    id: "s3",
    author: "papierleicht",
    role: "team",
    mine: false,
    body: "Genau so. Tipp: Vor dem Versand zeigt dir die Prüfung, ob alle Pflichtangaben drin sind.",
  },
  {
    id: "s4",
    author: "Sophie Café",
    role: "member",
    mine: false,
    body: "Danke! Sucht hier zufällig jemand einen Caterer für die Weihnachtsfeier im Raum Köln?",
  },
];

export type SampleListing = {
  id: string;
  kind: "offer" | "request";
  category: string;
  title: string;
  body: string;
  postalCode: string;
  city: string;
  remote: boolean;
  author: string;
};

export const sampleListings: readonly SampleListing[] = [
  {
    id: "l1",
    kind: "request",
    category: "handwerk",
    title: "Elektriker für Ladenumbau gesucht",
    body: "Neue Beleuchtung und vier Steckdosen im Verkaufsraum, Start im Dezember.",
    postalCode: "80331",
    city: "München",
    remote: false,
    author: "Anna Blumen",
  },
  {
    id: "l2",
    kind: "offer",
    category: "it",
    title: "Webseiten für Handwerksbetriebe",
    body: "Schnell, rechtssicher und mit Online-Terminbuchung. Festpreis nach Absprache.",
    postalCode: "10115",
    city: "Berlin",
    remote: true,
    author: "Jonas Web",
  },
  {
    id: "l3",
    kind: "offer",
    category: "gastro",
    title: "Catering für Firmenfeiern",
    body: "Fingerfood und Buffet für 10 bis 120 Personen, regional und saisonal.",
    postalCode: "50667",
    city: "Köln",
    remote: false,
    author: "Sophie Café",
  },
  {
    id: "l4",
    kind: "request",
    category: "foto",
    title: "Produktfotos für Onlineshop",
    body: "Rund 40 Artikel, freigestellt auf Weiß. Gern auch per Versand.",
    postalCode: "20095",
    city: "Hamburg",
    remote: true,
    author: "Kai Shop",
  },
];

/** "1 Angebot · 3 Gesuche" for the tile of the board. */
export function listingCounts(offers: number, requests: number): string {
  return `${offers} ${offers === 1 ? "Angebot" : "Angebote"} · ${requests} ${requests === 1 ? "Gesuch" : "Gesuche"}`;
}

/** The member numbers make the showcase convincing only above a few people. */
export function showNumbers(preview: { members: number } | null | undefined): boolean {
  return !!preview && preview.members >= 10;
}

/**
 * The page buttons of the board: always the first and last page, the
 * current page with one neighbour on each side, and "…" for the gaps. A gap
 * of a single page shows that page instead, so the row never jumps.
 */
export function pageButtons(page: number, pages: number): (number | "…")[] {
  const keep = new Set([1, pages, page - 1, page, page + 1]);
  if (page <= 3) [2, 3, 4].forEach((n) => keep.add(n));
  if (page >= pages - 2) [pages - 3, pages - 2, pages - 1].forEach((n) => keep.add(n));
  const shown = [...keep].filter((n) => n >= 1 && n <= pages).sort((a, b) => a - b);
  const out: (number | "…")[] = [];
  for (const n of shown) {
    const last = out.at(-1);
    if (typeof last === "number" && n - last === 2) out.push(last + 1);
    else if (typeof last === "number" && n - last > 2) out.push("…");
    out.push(n);
  }
  return out;
}

/** "25–48 von 200" above the pager. */
export function pageRange(page: number, pageSize: number, total: number): string {
  if (!total) return "";
  const from = (page - 1) * pageSize + 1;
  return `${from}–${Math.min(page * pageSize, total)} von ${total}`;
}
