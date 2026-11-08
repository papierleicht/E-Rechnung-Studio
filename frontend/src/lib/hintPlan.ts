// What the Plus hints need to know to stay quiet.
//
// The app is complete without Plus; hints only answer whether the automation or
// backup that Plus adds is already active. The subscription itself comes from
// the verified account status in plan.ts; this module adds the origin of the
// FinTS product ID, because an own or build-time ID already enables the
// automatic retrieval. It has no Wails imports so it stays testable in Node.

/** Origin of the FinTS product ID, as reported by `banking.overview()`. */
export type ProductSource = "" | "build" | "own" | "subscription";

export interface HintPlan {
  /** False until the status was loaded once; hints stay hidden meanwhile. */
  known: boolean;
  plusActive: boolean;
  backupActive: boolean;
  /** Kept for hints about the FinTS retrieval: "own"/"build" already have it. */
  productSource: ProductSource;
}

export const unknownHintPlan: HintPlan = Object.freeze({
  known: false,
  plusActive: false,
  backupActive: false,
  productSource: "",
});

export function hintPlanFromProductSource(productSource: ProductSource): HintPlan {
  return {
    known: true,
    plusActive: productSource === "subscription",
    backupActive: false,
    productSource,
  };
}

type Listener = (plan: HintPlan) => void;
let current: HintPlan = unknownHintPlan;
let loading: Promise<void> | null = null;
const listeners = new Set<Listener>();

/** Svelte-compatible store: `$plan` in components. */
export const hintPlan = {
  subscribe(listener: Listener): () => void {
    listeners.add(listener);
    listener(current);
    return () => listeners.delete(listener);
  },
};

export function setHintPlan(next: HintPlan): void {
  current = next;
  for (const listener of listeners) listener(current);
}

/** Loads the status once per session; failures leave hints hidden. */
export function loadHintPlan(load: () => Promise<{ productSource: ProductSource }>): Promise<void> {
  loading ??= load()
    .then((overview) => {
      if (!current.known) setHintPlan(hintPlanFromProductSource(overview.productSource));
    })
    .catch(() => {
      loading = null;
    });
  return loading;
}

/** Test helper: forget the loaded status. */
export function resetHintPlan(): void {
  loading = null;
  setHintPlan(unknownHintPlan);
}

/** The verified subscription fields of plan.ts that hints rely on. */
export interface AccountPlan {
  loaded: boolean;
  plusActive: boolean;
  backupActive: boolean;
}

/** Combines the FinTS ID origin with the verified subscription status. */
export function withAccount(hint: HintPlan, account: AccountPlan): HintPlan {
  return {
    known: hint.known && account.loaded,
    plusActive: account.plusActive || hint.productSource === "subscription",
    backupActive: account.backupActive,
    productSource: hint.productSource,
  };
}
