/** Limit only file selection, never a save that may already have written data.
 * Late native responses are ignored, so cancelling cannot start a delayed export.
 */
/** Native Wails dialogs reject on an ordinary user cancellation on some platforms. */
export function isFileDialogCancellation(error: unknown): boolean {
  const detail = error instanceof Error ? error.message : String(error);
  return (
    /\bcancel(?:l)?ed(?: by (?:the )?user)?\b/i.test(detail) ||
    /(?:vom|durch den) Benutzer abgebrochen/i.test(detail)
  );
}

export function waitForFileDialog<T>(
  open: () => Promise<T>,
  signal: AbortSignal,
  timeoutMs = 120_000,
): Promise<T | undefined> {
  return new Promise((resolve, reject) => {
    let done = false;
    let timer: ReturnType<typeof setTimeout>;
    function finish(error: Error | null, value?: T) {
      if (done) return;
      done = true;
      clearTimeout(timer);
      signal.removeEventListener("abort", cancel);
      if (error) reject(error);
      else resolve(value);
    }
    function cancel() {
      finish(null);
    }
    if (signal.aborted) {
      cancel();
      return;
    }
    signal.addEventListener("abort", cancel, { once: true });
    timer = setTimeout(
      () =>
        finish(
          new Error(
            "Die Dateiauswahl hat nicht geantwortet. Schließe einen noch offenen Dateidialog und versuche es noch einmal.",
          ),
        ),
      timeoutMs,
    );
    try {
      Promise.resolve(open()).then(
        (value) => finish(null, value),
        (error) =>
          isFileDialogCancellation(error)
            ? finish(null)
            : finish(error instanceof Error ? error : new Error(String(error))),
      );
    } catch (error) {
      if (isFileDialogCancellation(error)) finish(null);
      else finish(error instanceof Error ? error : new Error(String(error)));
    }
  });
}
