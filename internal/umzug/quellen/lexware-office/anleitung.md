## Vorher wissen

- Am besten kommst du mit zwei Exporten an deine Daten: dem DATEV-Export „Buchungsstapel und Belegbilder“ und dem PDF-Sammelexport deiner Belege.
- Belege mit dem Status „Zu prüfen“ oder „Entwurf“ exportiert Lexware Office nicht. Schließe sie vorher ab.
- Lexware rät, nicht ein ganzes Jahr auf einmal zu exportieren. Teile den Zeitraum zum Beispiel in Quartale.
- Die Public API gibt es bei Lexware Office nur im Tarif XL. papierleicht nutzt sie noch nicht. Bankumsätze und Excel-Dateien liest die App ebenfalls noch nicht.
- Nach der Kündigung hast du laut Lexware zehn Jahre lang Lesezugriff auf deine Daten, vorerst kostenlos. Exportiere trotzdem vorher alles.
- Rechnungen und Belege musst du in der Regel acht Jahre aufbewahren, Bücher und Abschlüsse zehn Jahre.

## In Lexware Office

**DATEV-Export mit Belegen**

1. Öffne Einstellungen › Mein Steuerberater und wähle „Ich nutze keine direkte Anbindung“. Arbeitet deine Steuerberatung über eine direkte Anbindung, sprich diesen Schritt vorher mit ihr ab.
2. Öffne Einstellungen › Export, Reiter „Meine Lexware Office-Daten herunterladen“.
3. Wähle die Exportart mit „Buchungsstapel und Belegbilder“.
4. Wähle den Zeitraum und klicke „Export starten“. Die ZIP-Datei findest du danach in der Export-Übersicht.
5. Wiederhole das für alle Zeiträume, die du mitnehmen willst.

**PDF-Sammelexport**

1. Wähle im selben Reiter unter den weiteren Exporten „Belege“.
2. Filtere nach Eingang oder Ausgang und nach Rechnungs- oder Zahldatum.
3. Speichere das Ergebnis, zum Beispiel in einem Ordner `Lexware-Belege`.

**Nur für abgeschlossene Jahre: Z3-Export für die Betriebsprüfung**

- Im selben Reiter findest du unter „Vordefinierter Zeitraum“ den Export „Daten für die Betriebsprüfung durch das Finanzamt (Z3- Export)“. Er enthält alle Belege, das Journal, den Kontenplan, Debitoren und Kreditoren und die Ausgangsrechnungen mit Positionen.
- **Achtung: Dieser Export schreibt die Buchungen des gewählten Zeitraums fest.** Danach kannst du sie in Lexware Office nicht mehr ändern. Nutze ihn deshalb nur für Jahre, die vollständig abgeschlossen sind.
- Mit Protokollierung kann der Export laut Lexware Stunden dauern.

## In papierleicht

1. Klicke auf **Dateien oder ZIP** und wähle die DATEV-ZIP-Dateien. Die App erkennt den DATEV-Export. Den Buchungsstapel legt sie als **DATEV-Export** ab. Belege aus einem DATEV-Belegpaket (mit einer Datei `document.xml`) übernimmt sie mit Belegnummer, Datum und Betrag, so wie sie im Export stehen.
2. Übernimm danach den PDF-Sammelexport mit **Ordner übernehmen** oder **Dateien oder ZIP**. E-Rechnungen (ZUGFeRD, XRechnung) liest die App mit Nummer, Datum und Betrag. Einfache PDFs übernimmt sie unverändert als Datei.
3. Hast du einen Z3-Export, wähle ihn genauso. Die App erkennt ihn an der Datei `index.xml` und legt ihn als Ganzes ab. Enthält er Debitoren, bietet die App danach **Kunden aus dem Export prüfen** an. Du siehst jede Zeile, bevor etwas gespeichert wird.
4. Mit **Übernehmen** landet alles im **Altbestand**: unverändert, durchsuchbar und in jeder Sicherung. In deine Zahlen fließt der Altbestand nicht ein.

## Prüfen

- Vergleiche unter **Altbestand** die Anzahl der Belege je Jahr mit Lexware Office. Bei E-Rechnungen zeigt die App auch die Summen je Jahr.
- Fehlt ein Zeitraum, exportiere ihn nach und übernimm ihn noch einmal. Doppelte Dateien erkennt die App und überspringt sie.

## Vor der Kündigung

- [ ] Für jeden Zeitraum liegt der DATEV-Export mit Belegbildern im Altbestand.
- [ ] Der PDF-Sammelexport für Eingang und Ausgang ist übernommen.
- [ ] Belege mit „Zu prüfen“ oder „Entwurf“ sind abgeschlossen oder gesichert.
- [ ] Kunden sind übernommen oder als Liste gesichert.
- [ ] Die ZIP-Dateien aus Lexware Office bewahrst du zusätzlich auf.
- [ ] Kontoauszüge sind bei deiner Bank gesichert, denn Bankumsätze übernimmt die App noch nicht.
- [ ] Du hast eine Sicherung von papierleicht gemacht (Einstellungen › Allgemein › Sicherung exportieren).
