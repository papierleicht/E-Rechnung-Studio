/**
 * Totals, input checks and the document HTML for the browser demo.
 *
 * The markup follows `internal/pdfinvoice/invoice.html` and uses the same
 * stylesheet, so the preview looks like the desktop app's. The page plan is an
 * estimate; the desktop app takes it from the PDF typesetter.
 */
import invoiceCSS from "../../../internal/pdfinvoice/invoice.css?raw";
import catalog from "../../../internal/catalog/invoice.json" with { type: "json" };
import type {
  Company,
  InvoiceDraft,
  InvoiceLine,
  Totals,
  ValidationIssue,
  ValidationResult,
} from "../../bindings/papierleicht.de/e-rechnung-studio/internal/model/models";
import { cents, displayIBAN, fixed, germanDate, money, parseAmount, plainNumber } from "./money";

type Tax = { category: string; rate: string; reason: string };

export function lineTax(d: InvoiceDraft, line: InvoiceLine): Tax {
  let kind = d.taxTreatment;
  let reason = (d.taxExemptionReason ?? "").trim();
  if (kind === "mixed") {
    kind = line.taxTreatment || "standard";
    reason = (line.exemptionReason ?? "").trim();
  }
  const treatment = catalog.taxTreatments.find((t) => t.id === kind) ?? catalog.taxTreatments[0];
  const tax: Tax = { category: "S", rate: line.taxRate, reason };
  if (kind === "standard") {
    if (line.taxCategory) tax.category = line.taxCategory;
    if (parseAmount(line.taxRate) === 0 && tax.category === "S") tax.category = "Z";
  } else {
    tax.category = treatment.category;
    tax.rate = "0";
  }
  if (tax.category === "S" || tax.category === "Z") tax.reason = "";
  else if (!tax.reason) tax.reason = treatment.reason;
  return tax;
}

type TaxGroup = { category: string; rate: number; basis: number; tax: number; reason: string };

export type Calculation = {
  totals: Totals;
  lineTotals: string[];
  lineCents: number[];
  groups: TaxGroup[];
  lineSum: number;
  allowances: number;
  charges: number;
};

export function calculate(d: InvoiceDraft): Calculation {
  const groups = new Map<string, TaxGroup>();
  const group = (category: string, rate: number, reason: string) => {
    const key = `${category}/${rate}`;
    let g = groups.get(key);
    if (!g) groups.set(key, (g = { category, rate, basis: 0, tax: 0, reason }));
    return g;
  };
  const lineCents = (d.lines ?? []).map((line) => {
    const total = Math.round(parseAmount(line.quantity) * parseAmount(line.unitPrice) * 100) || 0;
    const tax = lineTax(d, line);
    group(tax.category, parseAmount(tax.rate) || 0, tax.reason).basis += total;
    return total;
  });
  let allowances = 0,
    charges = 0;
  for (const a of d.adjustments ?? []) {
    const amount = cents(a.amount) || 0;
    const category =
      d.taxTreatment === "standard" || d.taxTreatment === "mixed"
        ? a.taxCategory || "S"
        : lineTax(d, d.lines?.[0] ?? ({} as InvoiceLine)).category;
    const rate = category === "S" ? parseAmount(a.taxRate) || 0 : 0;
    group(category, rate, "").basis += a.charge ? amount : -amount;
    if (a.charge) charges += amount;
    else allowances += amount;
  }
  let net = 0,
    tax = 0;
  for (const g of groups.values()) {
    g.tax = Math.round((g.basis * g.rate) / 100);
    net += g.basis;
    tax += g.tax;
  }
  const lineSum = lineCents.reduce((sum, value) => sum + value, 0);
  const gross = net + tax;
  return {
    totals: { net: fixed(net), tax: fixed(tax), gross: fixed(gross), due: fixed(gross) },
    lineTotals: lineCents.map(fixed),
    lineCents,
    groups: [...groups.values()],
    lineSum,
    allowances,
    charges,
  };
}

