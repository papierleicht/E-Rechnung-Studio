/** Why the updater cannot replace the app where it runs (InstallLocation.problem). */
export function installProblemText(problem: string): string {
  switch (problem) {
    case "translocated":
      return "Die App läuft noch aus dem Download-Ordner. macOS startet sie dann aus einer schreibgeschützten Kopie, und Updates lassen sich nicht installieren.";
    case "diskImage":
      return "Die App läuft direkt aus dem Installationsabbild. Dort lassen sich Updates nicht installieren.";
    case "temporary":
      return "Die App läuft aus einem temporären Ordner, zum Beispiel direkt aus der ZIP-Datei. Updates gingen dort verloren.";
    default:
      return "Die App darf ihren Ordner nicht ändern. Updates lassen sich dort nicht installieren.";
  }
}

/** A short name for the target folder: the programs folder on every system. */
export function installTargetName(target: string): string {
  const parts = target.split(/[\\/]/).filter(Boolean);
  if (parts.includes("Applications") || parts.includes("Programs")) return "Programme";
  return parts.at(-2) ?? target;
}
