# Versionshinweise

Für jede Version liegt hier eine Datei `<Version>.md` mit den Änderungen in
deutscher Alltagssprache. Die App zeigt sie im Update-Fenster und nach dem
Update unter „Neu in Version …“; der Release-Workflow setzt sie als Text des
GitHub-Releases. Ohne Datei zur Version in `version.txt` schlägt
`scripts/check-version.mjs` fehl.

Aufbau: nur die Abschnitte, die es gibt, in dieser Reihenfolge.

```markdown
## Neu

- Was du jetzt tun kannst, in einem Satz.

## Verbessert

## Behoben
```

Kein Fachjargon, keine Commit-Titel, keine Links auf Commits. Das technische
Änderungsprotokoll steht in `CHANGELOG.md`.