function issue(field: string, message: string): ValidationIssue {
  return { stage: "input", ruleId: "INPUT", severity: "error", field, message };
}

/** The desktop app's input checks (subset of `validation.InputIssues`). */
export function validate(d: InvoiceDraft, c: Company): ValidationResult {
  const issues: ValidationIssue[] = [];
  const required = (field: string, value: string | undefined, label: string) => {
    if (!(value ?? "").trim()) issues.push(issue(field, `${label} fehlt noch.`));
  };
  required("company.name", c.name, "Dein Unternehmensname");
  required("company.address.street", c.address.street, "Die Straße deines Unternehmens");
  required("company.address.postcode", c.address.postcode, "Die Postleitzahl deines Unternehmens");
  required("company.address.city", c.address.city, "Der Ort deines Unternehmens");
  if (!c.vatId.trim() && !c.taxNumber.trim())
    issues.push(issue("company.taxNumber", "Trag deine Steuernummer oder USt-IdNr. ein."));
  required("customer.name", d.customer.name, "Der Name des Rechnungsempfängers");
  required("customer.address.city", d.customer.address.city, "Der Ort des Rechnungsempfängers");
  required(
    "customer.address.country",
    d.customer.address.country,
    "Das Land des Rechnungsempfängers (z. B. DE)",
  );
  if (d.profile.startsWith("xrechnung-")) {
    required("buyerReference", d.buyerReference, "Die Käuferreferenz / Leitweg-ID für XRechnung");
    required("company.contactName", c.contactName, "Der Ansprechpartner für XRechnung");
    required("company.phone", c.phone, "Die Telefonnummer für Rückfragen");
    required("company.email", c.email, "Die Kontakt-E-Mail deines Unternehmens");
    required("company.endpoint", c.endpoint, "Die elektronische Adresse deines Unternehmens");
    required("customer.endpoint", d.customer.endpoint, "Die elektronische Adresse des Empfängers");
  }
  if (!/^\d{4}-\d{2}-\d{2}$/.test(d.issueDate))
    issues.push(issue("issueDate", "Wähle ein gültiges Rechnungsdatum."));
  if (d.periodStart && d.periodEnd && d.periodEnd < d.periodStart)
    issues.push(
      issue("periodEnd", "Das Ende des Leistungszeitraums darf nicht vor dem Beginn liegen."),
    );
  if (!d.dueDate && !d.paymentTerms.trim())
    issues.push(
      issue(
        "paymentTerms",
        "Die Zahlungsangaben fehlen. Trag ein Fälligkeitsdatum oder Zahlungsbedingungen ein, z. B. „Zahlbar innerhalb von 14 Tagen“.",
      ),
    );
  if (d.kind === "corrected-invoice" && !d.precedingInvoiceNumber?.trim())
    issues.push(issue("precedingInvoiceNumber", "Die Nummer der korrigierten Rechnung fehlt."));
  const lines = d.lines ?? [];
  if (!lines.length)
    issues.push(
      issue(
        "lines",
        "Die Rechnung hat noch keine Position. Füge eine Leistung oder einen Artikel hinzu.",
      ),
    );
  lines.forEach((line, i) => {
    const label = `Position ${i + 1}: `;
    required(`lines.${i}.name`, line.name, label + "Die Bezeichnung");
    const quantity = parseAmount(line.quantity);
    if (!Number.isFinite(quantity))
      issues.push(
        issue(`lines.${i}.quantity`, label + "Die Menge muss eine Zahl sein, z. B. 19,50."),
      );
    else if (d.kind !== "corrected-invoice" && quantity <= 0)
      issues.push(issue(`lines.${i}.quantity`, label + "Die Menge muss größer als 0 sein."));
    const price = parseAmount(line.unitPrice);
    if (!Number.isFinite(price))
      issues.push(
        issue(`lines.${i}.unitPrice`, label + "Der Einzelpreis muss eine Zahl sein, z. B. 19,50."),
      );
    else if (price < 0)
      issues.push(
        issue(`lines.${i}.unitPrice`, label + "Der Einzelpreis darf nicht negativ sein."),
      );
  });
  if (issues.length) return { state: "invalid", issues };
  return { state: "valid", issues: [] };
}

