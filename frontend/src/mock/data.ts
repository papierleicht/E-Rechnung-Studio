/**
 * Fictional sample data for the browser demo. All dates are relative to today,
 * so the overview, due dates and reminders stay plausible whenever the demo is
 * opened. Names, addresses and IBANs are invented (IBANs are test numbers).
 */
import type {
  Company,
  Customer,
  InvoiceDraft,
  InvoiceLine,
  Product,
  Quote,
  ReceivedInvoice,
} from "../../bindings/papierleicht.de/e-rechnung-studio/internal/model/models";
import type {
  BookingTreatment,
  DatevSettings,
  FinanceSettings,
  Payment,
  Reminder,
  ReminderSettings,
} from "../../bindings/papierleicht.de/e-rechnung-studio/internal/store/models";
import type {
  Account,
  Booking,
  Connection,
} from "../../bindings/papierleicht.de/e-rechnung-studio/internal/banking/models";
import type {
  LogEntry,
  Settings as MailSettings,
} from "../../bindings/papierleicht.de/e-rechnung-studio/internal/mail/models";
import type {
  Connection as ShopConnection,
  Order as ShopOrder,
} from "../../bindings/papierleicht.de/e-rechnung-studio/internal/shops/models";
import { addDays, day, stamp } from "./money";

export const year = Number(day().slice(0, 4));

export const company: Company = {
  name: "Atelier Lindner",
  contactName: "Mara Lindner",
  email: "hallo@atelier-lindner.example",
  phone: "+49 341 555 0192",
  vatId: "DE287654321",
  taxNumber: "231/456/78901",
  endpoint: "hallo@atelier-lindner.example",
  endpointScheme: "EM",
  iban: "DE02120300000000202051",
  bic: "BYLADEM1001",
  bankName: "Demo-Bank Leipzig",
  smallBusiness: false,
  exemptionReason: "",
  invoiceTaxPreset: "standard",
  taxStatusUnclear: false,
  defaultTaxTreatment: "standard",
  additionalAccounts: [],
  paymentMeansCode: "58",
  setupCompleted: true,
  address: {
    street: "Karl-Heine-Straße 42",
    additional: "",
    postcode: "04229",
    city: "Leipzig",
    country: "DE",
  },
  logo: "",
};

function customer(
  id: string,
  value: Partial<Customer> & Pick<Customer, "name" | "address">,
): Customer {
  return {
    id,
    contactName: "",
    email: "",
    vatId: "",
    endpoint: "",
    endpointScheme: "",
    buyerReference: "",
    paymentTermDays: 14,
    createdAt: stamp(day(-160)),
    updatedAt: stamp(day(-160)),
    ...value,
  };
}

export const customers: Customer[] = [
  customer("cus-sonnenkorn", {
    name: "Bäckerei Sonnenkorn GmbH",
    contactName: "Jana Kühn",
    email: "buchhaltung@sonnenkorn.example",
    vatId: "DE312345678",
    address: {
      street: "Brühl 8",
      additional: "",
      postcode: "04109",
      city: "Leipzig",
      country: "DE",
    },
  }),
  customer("cus-musterstadt", {
    name: "Stadt Musterstadt – Amt für Digitales",
    contactName: "Herr Okafor",
    email: "rechnungseingang@musterstadt.example",
    endpoint: "rechnungseingang@musterstadt.example",
    endpointScheme: "EM",
    buyerReference: "04011000-1234512345-06",
    paymentTermDays: 30,
    invoiceSettings: {
      profile: "xrechnung-cii",
      kind: "invoice",
      taxTreatment: "standard",
      taxExemptionReason: "",
      businessProcess: "",
    },
    address: {
      street: "Rathausplatz 1",
      additional: "",
      postcode: "01067",
      city: "Dresden",
      country: "DE",
    },
  }),
  customer("cus-weber", {
    name: "Weber & Partner Steuerberatung",
    contactName: "Dr. Selin Weber",
    email: "kanzlei@weber-partner.example",
    paymentTermDays: 30,
    address: {
      street: "Gohliser Straße 17",
      additional: "2. OG",
      postcode: "04155",
      city: "Leipzig",
      country: "DE",
    },
  }),
  customer("cus-morgenrot", {
    name: "Café Morgenrot",
    contactName: "Lea Brandt",
    email: "lea@cafe-morgenrot.example",
    address: {
      street: "Merseburger Straße 101",
      additional: "",
      postcode: "04177",
      city: "Leipzig",
      country: "DE",
    },
  }),
  customer("cus-nordlys", {
    name: "Nordlys Studio ApS",
    contactName: "Frederik Holm",
    email: "accounts@nordlys.example",
    vatId: "DK12345678",
    paymentTermDays: 30,
    invoiceSettings: {
      profile: "facturx-en16931",
      kind: "invoice",
      taxTreatment: "reverse-charge",
      taxExemptionReason: "",
      businessProcess: "",
    },
    address: {
      street: "Vesterbrogade 24",
      additional: "",
      postcode: "1620",
      city: "København",
      country: "DK",
    },
  }),
];

