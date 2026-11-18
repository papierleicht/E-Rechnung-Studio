## Vorher wissen

- Mit dem Tarif XL verbindet sich papierleicht direkt mit Lexware Office und holt Kunden, Rechnungen, Gutschriften, Abschlagsrechnungen und Belege mit ihren Dateien. Das ist der einfachste Weg.
- Ohne XL kommst du mit zwei Exporten an deine Daten: dem DATEV-Export „Buchungsstapel und Belegbilder“ und dem PDF-Sammelexport deiner Belege. Den DATEV-Export brauchst du auch mit XL, wenn du die Buchungen mitnehmen willst.
- Belege mit dem Status „Zu prüfen“ oder „Entwurf“ exportiert Lexware Office nicht. Schließe sie vorher ab.
- Lexware rät, nicht ein ganzes Jahr auf einmal zu exportieren. Teile den Zeitraum zum Beispiel in Quartale.
- Bankumsätze übernimmt die App noch nicht.
- Nach der Kündigung hast du laut Lexware zehn Jahre lang Lesezugriff auf deine Daten, vorerst kostenlos. Exportiere trotzdem vorher alles.
- Rechnungen und Belege musst du in der Regel acht Jahre aufbewahren, Bücher und Abschlüsse zehn Jahre.

## In Lexware Office

**Nur mit XL: API-Schlüssel für die direkte Verbindung**

1. Öffne Erweiterungen › „Weitere Apps“ › Public API und klicke „API-Schlüssel erstellen“.
2. Kopiere den Schlüssel. Lexware zeigt ihn nur einmal an.

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

**Mit XL: direkt verbinden**

1. Klicke auf **Direkt verbinden**, füge den Schlüssel ein und klicke **Verbinden und abrufen**. papierleicht liest nur. Den Schlüssel hält die App nur für diesen Abruf und speichert ihn nicht.
2. Die App holt Rechnungen, Gutschriften, Abschlagsrechnungen und Belege aus der Buchhaltung mit ihren Dateien. Eine XRechnung speichert sie als XML und dazu die Ansicht von Lexware als PDF. Entwürfe, Angebote, Auftragsbestätigungen und Lieferscheine bleiben in Lexware.
3. Lexware erlaubt nur zwei Abrufe pro Sekunde. Bei vielen Belegen dauert es deshalb eine Weile. Brichst du ab, holt ein neuer Abruf nur den Rest.
4. Danach klickst du auf **Kunden prüfen**. Du siehst jede Zeile, bevor etwas gespeichert wird.
5. Willst du die Buchungen mitnehmen, übernimm zusätzlich den DATEV-Export wie unten beschrieben.

**Mit den Exporten**

1. Klicke auf **Dateien oder ZIP** und wähle die DATEV-ZIP-Dateien. Die App erkennt den DATEV-Export. Den Buchungsstapel legt sie als **DATEV-Export** ab. Belege aus einem DATEV-Belegpaket (mit einer Datei `document.xml`) übernimmt sie mit Belegnummer, Datum und Betrag, so wie sie im Export stehen.
2. Übernimm danach den PDF-Sammelexport mit **Ordner übernehmen** oder **Dateien oder ZIP**. E-Rechnungen (ZUGFeRD, XRechnung) liest die App mit Nummer, Datum und Betrag. Einfache PDFs übernimmt sie unverändert als Datei.
3. Hast du einen Z3-Export, wähle ihn genauso. Die App erkennt ihn an der Datei `index.xml` und legt ihn als Ganzes ab. Enthält er Debitoren, bietet die App danach **Kunden aus dem Export prüfen** an. Du siehst jede Zeile, bevor etwas gespeichert wird.
4. Mit **Übernehmen** landet alles im **Altbestand**: unverändert, durchsuchbar und in jeder Sicherung. In deine Zahlen fließt der Altbestand nicht ein.

## Prüfen

- Vergleiche unter **Altbestand** die Anzahl der Belege je Jahr mit Lexware Office. Bei E-Rechnungen zeigt die App auch die Summen je Jahr.
- Fehlt ein Zeitraum, exportiere ihn nach und übernimm ihn noch einmal. Doppelte Dateien erkennt die App und überspringt sie. Einen abgebrochenen direkten Abruf startest du einfach noch einmal.

## Vor der Kündigung

- [ ] Mit XL: Der direkte Abruf ist ohne Fehler durchgelaufen.
- [ ] Für jeden Zeitraum liegt der DATEV-Export mit Belegbildern im Altbestand.
- [ ] Ohne XL: Der PDF-Sammelexport für Eingang und Ausgang ist übernommen.
- [ ] Belege mit „Zu prüfen“ oder „Entwurf“ sind abgeschlossen oder gesichert.
- [ ] Kunden sind übernommen oder als Liste gesichert.
- [ ] Die ZIP-Dateien aus Lexware Office bewahrst du zusätzlich auf.
- [ ] Kontoauszüge sind bei deiner Bank gesichert, denn Bankumsätze übernimmt die App noch nicht.
- [ ] Den API-Schlüssel hast du in Lexware Office gelöscht, falls du einen erstellt hast.
- [ ] Du hast eine Sicherung von papierleicht gemacht (Einstellungen › Allgemein › Sicherung exportieren).
