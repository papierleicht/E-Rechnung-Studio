## Vorher wissen

- Collmex kann nach unseren Unterlagen einen GDPdU-Export für die Betriebsprüfung, einen DATEV-Export und CSV-Listen erzeugen.
- Eine offizielle Anleitung von Collmex zum Export haben wir nicht gefunden. Die Schritte unten sind deshalb allgemein gehalten. Die genauen Menünamen kennen wir nicht.
- Nach unseren Unterlagen gibt Collmex nach der Kündigung zehn Jahre Lesezugang. Frag das vor der Kündigung bei Collmex nach und exportiere trotzdem vorher.
- Eine direkte Verbindung zu Collmex (API) hat papierleicht noch nicht. Bankumsätze übernimmt die App ebenfalls noch nicht.
- Rechnungen und Belege musst du in der Regel acht Jahre aufbewahren, Bücher und Abschlüsse zehn Jahre.

## In Collmex

1. **GDPdU-Export:** Suche nach dem Export für die Betriebsprüfung (GDPdU). Er ist der vollständigste Weg.
2. **DATEV-Export:** Erzeuge zusätzlich einen DATEV-Export, wenn möglich mit Belegen.
3. **Rechnungen:** Sichere deine Rechnungen als PDF, wenn Collmex dir das anbietet.
4. **Listen:** Kunden und Artikel exportierst du als CSV, wenn Collmex dir das anbietet.

## In papierleicht

1. Klicke auf **Dateien oder ZIP** oder **Ordner übernehmen** und wähle die Exporte.
2. Einen GDPdU-Export mit der Datei `index.xml` erkennt die App und legt ihn als Ganzes ab. Einen DATEV-Buchungsstapel legt sie als **DATEV-Export** ab. E-Rechnungen (ZUGFeRD, XRechnung) liest sie mit Nummer, Datum und Betrag, einfache PDFs übernimmt sie unverändert als Datei.
3. Mit **Übernehmen** landet alles im **Altbestand**: unverändert, durchsuchbar und in jeder Sicherung. In deine Zahlen fließt der Altbestand nicht ein.
4. Enthält ein Export Debitoren oder eine Kundentabelle, bietet die App danach **Kunden aus dem Export prüfen** an. CSV-Listen übernimmst du mit **Kunden aus Liste** bzw. **Artikel aus Liste**.

## Prüfen

- Vergleiche unter **Altbestand** die Anzahl der Belege je Jahr mit Collmex. Bei E-Rechnungen zeigt die App auch die Summen je Jahr.
- Fehlt etwas, exportiere es nach und übernimm es noch einmal. Doppelte Dateien erkennt die App und überspringt sie.

## Vor der Kündigung

- [ ] GDPdU- und DATEV-Export liegen für jedes Jahr im Altbestand.
- [ ] Die Exportdateien bewahrst du zusätzlich auf.
- [ ] Kunden und Artikel sind übernommen oder als Liste gesichert.
- [ ] Du weißt, wie lange du nach der Kündigung noch Zugang zu Collmex hast.
- [ ] Du hast eine Sicherung von papierleicht gemacht (Einstellungen › Allgemein › Sicherung exportieren).
