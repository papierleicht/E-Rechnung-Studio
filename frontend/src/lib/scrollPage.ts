/**
 * Brings the page content back to its top. In the app the workspace scrolls on
 * its own, beside the sidebar and below the top row; elsewhere (the setup) the
 * window scrolls.
 */
export function scrollPageToTop() {
  const workspace = document.querySelector<HTMLElement>(".app-shell .workspace");
  (workspace ?? window).scrollTo({ top: 0, behavior: "instant" });
}
