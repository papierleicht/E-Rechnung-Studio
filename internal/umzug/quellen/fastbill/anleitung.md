## Vorher wissen

- Alle Exporte findest du in FastBill im Bereich „Import & Export“: Ausgangsrechnungen, Eingangsrechnungen und Belege, Kunden und Produkte.
- Belegbilder bekommst du über die **DATEV XML-Schnittstelle online**. Der Export im DATEV-Format enthält dagegen nur Buchungsdaten ohne Belegbilder, je Export ein Jahr.
- Listen liest papierleicht als CSV oder als Excel-Datei (.xlsx); ältere Excel-Dateien (.xls) speicherst du vorher als .xlsx oder CSV. Eine direkte Verbindung zu FastBill (API) hat die App noch nicht.
- Rechnungen und Belege musst du in der Regel acht Jahre aufbewahren, Bücher und Abschlüsse zehn Jahre.

## In FastBill

1. Öffne den Bereich „Import & Export“.
2. **Belege mit Bildern:** Erzeuge den Export „DATEV XML-Schnittstelle online“. Du erhältst eine ZIP-Datei mit XML und Belegbildern.
3. **Kunden und Produkte:** Exportiere beide Listen als CSV.
4. **Rechnungen:** Exportiere Ausgangsrechnungen sowie Eingangsrechnungen und Belege. Wähle CSV oder Excel (.xlsx).
5. Wiederhole die Exporte für jeden Zeitraum, den du mitnehmen willst.

## In papierleicht

1. Klicke auf **Dateien oder ZIP** und wähle die ZIP-Datei mit den Belegbildern.
2. Ein DATEV-Belegpaket mit einer Datei `document.xml` erkennt die App und übernimmt die Belege mit Belegnummer, Datum und Betrag. E-Rechnungen (ZUGFeRD, XRechnung) liest sie mit Nummer, Datum und Betrag, andere Dateien übernimmt sie unverändert.
3. Mit **Übernehmen** landet alles im **Altbestand**: unverändert, durchsuchbar und in jeder Sicherung. In deine Zahlen fließt der Altbestand nicht ein.
4. Kunden und Produkte übernimmst du mit **Kunden aus Liste** bzw. **Artikel aus Liste**. Du siehst vorher jede Zeile und bestätigst, was übernommen wird.

## Prüfen

- Vergleiche unter **Altbestand** die Anzahl der Belege je Jahr mit FastBill. Bei E-Rechnungen zeigt die App auch die Summen je Jahr.
- Fehlt etwas, exportiere es nach und übernimm es noch einmal. Doppelte Dateien erkennt die App und überspringt sie.

## Vor der Kündigung

- [ ] Für jeden Zeitraum liegt das DATEV-Paket mit Belegbildern im Altbestand.
- [ ] Kunden und Produkte sind übernommen.
- [ ] Die Listen der Ausgangs- und Eingangsrechnungen und die ZIP-Dateien bewahrst du zusätzlich auf.
- [ ] Zugangsschlüssel (API-Key) in FastBill sind gelöscht, falls du einen genutzt hast.
- [ ] Du hast eine Sicherung von papierleicht gemacht (Einstellungen › Allgemein › Sicherung exportieren).
