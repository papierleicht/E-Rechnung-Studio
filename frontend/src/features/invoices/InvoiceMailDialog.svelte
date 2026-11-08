<script lang="ts">
  import { untrack } from "svelte";
  import FormDialog from "../../components/FormDialog.svelte";
  import { serviceErrorMessage } from "../../lib/errors";
  import {
    fileSizeLabel,
    recipientList,
    type MailDraftView,
    type MailFields,
  } from "../../lib/mail";
  let {
    title,
    prepared,
    send: deliver,
    onclose,
    onsent,
    onsettings,
  }: {
    title: string;
    prepared: MailDraftView;
    /** Sends the confirmed fields for the dialog's document. */
    send: (fields: MailFields) => Promise<{ to?: string[] | null }>;
    onclose: () => void;
    onsent: (message: string) => void;
    onsettings: () => void;
  } = $props();
  // The dialog edits a copy; the document itself never changes.
  const initial = untrack(() => prepared);
  let to = $state(initial.to);
  let cc = $state("");
  let subject = $state(initial.subject);
  let body = $state(initial.body);
  let includePDF = $state(initial.includePdf ?? false);
  let saveRecipient = $state(initial.canSaveEmail);
  let confirmed = $state(false);
  let step = $state<"edit" | "preview">("edit");
  let sending = $state(false);
  let error = $state("");
  const ready = $derived(prepared.configured && prepared.hasPassword);
  const attachments = $derived([
    ...(prepared.attachments ?? []),
    ...(prepared.xrechnung && includePDF && prepared.pdfAttachment?.filename
      ? [prepared.pdfAttachment]
      : []),
  ]);
  function showPreview() {
    error = "";
    if (!recipientList(to).length) {
      error = "Gib die E-Mail-Adresse des Empfängers ein.";
      return;
    }
    if (!subject.trim()) {
      error = "Gib einen Betreff ein.";
      return;
    }
    confirmed = false;
    step = "preview";
  }
  async function send() {
    if (sending || !confirmed) return;
    sending = true;
    error = "";
    try {
      const entry = await deliver({
        to,
        cc,
        subject,
        body,
        includePdf: includePDF,
        saveRecipient: prepared.canSaveEmail && saveRecipient,
        confirmed,
      });
      onsent(`E-Mail an ${(entry.to ?? []).join(", ")} versendet`);
    } catch (cause) {
      error = serviceErrorMessage(cause, "Die E-Mail ließ sich nicht versenden.");
    } finally {
      sending = false;
    }
  }
</script>

<FormDialog
  {title}
  description={step === "edit" ? "Nachricht bearbeiten" : "Vorschau und Bestätigung"}
  busy={sending}
  {error}
  {onclose}
  onsubmit={() => (step === "edit" ? showPreview() : send())}
>
  {#if !ready}
    <p class="mail-setup-hint" role="alert">
      {prepared.configured
        ? "Für dein E-Mail-Konto ist noch kein Passwort gespeichert."
        : "Du hast noch kein E-Mail-Konto eingerichtet."}
      <button type="button" class="text" onclick={onsettings}>E-Mail-Einstellungen öffnen</button>
    </p>
  {/if}
  {#if prepared.templateWarning}<p class="mail-setup-hint">
      Vorlage: {prepared.templateWarning}
    </p>{/if}
  {#if step === "edit"}
    <div class="form-grid">
      <label
        >An<input
          type="text"
          bind:value={to}
          required
          placeholder="kunde@example.de"
          autocomplete="email"
        /></label
      >
      {#if prepared.canSaveEmail}<label class="mail-inline"
          ><input type="checkbox" bind:checked={saveRecipient} /> Adresse beim Kunden {prepared.customerName}
          speichern</label
        >{:else if !prepared.customerId && !prepared.to}<p class="form-help">
          Dieser Beleg gehört zu keinem gespeicherten Kunden. Die Adresse gilt nur für diesen
          Versand.
        </p>{/if}
      <label>Kopie an (optional)<input type="text" bind:value={cc} /></label>
      <label>Betreff<input bind:value={subject} required /></label>
      <label>Text<textarea rows="10" bind:value={body}></textarea></label>
      {#if prepared.xrechnung}<label class="mail-inline"
          ><input type="checkbox" bind:checked={includePDF} /> Zusätzlich die PDF-Ansicht anhängen</label
        >{/if}
    </div>
  {:else}
    <dl class="mail-preview">
      <dt>Von</dt>
      <dd>{prepared.from}</dd>
      <dt>An</dt>
      <dd>{recipientList(to).join(", ")}</dd>
      {#if recipientList(cc).length}<dt>Kopie</dt>
        <dd>{recipientList(cc).join(", ")}</dd>{/if}
      {#if prepared.bccSelf}<dt>BCC</dt>
        <dd>{prepared.from}</dd>{/if}
      {#if prepared.replyTo}<dt>Antwort an</dt>
        <dd>{prepared.replyTo}</dd>{/if}
      <dt>Betreff</dt>
      <dd><strong>{subject}</strong></dd>
      <dt>Anhänge</dt>
      <dd>
        <ul>
          {#each attachments as file}<li>{file.filename} ({fileSizeLabel(file.size)})</li>{/each}
        </ul>
      </dd>
    </dl>
    <pre class="mail-preview-body">{body}</pre>
    <label class="mail-inline mail-confirm"
      ><input type="checkbox" bind:checked={confirmed} /> Ich habe Empfänger, Text und Anhänge geprüft
      und möchte die E-Mail jetzt senden.</label
    >
  {/if}
  {#snippet footer()}
    {#if step === "preview"}<button
        type="button"
        class="secondary"
        disabled={sending}
        onclick={() => (step = "edit")}>Zurück</button
      >{/if}
    <span class="footer-spacer"></span><button
      type="button"
      class="secondary"
      disabled={sending}
      onclick={onclose}>Abbrechen</button
    >{#if step === "edit"}<button class="primary" disabled={!ready}>Vorschau anzeigen</button
      >{:else}<button class="primary" disabled={!ready || !confirmed || sending}
        >{sending ? "Wird gesendet …" : "Jetzt senden"}</button
      >{/if}
  {/snippet}
</FormDialog>

<style>
  .mail-inline {
    flex-direction: row;
    align-items: center;
    gap: 8px;
    min-width: 0;
    font-weight: 400;
    overflow-wrap: anywhere;
  }
  textarea {
    font: inherit;
    resize: vertical;
  }
  .mail-setup-hint {
    padding: 10px 12px;
    border-radius: 7px;
    background: var(--theme-warning-bg);
    color: var(--theme-warning-text);
  }
  .mail-preview {
    display: grid;
    grid-template-columns: max-content minmax(0, 1fr);
    gap: 6px 14px;
    margin: 0 0 12px;
  }
  .mail-preview dt {
    color: var(--theme-muted);
  }
  .mail-preview dd {
    margin: 0;
    overflow-wrap: anywhere;
  }
  .mail-preview ul {
    margin: 0;
    padding-left: 18px;
  }
  .mail-preview-body {
    max-height: 260px;
    overflow: auto;
    padding: 12px;
    border: 1px solid var(--theme-border);
    border-radius: 7px;
    background: var(--theme-surface-muted);
    font: inherit;
    white-space: pre-wrap;
    overflow-wrap: anywhere;
    overscroll-behavior: contain;
  }
  .mail-confirm {
    margin-top: 12px;
  }
</style>
