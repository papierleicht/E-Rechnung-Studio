/**
 * Phone-width layout for the embedded demo. The desktop window is at least
 * 900 px wide, the demo iframe on papierleicht.de can be much narrower: there
 * the sidebar starts collapsed, overlays the page and closes after choosing a
 * section. The user's own choice (stored by the app) still wins.
 */
import "./narrow.css";

const narrow = window.matchMedia("(max-width: 760px)");

const getItem = Storage.prototype.getItem;
Storage.prototype.getItem = function (key: string) {
  const value = getItem.call(this, key);
  if (value === null && key === "e-rechnung-studio-sidebar-open" && narrow.matches) return "false";
  return value;
};

document.addEventListener("click", (event) => {
  if (!narrow.matches || !(event.target instanceof Element)) return;
  if (!event.target.closest("#app-sidebar nav button")) return;
  const toggle = document.querySelector<HTMLButtonElement>(".sidebar-toggle[aria-expanded='true']");
  if (toggle) setTimeout(() => toggle.click());
});