const esc = (value: string | undefined | null) =>
  String(value ?? "")
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;");
/** Like the PDF: at least two decimals, more if the price has them. */
function price(value: string) {
  const amount = parseAmount(value);
  if (!Number.isFinite(amount)) return value;
  const decimals = Math.max(2, (value.trim().split(/[.,]/)[1] ?? "").length);
  return amount.toFixed(decimals).replace(".", ",");
}

const units: Record<string, string> = {
  C62: "Stück",
  HUR: "Stunden",
  DAY: "Tage",
  KGM: "kg",
  MTR: "Meter",
};

function title(d: InvoiceDraft, quote: boolean) {
  if (quote) return "Angebot";
  if (d.kind === "credit-note") return "Gutschrift";
  if (d.kind === "corrected-invoice") return "Rechnungskorrektur";
  if (d.advancePayment) return "Anzahlungsrechnung";
  return "Rechnung";
}

function paymentLabel(code: string) {
  return code === "10"
    ? "Barzahlung"
    : code === "97"
      ? "Zahlung nach Vereinbarung"
      : code === "48"
        ? "Kartenzahlung"
        : "";
}

export type DocumentOptions = { quoteValidUntil?: string };

/** Mirrors `splitSellerName` in internal/pdfinvoice/heading.go. */
function sellerHeading(name: string): { html: string; lines: number } {
  const max = 32;
  name = name.trim();
  const length = (text: string) => Array.from(text).length;
  if (length(name) <= max) return { html: `<h1>${esc(name)}</h1>`, lines: 1 };
  for (const dash of [" – ", " — ", " - "]) {
    const at = name.indexOf(dash);
    if (at < 0) continue;
    const head = name.slice(0, at).trim();
    const rest = name.slice(at + dash.length).trim();
    if (head && rest && length(head) <= max)
      return {
        html: `<h1>${esc(head)}</h1><p class="seller-subtitle">${esc(rest)}</p>`,
        lines: 1 + Math.ceil(length(rest) / 55),
      };
  }
  return length(name) > 2 * max
    ? { html: `<h1 class="seller-compact">${esc(name)}</h1>`, lines: Math.ceil(length(name) / 50) }
    : { html: `<h1>${esc(name)}</h1>`, lines: 2 };
}

/** Estimated height in mm of each flow atom; pages hold 253 mm of content. */
function plan(atoms: number[]): number[] {
  const pages: number[] = [];
  let page = 1,
    used = 0;
  for (const height of atoms) {
    if (used > 0 && used + height > 240) {
      page++;
      used = 0;
    }
    used += height;
    pages.push(page);
  }
  return pages;
}

const textLines = (text: string, perLine: number) =>
  text.split("\n").reduce((sum, part) => sum + Math.max(1, Math.ceil(part.length / perLine)), 0);

