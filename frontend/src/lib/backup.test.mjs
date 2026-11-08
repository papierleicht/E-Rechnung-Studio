import test from "node:test";
import assert from "node:assert/strict";
import {
  backupOverdueDays,
  progressLabel,
  restoreLoss,
  storageFull,
  verificationDue,
  formatRecoveryKey,
  keyGroupsEntered,
  recoveryKeyLines,
} from "./backup.ts";
import { formatBackupSize } from "./planStatus.ts";

const now = new Date("2026-10-30T12:00:00Z");
const ok = {
  available: true,
  keyConfigured: true,
  pending: false,
  lastBackupAt: "2026-10-01T12:00:00Z",
  lastError: "",
};

test("a restore names what it removes", () => {
  assert.equal(restoreLoss({ invoices: [], received: 0, expenses: 0 }), "");
  assert.equal(restoreLoss({ invoices: null, received: 0, expenses: 0 }), "");
  const text = restoreLoss({
    invoices: ["RE-1", "RE-2", "RE-3", "RE-4"],
    received: 1,
    expenses: 2,
  });
  assert.match(text, /4 ausgestellte Rechnungen \(RE-1, RE-2, RE-3 …\)/);
  assert.match(text, /1 Eingangsbeleg,/);
  assert.match(text, /2 erfasste Ausgaben/);
  assert.match(
    restoreLoss({ invoices: ["RE-9"], received: 0, expenses: 0 }),
    /1 ausgestellte Rechnung \(RE-9\)/,
  );
});

test("progress reads as a sentence", () => {
  assert.equal(
    progressLabel({
      phase: "upload",
      done: 2,
      total: 40,
      doneBytes: 3 * 2 ** 20,
      totalBytes: 2 * 2 ** 30,
    }),
    "Sicherung läuft: 2 von 40 Dokumenten (3,0 MB von 2,0 GB)",
  );
  assert.equal(
    progressLabel({ phase: "verify", done: 1, total: 1, doneBytes: 0, totalBytes: 0 }),
    "Prüfung läuft: 1 von 1 Dokument",
  );
});

test("sizes reach gigabytes", () => {
  assert.equal(formatBackupSize(512), "512 B");
  assert.equal(formatBackupSize(5 << 20), "5,0 MB");
  assert.equal(formatBackupSize(25 * 2 ** 30), "25,0 GB");
});

test("storage warns before it is full", () => {
  assert.equal(storageFull(79, 100), false);
  assert.equal(storageFull(80, 100), true);
  assert.equal(storageFull(1, 0), false);
});

test("an overdue backup is only reported when something waits", () => {
  assert.equal(backupOverdueDays(ok, now), 0);
  assert.equal(backupOverdueDays({ ...ok, pending: true }, now), 29);
  assert.equal(backupOverdueDays({ ...ok, lastError: "offline" }, now), 29);
  assert.equal(
    backupOverdueDays({ ...ok, pending: true, lastBackupAt: "2026-10-29T12:00:00Z" }, now),
    0,
  );
  assert.equal(backupOverdueDays({ ...ok, pending: true, available: false }, now), 0);
  assert.equal(backupOverdueDays({ ...ok, pending: true, keyConfigured: false }, now), 0);
  assert.equal(backupOverdueDays({ ...ok, lastBackupAt: "", lastError: "voll" }, now), 3);
  assert.equal(backupOverdueDays({ ...ok, lastBackupAt: "", pending: true }, now), 0);
});

test("a check is due yearly", () => {
  assert.equal(verificationDue("", now), true);
  assert.equal(verificationDue("2026-01-01T00:00:00Z", now), false);
  assert.equal(verificationDue("2025-01-01T00:00:00Z", now), true);
});

test("formatRecoveryKey sets groups while typing and pasting", () => {
  assert.equal(formatRecoveryKey("abcd"), "ABCD");
  assert.equal(formatRecoveryKey("abcde"), "ABCD-E");
  assert.equal(formatRecoveryKey("ab cd-ef\ngh"), "ABCD-EFGH");
  assert.equal(formatRecoveryKey("A".repeat(70)).length, 14 * 4 + 13);
  assert.equal(formatRecoveryKey(""), "");
});

test("keyGroupsEntered counts complete groups", () => {
  assert.equal(keyGroupsEntered(""), 0);
  assert.equal(keyGroupsEntered("ABCD-EF"), 1);
  assert.equal(keyGroupsEntered(formatRecoveryKey("A".repeat(56))), 14);
});

test("recoveryKeyLines splits the key into two equal lines", () => {
  const key = formatRecoveryKey("ABCD".repeat(14));
  const [first, second] = recoveryKeyLines(key);
  assert.equal(first.length, second.length + 1);
  assert.equal(first + second, key);
});