function product(
  id: string,
  name: string,
  description: string,
  unit: string,
  unitPrice: string,
): Product {
  return {
    id,
    name,
    description,
    unit,
    unitPrice,
    taxRate: "19",
    taxCategory: "S",
    createdAt: stamp(day(-170)),
    updatedAt: stamp(day(-170)),
  };
}

export const products: Product[] = [
  product(
    "prd-logo",
    "Logo-Entwicklung",
    "Konzept, drei Entwürfe, Reinzeichnung in allen Formaten",
    "C62",
    "1450.00",
  ),
  product("prd-web", "Webdesign", "Gestaltung und Umsetzung nach Aufwand", "HUR", "95.00"),
  product(
    "prd-hosting",
    "Hosting & Wartung",
    "Monatliche Pflege, Updates und Backups der Website",
    "C62",
    "39.00",
  ),
  product(
    "prd-foto",
    "Fotoshooting",
    "Ganztägiges Shooting inkl. Bildbearbeitung",
    "DAY",
    "890.00",
  ),
  product("prd-flyer", "Druckvorlage Flyer", "DIN lang, beidseitig, druckfertig", "C62", "240.00"),
];

let lineCounter = 0;
function line(
  productId: string,
  quantity: string,
  override: Partial<InvoiceLine> = {},
): InvoiceLine {
  const p = products.find((item) => item.id === productId)!;
  return {
    id: `line-${++lineCounter}`,
    productId,
    name: p.name,
    description: p.description,
    quantity,
    unit: p.unit,
    unitPrice: p.unitPrice,
    taxRate: p.taxRate,
    taxCategory: p.taxCategory,
    ...override,
  };
}

export function blankDraft(): InvoiceDraft {
  const today = day();
  return {
    id: "",
    number: "",
    automaticNumber: true,
    kind: "invoice",
    status: "draft",
    customerId: "",
    customer: customer("", {
      name: "",
      address: { street: "", additional: "", postcode: "", city: "", country: "DE" },
    }),
    issueDate: today,
    deliveryDate: today,
    deliveryCountry: "",
    deliveryCity: "",
    deliveryPostcode: "",
    periodStart: "",
    periodEnd: "",
    dueDate: addDays(today, 14),
    currency: "EUR",
    buyerReference: "",
    businessProcess: "",
    paymentReference: "",
    notes: "",
    paymentTerms: "",
    profile: "facturx-en16931",
    syntax: "cii",
    taxTreatment: "standard",
    taxExemptionReason: "",
    taxSource: "company",
    lines: [
      {
        id: "line-new",
        productId: "",
        name: "",
        description: "",
        quantity: "1",
        unit: "C62",
        unitPrice: "0.00",
        taxRate: "19",
        taxCategory: "S",
      },
    ],
    adjustments: [],
    createdAt: "",
    updatedAt: "",
    issuedAt: "",
    paidAt: "",
  };
}

