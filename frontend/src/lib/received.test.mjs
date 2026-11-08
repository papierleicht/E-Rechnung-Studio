import assert from "node:assert/strict";
import test from "node:test";
import {
  attachmentSummary,
  attachmentTitle,
  attachmentTypeLabel,
  awaitsInputTaxDecision,
  countsForInputTax,
  documentKindLabel,
  encodeInvoiceFile,
  fileSizeLabel,
  inputTaxChoiceLabel,
  inspectionLabel,
  issueClassLabels,
  originalOf,
  precedingLabel,
  receiptSubtitle,
} from "./received.ts";

test("binary import preserves all byte values across encoding chunks", async () => {
  const bytes = Uint8Array.from({ length: 20000 }, (_, index) => index % 256);
  const encoded = await encodeInvoiceFile(new File([bytes], "invoice.pdf"));
  assert.deepEqual(new Uint8Array(Buffer.from(encoded, "base64")), bytes);
});

test("empty and oversized imports fail before reading the file", async () => {
  for (const size of [0, 20 * 1024 * 1024 + 1]) {
    await assert.rejects(
      encodeInvoiceFile({
        size,
        arrayBuffer() {
          throw new Error("must not read");
        },
      }),
      /20 MB/,
    );
  }
});

test("a document counts for the Vorsteuer by its status, not by its check state", () => {
  const withStatus = (state, allowed) => ({
    validation: { state, issues: [] },
    inputTax: { allowed },
  });
  assert.equal(countsForInputTax(withStatus("valid", true)), true);
  // Only findings without a Pflichtangabe: the check failed, the Vorsteuer counts.
  assert.equal(countsForInputTax(withStatus("invalid", true)), true);
  assert.equal(countsForInputTax(withStatus("invalid", false)), false);
  // Without a status (older answers) only a passed check counts.
  assert.equal(countsForInputTax({ validation: { state: "valid", issues: [] } }), true);
  assert.equal(countsForInputTax({ validation: { state: "warning", issues: [] } }), false);
});

test("the list label says when findings do not touch the Vorsteuer", () => {
  const check = (state, inputTax) => ({ validation: { state, issues: [] }, inputTax });
  assert.equal(inspectionLabel(check("valid", { allowed: true })), "Technische Prüfung bestanden");
  assert.equal(
    inspectionLabel(check("invalid", { allowed: true })),
    "Hinweise, Vorsteuer eingerechnet",
  );
  assert.equal(
    inspectionLabel(check("invalid", { allowed: true, onPayment: true })),
    "Hinweise, Vorsteuer mit der Zahlung",
  );
  assert.equal(inspectionLabel(check("invalid", { allowed: false })), "Fehler gefunden");
  assert.equal(inspectionLabel(check("invalid")), "Fehler gefunden");
  assert.equal(inspectionLabel(check("unsupported", { allowed: false })), "Nicht bestätigt");
  assert.equal(inspectionLabel(check("unknown")), "Ungeprüft");
});

test("every class of a finding has a label", () => {
  for (const name of ["format", "business", "hint", "unchecked"]) {
    assert.ok(issueClassLabels[name], name);
  }
});

test("the user's decision replaces the app's suggestion", () => {
  const check = (allowed, choice) => ({
    validation: { state: "invalid", issues: [] },
    inputTax: { allowed, choice },
  });
  assert.equal(countsForInputTax(check(false, { mode: "full" })), true);
  assert.equal(countsForInputTax(check(false, { mode: "amount", amount: "5.00" })), true);
  assert.equal(countsForInputTax(check(true, { mode: "none" })), false);
  // A decided document needs no further look, whatever the decision was.
  assert.equal(awaitsInputTaxDecision(check(false)), true);
  assert.equal(awaitsInputTaxDecision(check(true)), false);
  assert.equal(awaitsInputTaxDecision(check(false, { mode: "none" })), false);

  assert.equal(inputTaxChoiceLabel({ mode: "none" }), "Nicht in die Vorsteuer eingerechnet");
  assert.equal(inputTaxChoiceLabel({ mode: "full" }), "Die ganze Steuer eingerechnet");
  assert.match(inputTaxChoiceLabel({ mode: "amount", amount: "94.20" }), /^94,20\s€ eingerechnet$/);
  assert.equal(
    inputTaxChoiceLabel({ mode: "payment" }),
    "Mit der Zahlung eingerechnet, jeweils der bezahlte Anteil",
  );

  const item = (choice) => ({
    filename: "a.xml",
    kind: "xml",
    inspection: { invoiceNumber: "RE-1", ...check(false, choice) },
  });
  assert.equal(receiptSubtitle(item()), "RE-1 · XML · Fehler gefunden");
  assert.equal(
    receiptSubtitle(item({ mode: "full" })),
    "RE-1 · XML · Fehler gefunden · Vorsteuer von dir festgelegt",
  );
});

test("sizes are shown in whole KB below 1 MB and in MB with one decimal above", () => {
  assert.equal(fileSizeLabel(10), "1 KB");
  assert.equal(fileSizeLabel(48 * 1024), "48 KB");
  assert.equal(fileSizeLabel(1024 * 1024 - 1), "1024 KB");
  assert.equal(fileSizeLabel(Math.round(1.5 * 1024 * 1024)), "1,5 MB");
});

