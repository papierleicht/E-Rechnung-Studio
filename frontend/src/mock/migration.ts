/**
 * The move page in the live demo: the same guides as the app (read from the
 * Go package at build time) and a small legacy archive. Reading files needs
 * the desktop app.
 */
import { desktopOnly } from "./hints";

const metadata = import.meta.glob("../../../internal/umzug/quellen/*/quelle.json", {
  import: "default",
  eager: true,
}) as Record<string, { id: string; name: string; deadline?: string } & Record<string, unknown>>;
const texts = import.meta.glob("../../../internal/umzug/quellen/*/anleitung.md", {
  query: "?raw",
  import: "default",
  eager: true,
}) as Record<string, string>;

function sources() {
  const list = Object.entries(metadata).map(([path, meta]) => ({
    ...meta,
    ways: (meta.ways as string[]) ?? [],
    sources: (meta.sources as string[]) ?? [],
    guide: texts[path.replace("quelle.json", "anleitung.md")] ?? "",
  }));
  return list.sort((a, b) => {
    if ((a.id === "andere") !== (b.id === "andere")) return a.id === "andere" ? 1 : -1;
    if (!!a.deadline !== !!b.deadline) return a.deadline ? -1 : 1;
    return a.name.localeCompare(b.name);
  });
}

const file = (name: string) => ({
  name,
  mime: name.endsWith(".xml") ? "application/xml" : "application/pdf",
  sha256: name,
  size: 48_000,
});

const documents = [
  {
    id: "leg-demo-1",
    source: "invoiz",
    sourceId: "1",
    direction: "outgoing",
    kind: "invoice",
    number: "RE-2025-118",
    date: "2025-12-12",
    partner: "Atelier Nordlicht GmbH",
    currency: "EUR",
    net: "1450.00",
    tax: "275.50",
    gross: "1725.50",
    eInvoice: true,
    origin: "invoiz-Rechnungen/2025/RE-2025-118.pdf",
    importedAt: "2026-11-18T10:00:00Z",
    files: [file("RE-2025-118.pdf")],
  },
  {
    id: "leg-demo-2",
    source: "invoiz",
    sourceId: "2",
    direction: "outgoing",
    kind: "credit-note",
    number: "GS-2025-004",
    date: "2025-11-03",
    partner: "Café Kranich",
    currency: "EUR",
    net: "120.00",
    tax: "22.80",
    gross: "142.80",
    eInvoice: true,
    origin: "invoiz-Rechnungen/2025/GS-2025-004.pdf",
    importedAt: "2026-11-18T10:00:00Z",
    files: [file("GS-2025-004.pdf")],
  },
  {
    id: "leg-demo-3",
    source: "invoiz",
    sourceId: "3",
    direction: "",
    kind: "document",
    number: "",
    date: "",
    partner: "",
    currency: "",
    net: "",
    tax: "",
    gross: "",
    eInvoice: false,
    origin: "invoiz-Rechnungen/2024/Rechnung_2024_031.pdf",
    importedAt: "2026-11-18T10:00:00Z",
    files: [file("Rechnung_2024_031.pdf")],
  },
];

const years = [
  {
    year: "2025",
    direction: "outgoing",
    count: 2,
    structured: 2,
    net: "1330.00",
    tax: "252.70",
    gross: "1582.70",
  },
  {
    year: "ohne Datum",
    direction: "",
    count: 1,
    structured: 0,
    net: "0.00",
    tax: "0.00",
    gross: "0.00",
  },
];

type Filter = { year?: string; direction?: string; query?: string };

export const migrationHandlers = {
  "MigrationService.Overview": () => ({ sources: sources(), years, total: documents.length }),
  "MigrationService.Archive": (filter: Filter) => {
    const query = (filter?.query ?? "").trim().toLowerCase();
    return documents.filter(
      (doc) =>
        (!filter?.year || doc.date.startsWith(filter.year)) &&
        (!filter?.direction || doc.direction === filter.direction) &&
        (!query ||
          [doc.number, doc.partner, doc.files[0].name].some((v) =>
            v.toLowerCase().includes(query),
          )),
    );
  },
  "MigrationService.PreviewFiles": () => {
    throw desktopOnly("der Umzug");
  },
  "MigrationService.ImportFiles": () => {
    throw desktopOnly("der Umzug");
  },
  "MigrationService.PreviewList": () => {
    throw desktopOnly("der Umzug");
  },
  "MigrationService.ImportList": () => {
    throw desktopOnly("der Umzug");
  },
  "MigrationService.OpenArchiveFile": () => {
    throw desktopOnly("das Öffnen von Dateien");
  },
  "MigrationService.SaveArchiveFile": () => {
    throw desktopOnly("das Speichern von Dateien");
  },
};
