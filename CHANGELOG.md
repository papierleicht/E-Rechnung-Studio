# Changelog

## [1.3.0](https://github.com/papierleicht/papierleicht-app/compare/v1.2.0...v1.3.0) (2026-11-18)


### Features

* **migration:** bring invoices, receipts and customers over from other programs ([#20](https://github.com/papierleicht/papierleicht-app/issues/20)) ([53c0e4a](https://github.com/papierleicht/papierleicht-app/commit/53c0e4a97821a01cf978a41df1615b532c63c41f))
* rename the app to papierleicht ([#19](https://github.com/papierleicht/papierleicht-app/issues/19)) ([33ad723](https://github.com/papierleicht/papierleicht-app/commit/33ad7232fd2679a483f00850f38a41aa17d27916))
* **updates:** ask papierleicht for updates, with one choice for the statistics ([#21](https://github.com/papierleicht/papierleicht-app/issues/21)) ([440a796](https://github.com/papierleicht/papierleicht-app/commit/440a796d8f15724671d3edd6ef2c9e9077a4f3a5))
* **updates:** show what is new in German before and after an update ([#22](https://github.com/papierleicht/papierleicht-app/issues/22)) ([c2bc26f](https://github.com/papierleicht/papierleicht-app/commit/c2bc26ffda641285f281f93fc3c9e6d928558a84))


### Bug Fixes

* no busy box flashing in the corner when the app starts ([#18](https://github.com/papierleicht/papierleicht-app/issues/18)) ([184f55b](https://github.com/papierleicht/papierleicht-app/commit/184f55b8279e8a950f33b5fb1bd81cf400a984d5))

## [1.2.0](https://github.com/papierleicht/E-Rechnung-Studio/compare/v1.1.1...v1.2.0) (2026-11-15)


### Features

* **tax:** compute the tax on the margin from purchase prices ([#13](https://github.com/papierleicht/E-Rechnung-Studio/issues/13)) ([c38a5f4](https://github.com/papierleicht/E-Rechnung-Studio/commit/c38a5f453b5ad2166041cb507f8b3f2ee9740ecf))
* **updates:** offer to move the app where updates can install ([#12](https://github.com/papierleicht/E-Rechnung-Studio/issues/12)) ([669425c](https://github.com/papierleicht/E-Rechnung-Studio/commit/669425c2cf865bc383dc89badf243933a39031d9))

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
