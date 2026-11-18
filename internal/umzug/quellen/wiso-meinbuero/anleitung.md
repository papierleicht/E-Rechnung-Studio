## Vorher wissen

- WISO MeinBüro gibt es im Browser (Web) und als Programm für Windows (Desktop). Für beide steht unten ein eigener Weg.
- Im Web ist der **Steuerberaterexport** der beste Weg. Er liefert eine ZIP-Datei mit Buchungsstapel, Stammdaten und einer inneren ZIP mit allen Belegen als PDF.
- Für die Desktop-Version kennen wir nur ein älteres Handbuch (Version 2017). Die Menünamen können in deiner Version abweichen.
- Eine direkte Verbindung zu MeinBüro (API) hat papierleicht noch nicht. Listen liest die App als CSV oder Excel-Datei (.xlsx).
- Rechnungen und Belege musst du in der Regel acht Jahre aufbewahren, Bücher und Abschlüsse zehn Jahre.

## In WISO MeinBüro

**Web: Steuerberaterexport**

1. Öffne Buchhaltung › Steuerberaterexport.
2. Wähle die Buchungssätze: Rechnungen, Ausgaben & Sonstige Einnahmen, Zahlungen und Stammdaten. Setze den Haken bei „Belege als PDF“.
3. Trage den Zeitraum, Berater- und Mandantennummer und den Beginn des Wirtschaftsjahres ein und klicke „Erstellen“.
4. Lade die ZIP-Datei unter „Erzeugte Exporte“ herunter.
5. Wiederhole das für jeden Zeitraum, den du mitnehmen willst.

**Desktop (Windows)**

1. **Kunden und Artikel:** Datei › Daten-Export. Der Assistent exportiert unter anderem Artikel, Kunden und Lieferanten. Wähle als Format TXT oder eine Excel-Datei (.xlsx), denn beides liest papierleicht.
2. **Rechnungen als PDF:** Ist das Rechnungsarchiv eingeschaltet (Datei › Einstellungen › Grundeinstellungen), liegen die PDFs in `C:\ProgramData\Buhl Data Service GmbH\Mein Büro\Archiv`. Sonst druckst du die markierten Rechnungen mit „Microsoft Print to PDF“. Windows fragt dann jeden Dateinamen einzeln ab.
3. **GoBD-Dateien:** Finanzen › GoBD-Ausgabedateien erzeugen. Die CSV-Dateien liegen danach in `…\Dokumente\Buhl Data Service GmbH\Mein Büro\FibuExport\`. Bewahre sie zusätzlich auf.

## In papierleicht

1. Klicke auf **Dateien oder ZIP** und wähle die ZIP-Datei aus dem Steuerberaterexport. Die innere ZIP mit den Belegen liest die App mit. Unter Desktop wählst du mit **Ordner übernehmen** den Archiv-Ordner mit den PDFs.
2. Erkennt die App einen Buchungsstapel im DATEV-Format, legt sie ihn als **DATEV-Export** ab. E-Rechnungen (ZUGFeRD, XRechnung) liest sie mit Nummer, Datum und Betrag. Einfache PDFs übernimmt sie unverändert als Datei.
3. Mit **Übernehmen** landet alles im **Altbestand**: unverändert, durchsuchbar und in jeder Sicherung. In deine Zahlen fließt der Altbestand nicht ein.
4. Enthält der Export Debitoren oder eine Kundentabelle, bietet die App danach **Kunden aus dem Export prüfen** an. Du siehst jede Zeile, bevor etwas gespeichert wird.
5. Listen aus dem Daten-Export der Desktop-Version übernimmst du mit **Kunden aus Liste** bzw. **Artikel aus Liste**.

## Prüfen

- Vergleiche unter **Altbestand** die Anzahl der Rechnungen und Belege je Jahr mit MeinBüro. Bei E-Rechnungen zeigt die App auch die Summen je Jahr.
- Fehlt etwas, exportiere es nach und übernimm es noch einmal. Doppelte Dateien erkennt die App und überspringt sie.

## Vor der Kündigung

- [ ] Für jeden Zeitraum liegt der Steuerberaterexport im Altbestand.
- [ ] Die ZIP-Dateien und, bei Desktop, die GoBD-Dateien bewahrst du zusätzlich auf.
- [ ] Kunden und Artikel sind übernommen oder als Liste gesichert.
- [ ] Du hast eine Sicherung von papierleicht gemacht (Einstellungen › Allgemein › Sicherung exportieren).
