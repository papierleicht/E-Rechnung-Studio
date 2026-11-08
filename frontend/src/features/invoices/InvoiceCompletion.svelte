<script lang="ts">
  import type {
    InvoiceDraft,
    InvoiceSummary,
    InvoiceView,
  } from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/model/models";
  import InvoicePayments from "../payments/InvoicePayments.svelte";
  import InvoiceReminders from "../reminders/InvoiceReminders.svelte";
  import Icon from "../../components/Icon.svelte";
  import ExportMenu from "./ExportMenu.svelte";
  import InvoiceMail from "./InvoiceMail.svelte";
  import TaxReview from "./TaxReview.svelte";
  import ValidationReport from "../../components/ValidationReport.svelte";
  import MoreMenu, { type MoreMenuItem } from "../../components/MoreMenu.svelte";
  import FileDownIcon from "@lucide/svelte/icons/file-down";
  import FileCodeIcon from "@lucide/svelte/icons/file-code";
  import CopyIcon from "@lucide/svelte/icons/copy";
  import RepeatIcon from "@lucide/svelte/icons/repeat";
  import ReceiptTextIcon from "@lucide/svelte/icons/receipt-text";
  import TrashIcon from "@lucide/svelte/icons/trash";
  import { invoiceFormats } from "../../lib/formats";
  import { amountLabel, dateLabel, statusLabel } from "../../lib/format";
  import { reverseChargeHint } from "../../lib/taxReview";
  import { sentAtLabel } from "../../lib/mail";
  import { correctionReasons, documentLabel } from "../../lib/tax";
  let {
    onpaymentschange = async () => {},
    onpaymentbusy = (_busy: boolean) => {},
    onmailsent = (_message: string) => {},
    onmailsettings = () => {},
    draft,
    view,
    busy,
    previewPending,
    issuing,
    exportedPDF,
    resolveField,
    deleteDraft,
    validate,
    issue,
    retryExport,
    openPDF,
    exportPDF,
    exportXML,
    createCorrection,
    useInvoiceAsTemplate,
    repeatInvoice,
    applyTaxReview,
    markSent,
    deleteIssued,
    invoices = [],
  }: {
    /** All invoices, to compare dates and see whether this one went out. */
    invoices?: InvoiceSummary[];
    onpaymentschange?: () => Promise<void>;
    onpaymentbusy?: (busy: boolean) => void;
    onmailsent?: (message: string) => void;
    onmailsettings?: () => void;
    draft: InvoiceDraft;
    view: InvoiceView | null;
    busy: boolean;
    previewPending: boolean;
    issuing: boolean;
    exportedPDF: string;
    resolveField: (field: string) => void;
    deleteDraft: () => void;
    validate: () => void;
    issue: (format?: "default" | "xml") => void;
    retryExport: () => void;
    openPDF: () => void;
    exportPDF: () => void;
    exportXML: () => void;
    createCorrection: () => void;
    useInvoiceAsTemplate: () => void;
    repeatInvoice: () => void;
    applyTaxReview: (draft: InvoiceDraft) => void;
    /** Notes or withdraws that the invoice was sent outside the app. */
    markSent: (sent: boolean) => void;
    deleteIssued: () => void;
  } = $props();
  const selectedFormat = $derived(invoiceFormats.find((format) => format.id === draft.profile));
  const isXRechnung = $derived(draft.profile.startsWith("xrechnung-"));
  // Reminders follow the open balance, so they reload after payment changes.
  let paymentRevision = $state(0);
  // A mail from the app already counts as sent, so the note would only add noise.
  let mailed = $state(false);
  const sent = $derived(
    Boolean(draft.markedSentAt || invoices.find((item) => item.id === draft.id)?.sentAt),
  );
  // The latest issued invoice dated after this draft; an earlier date deserves a look.
  const laterInvoice = $derived(
    invoices
      .filter(
        (item) =>
          item.status !== "draft" && item.id !== draft.id && item.issueDate > draft.issueDate,
      )
      .sort((a, b) => b.issueDate.localeCompare(a.issueDate))[0],
  );
  const prepaid = $derived(Number(draft.prepaidAmount || "0") > 0);
  const fullyPrepaid = $derived(prepaid && view !== null && Number(view.totals.due) === 0);
  // Follow-up tasks of an issued invoice: one "Mehr" button next to "PDF öffnen".
  const moreItems = $derived.by((): MoreMenuItem[] => [
    {
      id: "save-pdf",
      icon: FileDownIcon,
      label: "PDF speichern unter …",
      hint: "Eine zusätzliche Kopie an einem Ort deiner Wahl.",
      disabled: !view?.xml,
      onclick: exportPDF,
    },
    {
      id: "save-xml",
      icon: FileCodeIcon,
      label: "XML speichern unter …",
      hint: "Nur die Rechnungsdaten, etwa für ein Rechnungsportal.",
      disabled: !view?.xml,
      onclick: () => exportXML(),
    },
    {
      id: "template",
      icon: CopyIcon,
      label: "Als Vorlage verwenden",
      hint: "Neue Rechnung mit heutigem Datum, Kunde, Positionen und Konditionen.",
      onclick: useInvoiceAsTemplate,
    },
    ...(draft.kind === "invoice"
      ? [
          {
            id: "repeat",
            icon: RepeatIcon,
            label: draft.recurringId ? "Dauerrechnung anpassen" : "Regelmäßig wiederholen",
            hint: draft.recurringId
              ? "Übernimmt Positionen und Preise für die nächsten Entwürfe."
              : "Legt zu jedem Termin einen Entwurf zur Prüfung an, etwa für Hosting.",
            onclick: repeatInvoice,
          },
          {
            id: "correction",
            icon: ReceiptTextIcon,
            label: "Korrekturgutschrift erstellen",
            hint: "Eigener Entwurf mit Bezug auf diese Rechnung, ganz oder teilweise.",
            onclick: createCorrection,
          },
        ]
      : []),
    {
      id: "delete",
      icon: TrashIcon,
      danger: true,
      label: `${documentLabel(draft.kind)} löschen …`,
      hint: "Nur solange du sie niemandem geschickt hast. Die Nummer bleibt gesperrt.",
      onclick: deleteIssued,
    },
  ]);
