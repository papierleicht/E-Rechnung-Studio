import * as API from "../../bindings/papierleicht.de/e-rechnung-studio/internal/app/invoiceservice";
import * as ChatAPI from "../../bindings/papierleicht.de/e-rechnung-studio/internal/app/chatservice";
import * as AccountAPI from "../../bindings/papierleicht.de/e-rechnung-studio/internal/app/accountservice";
import { remember } from "./cache";
import { banking, newestFirst } from "./banking-api";
import { periodRange } from "./financePeriod";
import { localToday } from "./format";

// Warms the cache for the pages whose data takes a moment to load. Failures
// are ignored: the page itself loads again and reports errors.
const loaders: Record<string, () => Promise<unknown>> = {
  received: async () => {
    // The receipts and payments behind the tiles come first, so a failure in the
    // expense list below cannot keep them from being warm.
    const [list, payments] = await Promise.all([API.ListReceivedInvoices(), API.Payments()]);
    remember("received:list", list ?? []);
    remember("payments:overview", payments);
    const [expenses, categories, setup] = await Promise.all([
      API.Expenses(),
      API.ExpenseCategories(),
      API.TaxSetup(),
    ]);
    remember("expenses:data", {
      expenses: expenses ?? [],
      categories: categories ?? [],
      smallBusiness: setup.smallBusiness,
    });
  },
  // Opening Kundenrechnungen may lead to the Angebote tab.
  invoices: async () => {
    remember("quotes:list", (await API.ListQuotes()) ?? []);
  },
  // Read-only on purpose: the reconciliation assigns payments and runs when the page opens.
  banking: async () => {
    const [overview, payments] = await Promise.all([banking.overview(), API.Payments()]);
    const bookings = await Promise.all(overview.accounts.map((item) => banking.bookings(item.id)));
    remember("banking:overview", overview);
    remember(
      "banking:bookings",
      Object.fromEntries(overview.accounts.map((item, i) => [item.id, newestFirst(bookings[i])])),
    );
    remember("payments:overview", payments);
  },
  // Konto & Finanzen opens on the overview; the bank views are behind its tabs.
  finance: async () => {
    const today = new Date();
    const date = localToday(today);
    const range = periodRange("quarter", today.getFullYear(), Math.floor(today.getMonth() / 3));
    const [report, settings, current, todo] = await Promise.all([
      API.FinanceReport(range.from, range.to),
      API.FinanceSettings(),
      API.FinanceReport(`${date.slice(0, 4)}-01-01`, date),
      // The open points cover the last years, as on the page.
      API.FinanceReport(`${Number(date.slice(0, 4)) - 5}-01-01`, date),
      loaders.banking(),
    ]);
    remember(`finance:${range.from}:${range.to}`, { report, settings });
    remember(`finance:current:${date}`, current);
    remember(`finance:todo:${date}`, todo);
  },
  taxes: async () => {
    const year = new Date().getFullYear();
    remember(`tax:overview:${year}`, await API.TaxOverview(year));
  },
  // The messages stay out: asking for them counts as a visit and shows the
  // customer as online.
  community: async () => {
    const [state, preview, prices] = await Promise.all([
      ChatAPI.State(),
      ChatAPI.Preview(),
      AccountAPI.Prices(),
    ]);
    remember("community:state", state);
    remember("community:preview", preview);
    remember("community:prices", prices ?? []);
  },
};

const pending = new Map<string, number>();
export function prefetch(section: string) {
  const load = loaders[section];
  const now = Date.now();
  // A hover over the same item within a second needs no second request.
  if (!load || now - (pending.get(section) ?? 0) < 1000) return;
  pending.set(section, now);
  void load().catch(() => {});
}
