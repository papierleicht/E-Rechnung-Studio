<script lang="ts">
  import { onMount } from "svelte";
  import { observePDFSize } from "../../lib/pdfSizing";
  import type { PDFDocumentLoadingTask, PDFDocumentProxy } from "pdfjs-dist/legacy/build/pdf.mjs";
  import Icon from "../../components/Icon.svelte";
  let { encoded }: { encoded: string } = $props();
  let host: HTMLDivElement;
  let renderSize = $state({ width: 0, density: 1 });
  onMount(() =>
    observePDFSize(host, (width, density) => {
      renderSize = { width, density };
    }),
  );
  let pageNumber = $state(1),
    pageCount = $state(0),
    loading = $state(true),
    error = $state(""),
    retry = $state(0);
  let documentSource: string | undefined;
  let loaded = $state.raw<{
    source: string;
    pdf: PDFDocumentProxy;
    render: typeof import("../../lib/pdfViewer").renderPDFPage;
  } | null>(null);
  // Own the document separately from page rendering: resizing and paging must
  // not decode the source again or recreate the PDF worker.
  $effect(() => {
    retry;
    const source = encoded;
    if (source !== documentSource) {
      documentSource = source;
      pageNumber = 1;
      pageCount = 0;
    }
    loaded = null;
    let cancelled = false;
    let task: PDFDocumentLoadingTask | undefined;
    loading = true;
    error = "";
    // A failed or pending update must never show the previous document/page.
    host?.replaceChildren();
    void (async () => {
      try {
        const viewer = await import("../../lib/pdfViewer");
        if (cancelled) return;
        task = viewer.loadPDF(source);
        const pdf = await task.promise;
        if (cancelled) return;
        pageCount = pdf.numPages;
        loaded = { source, pdf, render: viewer.renderPDFPage };
      } catch (e) {
        if (!cancelled) {
          error = String(e);
          loading = false;
        }
      }
    })();
    return () => {
      cancelled = true;
      void task?.destroy().catch(() => {});
    };
  });
  $effect(() => {
    const document = loaded;
    const source = encoded;
    const size = renderSize;
    const number = pageNumber;
    if (!document || document.source !== source || !size.width) return;
    let cancelled = false;
    const controller = new AbortController();
    loading = true;
    error = "";
    host.replaceChildren();
    void (async () => {
      try {
        const page = await document.pdf.getPage(number);
        if (cancelled) return;
        const { canvas, accessibleText } = await document.render(
          page,
          number,
          size.width,
          size.density,
          controller.signal,
        );
        if (!cancelled) host.replaceChildren(canvas, accessibleText);
      } catch (e) {
        if (!cancelled) error = String(e);
      } finally {
        if (!cancelled) loading = false;
      }
    })();
    return () => {
      cancelled = true;
      controller.abort();
    };
  });
</script>

<div class="received-pdf-viewer">
  <div class="page-controls">
    <button
      type="button"
      class="secondary"
      aria-label="Vorherige Seite"
      disabled={loading || !!error || pageNumber <= 1}
      onclick={() => pageNumber--}><Icon name="back" size={16} /></button
    >
    <span>Seite {pageNumber} von {pageCount || "…"}</span>
    <button
      type="button"
      class="secondary"
      aria-label="Nächste Seite"
      disabled={loading || !!error || pageNumber >= pageCount}
      onclick={() => pageNumber++}><Icon name="next" size={16} /></button
    >
  </div>
  {#if loading}<p role="status">Belegansicht wird geladen …</p>{/if}
  {#if error}
    <p role="alert">Die Belegansicht ließ sich nicht laden.</p>
    <details class="chevron-toggle">
      <summary>Technische Details</summary>
      <pre>{error}</pre>
    </details>
    <button type="button" class="secondary" onclick={() => retry++}>Erneut versuchen</button>
  {/if}
  <div
    bind:this={host}
    class="received-pdf-page"
    aria-label="Belegvorschau"
    aria-busy={loading}
  ></div>
</div>

<style>
  .received-pdf-viewer {
    border-radius: 8px;
    background: var(--theme-paper-mat);
    padding: 12px;
  }
  .page-controls {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    justify-content: center;
    gap: 12px;
    margin-bottom: 12px;
    font-size: 12px;
  }
  .received-pdf-page {
    position: relative;
  }
  .received-pdf-page :global(canvas) {
    width: 100%;
    height: auto;
    display: block;
    box-shadow: 0 2px 8px var(--theme-shadow);
  }
  p {
    font-size: 13px;
  }
  pre {
    max-height: 200px;
    overflow: auto;
    overscroll-behavior: contain;
    white-space: pre-wrap;
    overflow-wrap: anywhere;
  }
</style>
