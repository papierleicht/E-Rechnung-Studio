import {
  getDocument,
  GlobalWorkerOptions,
  type PDFPageProxy,
} from "pdfjs-dist/legacy/build/pdf.mjs";
import workerURL from "pdfjs-dist/legacy/build/pdf.worker.min.mjs?url";
import { readPageText } from "./pdfText";
import { pdfRasterSize } from "./pdfSizing";

GlobalWorkerOptions.workerSrc = workerURL;

/** The caller owns the loading task and must destroy it when the preview is discarded. */
export function loadPDF(encoded: string) {
  return getDocument({
    data: Uint8Array.from(atob(encoded), (character) => character.charCodeAt(0)),
    useSystemFonts: false,
  });
}

/** Render offscreen so callers can discard stale results before updating their host. */
export async function renderPDFPage(
  page: PDFPageProxy,
  pageNumber: number,
  displayWidth: number,
  density: number,
  signal?: AbortSignal,
) {
  signal?.throwIfAborted();
  const viewport = page.getViewport({ scale: 1 });
  const size = pdfRasterSize(viewport.width, viewport.height, displayWidth, density);
  const canvas = document.createElement("canvas");
  canvas.width = size.width;
  canvas.height = size.height;
  canvas.setAttribute("aria-hidden", "true");
  const rendering = page.render({
    canvas,
    viewport,
    transform: [size.width / viewport.width, 0, 0, size.height / viewport.height, 0, 0],
  });
  const cancel = () => rendering.cancel();
  signal?.addEventListener("abort", cancel, { once: true });
  try {
    await rendering.promise;
    signal?.throwIfAborted();
  } finally {
    signal?.removeEventListener("abort", cancel);
  }

  const content = await readPageText(page);
  signal?.throwIfAborted();
  const accessibleText = document.createElement("p");
  accessibleText.className = "pdf-accessible-text";
  accessibleText.textContent =
    `Seite ${pageNumber}. ` +
    content.items.map((item) => ("str" in item ? item.str : "")).join(" ");
  return { canvas, accessibleText, content };
}
