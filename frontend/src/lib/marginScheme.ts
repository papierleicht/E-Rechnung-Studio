/** The positions of an invoice sold under the Differenzbesteuerung (§ 25a UStG). */
export function marginLines<L extends { taxTreatment?: string }>(draft: {
  taxTreatment: string;
  lines: L[] | null;
}): L[] {
  return (draft.lines ?? []).filter((line) =>
    (draft.taxTreatment === "mixed" ? line.taxTreatment || "" : draft.taxTreatment).startsWith(
      "margin-",
    ),
  );
}
