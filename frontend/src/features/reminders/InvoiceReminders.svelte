<script lang="ts">
  import { onDestroy } from "svelte";
  import { Dialogs } from "@wailsio/runtime";
  import * as API from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/app/invoiceservice";
  import type { InvoiceReminderState } from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/app/models";
  import { paymentMoney as money } from "../../lib/paymentWorkflow";
  import { serviceErrorMessage } from "../../lib/errors";
  import { dateLabel } from "../../lib/format";
  import { waitForFileDialog } from "../../lib/fileDialog";
  import DocumentMailButton from "../invoices/DocumentMailButton.svelte";
  import PlusHint from "../../components/PlusHint.svelte";
  import {
    hasHintAmounts,
    reminderBasisLine,
    reminderHistoryLine,
    reminderTiming,
  } from "../../lib/reminders";

  let {
    invoiceID,
    unsent = false,
    revision = 0,
    onbusychange = (_busy: boolean) => {},
    onmailsettings = () => {},
  }: {
    invoiceID: string;
    /** No delivery or sent note exists for this invoice yet. */
    unsent?: boolean;
    /** Changes whenever payments of this invoice were edited elsewhere. */
    revision?: number;
    onbusychange?: (busy: boolean) => void;
    onmailsettings?: () => void;
  } = $props();

  let info = $state<InvoiceReminderState | null>(null),
    level = $state(1),
    busy = $state(false),
    error = $state(""),
    notice = $state("");
  let version = 0,
    disposed = false;
  const dialogs = new AbortController();
  const proposal = $derived(info?.proposal ?? null);
  const history = $derived(info?.history ?? []);
  const payLine = $derived.by(() => {
    if (!proposal) return "";
    const p = proposal;
    const extra = hasHintAmounts(p)
      ? ` (inkl. ${money(p.fee, p.currency)} Mahngebühr und ${money(p.interest, p.currency)} Verzugszinsen als Hinweisbetrag)`
      : "";
    return `Zu zahlen: ${money(p.total, p.currency)} bis ${dateLabel(p.paymentDue)}${extra}.`;
  });

  async function load(id: string) {
    const v = ++version;
    error = "";
    try {
      const result = await API.InvoiceReminders(id);
      if (disposed || v !== version) return;
      info = result;
      level = result.proposal.level || 1;
    } catch (e) {
      if (!disposed && v === version)
        error = serviceErrorMessage(
          e,
          "Die Zahlungserinnerungen konnten nicht geladen werden. Versuche es gleich noch einmal.",
        );
    }
  }
  $effect(() => {
    revision;
    notice = "";
    void load(invoiceID);
  });
  onDestroy(() => {
    disposed = true;
    version++;
    dialogs.abort();
    onbusychange(false);
  });

  async function perform(action: () => Promise<void>) {
    if (busy) return;
    busy = true;
    onbusychange(true);
    error = "";
    notice = "";
    try {
      await action();
    } catch (e) {
      error = serviceErrorMessage(
        e,
        "Die Erinnerung konnte nicht erstellt werden. Versuche es noch einmal.",
      );
    } finally {
      busy = false;
      if (!disposed) onbusychange(false);
    }
  }
  function choosePath(title: string, filename: string) {
    return waitForFileDialog(
      () =>
        Dialogs.SaveFile({
          Title: title,
          Filename: filename,
          CanCreateDirectories: true,
          Filters: [{ DisplayName: "PDF", Pattern: "*.pdf" }],
        }),
      dialogs.signal,
    );
  }
  function suggestedName() {
    const title = proposal?.title && level === proposal.level ? proposal.title : `Stufe ${level}`;
    return `${title}-${proposal?.number || "Rechnung"}.pdf`.replace(/[^\p{L}\p{N}.-]+/gu, "-");
  }
  async function create() {
    await perform(async () => {
      const path = await choosePath("Erinnerung als PDF speichern", suggestedName());
      if (!path) return;
      const recorded = await API.ExportReminderPDF(invoiceID, level, path);
      notice = `${recorded.title} ist gespeichert und in der Mahnhistorie eingetragen: ${path}`;
      await load(invoiceID);
    });
  }
  async function exportAgain(id: string, name: string) {
    await perform(async () => {
      const path = await choosePath("Erinnerung erneut speichern", name || "Erinnerung.pdf");
      if (!path) return;
      await API.ExportStoredReminderPDF(id, path);
      notice = `Die unveränderte Kopie ist gespeichert: ${path}`;
    });
  }
</script>