function invoice(
  seq: number,
  customerId: string,
  issued: number,
  lines: InvoiceLine[],
  extra: Partial<InvoiceDraft> = {},
): InvoiceDraft {
  const c = customers.find((item) => item.id === customerId)!;
  const issueDate = day(issued);
  const number = seq ? `RE-${year}-${String(seq).padStart(4, "0")}` : "";
  const settings = c.invoiceSettings;
  return {
    ...blankDraft(),
    id: seq ? `inv-${seq}` : "inv-draft",
    number,
    automaticNumber: true,
    status: seq ? "issued" : "draft",
    customerId,
    customer: structuredClone(c),
    issueDate,
    deliveryDate: issueDate,
    dueDate: addDays(issueDate, c.paymentTermDays ?? 14),
    buyerReference: c.buyerReference,
    paymentReference: number,
    profile: settings?.profile ?? "facturx-en16931",
    syntax: "cii",
    taxTreatment: settings?.taxTreatment ?? "standard",
    lines,
    issuerSnapshot: seq ? structuredClone(company) : null,
    createdAt: stamp(issueDate, 9),
    updatedAt: stamp(issueDate, 11),
    issuedAt: seq ? stamp(issueDate, 11) : "",
    lastValidationState: seq ? "valid" : "",
    ...extra,
  };
}

export const invoices: InvoiceDraft[] = [
  invoice(1, "cus-sonnenkorn", -96, [
    line("prd-logo", "1"),
    line("prd-web", "6", { description: "Anpassung Verpackungsetiketten" }),
  ]),
  invoice(
    2,
    "cus-weber",
    -71,
    [
      line("prd-web", "18", {
        description: "Relaunch der Kanzlei-Website, laut Angebot AN-" + year + "-0001",
      }),
      line("prd-hosting", "3"),
    ],
    {
      quoteId: "quo-1",
      quoteNumber: `AN-${year}-0001`,
    },
  ),
  invoice(3, "cus-musterstadt", -55, [
    line("prd-flyer", "2", {
      name: "Druckvorlage Bürgerinformation",
      description: "Faltblatt „Digitale Verwaltung“, zwei Sprachfassungen",
    }),
    line("prd-web", "4"),
  ]),
  invoice(4, "cus-morgenrot", -38, [
    line("prd-foto", "1", { description: "Food- und Raumfotos für Karte und Website" }),
    line("prd-flyer", "1"),
  ]),
  invoice(5, "cus-sonnenkorn", -20, [
    line("prd-hosting", "6", { description: "Hosting & Wartung, erstes Halbjahr" }),
    line("prd-web", "3"),
  ]),
  invoice(6, "cus-nordlys", -9, [
    line("prd-logo", "1", {
      name: "Brand identity",
      description: "Logosystem und Typografie-Richtlinien",
    }),
  ]),
  invoice(7, "cus-weber", -3, [
    line("prd-web", "8", { description: "Barrierefreiheits-Überarbeitung nach BITV" }),
    line("prd-hosting", "1"),
  ]),
  invoice(
    0,
    "cus-morgenrot",
    -1,
    [
      line("prd-flyer", "1", { name: "Druckvorlage Wochenkarte", description: "A4, einseitig" }),
      line("prd-web", "2"),
    ],
    {
      notes: "Vielen Dank für die gute Zusammenarbeit!",
    },
  ),
];

export const counters = { invoice: 7, quote: 3 };

function quote(
  seq: number,
  customerId: string,
  issued: number,
  lines: InvoiceLine[],
  extra: Partial<Quote>,
): Quote {
  const document = invoice(0, customerId, issued, lines, {
    id: "",
    number: `AN-${year}-${String(seq).padStart(4, "0")}`,
  });
  return {
    id: `quo-${seq}`,
    number: document.number,
    status: "draft",
    validUntil: addDays(document.issueDate, 30),
    automaticNumber: true,
    document,
    createdAt: stamp(document.issueDate, 9),
    updatedAt: stamp(document.issueDate, 9),
    ...extra,
  };
}

export const quotes: Quote[] = [
  quote(
    1,
    "cus-weber",
    -84,
    [
      line("prd-web", "18", { description: "Relaunch der Kanzlei-Website" }),
      line("prd-hosting", "3"),
    ],
    {
      status: "converted",
      invoiceId: "inv-2",
      sentAt: stamp(day(-84), 15),
      decidedAt: stamp(day(-77), 10),
      convertedAt: stamp(day(-71), 10),
    },
  ),
  quote(
    2,
    "cus-morgenrot",
    -6,
    [
      line("prd-foto", "1", { description: "Saisonales Shooting Sommerkarte" }),
      line("prd-web", "5", { description: "Online-Reservierung einbinden" }),
    ],
    {
      status: "sent",
      sentAt: stamp(day(-6), 16),
    },
  ),
  quote(
    3,
    "cus-sonnenkorn",
    -2,
    [
      line("prd-logo", "1", {
        name: "Logo-Variante Filiale",
        description: "Ableitung des bestehenden Logos",
      }),
      line("prd-flyer", "3"),
    ],
    {},
  ),
];

