import type { HintPlan } from "./hintPlan";

/** Single place for the public Plus information page. */
export const PLUS_INFO_URL = "https://papierleicht.de/#preise";

/**
 * What a hint promotes:
 * - "fints": FinTS retrieval (hidden with Plus or an own/build product ID)
 * - "plus": the outside confirmation and support included in Plus
 * - "backup": the encrypted automatic backup (add-on to every paid tier)
 */
export type PlusHintTopic = "fints" | "plus" | "backup";

export const DISMISSED_STORAGE_KEY = "papierleicht-plus-hints-dismissed";

type Storage = Pick<globalThis.Storage, "getItem" | "setItem">;

// Keeps a dismissal for this session even when storage is blocked.
const sessionDismissed = new Set<string>();

export function plusHintRelevant(topic: PlusHintTopic, plan: HintPlan): boolean {
  if (!plan.known) return false;
  if (topic === "backup") return !plan.backupActive;
  if (plan.plusActive) return false;
  // An own registration or a build-time ID already enables the retrieval.
  return topic !== "fints" || plan.productSource === "";
}

export function plusHintVisible(
  id: string,
  topic: PlusHintTopic,
  plan: HintPlan,
  dismissed: ReadonlySet<string>,
): boolean {
  return plusHintRelevant(topic, plan) && !dismissed.has(id);
}

function defaultStorage(): Storage | null {
  try {
    return globalThis.localStorage ?? null;
  } catch {
    return null;
  }
}

export function dismissedPlusHints(storage: Storage | null = defaultStorage()): Set<string> {
  try {
    const parsed: unknown = JSON.parse(storage?.getItem(DISMISSED_STORAGE_KEY) ?? "[]");
    const stored = Array.isArray(parsed) ? parsed.filter((id) => typeof id === "string") : [];
    return new Set([...sessionDismissed, ...stored]);
  } catch {
    return new Set(sessionDismissed);
  }
}

/** Remembers the choice; returns the new set even if storage is unavailable. */
export function dismissPlusHint(
  id: string,
  storage: Storage | null = defaultStorage(),
): Set<string> {
  sessionDismissed.add(id);
  const next = dismissedPlusHints(storage);
  try {
    storage?.setItem(DISMISSED_STORAGE_KEY, JSON.stringify([...next].sort()));
  } catch {}
  return next;
}
