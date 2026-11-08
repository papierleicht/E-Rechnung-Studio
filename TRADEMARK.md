# Namen, Logos und Infrastruktur

Der Quellcode von E-Rechnung-Studio steht unter der [MPL-2.0](LICENSE). Diese
Lizenz überträgt ausdrücklich keine Rechte an Marken, Namen oder Logos
(MPL-2.0, Abschnitt 2.1). Diese Datei beschreibt, was mit den Kennzeichen des
Projekts erlaubt ist und was nicht.

## Kennzeichen

Inhaber der Kennzeichen ist der Betreiber von papierleicht. Dazu gehören:

- die Namen **papierleicht** und **Papierleicht** in jeder Schreibweise,
- der Produktname **E-Rechnung-Studio** (auch E-Rechnung Studio und E-Rechnungs-Studio),
- das Papierfeder-Logo und das App-Symbol, insbesondere die Dateien
  `frontend/src/brand/logo-light.svg`, `logo-dark.svg`, `brand-icon.svg`,
  `app-icon.svg` sowie `PapierleichtLogo.svelte` und `Icon.svelte`,
- die Domain papierleicht.de und ihre Subdomains.

Dass diese Dateien im Repository unter MPL-2.0 liegen, erlaubt das Kopieren
und Ändern der Dateien als Werk. Es erlaubt nicht, die Kennzeichen zur
Bezeichnung eines anderen Produkts zu verwenden.

## Erlaubt ohne Rückfrage

- Das Projekt beim Namen nennen, darüber berichten, es verlinken und in
  Artikeln, Vorträgen oder Vergleichen erwähnen.
- Sachlich darauf hinweisen, dass eine eigene Software auf E-Rechnung-Studio
  basiert oder damit zusammenarbeitet, zum Beispiel „basiert auf
  E-Rechnung-Studio von papierleicht“.
- Unveränderte offizielle Release-Pakete unter ihrem Namen weitergeben, etwa
  in Paketquellen, solange Quelle und Version erkennbar bleiben.
- Das Logo in Verweisen auf das Projekt zeigen, zum Beispiel neben einem Link
  zur Projektseite, ohne es zu verändern.

## Nur mit schriftlicher Zustimmung

- Geänderte Versionen, Forks oder eigene Builds unter den genannten Namen oder
  mit dem Logo veröffentlichen.
- Die Namen oder das Logo als Name, Bestandteil des Namens, Symbol oder
  Domain eines eigenen Produkts, Dienstes oder Unternehmens verwenden.
- Support, Hosting, Schulungen oder andere Dienstleistungen unter den Namen
  anbieten oder den Eindruck erwecken, sie kämen von papierleicht oder seien
  von papierleicht geprüft oder empfohlen.
- Einträge in App-Stores, Software-Verzeichnissen oder Paketquellen unter den
  Namen anlegen, die nicht die unveränderten offiziellen Pakete enthalten.
- Das Logo verändern, mit anderen Zeichen kombinieren oder als Grundlage für
  ein eigenes Logo verwenden.

Anfragen an [kontakt@papierleicht.de](mailto:kontakt@papierleicht.de?subject=Markennutzung%20E-Rechnung-Studio).

## Forks und eigene Builds

Wer eine geänderte Version verbreitet, gibt ihr einen eigenen Namen und ein
eigenes Logo. Dazu gehört:

1. Die Dateien in `frontend/src/brand/` durch eigene ersetzen, ebenso die
   Symbole und Namen in `build/` (App-Bundle, Installer, Fenstertitel).
2. Den Namen in der App, in der Dokumentation und in Release-Paketen ersetzen.
   Der Hinweis „basiert auf E-Rechnung-Studio“ bleibt erlaubt und erwünscht.
3. Die Lizenz- und Urheberhinweise des Quellcodes erhalten (MPL-2.0,
   Abschnitt 3.4). Das Entfernen der Kennzeichen ändert nichts an der
   Pflicht, die Quelle des Codes zu nennen.

## Infrastruktur

Lizenzserver, Backup-Dienst und Update-Quelle von papierleicht sind
Dienstleistungen für die offiziellen Builds. Sie sind nicht Teil der
Quellcode-Lizenz. Geänderte Versionen dürfen diese Dienste nicht verwenden
und tragen eigene Adressen ein:

- `licenseServerURL`, `licensePublicKey` und die Konto-Verwaltungsadresse in
  `internal/app/accountservice.go`,
- den Updater-Eintrag `releaseRepository` in `internal/app/app.go` und den öffentlichen
  Update-Schlüssel `PublicKey` in `internal/updatesign` (eigenes Schlüsselpaar
  mit `go run ./cmd/release-sign keygen -out <Datei>`),
- eine eigene FinTS-Produktkennung, die bei der Deutschen Kreditwirtschaft
  registriert ist (siehe `banking_service.go`). Die Kennung von papierleicht
  ist Abonnenten der offiziellen Builds vorbehalten.

Stand: 7. November 2026
