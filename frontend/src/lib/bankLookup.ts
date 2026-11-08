import { germanBanks } from "./bankDirectory.generated.ts";

export interface BankDetails {
  name: string;
  bic: string;
}

export function normalizeIBAN(value: string): string {
  return value.replace(/\s/g, "").toUpperCase();
}

function hasValidChecksum(iban: string): boolean {
  const rearranged = iban.slice(4) + iban.slice(0, 4);
  let remainder = 0;
  for (const character of rearranged) {
    const numeric =
      character >= "A" && character <= "Z" ? String(character.charCodeAt(0) - 55) : character;
    for (const digit of numeric) remainder = (remainder * 10 + Number(digit)) % 97;
  }
  return remainder === 1;
}

export function lookupGermanBank(value: string): BankDetails | undefined {
  const iban = normalizeIBAN(value);
  if (!/^DE\d{20}$/.test(iban) || !hasValidChecksum(iban)) return undefined;
  const bank = germanBanks[iban.slice(4, 12)];
  return bank ? { name: bank[0], bic: bank[1] } : undefined;
}