function inspection(value: {
  number: string;
  issued: number;
  due: number;
  seller: string;
  net: string;
  tax: string;
  gross: string;
  iban?: string;
  syntax?: string;
  profile?: string;
  state?: string;
}) {
  return {
    invoiceNumber: value.number,
    issueDate: day(value.issued),
    dueDate: day(value.due),
    iban: value.iban ?? "",
    syntax: value.syntax ?? "CII",
    profile: value.profile ?? "ZUGFeRD / Factur-X · EN 16931",
    sellerName: value.seller,
    buyerName: company.name,
    currency: "EUR",
    totals: { net: value.net, tax: value.tax, gross: value.gross, due: value.gross },
    validation: { state: value.state ?? "valid", issues: [] },
    inputTax: { allowed: true, tax: value.tax },
  };
}

export const received: ReceivedInvoice[] = [
  {
    id: "rcv-serverwerk",
    filename: "serverwerk-rechnung-" + year + "-0815.xml",
    kind: "xml",
    importedAt: stamp(day(-44)),
    size: 14_212,
    status: "active",
    history: [],
    inspection: inspection({
      number: "SW-" + year + "-0815",
      issued: -45,
      due: -31,
      seller: "Serverwerk Hosting GmbH",
      net: "20.00",
      tax: "3.80",
      gross: "23.80",
      iban: "DE89370400440532013000",
      syntax: "UBL",
      profile: "XRechnung 3.0 · UBL",
    }),
  },
  {
    id: "rcv-pixelwerk",
    filename: "pixelwerk-lizenz.pdf",
    kind: "pdf",
    importedAt: stamp(day(-10)),
    size: 88_310,
    status: "active",
    history: [],
    inspection: inspection({
      number: "PW-20391",
      issued: -10,
      due: 4,
      seller: "Pixelwerk Software GmbH",
      net: "59.99",
      tax: "11.40",
      gross: "71.39",
      iban: "DE75512108001245126199",
    }),
  },
  {
    id: "rcv-hansen",
    filename: "druckerei-hansen-4711.pdf",
    kind: "pdf",
    importedAt: stamp(day(-21)),
    size: 132_044,
    status: "active",
    history: [],
    inspection: inspection({
      number: "4711",
      issued: -22,
      due: -4,
      seller: "Druckerei Hansen KG",
      net: "346.50",
      tax: "65.84",
      gross: "412.34",
      iban: "DE12500105170648489890",
    }),
  },
  {
    id: "rcv-lehmann",
    filename: "buerobedarf-lehmann-quittung.pdf",
    kind: "pdf",
    importedAt: stamp(day(-30)),
    size: 41_998,
    status: "active",
    history: [],
    inspection: {
      ...inspection({
        number: "",
        issued: -30,
        due: -30,
        seller: "",
        net: "",
        tax: "",
        gross: "",
        syntax: "",
        profile: "",
        state: "unsupported",
      }),
      issueDate: "",
      dueDate: "",
      validation: {
        state: "unsupported",
        issues: [
          {
            stage: "import",
            ruleId: "PDF-NO-XML",
            severity: "warning",
            class: "unchecked",
            message:
              "Keine eingebetteten Rechnungsdaten gefunden. Du kannst dieses PDF als Beleg speichern, es gilt aber nicht als strukturierte E-Rechnung. Ein PDF kann noch ausreichen, wenn du zugestimmt hast: für Leistungen bis Ende 2026, für Leistungen im Jahr 2027 nur, wenn der Aussteller im Vorjahr höchstens 800.000 € Gesamtumsatz hatte (§ 27 Abs. 38 UStG). Ob das auf diese Rechnung zutrifft, prüft die App nicht.",
          },
        ],
      },
      inputTax: {
        allowed: false,
        blocker: "unchecked",
        short: "nicht geprüft",
        reason:
          "Die App konnte diese Rechnung nicht vollständig prüfen. Die App rechnet die Steuer deshalb nicht von selbst ein. Du entscheidest unten, ob und wie viel davon zählt.",
      },
    },
  },
  {
    id: "rcv-planbuero",
    filename: "planbuero-schlussrechnung-" + year + "-0042.xml",
    kind: "xml",
    importedAt: stamp(day(-3)),
    size: 15_880,
    status: "active",
    history: [],
    inspection: {
      ...inspection({
        number: "PB-" + year + "-0042",
        issued: -4,
        due: 10,
        seller: "Planbüro Winter",
        net: "1000.00",
        tax: "190.00",
        gross: "1190.00",
        iban: "DE89370400440532013000",
      }),
      // A final invoice deducts the payments made before (BT-113).
      totals: { net: "1000.00", tax: "190.00", gross: "1190.00", due: "590.00", prepaid: "600.00" },
      inputTax: {
        allowed: true,
        tax: "94.20",
        reason:
          "Eine Endrechnung weist die Steuer der ganzen Leistung aus. Gab es Anzahlungsrechnungen, entfällt auf die Endrechnung in der Regel nur die Steuer der Restzahlung (Abschnitt 15.3 Abs. 5 UStAE). Die App rechnet deshalb nur diesen Anteil ein. Gab es keine Anzahlungsrechnungen, entscheide im Beleg anders.",
      },
    },
  },
  {
    id: "rcv-messebau",
    filename: "messebau-anzahlung-" + year + "-0311.xml",
    kind: "xml",
    importedAt: stamp(day(-2)),
    size: 14_730,
    status: "active",
    history: [],
    inspection: {
      ...inspection({
        number: "MB-" + year + "-0311",
        issued: -3,
        due: 11,
        seller: "Messebau Kern GmbH",
        net: "1500.00",
        tax: "285.00",
        gross: "1785.00",
        iban: "DE02120300000000202051",
      }),
      documentType: 386,
      inputTax: {
        allowed: true,
        tax: "285.00",
        onPayment: true,
        reason:
          "Laut Datei ist das eine Anzahlungs-, Abschlags- oder Teilrechnung. Wird vor der Leistung gezahlt, zählt die Steuer in der Regel erst mit der Zahlung und nur für den bezahlten Anteil (Abschnitt 15.3 UStAE). Ob die Leistung schon ausgeführt ist, zeigt die Datei nicht. Die App rechnet deshalb nur erfasste Zahlungen ein. Ist die Leistung ausgeführt, entscheide im Beleg anders.",
      },
    },
  },
  {
    id: "rcv-kreativ",
    filename: "kreativbuero-rechnung-" + year + "-117.xml",
    kind: "xml",
    importedAt: stamp(day(-6)),
    size: 13_410,
    status: "active",
    history: [],
    inspection: {
      ...inspection({
        number: "KB-" + year + "-117",
        issued: -7,
        due: 7,
        seller: "Kreativbüro Nord",
        net: "400.00",
        tax: "76.00",
        gross: "476.00",
        syntax: "UBL",
        profile: "XRechnung 3.0 · UBL",
        state: "invalid",
      }),
      // A correction of an invoice that is not in the app.
      documentType: 384,
      precedingInvoices: [{ number: "KB-" + year + "-098", date: day(-30) }],
      validation: {
        state: "invalid",
        issues: [
          {
            stage: "rules",
            ruleId: "BR-DE-15",
            severity: "error",
            class: "business",
            field: "buyerReference",
            message: "Die Rechnung enthält keine Käuferreferenz (Leitweg-ID).",
          },
        ],
      },
      inputTax: {
        allowed: true,
        reason:
          "Die technische Prüfung meldet nur Hinweise zu Angaben, die in der Regel keine Pflichtangaben nach § 14 UStG sind, etwa Käuferreferenz, Kontakt- oder Zahlungsangaben. Die App rechnet die Steuer deshalb ein. Ob die Rechnung inhaltlich stimmt und du die Vorsteuer abziehen darfst, prüft sie nicht.",
      },
    },
  },
  {
    id: "rcv-druckwerk",
    filename: "druckwerk-rechnung-" + year + "-2290.xml",
    kind: "xml",
    importedAt: stamp(day(-2)),
    size: 12_760,
    status: "active",
    history: [],
    inspection: {
      ...inspection({
        number: "DW-" + year + "-2290",
        issued: -2,
        due: 12,
        seller: "Druckwerk Süd GmbH",
        net: "250.00",
        tax: "47.50",
        gross: "297.50",
        state: "invalid",
      }),
      validation: {
        state: "invalid",
        issues: [
          {
            stage: "xsd",
            ruleId: "XSD",
            severity: "error",
            class: "format",
            message:
              "Der Aufbau oder ein Wert der XML-Datei entspricht nicht dem gewählten Rechnungsformat. Prüfe die betroffene Angabe; technische Einzelheiten findest du unter „Prüfdetails“.",
          },
        ],
      },
      inputTax: {
        allowed: false,
        blocker: "format",
        short: "Formatfehler",
        reason:
          "Die Datei hat einen Formatfehler. Steuerlich ist sie dann in der Regel keine E-Rechnung, sondern eine sonstige Rechnung. Das kann in den Übergangsfristen noch ausreichen, und auch danach ist ein Vorsteuerabzug nicht ausgeschlossen. Die App rechnet die Steuer deshalb nicht von selbst ein. Du entscheidest unten, ob und wie viel davon zählt.",
      },
    },
  },
];

