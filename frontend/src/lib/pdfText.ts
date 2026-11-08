import type { PDFPageProxy } from "pdfjs-dist/legacy/build/pdf.mjs";

/**
 * WKWebView does not support async iteration on ReadableStream on every supported
 * macOS version. PDF.js's getTextContent uses it, so consume the reader directly.
 * Keep this path shared by the accessible text and the clickable invoice regions.
 */
export async function readPageText(page: Pick<PDFPageProxy, "streamTextContent">) {
  const items: Awaited<ReturnType<PDFPageProxy["getTextContent"]>>["items"] = [];
  const reader = page.streamTextContent().getReader();
  try {
    while (true) {
      const { value, done } = await reader.read();
      if (done) break;
      items.push(...value.items);
    }
  } finally {
    reader.releaseLock();
  }
  return { items };
}
