<script lang="ts">
  import { onDestroy, onMount } from "svelte";
  import { get } from "svelte/store";
  import { Browser } from "@wailsio/runtime";
  import * as AccountService from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/app/accountservice";
  import type {
    Device,
    Status,
  } from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/account/models";
  import { plan, refreshPlan, setPlanStatus } from "../../lib/plan";
  import {
    accountErrorMessage,
    formatPlanDate,
    planLabel,
    plusBookingUrl,
  } from "../../lib/planStatus";
  import { watchActivation, type ActivationWatch } from "../../lib/watchActivation";
  import { cached, remember } from "../../lib/cache";
  import { CONTACT_EMAIL } from "../../lib/community";
  import CloudBackupSettings from "./CloudBackupSettings.svelte";
  import SecurityCard from "../../components/SecurityCard.svelte";
  import type { StageAction } from "../../lib/securityLevel";
  import * as BackupService from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/app/backupservice";

  /** Leaves the page for a step of the way: the bank or the audit tab. */
  let { onopen = () => {} }: { onopen?: (target: "banking" | "audit") => void } = $props();

  // The state of the last visit shows right away while it reloads.
  let status = $state<Status | null>(cached<Status>("account:status") ?? null);
  let email = $state("");
  let code = $state("");
  let working = $state(false);
  let error = $state("");
  let devices = $state<Device[]>(cached<Device[]>("account:devices") ?? []);
  let devicesError = $state("");
  let activated = $state(false);
  // "Überall abmelden" asks first and reports next to its button.
  let confirmSignOutAll = $state(false);
  let signOutAllError = $state("");
  let signedOutEverywhere = $state(false);
  let activationWatch: ActivationWatch | null = null;
  // After the plan ended, stored backups stay readable for a while.
  let backupReadable = $state(cached<boolean>("account:backup-readable") ?? false);
  $effect(() => {
    if (status) remember("account:status", $state.snapshot(status));
    remember("account:devices", $state.snapshot(devices));
    remember("account:backup-readable", backupReadable);
  });

  const signedIn = $derived(status !== null && status.state !== "signed-out");

  function apply(next: Status) {
    status = next;
    setPlanStatus(next);
    if (next.state !== "signed-out") {
      signedOutEverywhere = false;
      void loadDevices();
      void loadBackupAccess();
    } else {
      devices = [];
      backupReadable = false;
    }
  }

  async function loadBackupAccess() {
    try {
      backupReadable = (await BackupService.BackupAccount()).access === "read-only";
    } catch {
      backupReadable = false;
    }
  }

  async function act(fn: () => Promise<Status>, fallback: string) {
    if (working) return;
    working = true;
    error = "";
    try {
      apply(await fn());
    } catch (e) {
      error = accountErrorMessage(e, fallback);
      // Keep the visible state in line with what Go kept.
      try {
        apply(await AccountService.Status());
      } catch {
        /* status stays as is */
      }
    } finally {
      working = false;
    }
  }

  async function loadDevices() {
    devicesError = "";
    try {
      devices = (await AccountService.Devices()) ?? [];
    } catch (e) {
      devicesError = accountErrorMessage(e, "Deine Geräte konnten nicht geladen werden.");
    }
  }

  const requestCode = () =>
    act(
      () => AccountService.RequestCode(email.trim()),
      "Den Code konnten wir nicht anfordern. Versuche es gleich noch einmal.",
    );
  const signIn = () =>
    act(async () => {
      const next = await AccountService.SignIn(code);
      code = "";
      return next;
    }, "Die Anmeldung hat nicht geklappt. Prüfe den Code und versuche es noch einmal.");
  const cancelCode = () => act(() => AccountService.CancelCode(), "");
  const refresh = () => act(() => AccountService.Refresh(), "Dein Abo konnten wir nicht prüfen.");
  const signOut = () => act(() => AccountService.SignOut(), "Das Abmelden hat nicht geklappt.");
  const removeDevice = (id: string) =>
    act(() => AccountService.RemoveDevice(id), "Das Gerät konnten wir nicht entfernen.");

  async function signOutEverywhere() {
    if (working) return;
    working = true;
    signOutAllError = "";
    const address = status?.email ?? "";
    try {
      const next = await AccountService.SignOutEverywhere();
      confirmSignOutAll = false;
      signedOutEverywhere = next.state === "signed-out";
      if (signedOutEverywhere) email = address;
      apply(next);
    } catch (e) {
      signOutAllError = accountErrorMessage(
        e,
        "Das Abmelden auf allen Geräten hat nicht geklappt.",
      );
    } finally {
      working = false;
    }
  }

  // Plans are booked on the website. The app notices a new subscription by
  // itself, see watchActivation.
  function openWebsite(url = status?.manageUrl || "https://papierleicht.de/konto") {
    void Browser.OpenURL(url);
    if (!signedIn) return;
    const before = get(plan);
    activationWatch?.stop();
    activationWatch = watchActivation(async () => {
      if (working) return false;
      // Only the status: devices are loaded once the plan changed.
      const next = await AccountService.Refresh();
      status = next;
      setPlanStatus(next);
      const now = get(plan);
      if (now.plusActive === before.plusActive && now.backupActive === before.backupActive) {
        return false;
      }
      activated = true;
      void loadDevices();
      return true;
    });
  }

  onDestroy(() => activationWatch?.stop());

  function stageAction(kind: StageAction) {
    if (kind === "website") openWebsite();
    else if (kind === "bookPlus" || kind === "bookBackup") {
      const tier = kind === "bookPlus" ? "plus" : "backup";
      openWebsite(plusBookingUrl(status?.manageUrl ?? "", tier, get(plan).supportActive));
    } else if (kind === "setup") {
      const backup = document.querySelector('[aria-labelledby="cloud-backup-title"]');
      backup?.scrollIntoView({ behavior: "smooth" });
      backup?.querySelector<HTMLElement>("button.primary")?.focus({ preventScroll: true });
    } else onopen(kind);
  }

  function lastSeen(value: string) {
    const date = formatPlanDate(value.replace(" ", "T"));
    return date ? `zuletzt aktiv ${date}` : "";
  }

  onMount(async () => {
    await refreshPlan();
    try {
      apply(await AccountService.Status());
    } catch (e) {
      error = accountErrorMessage(e, "Deinen Kontostatus konnten wir nicht laden.");
    }
  });