export const connection: Connection = {
  id: "con-demo",
  label: "Demo-Bank Leipzig",
  endpoint: "https://fints.demo-bank.example/fints",
  bankCode: "12030000",
  userId: "lindner",
  customerId: "",
  securityFunction: "922",
  tanMedium: "Smartphone",
  rememberPin: false,
};

export const account: Account = {
  id: "acc-demo",
  connectionId: connection.id,
  data: {
    iban: company.iban,
    bic: company.bic,
    account_number: "202051",
    bank_code: "12030000",
    currency: "EUR",
    owner_name: "Mara Lindner",
    product_name: "Geschäftskonto",
  },
  lastSync: stamp(day(-1), 8),
  label: "",
  balance: {
    booked: { amount: 843_127, currency: "EUR", date: day(-1) },
    fetchedAt: stamp(day(-1), 8),
    source: "fints",
  },
};

function booking(
  id: string,
  offset: number,
  amount: number,
  name: string,
  description: string,
  iban = "",
): Booking {
  return {
    id,
    accountId: account.id,
    data: {
      booking_date: day(offset),
      value_date: day(offset),
      amount: Math.round(amount * 100), // minor units, like FinTS
      currency: "EUR",
      counterparty_name: name,
      counterparty_iban: iban,
      description,
    },
  };
}

