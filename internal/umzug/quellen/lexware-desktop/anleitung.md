## Vorher wissen

- Diese Anleitung gilt für die Windows-Programme Lexware faktura+auftrag und Lexware buchhaltung. Für Lexware Office im Browser gibt es eine eigene Anleitung.
- Ab Version 2026 stellt Lexware die Datenbank auf PostgreSQL um. Laut Lexware muss die Umstellung innerhalb von 2027 erfolgen.
- Läuft dein Abo aus, bleiben die Daten laut Lexware erhalten, du kannst sie aber nicht mehr ändern. Ob dann noch ein Export geht, ist nicht bestätigt. Exportiere deshalb vor dem Ende des Abos.
- Listen liest papierleicht als CSV oder als Excel-Datei (.xlsx); ältere Excel-Dateien (.xls) speicherst du vorher als .xlsx oder CSV. Bankumsätze übernimmt die App noch nicht.
- Rechnungen und Belege musst du in der Regel acht Jahre aufbewahren, Bücher und Abschlüsse zehn Jahre.

## In Lexware faktura+auftrag

1. **Kunden und Artikel:** Öffne Datei › Export › Text/ASCII… (in der FAQ „Datei – Export – ASCII“). Wähle je Durchgang nur eine Datenquelle, also erst Kunden, dann Artikel.
2. Stelle ein: Dateiart „ANSI (Windows)“, alle Felder, Trennzeichen `;`, Textbegrenzer `"` und den Haken bei „Feldnamen in den ersten Datensatz“. Du erhältst eine `.txt`-Datei. Ansprechpartner exportiert Lexware dabei nicht.
3. **Belege:** Öffne Bearbeiten › Dokumente verwalten bzw. den Belegviewer und speichere jeden Beleg mit „Kopie speichern“ als PDF oder XRechnung-XML. Ob es einen Massenexport als PDF gibt, ist nicht bestätigt.
4. **Betriebsprüfung:** Datei › Export › Daten Betriebsprüfung… erzeugt einen Export im GDPdU-Format. Für eingeschränkte Benutzer ist er nicht verfügbar.

## In Lexware buchhaltung

1. **DATEV-Export:** In der Version plus öffnest du Datei › Export › DATEV, in pro Datei › DATEV-Schnittstelle › DATEV-Export.
2. Wähle nacheinander „Buchungsdaten“ und „Kontendaten Kunden und Lieferanten“, jeweils mit „DATEV-Format (ASCII csv)“, Zeitraum und Festschreibungskennzeichen. Du erhältst `EXTF_Buchungsstapel_*.csv` und `EXTF_GP_Stamm_*.csv`. Belege sind dabei **nicht** enthalten.
3. **Betriebsprüfung:** Öffne Datei › Export › Daten Betriebsprüfung. Wähle Daten und Wirtschaftsjahre, ein Verzeichnis und auf Wunsch eine ZIP-Datei. Das geht nur, wenn alle Buchungen festgeschrieben sind. Im Unterordner „Belege“ liegt nur der erste Beleg je Buchung.

## In papierleicht

1. Klicke auf **Dateien oder ZIP** und wähle die DATEV-Dateien. Die App erkennt den Buchungsstapel und die Geschäftspartner-Datei und legt beide als **DATEV-Export** ab.
2. Den Export für die Betriebsprüfung wählst du mit **Ordner übernehmen** oder als ZIP. Die App erkennt ihn an der Datei `index.xml` und legt ihn als Ganzes ab.
3. Belege, die du als PDF oder XRechnung gespeichert hast, übernimmst du mit **Ordner übernehmen**. E-Rechnungen liest die App mit Nummer, Datum und Betrag, einfache PDFs übernimmt sie unverändert als Datei.
4. Mit **Übernehmen** landet alles im **Altbestand**: unverändert, durchsuchbar und in jeder Sicherung. In deine Zahlen fließt der Altbestand nicht ein.
5. Enthält ein Export Debitoren, bietet die App danach **Kunden aus dem Export prüfen** an. Du siehst jede Zeile, bevor etwas gespeichert wird.
6. Die Textdateien aus faktura+auftrag übernimmst du mit **Kunden aus Liste** bzw. **Artikel aus Liste**. ANSI-Dateien liest die App. Die Spalten von Lexware (zum Beispiel Firma, Name, Vorname, HausNr, EG ID, Preis 1) ordnet sie von selbst zu, wenn du beim Export die Feldnamen mitgegeben hast. Als Preis nimmt sie „Preis 1“; ist er bei dir ein Bruttopreis (Spalte „B/N“), ändere ihn danach im Artikel.

## Prüfen

- Vergleiche unter **Altbestand** die Anzahl der Belege je Jahr mit Lexware. Bei E-Rechnungen zeigt die App auch die Summen je Jahr.
- Prüfe bei übernommenen Kunden die Ansprechpartner. Lexware exportiert sie nicht mit.
- Fehlt etwas, exportiere es nach und übernimm es noch einmal. Doppelte Dateien erkennt die App und überspringt sie.

## Vor der Kündigung

- [ ] DATEV-Export und Export für die Betriebsprüfung liegen für jedes Jahr im Altbestand.
- [ ] Belege sind als PDF oder XRechnung gesichert und übernommen.
- [ ] Kunden und Artikel sind übernommen.
- [ ] Die Exportdateien bewahrst du zusätzlich auf.
- [ ] Du hast eine Sicherung von papierleicht gemacht (Einstellungen › Allgemein › Sicherung exportieren).
