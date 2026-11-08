import test from "node:test";
import assert from "node:assert/strict";
import {
  hasHintAmounts,
  reminderBasisLine,
  reminderChannelLabel,
  reminderHistoryLine,
  reminderTiming,
} from "./reminders.ts";

const proposal = {
  title: "Zahlungserinnerung",
  amount: "200.00",
  currency: "EUR",
  dueSince: "2026-09-23",
  daysOverdue: 1,
  installment: false,
  due: false,
  complete: false,
  nextDate: "2026-09-30",
  fee: "0.00",
  interest: "0.00",
};

test("basis line distinguishes remaining balance and due installments", () => {
  assert.match(
    reminderBasisLine(proposal),
    /^Offen: 200,00\s€ · fällig seit 23\.09\.2026 \(1 Tag\)$/,
  );
  assert.match(
    reminderBasisLine({ ...proposal, installment: true, daysOverdue: 3 }),
    /^Fällige Raten: .* \(3 Tage\)$/,
  );
});

test("timing names the recommended level and the end of escalation", () => {
  assert.equal(reminderTiming(proposal), "Zahlungserinnerung empfohlen ab 30.09.2026");
  assert.equal(reminderTiming({ ...proposal, due: true }), "Zahlungserinnerung empfohlen");
  assert.match(reminderTiming({ ...proposal, complete: true }), /Alle Erinnerungsstufen/);
});

test("history line shows level, date, amount and channel", () => {
  assert.match(
    reminderHistoryLine({
      title: "1. Mahnung",
      date: "2026-10-26",
      total: "205.00",
      currency: "EUR",
      channel: "manual-export",
    }),
    /^1\. Mahnung · 26\.10\.2026 · 205,00\s€ · manuell exportiert$/,
  );
  assert.equal(reminderChannelLabel("email"), "per E-Mail versendet");
});

test("hint amounts are only flagged when positive", () => {
  assert.equal(hasHintAmounts(proposal), false);
  assert.equal(hasHintAmounts({ fee: "5.00", interest: "0.00" }), true);
});
