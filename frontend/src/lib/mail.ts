/** UI helpers for e-mail settings and sending. Delivery and validation stay in Go. */

export interface MailAccount {
  preset: string;
  host: string;
  port: number;
  security: string;
  username: string;
  fromName: string;
  fromAddress: string;
  replyTo: string;
  bccSelf: boolean;
}

export interface MailPreset {
  id: string;
  label: string;
  host: string;
  port: number;
  security: string;
  hint: string;
}

export const securityChoices = [
  { id: "starttls", label: "STARTTLS (meist Port 587)" },
  { id: "tls", label: "SSL/TLS (meist Port 465)" },
] as const;

/** Apply a provider preset; an empty id switches to manual entry and keeps the values. */
export function applyMailPreset(
  account: MailAccount,
  presets: MailPreset[],
  id: string,
): MailAccount {
  const preset = presets.find((item) => item.id === id);
  if (!preset) return { ...account, preset: "" };
  return {
    ...account,
    preset: preset.id,
    host: preset.host,
    port: preset.port,
    security: preset.security,
  };
}

/** Suggest the matching port when the encryption changes and the port is a default. */
export function portForSecurity(security: string, port: number): number {
  if (security === "tls" && (port === 587 || !port)) return 465;
  if (security === "starttls" && (port === 465 || !port)) return 587;
  return port;
}

export function fileSizeLabel(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${Math.round(bytes / 1024)} KB`;
  return `${(bytes / (1024 * 1024)).toLocaleString("de-DE", { maximumFractionDigits: 1 })} MB`;
}

export function sentAtLabel(value: string): string {
  const date = new Date(value);
  if (!value || Number.isNaN(date.getTime())) return "—";
  return date.toLocaleString("de-DE", { dateStyle: "medium", timeStyle: "short" });
}

/** Placeholders that are not in the allowed list, e.g. ["{Nummer}"]. */
export function unknownPlaceholders(text: string, allowed: string[]): string[] {
  const unknown: string[] = [];
  for (const match of text.matchAll(/\{([^{}\r\n]{1,40})\}/g)) {
    if (!allowed.includes(match[1].trim()) && !unknown.includes(match[0])) unknown.push(match[0]);
  }
  return unknown;
}

/** Split typed recipients for the preview; Go parses and validates them again. */
export function recipientList(value: string): string[] {
  return value
    .split(/[,;]/)
    .map((item) => item.trim())
    .filter(Boolean);
}

/** What the send dialog shows; invoice, reminder and quote drafts all fit. */
export interface MailDraftView {
  customerId: string;
  customerName: string;
  to: string;
  canSaveEmail: boolean;
  subject: string;
  body: string;
  from: string;
  replyTo: string;
  bccSelf: boolean;
  attachments: { filename: string; size: number }[] | null;
  configured: boolean;
  hasPassword: boolean;
  templateWarning?: string;
  /** Invoices only: XRechnung may attach the PDF view in addition. */
  xrechnung?: boolean;
  includePdf?: boolean;
  pdfAttachment?: { filename: string; size: number };
}

/** The confirmed message fields; callers add the document reference. */
export interface MailFields {
  to: string;
  cc: string;
  subject: string;
  body: string;
  includePdf: boolean;
  saveRecipient: boolean;
  confirmed: boolean;
}
