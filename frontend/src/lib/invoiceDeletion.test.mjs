import assert from "node:assert/strict";
import test from "node:test";
import { deletionReason, deletionReasons } from "./invoiceDeletion.ts";

test("the logged reason combines the choice with optional details", () => {
  assert.equal(deletionReason("", "egal"), "");
  assert.equal(deletionReason(deletionReasons[0], " "), "Tippfehler oder falsche Angaben");
  assert.equal(
    deletionReason("Falscher Kunde", "Müller statt Maier"),
    "Falscher Kunde: Müller statt Maier",
  );
  // "Anderer Grund" alone says nothing; it needs the user's own words.
  assert.equal(deletionReason("Anderer Grund", ""), "");
  assert.equal(deletionReason("Anderer Grund", "Auftrag storniert"), "Auftrag storniert");
});
