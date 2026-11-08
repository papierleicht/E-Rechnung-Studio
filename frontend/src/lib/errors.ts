/** Preserve actionable German service errors and keep technical details out of the primary UI. */
export function errorDetail(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}

const actionable = new RegExp(
  "^(?:Bitte |Die |Der |Das |Dieser |Diese |Dieses |Dateien |PDF |Original |Keine |Ein |Eine |Löschen |Nur |Unbekannte?r? |" +
    "Du |Dein(?:e|en|em|er|es)? |" +
    "(?:Gib|Wähle|Prüfe|Lege|Trage|Versuche|Starte|Öffne|Melde|Verwende|Lade|Warte|Gehe|Nimm|Erfasse|Ordne|Hebe|" +
    "Entferne|Lösche|Korrigiere|Aktualisiere|Wiederhole|Kontrolliere|Beende|Setze|Sende|Importiere|Exportiere|" +
    "Verbinde|Nenne|Schreibe|Passe|Beachte|Achte|Sieh|Stimme|Überprüfe|Suche|Ändere|Vergib|Speichere|Stelle|" +
    "Bestätige|Richte|Fülle|Füge|Erstelle|Ergänze|Schließe|Lass|Mach|Halte|Gleiche|Kopiere|Klicke|Scrolle) )",
);

export function serviceErrorMessage(error: unknown, fallback: string): string {
  const detail = errorDetail(error).trim();
  if (detail.includes("readonly database"))
    return "Die Nur-Lese-Ansicht ist aktiv. Beende sie, um wieder etwas zu ändern.";
  // Wrapped storage/OS/parser errors are retained in technical details, not
  // appended in English to the actionable German headline.
  const message = detail.split(/: (?=\S)/)[0];
  return actionable.test(detail) ? (detail.includes("„") ? detail : message) : fallback;
}
