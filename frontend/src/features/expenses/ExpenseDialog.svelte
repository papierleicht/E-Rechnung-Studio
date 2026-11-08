<script lang="ts">
  import SettingSection from "../../components/SettingSection.svelte";
  import { untrack } from "svelte";
  import * as API from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/app/invoiceservice";
  import type { Expense } from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/store/models";
  import type { Category } from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/taxforms/models";
  import FormDialog from "../../components/FormDialog.svelte";
  import FilePicker from "../../components/FilePicker.svelte";
  import Icon from "../../components/Icon.svelte";
  import { serviceErrorMessage } from "../../lib/errors";
  import { amountLabel, localToday } from "../../lib/format";
  import { encodeInvoiceFile } from "../../lib/received";
  import {
    assetTreatment,
    attachmentType,
    isReverseCharge,
    normalizeAmount,
    paidOptions,
    reverseChargeHint,
    splitGross,
    taxRates,
    usefulLives,
  } from "../../lib/expenses";

  let {
    initial,
    categories,
    smallBusiness = false,
    onclose,
    onsaved,
    ondelete,
    onopenfile,
  }: {
    /** Prefilled values, e.g. from a bank booking; an ID edits a receipt. */
    initial: Partial<Expense>;
    categories: Category[];
    smallBusiness?: boolean;
    onclose: () => void;
    onsaved: (expense: Expense) => void;
    /** Offered for a saved receipt. */
    ondelete?: () => void;
    /** Offered when the receipt has a file attached. */
    onopenfile?: () => void;
  } = $props();

  const start = untrack(() => initial);
  const today = localToday();
  const cancelled = !!start.cancelledAt;
  const locked = !!start.locked;
  // A closed receipt only takes its missing file; a cancelled one nothing.
  const frozen = cancelled || locked;
  const canSave = !cancelled && (!locked || !start.fileSum);
  let date = $state(start.date || today);
  let supplier = $state(start.supplier ?? "");
  let category = $state(start.category ?? "");
  let gross = $state(start.gross ? start.gross.replace(".", ",") : "");
  let taxRate = $state(start.taxRate || (untrack(() => smallBusiness) ? "none" : "19"));
  let reducedGross = $state(start.reducedGross ? start.reducedGross.replace(".", ",") : "");
  let paid = $state(start.id && !start.paidDate ? "open" : start.paidWith || "bank");
  let paidDate = $state(start.paidDate || start.date || today);
  let advance = $state(!!start.advance);
  // No preset useful life: a wrong one would write off the whole price at once.
  let years = $state(start.years || 0);
  let note = $state(start.note ?? "");
  let documentNumber = $state(start.documentNumber ?? "");
  let currency = $state(start.currency ?? "");
  let originalGross = $state(start.originalGross ? start.originalGross.replace(".", ",") : "");
  let file = $state<File | null>(null);
  let busy = $state(false);
  let error = $state("");

  const chosen = $derived(categories.find((c) => c.id === category));
  const split = $derived(splitGross(gross, taxRate, reducedGross));
  const linked = $derived(!!start.bookingId);
  // Only tax that goes into the Voranmeldung can wait for the payment.
  const taxed = $derived(
    !!split && (isReverseCharge(taxRate) || (!smallBusiness && split.tax > 0)),
  );

  async function save() {
    if (busy) return;
    error = "";
    const amount = normalizeAmount(gross);
    if (!amount) {
      error = "Gib den Betrag ein, der auf dem Beleg steht.";
      return;
    }
    if (category === "asset" && !years) {
      error = "Wähle, wie lange du die Anschaffung nutzt.";
      return;
    }
    if (taxRate === "mixed" && !split) {
      error =
        "Gib an, welcher Teil des Betrags mit 7 % besteuert ist. Er muss kleiner sein als der Gesamtbetrag.";
      return;
    }
    busy = true;
    try {
      let encoded = "";
      let attachmentName = start.attachmentName ?? "";
      let type = start.attachmentType ?? "";
      if (file) {
        type = attachmentType(file.name);
        if (!type) throw new Error("Hänge ein PDF oder Foto (JPG, PNG, HEIC) an.");
        if (file.size > 10 * 1024 * 1024)
          throw new Error("Die Datei ist größer als 10 MB. Wähle eine kleinere Datei.");
        encoded = await encodeInvoiceFile(file);
        attachmentName = file.name;
      }
      const saved = await API.SaveExpense({
        expense: {
          ...(start as Expense),
          date,
          supplier,
          category,
          gross: amount,
          taxRate,
          reducedGross: taxRate === "mixed" ? normalizeAmount(reducedGross) : "",
          paidWith: paid === "open" ? "bank" : paid,
          paidDate: paid === "open" ? "" : paidDate,
          advance: taxed && advance,
          years: category === "asset" ? years : 0,
          note,
          documentNumber,
          currency,
          originalGross: currency ? normalizeAmount(originalGross) : "",
          attachmentName,
          attachmentType: type,
        },
        file: encoded,
      });
      onsaved(saved);
    } catch (cause) {
      error = serviceErrorMessage(
        cause,
        "Der Beleg konnte nicht gespeichert werden. Versuche es noch einmal.",
      );
    } finally {
      busy = false;
    }
  }
