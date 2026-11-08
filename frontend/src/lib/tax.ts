import type {
  Company,
  InvoiceDraft,
  InvoiceSettings,
} from "../../bindings/papierleicht.de/e-rechnung-studio/internal/model/models";

import catalog from "../../../internal/catalog/invoice.json" with { type: "json" };

export const taxChoices = catalog.taxTreatments;

export const paymentChoices = [
  { id: "58", label: "SEPA-Überweisung" },
  { id: "30", label: "Überweisung" },
  { id: "10", label: "Barzahlung" },
  { id: "97", label: "Nach Vereinbarung" },
] as const;

/** Display choices only. The Go engine calculates and validates the invoice. */
export function taxGroups(draft: InvoiceDraft) {
  const groups = new Map<string, { key: string; category: string; rate: string; label: string }>();
  for (const line of draft.lines ?? []) {
    const kind =
      draft.taxTreatment === "mixed" ? line.taxTreatment || "standard" : draft.taxTreatment;
    const choice = taxChoices.find((choice) => choice.id === kind);
    let category = kind === "standard" ? line.taxCategory || "S" : choice?.category || "S";
    const number = kind === "standard" ? Number(line.taxRate.replace(",", ".")) : 0;
    if (kind === "standard" && category === "S" && line.taxRate.trim() && number === 0)
      category = "Z";
    if (!Number.isFinite(number)) continue;
    const rate = String(number),
      key = `${category}|${rate}`;
    groups.set(key, {
      key,
      category,
      rate,
      label: `${category === "Z" ? "Nullsteuersatz" : (choice?.label ?? category)} · ${rate.replace(".", ",")} %`,
    });
  }
  return [...groups.values()];
}

/** Why an invoice is corrected; decides the Voranmeldungszeitraum (§ 17 Abs. 1 UStG). */
export const correctionReasons = [
  {
    id: "change",
    label: "Der Betrag ändert sich",
    effect:
      "Rabatt, Rückgabe, Minderung oder die Leistung entfällt. Die Umsatzsteuer wird im Zeitraum dieser Gutschrift berichtigt.",
  },
  {
    id: "replace",
    label: "Die Rechnung war fehlerhaft und wird ersetzt",
    effect:
      "Du stellst danach eine neue Rechnung aus, etwa wegen einer falschen Anschrift. Gutschrift und neue Rechnung zählen im Zeitraum der ursprünglichen Leistung.",
  },
] as const;

export function documentLabel(kind: string) {
  return kind === "credit-note"
    ? "Gutschrift"
    : kind === "corrected-invoice"
      ? "Rechnungskorrektur"
      : "Rechnung";
}

export function companyTaxTreatment(company: Company) {
  if (company.invoiceTaxPreset != null)
    return company.invoiceTaxPreset || (company.smallBusiness ? "small-business" : "standard");
  return company.smallBusiness ? "small-business" : company.defaultTaxTreatment || "standard";
}

export function invoiceTaxDefaults(company: Company, customer?: InvoiceSettings | null) {
  // Historical standard/small-business customer defaults were stored implicitly.
  // Preserve historical special treatments, but let routine invoices follow the profile.
  const override =
    customer?.taxTreatment &&
    customer.taxSource !== "company" &&
    (customer.taxSource === "manual" ||
      customer.taxSource === "customer" ||
      !["standard", "small-business"].includes(customer.taxTreatment));
  return {
    taxTreatment: override ? customer.taxTreatment : companyTaxTreatment(company),
    taxExemptionReason: override
      ? customer.taxExemptionReason || ""
      : company.exemptionReason || "",
    taxSource: override ? "customer" : "company",
  };
}

export function invoiceTaxPreset(company: Company) {
  if (company.invoiceTaxPreset != null) return company.invoiceTaxPreset;
  const legacy = companyTaxTreatment(company);
  return ["standard", "small-business"].includes(legacy) ? "" : legacy;
}

export function chooseCompanyTaxStatus(company: Company, value: string) {
  const previous = company.smallBusiness;
  company.invoiceTaxPreset = invoiceTaxPreset(company);
  company.taxStatusUnclear = value === "unknown";
  if (value !== "unknown") company.smallBusiness = value === "small-business";
  if (company.smallBusiness && company.invoiceTaxPreset === "standard")
    company.invoiceTaxPreset = "";
  if (!company.invoiceTaxPreset && previous !== company.smallBusiness) company.exemptionReason = "";
}

export function chooseInvoiceTaxPreset(company: Company, value: string) {
  if (invoiceTaxPreset(company) !== value) company.exemptionReason = "";
  company.invoiceTaxPreset = value;
}

export function initializePositionTaxes(draft: InvoiceDraft, treatment: string, reason = "") {
  for (const line of draft.lines ?? []) {
    if (!line.taxTreatment) {
      line.taxTreatment = treatment === "mixed" ? "standard" : treatment;
      line.exemptionReason = reason;
    }
  }
}

export const taxReasons: Record<string, string> = Object.fromEntries(
  catalog.taxTreatments.filter((t) => t.reason).map((t) => [t.id, t.reason]),
);

export function taxLabel(kind: string) {
  if (kind === "mixed") return "Steuerfall je Position";
  return taxChoices.find((choice) => choice.id === kind)?.label ?? kind;
}
