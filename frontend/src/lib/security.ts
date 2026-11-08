/**
 * Security level for the whole UI: the plan, the state of the backup and the
 * last automatic check of the change log, kept current by their Wails events.
 */
import { Events } from "@wailsio/runtime";
import { derived, writable, type Readable } from "svelte/store";
import * as AuditService from "../../bindings/papierleicht.de/e-rechnung-studio/internal/app/auditservice";
import * as BackupService from "../../bindings/papierleicht.de/e-rechnung-studio/internal/app/backupservice";
import type {
  AuditHealth,
  CloudBackupStatus,
} from "../../bindings/papierleicht.de/e-rechnung-studio/internal/app/models";
import { backupOverdueDays } from "./backup";
import { plan } from "./plan";
import { banking } from "./banking-api";
import {
  journey,
  nextAction,
  securityState,
  type SecurityInput,
  type Stage,
  type SecurityState,
} from "./securityLevel";

function eventData<T>(event: { data: unknown }): T | null {
  const data = (Array.isArray(event.data) ? event.data[0] : event.data) as T;
  return data && typeof data === "object" ? data : null;
}

const backup = writable<CloudBackupStatus | null>(null, (set) => {
  void BackupService.BackupStatus().then(set, () => undefined);
  return Events.On("backup:status", (event) => {
    const data = eventData<CloudBackupStatus>(event);
    if (data) set(data);
  });
});

/** The last automatic check of the change log; null until it ran. */
export const auditHealth: Readable<AuditHealth | null> = writable<AuditHealth | null>(
  null,
  (set) => {
    void AuditService.Status().then(
      (status) => set(status.health),
      () => undefined,
    );
    return Events.On("audit:health", (event) => {
      const data = eventData<AuditHealth>(event);
      if (data) set(data);
    });
  },
);

const bankConnected = writable(false, (set) => {
  void banking.overview().then(
    (overview) => set((overview.accounts?.length ?? 0) > 0 && overview.productSource !== ""),
    () => undefined,
  );
});

const input: Readable<SecurityInput & { bankConnected: boolean }> = derived(
  [plan, backup, auditHealth, bankConnected],
  ([$plan, $backup, $health, $bank]) => ({
    plusActive: $plan.plusActive,
    backupActive: $plan.backupActive,
    supportActive: $plan.supportActive,
    keyConfigured: $backup?.keyConfigured ?? false,
    auto: $backup?.auto ?? false,
    lastBackupAt: $backup?.lastBackupAt ?? "",
    lastError: $backup?.lastError ?? "",
    overdueDays: $backup ? backupOverdueDays($backup, new Date()) : 0,
    // Before the first check there is nothing to report.
    logProblem: Boolean(
      $health?.checkedAt && (!$health.ok || $health.conflicts > 0 || $health.stale),
    ),
    bankConnected: $bank,
  }),
);

export const security: Readable<SecurityState> = derived(input, securityState);
export const stages: Readable<Stage[]> = derived(input, journey);
export const next: Readable<Stage["action"]> = derived(stages, nextAction);
export const lastBackupAt: Readable<string> = derived(input, (i) => i.lastBackupAt);
