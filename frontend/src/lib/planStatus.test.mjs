import test from "node:test";
import assert from "node:assert/strict";
import {
  accountErrorMessage,
  formatBackupSize,
  formatPlanDate,
  planFromStatus,
  planLevel,
  planLabel,
  plusBookingUrl,
} from "./planStatus.ts";

const base = {
  configured: true,
  email: "kunde@example.com",
  plan: "",
  features: null,
  expiresAt: "",
  notice: "",
  noticeKind: "",
};

test("plan flags follow the verified account state", () => {
  assert.deepEqual(
    ["signed-out", "signed-in", "plus", "plus-backup"].map((state) => {
      const p = planFromStatus({ ...base, state });
      return [p.plusActive, p.backupActive];
    }),
    [
      [false, false],
      [false, false],
      [true, false],
      [true, true],
    ],
  );
  const tax = planFromStatus({
    ...base,
    state: "plus-backup",
    plan: "plus_tax_backup",
    features: ["fints", "elster", "backup"],
  });
  assert.deepEqual(
    [tax.plusActive, tax.backupActive, tax.taxActive, tax.plan],
    [true, true, true, "plus_tax_backup"],
  );
  // Features of an expired or missing entitlement never count.
  const signedIn = planFromStatus({ ...base, state: "signed-in", features: ["elster"] });
  assert.equal(signedIn.taxActive, false);
  // Community Support unlocks nothing in the app but is named.
  const support = planFromStatus({ ...base, state: "signed-in", plan: "support" });
  assert.deepEqual(
    [support.supportActive, support.plusActive, support.plan],
    [true, false, "support"],
  );
  assert.equal(
    planFromStatus({ ...base, state: "signed-out", plan: "support" }).supportActive,
    false,
  );
  assert.equal(planLabel(support), "Community Support aktiv");
  const unknown = planFromStatus({ ...base, state: "gold" });
  assert.equal(unknown.state, "signed-out");
  assert.equal(unknown.plusActive, false);
  assert.equal(unknown.loaded, true);
});

test("labels, dates and sizes are German", () => {
  assert.equal(
    planLabel({ state: "plus-backup", taxActive: false, backupActive: true }),
    "Plus mit Backup aktiv",
  );
  assert.equal(
    planLabel({ state: "plus", taxActive: true, backupActive: false }),
    "Plus Steuer aktiv",
  );
  assert.match(
    planLabel({ state: "signed-in", taxActive: false, backupActive: false }),
    /Community/,
  );
  assert.equal(formatPlanDate("2026-11-09T12:00:00Z"), "09.11.2026");
  assert.equal(formatPlanDate(""), "");
  assert.equal(formatBackupSize(512), "512 B");
  assert.equal(formatBackupSize(3 * 1024 * 1024 + 300000), "3,3 MB");
});

test("service errors keep the German headline", () => {
  assert.equal(
    accountErrorMessage(
      new Error("Der Kontoserver ist nicht erreichbar. Bitte prüfen. (dial tcp: refused)"),
      "x",
    ),
    "Der Kontoserver ist nicht erreichbar. Bitte prüfen.",
  );
  assert.equal(accountErrorMessage("", "Fallback"), "Fallback");
});

test("plan level advertises the next tier", () => {
  assert.deepEqual(planLevel({ plusActive: false, taxActive: false }), {
    level: 0,
    tier: "community",
    next: "plus",
    cta: "Plus freischalten",
  });
  assert.deepEqual(planLevel({ plusActive: true, taxActive: false }), {
    level: 1,
    tier: "plus",
    next: null,
    cta: "",
  });
  // Plus Steuer is not for sale yet, so no account is offered it.
  assert.deepEqual(planLevel({ plusActive: true, taxActive: true }), {
    level: 1,
    tier: "plus",
    next: null,
    cta: "",
  });
});

test("Community Support customers switch to Plus instead of booking it again", () => {
  const manage = "https://papierleicht.de/konto";
  assert.equal(plusBookingUrl(manage, "plus", false), "https://papierleicht.de/konto?tarif=plus");
  assert.equal(
    plusBookingUrl(manage, "plus", true),
    "https://papierleicht.de/konto/wechsel?tarif=plus",
  );
  assert.equal(
    plusBookingUrl(`${manage}/`, "backup", true),
    "https://papierleicht.de/konto/wechsel?tarif=backup",
  );
  assert.equal(plusBookingUrl("", "plus", false), "https://papierleicht.de/konto?tarif=plus");
});
