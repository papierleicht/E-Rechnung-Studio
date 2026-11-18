## Vorher wissen

- Der beste Weg aus sevdesk ist der DATEV-Export „Buchungsdaten & Belegbilder“ als Datei. Er enthält Buchungen, Geschäftspartner und die Belegbilder für Ausgangs- und Eingangsrechnungen.
- Nach der Kündigung bleiben deine Daten laut sevdesk bis zu drei Monate erhalten. Exportiere sie trotzdem vorher. Kündigen musst du laut sevdesk spätestens 14 Tage vor Ende der Laufzeit.
- Rechnungen und Belege musst du in der Regel acht Jahre aufbewahren, Bücher und Abschlüsse zehn Jahre. Das gilt auch nach der Kündigung.
- Eine direkte Verbindung zu sevdesk (API) hat papierleicht noch nicht. Bankumsätze übernimmt die App ebenfalls noch nicht.
- Am einfachsten ist ein Wechsel zum Jahreswechsel: Das alte Jahr schließt du in sevdesk ab, das neue beginnt in papierleicht.

## In sevdesk

1. Öffne Steuern › Mein Steuerberater.
2. Wähle „Buchungsdaten & Belegbilder“.
3. Wähle als Übertragungsart „Datei-Export“.
4. Trage Berater- und Mandantennummer, den Beginn des Wirtschaftsjahres und den Zeitraum ein. Setze den Haken bei Belegbilder und wähle, ob festgeschriebene oder offene Daten exportiert werden.
5. Speichere die ZIP-Datei. Sie enthält eine Buchungsstapel-CSV, eine Stammdaten-CSV und je eine ZIP mit Belegbildern für Rechnungsausgang und Rechnungseingang.
6. Wiederhole den Export für jeden Zeitraum, den du mitnehmen willst.

## In papierleicht

1. Klicke auf **Dateien oder ZIP** und wähle die ZIP-Datei aus sevdesk. Die inneren ZIP-Dateien mit den Belegbildern liest die App mit.
2. Die App erkennt den DATEV-Export. Buchungsstapel und Stammdaten legt sie als **DATEV-Export** ab. Belege aus einem DATEV-Belegpaket (mit einer Datei `document.xml`) übernimmt sie mit Belegnummer, Datum und Betrag, so wie sie im Export stehen. Andere Belegbilder übernimmt sie unverändert als Datei.
3. Mit **Übernehmen** landen die Dateien im **Altbestand**. Dort bleiben sie unverändert, sind durchsuchbar und in jeder Sicherung enthalten. In deine Zahlen, etwa Umsatzsteuer und Ergebnis, fließen sie nicht ein.
4. Enthält der Export Debitoren, bietet die App danach **Kunden aus dem Export prüfen** an. Du siehst jede Zeile, bevor etwas gespeichert wird.

## Prüfen

- Vergleiche unter **Altbestand** die Anzahl der Belege je Jahr mit sevdesk. Bei E-Rechnungen zeigt die App auch die Summen je Jahr.
- Fehlt ein Zeitraum, exportiere ihn in sevdesk nach und übernimm die ZIP-Datei noch einmal. Doppelte Dateien erkennt die App und überspringt sie.

## Vor der Kündigung

- [ ] Für jeden Zeitraum liegt der DATEV-Export mit Belegbildern im Altbestand.
- [ ] Die ZIP-Dateien aus sevdesk bewahrst du zusätzlich auf.
- [ ] Kunden sind übernommen.
- [ ] Kontoauszüge sind bei deiner Bank gesichert, denn Bankumsätze übernimmt die App noch nicht.
- [ ] Du kündigst spätestens 14 Tage vor Ende der Laufzeit.
- [ ] Du hast eine Sicherung von papierleicht gemacht (Einstellungen › Allgemein › Sicherung exportieren).
