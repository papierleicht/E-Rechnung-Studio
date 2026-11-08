import type {
  InvoiceDraft,
  InvoiceView,
} from "../../bindings/papierleicht.de/e-rechnung-studio/internal/model/models";

type Options = {
  draft: () => InvoiceDraft | null;
  clone: (draft: InvoiceDraft) => InvoiceDraft;
  request: (draft: InvoiceDraft) => Promise<InvoiceView>;
  publish: (view: InvoiceView) => void;
  pending: (value: boolean) => void;
  error: (error: unknown) => void;
};

/** Owns debouncing and invalidation for both automatic and explicit checks. */
export function createInvoicePreview(options: Options) {
  let revision = 0;
  let timer: ReturnType<typeof setTimeout> | undefined;
  function cancel() {
    clearTimeout(timer);
    revision++;
    options.pending(false);
  }
  async function check(
    request: (draft: InvoiceDraft) => Promise<InvoiceView | undefined>,
    publish: (view: InvoiceView) => void,
  ) {
    const draft = options.draft();
    if (!draft) return;
    clearTimeout(timer);
    const current = ++revision;
    const relevant = () => revision === current && options.draft() === draft;
    options.pending(true);
    try {
      const result = await request(options.clone(draft));
      if (result && relevant()) publish(result);
    } catch (error) {
      if (relevant()) options.error(error);
    } finally {
      if (relevant()) options.pending(false);
    }
  }
  async function refresh() {
    if (options.draft()?.status !== "draft") return;
    await check(options.request, options.publish);
  }
  function schedule() {
    if (options.draft()?.status !== "draft") return;
    clearTimeout(timer);
    revision++;
    options.pending(true);
    timer = setTimeout(refresh, 350);
  }
  return { cancel, schedule, refresh, check };
}
