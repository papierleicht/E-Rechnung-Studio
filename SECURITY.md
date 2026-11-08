# Sicherheitslücken melden

E-Rechnung-Studio verarbeitet Rechnungen, Bankumsätze und Steuerdaten. Hinweise
auf Schwachstellen sind willkommen und werden vertraulich behandelt.

## Meldung

Schreib an [kontakt@papierleicht.de](mailto:kontakt@papierleicht.de?subject=Sicherheit%20E-Rechnung-Studio)
mit dem Betreff „Sicherheit“ oder melde die Lücke vertraulich über GitHub
([Report a vulnerability](https://github.com/papierleicht/E-Rechnung-Studio/security/advisories/new)).
Bitte **kein öffentliches Issue** und keine Diskussion in Pull Requests, bevor
eine Korrektur verfügbar ist.

Hilfreich sind:

- betroffene Komponente (Desktop-App, Lizenzserver, Backup-Dienst, Website)
  und Version (App: Menü „Über“, Datei `version.txt` im Quellcode),
- Schritte zum Nachvollziehen, möglichst mit Beispieldaten,
- Auswirkung aus deiner Sicht (welche Daten, welche Nutzer).

Bitte sende keine echten Bankzugangsdaten, PIN, TAN, Rechnungsdatenbanken,
Sicherungen oder FinTS-Produktkennungen. Ersetze sie durch Beispielwerte.

## Ablauf

- Eingangsbestätigung in der Regel innerhalb von drei Werktagen.
- Einschätzung und geplantes Vorgehen in der Regel innerhalb von 14 Tagen.
- Korrekturen erscheinen als Release; die Release-Notes nennen die Lücke erst,
  wenn eine behobene Version verfügbar ist.
- Nach 90 Tagen oder nach der Veröffentlichung der Korrektur, je nachdem was
  früher eintritt, darfst du die Lücke veröffentlichen. Sag vorher Bescheid,
  dann wird die Veröffentlichung abgestimmt. Auf Wunsch wirst du als Finder
  genannt.

Es gibt kein Bug-Bounty-Programm. Antwortzeiten können in Urlaubszeiten
länger sein.

## Unterstützte Versionen

Korrekturen erhält nur die jeweils neueste veröffentlichte Version. Ältere
Versionen aktualisierst du über die Update-Prüfung in den Einstellungen oder
über die Release-Seite.

| Version | Korrekturen |
|---|---|
| neueste Release | ja |
| ältere Releases | nein, bitte aktualisieren |
| Eigene Builds aus dem Quellcode | nach eigener Verantwortung |

## Umfang

In Scope:

- die Desktop-App, einschließlich Updater, Bankabruf,
  E-Mail-Versand und Export,
- der Konto- und Lizenzdienst sowie der Backup-Dienst,
- die Website papierleicht.de und die darin eingebettete Live-Demo.

Nicht in Scope:

- Angriffe, die vollständigen Zugriff auf das Gerät oder das Betriebssystem
  des Nutzers voraussetzen,
- Lastangriffe (DoS) und automatisierte Massenanfragen an unsere Dienste,
- Social Engineering gegen Nutzer oder den Betreiber,
- Schwachstellen in Diensten Dritter (FinTS-Server der Banken,
  Zahlungsdienstleister, E-Mail-Anbieter, GitHub). Melde sie bitte dort.

Tests gegen den Konto-, Lizenz- und Backup-Dienst nur mit einem eigenen
Konto und ohne Zugriff auf Daten anderer Kunden. Wer dabei versehentlich auf
fremde Daten stößt, bricht ab und meldet es.

## Was die App schützt

Zur Orientierung, nicht als Vollständigkeitsgarantie:

- Bank-PIN, SMTP-Passwort und das Abo-Token liegen im Schlüsselbund des
  Betriebssystems, nicht in der Datenbank.
- Die Cloud-Sicherung wird auf dem Gerät verschlüsselt; der Schlüssel verlässt
  das Gerät nicht.
- Der Bankabruf verbindet sich direkt mit der Bank; PIN und TAN gehen nicht
  über Server von papierleicht.
- Updates installiert die App nur mit einer Ed25519-Signatur, die zum in der
  App hinterlegten Schlüssel passt; der private Schlüssel liegt nicht im
  Repository.

Stand: 7. November 2026
