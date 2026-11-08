/** Account states reported by AccountService.Status(). */
export type AccountState = "signed-out" | "signed-in" | "plus" | "plus-backup";

/** Shape of the Go type account.Status (bindings/…/internal/account). */
export interface AccountStatusLike {
  configured: boolean;
  state: string;
  email: string;
  plan: string;
  features: string[] | null;
  expiresAt: string;
  notice: string;
  noticeKind: string;
}

export interface PlanStatus {
  /** False until the first status arrived from Go. */
  loaded: boolean;
  /** False when this build has no license server/public key configured. */
  configured: boolean;
  state: AccountState;
  email: string;
  /** Plan id of the verified entitlement, e.g. "plus_tax_backup", or "". */
  plan: string;
  /** Verified, unexpired automatic bank fetch; true for every paid tier. */
  plusActive: boolean;
  /** Verified, unexpired backup add-on (encrypted cloud backup). */
  backupActive: boolean;
  /** Verified, unexpired "Plus Steuer" (transmission to ELSTER). */
  taxActive: boolean;
  /** Community Support is booked: e-mail support and the community, no app features. */
  supportActive: boolean;
  /** ISO timestamp until which the offline confirmation is valid, or "". */
  expiresAt: string;
  notice: string;
  noticeKind: string;
}

export const initialPlanStatus: PlanStatus = {
  loaded: false,
  configured: false,
  state: "signed-out",
  email: "",
  plan: "",
  plusActive: false,
  backupActive: false,
  taxActive: false,
  supportActive: false,
  expiresAt: "",
  notice: "",
  noticeKind: "",
};

const states: AccountState[] = ["signed-out", "signed-in", "plus", "plus-backup"];

/**
 * Normalize the Go status; unknown states fall back to "signed-out". Paid
 * features follow the verified token's feature list, so the backup add-on
 * counts on every tier.
 */
export function planFromStatus(status: AccountStatusLike): PlanStatus {
  const state = states.find((item) => item === status.state) ?? "signed-out";
  const paid = state === "plus" || state === "plus-backup";
  const features = paid ? (status.features ?? []) : [];
  // Community Support has no token; Go names it while the state is signed-in.
  const support = state === "signed-in" && status.plan === "support";
  return {
    loaded: true,
    configured: status.configured,
    state,
    email: status.email ?? "",
    plan: paid ? (status.plan ?? "") : support ? "support" : "",
    plusActive: paid,
    backupActive: state === "plus-backup" || features.includes("backup"),
    taxActive: features.includes("elster"),
    supportActive: support,
    expiresAt: status.expiresAt ?? "",
    notice: status.notice ?? "",
    noticeKind: status.noticeKind ?? "",
  };
}

/** Human label of the account's plan for the account page. */
export function planLabel(
  status: Pick<PlanStatus, "state" | "taxActive" | "backupActive"> & { supportActive?: boolean },
): string {
  switch (status.state) {
    case "plus-backup":
    case "plus": {
      const tier = status.taxActive ? "Plus Steuer" : "Plus";
      return status.backupActive ? `${tier} mit Backup aktiv` : `${tier} aktiv`;
    }
    case "signed-in":
      return status.supportActive
        ? "Community Support aktiv"
        : "Angemeldet, kein Abo aktiv (Community)";
    default:
      return "Nicht angemeldet (Community)";
  }
}

/**
 * The website page that books Plus ("plus") or Plus with backup ("backup").
 * A running Community Support is switched, not booked a second time, so its
 * customers land on the switch page.
 */
export function plusBookingUrl(
  manageUrl: string,
  tier: "plus" | "backup",
  supportActive: boolean,
): string {
  const base = (manageUrl || "https://papierleicht.de/konto").replace(/\/+$/, "");
  return `${base}${supportActive ? "/wechsel" : ""}?tarif=${tier}`;
}

/** German date for an ISO timestamp, "" for empty or invalid input. */
export function formatPlanDate(iso: string): string {
  if (!iso) return "";
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) return "";
  return date.toLocaleDateString("de-DE", { day: "2-digit", month: "2-digit", year: "numeric" });
}

/** Size of a backup or of the storage it uses. */
export function formatBackupSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(0)} KB`;
  if (bytes < 1024 * 1024 * 1024) return `${(bytes / 1024 / 1024).toFixed(1).replace(".", ",")} MB`;
  return `${(bytes / 1024 / 1024 / 1024).toFixed(1).replace(".", ",")} GB`;
}

/** Keep the German headline of a service error, drop technical suffixes. */
export function accountErrorMessage(error: unknown, fallback: string): string {
  const detail = (error instanceof Error ? error.message : String(error ?? "")).trim();
  if (!detail) return fallback;
  return detail.replace(/\s\([^)]*\)$/s, "").split(/: (?=\S)/)[0] || fallback;
}

/** Plus Steuer needs the ELSTER transmission (ERiC); until then it is hidden. */
export const TAX_TIER_AVAILABLE: boolean = false;

const allTiers = [
  { id: "community", label: "Community" },
  { id: "plus", label: "Plus" },
  { id: "plus_tax", label: "Plus Steuer" },
] as const;

export type PlanTierId = (typeof allTiers)[number]["id"];

/** The tiers shown as levels in the sidebar, lowest first. */
export const planTiers: readonly (typeof allTiers)[number][] = TAX_TIER_AVAILABLE
  ? allTiers
  : allTiers.slice(0, 2);

export interface PlanLevel {
  /** Index into planTiers of the active tier. */
  level: number;
  tier: PlanTierId;
  /** The tier to advertise, or null on the highest tier. */
  next: PlanTierId | null;
  /** Call to action for the next tier, "" on the highest tier. */
  cta: string;
}

/** Which tier is active, and which one to advertise next. */
export function planLevel(status: Pick<PlanStatus, "plusActive" | "taxActive">): PlanLevel {
  const level = Math.min(status.taxActive ? 2 : status.plusActive ? 1 : 0, planTiers.length - 1);
  const tier = planTiers[level].id;
  const nextTier = planTiers[level + 1];
  return {
    level,
    tier,
    next: nextTier ? nextTier.id : null,
    cta: nextTier ? `${nextTier.label} freischalten` : "",
  };
}
