/** Supersample fine PDF text before the browser fits it to the physical pixel grid.
 * Keep the extra resolution bounded for large pages and high-DPI displays. */
export function pdfRasterSize(
  width: number,
  height: number,
  displayWidth: number,
  density: number,
) {
  const requested = (displayWidth / width) * Math.max(1, density) * 2;
  const scale = Math.min(
    requested,
    Math.sqrt(16_000_000 / (width * height)),
    8192 / Math.max(width, height),
  );
  return {
    width: Math.max(1, Math.floor(width * scale)),
    height: Math.max(1, Math.floor(height * scale)),
  };
}

/** Watch both container resizing and moving the window between displays. */
export function observePDFSize(
  host: HTMLElement,
  update: (width: number, density: number) => void,
) {
  let timer: ReturnType<typeof setTimeout>;
  let previous = "";
  let media: MediaQueryList;
  const measure = () => {
    const width = host.getBoundingClientRect().width;
    const density = window.devicePixelRatio || 1;
    const key = `${width}:${density}`;
    if (width > 0 && key !== previous) {
      previous = key;
      update(width, density);
    }
  };
  const schedule = () => {
    clearTimeout(timer);
    timer = setTimeout(measure, 150);
  };
  const watchDensity = () => {
    media?.removeEventListener("change", watchDensity);
    media = window.matchMedia(`(resolution: ${window.devicePixelRatio || 1}dppx)`);
    media.addEventListener("change", watchDensity);
    schedule();
  };
  const observer = new ResizeObserver(schedule);
  observer.observe(host);
  window.addEventListener("resize", schedule);
  watchDensity();
  measure();
  return () => {
    clearTimeout(timer);
    observer.disconnect();
    media.removeEventListener("change", watchDensity);
    window.removeEventListener("resize", schedule);
  };
}
