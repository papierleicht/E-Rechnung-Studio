<script lang="ts">
  import { Dialogs } from "@wailsio/runtime";
  import * as API from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/app/invoiceservice";
  import type { ReceivedAttachment } from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/model/models";
  import Icon from "../../components/Icon.svelte";
  import { serviceErrorMessage } from "../../lib/errors";
  import { attachmentSummary, attachmentTitle } from "../../lib/received";

  let {
    id,
    pickFile,
  }: {
    id: string;
    pickFile: <T>(open: () => Promise<T>) => Promise<T | undefined>;
  } = $props();

  let attachments = $state<ReceivedAttachment[]>([]);
  let error = $state("");
  let message = $state("");
  let working = $state(false);

  // The list belongs to the receipt that is open; a late answer for another one is dropped.
  $effect(() => {
    const current = id;
    attachments = [];
    error = "";
    message = "";
    API.ReceivedAttachments(current).then(
      (list) => {
        if (current === id) attachments = list ?? [];
      },
      (cause) => {
        if (current === id)
          error = serviceErrorMessage(cause, "Die Anlagen lassen sich nicht lesen.");
      },
    );
  });

  async function perform(action: () => Promise<void>) {
    if (working) return;
    working = true;
    error = "";
    message = "";
    try {
      await action();
    } catch (cause) {
      error = serviceErrorMessage(cause, "Das hat leider nicht geklappt. Versuche es noch einmal.");
    } finally {
      working = false;
    }
  }
  const open = (attachment: ReceivedAttachment) =>
    perform(() => API.OpenReceivedAttachment(id, attachment.index));
  const save = (attachment: ReceivedAttachment) =>
    perform(async () => {
      const path = await pickFile(() =>
        Dialogs.SaveFile({ Title: "Anlage speichern", Filename: attachment.filename }),
      );
      if (!path) return;
      await API.SaveReceivedAttachment(id, attachment.index, path);
      message = "Anlage gespeichert.";
    });
</script>

{#if attachments.length || error}
  <section class="received-attachments" aria-label="Anlagen der Rechnung">
    <h3>Anlagen der Rechnung{attachments.length ? ` (${attachments.length})` : ""}</h3>
    {#if error}<p class="error" role="alert">{error}</p>{/if}
    {#if message}<p class="hint" role="status">{message}</p>{/if}
    {#if attachments.length}
      <ul>
        {#each attachments as attachment (attachment.index)}
          <li>
            <div class="what">
              <strong>{attachmentTitle(attachment)}</strong>
              <small>{attachmentSummary(attachment)}</small>
              {#if attachment.kind === "file" && !attachment.openable}<small
                  >Wird nicht geöffnet, weil der Typ nicht eindeutig ist. Du kannst die Datei
                  speichern und selbst prüfen.</small
                >{/if}
            </div>
            {#if attachment.kind === "file"}
              <div class="row-actions">
                {#if attachment.openable}<button
                    class="secondary"
                    disabled={working}
                    aria-label="{attachmentTitle(attachment)} öffnen"
                    onclick={() => open(attachment)}><Icon name="open" size={16} /> Öffnen</button
                  >{/if}
                <button
                  class="secondary"
                  disabled={working}
                  aria-label="{attachmentTitle(attachment)} speichern"
                  onclick={() => save(attachment)}
                  ><Icon name="download" size={16} /> Speichern</button
                >
              </div>
            {/if}
          </li>
        {/each}
      </ul>
      <p class="hint">
        Ergänzende Angaben dürfen in einer Anlage stehen. Ein Link auf ein externes Ziel genügt in
        der Regel nicht als Pflichtangabe.
      </p>
    {/if}
  </section>
{/if}

<style>
  .received-attachments {
    padding: 20px;
    margin: 20px 0;
    border: 1px solid var(--theme-border);
    border-radius: 10px;
    background: var(--theme-surface);
  }
  h3 {
    margin: 0 0 10px;
    font-size: 14px;
  }
  ul {
    margin: 0;
    padding: 0;
    list-style: none;
  }
  li {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 8px 16px;
    padding: 10px 0;
    border-top: 1px solid var(--theme-border);
  }
  li:first-child {
    border-top: 0;
  }
  .what {
    flex: 1 1 260px;
    min-width: 0;
  }
  strong,
  small {
    display: block;
    overflow-wrap: anywhere;
  }
  small {
    margin-top: 2px;
    color: var(--theme-muted);
    font-size: 12px;
    line-height: 1.5;
  }
  .row-actions {
    display: flex;
    flex-wrap: wrap;
    gap: 8px;
    margin-left: auto;
  }
  .hint {
    margin: 10px 0 0;
    color: var(--theme-muted);
    font-size: 12px;
    line-height: 1.5;
  }
  .error {
    color: var(--theme-error-text);
  }
</style>