</script>

<section class="account-settings" aria-label="Konto und Abo" aria-busy={working}>
  {#if status?.configured}<SecurityCard onaction={stageAction} />{/if}

  {#if error}<p class="account-message error" role="alert">{error}</p>{/if}

  {#if status && !status.configured}
    <p class="account-message info">
      In dieser Version ist keine Konto-Anmeldung eingerichtet. Alle Community-Funktionen kannst du
      trotzdem nutzen.
    </p>
  {:else if status && !signedIn}
    <section class="settings-group" aria-labelledby="account-signin-title">
      <h2 id="account-signin-title">Auf diesem Gerät anmelden</h2>
      {#if signedOutEverywhere && !status.codePending}
        <p class="account-message success" role="status">
          Du bist jetzt auf allen Geräten abgemeldet. Melde dich hier mit einem neuen Code wieder
          an.
        </p>
      {/if}
      {#if !status.codePending}
        <p class="settings-help">
          Melde dich mit der E-Mail-Adresse deines Abos an, dann schaltet die App es auf diesem
          Gerät frei. Du bekommst einen Code per E-Mail, ein Passwort brauchst du nicht.
        </p>
        <form
          class="account-row"
          onsubmit={(event) => {
            event.preventDefault();
            void requestCode();
          }}
        >
          <label
            >E-Mail<input
              type="email"
              autocomplete="email"
              bind:value={email}
              required
              disabled={working}
            /></label
          >
          <button class="primary" type="submit" disabled={working || !email.trim()}
            >Code anfordern</button
          >
        </form>
      {:else}
        <p class="settings-help">
          Wir haben einen Code an <strong>{status.pendingEmail}</strong> geschickt. Er gilt einige Minuten.
        </p>
        <form
          class="account-row"
          onsubmit={(event) => {
            event.preventDefault();
            void signIn();
          }}
        >
          <label
            >Code aus der E-Mail<input
              inputmode="numeric"
              autocomplete="one-time-code"
              bind:value={code}
              required
              disabled={working}
            /></label
          >
          <button class="primary" type="submit" disabled={working || !code.trim()}>Anmelden</button>
          <button class="secondary" type="button" disabled={working} onclick={cancelCode}
            >Andere E-Mail</button
          >
        </form>
      {/if}
    </section>
  {:else if status}
    <section class="settings-group" aria-labelledby="account-status-title">
      <h2 id="account-status-title">Dein Konto</h2>
      <dl class="account-facts">
        <dt>Konto</dt>
        <dd>{status.email}</dd>
        <dt>Tarif</dt>
        <dd>{planLabel($plan)}</dd>
        <dt>Dieses Gerät</dt>
        <dd>{status.deviceName}</dd>
        {#if $plan.plusActive || $plan.supportActive}
          <!-- Paying customers find their support without looking for it. -->
          <dt>Support</dt>
          <dd>
            <button
              type="button"
              class="text account-mail"
              onclick={() => void Browser.OpenURL(`mailto:${CONTACT_EMAIL}`)}
              >{CONTACT_EMAIL}</button
            >
            <small>Software-Support per E-Mail, in deinem Abo enthalten</small>
          </dd>
        {/if}
      </dl>
      {#if activated}
        <p class="account-message success" role="status">
          Danke! Dein Abo ist jetzt auf diesem Gerät aktiv.
        </p>
      {/if}
      {#if status.notice}
        <p
          class="account-message"
          class:warning={status.noticeKind !== "offline" && status.noticeKind !== "support-plan"}
          class:info={status.noticeKind === "offline" || status.noticeKind === "support-plan"}
          role="status"
        >
          {status.notice}
        </p>
      {/if}
      <div class="actions">
        <button class="secondary" type="button" disabled={working} onclick={refresh}
          >Abo jetzt prüfen</button
        >
        <button class="secondary" type="button" disabled={working} onclick={signOut}
          >Abmelden</button
        >
      </div>
    </section>

    {#if $plan.backupActive || backupReadable}
      <CloudBackupSettings />
    {/if}

    <section class="settings-group" aria-labelledby="account-devices-title">
      <h2 id="account-devices-title">Deine Geräte</h2>
      <p class="settings-help">
        Bis zu drei Geräte. Wenn du dich abmeldest, wird dieses Gerät entfernt.
      </p>
      {#if devicesError}<p class="account-message error" role="alert">{devicesError}</p>{/if}
      {#if devices.length === 0 && !devicesError}
        <p class="settings-help">Noch kein Gerät aktiviert.</p>
      {:else}
        <ul class="account-devices">
          {#each devices as device (device.id)}
            <li>
              <span
                ><strong>{device.name || "Unbenanntes Gerät"}</strong>
                {#if device.current}<em>dieses Gerät</em>{/if}
                <small>{lastSeen(device.lastSeen)}</small></span
              >
              <button
                class="text"
                type="button"
                disabled={working}
                onclick={() => removeDevice(device.id)}>Entfernen</button
              >
            </li>
          {/each}
        </ul>
      {/if}
      <p class="settings-help account-signout-all">
        Kennst du ein Gerät nicht oder kam eine Warnmail „Neue Anmeldung“, die nicht von dir war?
        Dann melde dich überall ab. Entfernen gibt nur den Platz frei, die Anmeldung dort bleibt
        bestehen.
      </p>
      {#if signOutAllError}<p class="account-message error" role="alert">
          {signOutAllError}
        </p>{/if}
      {#if confirmSignOutAll}
        <p class="account-message warning" role="status">
          Jede Anmeldung deines Kontos endet sofort, auch auf diesem Gerät. Bis du dich mit einem
          neuen Code wieder anmeldest, läuft die App als Community-Version. Deine Daten bleiben auf
          deinen Geräten.
        </p>
        <div class="actions">
          <button class="secondary" type="button" disabled={working} onclick={signOutEverywhere}
            >Ja, überall abmelden</button
          >
          <button class="text" type="button" onclick={() => (confirmSignOutAll = false)}
            >Abbrechen</button
          >
        </div>
      {:else}
        <div class="actions">
          <button
            class="secondary"
            type="button"
            disabled={working}
            onclick={() => {
              signOutAllError = "";
              confirmSignOutAll = true;
            }}>Überall abmelden</button
          >
        </div>
      {/if}
    </section>
  {:else}
    <p class="settings-help">Wird geladen …</p>
  {/if}
</section>

<style>
  /* Sections use the 18px headings of the other settings tabs; cards use 15px
     headings, everything else is 13px. */
  .account-settings {
    font-size: 13px;
    line-height: 1.5;
  }
  .account-settings :global(.settings-group:first-of-type) {
    margin-top: 0;
    padding-top: 0;
    border-top: 0;
  }
  .account-settings :global(.settings-group > h2) {
    margin: 0 0 4px;
  }
  .account-settings :global(h3) {
    margin: 0 0 4px;
    font-size: 15px;
    font-weight: 600;
  }
  .account-settings :global(h4) {
    font-size: 13px;
    font-weight: 600;
  }
  .account-settings :global(.settings-help) {
    max-width: 640px;
    margin: 0;
    font-size: 13px;
  }
  .account-settings :global(dl),
  .account-settings :global(ul),
  .account-settings :global(label),
  .account-settings :global(summary),
  .account-settings :global(small),
  .account-settings :global(button),
  .account-settings :global(input) {
    font-size: 13px;
  }
  .account-settings :global(small),
  .account-settings :global(em) {
    color: var(--theme-muted);
    font-style: normal;
  }
  .account-row {
    display: flex;
    align-items: flex-end;
    gap: 10px;
    flex-wrap: wrap;
    margin-top: 12px;
  }
  .account-row label {
    display: grid;
    flex: 1 1 260px;
    gap: 4px;
    max-width: 380px;
  }
  .account-facts {
    display: grid;
    grid-template-columns: max-content minmax(0, 1fr);
    gap: 6px 18px;
    margin: 8px 0 0;
  }
  .account-facts dt {
    color: var(--theme-muted);
  }
  .account-facts dd {
    margin: 0;
    overflow-wrap: anywhere;
  }
  .account-settings :global(.settings-help strong) {
    overflow-wrap: anywhere;
  }
  .account-mail {
    padding: 0;
    margin-right: 8px;
  }
  .actions {
    display: flex;
    flex-wrap: wrap;
    gap: 8px;
    margin: 12px 0 0;
  }
  .account-message {
    overflow-wrap: anywhere;
    margin: 12px 0 0;
    padding: 10px 12px;
    border-radius: 8px;
    border: 1px solid var(--theme-info-border);
    background: var(--theme-info-bg);
    color: var(--theme-info-text);
  }
  .account-message.success {
    border-color: var(--theme-success-border);
    background: var(--theme-success-bg);
    color: var(--theme-success-text);
  }
  .account-message.warning {
    border-color: var(--theme-warning-border);
    background: var(--theme-warning-bg);
    color: var(--theme-warning-text);
  }
  .account-message.error {
    border-color: var(--theme-error-border);
    background: var(--theme-error-bg);
    color: var(--theme-error-text);
  }
  .account-devices {
    list-style: none;
    margin: 8px 0 0;
    padding: 0;
  }
  .account-devices li {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 12px;
    padding: 8px 0;
    border-bottom: 1px solid var(--theme-border);
  }
  .account-devices li span {
    display: flex;
    flex-wrap: wrap;
    gap: 6px;
    align-items: baseline;
    min-width: 0;
  }
  .account-devices li > button {
    margin-left: auto;
  }
  /* Apart from the device list, above the shared .settings-group rule. */
  .settings-group > .account-signout-all {
    margin-top: 16px;
  }
  .account-devices strong {
    font-weight: 500;
    overflow-wrap: anywhere;
  }
</style>
