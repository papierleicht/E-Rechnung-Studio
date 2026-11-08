/** German notices for features the browser demo only simulates. They start with
 * "Diese …" so `serviceErrorMessage` shows them instead of a generic error. */
export function demoHint(feature: string): string {
  return `Diese Funktion gibt es in der Live-Demo nicht – ${feature} geht nur in der Desktop-App.`;
}

export function desktopOnly(feature: string): Error {
  return new Error(demoHint(feature));
}
