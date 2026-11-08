import { Browser } from "@wailsio/runtime";

const ELSTER_URL = "https://www.elster.de/eportal/start";

/** Opens Mein ELSTER in the system browser; the returns are filed there. */
export async function openElster() {
  try {
    await Browser.OpenURL(ELSTER_URL);
  } catch {
    window.open(ELSTER_URL, "_blank", "noopener");
  }
}
