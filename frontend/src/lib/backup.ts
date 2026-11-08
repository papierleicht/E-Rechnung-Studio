import { formatBackupSize } from "./planStatus.ts";

interface RestoreImpactLike {
  invoices?: string[] | null;
  received: number;
  expenses: number;
}

interface ProgressLike {
  phase: string;
  done: number;
  total: number;
  doneBytes: number;
  totalBytes: number;
}

interface BackupStatusLike {
  available: boolean;
  keyConfigured: boolean;
  pending: boolean;
  lastBackupAt: string;
  lastError: string;
}

function plural(count: number, one: string, many: string) {
  return `${count.toLocaleString("de-DE")} ${count === 1 ? one : many}`;
}

/** Names what a restore removes; empty if the backup contains everything. */
export function restoreLoss(impact: RestoreImpactLike): string {
  const invoices = impact.invoices ?? [];
  const parts: string[] = [];
  if (invoices.length) {
    const shown = invoices.slice(0, 3).join(", ");
    const more = invoices.length > 3 ? " …" : "";
    parts.push(
      `${plural(invoices.length, "ausgestellte Rechnung", "ausgestellte Rechnungen")} (${shown}${more})`,
    );
  }
  if (impact.received) {
    parts.push(plural(impact.received, "Eingangsbeleg", "Eingangsbelege"));
  }
  if (impact.expenses) {
    parts.push(plural(impact.expenses, "erfasste Ausgabe", "erfasste Ausgaben"));
  }
  if (!parts.length) return "";
  return `Diese Sicherung enthält nicht: ${parts.join(", ")}. Sie verschwinden damit aus der App und bleiben nur in der automatischen Sicherung des jetzigen Stands erhalten.`;
}

const phases: Record<string, string> = {
  upload: "Sicherung läuft",
  verify: "Prüfung läuft",
  restore: "Wiederherstellung läuft",
  archive: "Datei wird geschrieben",
};

export function progressLabel(p: ProgressLike): string {
  const documents = `${p.done.toLocaleString("de-DE")} von ${plural(p.total, "Dokument", "Dokumenten")}`;
  const size = p.totalBytes
    ? ` (${formatBackupSize(p.doneBytes)} von ${formatBackupSize(p.totalBytes)})`
    : "";
  return `${phases[p.phase] ?? "Sicherung läuft"}: ${documents}${size}`;
}

/** Share of the storage from which the app warns. */
export const STORAGE_WARNING = 0.8;

export function storageFull(usedBytes: number, quotaBytes: number): boolean {
  return quotaBytes > 0 && usedBytes / quotaBytes >= STORAGE_WARNING;
}

const DAY = 24 * 60 * 60 * 1000;
/** Days without a backup before the app points out an open problem. */
export const OVERDUE_DAYS = 3;

/**
 * Days since the last successful backup if changes wait or the last run
 * failed; 0 while everything is fine. Only then is there something to say:
 * without changes nothing needs to be backed up.
 */
export function backupOverdueDays(status: BackupStatusLike, now: Date): number {
  if (!status.available || !status.keyConfigured) return 0;
  if (!status.pending && !status.lastError) return 0;
  const last = status.lastBackupAt ? new Date(status.lastBackupAt).getTime() : Number.NaN;
  if (Number.isNaN(last)) return status.lastError ? OVERDUE_DAYS : 0;
  const days = Math.floor((now.getTime() - last) / DAY);
  return days >= OVERDUE_DAYS ? days : 0;
}

/** A recovery key has 14 groups of four characters. */
export const KEY_GROUPS = 14;
const KEY_GROUP_SIZE = 4;

/** Uppercases, drops everything else and sets the dashes while typing or pasting. */
export function formatRecoveryKey(text: string): string {
  const clean = text
    .toUpperCase()
    .replace(/[^A-Z0-9]/g, "")
    .slice(0, KEY_GROUPS * KEY_GROUP_SIZE);
  return (clean.match(/.{1,4}/g) ?? []).join("-");
}

/** Two lines of seven groups each, so a key reads and copies the same everywhere. */
export function recoveryKeyLines(key: string): string[] {
  const groups = key.split("-");
  return [groups.slice(0, 7).join("-") + "-", groups.slice(7).join("-")];
}

/** How many groups are fully entered. */
export function keyGroupsEntered(key: string): number {
  return key.split("-").filter((group) => group.length === KEY_GROUP_SIZE).length;
}

/** A check older than a year is due again. */
export function verificationDue(lastVerifiedAt: string, now: Date): boolean {
  if (!lastVerifiedAt) return true;
  const last = new Date(lastVerifiedAt).getTime();
  return Number.isNaN(last) || now.getTime() - last > 365 * DAY;
}
