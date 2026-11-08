import assert from "node:assert/strict";
import test from "node:test";
import { newCustomer } from "./invoiceDraft.ts";
import {
  applyQuoteCustomer,
  canConvertQuote,
  filterQuotes,
  newQuote,
  quoteBadgeClass,
  quoteStatusLabel,
} from "./quotes.ts";

const company = { smallBusiness: false, exemptionReason: "", defaultTaxTreatment: "standard" };
const base = {
  id: "draft",
  number: "RE-2026-0001",
  kind: "credit-note",
  status: "draft",
  issueDate: "2026-01-01",
  deliveryDate: "2026-01-01",
  dueDate: "2026-01-15",
  paymentReference: "RE-2026-0001",
  paymentTerms: "14 Tage",
  notes: "Alt",
  profile: "facturx-en16931",
  syntax: "cii",
  customer: newCustomer(),
  lines: [],
};

test("a new quote has a 30-day validity and no invoice-only payment data", () => {
  const quote = newQuote(base, company, "2026-12-15");
  assert.equal(quote.status, "draft");
  assert.equal(quote.validUntil, "2027-01-14");
  assert.equal(quote.document.issueDate, "2026-12-15");
  assert.equal(quote.document.kind, "invoice");
  for (const key of ["id", "number", "dueDate", "deliveryDate", "paymentReference", "notes"])
    assert.equal(quote.document[key], "", key);
  assert.equal(quote.document.lines.length, 1);
  assert.equal(base.number, "RE-2026-0001", "template must not change");
});

test("choosing a customer applies the same invoice defaults as a new invoice", () => {
  const quote = newQuote(base, company, "2026-12-15");
  const customer = {
    ...newCustomer(),
    id: "cus-1",
    name: "Kunde",
    buyerReference: "Leitweg",
    invoiceSettings: {
      profile: "xrechnung-ubl",
      kind: "invoice",
      taxTreatment: "reverse-charge",
      taxSource: "manual",
      taxExemptionReason: "Steuerschuldnerschaft des Leistungsempfängers",
      businessProcess: "",
      paymentMeansCode: "58",
    },
  };
  applyQuoteCustomer(quote, customer, company);
  assert.equal(quote.document.customerId, "cus-1");
  assert.equal(quote.document.buyerReference, "Leitweg");
  assert.equal(quote.document.profile, "xrechnung-ubl");
  assert.equal(quote.document.syntax, "ubl");
  assert.equal(quote.document.taxTreatment, "reverse-charge");
  assert.equal(quote.document.taxSource, "customer");
  assert.equal(quote.document.paymentMeansCode, "58");
});

test("status labels, badges, conversion and search", () => {
  assert.equal(quoteStatusLabel("expired"), "Abgelaufen");
  assert.equal(quoteStatusLabel("converted"), "In Rechnung umgewandelt");
  assert.equal(quoteBadgeClass("sent"), "issued");
  assert.equal(quoteBadgeClass("accepted"), "paid");
  assert.equal(quoteBadgeClass("rejected"), "rejected");
  assert.deepEqual(
    ["draft", "sent", "expired", "accepted", "rejected", "converted"].filter(canConvertQuote),
    ["sent", "expired", "accepted"],
  );
  const list = [
    { number: "AN-2026-0001", customerName: "Alpha", issueDate: "2026-01-01", status: "sent" },
    { number: "AN-2026-0002", customerName: "Beta", issueDate: "2026-01-02", status: "rejected" },
  ];
  assert.equal(filterQuotes(list, " ").length, 2);
  assert.deepEqual(
    filterQuotes(list, "abgelehnt").map((q) => q.customerName),
    ["Beta"],
  );
  assert.deepEqual(
    filterQuotes(list, "0001").map((q) => q.customerName),
    ["Alpha"],
  );
});