export const bookings: Booking[] = [
  booking(
    "bk-1",
    -84,
    2403.8,
    "Bäckerei Sonnenkorn GmbH",
    `RE-${year}-0001 Logo und Etiketten`,
    "DE44500105175407324931",
  ),
  booking(
    "bk-2",
    -48,
    2174.13,
    "Weber & Partner Steuerberatung",
    `Rechnung RE-${year}-0002`,
    "DE27100777770209299700",
  ),
  booking(
    "bk-3",
    -30,
    -23.8,
    "Serverwerk Hosting GmbH",
    "SW-" + year + "-0815 Hosting",
    "DE89370400440532013000",
  ),
  booking(
    "bk-4",
    -29,
    1023.4,
    "Stadtkasse Musterstadt",
    `RE-${year}-0003 Kassenzeichen 7781`,
    "DE91100000000123456789",
  ),
  booking("bk-5", -27, -18.45, "Bürobedarf Lehmann", "Kartenzahlung Bürobedarf"),
  booking(
    "bk-6",
    -25,
    -650.0,
    "Hausverwaltung Plagwitz",
    "Miete Atelier",
    "DE02300209000106531065",
  ),
  booking(
    "bk-7",
    -12,
    200.0,
    "Bäckerei Sonnenkorn GmbH",
    `Abschlag RE-${year}-0005`,
    "DE44500105175407324931",
  ),
  booking(
    "bk-8",
    -2,
    950.81,
    "Weber & Partner Steuerberatung",
    `RE-${year}-0007`,
    "DE27100777770209299700",
  ),
  booking("bk-9", -1, -59.9, "Telekommunikation Nord", "Internet und Telefon Atelier"),
  booking(
    "bk-10",
    -20,
    -312.4,
    "Finanzamt Musterstadt",
    "USt-Vorauszahlung Vorquartal",
    "DE71300000000030001501",
  ),
];

