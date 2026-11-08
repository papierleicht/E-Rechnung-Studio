import test from "node:test";
import assert from "node:assert/strict";
import { dashboardTasks } from "./dashboard.ts";

const bucket = { count: 0, amount: "0.00", foreign: 0 };
const empty = {
  receivable: bucket,
  receivableOverdue: bucket,
  payable: bucket,
  payableOverdue: bucket,
  unassignedBookings: 0,
  receivedToReview: 0,
  plansToReview: 0,
  refundsToCredit: 0,
  taxProfileMissing: false,
  financeLabel: "Q4 2026",
  financeIssues: [],
};

test("nothing to do yields no tasks", () => {
  assert.deepEqual(dashboardTasks(empty, 0), []);
});

test("urgent tasks come first and use singular and plural", () => {
  const tasks = dashboardTasks(
    {
      ...empty,
      receivableOverdue: { ...bucket, count: 1 },
      unassignedBookings: 3,
      financeIssues: [{ code: "stale_balance", message: "Kontostand veraltet" }],
    },
    2,
  );
  assert.deepEqual(
    tasks.map((t) => t.id),
    ["overdue", "bookings", "drafts", "finance"],
  );
  assert.equal(tasks[0].title, "1 Kundenrechnung überfällig");
  assert.equal(tasks[0].urgent, true);
  assert.equal(tasks[1].title, "3 Bankumsätze ohne Zuordnung");
  assert.equal(tasks[2].title, "2 Entwürfe noch nicht ausgestellt");
  assert.equal(tasks[3].title, "Q4 2026: 1 offener Punkt in den Finanzen");
  assert.equal(tasks[3].detail, "Kontostand veraltet");
});

test("a refund without credit note becomes a calm task", () => {
  const [task] = dashboardTasks({ ...empty, refundsToCredit: 1 }, 0);
  assert.equal(task.id, "refunds");
  assert.equal(task.title, "1 Erstattung ohne Gutschrift");
  assert.ok(!task.urgent);
});
