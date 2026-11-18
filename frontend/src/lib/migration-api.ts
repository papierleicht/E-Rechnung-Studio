import * as Native from "../../bindings/papierleicht.de/e-rechnung-studio/internal/app/migrationservice";
import type {
  FileImportResult as NativeFileImportResult,
  FilePreview as NativeFilePreview,
  FilePreviewItem,
  ListImportResult as NativeListImportResult,
  ListPreview as NativeListPreview,
} from "../../bindings/papierleicht.de/e-rechnung-studio/internal/app/models";
import type {
  LegacyDocument as NativeLegacyDocument,
  LegacyFile,
  LegacyYear,
} from "../../bindings/papierleicht.de/e-rechnung-studio/internal/model/models";
import type { LegacyFilter } from "../../bindings/papierleicht.de/e-rechnung-studio/internal/store/models";
import type {
  CustomerProposal,
  Field,
  ProductProposal,
  Skipped,
  Source as NativeSource,
} from "../../bindings/papierleicht.de/e-rechnung-studio/internal/umzug/models";

// Go sends empty lists as null; the page works with these complete shapes.
export type Source = Omit<NativeSource, "ways" | "sources"> & { ways: string[]; sources: string[] };
export type FilePreview = Omit<NativeFilePreview, "items" | "skipped"> & {
  items: FilePreviewItem[];
  skipped: Skipped[];
};
export type FileImportResult = Omit<NativeFileImportResult, "failed"> & { failed: Skipped[] };
export type ListPreview = Omit<
  NativeListPreview,
  "fields" | "columns" | "mapping" | "customers" | "products"
> & {
  fields: Field[];
  columns: string[];
  mapping: Record<string, number>;
  customers: CustomerProposal[];
  products: ProductProposal[];
};
export type ListImportResult = Omit<NativeListImportResult, "failed"> & { failed: Skipped[] };
export type LegacyDocument = Omit<NativeLegacyDocument, "files"> & { files: LegacyFile[] };
export type { LegacyYear };
export type MoveList = "customers" | "products";

function mappingOf(value: { [_ in string]?: number } | null | undefined): Record<string, number> {
  const out: Record<string, number> = {};
  for (const [key, at] of Object.entries(value ?? {})) if (typeof at === "number") out[key] = at;
  return out;
}

export const migration = {
  overview: async (): Promise<{ sources: Source[]; years: LegacyYear[]; total: number }> => {
    const result = await Native.Overview();
    return {
      total: result.total,
      years: result.years ?? [],
      sources: (result.sources ?? []).map((s) => ({
        ...s,
        ways: s.ways ?? [],
        sources: s.sources ?? [],
      })),
    };
  },
  previewFiles: async (paths: string[]): Promise<FilePreview> => {
    const result = await Native.PreviewFiles(paths);
    return { ...result, items: result.items ?? [], skipped: result.skipped ?? [] };
  },
  importFiles: async (source: string, paths: string[]): Promise<FileImportResult> => {
    const result = await Native.ImportFiles(source, paths);
    return { ...result, failed: result.failed ?? [] };
  },
  archive: async (filter: LegacyFilter): Promise<LegacyDocument[]> =>
    ((await Native.Archive(filter)) ?? []).map((doc) => ({ ...doc, files: doc.files ?? [] })),
  openFile: (id: string, sum: string) => Native.OpenArchiveFile(id, sum),
  saveFile: (id: string, sum: string, path: string) => Native.SaveArchiveFile(id, sum, path),
  previewList: async (
    list: MoveList,
    path: string,
    mapping: Record<string, number> | null,
  ): Promise<ListPreview> => {
    const result = await Native.PreviewList(list, path, mapping);
    return {
      ...result,
      fields: result.fields ?? [],
      columns: result.columns ?? [],
      mapping: mappingOf(result.mapping),
      customers: result.customers ?? [],
      products: result.products ?? [],
    };
  },
  importList: async (
    list: MoveList,
    path: string,
    mapping: Record<string, number>,
  ): Promise<ListImportResult> => {
    const result = await Native.ImportList(list, path, mapping);
    return { ...result, failed: result.failed ?? [] };
  },
};

export const directionLabels: Record<string, string> = {
  outgoing: "Ausgangsrechnung",
  incoming: "Eingangsrechnung",
  "": "nicht zugeordnet",
};

export function kindLabel(kind: string): string {
  return kind === "credit-note" ? "Gutschrift" : kind === "invoice" ? "E-Rechnung" : "Datei";
}

/** Days left until a program stops, or null without a deadline. */
export function daysLeft(deadline: string | undefined, today = new Date()): number | null {
  if (!deadline) return null;
  const [y, m, d] = deadline.split("-").map(Number);
  if (!y || !m || !d) return null;
  const end = Date.UTC(y, m - 1, d);
  const now = Date.UTC(today.getFullYear(), today.getMonth(), today.getDate());
  return Math.round((end - now) / 86_400_000);
}
