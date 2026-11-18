## Vorher wissen

- Aus MonKey Office nimmst du den DATEV-Export mit Anhängen und die Adressen als Textdatei mit. Dazu kommt der GoBD-Export für die Betriebsprüfung.
- Den GoBD-Export gibt es laut Hersteller nur für festgeschriebene Buchungen.
- Bankumsätze übernimmt papierleicht noch nicht.
- Rechnungen und Belege musst du in der Regel acht Jahre aufbewahren, Bücher und Abschlüsse zehn Jahre.

## In MonKey Office

1. **DATEV-Export:** Öffne Import & Export › Export DATEV. Wähle den Bereich und setze den Haken bei „Anhänge Exportieren“. Du erhältst `EXTF_Buchungsstapel_01.csv` und `Dokumente_ Buchungsstapel_01.zip`.
2. Schon exportierte Buchungen tragen in MonKey Office ein Kennzeichen. Meldet der Export „0 Buchungen“, setze das Kennzeichen zurück und exportiere noch einmal.
3. **Adressen:** Öffne Import & Export › Export Textdateien und lege über „+“ eine Exportdefinition an. Wähle den Bereich Adressen, Feldnamen, ein Trennzeichen und den Zeichensatz UTF-8.
4. **GoBD-Export:** Öffne Import & Export › Export GoBD. Das geht nur mit festgeschriebenen Buchungen.
5. **Einzelne Belege als PDF:** Aktion › Ausgeben › PDF.

## In papierleicht

1. Klicke auf **Dateien oder ZIP** und wähle die Buchungsstapel-Datei und die ZIP mit den Dokumenten.
2. Die App erkennt den DATEV-Export und legt den Buchungsstapel als **DATEV-Export** ab. Die Dokumente übernimmt sie unverändert als Datei. E-Rechnungen (ZUGFeRD, XRechnung) liest sie mit Nummer, Datum und Betrag.
3. Einen GoBD-Export mit der Datei `index.xml` erkennt die App als Export für die Betriebsprüfung und legt ihn als Ganzes ab.
4. Mit **Übernehmen** landet alles im **Altbestand**: unverändert, durchsuchbar und in jeder Sicherung. In deine Zahlen fließt der Altbestand nicht ein.
5. Die Adressen übernimmst du mit **Kunden aus Liste**. Du siehst vorher jede Zeile und bestätigst, was übernommen wird.

## Prüfen

- Vergleiche unter **Altbestand** die Anzahl der Belege je Jahr mit MonKey Office.
- Fehlt etwas, exportiere es nach und übernimm es noch einmal. Doppelte Dateien erkennt die App und überspringt sie.

## Vor der Kündigung

- [ ] Für jedes Jahr liegt der DATEV-Export mit Dokumenten im Altbestand.
- [ ] Der GoBD-Export ist zusätzlich abgelegt.
- [ ] Die Adressen sind übernommen.
- [ ] Du bewahrst die Daten von MonKey Office zusätzlich auf.
- [ ] Du hast eine Sicherung von papierleicht gemacht (Einstellungen › Allgemein › Sicherung exportieren).
