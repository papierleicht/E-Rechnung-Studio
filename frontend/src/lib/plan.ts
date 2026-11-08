/**
 * Plan status store for the whole UI.
 *
 * Use `$plusActive` / `$backupActive` in components (or `get(plusActive)` in
 * plain TS). The values come from the Ed25519-verified, unexpired entitlement
 * that Go checks offline; they update on sign-in/out, renewal and expiry via
 * the Wails event "account:status". `refreshPlan()` re-reads the local status
 * without network access.
 */
import { Events } from "@wailsio/runtime";
import { derived, writable, type Readable } from "svelte/store";
import * as AccountService from "../../bindings/papierleicht.de/e-rechnung-studio/internal/app/accountservice";
import {
  initialPlanStatus,
  planFromStatus,
  type AccountStatusLike,
  type PlanStatus,
} from "./planStatus";

export type { PlanStatus } from "./planStatus";

let listening = false;
// The first subscriber loads the status, so no app-level setup is needed.
const status = writable<PlanStatus>(initialPlanStatus, () => {
  void refreshPlan();
});

/** Full status: state, e-mail, expiry, notice. */
export const plan: Readable<PlanStatus> = { subscribe: status.subscribe };
/** A paid tier (automatic bank fetch) is active: Plus or Plus Steuer. */
export const plusActive: Readable<boolean> = derived(status, (s) => s.plusActive);
/** The backup add-on (encrypted cloud backup) is active. */
export const backupActive: Readable<boolean> = derived(status, (s) => s.backupActive);
/** Plus Steuer (transmission to ELSTER) is active. */
export const taxActive: Readable<boolean> = derived(status, (s) => s.taxActive);

/** Apply a status returned by an AccountService call. */
export function setPlanStatus(value: AccountStatusLike): PlanStatus {
  const next = planFromStatus(value);
  status.set(next);
  return next;
}

/** Re-read the locally verified status; starts listening on first use. */
export async function refreshPlan(): Promise<PlanStatus> {
  if (!listening) {
    listening = true;
    Events.On("account:status", (event) => {
      const data = (Array.isArray(event.data) ? event.data[0] : event.data) as AccountStatusLike;
      if (data && typeof data === "object") setPlanStatus(data);
    });
  }
  try {
    return setPlanStatus(await AccountService.Status());
  } catch {
    // Without the backend (e.g. tests) the UI stays in Community mode.
    const fallback = { ...initialPlanStatus, loaded: true };
    status.set(fallback);
    return fallback;
  }
}
