export type PeriodKind = "month" | "quarter" | "year";

const pad = (n: number) => String(n).padStart(2, "0");

/** Inclusive ISO date range; index is the month (0–11) or quarter (0–3). */
export function periodRange(
  kind: PeriodKind,
  year: number,
  index: number,
): { from: string; to: string } {
  const firstMonth = kind === "month" ? index : kind === "quarter" ? index * 3 : 0;
  const months = kind === "month" ? 1 : kind === "quarter" ? 3 : 12;
  const lastMonth = firstMonth + months - 1;
  // Day 0 of the following month is the last day of lastMonth.
  const lastDay = new Date(year, lastMonth + 1, 0).getDate();
  return {
    from: `${year}-${pad(firstMonth + 1)}-01`,
    to: `${year}-${pad(lastMonth + 1)}-${pad(lastDay)}`,
  };
}

const monthNames = Array.from({ length: 12 }, (_, i) =>
  new Date(2026, i, 1).toLocaleDateString("de-DE", { month: "long" }),
);

/** "März 2026", "Q1 2026" or "2026". */
export function periodLabel(kind: PeriodKind, year: number, index: number): string {
  return kind === "month"
    ? `${monthNames[index]} ${year}`
    : kind === "quarter"
      ? `Q${index + 1} ${year}`
      : String(year);
}