export const payments: Payment[] = [
  payment("pay-1", "inv-1", "outgoing", "2403.80", -84, "bank", "bk-1"),
  payment("pay-2", "inv-2", "outgoing", "2174.13", -48, "bank", "bk-2"),
  payment("pay-3", "rcv-serverwerk", "incoming", "23.80", -30, "bank", "bk-3"),
  payment("pay-4", "inv-3", "outgoing", "1023.40", -29, "bank", "bk-4"),
  payment("pay-5", "rcv-lehmann", "incoming", "18.45", -27, "bank", "bk-5"),
  payment("pay-6", "inv-5", "outgoing", "200.00", -12, "bank", "bk-7"),
];

function payment(
  id: string,
  invoiceId: string,
  direction: string,
  amount: string,
  offset: number,
  source: string,
  bookingId: string,
): Payment {
  return {
    id,
    invoiceId,
    direction,
    amount,
    currency: "EUR",
    date: day(offset),
    source,
    bookingId,
    reason: source === "bank" ? "Zahlungsabgleich" : "",
    createdAt: stamp(day(offset), 12),
    revokedAt: "",
    revocationReason: "",
  };
}

// bk-6 and bk-9 are ordinary expenses without a receipt in the app; bk-10
// pays the Umsatzsteuer of the previous quarter.
export const treatments: BookingTreatment[] = [
  {
    bookingId: "bk-10",
    kind: "vat",
    note: "Umsatzsteuer-Vorauszahlung",
    at: stamp(day(-20), 12),
  },
  {
    bookingId: "bk-6",
    kind: "other",
    note: "Miete Atelier, Beleg liegt beim Steuerbüro",
    at: stamp(day(-25), 12),
  },
];

// The lehmann receipt has no amount in the PDF; its payment is recorded above.
received[3].inspection.totals = { net: "15.50", tax: "2.95", gross: "18.45", due: "18.45" };

export const reminderSettings: ReminderSettings = {
  interestRate: "",
  levels: [
    {
      title: "Zahlungserinnerung",
      waitDays: 7,
      paymentDays: 7,
      fee: "0.00",
      subject: "Zahlungserinnerung zur Rechnung {rechnungsnummer}",
      text: "Guten Tag,\n\nsicher ist es im Alltag untergegangen: Für unsere Rechnung {rechnungsnummer} vom {rechnungsdatum} haben wir bisher keinen Zahlungseingang gefunden. Offen sind noch {betrag}, fällig seit {faelligkeit}.\n\nBitte überweisen Sie den Betrag bis zum {frist}. Falls sich Ihre Zahlung mit diesem Schreiben überschnitten hat, betrachten Sie es einfach als gegenstandslos.\n\nMit freundlichen Grüßen\n{firma}",
    },
    {
      title: "1. Mahnung",
      waitDays: 3,
      paymentDays: 7,
      fee: "0.00",
      subject: "1. Mahnung zur Rechnung {rechnungsnummer}",
      text: "Guten Tag,\n\nzu unserer Zahlungserinnerung für die Rechnung {rechnungsnummer} vom {rechnungsdatum} haben wir bisher keinen vollständigen Zahlungseingang erhalten. Offen sind weiterhin {betrag}, fällig seit {faelligkeit}.\n\nBitte überweisen Sie den Betrag bis spätestens {frist}. Bei Fragen zur Rechnung melden Sie sich gern bei uns.\n\nMit freundlichen Grüßen\n{firma}",
    },
    {
      title: "2. Mahnung",
      waitDays: 3,
      paymentDays: 7,
      fee: "0.00",
      subject: "2. Mahnung zur Rechnung {rechnungsnummer}",
      text: "Guten Tag,\n\ntrotz unserer bisherigen Schreiben ist die Rechnung {rechnungsnummer} vom {rechnungsdatum} noch nicht vollständig bezahlt. Offen sind {betrag}, fällig seit {faelligkeit}.\n\nWir bitten Sie dringend, den Betrag bis spätestens {frist} zu überweisen. Wenn Sie Fragen haben oder eine Ratenzahlung vereinbaren möchten, sprechen Sie uns bitte an.\n\nMit freundlichen Grüßen\n{firma}",
    },
  ],
};

export const reminders: Reminder[] = [];

export const financeSettings: FinanceSettings = {
  taxBasis: "soll",
  reservePercent: "25",
  activity: "trade",
  balances: [],
  updatedAt: "",
};

