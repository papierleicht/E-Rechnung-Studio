/**
 * The height that lets an element end exactly at the bottom of the window,
 * with everything below it (later sections, the page padding) still
 * visible. It never goes below min, so a small window scrolls instead of
 * squeezing the element.
 */
export function fillHeight(viewport: number, top: number, below: number, min: number): number {
  return Math.max(min, Math.floor(viewport - top - below));
}

/**
 * Svelte action: sizes the element with fillHeight and follows window
 * resizes and layout changes above or below it (wrapping tiles, banners).
 * The workspace scrolls on its own from the top of the window, so top is
 * measured against its content. Below counts the content after the element
 * up to the end of the workspace, plus its bottom padding; the page's own
 * minimum height would otherwise count as content and keep the element small.
 */
export function fillWindow(node: HTMLElement, min = 320) {
  let frame = 0;
  const scroller = node.closest<HTMLElement>(".workspace");
  const measure = () => {
    cancelAnimationFrame(frame);
    frame = requestAnimationFrame(() => {
      const box = node.getBoundingClientRect();
      const top = box.top + (scroller ? scroller.scrollTop : window.scrollY);
      const below = contentBelow(node, box.bottom);
      const height = fillHeight(window.innerHeight, top, below, min);
      if (Math.abs(height - box.height) >= 1) node.style.height = `${height}px`;
    });
  };
  // Setting the height changes the page size and calls measure once more;
  // top and below stay the same, so the second pass changes nothing.
  const observer = new ResizeObserver(measure);
  observer.observe(document.body);
  // The workspace keeps the window's height; its pages change size instead.
  for (const page of scroller?.children ?? []) observer.observe(page);
  window.addEventListener("resize", measure);
  measure();
  return {
    destroy() {
      cancelAnimationFrame(frame);
      observer.disconnect();
      window.removeEventListener("resize", measure);
    },
  };
}

function contentBelow(node: HTMLElement, bottom: number): number {
  const scope = node.closest<HTMLElement>(".workspace");
  if (!scope) return document.documentElement.scrollHeight - (bottom + window.scrollY);
  let end = bottom;
  for (const child of scope.children) end = Math.max(end, child.getBoundingClientRect().bottom);
  return end - bottom + (parseFloat(getComputedStyle(scope).paddingBottom) || 0);
}
