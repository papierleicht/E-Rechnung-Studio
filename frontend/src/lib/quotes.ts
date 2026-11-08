import type {
  Company,
  Customer,
  InvoiceDraft,
  Quote,
  QuoteSummary,
} from "../../bindings/papierleicht.de/e-rechnung-studio/internal/model/models";
import { clone, defaultProfile, dueAfter, fillElectronicAddress, newLine } from "./invoiceDraft.ts";
import { companyTaxTreatment, initializePositionTaxes, invoiceTaxDefaults } from "./tax.ts";
import { invoiceFormats } from "./formats.ts";

/** Quotes stay valid for 30 days unless the user chooses another date. */
export const DEFAULT_QUOTE_DAYS = 30;

const statusLabels: Record<string, string> = {
  draft: "Entwurf",
  sent: "Offen",
  accepted: "Angenommen",
  rejected: "Abgelehnt",
  expired: "Abgelaufen",
  converted: "In Rechnung umgewandelt",
};

export function quoteStatusLabel(status: string): string {
  return statusLabels[status] ?? status;
}

/** Badge classes reuse the invoice colours: open is a warning, done is a success. */
export function quoteBadgeClass(status: string): string {
  if (status === "sent" || status === "expired") return "issued";
  if (status === "accepted" || status === "converted") return "paid";
  return status;
}

/** "expired" is derived from the validity; the customer may still accept. */
export function canConvertQuote(status: string): boolean {
  return ["sent", "expired", "accepted"].includes(status);
}

/** A new quote starts from the invoice defaults; due dates belong to invoices only. */
export function newQuote(
  base: InvoiceDraft,
  company: Company,
  today = new Date().toLocaleDateString("sv-SE"),
): Quote {
  const document = clone(base);
  document.id = "";
  document.number = "";
  document.status = "draft";
  document.kind = "invoice";
  document.issueDate = today;
  document.deliveryDate = "";
  document.dueDate = "";
  document.paymentReference = "";
  document.paymentTerms = "";
  document.notes = "";
  document.taxTreatment = companyTaxTreatment(company);
  document.taxSource = "company";
  document.taxExemptionReason = company.exemptionReason;
  document.lines = [newLine()];
  if (document.taxTreatment === "mixed")
    initializePositionTaxes(document, company.smallBusiness ? "small-business" : "standard");
  return {
    id: "",
    number: "",
    status: "draft",
    validUntil: dueAfter(today, DEFAULT_QUOTE_DAYS),
    document,
    createdAt: "",
    updatedAt: "",
  };
}

/** A new quote with the customer and positions of an earlier one, dated today. */
export function quoteFromTemplate(
  source: Quote,
  today = new Date().toLocaleDateString("sv-SE"),
): Quote {
  const document = clone(source.document);
  document.id = "";
  document.number = "";
  document.status = "draft";
  document.issueDate = today;
  document.deliveryDate = "";
  delete document.issuerSnapshot;
  document.lines = (document.lines ?? []).map((line) => ({ ...line, id: crypto.randomUUID() }));
  return {
    id: "",
    number: "",
    status: "draft",
    validUntil: dueAfter(today, DEFAULT_QUOTE_DAYS),
    document,
    createdAt: "",
    updatedAt: "",
  };
}

/** Same customer defaults as a new invoice, so the later invoice needs no rework. */
export function applyQuoteCustomer(quote: Quote, customer: Customer, company: Company) {
  const document = quote.document;
  const defaults = customer.invoiceSettings;
  document.customer = clone(customer);
  document.customerId = customer.id;
  document.buyerReference = customer.buyerReference;
  document.profile = defaults?.profile || defaultProfile(customer);
  if (!defaults?.profile && document.profile !== "facturx-en16931")
    fillElectronicAddress(document.customer);
  document.syntax = invoiceFormats.find((f) => f.id === document.profile)?.syntax || "cii";
  Object.assign(document, invoiceTaxDefaults(company, defaults));
  if (document.taxTreatment === "mixed")
    initializePositionTaxes(document, company.smallBusiness ? "small-business" : "standard");
  document.businessProcess = defaults?.businessProcess || "";
  document.paymentMeansCode = defaults?.paymentMeansCode || "";
}

export function filterQuotes(quotes: QuoteSummary[], query: string): QuoteSummary[] {
  const q = query.trim().toLocaleLowerCase("de");
  if (!q) return quotes;
  return quotes.filter((item) =>
    `${item.number} ${item.customerName} ${item.issueDate} ${quoteStatusLabel(item.status)}`
      .toLocaleLowerCase("de")
      .includes(q),
  );
}