</script>

<section id="invoice-completion" class="invoice-completion no-print">
  {#if draft.status === "draft" && draft.sourceTaxes?.length}<p class="shop-notice">
      Die Umsatzsteuer übernimmt die App aus dem Shop, solange du die Positionen nicht änderst. Der
      Shop rundet jede Position einzeln, deshalb weicht sie um wenige Cent von der Berechnung je
      Steuersatz ab, und die Rechnung passt zum bezahlten Betrag.
    </p>{/if}
  {#if prepaid}<p class="shop-notice">
      Bereits im Shop bezahlt: {amountLabel(draft.prepaidAmount || "0")}.
      {#if fullyPrepaid}Es ist nichts mehr offen.{/if}
      {#if fullyPrepaid && draft.status === "draft"}Beim Ausstellen trägt die App die Zahlung ins
        Zahlungsbuch ein.{/if}
    </p>{/if}
  {#if draft.precedingInvoiceNumber}<p class="correction-reference">
      Korrektur zu Rechnung {draft.precedingInvoiceNumber} vom {dateLabel(
        draft.precedingInvoiceDate ?? "",
      )}. {draft.status === "draft"
        ? "Die Korrektur gilt erst, wenn du sie ausstellst."
        : "Die ursprüngliche Rechnung bleibt unverändert erhalten."}
    </p>{/if}
  {#if draft.status === "draft" && draft.kind === "credit-note" && draft.precedingInvoiceNumber}
    <div class="correction-reason" role="radiogroup" aria-labelledby="correction-reason-title">
      <strong id="correction-reason-title">Warum korrigierst du die Rechnung?</strong>
      {#each correctionReasons as reason (reason.id)}
        <label
          ><input
            type="radio"
            name="correction-reason"
            value={reason.id}
            checked={draft.correctionReason === reason.id}
            onchange={() => applyTaxReview({ ...draft, correctionReason: reason.id })}
          /><span>{reason.label}<small>{reason.effect}</small></span></label
        >
      {/each}
      {#if !draft.correctionReason}<small
          >Ohne Auswahl zählt eine Gutschrift über den vollen Betrag im ursprünglichen Zeitraum,
          eine teilweise im Zeitraum der Gutschrift.</small
        >{/if}
    </div>
  {/if}
  {#if draft.status === "draft"}
    <header class="completion-heading">
      <div>
        <h2>Rechnung prüfen und ausstellen</h2>
        <p class="heading-subtitle">Abschluss</p>
        <p>
          {draft.automaticNumber
            ? "Die angezeigte Rechnungsnummer ist vorläufig. Beim Ausstellen wird sie noch einmal geprüft und endgültig vergeben."
            : "Du hast eine eigene Rechnungsnummer gewählt. Beim Ausstellen prüft die App noch einmal, dass sie frei ist."}
          Klicke die Nummer in der Rechnung an, um einen anderen Nummernkreis oder eine eigene Nummer
          zu wählen. Fehlende Angaben erkennt die App automatisch. Du kannst auch einen unvollständigen
          Entwurf speichern und später weiterarbeiten.
        </p>
      </div>
      <span class="badge {draft.status}">{statusLabel(draft.status)}</span>
    </header>
    <ValidationReport result={view?.validation} pending={previewPending} onresolve={resolveField} />
    <TaxReview
      {draft}
      {busy}
      onapply={applyTaxReview}
      onsettings={() => resolveField("taxTreatment")}
    />
    {#if reverseChargeHint(draft)}<aside class="completion-hint">
        <p>
          Dein Kunde sitzt in einem anderen EU-Land und hat eine USt-IdNr. Bei sonstigen Leistungen
          an Unternehmen schuldet dann in der Regel der Kunde die Steuer, bei Warenlieferungen kann
          die Lieferung steuerfrei sein. Ob das hier zutrifft, entscheidest du.
        </p>
        <button class="secondary" disabled={busy} onclick={() => resolveField("taxTreatment")}
          >Steuerbehandlung ansehen</button
        >
      </aside>{/if}
    {#if laterInvoice}<aside class="completion-hint">
        <p>
          Das Rechnungsdatum {dateLabel(draft.issueDate)} liegt vor deiner Rechnung {laterInvoice.number}
          vom {dateLabel(laterInvoice.issueDate)}. Prüfe das Datum, bevor du ausstellst.
        </p>
      </aside>{/if}
    <footer class="completion-actions">
      <div class="draft-actions">
        {#if draft.id}<button class="danger-text" onclick={deleteDraft} disabled={busy}
            >Entwurf löschen</button
          >{/if}
        <button class="secondary" onclick={validate} disabled={busy}
          ><Icon name="checkFile" size={16} /> Erneut prüfen</button
        >
      </div>
      <div class="issue-action">
        <span
          >{previewPending
            ? "Änderungen werden geprüft …"
            : view?.validation.state === "valid"
              ? isXRechnung
                ? "Beim Ausstellen speichert die App die XRechnung-XML und die passende PDF automatisch im Exportordner."
                : "Beim Ausstellen speichert die App die passende E-Rechnungs-PDF automatisch im Exportordner."
              : "Ergänze die oben genannten Angaben. Einen Entwurf kannst du jederzeit speichern."}</span
        >
        <div class="issue-split">
          <button
            class="primary issue-button"
            onclick={() => issue()}
            disabled={busy || previewPending || view?.validation.state !== "valid"}
            >{issuing
              ? "Wird ausgestellt …"
              : documentLabel(draft.kind) + " verbindlich ausstellen"}</button
          >
          <ExportMenu
            disabled={busy || previewPending || view?.validation.state !== "valid"}
            onxml={() => issue("xml")}
          />
        </div>
      </div>
    </footer>
  {:else}
    <header class="completion-heading">
      <div>
        <h2>
          {documentLabel(draft.kind)} ausgestellt
        </h2>
        <p class="heading-subtitle">Export & Zahlung</p>
        <p role="status">{draft.number} · Endgültig gespeichert</p>
      </div>
      <span class="badge {draft.status}">{statusLabel(draft.status)}</span>
    </header>
    <div class="issued-downloads">
      <div>
        <h3>{selectedFormat?.label ?? "E-Rechnung exportieren"}</h3>
        <p>
          {isXRechnung
            ? "Die XRechnung-XML ist bereit für den Versand oder das Rechnungsportal. Die zusätzlich erzeugte PDF enthält dieselbe XML als Anhang."
            : "Die PDF, die beim Ausstellen erzeugt wurde, enthält die eingebetteten Rechnungsdaten und ist bereit für den Versand."}
        </p>
      </div>
      <div class="document-actions">
        {#if view?.export.error}<button class="primary" onclick={retryExport} disabled={busy}
            ><Icon name="refresh" size={16} /> Export wiederholen</button
          >{:else}<button class="primary" onclick={openPDF} disabled={busy || !view?.xml}
            ><Icon name="open" size={16} /> PDF öffnen</button
          >{/if}
        <MoreMenu items={moreItems} disabled={busy} title="Weitere Rechnungsaktionen" />
      </div>
      {#if draft.id}<InvoiceMail
          invoiceId={draft.id}
          {busy}
          onsent={onmailsent}
          onsettings={onmailsettings}
          onmailed={(sent) => (mailed = sent)}
        />{/if}
      {#if draft.markedSentAt}
        <div class="sent-mark">
          <p role="status">
            <Icon name="check" size={15} /> Als verschickt markiert am {sentAtLabel(
              draft.markedSentAt,
            )}.
          </p>
          <button class="text" disabled={busy} onclick={() => markSent(false)}
            >Markierung zurücknehmen</button
          >
        </div>
      {:else if !mailed}
        <div class="sent-mark">
          <p>Selbst verschickt, etwa mit deinem E-Mail-Programm?</p>
          <button class="text" disabled={busy} onclick={() => markSent(true)}
            >Als verschickt markieren</button
          >
        </div>
      {/if}
      {#if view?.export.error}<p class="export-error" role="alert">
          {view.export.error}
        </p>{/if}
      {#if exportedPDF}<p class="export-result" role="status">
          Gespeichert: {exportedPDF}
        </p>{/if}
      {#if view?.export.xmlPath}<p class="export-result">
          XML: {view.export.xmlPath}
        </p>{/if}
    </div>
    {#if view?.validation.state === "valid"}<div class="detail-list issued-validation">
        <details>
          <summary
            ><span class="detail-title">Technische Prüfung</span><span class="detail-state good"
              >Bestanden</span
            ></summary
          >
          <p>Die gespeicherte E-Rechnung entspricht den unterstützten Format- und Rechenregeln.</p>
        </details>
      </div>{:else}<ValidationReport result={view?.validation} />{/if}
    <InvoicePayments
      invoiceID={draft.id}
      onchanged={async () => {
        await onpaymentschange();
        paymentRevision++;
      }}
      onbusychange={onpaymentbusy}
      oncorrection={draft.kind === "credit-note" ? undefined : createCorrection}
    />
    {#if draft.kind !== "credit-note"}<InvoiceReminders
        invoiceID={draft.id}
        unsent={!sent}
        revision={paymentRevision}
        onbusychange={onpaymentbusy}
        {onmailsettings}
      />{/if}
  {/if}
</section>

<style>
  .completion-hint {
    display: flex;
    flex-wrap: wrap;
    gap: 10px 16px;
    align-items: center;
    justify-content: space-between;
    margin: 0 24px 16px;
    padding: 12px 14px;
    border: 1px solid var(--theme-border);
    border-radius: 8px;
    background: var(--theme-surface-muted);
    font-size: 13px;
    line-height: 1.5;
  }
  .completion-hint p {
    flex: 1 1 320px;
    margin: 0;
  }
  .shop-notice {
    margin: 0;
    padding: 16px 24px;
    background: var(--theme-success-bg);
    color: var(--theme-success-text);
    font-size: 13px;
    line-height: 1.5;
  }
</style>
