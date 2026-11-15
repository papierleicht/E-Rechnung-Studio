/** Automatic checks run at every start and then at most this often. */
export const UPDATE_INTERVAL = 6 * 60 * 60 * 1000;
const RETRY_DELAY = 15 * 60 * 1000;
const CACHE_KEY = "papierleicht-update-check";
export type ReleaseStatus = {
  currentVersion: string;
  updateAvailable: boolean;
  availableVersion?: string;
};

export function createUpdateMonitor(options: {
  enabled: () => boolean;
  online: () => boolean;
  now: () => number;
  storage: Pick<Storage, "getItem" | "setItem">;
  check: () => Promise<ReleaseStatus>;
  publish: (status: ReleaseStatus) => void;
  currentVersion: string;
}) {
  // The cached result only fills the sidebar until the first check of this
  // start. It never postpones that check: a release published after the last
  // check must show up when the app opens again.
  let lastSuccess = 0;
  let lastAttempt: number | undefined;
  let inflight: Promise<ReleaseStatus> | undefined;
  let stopped = false;
  try {
    const cached = JSON.parse(options.storage.getItem(CACHE_KEY) || "null");
    if (
      cached?.status?.currentVersion === options.currentVersion &&
      typeof cached.status.updateAvailable === "boolean" &&
      (!cached.status.updateAvailable || typeof cached.status.availableVersion === "string")
    ) {
      options.publish(cached.status);
    }
  } catch {
    /* Storage may be unavailable; in-memory scheduling still works. */
  }

  function run(): Promise<ReleaseStatus> {
    if (inflight) return inflight;
    lastAttempt = options.now();
    inflight = (async () => {
      try {
        const status = await options.check();
        if (stopped) return status;
        lastSuccess = options.now();
        if (options.enabled()) options.publish(status);
        try {
          options.storage.setItem(CACHE_KEY, JSON.stringify({ checkedAt: lastSuccess, status }));
        } catch {
          /* Keep the successful check in memory. */
        }
        return status;
      } finally {
        inflight = undefined;
      }
    })();
    return inflight;
  }

  return {
    async tick() {
      const now = options.now();
      if (stopped || inflight || !options.enabled() || !options.online()) return;
      if (lastSuccess && now - lastSuccess < UPDATE_INTERVAL) return;
      if (lastAttempt !== undefined && now - lastAttempt < RETRY_DELAY) return;
      try {
        await run();
      } catch {
        /* Network failures must never interrupt invoice work. */
      }
    },
    /** A check the user asked for: runs now, also with automatic checks off, and reports errors. */
    async checkNow(): Promise<ReleaseStatus> {
      const status = await run();
      if (!stopped) options.publish(status);
      return status;
    },
    stop() {
      stopped = true;
    },
  };
}
