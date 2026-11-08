/** Reasons offered when an unsent invoice is deleted; the log keeps the text. */
export const deletionReasons = [
  "Tippfehler oder falsche Angaben",
  "Falscher Kunde",
  "Versehentlich doppelt ausgestellt",
  "Anderer Grund",
] as const;

/** The reason text for the log: the choice, with details if given. */
export function deletionReason(reason: string, details: string): string {
  const extra = details.trim();
  if (reason === "Anderer Grund") return extra;
  if (!reason) return "";
  return extra ? `${reason}: ${extra}` : reason;
}
