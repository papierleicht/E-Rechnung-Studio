/**
 * After the payment page opened in the browser, the app looks for the new
 * subscription by itself: every few seconds for a while and whenever the
 * window gets focus again. Card payments arrive within seconds; a direct debit
 * can take days, then the confirmation mail and the next start pick it up.
 */
export interface ActivationWatch {
  stop(): void;
}

export interface WatchOptions {
  /** Pause between checks in milliseconds. */
  every?: number;
  /** How long to keep checking in milliseconds. */
  duration?: number;
  now?: () => number;
  target?: Pick<Window, "addEventListener" | "removeEventListener">;
}

/** Calls `check` until it reports the subscription as active (true). */
export function watchActivation(
  check: () => Promise<boolean>,
  { every = 5000, duration = 15 * 60 * 1000, now = Date.now, target = window }: WatchOptions = {},
): ActivationWatch {
  const started = now();
  let busy = false;
  let stopped = false;

  async function tick() {
    if (stopped || busy) return;
    if (now() - started > duration) return stop();
    busy = true;
    try {
      if (await check()) stop();
    } catch {
      // A failed check is retried with the next tick.
    } finally {
      busy = false;
    }
  }

  const timer = setInterval(() => void tick(), every);
  const onFocus = () => void tick();
  target.addEventListener("focus", onFocus);

  function stop() {
    if (stopped) return;
    stopped = true;
    clearInterval(timer);
    target.removeEventListener("focus", onFocus);
  }
  return { stop };
}
