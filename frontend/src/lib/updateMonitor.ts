export const UPDATE_DAY = 24 * 60 * 60 * 1000;
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
  let lastSuccess = 0;
  let lastAttempt: number | undefined;
  let checking = false;
  let stopped = false;
  try {
    const cached = JSON.parse(options.storage.getItem(CACHE_KEY) || "null");
    if (
      cached?.status?.currentVersion === options.currentVersion &&
      typeof cached.status.updateAvailable === "boolean" &&
      (!cached.status.updateAvailable || typeof cached.status.availableVersion === "string") &&
      Number.isFinite(cached.checkedAt) &&
      cached.checkedAt > 0 &&
      cached.checkedAt <= options.now()
    ) {
      lastSuccess = cached.checkedAt;
      options.publish(cached.status);
    }
  } catch {
    /* Storage may be unavailable; in-memory scheduling still works. */
  }

  return {
    async tick() {
      const now = options.now();
      if (stopped || checking || !options.enabled() || !options.online()) return;
      if (lastSuccess && now - lastSuccess < UPDATE_DAY) return;
      if (lastAttempt !== undefined && now - lastAttempt < RETRY_DELAY) return;
      checking = true;
      lastAttempt = now;
      try {
        const status = await options.check();
        if (stopped) return;
        lastSuccess = options.now();
        if (options.enabled()) options.publish(status);
        try {
          options.storage.setItem(CACHE_KEY, JSON.stringify({ checkedAt: lastSuccess, status }));
        } catch {
          /* Keep the successful check in memory. */
        }
      } catch {
        /* Network failures must never interrupt invoice work. */
      } finally {
        checking = false;
      }
    },
    stop() {
      stopped = true;
    },
  };
}
