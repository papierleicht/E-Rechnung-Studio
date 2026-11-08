import test from "node:test";
import assert from "node:assert/strict";
import { newsletterDate, validNewsletterEmail } from "./newsletter.ts";

test("newsletter accepts plain addresses only", () => {
  for (const ok of ["kunde@example.com", " a.b+news@firma.de "])
    assert.ok(validNewsletterEmail(ok), ok);
  for (const bad of ["", "kein-at", "a@b", "Name <a@example.com>", "a b@example.com", "a@example."])
    assert.ok(!validNewsletterEmail(bad), bad);
});

test("newsletter shows the request date in German", () => {
  assert.equal(newsletterDate("2026-11-02T09:30:00Z"), "02.11.2026");
  assert.equal(newsletterDate(""), "einem früheren Tag");
});
