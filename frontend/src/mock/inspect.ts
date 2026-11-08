/**
 * Reads the key fields of a CII or UBL e-invoice in the browser, also from the
 * XML attachment of a ZUGFeRD/Factur-X PDF. The desktop app additionally runs
 * the full XSD and Schematron validation; the demo says so in its result.
 */
import type {
  ImportInspection,
  ValidationIssue,
} from "../../bindings/papierleicht.de/e-rechnung-studio/internal/model/models";

const demoNotice: ValidationIssue = {
  stage: "demo",
  ruleId: "DEMO",
  severity: "warning",
  message:
    "Die Live-Demo liest nur die wichtigsten Angaben. Die vollständige Prüfung nach XSD und Schematron (EN 16931, XRechnung) macht die Desktop-App.",
};

function empty(state: string, issue: ValidationIssue): ImportInspection {
  return {
    dueDate: "",
    iban: "",
    syntax: "",
    profile: "",
    invoiceNumber: "",
    issueDate: "",
    sellerName: "",
    buyerName: "",
    currency: "",
    totals: { net: "", tax: "", gross: "", due: "" },
    validation: { state, issues: [issue] },
  };
}

function decode(encoded: string): Uint8Array {
  return Uint8Array.from(atob(encoded), (character) => character.charCodeAt(0));
}

/** First element with this local name (namespaces vary between documents). */
function first(root: Element | Document, path: string[]): string {
  let scope: (Element | Document)[] = [root];
  for (const name of path) {
    scope = scope.flatMap((element) => Array.from(element.getElementsByTagNameNS("*", name)));
    if (!scope.length) return "";
  }
  return scope[0].textContent?.trim() ?? "";
}

function isoDate(value: string): string {
  const compact = value.replace(/\D/g, "");
  return compact.length >= 8
    ? `${compact.slice(0, 4)}-${compact.slice(4, 6)}-${compact.slice(6, 8)}`
    : "";
}

function inspectXML(text: string): ImportInspection {
  const doc = new DOMParser().parseFromString(text, "application/xml");
  if (doc.getElementsByTagName("parsererror").length)
    return empty("error", {
      stage: "xml",
      ruleId: "XML",
      severity: "error",
      message: "Die Datei ist kein gültiges XML.",
    });
  const root = doc.documentElement.localName;
  if (root === "CrossIndustryInvoice") {
    const guideline = first(doc, ["GuidelineSpecifiedDocumentContextParameter", "ID"]);
    const sums = (name: string) =>
      first(doc, ["SpecifiedTradeSettlementHeaderMonetarySummation", name]);
    return {
      syntax: "CII",
      profile: guideline.includes("xrechnung")
        ? "XRechnung · CII"
        : guideline.includes("en16931")
          ? "ZUGFeRD / Factur-X · EN 16931"
          : guideline || "CII",
      invoiceNumber: first(doc, ["ExchangedDocument", "ID"]),
      issueDate: isoDate(first(doc, ["ExchangedDocument", "IssueDateTime"])),
      dueDate: isoDate(first(doc, ["SpecifiedTradePaymentTerms", "DueDateDateTime"])),
      sellerName: first(doc, ["SellerTradeParty", "Name"]),
      buyerName: first(doc, ["BuyerTradeParty", "Name"]),
      iban: first(doc, ["PayeePartyCreditorFinancialAccount", "IBANID"]),
      currency: first(doc, ["InvoiceCurrencyCode"]) || "EUR",
      totals: {
        net: sums("TaxBasisTotalAmount"),
        tax: sums("TaxTotalAmount"),
        gross: sums("GrandTotalAmount"),
        due: sums("DuePayableAmount"),
      },
      validation: { state: "warning", issues: [demoNotice] },
    };
  }
  if (root === "Invoice" || root === "CreditNote") {
    const top = doc.documentElement;
    const direct = (name: string) =>
      Array.from(top.children)
        .find((element) => element.localName === name)
        ?.textContent?.trim() ?? "";
    const sums = (name: string) => first(doc, ["LegalMonetaryTotal", name]);
    const customization = direct("CustomizationID");
    return {
      syntax: "UBL",
      profile: customization.includes("xrechnung") ? "XRechnung · UBL" : "UBL",
      invoiceNumber: direct("ID"),
      issueDate: direct("IssueDate"),
      dueDate: direct("DueDate"),
      sellerName:
        first(doc, ["AccountingSupplierParty", "RegistrationName"]) ||
        first(doc, ["AccountingSupplierParty", "Name"]),
      buyerName:
        first(doc, ["AccountingCustomerParty", "RegistrationName"]) ||
        first(doc, ["AccountingCustomerParty", "Name"]),
      iban: first(doc, ["PayeeFinancialAccount", "ID"]),
      currency: direct("DocumentCurrencyCode") || "EUR",
      totals: {
        net: sums("TaxExclusiveAmount"),
        tax: first(doc, ["TaxTotal", "TaxAmount"]),
        gross: sums("TaxInclusiveAmount"),
        due: sums("PayableAmount"),
      },
      validation: { state: "warning", issues: [demoNotice] },
    };
  }
  return empty("unsupported", {
    stage: "import",
    ruleId: "UNSUPPORTED",
    severity: "warning",
    message: "Diese XML-Datei ist keine E-Rechnung im Format CII oder UBL.",
  });
}

async function attachedXML(data: Uint8Array): Promise<string | null> {
  const { loadPDF } = await import("../lib/pdfViewer");
  const task = loadPDF(btoa(Array.from(data, (byte) => String.fromCharCode(byte)).join("")));
  try {
    const pdf = await task.promise;
    const attachments = ((await pdf.getAttachments()) ?? {}) as Record<
      string,
      { filename: string; content: Uint8Array }
    >;
    const file = Object.values(attachments).find((a) => /\.xml$/i.test(a.filename));
    return file ? new TextDecoder().decode(file.content) : null;
  } finally {
    void task.destroy();
  }
}

export async function inspectFile(
  filename: string,
  encoded: string,
): Promise<{ inspection: ImportInspection }> {
  const data = decode(encoded);
  if (/\.pdf$/i.test(filename)) {
    let xml: string | null = null;
    try {
      xml = await attachedXML(data);
    } catch {
      return {
        inspection: empty("error", {
          stage: "pdf",
          ruleId: "PDF",
          severity: "error",
          message: "Die PDF-Datei lässt sich nicht lesen. Prüfe, ob sie beschädigt ist.",
        }),
      };
    }
    if (!xml)
      return {
        inspection: empty("unsupported", {
          stage: "import",
          ruleId: "PDF-NO-XML",
          severity: "warning",
          message:
            "Keine eingebetteten Rechnungsdaten gefunden. Du kannst dieses PDF als Beleg speichern, es gilt aber nicht als strukturierte E-Rechnung. Ein PDF kann noch ausreichen, wenn du zugestimmt hast: für Leistungen bis Ende 2026, für Leistungen im Jahr 2027 nur, wenn der Aussteller im Vorjahr höchstens 800.000 € Gesamtumsatz hatte (§ 27 Abs. 38 UStG). Ob das auf diese Rechnung zutrifft, prüft die App nicht.",
        }),
      };
    return { inspection: inspectXML(xml) };
  }
  return { inspection: inspectXML(new TextDecoder().decode(data)) };
}
