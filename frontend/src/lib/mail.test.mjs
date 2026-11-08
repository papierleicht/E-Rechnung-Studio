import test from "node:test";
import assert from "node:assert/strict";
import {
  applyMailPreset,
  fileSizeLabel,
  portForSecurity,
  recipientList,
  unknownPlaceholders,
} from "./mail.ts";

const account = {
  preset: "",
  host: "smtp.eigener-server.de",
  port: 2525,
  security: "starttls",
  username: "me@example.de",
  fromName: "Muster GmbH",
  fromAddress: "me@example.de",
  replyTo: "",
  bccSelf: true,
};
const presets = [
  { id: "strato", label: "Strato", host: "smtp.strato.de", port: 465, security: "tls", hint: "" },
];

test("presets fill server settings and keep sender data", () => {
  const applied = applyMailPreset(account, presets, "strato");
  assert.deepEqual(
    [applied.preset, applied.host, applied.port, applied.security],
    ["strato", "smtp.strato.de", 465, "tls"],
  );
  assert.equal(applied.fromAddress, "me@example.de");
  assert.equal(applied.bccSelf, true);
  const manual = applyMailPreset(applied, presets, "");
  assert.equal(manual.preset, "");
  assert.equal(manual.host, "smtp.strato.de");
});

test("security change only replaces default ports", () => {
  assert.equal(portForSecurity("tls", 587), 465);
  assert.equal(portForSecurity("starttls", 465), 587);
  assert.equal(portForSecurity("tls", 2525), 2525);
});

test("unknown placeholders are reported once", () => {
  const allowed = ["Rechnungsnummer", "Kunde"];
  assert.deepEqual(unknownPlaceholders("{Rechnungsnummer} { Kunde }", allowed), []);
  assert.deepEqual(unknownPlaceholders("{Nummer} und {Nummer} {Betrag}", allowed), [
    "{Nummer}",
    "{Betrag}",
  ]);
});

test("recipients and sizes are formatted for the preview", () => {
  assert.deepEqual(recipientList(" a@example.de; b@example.de, ,"), [
    "a@example.de",
    "b@example.de",
  ]);
  assert.equal(fileSizeLabel(512), "512 B");
  assert.equal(fileSizeLabel(2048), "2 KB");
  assert.equal(fileSizeLabel(1572864), "1,5 MB");
});
