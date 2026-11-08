import type {
  ImportInspection,
  InputTaxChoice,
  ReceivedAttachment,
  ReceivedInvoice,
} from "../../bindings/papierleicht.de/e-rechnung-studio/internal/model/models";
import { amountLabel, dateLabel } from "./format.ts";

/** Read binary files without UTF-8 conversion; PDF originals must remain byte-identical. */
export async function encodeInvoiceFile(file: File): Promise<string> {
  if (file.size === 0 || file.size > 20 * 1024 * 1024) {
    throw new Error("Wähle eine Datei, die nicht leer und höchstens 20 MB groß ist.");
  }
  const bytes = new Uint8Array(await file.arrayBuffer());
  let binary = "";
  for (let offset = 0; offset < bytes.length; offset += 8192) {
    binary += String.fromCharCode(...bytes.subarray(offset, offset + 8192));
  }
  return btoa(binary);
}

export const inspectionLabels: Record<string, string> = {
  valid: "Technische Prüfung bestanden",
  invalid: "Fehler gefunden",
  incomplete: "Angaben fehlen",
  unsupported: "Nicht bestätigt",
  warning: "Hinweise prüfen",
  error: "Prüfung fehlgeschlagen",
};

/** Class of a finding in a received document, as set by internal/validation/received.go. */
export const issueClassLabels: Record<string, string> = {
  format: "Formatfehler",
  business: "Geschäftsregel",
  hint: "Hinweis der App",
  unchecked: "Nicht geprüft",
};

type Checked = Pick<ImportInspection, "validation" | "inputTax">;

/**
 * Whether the Vorsteuer of a received document goes into the figures: by the user's
 * decision, otherwise by the app's suggestion. Without a status, only a passed check counts.
 */
export function countsForInputTax(inspection: Checked): boolean {
  const status = inspection.inputTax;
  if (status?.choice) return status.choice.mode !== "none";
  return status ? status.allowed : inspection.validation.state === "valid";
}

/** Whether a document waits for the user: the app left it out and nobody has decided yet. */
export function awaitsInputTaxDecision(inspection: Checked): boolean {
  return !inspection.inputTax?.choice && !countsForInputTax(inspection);
}

/** List label of a check; findings that concern no Pflichtangabe say so instead of "Fehler gefunden". */
export function inspectionLabel(inspection: Checked): string {
  const state = inspection.validation.state;
  if (state !== "valid" && inspection.inputTax?.allowed) {
    return inspection.inputTax.onPayment
      ? "Hinweise, Vorsteuer mit der Zahlung"
      : "Hinweise, Vorsteuer eingerechnet";
  }
  return inspectionLabels[state] || "Ungeprüft";
}

/** What the user decided about the Vorsteuer of a document, in a few words. */
export function inputTaxChoiceLabel(choice: InputTaxChoice): string {
  if (choice.mode === "none") return "Nicht in die Vorsteuer eingerechnet";
  if (choice.mode === "amount") return `${amountLabel(choice.amount ?? "")} eingerechnet`;
  if (choice.mode === "payment") return "Mit der Zahlung eingerechnet, jeweils der bezahlte Anteil";
  return "Die ganze Steuer eingerechnet";
}

const attachmentTypeLabels: Record<string, string> = {
  "application/pdf": "PDF",
  "image/png": "Bild (PNG)",
  "image/jpeg": "Bild (JPEG)",
  "text/csv": "Tabelle (CSV)",
  "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet": "Tabelle (XLSX)",
  "application/vnd.oasis.opendocument.spreadsheet": "Tabelle (ODS)",
};

/** Plain wording for the media type of an attachment; an unknown type is shown as declared. */
export function attachmentTypeLabel(mimeType: string): string {
  if (!mimeType) return "Datei ohne Typangabe";
  return attachmentTypeLabels[mimeType] ?? `Datei (${mimeType})`;
}

export function fileSizeLabel(bytes: number): string {
  if (bytes < 1024 * 1024) return `${Math.max(1, Math.ceil(bytes / 1024))} KB`;
  return `${(bytes / (1024 * 1024)).toLocaleString("de-DE", { maximumFractionDigits: 1 })} MB`;
}

export const attachmentTitle = (a: Pick<ReceivedAttachment, "name" | "filename" | "reference">) =>
  a.name || a.filename || a.reference || "Anlage";

/** The line under the title of an attachment: what it is and where it is. */
export function attachmentSummary(a: ReceivedAttachment): string {
  const title = attachmentTitle(a);
  if (a.kind === "file") {
    return [
      attachmentTypeLabel(a.mimeType),
      fileSizeLabel(a.size),
      a.filename !== title ? a.filename : "",
    ]
      .filter(Boolean)
      .join(" · ");
  }
  if (a.kind === "link") return `Verweist auf eine Webadresse, die die App nicht öffnet: ${a.link}`;
  const reference = a.reference && a.reference !== title ? ` (${a.reference})` : "";
  return `Nur ein Verweis${reference}. Die Datei liegt der Rechnung nicht bei.`;
}

/** Plain wording for the document type (BT-3); an ordinary invoice has none. */
export function documentKindLabel(inspection: Pick<ImportInspection, "documentType">): string {
  switch (inspection.documentType) {
    case 381:
      return "Gutschrift";
    case 384:
      return "Berichtigung";
    case 386:
      return "Anzahlungsrechnung";
    case 326:
      return "Teilrechnung";
    case 875:
      return "Abschlagsrechnung";
    default:
      return "";
  }
}

/** The line under a receipt in the list: number, file type, kind of document and check result. */
export function receiptSubtitle(item: ReceivedInvoice): string {
  return [
    item.inspection.invoiceNumber || item.filename,
    item.kind.toUpperCase(),
    documentKindLabel(item.inspection),
    inspectionLabel(item.inspection),
    item.inspection.inputTax?.choice ? "Vorsteuer von dir festgelegt" : "",
  ]
    .filter(Boolean)
    .join(" · ");
}

/** The invoices a document refers to (BG-3), such as "RE-1 vom 12.03.2026". */
export function precedingLabel(inspection: Pick<ImportInspection, "precedingInvoices">): string {
  return (inspection.precedingInvoices ?? [])
    .map((ref) => (ref.date ? `${ref.number} vom ${dateLabel(ref.date)}` : ref.number))
    .join(", ");
}

const folded = (text: string) => text.trim().replace(/\s+/g, " ").toLowerCase();

/**
 * The received invoice that a correcting document refers to: same supplier, and the
 * number it names. Credit notes already count negative, so they have no original.
 * Mirrors receivedOriginals in received_corrections.go.
 */
export function originalOf(
  item: ReceivedInvoice,
  items: ReceivedInvoice[],
): ReceivedInvoice | undefined {
  const { documentType, precedingInvoices, sellerName } = item.inspection;
  if (documentType === 381 || (documentType !== 384 && !precedingInvoices?.length)) return;
  const supplier = folded(sellerName);
  const numbers = new Set((precedingInvoices ?? []).map((ref) => folded(ref.number)));
  if (!supplier) return;
  return items.find(
    (other) =>
      other.id !== item.id &&
      other.status !== "misimport" &&
      folded(other.inspection.sellerName) === supplier &&
      numbers.has(folded(other.inspection.invoiceNumber)) &&
      folded(other.inspection.invoiceNumber) !== "",
  );
}
