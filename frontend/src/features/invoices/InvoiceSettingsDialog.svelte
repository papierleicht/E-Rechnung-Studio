<script lang="ts">
  import SettingSection from "../../components/SettingSection.svelte";
  import { dismissDialog } from "../../lib/dismissDialog";
  import { clone } from "../../lib/invoiceDraft";
  import Icon from "../../components/Icon.svelte";
  import { onDestroy, onMount, tick, untrack } from "svelte";
  import type {
    Company,
    InvoiceDraft,
  } from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/model/models";
  import ElectronicAddress from "../../components/ElectronicAddress.svelte";
  import { invoiceFormats } from "../../lib/formats";
  import {
    taxChoices,
    taxLabel,
    documentLabel,
    paymentChoices,
    invoiceTaxDefaults,
    initializePositionTaxes,
  } from "../../lib/tax";
  import AdjustmentsEditor from "./AdjustmentsEditor.svelte";
  import PaymentQRControls from "./PaymentQRControls.svelte";

  let {
    draft,
    company,
    kind = "invoice",
    busy,
    error,
    focusField = "",
    onsave,
    ondirtychange,
    oncancel,
    oncompany,
  }: {
    draft: InvoiceDraft;
    company: Company;
    /** Quotes share the tax and discount settings; payment and e-invoice data are invoice-only. */
    kind?: "invoice" | "quote";
    busy: boolean;
    error: string;
    focusField?: string;
    onsave: (value: InvoiceDraft) => void;
    ondirtychange: (dirty: boolean) => void;
    oncancel: () => void;
    oncompany?: () => void;
  } = $props();
  const invoice = $derived(kind === "invoice");
  const noun = $derived(invoice ? "Rechnung" : "Angebot");
  let dialog: HTMLDialogElement;
  let working: InvoiceDraft = $state(
    untrack(() => ({ ...clone(draft), paymentMeansCode: draft.paymentMeansCode || "" })),
  );
  const initial = untrack(() => JSON.stringify(working));
  let uniformTax = $state(
    untrack(() =>
      draft.taxTreatment === "mixed"
        ? company.smallBusiness
          ? "small-business"
          : "standard"
        : draft.taxTreatment,
    ),
  );
  let uniformReason = $state(untrack(() => draft.taxExemptionReason));
  function togglePositionTax(enabled: boolean) {
    if (enabled) {
      uniformTax = working.taxTreatment;
      uniformReason = working.taxExemptionReason;
      initializePositionTaxes(working, uniformTax, uniformReason);
      working.taxTreatment = "mixed";
      working.taxExemptionReason = "";
    } else {
      working.taxTreatment = uniformTax;
      working.taxExemptionReason = uniformReason;
    }
    working.taxSource = "manual";
  }
  const format = $derived(invoiceFormats.find((f) => f.id === working.profile));
  const xrechnung = $derived(working.profile.startsWith("xrechnung-"));
  function revealField(field: HTMLElement | null) {
    let parent = field?.parentElement;
    while (parent && parent !== dialog) {
      if (parent instanceof HTMLDetailsElement) parent.open = true;
      parent = parent.parentElement;
    }
  }
  onMount(() => {
    dialog.showModal();
    if (focusField) {
      const field = dialog.querySelector<HTMLElement>(`[data-field="${focusField}"]`);
      revealField(field);
      field?.focus();
    }
    // Native form validation must reveal fields even in collapsed advanced groups.
    const invalid = (event: Event) => revealField(event.target as HTMLElement);
    dialog.addEventListener("invalid", invalid, true);
    return () => dialog.removeEventListener("invalid", invalid, true);
  });
  function cancel() {
    if (!busy) oncancel();
  }
  $effect(() => {
    if (error)
      tick().then(() =>
        dialog.querySelector<HTMLElement>(".dialog-error")?.scrollIntoView({ block: "nearest" }),
      );
  });
  $effect(() => ondirtychange(JSON.stringify(working) !== initial));
  onDestroy(() => ondirtychange(false));
</script>

<dialog
  use:dismissDialog
  aria-labelledby="page-settings-title"
  bind:this={dialog}
  class="invoice-settings-dialog"
  oncancel={(event) => {
    event.preventDefault();
    cancel();
  }}