export const datevSettings: DatevSettings = {
  consultantNumber: "",
  clientNumber: "",
  fiscalYearStartMonth: 1,
  accountLength: 4,
  chart: "SKR03",
  personAccounts: "collective",
  accounts: {
    revenue: {
      standard19: "8400",
      reduced7: "8300",
      zeroRated: "8290",
      exempt: "8100",
      export: "8120",
      intraCommunity: "8125",
      reverseCharge: "8337",
      reverseChargeEU: "8336",
      outOfScope: "8338",
      smallBusiness: "8195",
      margin: "8193",
    },
    expense: "4980",
    bank: "1200",
    debtor: "10000",
    creditor: "70000",
  },
  includePayments: true,
  includeDocuments: true,
  updatedAt: "",
};

export const mailSettings: MailSettings = {
  account: {
    preset: "",
    host: "",
    port: 587,
    security: "starttls",
    username: "",
    fromName: "",
    fromAddress: "",
    replyTo: "",
    bccSelf: false,
  },
  templates: {},
  hasPassword: false,
};

export const mailLog: LogEntry[] = [];

// One connected online shop. The demo only shows its orders; connecting,
// fetching and creating drafts from them need the desktop app.
export const shopConnection: ShopConnection = {
  id: "shop-demo",
  provider: "woocommerce",
  label: "Atelier Lindner Shop",
  url: "https://shop.atelier-lindner.example",
  enabled: true,
  autoCreateDrafts: false,
  importFrom: day(-30),
  lastSync: stamp(day(0), 8),
  lastError: "",
  hasCredentials: true,
};

function shopOrder(
  externalId: string,
  daysAgo: number,
  buyer: Customer,
  value: Partial<ShopOrder> & Pick<ShopOrder, "gross" | "lines">,
): ShopOrder {
  return {
    id: `shop-order-${externalId}`,
    connectionId: shopConnection.id,
    externalId,
    number: `#${externalId}`,
    customerExternalId: `shop-customer-${externalId}`,
    customer: buyer,
    currency: "EUR",
    createdAt: stamp(day(-daysAgo), 9),
    updatedAt: stamp(day(-daysAgo), 11),
    status: "processing",
    paid: true,
    adjustments: [],
    reviewReason: "",
    invoiceId: "",
    importedAt: stamp(day(-daysAgo), 11),
    ...value,
  };
}

const shopLine = (name: string, quantity: string, unitPrice: string): InvoiceLine => ({
  id: `line-${++lineCounter}`,
  productId: "",
  name,
  description: "",
  quantity,
  unit: "H87",
  unitPrice,
  taxRate: "19",
  taxCategory: "S",
  taxTreatment: "standard",
});

export const shopOrders: ShopOrder[] = [
  shopOrder(
    "1043",
    1,
    customer("shop-customer-1043", {
      name: "Hannah Vogt",
      email: "hannah.vogt@example.com",
      address: {
        street: "Kantstraße 12",
        additional: "",
        postcode: "04275",
        city: "Leipzig",
        country: "DE",
      },
    }),
    {
      gross: "71.40",
      paidAt: stamp(day(-1), 9),
      lines: [shopLine("Kunstdruck „Auwald“ A3", "2", "30.00")],
    },
  ),
  shopOrder(
    "1042",
    3,
    customer("shop-customer-1042", {
      name: "Tom Albrecht",
      email: "t.albrecht@example.com",
      address: { street: "", additional: "", postcode: "01067", city: "Dresden", country: "DE" },
    }),
    {
      gross: "23.80",
      paid: false,
      status: "on-hold",
      lines: [shopLine("Postkarten-Set Leipzig", "1", "20.00")],
      conversionError:
        "Die Rechnungsadresse ist unvollständig. Ergänze sie im Shop und ruf erneut ab.",
    },
  ),
  shopOrder(
    "1041",
    6,
    customer("shop-customer-1041", {
      name: "Studio Fernweh UG",
      contactName: "Mira Engel",
      email: "hallo@studio-fernweh.example",
      address: {
        street: "Riesaer Straße 5",
        additional: "",
        postcode: "04328",
        city: "Leipzig",
        country: "DE",
      },
    }),
    {
      gross: "142.80",
      paidAt: stamp(day(-6), 9),
      lines: [shopLine("Kunstdruck „Auwald“ A2", "2", "60.00")],
    },
  ),
];
