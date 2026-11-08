<!-- Titel im Format type(scope): description, zum Beispiel fix(ui): Beispiel; siehe README, Abschnitt „Mitwirken“ -->

## Was ändert sich?

<!-- Wirkung für Nutzer, verknüpftes Issue (zum Beispiel "Closes #12"), Begründung -->

## Prüfung

- [ ] `npm --prefix frontend run check`, `test`, `format:check`, `build` und `go test ./...` laufen durch
- [ ] Getestet auf: <!-- macOS / Windows / Linux-Build / Live-Demo -->
- [ ] UI-Änderungen auch bei 900 px Fensterbreite geprüft (sonst entfällt)
- [ ] Keine echten Daten (Rechnungen, Kunden, IBANs, Zugänge, Produktkennungen, Schlüssel)
- [ ] Steuer- oder Formatlogik: Test und Quelle (Paragraf, BMF-Schreiben, Spezifikation) genannt
- [ ] Neue Abhängigkeiten in `docs/third-party-notices.md` eingetragen, Lizenz mit MPL-2.0 verträglich
