/**
 * Security level shown to the customer: three steps that build on each other.
 * It reports what is really in place, not what was booked, so a backup that was
 * never set up or stopped working does not count.
 */
export const SECURITY_STEPS = 3;

export interface SecurityInput {
  plusActive: boolean;
  backupActive: boolean;
  keyConfigured: boolean;
  auto: boolean;
  lastBackupAt: string;
  lastError: string;
  overdueDays: number;
  /** The daily check found a problem with the log or its confirmations. */
  logProblem: boolean;
  /**
   * Community Support is booked: help by e-mail and the community network. It
   * is the first paid step, so it fills the first bar; only the free version
   * stays at 0.
   */
  supportActive?: boolean;
}

export interface SecurityState {
  /** 0 to SECURITY_STEPS. */
  level: number;
  title: string;
  steps: { id: string; label: string; done: boolean }[];
}

const levels = [
  "Nur auf diesem Gerät",
  "Abweichung gefunden",
  "Weniger Handarbeit, extern bestätigt",
  "Komplett: bestätigt und gesichert",
];

export function securityState(input: SecurityInput): SecurityState {
  const confirmed = input.plusActive;
  const watched = confirmed && !input.logProblem;
  const secured =
    watched &&
    input.backupActive &&
    input.keyConfigured &&
    Boolean(input.lastBackupAt) &&
    input.auto &&
    !input.lastError &&
    input.overdueDays === 0;
  const level = secured ? 3 : watched ? 2 : confirmed || input.supportActive ? 1 : 0;
  return {
    level,
    title: level === 1 && !confirmed ? "Mit Support und Community" : levels[level],
    steps: [
      { id: "confirmed", label: "Änderungsprotokoll täglich extern bestätigt", done: confirmed },
      { id: "watched", label: "Nachweise außer Haus, Protokoll täglich geprüft", done: watched },
      {
        id: "secured",
        label: "Daten außer Haus gesichert, nur du kannst sie lesen",
        done: secured,
      },
    ],
  };
}

export type StageAction = "website" | "bookPlus" | "bookBackup" | "banking" | "audit" | "setup";

export interface StageItem {
  /** What the customer gets out of it: time saved or protection. */
  kind: "time" | "safe";
  label: string;
  done: boolean;
}

export interface Stage {
  id: "community" | "plus" | "backup";
  title: string;
  summary: string;
  /** The station really runs: booked is not enough for a backup without a key. */
  reached: boolean;
  items: StageItem[];
  /** The next thing to do at this stage, if there is one. */
  action: { label: string; kind: StageAction } | null;
}

/**
 * One way with three stations. A plan raises protection and saves time at once,
 * so both stand side by side in the same station instead of in two scales.
 */
export function journey(input: SecurityInput & { bankConnected: boolean }): Stage[] {
  const state = securityState(input);
  const watched = state.level >= 2;
  const secured = state.level === 3;
  let plusAction: Stage["action"] = null;
  if (!input.plusActive) plusAction = { label: "Plus buchen", kind: "bookPlus" };
  else if (input.logProblem) plusAction = { label: "Protokoll prüfen", kind: "audit" };
  else if (!input.bankConnected) plusAction = { label: "Bank verbinden", kind: "banking" };
  let backupAction: Stage["action"] = null;
  if (!input.backupActive) {
    backupAction = input.plusActive
      ? { label: "Backup hinzubuchen", kind: "website" }
      : { label: "Plus mit Backup buchen", kind: "bookBackup" };
  } else if (!input.keyConfigured) {
    backupAction = { label: "Sicherung einrichten", kind: "setup" };
  } else if (!secured && input.plusActive) {
    backupAction = { label: "Sicherung prüfen", kind: "setup" };
  }
  return [
    {
      id: "community",
      title: input.supportActive ? "Community Support" : "Community",
      summary: input.supportActive
        ? "Die ganze App, dazu Support per E-Mail und das Unternehmer-Netzwerk"
        : "Die ganze App, kostenlos auf deinem Gerät",
      reached: true,
      items: [
        { kind: "time", label: "Zahlungen werden automatisch zugeordnet", done: true },
        { kind: "safe", label: "Änderungsprotokoll und Prüffunktion in der App", done: true },
      ],
      action: null,
    },
    {
      id: "plus",
      title: "Plus",
      summary: "Spart dir Zeit und lässt dein Protokoll extern bestätigen",
      reached: watched,
      items: [
        {
          kind: "time",
          label: "Umsätze per Klick von der Bank, ohne CSV-Export",
          done: input.plusActive && input.bankConnected,
        },
        {
          kind: "safe",
          label: "Änderungsprotokoll täglich extern bestätigt",
          done: input.plusActive,
        },
        { kind: "safe", label: "Nachweise aufbewahrt, Protokoll täglich geprüft", done: watched },
      ],
      action: plusAction,
    },
    {
      id: "backup",
      title: "Backup-Zusatz",
      summary: "Macht deinen Schutz komplett: deine Daten auch außer Haus",
      reached: secured,
      items: [
        {
          kind: "safe",
          label: "Daten außer Haus gesichert, nur du kannst sie lesen",
          done: secured,
        },
        { kind: "time", label: "Die Sicherung läuft von selbst mit", done: secured },
      ],
      action: backupAction,
    },
  ];
}

