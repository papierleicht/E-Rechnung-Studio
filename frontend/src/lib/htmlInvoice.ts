/** Browser adaptation of the shared PDF stylesheet. Paper dimensions remain
 * fixed while the browser's zoom adjusts the native text to the available space. */
export const paperCSS = `
html { background: transparent; width:210mm; }
body { margin:0; font-family:'sans'; background:transparent; --body-leading:13.05pt; }
* { box-sizing:border-box; }
h1,h2,.recipient-name,.seller-subtitle { font-family:'serif'; }
/* htmlbag ignores paragraph padding, borders and text-transform. Match those
   semantics in the browser. */
/* htmlbag sets these headings on their own 1.45 leading but places the glyphs
   differently. A single line keeps the body-leading footprint that matches the
   PDF page plan; each further line adds the real leading, so long names wrap
   without overlapping. */
h1:not(.seller-compact),h2,.recipient-name { --leading:1.45em; --gap:2mm; line-height:var(--leading); position:relative; top:calc((var(--body-leading) - var(--leading)) / 2); margin-bottom:calc(var(--gap) + var(--body-leading) - var(--leading)); }
.recipient-name { --gap:1.5mm; }
/* htmlbag hyphenates by default; long names need that here too. */
.invoice-head td,.recipient { hyphens:auto; overflow-wrap:anywhere; }
.total-entry,.grand { padding:0; border:0; }
.eyebrow,.caption,.positions th { text-transform:none; }
table { table-layout:fixed; }
td,th { overflow-wrap:anywhere; }
/* htmlbag sets the logo without a text baseline below it. */
.invoice-logo img { display:block; margin-left:auto; }
.html-page { position:relative; width:210mm; height:297mm; padding:12mm 14mm 32mm; margin-bottom:24px; background:white; }
.html-content { height:253mm; display:flow-root; }
.pagefooter { position:absolute; top:265mm; left:14mm; right:14mm; font-family:'sans'; }
.html-page-number { position:absolute; right:17mm; bottom:7mm; font:7pt 'sans'; color:var(--paper-muted); }
.html-content > :first-child { margin-top:0; }
.html-page:last-child { margin-bottom:0; }
[data-edit] { cursor:pointer; }
[data-edit]:hover { outline:1px solid var(--brand-sage); outline-offset:2px; }
/* The number inside the head and meta regions has its own target. */
[data-edit]:has([data-edit]:hover) { outline:none; }
[data-edit]:focus-visible { outline:2px solid var(--brand-sage); outline-offset:2px; }
@media print { .html-page { margin:0; break-after:page; } }
`;

/** Use the PDF typesetter's page assignment, including every split table row.
 * Browser measurements are only used to fit spacing inside those fixed pages. */
export function paginateInvoice(doc: Document) {
  const body = doc.body;
  const count = Number(body.dataset.pageCount);
  if (!Number.isInteger(count) || count < 1)
    throw new Error("Der Seitenplan fehlt. Lade die Vorschau bitte neu.");
  const footer = body.querySelector(".pagefooter");
  footer?.remove();
  const blocks = Array.from(body.children);
  body.replaceChildren();
  const contents: HTMLElement[] = [];
  for (let i = 0; i < count; i++) {
    const page = doc.createElement("section");
    page.className = "html-page";
    const content = doc.createElement("div");
    content.className = "html-content";
    page.append(content);
    contents.push(content);
    if (footer) page.append(footer.cloneNode(true));
    const number = doc.createElement("span");
    number.className = "html-page-number";
    number.textContent = "Seite " + (i + 1);
    page.append(number);
    body.append(page);
  }
  let previous = 1;
  const target = (atom: HTMLElement) => {
    const page = Number(atom.dataset.flowPage);
    if (!Number.isInteger(page) || page < previous || page > count)
      throw new Error(
        "Die Seitenzuordnung ist unvollständig. Sieh dir das PDF über „PDF öffnen“ an.",
      );
    previous = page;
    return page - 1;
  };
  for (const block of blocks) {
    const table = block as HTMLTableElement;
    if (table.tagName !== "TABLE") {
      contents[target(block as HTMLElement)].append(block);
      continue;
    }
    let current: HTMLTableElement | undefined,
      currentPage = -1;
    const rows = Array.from(table.tBodies).flatMap((b) => Array.from(b.rows));
    for (const row of rows) {
      const page = target(row);
      if (page !== currentPage) {
        current = table.cloneNode(true) as HTMLTableElement;
        Array.from(current.tBodies).forEach((b) => b.remove());
        current.createTBody();
        contents[page].append(current);
        currentPage = page;
      }
      current!.tBodies[0].append(row);
    }
  }
  for (const content of contents) {
    // Different engines have slightly different paragraph/row metrics. Tighten
    // vertical whitespace and leading, never fonts or amounts, only when needed.
    for (
      let attempt = 0;
      content.scrollHeight > content.clientHeight + 1 && attempt < 8;
      attempt++
    ) {
      const ratio = Math.min(0.96, (content.clientHeight - 2) / content.scrollHeight);
      const metrics = Array.from(content.querySelectorAll<HTMLElement>("*"))
        .map((el) => ({ el, style: doc.defaultView!.getComputedStyle(el) }))
        .map(({ el, style }) => ({
          el,
          font: parseFloat(style.fontSize),
          values: ["marginTop", "marginBottom", "paddingTop", "paddingBottom", "lineHeight"].map(
            (key) => [key, parseFloat(style[key as keyof CSSStyleDeclaration] as string)] as const,
          ),
        }));
      for (const { el, font, values } of metrics)
        for (const [key, value] of values) {
          if (!Number.isFinite(value) || value <= 0) continue;
          const next =
            key === "lineHeight"
              ? Math.min(value, Math.max(font * 1.05, value * ratio))
              : value * ratio;
          (el.style as unknown as Record<string, string>)[key] = next + "px";
        }
      content.dataset.spacingAdjusted = "true";
    }
    if (content.scrollHeight > content.clientHeight + 1)
      throw new Error(
        "Dieser Abschnitt passt nicht in den PDF-Seitenplan. Sieh dir das PDF über „PDF öffnen“ an.",
      );
  }
  return count;
}

