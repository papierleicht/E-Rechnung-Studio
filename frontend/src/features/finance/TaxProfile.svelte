<script lang="ts">
  import * as API from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/app/invoiceservice";
  import type { TaxSetup } from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/app/models";
  import { serviceErrorMessage } from "../../lib/errors";
  import { frequencyLabels } from "../../lib/expenses";

  let { setup, onsaved }: { setup: TaxSetup; onsaved: () => Promise<void> } = $props();

  // A regular business has to choose how often it files before anything is due;
  // every business says whether it runs a trade, which decides the forms.
  const missing = $derived((!setup.smallBusiness && !setup.vatPeriod) || !setup.activity);
  let editing = $state(false);
  let filing = $state("self");
  let activity = $state("");
  let taxBasis = $state("soll");
  let vatPeriod = $state("quarterly");
  let vatExtension = $state(false);
  let specialPrepayment = $state("");
  let busy = $state(false);
  let error = $state("");

  function edit() {
    filing = setup.filing || "self";
    activity = setup.activity || "";
    taxBasis = setup.taxBasis || "soll";
    vatPeriod = setup.vatPeriod || "quarterly";
    vatExtension = setup.vatExtension;
    specialPrepayment = setup.specialPrepayment.replace(".", ",");
    error = "";
    editing = true;
  }
  $effect(() => {
    if (missing && !editing) edit();
  });

  async function save() {
    error = "";
    if (!activity) {
      error = "Wähle, ob du ein Gewerbe hast oder freiberuflich arbeitest.";
      return;
    }
    busy = true;
    try {
      const settings = await API.FinanceSettings();
      await API.SaveFinanceSettings({
        ...settings,
        filing,
        activity,
        taxBasis,
        vatPeriod,
        vatExtension: vatPeriod !== "yearly" && vatExtension,
        specialPrepayment,
      });
      // Reload first: closing while the old profile still counts as missing
      // would open the form again with the old answers.
      await onsaved();
      editing = false;
    } catch (cause) {
      error = serviceErrorMessage(cause, "Das Steuerprofil konnte nicht gespeichert werden.");
    } finally {
      busy = false;
    }
  }

  const activityLabels: Record<string, string> = { trade: "Gewerbe", freelance: "freiberuflich" };
  const summary = $derived(
    [
      activityLabels[setup.activity] ?? "Tätigkeit nicht festgelegt",
      setup.smallBusiness ? "Kleinunternehmer (§ 19 UStG)" : "Regelbesteuerung",
      setup.smallBusiness
        ? "Voranmeldung nur bei Steuerschuld als Empfänger"
        : setup.taxBasis === "ist"
          ? "Ist-Versteuerung"
          : "Soll-Versteuerung",
      !setup.smallBusiness && setup.vatPeriod
        ? `Voranmeldung ${frequencyLabels[setup.vatPeriod]}${setup.vatExtension ? " mit Dauerfristverlängerung" : ""}`
        : "",
      setup.filing === "advisor" ? "Steuerkanzlei übermittelt" : "du übermittelst selbst",
    ]
      .filter(Boolean)
      .join(" · "),
  );
</script>

