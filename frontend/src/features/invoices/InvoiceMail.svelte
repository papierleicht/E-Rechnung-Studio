<script lang="ts">
  import MailIcon from "@lucide/svelte/icons/mail";
  import * as MailAPI from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/app/mailservice";
  import type { InvoiceMailDraft } from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/app/models";
  import type { LogEntry } from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/mail/models";
  import InvoiceMailDialog from "./InvoiceMailDialog.svelte";
  import { serviceErrorMessage } from "../../lib/errors";
  import { sentAtLabel } from "../../lib/mail";
  let {
    invoiceId,
    busy = false,
    onsent,
    onsettings,
    onmailed,
  }: {
    invoiceId: string;
    busy?: boolean;
    onsent?: (message: string) => void;
    onsettings?: () => void;
    /** Reports whether the app has sent this invoice by mail. */
    onmailed?: (mailed: boolean) => void;
  } = $props();
  let log = $state<LogEntry[]>([]);
  let prepared = $state<InvoiceMailDraft | null>(null);
  let loading = $state(false);
  let error = $state("");
  const lastSent = $derived(log.find((entry) => entry.status === "sent"));
  $effect(() => {
    onmailed?.(lastSent !== undefined);
  });
  async function loadLog(id: string) {
    try {
      log = (await MailAPI.InvoiceMailLog(id)) ?? [];
    } catch {
      log = [];
    }
  }
  $effect(() => {
    void loadLog(invoiceId);
  });
  async function open() {
    if (loading) return;
    loading = true;
    error = "";
    try {
      prepared = await MailAPI.PrepareInvoiceMail(invoiceId);
      log = prepared.log ?? [];
    } catch (cause) {
      error = serviceErrorMessage(cause, "Die E-Mail ließ sich nicht vorbereiten.");
    } finally {
      loading = false;
    }
  }
  async function close() {
    prepared = null;
    await loadLog(invoiceId);
  }
</script>

<div class="invoice-mail">
  <div class="invoice-mail-row">
    <button class="secondary" onclick={open} disabled={busy || loading}
      ><MailIcon size={16} aria-hidden="true" /> Per E-Mail senden …</button
    >
    {#if lastSent}<span class="badge mail-sent" role="status"
        >Versendet {sentAtLabel(lastSent.sentAt)}</span
      >{/if}
  </div>
  {#if error}<p class="export-error" role="alert">{error}</p>{/if}
  {#if log.length}
    <details class="invoice-mail-log detail-row">
      <summary
        ><span class="detail-title">Versandprotokoll</span><span class="detail-state"
          >{log.length} {log.length === 1 ? "Eintrag" : "Einträge"}</span
        ></summary
      >
      <ul>
        {#each log as entry (entry.id)}
          <li class:failed={entry.status !== "sent"}>
            <strong>{sentAtLabel(entry.sentAt)}</strong> ·
            {entry.status === "sent" ? "Versendet" : "Fehlgeschlagen"} an {(entry.to ?? []).join(
              ", ",
            )}{#if entry.cc?.length}, Kopie {entry.cc.join(", ")}{/if}
            <br /><span>{entry.subject}</span>
            <br /><small
              >Anhänge: {(entry.attachments ?? []).map((file) => file.filename).join(", ") ||
                "keine"}</small
            >
            {#if entry.error}<br /><small class="invoice-mail-error">{entry.error}</small>{/if}
          </li>
        {/each}
      </ul>
    </details>
  {/if}
</div>
{#if prepared}
  <InvoiceMailDialog
    title={`Rechnung ${prepared.number} per E-Mail senden`}
    {prepared}
    send={(fields) => MailAPI.SendInvoiceMail({ invoiceId, ...fields })}
    onclose={close}
    onsent={async (message) => {
      await close();
      onsent?.(message);
    }}
    onsettings={() => {
      prepared = null;
      onsettings?.();
    }}
  />
{/if}

<style>
  .invoice-mail {
    margin-top: 12px;
  }
  .invoice-mail-row {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 10px;
  }
  .mail-sent {
    background: var(--theme-success-bg);
    color: var(--theme-success-text);
    text-transform: none;
    letter-spacing: 0;
  }
  .invoice-mail-log {
    margin-top: 10px;
  }
  .invoice-mail-log ul {
    margin: 8px 0 0;
    padding-left: 18px;
  }
  .invoice-mail-log li {
    margin-bottom: 8px;
    overflow-wrap: anywhere;
  }
  .invoice-mail-log li.failed strong,
  .invoice-mail-error {
    color: var(--theme-error-text);
  }
</style>