// What protects first, what saves time next, and only then what costs money.
const actionOrder: StageAction[] = [
  "audit",
  "setup",
  "banking",
  "website",
  "bookPlus",
  "bookBackup",
];

/** The one next step, shown the same in the sidebar, the overview and on the way. */
export function nextAction(stages: Stage[]): Stage["action"] {
  const actions = stages.flatMap((stage) => (stage.action ? [stage.action] : []));
  actions.sort((a, b) => actionOrder.indexOf(a.kind) - actionOrder.indexOf(b.kind));
  return actions[0] ?? null;
}

/** The station to show open: the one with the next step. */
export function openStage(stages: Stage[]): Stage["id"] {
  const next = nextAction(stages);
  return (stages.find((stage) => next && stage.action === next) ?? stages[stages.length - 1]).id;
}

/** Quiet confirmation for customers who are fully set up. */
export function lastSavedLabel(iso: string, now: Date): string {
  if (!iso) return "";
  const at = new Date(iso);
  if (Number.isNaN(at.getTime())) return "";
  const time = at.toLocaleTimeString("de-DE", { hour: "2-digit", minute: "2-digit" });
  const age = daysBetween(at, now);
  if (age === 0) return `Gesichert heute, ${time}`;
  if (age === 1) return `Gesichert gestern, ${time}`;
  return `Gesichert am ${at.toLocaleDateString("de-DE", { day: "2-digit", month: "2-digit" })}`;
}

interface ConfirmationLike {
  plan: boolean;
  firstAnchor: string;
  lastAnchor: string;
  comparedAt: string;
  conflicts: number;
}

const DAY = 86_400_000;

function daysBetween(from: Date, to: Date): number {
  const start = Date.UTC(from.getFullYear(), from.getMonth(), from.getDate());
  const end = Date.UTC(to.getFullYear(), to.getMonth(), to.getDate());
  return Math.round((end - start) / DAY);
}

/** "Seit 412 Tagen extern bestätigt, zuletzt heute, keine Abweichung." */
export function confirmationLine(status: ConfirmationLike, now: Date): string {
  if (!status.lastAnchor) {
    return status.plan ? "Wird bei der nächsten Verbindung erstellt" : "Gehört zu Plus";
  }
  const since = daysBetween(new Date(status.firstAnchor), now);
  const start =
    since === 0
      ? "Seit heute extern bestätigt"
      : `Seit ${since.toLocaleString("de-DE")} ${since === 1 ? "Tag" : "Tagen"} extern bestätigt`;
  const last = new Date(status.lastAnchor);
  const age = daysBetween(last, now);
  const latest =
    age === 0
      ? "zuletzt heute"
      : age === 1
        ? "zuletzt gestern"
        : `zuletzt am ${last.toLocaleDateString("de-DE", { day: "2-digit", month: "2-digit", year: "numeric" })}`;
  if (status.conflicts > 0) {
    const count = status.conflicts === 1 ? "1 Abweichung" : `${status.conflicts} Abweichungen`;
    return `${start}, ${latest}, ${count}.`;
  }
  return status.comparedAt ? `${start}, ${latest}, keine Abweichung.` : `${start}, ${latest}.`;
}
