<script module lang="ts">
  let fontsPromise: Promise<string> | undefined;
</script>

<script lang="ts">
  import { onMount, onDestroy } from "svelte";
  import {
    InvoiceDocumentHTML,
    InvoicePreviewFonts,
  } from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/app/invoiceservice";
  import type {
    Company,
    InvoiceDraft,
  } from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/model/models";
  import { prepareInvoice, paginateInvoice, enableInvoiceEditing } from "../../lib/htmlInvoice";
  let {
    draft,
    company,
    disabled,
    pending = false,
    onedit,
    load,
    renderKey = "",
    numberEditable = false,
  }: {
    draft: InvoiceDraft;
    company: Company;
    disabled: boolean;
    pending?: boolean;
    onedit: (part: string) => void;
    /** Invoices let the number be clicked; quotes number themselves elsewhere. */
    numberEditable?: boolean;
    /** Quotes render the same document through their own backend call. */
    load?: (draft: InvoiceDraft) => Promise<string>;
    renderKey?: string;
  } = $props();
  let frame: HTMLIFrameElement;
  let ready = $state(false),
    loading = $state(true),
    error = $state(""),
    retry = $state(0);
  const contentKey = $derived(JSON.stringify({ draft, company, renderKey }));
  let height = $state(1123),
    rendered = $state(false);
  let revision = 0;
  const editLabels: Record<string, string> = {
    company: "Absender bearbeiten",
    customer: "Empfänger bearbeiten",
    dates: "Rechnungsdaten bearbeiten",
    lines: "Positionen bearbeiten",
    payment: "Zahlungsangaben bearbeiten",
    qr: "QR-Code bearbeiten",
  };
  const labels = $derived(
    numberEditable ? { ...editLabels, number: "Rechnungsnummer ändern" } : editLabels,
  );
  let frameReady: () => void;
  const loaded = new Promise<void>((resolve) => (frameReady = resolve));
  function fit() {
    const doc = frame?.contentDocument;
    if (!doc || !rendered) return;
    const zoom = Math.min(1, frame.clientWidth / ((210 * 96) / 25.4));
    doc.documentElement.style.zoom = String(zoom);
    height = Math.ceil(doc.body.getBoundingClientRect().height);
  }
  onMount(() => {
    let width = -1;
    let scheduled = 0;
    const observer = new ResizeObserver(([entry]) => {
      if (entry.contentRect.width === width) return;
      width = entry.contentRect.width;
      cancelAnimationFrame(scheduled);
      scheduled = requestAnimationFrame(fit);
    });
    observer.observe(frame);
    void loaded.then(() => (ready = true));
    return () => {
      observer.disconnect();
      cancelAnimationFrame(scheduled);
    };
  });
  onDestroy(() => revision++);
  $effect(() => {
    retry;
    if (!ready) return;
    const key = contentKey;
    const waiting = pending;
    const current = ++revision;
    loading = true;
    error = "";
    // Hide old data immediately while edits are being prepared.
    rendered = false;
    if (waiting) return;
    const snapshot: InvoiceDraft = JSON.parse(key).draft;
    const timer = setTimeout(() => render(snapshot, current), 80);
    return () => clearTimeout(timer);
  });
  async function render(snapshot: InvoiceDraft, current: number) {
    try {
      fontsPromise ??= Promise.resolve(InvoicePreviewFonts()).catch((e) => {
        fontsPromise = undefined;
        throw e;
      });
      const [html, fonts] = await Promise.all([
        (load ?? InvoiceDocumentHTML)(snapshot),
        fontsPromise,
      ]);
      if (current !== revision) return;
      const doc = frame.contentDocument!;
      prepareInvoice(doc, html, fonts);
      // Trigger style/layout before waiting for the newly referenced font faces.
      void doc.body.offsetHeight;
      await doc.fonts.ready;
      await Promise.all(Array.from(doc.images).map((image) => image.decode()));
      if (current !== revision) return;
      paginateInvoice(doc);
      enableInvoiceEditing(
        doc,
        snapshot.status === "draft",
        (part) => {
          if (!disabled && !loading) onedit(part);
        },
        numberEditable,
      );
      rendered = true;
      fit();
    } catch (e) {
      if (current === revision) {
        error = e instanceof Error ? e.message : String(e);
        rendered = false;
        frame.contentDocument?.body.replaceChildren();
      }
    } finally {
      if (current === revision) loading = false;
    }
  }
</script>

<div class="html-preview" aria-busy={loading}>
  {#if loading}<p role="status">Ansicht wird vorbereitet …</p>{/if}
  {#if error}<div class="preview-error" role="alert">
      <p>
        Die Rechnungsansicht ließ sich nicht laden. Prüfe die Angaben, die du zuletzt geändert hast.
      </p>
      <details class="chevron-toggle">
        <summary>Technische Details</summary>
        <pre>{error}</pre>
      </details>
      <div class="error-actions">
        <button type="button" class="secondary" onclick={() => retry++}>Erneut versuchen</button>
        {#if draft.status === "draft"}
          {#each Object.entries(labels) as [part, label]}
            <button type="button" class="secondary" {disabled} onclick={() => onedit(part)}
              >{label}</button
            >
          {/each}
        {/if}
      </div>
    </div>{/if}
  <!-- WebKit requires allow-scripts even for handlers installed by the parent.
       CSP still blocks scripts and inline handlers supplied by invoice HTML. -->
  <iframe
    bind:this={frame}
    title="Rechnung als HTML"
    sandbox="allow-same-origin allow-scripts"
    srcdoc={"<!doctype html><html lang=\"de\"><head><meta http-equiv=\"Content-Security-Policy\" content=\"default-src 'none'; script-src 'none'; style-src 'unsafe-inline'; font-src data:; img-src data:\"></head><body></body></html>"}
    onload={() => frameReady()}
    style:height={error ? "0px" : `${height}px`}
    style:visibility={rendered ? "visible" : "hidden"}
  ></iframe>
</div>

<style>
  .html-preview {
    position: relative;
    width: 100%;
    max-width: 210mm;
    margin: 0 auto;
  }
  iframe {
    display: block;
    width: 100%;
    border: 0;
  }
  p {
    font-size: 12px;
    color: var(--theme-muted);
  }
  [role="status"] {
    position: absolute;
    top: 12px;
    left: 12px;
    margin: 0;
  }
  .preview-error pre {
    white-space: pre-wrap;
    overflow-wrap: anywhere;
  }
  .error-actions {
    display: flex;
    flex-wrap: wrap;
    gap: 8px;
    margin-top: 12px;
  }
</style>
