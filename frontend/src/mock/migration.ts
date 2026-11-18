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

/** The programs the desktop app reads over their API. */
const apiSources = ["lexware-office", "sevdesk", "easybill"];

function sources() {
  const list = Object.entries(metadata).map(([path, meta]) => ({
    ...meta,
    ways: (meta.ways as string[]) ?? [],
    api: apiSources.includes(meta.id),
    sources: (meta.sources as string[]) ?? [],
    guide: texts[path.replace("quelle.json", "anleitung.md")] ?? "",
  }));
  return list.sort((a, b) => {
    if ((a.id === "andere") !== (b.id === "andere")) return a.id === "andere" ? 1 : -1;
    if (!!a.deadline !== !!b.deadline) return a.deadline ? -1 : 1;
    if (a.deadline !== b.deadline) return (a.deadline ?? "") < (b.deadline ?? "") ? -1 : 1;
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
    basis: "einvoice",
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
    basis: "einvoice",
    origin: "invoiz-Rechnungen/2025/GS-2025-004.pdf",
    importedAt: "2026-11-18T10:00:00Z",
    files: [file("GS-2025-004.pdf")],
  },
  {
    id: "leg-demo-4",
    source: "sevdesk",
    sourceId: "4",
    direction: "incoming",
    kind: "document",
    number: "ER-2025-088",
    date: "2025-06-02",
    partner: "Papierhandel Süd",
    currency: "EUR",
    net: "",
    tax: "",
    gross: "59.50",
    eInvoice: false,
    basis: "datev",
    origin: "sevdesk-export.zip/DATEV_Rechnungseingang_2025.zip/beleg-88.pdf",
    importedAt: "2026-11-18T10:00:00Z",
    files: [file("beleg-88.pdf")],
  },
  {
    id: "leg-demo-5",
    source: "sevdesk",
    sourceId: "5",
    direction: "",
    kind: "export",
    number: "DATEV-Buchungsstapel 01.01.2025 – 31.12.2025",
    date: "2025-12-31",
    partner: "",
    currency: "",
    net: "",
    tax: "",
    gross: "",
    eInvoice: false,
    origin: "sevdesk-export.zip",
    importedAt: "2026-11-18T10:00:00Z",
    files: [file("EXTF_Buchungsstapel_2025.csv"), file("EXTF_GP_Stamm_2025.csv")],
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
    year: "2025",
    direction: "incoming",
    count: 1,
    structured: 0,
    net: "0.00",
    tax: "0.00",
    gross: "0.00",
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
  "MigrationService.Overview": () => ({
    sources: sources(),
    years,
    total: documents.filter((doc) => doc.kind !== "export").length,
  }),
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
  "MigrationService.PreviewExportList": () => {
    throw desktopOnly("der Umzug");
  },
  "MigrationService.ImportExportList": () => {
    throw desktopOnly("der Umzug");
  },
  "MigrationService.ConnectAPI": () => {
    throw desktopOnly("die direkte Verbindung zu deinem bisherigen Programm");
  },
  "MigrationService.APIStatus": () => ({
    state: "idle",
    source: "",
    account: "",
    phase: "",
    total: 0,
    done: 0,
    imported: 0,
    duplicates: 0,
    failed: [],
    message: "",
    customers: 0,
  }),
  "MigrationService.CancelAPI": () => undefined,
  "MigrationService.PreviewAPIList": () => {
    throw desktopOnly("der Umzug");
  },
  "MigrationService.ImportAPIList": () => {
    throw desktopOnly("der Umzug");
  },
  "MigrationService.SaveArchiveFiles": () => {
    throw desktopOnly("das Speichern von Dateien");
  },
  "MigrationService.OpenArchiveFile": () => {
    throw desktopOnly("das Öffnen von Dateien");
  },
  "MigrationService.SaveArchiveFile": () => {
    throw desktopOnly("das Speichern von Dateien");
  },
};
