import type {
  Address,
  Customer,
  InvoiceLine,
  InvoiceDraft,
  Product,
} from "../../bindings/papierleicht.de/e-rechnung-studio/internal/model/models";

export const DEFAULT_PAYMENT_DAYS = 14;
export const invoiceUnits = [
  ["C62", "Stück"],
  ["HUR", "Stunden"],
  ["DAY", "Tage"],
  ["KGM", "Kilogramm"],
  ["MTR", "Meter"],
  ["LS", "Pauschale"],
  ["WEE", "Wochen"],
  ["MON", "Monate"],
  ["LTR", "Liter"],
  ["MTK", "Quadratmeter"],
  ["MTQ", "Kubikmeter"],
  ["KMT", "Kilometer"],
  ["TNE", "Tonnen"],
] as const;

const leitwegId = /^\d{2,12}(-[a-z0-9]{1,30})?-\d{2}$/i;

/** A Leitweg-ID means a public buyer, which expects an XRechnung. */
export function defaultProfile(customer: Customer): string {
  return leitwegId.test(customer.buyerReference?.trim() ?? "")
    ? "xrechnung-ubl"
    : "facturx-en16931";
}

/** XRechnung needs an electronic address; the customer's e-mail is the usual one. */
export function fillElectronicAddress(customer: Customer) {
  if (customer.endpoint || !customer.email) return;
  customer.endpoint = customer.email;
  customer.endpointScheme = "EM";
}

/** API models contain JSON data only; copy them before editing or awaiting a request. */
export function clone<T>(value: T): T {
  return JSON.parse(JSON.stringify(value));
}

export function newAddress(): Address {
  return { street: "", additional: "", postcode: "", city: "", country: "DE" };
}

export function newCustomer(): Customer {
  return {
    id: "",
    name: "",
    contactName: "",
    email: "",
    vatId: "",
    endpoint: "",
    endpointScheme: "",
    buyerReference: "",
    paymentTermDays: DEFAULT_PAYMENT_DAYS,
    address: newAddress(),
    createdAt: "",
    updatedAt: "",
  };
}

export function newProduct(): Product {
  return {
    id: "",
    name: "",
    description: "",
    unit: "C62",
    unitPrice: "0.00",
    taxRate: "19",
    taxCategory: "S",
    createdAt: "",
    updatedAt: "",
  };
}

export function newLine(): InvoiceLine {
  return {
    id: crypto.randomUUID(),
    productId: "",
    name: "",
    description: "",
    quantity: "1",
    unit: "C62",
    unitPrice: "0.00",
    taxRate: "19",
    taxCategory: "S",
  };
}

export function isBlankLine(line: InvoiceLine): boolean {
  return (
    !line.name.trim() && !line.description.trim() && Number(line.unitPrice.replace(",", ".")) === 0
  );
}

/** UTC arithmetic keeps a payment period stable across daylight-saving changes. */
export function dueAfter(issueDate: string, days = DEFAULT_PAYMENT_DAYS): string {
  const date = new Date(issueDate + "T00:00:00Z");
  if (!Number.isFinite(date.getTime())) return "";
  date.setUTCDate(date.getUTCDate() + days);
  return date.toISOString().slice(0, 10);
}

export function paymentDaysBetween(issueDate: string, dueDate: string): number | undefined {
  const days = (Date.parse(dueDate) - Date.parse(issueDate)) / 86400000;
  return Number.isInteger(days) && days >= 0 ? days : undefined;
}

/** Reuse invoice content without carrying over its identity or accounting history. */
export function invoiceFromTemplate(
  source: InvoiceDraft,
  today = new Date().toLocaleDateString("sv-SE"),
): InvoiceDraft {
  const draft = clone(source);
  draft.id = "";
  draft.number = "";
  draft.automaticNumber = true;
  draft.kind = "invoice";
  draft.status = "draft";
  draft.issueDate = today;
  draft.deliveryDate = today;
  draft.periodStart = "";
  draft.periodEnd = "";
  draft.dueDate = source.dueDate
    ? dueAfter(
        today,
        paymentDaysBetween(source.issueDate, source.dueDate) ??
          source.customer.paymentTermDays ??
          DEFAULT_PAYMENT_DAYS,
      )
    : "";
  draft.paymentReference = "";
  draft.prepaidAmount = "";
  if (Number(source.prepaidAmount || "0") > 0) {
    draft.paymentTerms = "";
    draft.paymentMeansCode = "";
    draft.disablePaymentQR = false;
    draft.dueDate = dueAfter(today, source.customer.paymentTermDays ?? DEFAULT_PAYMENT_DAYS);
  }
  draft.correctsInvoiceId = "";
  draft.precedingInvoiceNumber = "";
  draft.precedingInvoiceDate = "";
  draft.correctionReason = "";
  delete draft.advancePayment;
  delete draft.sourceTaxes;
  delete draft.quoteId;
  delete draft.quoteNumber;
  delete draft.recurringId;
  delete draft.recurringName;
  delete draft.issuerSnapshot;
  delete draft.markedSentAt;
  draft.createdAt = "";
  draft.updatedAt = "";
  draft.issuedAt = "";
  draft.paidAt = "";
  draft.originalXml = "";
  draft.generatedXml = "";
  draft.generatedAt = "";
  draft.lastValidationState = "";
  return draft;
}
