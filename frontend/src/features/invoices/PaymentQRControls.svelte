<script lang="ts">
  import type {
    Company,
    InvoiceDraft,
  } from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/model/models";
  let { draft = $bindable(), company }: { draft: InvoiceDraft; company: Company } = $props();
  const hintId = $props.id();
  const hasIBAN = $derived(
    !!company.iban?.trim() || !!company.additionalAccounts?.some((account) => account.iban?.trim()),
  );
</script>

<div class="payment-qr-settings" role="group" aria-labelledby={`${hintId}-title`}>
  <p id={`${hintId}-title`} class="qr-title">Zahlungs-QR-Code</p>
  <div class="qr-options">
    <label>
      <input
        type="checkbox"
        data-field="disablePaymentQR"
        disabled={!hasIBAN}
        aria-describedby={!hasIBAN ? hintId : undefined}
        checked={hasIBAN && !draft.disablePaymentQR}
        onchange={(event) => (draft.disablePaymentQR = !event.currentTarget.checked)}
      />
      QR-Code anzeigen
    </label>
    <div class="qr-details" class:unavailable={!hasIBAN || draft.disablePaymentQR}>
      <label>
        <input
          type="checkbox"
          data-field="hidePaymentQRDetails"
          disabled={!hasIBAN || draft.disablePaymentQR}
          aria-describedby={`${hintId}-details${!hasIBAN ? ` ${hintId}` : ""}`}
          checked={hasIBAN && !draft.hidePaymentQRDetails}
          onchange={(event) => (draft.hidePaymentQRDetails = !event.currentTarget.checked)}
        />
        Zusätzliche Informationen anzeigen
      </label>
      <p id={`${hintId}-details`} class="qr-description">
        Erklärungstext, Empfänger, IBAN, Betrag und Verwendungszweck neben dem QR-Code.
      </p>
    </div>
  </div>
  <p class="qr-note">
    Gilt für Rechnungsansicht und PDF. Den QR-Code gibt es nur bei SEPA-Überweisungen in EUR mit
    passendem Betrag und vollständigen Zahlungsdaten, nicht bei Gutschriften.
  </p>
  {#if !hasIBAN}
    <p id={hintId} class="qr-hint">
      Ohne IBAN gibt es keinen QR-Code. Trage deine IBAN im Unternehmensprofil unter Bankverbindung
      ein.
    </p>
  {/if}
</div>

<style>
  .payment-qr-settings {
    margin: 8px 0;
    padding: 16px;
    border: 1px solid var(--theme-border);
    border-radius: 8px;
  }
  .qr-title {
    margin: 0 0 12px;
    color: var(--theme-text);
    font-size: 13px;
    font-weight: 600;
  }
  .qr-options {
    display: flex;
    flex-direction: column;
    gap: 12px;
  }
  .qr-details {
    margin-left: 26px;
  }
  .qr-description,
  .qr-note,
  .qr-hint {
    color: var(--theme-muted);
    font-size: 12px;
    line-height: 1.5;
  }
  .qr-description {
    margin: 4px 0 0 26px;
  }
  .qr-details.unavailable .qr-description {
    color: var(--theme-muted);
  }
  .qr-note {
    margin: 14px 0 0;
  }
  .qr-hint {
    margin: 12px 0 0;
    padding-top: 12px;
    border-top: 1px solid var(--theme-border);
  }
</style>
