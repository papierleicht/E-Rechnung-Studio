import type { Company } from "../../bindings/papierleicht.de/e-rechnung-studio/internal/model/models";
import { companyTaxTreatment, chooseCompanyTaxStatus } from "./tax.ts";

export function setupMissing(company: Company): { step: number; message: string }[] {
  const missing = [];
  if (
    !company.name.trim() ||
    !company.address.street.trim() ||
    !company.address.city.trim() ||
    !company.address.postcode.trim()
  )
    missing.push({
      step: 0,
      message: "Trage deinen Unternehmensnamen und deine vollständige Anschrift ein.",
    });
  if (!/^[A-Z]{2}$/i.test(company.address.country.trim()))
    missing.push({ step: 0, message: "Trage ein zweistelliges Länderkürzel ein, z. B. DE." });
  if (!company.email.trim()) missing.push({ step: 1, message: "Trage deine Kontakt-E-Mail ein." });
  if (!company.vatId.trim() && !company.taxNumber.trim())
    missing.push({ step: 2, message: "Trage deine Steuernummer oder USt-IdNr. ein." });
  if (companyTaxTreatment(company) === "intra-community" && !company.vatId.trim())
    missing.push({
      step: 2,
      message: "Trage deine USt-IdNr. ein. Für innergemeinschaftliche Lieferungen brauchst du sie.",
    });
  if (["58", "30"].includes(company.paymentMeansCode || "58") && !company.iban.trim())
    missing.push({
      step: 4,
      message: "Trage deine IBAN ein, damit Kunden per Überweisung zahlen können.",
    });
  return missing;
}

/** The tax profile asked during the setup; saved with the finance settings. */
export interface SetupTaxProfile {
  activity: string;
  filing: string;
  vatPeriod: string;
  taxBasis: string;
  vatExtension: boolean;
}

export function chooseSetupTax(company: Company, value: string) {
  chooseCompanyTaxStatus(company, value);
}
