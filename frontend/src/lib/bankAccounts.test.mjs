import test from "node:test";
import assert from "node:assert/strict";
import { accountChoices, accountName, sortByDate } from "./bankAccounts.ts";

const connections = [{ id: "c1", label: "Sparkasse" }];
const account = (id, data = {}, label = "") => ({
  id,
  connectionId: "c1",
  label,
  lastSync: "",
  data,
});

test("a given name wins over the bank's labels", () => {
  assert.equal(
    accountName(account("a", { product_name: "Giro" }, " Kasse "), connections[0]),
    "Kasse",
  );
  assert.equal(accountName(account("a", { product_name: "Giro" }), connections[0]), "Sparkasse");
  assert.equal(accountName(account("a", { product_name: "Giro" })), "Giro");
  assert.equal(accountName(account("a")), "Bankkonto");
});

test("accounts with the same name are told apart by the end of the IBAN", () => {
  const choices = accountChoices(
    [
      account("a", { iban: "DE02 1203 0000 0000 2020 51" }),
      account("b", { iban: "DE89370400440532013000" }),
      account("c", { iban: "DE75512108001245126199" }, "Tagesgeld"),
    ],
    connections,
  );
  assert.deepEqual(
    choices.map((choice) => choice.name),
    ["Sparkasse (…2051)", "Sparkasse (…3000)", "Tagesgeld"],
  );
});

test("bookings sort by date in both directions and keep a day's order", () => {
  const rows = [
    { id: 1, date: "2026-03-02" },
    { id: 2, date: "2026-01-15" },
    { id: 3, date: "2026-03-02" },
  ];
  const ids = (order) => sortByDate(rows, (row) => row.date, order).map((row) => row.id);
  assert.deepEqual(ids("newest"), [1, 3, 2]);
  assert.deepEqual(ids("oldest"), [2, 1, 3]);
  assert.deepEqual(
    rows.map((row) => row.id),
    [1, 2, 3],
  );
});