>
  <form
    class="modal page-settings"
    onsubmit={(event) => {
      event.preventDefault();
      onsave(working);
    }}
    aria-labelledby="page-settings-title"
  >
    <header>
      <div>
        <h2 id="page-settings-title">{noun}seinstellungen</h2>
        <p>
          {!invoice
            ? "Gilt für dieses Angebot. Eine spätere Rechnung übernimmt Steuerfall, Rabatte und Zuschläge."
            : draft.customerId
              ? `Für ${draft.customer.name} und künftige Rechnungen an diesen Kunden.`
              : "Gilt für diesen Entwurf. Bei gespeicherten Kunden merkt sich die App die Einstellungen für künftige Rechnungen."}
        </p>
      </div>
      <button
        type="button"
        aria-label="Schließen"
        disabled={busy}
        class="dialog-close"
        title="Schließen (Esc)"
        onclick={cancel}><Icon name="close" /></button
      >
    </header>
    <div class="page-settings-body">
      <fieldset class="dialog-fields" disabled={busy}>
        <dl class="invoice-settings-summary">
          <div>
            <dt>Abrechnung</dt>
            <dd>{taxLabel(working.taxTreatment)}</dd>
          </div>
          {#if invoice}<div>
              <dt>Ausgabe</dt>
              <dd>
                {working.profile === "facturx-en16931"
                  ? "E-Rechnung als PDF"
                  : format?.label || working.profile}
              </dd>
            </div>{/if}
          {#if invoice && working.kind !== "invoice"}<div>
              <dt>Beleg</dt>
              <dd>{documentLabel(working.kind)}</dd>
            </div>{/if}
          {#if working.adjustments?.length}<div>
              <dt>Rabatte / Zuschläge</dt>
              <dd>{working.adjustments.length} Einträge</dd>
            </div>{/if}
        </dl>
        <SettingSection
          title="Erweiterte Einstellungen"
          summary={invoice
            ? "Belegart, Steuerfall, Rabatte, Zahlungsart und E-Rechnungsformat"
            : "Steuerfall sowie Rabatte und Zuschläge"}
        >
          <div class="form-grid cols-2">
            {#if invoice}<label
                >Belegart<select data-field="kind" bind:value={working.kind}
                  ><option value="invoice">Rechnung</option><option value="credit-note"
                    >Gutschrift</option
                  ><option value="corrected-invoice">Rechnungskorrektur</option></select
                ></label
              >{/if}
            <label class="wide-setting position-tax-toggle">
              <input
                type="checkbox"
                data-field="taxTreatment"
                checked={working.taxTreatment === "mixed"}
                onchange={(event) => togglePositionTax(event.currentTarget.checked)}
              />
              <span>Umsatzsteuer je Position festlegen</span>
            </label>
            <label class="wide-setting" class:tax-muted={working.taxTreatment === "mixed"}
              >Steuerfall<select
                disabled={working.taxTreatment === "mixed"}
                value={working.taxTreatment === "mixed" ? uniformTax : working.taxTreatment}
                onchange={(event) => {
                  working.taxTreatment = event.currentTarget.value;
                  working.taxSource = "manual";
                  working.taxExemptionReason =
                    working.taxTreatment === "small-business"
                      ? "Umsatzsteuerbefreit nach § 19 UStG"
                      : "";
                }}
              >
                <optgroup label="Übliche Abrechnung"
                  >{#each taxChoices.filter( (c) => ["standard", "small-business"].includes(c.id) ) as choice}<option
                      value={choice.id}>{choice.label}</option
                    >{/each}</optgroup
                >
                <optgroup label="Differenzbesteuerung"
                  >{#each taxChoices.filter((c) => c.id.startsWith("margin-")) as choice}<option
                      value={choice.id}>{choice.label}</option
                    >{/each}</optgroup
                >
                <optgroup label="Weitere Steuerfälle"
                  >{#each taxChoices.filter((c) => !["standard", "small-business"].includes(c.id) && !c.id.startsWith("margin-")) as choice}<option
                      value={choice.id}>{choice.label}</option
                    >{/each}</optgroup
                >
              </select></label
            >
            <div class="wide-setting">
              <p class="form-help">
                {working.taxSource === "company"
                  ? "Aus den Profilvorgaben übernommen."
                  : working.taxSource === "customer"
                    ? "Abweichende Kundenvorgabe übernommen."
                    : invoice
                      ? "Auf dieser Rechnung festgelegt."
                      : "Auf diesem Angebot festgelegt."} Aktuelle Profilvorgabe: {taxLabel(
                  invoiceTaxDefaults(company).taxTreatment,
                )}.
              </p>
              <button
                type="button"
                class="text"
                disabled={busy}
                onclick={() => {
                  Object.assign(working, invoiceTaxDefaults(company));
                  if (working.taxTreatment === "mixed")
                    initializePositionTaxes(
                      working,
                      company.smallBusiness ? "small-business" : "standard",
                    );
                }}>Profilvorgaben übernehmen</button
              >
              <p class="form-help">
                {invoice
                  ? "Übernimmt Steuerbehandlung und Hinweis in diese Rechnung. Nach dem Speichern richten sich neue Rechnungen dieses Kunden wieder nach dem Profil."
                  : "Übernimmt Steuerbehandlung und Hinweis in dieses Angebot."}
              </p>
            </div>
            {#if working.taxTreatment === "mixed"}<p class="form-help wide-setting">
                Steuerfall und Befreiungsgrund wählst du beim Bearbeiten der einzelnen Positionen
                aus.
              </p>{/if}
            {#if working.taxTreatment.startsWith("margin-")}<p class="form-help wide-setting">
                {invoice ? "Die Rechnung" : "Das Angebot"} weist die Umsatzsteuer nicht gesondert aus
                und enthält den passenden Hinweis zur Sonderregelung. Die interne Besteuerung deiner Marge
                berechnet die App nicht.
              </p>{/if}
            {#if working.taxTreatment === "out-of-scope"}<p class="form-help wide-setting">
                Kategorie O wird als eigene Rechnung ausgegeben. USt-IdNrn. werden in deren XML
                nicht übermittelt. Deine Stammdaten bleiben gespeichert.
              </p>{/if}
            {#if !["standard", "zero-rated", "mixed", "margin-goods", "margin-art", "margin-collectibles"].includes(working.taxTreatment)}<label
                class="wide-setting"
                >Steuerlicher Hinweis<input
                  data-field="taxExemptionReason"
                  bind:value={working.taxExemptionReason}
                  oninput={() => (working.taxSource = "manual")}
                  required={working.taxTreatment === "exempt"}
                  placeholder={working.taxTreatment === "exempt"
                    ? "Grund der Steuerbefreiung"
                    : "Standardhinweis automatisch verwenden"}
                /></label
              >{/if}
            {#if invoice}<label class="wide-setting"
                >Zahlungsart<select
                  data-field="paymentMeansCode"
                  bind:value={working.paymentMeansCode}
                  ><option value="">Unternehmensstandard</option
                  >{#each paymentChoices as choice}<option value={choice.id}>{choice.label}</option
                    >{/each}</select
                ></label
              >
              <div class="wide-setting">
                <PaymentQRControls bind:draft={working} {company} />
              </div>
              {#if working.kind === "corrected-invoice" || working.precedingInvoiceNumber}
                <label
                  >Ursprüngliche Rechnungsnummer<input
                    data-field="precedingInvoiceNumber"
                    required={working.kind === "corrected-invoice"}
                    bind:value={working.precedingInvoiceNumber}
                  /></label
                >
                <label
                  >Ursprüngliches Rechnungsdatum<input
                    type="date"
                    data-field="precedingInvoiceDate"
                    bind:value={working.precedingInvoiceDate}
                  /></label
                >
                {#if working.kind === "corrected-invoice"}<p class="form-help wide-setting">
                    Negative Mengen mindern den ursprünglichen Betrag, positive Mengen erhöhen ihn.
                    Einzelpreise bleiben positiv. Die ursprüngliche Rechnung bleibt unverändert
                    erhalten.
                  </p>{/if}
              {/if}
              <label class="wide-setting"
                >E-Rechnungsformat<select
                  data-field="profile"
                  bind:value={working.profile}
                  onchange={() => {
                    if (format) working.syntax = format.syntax;
                  }}
                >
                  {#each invoiceFormats as choice}<option value={choice.id}>{choice.label}</option
                    >{/each}
                  {#if !format}<option value={working.profile}>Nicht unterstütztes Format</option
                    >{/if}
                </select><small>{format?.description}</small></label
              >{/if}
          </div>
          <AdjustmentsEditor bind:draft={working} />
          {#if invoice && xrechnung}<div class="xrechnung-options form-grid">
              <strong>Angaben für XRechnung</strong>
              <label
                >Käuferreferenz / Leitweg-ID<input
                  data-field="buyerReference"
                  bind:value={working.buyerReference}
                  placeholder="Vom Empfänger mitgeteilt"
                /></label
              >
              <ElectronicAddress
                bind:endpoint={working.customer.endpoint}
                bind:scheme={working.customer.endpointScheme}
                email={working.customer.email}
                field="customer"
              />
              {#if working.taxTreatment === "intra-community" || (working.taxTreatment === "mixed" && working.lines?.some((line) => line.taxTreatment === "intra-community"))}
                <label
                  >Lieferort<input
                    data-field="deliveryCity"
                    bind:value={working.deliveryCity}
                  /></label
                ><label
                  >Postleitzahl der Lieferung<input
                    data-field="deliveryPostcode"
                    bind:value={working.deliveryPostcode}
                  /></label
                >
              {/if}
              <label
                >Geschäftsprozess<input
                  data-field="businessProcess"
                  bind:value={working.businessProcess}
                  placeholder="urn:fdc:peppol.eu:2017:poacc:billing:01:1.0"
                /></label
              >
              {#if oncompany}<button type="button" class="text" disabled={busy} onclick={oncompany}
                  >Absenderangaben bearbeiten</button
                >{/if}
            </div>{/if}
        </SettingSection>
        {#if error}<p class="dialog-error" role="alert">{error}</p>{/if}
      </fieldset>
    </div>
    <footer>
      <button class="secondary" type="button" disabled={busy} onclick={cancel}>Abbrechen</button
      ><button class="primary" disabled={busy}
        >{busy ? "Wird gespeichert …" : "Einstellungen speichern"}</button
      >
    </footer>
  </form>
</dialog>

<style>
  .position-tax-toggle {
    display: flex;
    align-items: center;
    gap: 10px;
  }
  .position-tax-toggle input {
    width: 16px;
    height: 16px;
    min-height: 0;
    margin: 0;
    padding: 0;
    flex: 0 0 auto;
  }
  .tax-muted {
    color: var(--theme-muted);
  }
  .tax-muted select {
    background: var(--theme-surface-muted);
    color: var(--theme-muted);
    cursor: not-allowed;
  }
</style>
