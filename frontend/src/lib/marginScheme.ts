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

/** The purchase price per unit a document suggests: its gross amount spread over the quantity. */
export function purchasePriceFrom(gross: string, quantity: string): string {
  const total = Number(String(gross).replace(",", "."));
  const qty = Number(String(quantity).replace(",", "."));
  if (!Number.isFinite(total)) return "";
  const perUnit = Number.isFinite(qty) && qty > 0 ? total / qty : total;
  return (Math.round(perUnit * 100) / 100).toFixed(2).replace(".", ",");
}

/**
 * The invoices besides this one that already take a purchase price from a
 * document. While the document is linked here, one use is this invoice's.
 */
export function otherUses(usedBy: string[], invoiceNumber: string, linkedHere: boolean): string[] {
  if (!linkedHere) return usedBy;
  const own = invoiceNumber || "Entwurf";
  const i = usedBy.indexOf(own);
  return i < 0 ? usedBy : [...usedBy.slice(0, i), ...usedBy.slice(i + 1)];
}
