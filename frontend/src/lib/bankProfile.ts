import type { Company } from "../../bindings/papierleicht.de/e-rechnung-studio/internal/model/models";
import { lookupGermanBank, normalizeIBAN } from "./bankLookup.ts";
import { bankPresetForIBAN, presetBankCode } from "./bankPresets.ts";

export interface ProfileBank {
  iban: string;
  bankCode: string;
  label: string;
}

export function connectionDefaultsFromProfile(company: Pick<Company, "iban" | "bankName">) {
  const profile = bankFromCompanyProfile(company);
  const preset = bankPresetForIBAN(profile.iban);
  return {
    label: profile.label || preset?.name || "",
    bankCode: preset ? presetBankCode(preset, profile.bankCode) : profile.bankCode,
    endpoint: preset?.endpoint ?? "",
    securityFunction: preset?.defaultTAN ?? "",
  };
}

export function bankFromCompanyProfile(company: Pick<Company, "iban" | "bankName">): ProfileBank {
  const iban = normalizeIBAN(company.iban ?? "");
  const directoryBank = lookupGermanBank(iban);
  return {
    iban,
    bankCode: directoryBank ? iban.slice(4, 12) : "",
    label: company.bankName?.trim() || directoryBank?.name || "",
  };
}
