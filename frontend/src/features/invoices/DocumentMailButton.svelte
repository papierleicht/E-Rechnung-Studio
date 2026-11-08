<script lang="ts">
  import * as MailAPI from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/app/mailservice";
  import type { DocumentMailDraft } from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/app/models";
  import InvoiceMailDialog from "./InvoiceMailDialog.svelte";
  import { serviceErrorMessage } from "../../lib/errors";

  // Sends a reminder or quote through the shared e-mail delivery.
  let {
    documentType,
    documentId,
    level = 0,
    label = "Per E-Mail senden",
    busy = false,
    onsent,
    onerror,
    onsettings,
  }: {
    documentType: "reminder" | "quote";
    documentId: string;
    /** Reminder level; 0 uses the proposed level. */
    level?: number;
    label?: string;
    busy?: boolean;
    onsent?: (message: string) => void;
    onerror?: (message: string) => void;
    onsettings?: () => void;
  } = $props();

  let prepared = $state<DocumentMailDraft | null>(null);
  let opening = $state(false);

  async function open() {
    if (opening) return;
    opening = true;
    try {
      prepared = await MailAPI.PrepareDocumentMail(documentType, documentId, level);
    } catch (cause) {
      onerror?.(serviceErrorMessage(cause, "Die E-Mail ließ sich nicht vorbereiten."));
    } finally {
      opening = false;
    }
  }
</script>

<button type="button" class="secondary" disabled={busy || opening} onclick={open}>{label}</button>
{#if prepared}
  <InvoiceMailDialog
    title={`${prepared.title} per E-Mail senden`}
    {prepared}
    send={(fields) =>
      MailAPI.SendDocumentMail({
        documentType,
        documentId,
        level: prepared?.level ?? level,
        ...fields,
      })}
    onclose={() => (prepared = null)}
    onsent={(message) => {
      prepared = null;
      onsent?.(message);
    }}
    onsettings={() => {
      prepared = null;
      onsettings?.();
    }}
  />
{/if}