test("an attachment is described by what it is and where it is", () => {
  const file = {
    index: 0,
    kind: "file",
    name: "Stundennachweis",
    reference: "SN-1",
    filename: "nachweis.pdf",
    mimeType: "application/pdf",
    size: 48 * 1024,
    link: "",
    openable: true,
  };
  assert.equal(attachmentTypeLabel("application/pdf"), "PDF");
  assert.equal(attachmentTypeLabel("text/html"), "Datei (text/html)");
  assert.equal(attachmentTypeLabel(""), "Datei ohne Typangabe");

  assert.equal(attachmentTitle(file), "Stundennachweis");
  assert.equal(attachmentSummary(file), "PDF · 48 KB · nachweis.pdf");
  // Without a description the file name is the title and is not repeated.
  const unnamed = { ...file, name: "" };
  assert.equal(attachmentTitle(unnamed), "nachweis.pdf");
  assert.equal(attachmentSummary(unnamed), "PDF · 48 KB");

  const link = {
    ...file,
    kind: "link",
    filename: "",
    mimeType: "",
    size: 0,
    link: "https://x.example/a",
  };
  assert.equal(
    attachmentSummary(link),
    "Verweist auf eine Webadresse, die die App nicht öffnet: https://x.example/a",
  );
  const reference = { ...link, kind: "reference", link: "" };
  assert.equal(
    attachmentSummary(reference),
    "Nur ein Verweis (SN-1). Die Datei liegt der Rechnung nicht bei.",
  );
  // The reference is the title when nothing else names the entry.
  const bare = { ...reference, name: "" };
  assert.equal(attachmentTitle(bare), "SN-1");
  assert.equal(attachmentSummary(bare), "Nur ein Verweis. Die Datei liegt der Rechnung nicht bei.");
  assert.equal(attachmentTitle({ name: "", filename: "", reference: "" }), "Anlage");
});

test("a correcting document says what it is and which invoice it refers to", () => {
  assert.equal(documentKindLabel({ documentType: 384 }), "Berichtigung");
  assert.equal(documentKindLabel({ documentType: 381 }), "Gutschrift");
  assert.equal(documentKindLabel({ documentType: 386 }), "Anzahlungsrechnung");
  assert.equal(documentKindLabel({ documentType: 326 }), "Teilrechnung");
  assert.equal(documentKindLabel({ documentType: 875 }), "Abschlagsrechnung");
  assert.equal(documentKindLabel({ documentType: 380 }), "");
  assert.equal(documentKindLabel({}), "");

  assert.equal(precedingLabel({}), "");
  assert.equal(
    precedingLabel({
      precedingInvoices: [{ number: "RE-1", date: "2026-03-12" }, { number: "RE-2" }],
    }),
    "RE-1 vom 12.03.2026, RE-2",
  );
});

test("the original of a correction is the invoice of the same supplier with the number it names", () => {
  const invoice = (id, supplier, number, documentType, ...numbers) => ({
    id,
    status: "active",
    inspection: {
      sellerName: supplier,
      invoiceNumber: number,
      documentType,
      precedingInvoices: numbers.map((n) => ({ number: n })),
    },
  });
  const original = invoice("a", "Büro Nord GmbH", "RE-1", 380);
  const find = (correction, ...others) =>
    originalOf(correction, [original, correction, ...others])?.id;

  assert.equal(find(invoice("c", "Büro Nord GmbH", "RE-1K", 384, "RE-1")), "a");
  assert.equal(find(invoice("c", "Büro Nord GmbH", "RE-1K", 380, "RE-1")), "a");
  assert.equal(find(invoice("c", "  büro  nord gmbh", "RE-1K", 384, " re-1 ")), "a");
  assert.equal(find(invoice("c", "Büro Nord GmbH", "RE-1K", 384, "RE-0", "RE-1")), "a");
  // Nothing to find: no reference, another supplier, a credit note, an unknown invoice,
  // an ordinary invoice, a document that refers to itself or has no supplier name.
  assert.equal(find(invoice("c", "Büro Nord GmbH", "RE-1K", 384)), undefined);
  assert.equal(find(invoice("c", "Anderes Büro", "RE-1K", 384, "RE-1")), undefined);
  assert.equal(find(invoice("c", "Büro Nord GmbH", "G-1", 381, "RE-1")), undefined);
  assert.equal(find(invoice("c", "Büro Nord GmbH", "RE-9K", 384, "RE-9")), undefined);
  assert.equal(find(invoice("c", "Büro Nord GmbH", "RE-2", 380)), undefined);
  assert.equal(find(invoice("c", "Büro Nord GmbH", "RE-5", 384, "RE-5")), undefined);
  assert.equal(find(invoice("c", "", "RE-1K", 384, "RE-1")), undefined);
  // A misimported invoice is no invoice that counts.
  assert.equal(
    originalOf(invoice("c", "Büro Nord GmbH", "RE-1K", 384, "RE-1"), [
      { ...original, status: "misimport" },
    ]),
    undefined,
  );
});
