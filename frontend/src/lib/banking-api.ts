import * as Native from "../../bindings/papierleicht.de/e-rechnung-studio/internal/app/bankingservice";

export interface BankConnection {
  id: string;
  label: string;
  endpoint: string;
  bankCode: string;
  userId: string;
  customerId: string;
  securityFunction: string;
  tanMedium: string;
  rememberPin: boolean;
}
export interface BankAccount {
  id: string;
  connectionId: string;
  lastSync: string;
  /** The name the user gave the account; empty until set. */
  label?: string;
  data: { iban?: string; account_number?: string; product_name?: string; currency?: string };
  /** Last balance reported by the bank (FinTS HKSAL); absent if never fetched. */
  balance?: {
    booked: { amount: number; currency: string; date: string; time?: string };
    available?: { amount: number; currency: string } | null;
    fetchedAt: string;
    /** "statement": closing balance sent with the transactions; "query": HKSAL. */
    source: string;
  } | null;
}
export interface BankBooking {
  id: string;
  accountId: string;
  data: {
    booking_date: string;
    value_date?: string;
    amount: number;
    currency: string;
    counterparty_name?: string;
    description?: string;
    bank_reference?: string;
  };
}
export interface BankOverview {
  connections: BankConnection[];
  accounts: BankAccount[];
  /** Origin of the FinTS product ID; the ID itself stays in the backend. */
  productSource: "" | "build" | "own" | "subscription";
  /** Names of accounts known only from CSV imports, by their IBAN. */
  csvLabels: Record<string, string>;
}
/** What removing an account took with it. */
export interface BankAccountRemoval {
  name: string;
  iban: string;
  bookings: number;
  from: string;
  to: string;
  connectionRemoved: boolean;
}
export interface BankOperation {
  id: string;
  connectionId: string;
  status: "running" | "tan" | "approval" | "done" | "error" | "cancelled";
  message: string;
  challenge?: {
    id: string;
    text: string;
    decoupled: boolean;
    imageMimeType: string;
    image: string;
  };
  methods: { security_function: string; name: string }[];
  media: { name: string }[];
  added: number;
}

const productSources = ["build", "own", "subscription"] as const;

/** Bookings as the account view lists them: newest first. */
export const newestFirst = (rows: BankBooking[]) =>
  rows.sort((a, b) => b.data.booking_date.localeCompare(a.data.booking_date));

export const banking = {
  overview: async (): Promise<BankOverview> => {
    const result = await Native.Overview();
    return {
      ...result,
      connections: result.connections ?? [],
      accounts: result.accounts ?? [],
      productSource: productSources.find((item) => item === result.productSource) ?? "",
      csvLabels: Object.fromEntries(
        Object.entries(result.csvLabels ?? {}).filter((entry): entry is [string, string] =>
          Boolean(entry[1]),
        ),
      ),
    };
  },
  setProductID: async (id: string): Promise<void> => Native.SetProductID(id),
  connect: async (
    connection: BankConnection,
    pin: string,
    remember: boolean,
  ): Promise<BankOperation> => (await Native.Connect(connection, pin, remember)) as BankOperation,
  sync: async (account: string, pin: string, from: string, to: string): Promise<BankOperation> =>
    (await Native.Sync(account, pin, from, to)) as BankOperation,
  status: async (id: string): Promise<BankOperation> => (await Native.Status(id)) as BankOperation,
  submitTAN: async (id: string, challenge: string, tan: string): Promise<void> =>
    Native.SubmitTAN(id, challenge, tan),
  cancel: async (id: string): Promise<void> => Native.Cancel(id),
  bookings: async (account: string): Promise<BankBooking[]> =>
    (await Native.Bookings(account)) ?? [],
  setAccountLabel: async (id: string, label: string): Promise<void> =>
    Native.SetAccountLabel(id, label),
  removeAccount: async (id: string): Promise<BankAccountRemoval> =>
    (await Native.RemoveAccount(id)) as BankAccountRemoval,
  forgetPIN: async (id: string): Promise<void> => Native.ForgetPIN(id),
};