export function prepareInvoice(doc: Document, html: string, fonts: string) {
  const source = new DOMParser().parseFromString(html, "text/html");
  doc.documentElement.style.zoom = "1";
  // The isolated invoice document uses the same fixed paper palette.
  const palette = getComputedStyle(document.documentElement);
  for (const token of ["--brand-sage", "--paper-muted"]) {
    doc.documentElement.style.setProperty(token, palette.getPropertyValue(token));
  }
  let style = doc.getElementById("invoice-style");
  if (!style) {
    style = doc.createElement("style");
    style.id = "invoice-style";
    doc.head.append(style);
  }
  style.textContent = fonts + source.querySelector("style")!.textContent + paperCSS;
  doc.body.dataset.pageCount = source.body.dataset.pageCount;
  doc.body.replaceChildren(
    ...Array.from(source.body.childNodes).map((n) => doc.importNode(n, true)),
  );
  // htmlbag uses proportional data-width attributes, browsers use CSS widths.
  doc.querySelectorAll("colgroup").forEach((group) => {
    const cols = Array.from(group.children) as HTMLElement[];
    const total = cols.reduce((sum, col) => sum + parseFloat(col.dataset.width || "0"), 0);
    cols.forEach(
      (col) => (col.style.width = `${(parseFloat(col.dataset.width || "0") / total) * 100}%`),
    );
  });
}

export function enableInvoiceEditing(
  doc: Document,
  editable: boolean,
  edit: (part: string) => void,
  numberEditable = false,
) {
  if (!editable) return;
  // Nested targets come after their region; a click only reaches the innermost.
  const regions: [string, string, string][] = [
    [".invoice-head", "company", "Absender bearbeiten"],
    [".pagefooter", "company", "Unternehmensprofil bearbeiten"],
    [".recipient", "customer", "Empfänger bearbeiten"],
    [".invoice-meta", "dates", "Rechnungsdaten bearbeiten"],
    [".positions", "lines", "Positionen bearbeiten"],
    [".document-bottom", "payment", "Zahlungsangaben bearbeiten"],
    [".payment-qr", "qr", "QR-Code bearbeiten"],
  ];
  if (numberEditable)
    regions.push(
      [".invoice-head .identity", "number", "Rechnungsnummer ändern"],
      [".invoice-meta h2", "number", "Rechnungsnummer ändern"],
    );
  for (const [selector, part, label] of regions) {
    doc.querySelectorAll<HTMLElement>(selector).forEach((el) => {
      el.dataset.edit = part;
      el.tabIndex = 0;
      el.setAttribute("role", "button");
      el.setAttribute("aria-label", label);
      el.onclick = (event) => {
        event.stopPropagation();
        if (!doc.getSelection()?.toString()) edit(part);
      };
      el.onkeydown = (event) => {
        if (event.key === "Enter" || event.key === " ") {
          event.preventDefault();
          event.stopPropagation();
          edit(part);
        }
      };
    });
  }
}
