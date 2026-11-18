## Vorher wissen

- Haben ist Open Source und läuft auf deinem eigenen Server. Deine Daten liegen also bei dir, solange du die Instanz betreibst.
- Der Weg aus Haben ist der **Jahresexport** als ZIP-Datei (`Haben-<Jahr>-<Datum>.zip`). Er enthält die festgeschriebenen Rechnungen als ZUGFeRD-PDF oder XRechnung-XML, CSV-Dateien für Rechnungen, Belege, Journal und Kontakte und eine Datei `index.xml` für die Betriebsprüfung.
- Die Kontaktliste `stammdaten/kontakte.csv` enthält alle Kontakte, auch Lieferanten, denn Haben unterscheidet die beiden nicht. Deinen Artikelkatalog gibt Haben im Jahresexport nicht heraus.
- Wo der Jahresexport in deiner Version genau liegt, haben wir nicht an einer eigenen Instanz geprüft. Die Menünamen können abweichen.
- Eine direkte Verbindung zu Haben (API) hat papierleicht noch nicht. Bankumsätze übernimmt die App ebenfalls noch nicht.
- Rechnungen und Belege musst du in der Regel acht Jahre aufbewahren, Bücher und Abschlüsse zehn Jahre.

## In Haben

1. Erzeuge den Jahresexport für ein Jahr und speichere die ZIP-Datei.
2. Wiederhole das für jedes Jahr, das du mitnehmen willst.
3. Lege alle ZIP-Dateien in einen Ordner, zum Beispiel `Haben-Export`.

## In papierleicht

1. Klicke auf **Dateien oder ZIP** und wähle die ZIP-Dateien. Mit **Ordner übernehmen** wählst du den ganzen Ordner.
2. Die App erkennt den Prüfer-Export an der Datei `index.xml` und legt ihn als Ganzes ab. Die Rechnungen aus `rechnungen/` übernimmt sie einzeln mit Nummer, Datum und Betrag aus der E-Rechnung.
3. Mit **Übernehmen** landet alles im **Altbestand**: unverändert, durchsuchbar und in jeder Sicherung. In deine Zahlen fließt der Altbestand nicht ein.
4. Danach bietet die App **Kunden aus dem Export prüfen** für `stammdaten/kontakte.csv` an. Du siehst jede Zeile, bevor etwas gespeichert wird. Lieferanten landen dabei ebenfalls unter Kunden; lösche sie danach unter **Kunden**. Nimm die Kontakte aus dem jüngsten Jahr, denn jedes ZIP enthält alle.

## Prüfen

- Vergleiche unter **Altbestand** die Anzahl der Belege je Jahr mit Haben.
- Fehlt ein Jahr, exportiere es nach und übernimm es noch einmal. Doppelte Dateien erkennt die App und überspringt sie.

## Vor der Kündigung

Haben hat kein Abo. Die Punkte gelten, bevor du deine Instanz abschaltest.

- [ ] Für jedes Jahr liegt der Jahresexport im Altbestand.
- [ ] Die ZIP-Dateien bewahrst du zusätzlich auf.
- [ ] Du hast eine Datensicherung deiner Haben-Instanz aufbewahrt.
- [ ] Du hast eine Sicherung von papierleicht gemacht (Einstellungen › Allgemein › Sicherung exportieren).
