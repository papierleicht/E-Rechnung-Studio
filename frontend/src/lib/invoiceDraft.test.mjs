import assert from "node:assert/strict";
import test from "node:test";
import {
  clone,
  dueAfter,
  paymentDaysBetween,
  newCustomer,
  newLine,
  isBlankLine,
} from "./invoiceDraft.ts";
import { amountLabel, dateLabel } from "./format.ts";

test("a new customer defaults to 14 days and calendar dates survive DST and year boundaries", () => {
  assert.equal(newCustomer().paymentTermDays, 14);
  for (const [issued, due] of [
    ["2026-03-22", "2026-04-05"],
    ["2026-10-18", "2026-11-01"],
    ["2026-12-24", "2027-01-07"],
    ["2028-02-20", "2028-03-05"],
  ]) {
    assert.equal(dueAfter(issued), due);
    assert.equal(paymentDaysBetween(issued, due), 14);
  }
});

test("customer-specific payment periods include immediate payment", () => {
  assert.equal(dueAfter("2026-09-11", 0), "2026-09-11");
  assert.equal(dueAfter("2026-09-11", 30), "2026-10-11");
  assert.equal(paymentDaysBetween("2026-09-11", "2026-09-11"), 0);
  assert.equal(paymentDaysBetween("2026-09-11", "2026-09-10"), undefined);
  assert.equal(paymentDaysBetween("2026-09-11", ""), undefined);
  assert.equal(dueAfter(""), "");
});

test("editing a dialog copy cannot mutate the customer or another new invoice", () => {
  const original = newCustomer();
  original.address.city = "Berlin";
  const working = clone(original);
  working.address.city = "Hamburg";
  assert.equal(original.address.city, "Berlin");
  assert.equal(newCustomer().address.city, "");
  assert.notEqual(newLine().id, newLine().id);
});

test("adding an article can reuse a blank line without overwriting entered data", () => {
  const line = newLine();
  assert.equal(isBlankLine(line), true);
  line.unitPrice = "0,00";
  assert.equal(isBlankLine(line), true);
  line.description = "Bereits erfasst";
  assert.equal(isBlankLine(line), false);
  line.description = "";
  line.unitPrice = "12,50";
  assert.equal(isBlankLine(line), false);
});

test("German amount input displays consistently and invalid values never appear as NaN", () => {
  assert.equal(amountLabel("12,50"), amountLabel("12.50"));
  assert.equal(amountLabel("0.00"), "0,00\u00a0€");
  assert.equal(amountLabel("invalid"), "—");
  assert.equal(amountLabel(""), "—");
  assert.equal(dateLabel("2026-09-11"), "11.09.2026");
  assert.equal(dateLabel(""), "—");
});

test("an invoice template gets fresh dates and identity while preserving independent content", async () => {
  const { invoiceFromTemplate } = await import("./invoiceDraft.ts");
  const source = {
    id: "old",
    number: "RE-2025-42",
    kind: "invoice",
    status: "paid",
    issueDate: "2025-12-20",
    deliveryDate: "2025-12-19",
    dueDate: "2026-01-19",
    periodStart: "2025-12-01",
    periodEnd: "2025-12-19",
    customer: newCustomer(),
    lines: [newLine()],
    adjustments: [{ amount: "10" }],
    paymentTerms: "30 Tage netto",
    paymentReference: "RE-2025-42",
    prepaidAmount: "",
    correctsInvoiceId: "previous",
    precedingInvoiceNumber: "RE-1",
    precedingInvoiceDate: "2025-01-01",
    correctionReason: "replace",
    issuerSnapshot: { name: "Old company" },
    quoteId: "quo-1",
    quoteNumber: "AN-2025-0001",
    createdAt: "old",
    updatedAt: "old",
    issuedAt: "old",
    paidAt: "old",
    originalXml: "old",
    generatedXml: "old",
    generatedAt: "old",
    lastValidationState: "valid",
  };
  const before = clone(source);
  const draft = invoiceFromTemplate(source, "2026-09-20");
  assert.equal(draft.status, "draft");
  assert.equal(draft.automaticNumber, true);
  assert.equal(draft.issueDate, "2026-09-20");
  assert.equal(draft.deliveryDate, "2026-09-20");
  assert.equal(draft.dueDate, "2026-10-20");
  for (const key of [
    "id",
    "number",
    "periodStart",
    "periodEnd",
    "paymentReference",
    "prepaidAmount",
    "correctsInvoiceId",
    "precedingInvoiceNumber",
    "precedingInvoiceDate",
    "correctionReason",
    "createdAt",
    "updatedAt",
    "issuedAt",
    "paidAt",
    "originalXml",
    "generatedXml",
    "generatedAt",
    "lastValidationState",
  ])
    assert.equal(draft[key], "", key);
  assert.equal(draft.issuerSnapshot, undefined);
  assert.equal(draft.quoteId, undefined);
  assert.equal(draft.quoteNumber, undefined);
  assert.equal(
    invoiceFromTemplate({ ...source, advancePayment: true }).advancePayment,
    undefined,
    "the user marks each advance invoice",
  );
  assert.equal(
    invoiceFromTemplate({
      ...source,
      sourceTaxes: [{ rate: "19", basis: "16.78", amount: "3.20" }],
    }).sourceTaxes,
    undefined,
    "a shop's rounding belongs to its own order",
  );
  assert.equal(draft.paymentTerms, source.paymentTerms);
  draft.customer.name = "Changed";
  draft.lines[0].name = "Changed";
  draft.adjustments[0].amount = "20";
  assert.deepEqual(source, before);
  assert.equal(
    invoiceFromTemplate({ ...source, dueDate: source.issueDate }, "2026-09-20").dueDate,
    "2026-09-20",
  );
  assert.equal(invoiceFromTemplate({ ...source, dueDate: "" }, "2026-09-20").dueDate, "");
});

test("a prepaid shop invoice template resets payment terms and restores the customer payment period", async () => {
  const { invoiceFromTemplate } = await import("./invoiceDraft.ts");
  const source = {
    status: "paid",
    customer: newCustomer(),
    dueDate: "",
    paymentTerms: "Bereits im Shop bezahlt.",
    paymentMeansCode: "97",
    disablePaymentQR: true,
    prepaidAmount: "119.00",
  };
  const before = clone(source);
  const draft = invoiceFromTemplate(source, "2026-10-15");
  assert.equal(draft.prepaidAmount, "");
  assert.equal(draft.paymentTerms, "");
  assert.equal(draft.paymentMeansCode, "");
  assert.equal(draft.disablePaymentQR, false);
  assert.equal(draft.dueDate, "2026-10-29");
  assert.deepEqual(source, before);
  for (const [days, expected] of [
    [0, "2026-10-15"],
    [30, "2026-11-14"],
    [null, "2026-10-29"],
  ]) {
    assert.equal(
      invoiceFromTemplate(
        { ...source, customer: { ...source.customer, paymentTermDays: days } },
        "2026-10-15",
      ).dueDate,
      expected,
    );
  }
});
