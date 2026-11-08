import assert from "node:assert/strict";
import test from "node:test";
import { errorDetail, serviceErrorMessage } from "./errors.ts";

test("actionable service errors remain visible", () => {
  for (const text of [
    "Gib eine Bezeichnung ein.",
    "Wähle dein Konto aus.",
    "Bitte gib eine Bezeichnung ein.",
  ]) {
    const error = new Error(text);
    assert.equal(serviceErrorMessage(error, "fallback"), text);
    assert.equal(errorDetail(error), text);
  }
});

test("technical errors use a safe contextual fallback", () => {
  assert.equal(
    serviceErrorMessage(
      new Error("Die Datei konnte nicht gelesen werden: permission denied"),
      "Fehler",
    ),
    "Die Datei konnte nicht gelesen werden",
  );
  assert.equal(
    serviceErrorMessage(new Error("SQLITE_BUSY: database is locked"), "Speichern fehlgeschlagen."),
    "Speichern fehlgeschlagen.",
  );
  assert.equal(errorDetail("bridge disconnected"), "bridge disconnected");
});
