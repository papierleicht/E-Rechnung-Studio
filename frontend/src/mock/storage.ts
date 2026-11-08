/**
 * The demo may keep only display preferences in the browser (see the privacy
 * policy on papierleicht.de). Writes to any other key are dropped, so app code
 * that caches data in the desktop WebView stores nothing here.
 */
const allowed = new Set(["e-rechnung-studio-sidebar-open", "papierleicht-plus-hints-dismissed"]);

const setItem = Storage.prototype.setItem;
Storage.prototype.setItem = function (key: string, value: string) {
  if (this === globalThis.sessionStorage || allowed.has(key)) setItem.call(this, key, value);
};