{#if !editing}
  <p class="tax-profile">
    <span class="tax-muted">Steuerprofil:</span>
    {summary} · <button class="text" onclick={edit}>Ändern</button>
  </p>
{:else}
  <section class="tax-card tax-profile-form" aria-labelledby="tax-profile-title">
    <h2 id="tax-profile-title">{missing ? "Richte dein Steuerprofil ein" : "Steuerprofil"}</h2>
    <p class="tax-muted">
      Einmal festlegen, dann richten sich Fristen, Formulare und Hinweise danach. Die Angaben stehen
      im Schreiben deines Finanzamts.
    </p>
    {#if error}<div class="alert error" role="alert">{error}</div>{/if}
    <fieldset>
      <legend>Wer übermittelt deine Steuererklärungen?</legend>
      <label
        ><input type="radio" bind:group={filing} value="self" /> Ich selbst über Mein ELSTER – die App
        zeigt mir die Werte</label
      >
      <label
        ><input type="radio" bind:group={filing} value="advisor" /> Meine Steuerkanzlei – ich gebe ihr
        die Daten als DATEV-Export</label
      >
    </fieldset>
    <fieldset>
      <legend>Was für eine Tätigkeit hast du?</legend>
      <p class="tax-muted">
        Danach richten sich die Anlage zur Einkommensteuer und die Gewerbesteuer. Freiberuflich sind
        die Berufe nach § 18 EStG, etwa Ärzte, Architektinnen, Journalisten, Übersetzerinnen oder
        Künstler; sie melden kein Gewerbe an. Was für dich gilt, steht im Schreiben des Finanzamts
        zur steuerlichen Erfassung.
      </p>
      <label
        ><input type="radio" bind:group={activity} value="trade" /> Gewerbe – mit Gewerbeanmeldung, Anlage
        G, Gewerbesteuer auf den Gewerbeertrag über 24.500 € im Jahr</label
      >
      <label
        ><input type="radio" bind:group={activity} value="freelance" /> Freiberuflich – ohne Gewerbeanmeldung,
        Anlage S, keine Gewerbesteuer</label
      >
    </fieldset>
    {#if setup.smallBusiness}
      <p class="tax-muted">
        Du nutzt die Kleinunternehmerregelung. Das änderst du in den Unternehmensdaten unter
        Einstellungen.
      </p>
    {:else}
      <fieldset>
        <legend>Wann entsteht deine Umsatzsteuer?</legend>
        <label
          ><input type="radio" bind:group={taxBasis} value="soll" /> Mit der Leistung (Soll-Versteuerung,
          der Normalfall)</label
        >
        <label
          ><input type="radio" bind:group={taxBasis} value="ist" /> Mit der Zahlung (Ist-Versteuerung,
          nur mit Genehmigung des Finanzamts nach § 20 UStG)</label
        >
      </fieldset>
    {/if}
    <fieldset>
      <legend>Wie oft gibst du die Umsatzsteuer-Voranmeldung ab?</legend>
      <p class="tax-muted">
        {setup.smallBusiness
          ? "Nur für Zeiträume, in denen du als Empfänger Steuer schuldest – meist für das Kalendervierteljahr, sonst wie vom Finanzamt festgelegt."
          : "In der Regel vierteljährlich, monatlich bei mehr als 9.000 € Steuer im Vorjahr. Bei Gründung zählt die voraussichtliche Steuer des ersten Jahres; die frühere Pflicht zur monatlichen Abgabe ist bis Ende 2026 ausgesetzt."}
      </p>
      {#each setup.smallBusiness ? ["monthly", "quarterly"] : ["monthly", "quarterly", "yearly"] as value}<label
          ><input type="radio" bind:group={vatPeriod} {value} />
          {value === "yearly"
            ? "Gar nicht – nur die Jahreserklärung"
            : frequencyLabels[value]}</label
        >{/each}
      {#if vatPeriod !== "yearly"}<label
          ><input type="checkbox" bind:checked={vatExtension} /> Dauerfristverlängerung (Abgabe einen
          Monat später)</label
        >{/if}
      {#if vatExtension && vatPeriod === "monthly"}<label class="stacked"
          >Festgesetzte Sondervorauszahlung (wird im Dezember abgezogen)<input
            inputmode="decimal"
            bind:value={specialPrepayment}
            placeholder="0,00"
          /></label
        >{/if}
    </fieldset>
    <div class="tax-actions">
      <button class="primary" disabled={busy} onclick={save}>Speichern</button>
      {#if !missing}<button class="secondary" disabled={busy} onclick={() => (editing = false)}
          >Abbrechen</button
        >{/if}
    </div>
  </section>
{/if}

<style>
  .tax-profile {
    margin: 0;
    font-size: 13px;
  }
  .tax-profile-form {
    display: grid;
    gap: 12px;
  }
  fieldset {
    display: grid;
    gap: 6px;
    margin: 0;
    padding: 0;
    border: 0;
  }
  legend {
    font-weight: 600;
    margin-bottom: 4px;
  }
  fieldset label {
    display: flex;
    align-items: flex-start;
    gap: 6px;
  }
  fieldset label.stacked {
    flex-direction: column;
    max-width: 280px;
  }
  fieldset p {
    margin: 0;
  }
</style>
