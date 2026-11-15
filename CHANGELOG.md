# Changelog

## [1.1.1](https://github.com/papierleicht/E-Rechnung-Studio/compare/v1.1.0...v1.1.1) (2026-11-15)


### Bug Fixes

* **updates:** check at every start and on request ([#9](https://github.com/papierleicht/E-Rechnung-Studio/issues/9)) ([fa2b249](https://github.com/papierleicht/E-Rechnung-Studio/commit/fa2b249f1f6a0e1449a5c6a9f2bb549ccdb75888))

## [1.1.0](https://github.com/papierleicht/E-Rechnung-Studio/compare/v1.0.0...v1.1.0) (2026-11-15)


### Features

* reset the app from the settings, optionally keeping the setup ([#7](https://github.com/papierleicht/E-Rechnung-Studio/issues/7)) ([b7182b1](https://github.com/papierleicht/E-Rechnung-Studio/commit/b7182b1360a5bf09e9c087cf70705ab409cca7ca))

## 1.0.0 (2026-11-07)

Erste öffentliche Version von E-Rechnung-Studio. Das Repository beginnt mit
diesem Stand; die Entwicklungsgeschichte davor ist nicht enthalten.

- Unternehmen, Kunden und Artikel verwalten; Rechnungen, Gutschriften,
  Anzahlungsrechnungen und Angebote mit eigenen Nummernkreisen erstellen;
  wiederkehrende Rechnungen als Entwürfe zur Prüfung.
- ZUGFeRD/Factur-X (BASIC, EN 16931, EXTENDED) und XRechnung 3.0 in CII und
  UBL erzeugen, mit eingebundenen XSD- und Geschäftsregeln prüfen und als
  PDF/A-3b mit XML-Anhang ausgeben.
- Ausgaben und Belege: Eingangsrechnungen als PDF oder XML prüfen, verwalten
  und mit Vorsteuer erfassen; Kassenbons und Fotos erfassen, Anschaffungen
  abschreiben.
- Zahlungen: Zahlungshinweise und SEPA-QR-Codes, Teilzahlungen und
  Ratenpläne, CSV-Import von Kontoumsätzen, Bankabruf über FinTS mit eigener
  Produktkennung, Rechnungsabgleich, Zahlungserinnerungen und Mahnungen.
- Konto & Finanzen: Einnahmen, Ausgaben und Ergebnis je Monat und Jahr,
  „Reicht das Geld?“ mit offenen Posten und anstehenden Steuern, Konten aus
  CSV-Importen benennen und entfernen.
- Steuern: Steuertermine und Werte für Mein ELSTER (Voranmeldung,
  Zusammenfassende Meldung, EÜR), Jahresübersicht nach Steuerart mit Umsatz-,
  Einkommen- und Gewerbesteuer, Steuerprofil für Gewerbe oder freiberufliche
  Tätigkeit, DATEV-Export, Hinweise zu Steuerfällen als Vorschlag ohne
  Beratung.
- GoBD: fortlaufendes Änderungsprotokoll, Prüfer-Export und
  Verfahrensdokumentation.
- Daten sichern und wiederherstellen; Rechnungen, Angebote und Erinnerungen
  per E-Mail über das eigene SMTP-Konto versenden.
- Shop-Bestellungen aus Shopify, WooCommerce, Shopware 6, Gambio, Wix und
  PrestaShop als Rechnungsentwürfe übernehmen.
- Chat & Markt: Community-Chat sowie Angebote und Gesuche unter Unternehmern.
- Desktop-Pakete für macOS, Windows und Linux (AppImage), jeweils für `amd64`
  und `arm64`, mit Update-Prüfung über GitHub Releases; Updates installiert die
  App nur mit gültiger Ed25519-Signatur.
- Kommandozeile `e-rechnung-studio-cli` zum Prüfen und Erzeugen von
  Rechnungen.
