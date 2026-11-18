## Vorher wissen

- easybill gibt deine Rechnungen über die Auswertung „Rechnungsausgangsbuch“ als ZIP mit Einzeldokumenten heraus. Kontakte exportierst du als CSV.
- Fertige Exporte legt easybill unter „Hintergrundprozesse“ ab. Dort lädst du sie herunter.
- Eine direkte Verbindung zu easybill (API) hat papierleicht noch nicht. Excel-Dateien liest die App ebenfalls noch nicht.
- Rechnungen und Belege musst du in der Regel acht Jahre aufbewahren, Bücher und Abschlüsse zehn Jahre.

## In easybill

1. **Rechnungen:** Öffne Buchhaltung & Exporte › Berichte › Auswertungen. Setze die Filter, zum Beispiel ein Jahr, und klicke „Auswertung starten“.
2. Exportiere das Ergebnis als **ZIP mit Einzeldokumenten**. Lade die Datei danach unter „Hintergrundprozesse“ herunter.
3. **Kontakte:** Öffne Kontakte › Exportieren und wähle die Felder. Die CSV-Datei liegt danach ebenfalls unter „Hintergrundprozesse“.
4. **DATEV (zusätzlich):** Unter Buchhaltung & Exporte › Berichte › Export kannst du DATEV-Dateien erzeugen, zum Beispiel „DATEV XML online“ mit Belegsätzen.
5. Wiederhole die Exporte für jedes Jahr, das du mitnehmen willst.

## In papierleicht

1. Klicke auf **Dateien oder ZIP** und wähle die ZIP-Dateien aus easybill.
2. E-Rechnungen (ZUGFeRD, XRechnung) liest die App mit Nummer, Datum und Betrag. Einfache PDFs übernimmt sie unverändert als Datei. Ein DATEV-Belegpaket mit einer Datei `document.xml` übernimmt sie mit Belegnummer, Datum und Betrag.
3. Mit **Übernehmen** landet alles im **Altbestand**: unverändert, durchsuchbar und in jeder Sicherung. In deine Zahlen fließt der Altbestand nicht ein.
4. Die Kontakte übernimmst du mit **Kunden aus Liste**. Du siehst vorher jede Zeile und bestätigst, was übernommen wird.

## Prüfen

- Vergleiche unter **Altbestand** die Anzahl der Rechnungen je Jahr mit dem Rechnungsausgangsbuch. Bei E-Rechnungen zeigt die App auch die Summen je Jahr.
- Fehlt etwas, exportiere es nach und übernimm es noch einmal. Doppelte Dateien erkennt die App und überspringt sie.

## Vor der Kündigung

- [ ] Für jedes Jahr liegt das Rechnungsausgangsbuch mit Einzeldokumenten im Altbestand.
- [ ] Eingangsbelege sind gesichert und übernommen.
- [ ] Kontakte sind übernommen.
- [ ] Die Exportdateien bewahrst du zusätzlich auf.
- [ ] Zugangsschlüssel (API-Schlüssel) in easybill sind gelöscht, falls du welche angelegt hast.
- [ ] Du hast eine Sicherung von papierleicht gemacht (Einstellungen › Allgemein › Sicherung exportieren).