export function documentHTML(d: InvoiceDraft, c: Company, options: DocumentOptions = {}): string {
  const quote = options.quoteValidUntil !== undefined;
  const calc = calculate(d);
  const name = title(d, quote);
  const number = esc(d.number || (quote ? "Angebotsentwurf" : "Entwurf"));
  const a = c.address,
    b = d.customer.address;
  const accounts = [
    { iban: c.iban, bic: c.bic, name: c.bankName },
    ...(c.additionalAccounts ?? []),
  ].filter((x) => x.iban);
  const blocks: { html: string; height: number }[] = [];

  const logo = c.logo
    ? `<div class="invoice-logo"><p><img src="data:image/png;base64,${esc(c.logo)}" style="max-width:45mm;max-height:22mm"/></p></div><p>${number}</p>`
    : number;
  const seller = sellerHeading(c.name);
  blocks.push({
    height: (c.logo ? 62 : 40) + (seller.lines - 1) * 8,
    html: `<table class="invoice-head"><colgroup><col data-width="72*"/><col data-width="28*"/></colgroup><tbody><tr data-flow><td><p class="eyebrow" id="edit-company">${name}</p>${seller.html}<p class="sender">${esc(a.street)}${a.additional ? "<br/>" + esc(a.additional) : ""}<br/>${esc(a.postcode)} ${esc(a.city)}${a.country !== "DE" ? " · " + esc(a.country) : ""}</p></td><td class="identity">${logo}</td></tr></tbody></table>`,
  });
  const contact = d.customer.contactName ? esc(d.customer.contactName) + "<br/>" : "";
  blocks.push({
    height: 64,
    html: `<div class="recipient" data-flow><p class="return-address" id="edit-customer">${esc(c.name)} · ${esc(a.street)} · ${esc(a.postcode)} ${esc(a.city)}</p><p class="recipient-name">${esc(d.customer.name)}</p><p>${contact}${esc(b.street)}${b.additional ? "<br/>" + esc(b.additional) : ""}<br/>${esc(b.postcode)} ${esc(b.city)}<br/>${esc(b.country)}</p>${d.customer.vatId ? `<p class="small">USt-IdNr.: ${esc(d.customer.vatId)}</p>` : ""}${d.customer.email ? `<p class="small">${esc(d.customer.email)}</p>` : ""}</div>`,
  });

  const meta: string[] = [];
  const metaRow = (label: string, value: string) =>
    meta.push(
      `<tr data-flow><td></td><td class="date-label">${label}</td><td class="date-value"><p style="margin:0">${value}</p></td></tr>`,
    );
  const noteRow = (text: string) =>
    meta.push(`<tr data-flow><td><p class="small">${text}</p></td><td></td><td></td></tr>`);
  meta.push(
    `<tr data-flow><td><h2 id="edit-dates">${name} ${number}</h2></td><td class="date-label">${quote ? "Angebotsdatum" : "Rechnungsdatum"}</td><td class="date-value">${germanDate(d.issueDate)}</td></tr>`,
  );
  if (quote && options.quoteValidUntil) metaRow("Gültig bis", germanDate(options.quoteValidUntil));
  if (d.precedingInvoiceNumber)
    noteRow(
      `Bezug: Rechnung ${esc(d.precedingInvoiceNumber)}${d.precedingInvoiceDate ? " vom " + germanDate(d.precedingInvoiceDate) : ""}`,
    );
  if (d.buyerReference) noteRow(`Kundenreferenz: ${esc(d.buyerReference)}`);
  if (d.deliveryDate && !d.periodStart)
    metaRow(
      d.advancePayment ? "Voraussichtliche Leistung" : "Leistungsdatum",
      germanDate(d.deliveryDate),
    );
  if (d.periodStart) metaRow("Leistung von", germanDate(d.periodStart));
  if (d.periodEnd) metaRow("Leistung bis", germanDate(d.periodEnd));
  if (!quote && d.dueDate) metaRow("Fällig am", germanDate(d.dueDate));
  blocks.push({
    height: 8 + meta.length * 7,
    html: `<table class="invoice-meta"><colgroup><col data-width="57*"/><col data-width="24*"/><col data-width="19*"/></colgroup><tbody>${meta.join("")}</tbody></table>`,
  });

  const rows = (d.lines ?? []).map((line, i) => {
    const tax = lineTax(d, line);
    const details = line.description ? `<p class="description">${esc(line.description)}</p>` : "";
    return {
      height: 11 + (line.description ? textLines(line.description, 55) * 4 : 0),
      html: `<tr data-flow><td><p style="margin:0">${i + 1}</p></td><td>${line.name ? `<b>${esc(line.name)}</b>` : ""}${details}</td><td class="num">${esc(plainNumber(line.quantity))}<br/><span class="small">${esc(units[line.unit] ?? line.unit)}</span></td><td class="num">${esc(price(line.unitPrice))}</td><td class="num">${plainNumber(tax.rate)} %</td><td class="num">${money(calc.lineCents[i])}</td></tr>`,
    };
  });
  blocks.push({
    height: 9 + rows.reduce((s, r) => s + r.height, 0),
    html: `<table class="positions"><colgroup><col data-width="6*"/><col data-width="37*"/><col data-width="12*"/><col data-width="17*"/><col data-width="11*"/><col data-width="17*"/></colgroup><thead><tr><th>Pos.</th><th><p id="edit-lines">Beschreibung</p></th><th class="num">Menge</th><th class="num">Preis netto</th><th class="num">USt.</th><th class="num">Netto</th></tr></thead><tbody>${rows.map((r) => r.html).join("")}</tbody></table>`,
  });

  const adjustments = d.adjustments ?? [];
  if (adjustments.length)
    blocks.push({
      height: 10 + adjustments.length * 8,
      html: `<table class="adjustments"><colgroup><col data-width="57*"/><col data-width="24*"/><col data-width="19*"/></colgroup><thead><tr><th>Nachlässe und Zuschläge</th><th class="num">USt.</th><th class="num">Netto</th></tr></thead><tbody>${adjustments
        .map(
          (x) =>
            `<tr data-flow><td><p style="margin:0">${x.charge ? "Zuschlag" : "Rabatt"}: ${esc(x.reason)}</p></td><td class="num">${plainNumber(x.taxRate || "0")} %</td><td class="num">${x.charge ? "" : "−"}${money(cents(x.amount) || 0)} ${esc(d.currency)}</td></tr>`,
        )
        .join("")}</tbody></table>`,
    });

  const notes: { caption: string; text: string }[] = [];
  for (const g of calc.groups)
    if (g.reason) notes.push({ caption: "Steuerlicher Hinweis", text: g.reason });
  if (!quote) {
    const terms =
      d.paymentTerms.trim() ||
      (d.dueDate ? `Zahlbar ohne Abzug bis ${germanDate(d.dueDate)}.` : "");
    if (terms) notes.push({ caption: "Zahlungsbedingungen", text: terms });
    const means = paymentLabel(d.paymentMeansCode || c.paymentMeansCode || "58");
    if (means) notes.push({ caption: "Zahlungsart", text: means });
    if (d.paymentReference) notes.push({ caption: "Zahlungsreferenz", text: d.paymentReference });
  }
  if (d.notes.trim()) notes.push({ caption: "Hinweis", text: d.notes.trim() });

  const totals: { label: string; amount: number; grand?: boolean }[] = [];
  if (adjustments.length) {
    totals.push({ label: "Positionssumme", amount: calc.lineSum });
    if (calc.allowances) totals.push({ label: "Nachlässe", amount: -calc.allowances });
    if (calc.charges) totals.push({ label: "Zuschläge", amount: calc.charges });
  }
  totals.push({ label: "Netto", amount: cents(calc.totals.net) });
  for (const g of calc.groups)
    totals.push({ label: `Umsatzsteuer ${plainNumber(g.rate)} %`, amount: g.tax });
  totals.push({ label: "Gesamt", amount: cents(calc.totals.gross), grand: true });
  const first = notes[0];
  blocks.push({
    height: 12 + totals.length * 8,
    html: `<table class="document-bottom first-bottom"><colgroup><col data-width="57*"/><col data-width="24*"/><col data-width="19*"/></colgroup><tbody><tr data-flow><td class="payment">${first ? `<p class="caption">${first.caption}</p><p class="paragraph">${esc(first.text)}</p>` : ""}</td><td class="total-labels">${totals.map((t, i) => `<p class="total-entry ${t.grand ? "grand" : ""}"${i === 0 ? ' id="edit-payment"' : ""}>${t.label}</p>`).join("")}</td><td class="total-values">${totals.map((t) => `<p class="total-entry ${t.grand ? "grand" : ""}">${money(t.amount)} ${esc(d.currency)}</p>`).join("")}</td></tr></tbody></table>`,
  });
  if (notes.length > 1)
    blocks.push({
      height: notes.slice(1).reduce((s, n) => s + 9 + textLines(n.text, 70) * 4, 0),
      html: `<table class="document-bottom"><colgroup><col data-width="57*"/><col data-width="24*"/><col data-width="19*"/></colgroup><tbody>${notes
        .slice(1)
        .map(
          (n, i) =>
            `<tr data-flow><td class="payment"><p class="caption">${n.caption}</p><p class="paragraph" id="edit-payment-1-${i}">${esc(n.text)}</p></td><td></td><td></td></tr>`,
        )
        .join("")}</tbody></table>`,
    });

  const footer = `<footer class="pagefooter"><table><colgroup><col data-width="33*"/><col data-width="34*"/><col data-width="33*"/></colgroup><tbody><tr><td>${esc(c.name)}<br/>${esc(a.street)}<br/>${esc(a.postcode)} ${esc(a.city)}</td><td>${accounts
    .slice(0, 2)
    .map(
      (x) =>
        `${x.name ? esc(x.name) + "<br/>" : ""}IBAN: ${esc(displayIBAN(x.iban))}${x.bic ? "<br/>BIC: " + esc(x.bic) : ""}<br/>`,
    )
    .join(
      "",
    )}</td><td>${c.contactName ? esc(c.contactName) + "<br/>" : ""}${c.email ? esc(c.email) + "<br/>" : ""}${c.phone ? esc(c.phone) + "<br/>" : ""}${c.vatId ? "USt-IdNr.: " + esc(c.vatId) : ""}${c.taxNumber ? "<br/>Steuernummer: " + esc(c.taxNumber) : ""}</td></tr></tbody></table></footer>`;

  // Rows are the atoms of the shared page plan; estimate their pages.
  let html = blocks.map((b) => b.html).join("\n");
  const heights: number[] = [];
  for (const b of blocks) {
    const count = (b.html.match(/data-flow/g) ?? []).length;
    if (b.html.startsWith('<table class="positions"'))
      heights.push(...rows.map((r, i) => r.height + (i === 0 ? 9 : 0)));
    else for (let i = 0; i < count; i++) heights.push(b.height / count);
  }
  const pages = plan(heights);
  let index = 0;
  html = html.replace(/data-flow(?=[ >])/g, () => `data-flow-page="${pages[index++]}"`);
  const pageCount = Math.max(1, ...pages);
  return `<html lang="de"><head><meta charset="utf-8"><style>${invoiceCSS}</style></head><body data-page-count="${pageCount}">${footer}${html}</body></html>`;
}

/** Same families as the desktop renderer, resolved to fonts installed locally. */
export const previewFonts = [
  ["sans", 400, "'Helvetica Neue'), local('Helvetica'), local('Arial'), local('Liberation Sans"],
  [
    "sans",
    700,
    "'Helvetica Neue Bold'), local('Helvetica Bold'), local('Arial Bold'), local('Arial-BoldMT'), local('Liberation Sans Bold",
  ],
  [
    "serif",
    400,
    "'Crimson Pro'), local('Georgia'), local('Times New Roman'), local('Liberation Serif",
  ],
  [
    "serif",
    700,
    "'Crimson Pro Bold'), local('Georgia Bold'), local('Georgia-Bold'), local('Times New Roman Bold'), local('Liberation Serif Bold",
  ],
]
  .map(
    ([family, weight, sources]) =>
      `@font-face{font-family:'${family}';font-weight:${weight};src:local(${sources}')}`,
  )
  .join("\n");
