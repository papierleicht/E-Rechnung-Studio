import type { InvoiceDraft } from "../../bindings/papierleicht.de/e-rechnung-studio/internal/model/models";
import type {
  InvoiceNumbering,
  NumberSeriesOption,
} from "../../bindings/papierleicht.de/e-rechnung-studio/internal/app/models";

/** The dialog choice of a draft: the value of a series option or "manual". */
export function numberChoice(numbering: InvoiceNumbering): string {
  if (numbering.selected === "manual") return "manual";
  const options = numbering.options ?? [];
  return options.some((option) => option.value === numbering.selected)
    ? numbering.selected
    : (options[0]?.value ?? "manual");
}

export function numberSeriesTitle(option: NumberSeriesOption): string {
  if (option.kind === "customer") return `Fortlaufend für ${option.label}`;
  if (option.kind === "recent") return "Fortlaufend wie zuletzt verwendet";
  return "Fortlaufend (Standard)";
}

/** Applies a chosen number; a payment reference that showed the old number follows it. */
export function applyNumberChoice(
  draft: InvoiceDraft,
  value: { number: string; automaticNumber: boolean; numberSeries: string },
) {
  if (!draft.paymentReference || draft.paymentReference === draft.number)
    draft.paymentReference = value.number;
  draft.number = value.number;
  draft.automaticNumber = value.automaticNumber;
  draft.numberSeries = value.numberSeries;
}
