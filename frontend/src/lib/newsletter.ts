/** A plain address the server accepts: no display name, a dot in the domain. */
export function validNewsletterEmail(value: string): boolean {
  return /^[^\s@<>"]+@[^\s@<>"]+\.[^\s@<>".]+$/.test(value.trim());
}

/** "2026-11-02T09:30:00Z" as the local date "02.11.2026". */
export function newsletterDate(value: string): string {
  const date = new Date(value);
  if (!value || Number.isNaN(date.getTime())) return "einem früheren Tag";
  return date.toLocaleDateString("de-DE", { day: "2-digit", month: "2-digit", year: "numeric" });
}
