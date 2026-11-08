// Decimal arithmetic keeps currency rounding independent of binary floating point.
function decimal(value: string) {
  const text = value.trim();
  // "1.234,56" groups thousands; a lone dot stays decimal because stored prices use it.
  const normalized = (
    /^\d{1,3}(\.\d{3})+,\d*$/.test(text) ? text.replaceAll(".", "") : text
  ).replace(",", ".");
  if (!/^\d+(\.\d*)?$/.test(normalized) || normalized.length > 100) return null;
  const [whole, fraction = ""] = normalized.split(".");
  return { value: BigInt(whole + fraction), scale: 10n ** BigInt(fraction.length) };
}

function rounded(numerator: bigint, denominator: bigint, places: number) {
  const scale = 10n ** BigInt(places);
  const value = (numerator * scale * 2n + denominator) / (denominator * 2n);
  const digits = value.toString().padStart(places + 1, "0");
  return `${digits.slice(0, -places)}.${digits.slice(-places)}`;
}

/** Reads a typed "1.500" or "1.500,00" like the Go side; "0.125" keeps its decimal dot. */
export function typedPrice(value: string): string {
  const text = value.trim();
  return /^[1-9]\d{0,2}(\.\d{3})+(,\d*)?$/.test(text) ? text.replaceAll(".", "") : text;
}

/** Commercial cent rounding for displayed and explicitly entered prices. */
export function roundPrice(value: string): string {
  const price = decimal(value);
  return price ? rounded(price.value, price.scale, 2).replace(".", ",") : "";
}

export function grossUnitPrice(net: string, rate: string): string {
  const price = decimal(net),
    tax = decimal(rate);
  if (!price || !tax) return "";
  return rounded(
    price.value * (100n * tax.scale + tax.value),
    price.scale * 100n * tax.scale,
    2,
  ).replace(".", ",");
}

export function netUnitPrice(gross: string, rate: string): string {
  const price = decimal(gross),
    tax = decimal(rate);
  if (!price || !tax) return "";
  // Preserve fractional cents: rounding the unit net to cents distorts larger quantities.
  return rounded(price.value * 100n * tax.scale, price.scale * (100n * tax.scale + tax.value), 10)
    .replace(/0+$/, "")
    .replace(/\.$/, "");
}
