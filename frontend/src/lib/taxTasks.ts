/**
 * The tax page as a task list: every filing with its deadline, grouped into
 * what needs doing now, what comes next and what is done.
 */

export type TaxTaskKind = "vat" | "zm" | "annual";
export type TaxTaskState = "overdue" | "due" | "changed" | "running" | "upcoming" | "done";

export interface TaxTask {
  /** "vat:2026-Q3", "zm:2026-Q3" or "annual:2025". */
  id: string;
  kind: TaxTaskKind;
  periodId: string;
  title: string;
  /** Last day of the period. */
  to: string;
  due: string;
  state: TaxTaskState;
  filedOn?: string;
}

interface PeriodLike {
  period: { id: string; label: string; to: string; due: string };
  filing?: { submittedOn: string } | null;
  overdue: boolean;
  started: boolean;
  changed?: boolean;
}

interface AnnualLike {
  year: number;
  due: string;
  filing?: { submittedOn: string } | null;
  changed?: boolean;
}

const titles: Record<TaxTaskKind, string> = {
  vat: "Umsatzsteuer-Voranmeldung",
  zm: "Zusammenfassende Meldung",
  annual: "Steuererklärungen",
};

export const stateLabels: Record<TaxTaskState, string> = {
  overdue: "Überfällig",
  due: "Jetzt fällig",
  changed: "Geändert seit Übermittlung",
  running: "Läuft noch",
  upcoming: "Steht an",
  done: "Erledigt",
};

function periodTask(kind: "vat" | "zm", p: PeriodLike, today: string): TaxTask {
  const state: TaxTaskState = p.filing
    ? p.changed
      ? "changed"
      : "done"
    : p.period.to < today
      ? p.overdue
        ? "overdue"
        : "due"
      : p.started
        ? "running"
        : "upcoming";
  return {
    id: `${kind}:${p.period.id}`,
    kind,
    periodId: p.period.id,
    title: `${titles[kind]} ${p.period.label}`,
    to: p.period.to,
    due: p.period.due,
    state,
    filedOn: p.filing?.submittedOn,
  };
}

function annualTask(a: AnnualLike, today: string): TaxTask {
  const state: TaxTaskState = a.filing
    ? a.changed
      ? "changed"
      : "done"
    : a.due < today
      ? "overdue"
      : `${a.year}-12-31` < today
        ? "upcoming"
        : "running";
  return {
    id: `annual:${a.year}`,
    kind: "annual",
    periodId: String(a.year),
    title: `${titles.annual} ${a.year}`,
    to: `${a.year}-12-31`,
    due: a.due,
    state,
    filedOn: a.filing?.submittedOn,
  };
}

export interface TaxTaskGroups {
  now: TaxTask[];
  soon: TaxTask[];
  done: TaxTask[];
}

/**
 * Groups the filings of `year` and of the year before. In the current view
 * (`archive` false) the previous year only contributes what is still open;
 * periods that have not started yet stay hidden. Periods that ended before
 * `since`, the first record in the app, were handled elsewhere and are left
 * out unless they carry a transmission note.
 */
export function taxTasks(
  input: {
    year: number;
    vat: PeriodLike[];
    zm: PeriodLike[];
    annual: AnnualLike[];
    since?: string;
  },
  today: string,
  archive = false,
): TaxTaskGroups {
  const firstDay = `${input.year}-01-01`;
  const ofYear = (id: string) => id.startsWith(String(input.year));
  const tasks = [
    ...input.vat.map((p) => periodTask("vat", p, today)),
    ...input.zm.map((p) => periodTask("zm", p, today)),
    ...input.annual.map((a) => annualTask(a, today)),
  ].filter((t) => {
    if (t.state === "upcoming" && t.kind !== "annual") return false;
    if (!t.filedOn && input.since && t.to < input.since) return false;
    if (archive || ofYear(t.periodId)) return archive ? ofYear(t.periodId) : true;
    // Previous year: what is still to do, and returns due this year.
    return t.state !== "done" && (t.due >= firstDay || t.state === "changed");
  });
  const byDue = (a: TaxTask, b: TaxTask) => a.due.localeCompare(b.due);
  return {
    now: tasks
      .filter((t) => t.state === "overdue" || t.state === "due" || t.state === "changed")
      .sort(byDue),
    soon: tasks.filter((t) => t.state === "running" || t.state === "upcoming").sort(byDue),
    done: tasks.filter((t) => t.state === "done").sort((a, b) => byDue(b, a)),
  };
}