{#if proposal && (proposal.eligible || history.length)}
  <section class="invoice-reminders" aria-labelledby="reminders-title">
    <div class="heading">
      <h3 id="reminders-title">Zahlungserinnerung</h3>
      {#if proposal.eligible}<span class="state" class:due={proposal.due}
          >{reminderTiming(proposal)}</span
        >{/if}
    </div>
    {#if error}<p role="alert" class="error">{error}</p>{/if}
    {#if notice}<p role="status">{notice}</p>{/if}
    {#if proposal.eligible}
      <p>{reminderBasisLine(proposal)}</p>
      {#if unsent}<p class="unsent-hint" role="note">
          Diese Rechnung gilt laut App noch nicht als versendet. Dein Kunde kennt sie womöglich
          nicht. Per E-Mail hängt die App sie der Erinnerung an.
        </p>{/if}
      <div class="create">
        <label
          >Stufe<select bind:value={level} disabled={busy}>
            <option value={1}>Zahlungserinnerung</option>
            <option value={2}>1. Mahnung</option>
            <option value={3}>2. Mahnung</option>
          </select></label
        >
        <button class="primary" disabled={busy} onclick={create}>Erinnerung erstellen</button>
        <DocumentMailButton
          documentType="reminder"
          documentId={invoiceID}
          {level}
          {busy}
          onsent={(message) => {
            notice = `${message}. Die Mahnhistorie ist aktualisiert.`;
            void load(invoiceID);
          }}
          onerror={(message) => (error = message)}
          onsettings={onmailsettings}
        />
      </div>
      <PlusHint
        id="reminder-bank-sync"
        topic="fints"
        text="Mit Plus siehst du nach einem Bankabruf neue Zahlungseingänge, bevor du eine Erinnerung verschickst."
      />
      {#if level === proposal.level}
        <p class="hint">
          {payLine}
        </p>
        <details class="link-toggle">
          <summary>Text ansehen</summary>
          <p class="subject">{info?.subject}</p>
          <p class="letter">{info?.text}</p>
        </details>
      {:else}
        <p class="hint">Die Erinnerung nutzt Text, Frist und Gebühr der gewählten Stufe.</p>
      {/if}
      <p class="hint">
        „Erinnerung erstellen“ speichert die PDF und trägt sie als „manuell exportiert“ in die
        Mahnhistorie ein. „Per E-Mail senden“ verschickt sie über dein E-Mail-Konto und trägt den
        Versand ein. Gebühren und Zinsen sind Hinweisbeträge aus deinen Einstellungen, keine
        Rechtsberatung.
      </p>
    {:else}
      <p class="hint">{proposal.reason}</p>
    {/if}
    {#if history.length}
      <h4>Mahnhistorie</h4>
      <ul>
        {#each history as reminder (reminder.id)}
          <li>
            <span>{reminderHistoryLine(reminder)}</span><button
              class="text"
              disabled={busy}
              onclick={() => exportAgain(reminder.id, reminder.fileName)}>Erneut speichern</button
            >
          </li>
        {/each}
      </ul>
    {/if}
  </section>
{/if}

<style>
  .unsent-hint {
    color: var(--theme-warning-text);
    font-size: 13px;
  }
  .invoice-reminders {
    padding: 20px;
    margin: 20px 0;
    border: 1px solid var(--theme-border);
    border-radius: 10px;
    background: var(--theme-surface);
  }
  /* The timing note can carry a long reminder title: it wraps below the
     heading instead of squeezing it. */
  .heading {
    display: flex;
    flex-wrap: wrap;
    align-items: baseline;
    gap: 0 16px;
  }
  .heading h3 {
    flex: none;
  }
  h3,
  h4 {
    margin: 0 0 10px;
  }
  h4 {
    margin-top: 18px;
  }
  .state {
    min-width: 0;
    margin-left: auto;
    overflow-wrap: anywhere;
    font-size: 12px;
    color: var(--theme-muted);
  }
  .state.due {
    color: var(--theme-error-text);
    font-weight: 600;
  }
  .create {
    display: flex;
    align-items: end;
    gap: 12px;
    flex-wrap: wrap;
  }
  label {
    display: grid;
    gap: 6px;
  }
  .hint {
    color: var(--theme-muted);
    font-size: 12px;
    line-height: 1.5;
  }
  details {
    margin: 10px 0;
  }
  summary {
    cursor: pointer;
    font-size: 13px;
  }
  .subject {
    font-weight: 600;
  }
  .letter {
    white-space: pre-wrap;
    font-size: 12px;
    line-height: 1.5;
  }
  ul {
    margin: 0;
    padding-left: 20px;
  }
  li {
    padding: 6px 0;
    overflow-wrap: anywhere;
  }
  [role="status"],
  .error,
  .letter {
    overflow-wrap: anywhere;
  }
  li button {
    margin-left: 8px;
  }
  .error {
    color: var(--theme-error-text);
  }
</style>