</script>

<FormDialog
  title={start.id ? `Beleg ${start.number ?? ""}`.trim() : "Beleg erfassen"}
  description="Für Kassenbons, PDF-Rechnungen und Ausgaben ohne E-Rechnung"
  {busy}
  {error}
  {onclose}
  onsubmit={save}
>
  {#if cancelled}<p class="expense-state" role="status">
      Storniert: {start.cancelReason}. Der Beleg zählt nicht mehr und bleibt zur Nachvollziehbarkeit
      erhalten.
    </p>
  {:else if locked}<p class="expense-state" role="status">
      Festgeschrieben: {start.locked}
      {start.fileSum
        ? "Du kannst den Beleg nur noch stornieren."
        : "Du kannst den Beleg nur noch stornieren oder die fehlende Datei ergänzen."}
    </p>{/if}
  <fieldset class="form-grid expense-form" disabled={frozen}>
    <label
      >Bei wem hast du eingekauft?<input
        bind:value={supplier}
        required
        maxlength="200"
        placeholder="z. B. Bürohandel Müller"
      /></label
    >
    <label>Datum auf dem Beleg<input type="date" bind:value={date} required /></label>
    <label class="wide"
      >Nummer auf dem Beleg (falls vorhanden)<input
        bind:value={documentNumber}
        maxlength="100"
      /></label
    >
    <label class="wide"
      >Wofür war die Ausgabe?<select bind:value={category} required>
        <option value="" disabled>Auswählen …</option>
        {#each categories as c (c.id)}<option value={c.id}>{c.label}</option>{/each}
      </select></label
    >
    {#if chosen}<p class="form-help wide">{chosen.hint}</p>{/if}
    <label
      >Betrag laut Beleg (brutto)<input
        inputmode="decimal"
        bind:value={gross}
        required
        readonly={linked}
        placeholder="0,00"
      /></label
    >
    <label
      >Umsatzsteuer auf dem Beleg<select bind:value={taxRate}>
        {#each taxRates as r (r.id)}<option value={r.id}>{r.label}</option>{/each}
      </select></label
    >
    {#if taxRate === "mixed"}<label
        >Davon mit 7 % (brutto)<input
          inputmode="decimal"
          bind:value={reducedGross}
          required
          placeholder="z. B. Speisen"
        /></label
      >
      <p class="form-help">Der Rest des Betrags wird mit 19 % gerechnet.</p>{/if}
    {#if split && isReverseCharge(taxRate)}<p class="form-help wide expense-split">
        {reverseChargeHint(split.net, smallBusiness)}
      </p>
    {:else if split}<p class="form-help wide expense-split">
        {#if smallBusiness}Als Kleinunternehmer zählt der volle Betrag von {amountLabel(
            split.net + split.tax,
          )}.
        {:else}Netto {amountLabel(split.net)} · Vorsteuer {amountLabel(split.tax)} – die App rechnet die
          Vorsteuer in die Umsatzsteuer-Voranmeldung ein.{/if}
      </p>{/if}

    {#if category === "asset"}
      <label class="wide"
        >Wie lange nutzt du es?<select bind:value={years} required>
          <option value={0} disabled>Auswählen …</option>
          {#each usefulLives as l (l.years)}<option value={l.years}>{l.label}</option>{/each}
          {#if years && !usefulLives.some((l) => l.years === years)}<option value={years}
              >{years} Jahre</option
            >{/if}
        </select></label
      >
      {#if split}<p class="form-help wide">
          {assetTreatment(smallBusiness ? split.net + split.tax : split.net, years)}
        </p>{/if}
    {/if}

    <div class="wide expense-currency">
      <SettingSection
        title="Beleg in fremder Währung"
        summary={currency
          ? `${originalGross || "Betrag fehlt"} ${currency.toUpperCase()}`
          : "Nein, der Beleg ist in Euro"}
        open={!!currency}
      >
        <div class="expense-currency-fields">
          <label
            >Währung<input
              bind:value={currency}
              maxlength="3"
              placeholder="z. B. USD"
              autocapitalize="characters"
            /></label
          >
          <label
            >Betrag in dieser Währung<input
              inputmode="decimal"
              bind:value={originalGross}
              required={!!currency}
              placeholder="0,00"
            /></label
          >
        </div>
        <p class="form-help">
          Oben steht der Betrag in Euro, den du gezahlt hast. Den Kurs rechnet die App daraus.
        </p>
      </SettingSection>
    </div>

    <fieldset class="wide expense-paid" disabled={linked}>
      <legend>Wie hast du bezahlt?</legend>
      {#each paidOptions as o (o.id)}<label class="inline-choice"
          ><input type="radio" bind:group={paid} value={o.id} /> {o.label}</label
        >{/each}
    </fieldset>
    {#if paid !== "open"}<label
        >Bezahlt am<input type="date" bind:value={paidDate} required /></label
      >{/if}
    {#if linked}<p class="form-help wide">Betrag und Zahlung kommen aus dem Bankumsatz.</p>{/if}
    {#if taxed}<label class="wide inline-choice"
        ><input type="checkbox" bind:checked={advance} /> Anzahlung: Die Leistung folgt noch</label
      >
      {#if advance}<p class="form-help wide">
          Die Steuer zählt in der Regel erst mit der Zahlung. Die App rechnet sie deshalb zum
          Zahlungsdatum ein.
        </p>{/if}{/if}
    <label class="wide">Notiz (optional)<textarea rows="2" bind:value={note}></textarea></label>
  </fieldset>
  <div class="form-grid expense-form expense-file">
    {#if !cancelled && !(locked && start.fileSum)}<label class="wide"
        >Foto oder PDF des Belegs (optional)<FilePicker
          accept=".pdf,.jpg,.jpeg,.png,.heic,application/pdf,image/*"
          disabled={busy}
          filename={file?.name ?? ""}
          onpick={(picked) => (file = picked)}
        /></label
      >{/if}
    {#if start.attachmentName && !file}<p class="form-help wide">
        Gespeichert: {start.attachmentName}. Eine ersetzte Datei bleibt im Archiv erhalten.
      </p>{/if}
  </div>
  {#snippet footer()}
    {#if ondelete}<button type="button" class="danger-text" disabled={busy} onclick={ondelete}
        ><Icon name="trash" size={16} /> Stornieren</button
      >{/if}
    {#if onopenfile}<button type="button" class="secondary" disabled={busy} onclick={onopenfile}
        >Datei öffnen</button
      >{/if}
    <span class="footer-spacer"></span>
    <button type="button" class="secondary" disabled={busy} onclick={onclose}
      >{canSave ? "Abbrechen" : "Schließen"}</button
    >
    {#if canSave}<button class="primary" disabled={busy}
        >{busy ? "Wird gespeichert …" : "Speichern"}</button
      >{/if}
  {/snippet}
</FormDialog>

<style>
  .expense-form {
    grid-template-columns: repeat(2, minmax(0, 1fr));
    border: 0;
    margin: 0;
    padding: 0;
    min-width: 0;
  }
  .expense-file {
    margin-top: 16px;
  }
  .expense-state {
    margin: 0 0 14px;
    padding: 10px 12px;
    border-radius: 8px;
    border: 1px solid var(--theme-info-border);
    background: var(--theme-info-bg);
    color: var(--theme-info-text);
    line-height: 1.45;
    overflow-wrap: anywhere;
  }
  .expense-currency-fields {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 12px;
    margin-top: 10px;
  }
  .wide {
    grid-column: 1 / -1;
  }
  .expense-paid {
    min-width: 0;
    border: 0;
    padding: 0;
    margin: 0;
    display: flex;
    flex-wrap: wrap;
    gap: 6px 18px;
  }
  .expense-paid legend {
    font: inherit;
    font-weight: 600;
    margin-bottom: 6px;
  }
  .expense-form :global(.form-help) {
    margin-top: -8px;
    overflow-wrap: anywhere;
  }
  .inline-choice {
    flex-direction: row;
    align-items: center;
    gap: 6px;
    font-weight: 400;
  }
  .expense-split {
    margin-top: -4px;
  }
  textarea {
    font: inherit;
    resize: vertical;
  }
  @media (max-width: 640px) {
    .expense-form {
      grid-template-columns: 1fr;
    }
  }
</style>
