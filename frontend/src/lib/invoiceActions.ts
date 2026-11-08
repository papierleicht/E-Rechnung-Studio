import type {
  InvoiceDraft,
  InvoiceView,
} from "../../bindings/papierleicht.de/e-rechnung-studio/internal/model/models";
import { clone } from "./invoiceDraft.ts";
import { documentLabel } from "./tax.ts";

type Options = {
  state: () => {
    draft: InvoiceDraft | null;
    view: InvoiceView | null;
    busy: boolean;
    issuing: boolean;
    previewPending: boolean;
  };
  api: {
    SaveInvoice: (draft: InvoiceDraft) => Promise<InvoiceView>;
    IssueInvoice: (draft: InvoiceDraft) => Promise<InvoiceView>;
    SetPaid: (id: string, paid: boolean) => Promise<void>;
    OpenInvoice: (id: string) => Promise<InvoiceView>;
  };
  run: <T>(operation: () => Promise<T>) => Promise<T | undefined>;
  accept: (view: InvoiceView) => void;
  message: (value: string) => void;
  error: (value: string) => void;
  exported: (path: string) => void;
  issuing: (value: boolean) => void;
  refresh: () => Promise<void>;
  showCompletion: () => Promise<void>;
  exportXML: (id: string, number: string) => Promise<void>;
};

/** Invoice mutations stay separate from navigation and native UI concerns. */
export function createInvoiceActions(options: Options) {
  const { api, run } = options;
  async function saveDraft() {
    const { draft } = options.state();
    if (!draft) return;
    const result = await run(() => api.SaveInvoice(draft));
    if (!result) return;
    options.accept(result);
    options.message("Entwurf gespeichert");
    await run(options.refresh);
  }
  async function issue(exportAfter: "default" | "xml" = "default") {
    const { draft, view, busy, issuing, previewPending } = options.state();
    if (!draft || busy || issuing || previewPending || view?.validation.state !== "valid") return;
    const snapshot = clone(draft);
    options.issuing(true);
    try {
      const result = await run(() => api.IssueInvoice(snapshot));
      if (!result) return;
      options.accept(result);
      options.exported(result.export.pdfPath);
      options.message(
        `${documentLabel(result.draft.kind)} ${result.draft.number} ausgestellt. ${result.export.error ? "Der Export hat nicht geklappt. Wiederhole ihn bitte." : "Das PDF liegt im Exportordner."}`,
      );
      if (result.export.error) options.error(result.export.error);
      await options.showCompletion();
      await run(options.refresh);
      // A committed invoice remains issued even when its file export failed.
      if (result.export.error) options.error(result.export.error);
      if (exportAfter === "xml") await options.exportXML(result.draft.id, result.draft.number);
    } finally {
      options.issuing(false);
    }
  }
  async function changePaid(paid: boolean) {
    const id = options.state().draft?.id;
    if (!id) return;
    const ok = await run(async () => {
      await api.SetPaid(id, paid);
      return true;
    });
    if (!ok) return;
    const result = await run(() => api.OpenInvoice(id));
    if (!result) return;
    options.accept(result);
    options.message(paid ? "Rechnung als bezahlt markiert" : "Rechnung wieder als offen markiert");
    await run(options.refresh);
  }
  return { saveDraft, issue, changePaid };
}
