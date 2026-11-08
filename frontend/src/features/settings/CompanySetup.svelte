<script lang="ts">
  import { tick } from "svelte";
  import Icon from "../../components/Icon.svelte";
  import ElectronicAddress from "../../components/ElectronicAddress.svelte";
  import { taxLabel, paymentChoices } from "../../lib/tax";
  import { chooseSetupTax, setupMissing } from "../../lib/onboarding";
  import { setupTaxGuidance, setupPaymentGuidance } from "../../lib/setupGuidance";
  import { ValidateCompanyStep } from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/app/invoiceservice";
  import { Status as AccountStatus } from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/app/accountservice";
  import { serviceErrorMessage } from "../../lib/errors";
  import BankAccountsEditor from "../../components/BankAccountsEditor.svelte";
  import MailSettings from "./MailSettings.svelte";
  import NewsletterSignup from "../../components/NewsletterSignup.svelte";
  import SetupPlusOffer from "./SetupPlusOffer.svelte";
  import { Browser } from "@wailsio/runtime";
  import StoreIcon from "@lucide/svelte/icons/store";
  import LifeBuoyIcon from "@lucide/svelte/icons/life-buoy";
  import CheckIcon from "@lucide/svelte/icons/check";
  import { shopProviders, shopProviderByID } from "../../lib/shopProviders";
  import {
    setupPlanSummary,
    setupShopSummary,
    setupSupportOffer,
    type SetupPlusTier,
  } from "../../lib/setupOffers";
  import type { Company } from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/model/models";
  import { frequencyLabels } from "../../lib/expenses";
  import type { SetupTaxProfile } from "../../lib/onboarding";
  let {
    company,
    busy,
    error = "",
    oncomplete,
    onrestore,
  }: {
    company: Company;
    busy: boolean;
    error?: string;
    /** shop is the provider to connect right after the start, if any;
        taxProfile goes into the tax profile under "Steuern". */
    oncomplete: (choices: { shop: string; taxProfile: SetupTaxProfile }) => void;
    onrestore: () => void;
  } = $props();
  const steps = [
    "Unternehmen",
    "Kontakt",
    "Steuern",
    "Steuerprofil",
    "Zahlung",
    "E-Mail",
    "Shop",
    "Plus",
    "Newsletter",
    "Prüfen",
  ];
  const taxStep = 2;
  const profileStep = 3;
  const paymentStep = 4;
  const mailStep = 5;
  const shopStep = 6;
  const plusStep = 7;
  const newsStep = 8;
  const last = steps.length - 1;
  let mailSaved = $state(false);
  // "" while unanswered, "none" without a shop, otherwise the provider ID.
  let shop = $state("");
  let plusBooked = $state<"" | SetupPlusTier>("");
  let newsletterEmail = $state("");
  let step = $state(0);
  let furthest = $state(0);
  let taxConfirmed = $state(false);
  // How the business files its taxes; the same answers as the tax profile.
  let taxProfile = $state<SetupTaxProfile>({
    activity: "",
    filing: "self",
    vatPeriod: "quarterly",
    taxBasis: "soll",
    vatExtension: false,
  });
  let taxHelp = $state(false);
  let localError = $state("");
  let checking = $state(false);
  let errorElement: HTMLParagraphElement | undefined = $state();
  const locked = $derived(busy || checking);
  let heading: HTMLHeadingElement;
  const tax = $derived(
    company.taxStatusUnclear ? "unknown" : company.smallBusiness ? "small-business" : "standard",
  );
  const payment = $derived(company.paymentMeansCode || "58");
  const transfer = $derived(["58", "30"].includes(payment));
  const missing = $derived(setupMissing(company));
  async function openSupport() {
    let manage = "https://papierleicht.de/konto";
    try {
      manage = (await AccountStatus())?.manageUrl || manage;
    } catch {
      // The default page works as well.
    }
    try {
      await Browser.OpenURL(`${manage}?tarif=support`);
    } catch {
      localError = `Die Website ließ sich nicht öffnen. Du findest den Support unter ${manage}?tarif=support.`;
    }
  }
  async function go(next: number) {
    step = next;
    localError = "";
    await tick();
    heading.focus({ preventScroll: true });
    window.scrollTo({ top: 0, behavior: "instant" });
  }
  async function submit(next = step + 1) {
    if (locked) return;
    localError = "";
    if (step === profileStep && !taxProfile.activity) {
      localError = "Wähle, ob du ein Gewerbe hast oder freiberuflich arbeitest.";
      return;
    }
    if (step === taxStep) {
      if (!taxConfirmed) {
        localError =
          "Gib bitte an, ob du die Kleinunternehmerregelung nutzt. Du kannst auch „Noch unklar“ wählen.";
        return;
      }
    }
    if (step === taxStep || step === paymentStep || step === last) {
      checking = true;
      try {
        const stages = step === last ? [taxStep, paymentStep] : [step];
        for (const stage of stages) {
          const issues = await ValidateCompanyStep(
            JSON.parse(JSON.stringify(company)),
            stage === taxStep ? "tax" : "payment",
          );
          if (issues?.length) {
            await go(stage);
            localError = issues.map((issue) => issue.message).join(" ");
            await tick();
            errorElement?.focus();
            errorElement?.scrollIntoView({ block: "nearest", behavior: "smooth" });
            return;
          }
        }
      } catch (e) {
        localError = serviceErrorMessage(
          e,
          "Wir konnten deine Angaben nicht prüfen. Versuche es noch einmal.",
        );
        await tick();
        errorElement?.focus();
        return;
      } finally {
        checking = false;
      }
    }
    // Who opened the booking page gets the newsletter offer there.
    if (step === plusStep && plusBooked && next === newsStep) next = last;
    if (step < last) {
      furthest = Math.max(furthest, next);
      void go(next);
      return;
    }
    const addressIssue = missing.find((item) => item.step === 0);
    if (addressIssue) {
      void go(0);
      return;
    }
    if (!taxConfirmed) {
      void go(taxStep);
      return;
    }
    if (!taxProfile.activity) {
      void go(profileStep);
      return;
    }
    company.paymentMeansCode = payment;
    oncomplete({ shop: shop === "none" ? "" : shop, taxProfile: { ...taxProfile } });
  }
</script>

<main class="welcome-page setup-wizard">
  <div class="welcome-brand"><span>ERS</span> E-Rechnung-Studio</div>
  <nav aria-label="Einrichtungsschritte">
    <ol class="wizard-progress">
      {#each steps as label, index}
        <li>
          <button
            type="button"
            aria-current={step === index ? "step" : undefined}
            disabled={locked || index > furthest}
            onclick={() => (index > step ? submit(index) : go(index))}
          >
            <span>{index + 1}</span>{label}</button
          >
        </li>
      {/each}
    </ol>
  </nav>
  <form
    class="setup-form"
    oninput={() => (localError = "")}
    onsubmit={(event) => {
      event.preventDefault();
      submit();
    }}
  >
    <fieldset disabled={locked}>
      <h1 bind:this={heading} tabindex="-1">
        {[
          "Wie heißt dein Unternehmen?",
          "Wie können Kunden dich erreichen?",
          "Wie rechnest du normalerweise ab?",
          "Wie meldest du deine Steuern?",
          "Wie sollen deine Kunden bezahlen?",
          "Wie verschickst du Rechnungen per E-Mail?",
          "Verkaufst du über einen Online-Shop?",
          "Möchtest du mehr automatisieren?",
          "Neuigkeiten und Hilfe",
          "Stimmt alles?",
        ][step]}
      </h1>
      <p class="heading-subtitle">Schritt {step + 1} von {steps.length}</p>
      {#if step === 0}
        <details class="wizard-help link-toggle">
          <summary>Welchen Namen trage ich ein?</summary>
          <p>
            Bei einem Einzelunternehmen nimmst du deinen Vor- und Nachnamen, gegebenenfalls mit
            Geschäftsbezeichnung. Bei einer Gesellschaft nimmst du die vollständige
            Unternehmensbezeichnung einschließlich Rechtsform, zum Beispiel „Muster GmbH“.
          </p>
          <p>Die Anschrift ist die Adresse deines Unternehmens – nicht die deines Kunden.</p>
        </details>
        <p>
          Diese Angaben stehen als Absender auf deinen Rechnungen. Trage deinen vollständigen Namen
          bzw. die offizielle Unternehmensbezeichnung ein.
        </p>
        <div class="form-grid cols-2">
          <label class="full-width"
            >Unternehmensname / vollständiger Name *<input
              required
              bind:value={company.name}
              autocomplete="organization"
            /></label
          >
          <label class="full-width"
            >Straße und Hausnummer *<input
              required
              bind:value={company.address.street}
              autocomplete="address-line1"
            /></label
          >
          <label class="full-width"
            >Adresszusatz (optional)<input
              bind:value={company.address.additional}
              autocomplete="address-line2"
            /></label
          >
          <label
            >Postleitzahl *<input
              required
              bind:value={company.address.postcode}
              autocomplete="postal-code"
            /></label
          >
          <label
            >Ort *<input
              required
              bind:value={company.address.city}
              autocomplete="address-level2"
            /></label
          >
          <label
            >Land (zweistellig) *<input
              required
              pattern={"[A-Za-z]{2}"}
              maxlength="2"
              bind:value={company.address.country}
              placeholder="DE"
            /></label
          >
        </div>
      {:else if step === 1}
        <p>
          Eine Kontaktadresse hilft bei Rückfragen. Für strukturierte E-Rechnungen kannst du
          zusätzlich eine elektronische Absenderadresse hinterlegen.
        </p>
        <div class="form-grid cols-2">
          <label
            >Ansprechpartner (optional)<input
              bind:value={company.contactName}
              autocomplete="name"
            /></label
          >
          <label
            >Telefon (optional)<input
              type="tel"
              bind:value={company.phone}
              autocomplete="tel"
            /></label
          >
          <label class="full-width"
            >Kontakt-E-Mail<input
              type="email"
              bind:value={company.email}
              autocomplete="email"
              placeholder="rechnung@unternehmen.de"
            /></label
          >
        </div>
        <section class="optional-setup" aria-labelledby="setup-electronic-address-title">
          <h3 id="setup-electronic-address-title">Elektronische Absenderadresse</h3>
          <p>
            Die Kennung identifiziert dich in den Rechnungsdaten. Standardmäßig übernehmen wir deine
            Kontakt-E-Mail. Dadurch wird die Adresse nicht automatisch zum Versanddienst.
          </p>
          <ElectronicAddress
            bind:endpoint={company.endpoint}
            bind:scheme={company.endpointScheme}
            email={company.email}
            field="company"
            followEmail
          />
        </section>
      {:else if step === taxStep}
        <p>
          Nutzt du die Kleinunternehmerregelung (§ 19)? Die Frage betrifft deinen
          Unternehmensstatus, nicht die steuerliche Behandlung jeder einzelnen Leistung.
        </p>
        <button
          type="button"
          class="text"
          aria-expanded={taxHelp}
          onclick={() => (taxHelp = !taxHelp)}
          >Ich bin unsicher – hilf mir bei der Einordnung</button
        >
        {#if taxHelp}<aside class="wizard-note">
            <h2>Schau zuerst in deine Unterlagen</h2>
            <ol>
              <li>
                Hast du die Kleinunternehmerregelung mit deiner Steuerberatung oder bei der
                steuerlichen Erfassung festgelegt? Dann passt die Antwort mit § 19.
              </li>
              <li>
                Nutzt du die Kleinunternehmerregelung nicht? Dann wähle „Nein“. Das heißt nicht,
                dass jede deiner Leistungen umsatzsteuerpflichtig ist.
              </li>
              <li>
                Andere Steuerbefreiungen und Sonderregelungen legst du auf der Rechnung fest.
                Wiederkehrende Fälle kannst du später unter „Rechnungsvorgaben“ vorbelegen.
              </li>
            </ol>
            <p>
              „Kleingewerbe“, „nebenberuflich“ oder „freiberuflich“ beantwortet die
              Umsatzsteuerfrage allein nicht. Auch eine vorhandene USt-IdNr. entscheidet die Frage
              nicht allein. Wenn die Einordnung offen ist, kläre sie vor der Auswahl.
            </p>
          </aside>{/if}
        <div class="wizard-choices">
          {#each [{ id: "standard", title: "Nein, ich nutze die Kleinunternehmerregelung nicht", hint: "Steuersätze und mögliche Befreiungen prüfst du je Leistung." }, { id: "small-business", title: "Ja, ich nutze die Kleinunternehmerregelung (§ 19)", hint: "Neue Rechnungen starten mit der Kleinunternehmer-Behandlung." }, { id: "unknown", title: "Noch unklar", hint: "Du richtest die App weiter ein und speicherst Entwürfe. Kläre die Frage, bevor du eine Rechnung ausstellst." }] as choice}
            <label class:selected={taxConfirmed && tax === choice.id}>
              <input
                type="radio"
                name="tax"
                checked={taxConfirmed && tax === choice.id}
                onchange={() => {
                  chooseSetupTax(company, choice.id);
                  taxConfirmed = true;
                }}
              />
              <span><strong>{choice.title}</strong><small>{choice.hint}</small></span>
            </label>
          {/each}
        </div>
        <p class="form-help">
          Optionale Sonderbehandlungen findest du nach der Einrichtung unter Einstellungen →
          Rechnungsvorgaben. Sie ändern deinen Unternehmensstatus nicht.
        </p>
        {#if taxConfirmed && tax === "standard"}<aside class="wizard-note" aria-live="polite">
            <h2>Keine Kleinunternehmerregelung</h2>
            <p>
              Standard ist die reguläre Umsatzsteuer-Behandlung. Einzelne Leistungen können trotzdem
              steuerfrei sein oder einer Sonderregelung unterliegen. Das legst du auf der Rechnung
              fest.
            </p>
          </aside>
        {:else if taxConfirmed && setupTaxGuidance[tax]}<aside
            class="wizard-note"
            aria-live="polite"
          >
            <h2>{taxLabel(tax)} – das bedeutet deine Auswahl</h2>
            <p>{setupTaxGuidance[tax].meaning}</p>
            <p><strong>Zur Einordnung:</strong> {setupTaxGuidance[tax].example}</p>
            <p>
              <strong>So richtet die App dein Profil ein:</strong>
              {setupTaxGuidance[tax].effect}
            </p>
          </aside>{/if}
        <p class="form-help">
          Unsicher? Kläre den Steuerfall mit deiner Steuerberatung. Die Einrichtung ersetzt keine
          steuerliche Beratung.
        </p>
        <details class="wizard-help link-toggle">
          <summary>Steuernummer oder USt-IdNr. – wo finde ich das?</summary>
          <p>
            Die betriebliche Steuernummer findest du auf Schreiben deines Finanzamts. Die USt-IdNr.
            ist eine gesonderte Kennung, in Deutschland beginnt sie mit „DE“. Deine persönliche
            elfstellige Steuer-ID ist nicht die gesuchte betriebliche Steuernummer.
          </p>
          <p>
            Wenn du beide Kennungen hast, kannst du beide hinterlegen. Welche Angaben eine Rechnung
            braucht, hängt vom Steuerfall ab.
          </p>
        </details>
        <div class="form-grid cols-2">
          <label
            >Steuernummer<input
              bind:value={company.taxNumber}
              placeholder="Vom Finanzamt vergeben"
            /></label
          >
          <label
            >USt-IdNr.<input bind:value={company.vatId} placeholder="z. B. DE123456789" /></label
          >
        </div>
        {#if !company.taxNumber && !company.vatId}<p class="wizard-note">
            Die steuerliche Kennung kannst du später in den Einstellungen ergänzen.
          </p>{/if}
      {:else if step === profileStep}
        <p>
          Danach richtet die App Fristen, Formulare und Hinweise unter „Steuern“ aus. Die Angaben
          stehen im Schreiben deines Finanzamts zur steuerlichen Erfassung. Du kannst sie später
          unter „Steuern“ ändern.
        </p>
        <h2 class="wizard-question">Was für eine Tätigkeit hast du?</h2>
        <div class="wizard-choices">
          {#each [{ id: "trade", title: "Gewerbe", hint: "Mit Gewerbeanmeldung, etwa Handel, Handwerk, Online-Shop oder Agentur. Gewinn in der Anlage G, Gewerbesteuer auf den Gewerbeertrag über 24.500 € im Jahr." }, { id: "freelance", title: "Freiberuflich", hint: "Berufe nach § 18 EStG ohne Gewerbeanmeldung, etwa Ärzte, Architektinnen, Journalisten, Übersetzerinnen oder Künstler. Gewinn in der Anlage S, keine Gewerbesteuer." }] as choice}
            <label class:selected={taxProfile.activity === choice.id}>
              <input
                type="radio"
                name="activity"
                value={choice.id}
                bind:group={taxProfile.activity}
              />
              <span><strong>{choice.title}</strong><small>{choice.hint}</small></span>
            </label>
          {/each}
        </div>
        <h2 class="wizard-question">Wer übermittelt deine Steuererklärungen?</h2>
        <div class="wizard-choices">
          {#each [{ id: "self", title: "Ich selbst über Mein ELSTER", hint: "Die App zeigt dir die Werte Zeile für Zeile zum Übertragen." }, { id: "advisor", title: "Meine Steuerkanzlei", hint: "Du gibst ihr die Daten als DATEV-Export; die App erinnert an die Fristen." }] as choice}
            <label class:selected={taxProfile.filing === choice.id}>
              <input type="radio" name="filing" value={choice.id} bind:group={taxProfile.filing} />
              <span><strong>{choice.title}</strong><small>{choice.hint}</small></span>
            </label>
          {/each}
        </div>
        {#if !company.smallBusiness}
          <h2 class="wizard-question">Wie oft gibst du die Umsatzsteuer-Voranmeldung ab?</h2>
          <p class="form-help">
            In der Regel vierteljährlich, monatlich bei mehr als 9.000 € Umsatzsteuer im Vorjahr.
            Bei Gründung zählt die voraussichtliche Steuer des ersten Jahres. Was gilt, steht im
            Schreiben des Finanzamts.
          </p>
          <div class="wizard-options" role="radiogroup" aria-label="Rhythmus der Voranmeldung">
            {#each ["monthly", "quarterly", "yearly"] as value}<label
                ><input type="radio" name="vatPeriod" {value} bind:group={taxProfile.vatPeriod} />
                {value === "yearly"
                  ? "Gar nicht – nur die Jahreserklärung"
                  : frequencyLabels[value]}</label
              >{/each}
            {#if taxProfile.vatPeriod !== "yearly"}<label
                ><input type="checkbox" bind:checked={taxProfile.vatExtension} /> Mit Dauerfristverlängerung
                (Abgabe einen Monat später)</label
              >{/if}
          </div>
          <h2 class="wizard-question">Wann entsteht deine Umsatzsteuer?</h2>
          <div class="wizard-options" role="radiogroup" aria-label="Versteuerung">
            <label
              ><input type="radio" name="taxBasis" value="soll" bind:group={taxProfile.taxBasis} />
              Mit der Leistung (Soll-Versteuerung, der Normalfall)</label
            >
            <label
              ><input type="radio" name="taxBasis" value="ist" bind:group={taxProfile.taxBasis} />
              Mit der Zahlung (Ist-Versteuerung, nur mit Genehmigung des Finanzamts)</label
            >
          </div>
        {/if}
      {:else if step === paymentStep}
        <p>
          Diese Zahlungsart übernehmen wir für neue Rechnungen. Bei Bedarf änderst du sie pro
          Rechnung.
        </p>
        <div class="wizard-choices">
          {#each paymentChoices as choice}
            <label class:selected={payment === choice.id}>
              <input
                type="radio"
                name="payment"
                checked={payment === choice.id}
                onchange={() => (company.paymentMeansCode = choice.id)}
              />
              <span
                ><strong>{choice.label}</strong><small>{setupPaymentGuidance[choice.id]}</small
                ></span
              >
            </label>
          {/each}
        </div>
        {#if transfer || company.iban || company.bic || company.additionalAccounts?.length}
          {#if !transfer}<p class="form-help">
              Deine bereits eingegebenen Bankdaten bleiben erhalten. Du kannst sie hier korrigieren
              oder leeren; für die gewählte Zahlungsart brauchst du sie nicht.
            </p>{/if}
          <BankAccountsEditor bind:company field="setup" />
          <p class="form-help">IBAN mit oder ohne Leerzeichen; Bank und BIC werden ergänzt.</p>
          {#if !company.iban}<p class="wizard-note">
              Die Bankverbindung kannst du später hinzufügen.
            </p>{/if}
        {:else}<p class="wizard-note">
            {payment === "10"
              ? "Für Barzahlung brauchst du keine Bankverbindung."
              : "Die konkreten Zahlungsbedingungen vereinbarst du mit deinem Kunden."}
          </p>{/if}
        <p class="form-help">
          Das Zahlungsziel legst du beim Kunden oder auf der Rechnung fest. Neue Kunden starten
          derzeit mit 14 Tagen.
        </p>
      {:else if step === mailStep}
        <p>
          Optional: Verbinde dein E-Mail-Konto, um Rechnungen, Angebote und Erinnerungen direkt aus
          der App zu verschicken. Das Passwort liegt nur im Schlüsselbund dieses Geräts.
        </p>
        <MailSettings compact {busy} onsaved={() => (mailSaved = true)} />
        <p class="wizard-note">
          {mailSaved
            ? "E-Mail-Konto gespeichert."
            : "Du kannst diesen Schritt überspringen und das E-Mail-Konto später unter Einstellungen → E-Mail einrichten."}
        </p>
      {:else if step === shopStep}
        <p>
          Kostenlos in jeder Version: Die App holt neue Bestellungen aus deinem Shop und macht
          daraus fertige Rechnungsentwürfe, mit Kunde, Positionen und bezahltem Betrag. Du prüfst
          und stellst sie nur noch aus.
        </p>
        <div class="wizard-choices shop-choices">
          {#each shopProviders as provider}
            <label class:selected={shop === provider.id}>
              <input
                type="radio"
                name="shop"
                checked={shop === provider.id}
                onchange={() => (shop = provider.id)}
              />
              <span><strong>{provider.label}</strong></span>
            </label>
          {/each}
          <label class="shop-none" class:selected={shop === "none"}>
            <input
              type="radio"
              name="shop"
              checked={shop === "none"}
              onchange={() => (shop = "none")}
            />
            <span
              ><strong>Nein, ich habe keinen Online-Shop</strong><small
                >Den Shop-Import findest du jederzeit unter Shop-Bestellungen.</small
              ></span
            >
          </label>
        </div>
        {#if shop && shop !== "none"}<aside class="wizard-note shop-note" aria-live="polite">
            <StoreIcon size={20} aria-hidden="true" />
            <p>
              Direkt nach dem Start öffnen wir die Verbindung zu {shopProviderByID(shop)?.label}.
              Halte die Zugangsdaten mit Lesezugriff auf Bestellungen bereit; die App führt dich
              durch die Einrichtung.
            </p>
          </aside>{/if}
      {:else if step === plusStep}
        <p>
          E-Rechnung-Studio ist kostenlos und bleibt es. Mit Plus nimmt dir die App zusätzlich
          Arbeit ab und schützt dein Änderungsprotokoll von außen. Du kannst diesen Schritt
          überspringen.
        </p>
        <SetupPlusOffer bind:booked={plusBooked} />
      {:else if step === newsStep}
        <p>
          Zum Schluss zwei freiwillige Angebote von papierleicht. Du kannst beide überspringen; den
          Newsletter findest du auch später unter Einstellungen → Allgemein.
        </p>
        <NewsletterSignup
          email={company.email}
          source="app-onboarding"
          onrequested={(note) => (newsletterEmail = note.email)}
        />
        {#if !plusBooked}<section class="support-offer" aria-labelledby="setup-support-title">
            <span class="support-icon" aria-hidden="true"><LifeBuoyIcon size={20} /></span>
            <div>
              <p class="support-eyebrow">Support für die Community-Version</p>
              <h2 id="setup-support-title">{setupSupportOffer.name}</h2>
              <p>{setupSupportOffer.lead}</p>
              <ul>
                {#each setupSupportOffer.items as item}<li>
                    <CheckIcon size={15} aria-hidden="true" /><span>{item}</span>
                  </li>{/each}
              </ul>
              <button type="button" class="secondary" onclick={openSupport}
                >Community Support ansehen</button
              >
              <p class="form-help">
                Gebucht wird auf papierleicht.de. Steuerliche und rechtliche Fragen beantwortet der
                Support nicht.
              </p>
            </div>
          </section>{/if}
      {:else}
        <p>
          Kontrolliere deine Voreinstellungen. Erst mit „Profil speichern“ werden sie dauerhaft
          übernommen.
        </p>
        <div class="wizard-review">
          <section>
            <h2>
              Unternehmen <button type="button" class="text" onclick={() => go(0)}>Ändern</button>
            </h2>
            <p>
              {company.name}<br />{company.address.street}<br />{company.address.additional}<br
              />{company.address.postcode}
              {company.address.city} · {company.address.country.toUpperCase()}
            </p>
          </section>
          <section>
            <h2>
              Kontakt <button type="button" class="text" onclick={() => go(1)}>Ändern</button>
            </h2>
            <p>
              {company.contactName || "Kein Ansprechpartner"}<br />{company.email ||
                "Keine Kontakt-E-Mail"}<br />{company.phone || "Keine Telefonnummer"}
            </p>
            <p>
              Elektronische Adresse: {company.endpoint || "Nicht hinterlegt"}
              {company.endpoint ? "(" + company.endpointScheme + ")" : ""}
            </p>
          </section>
          <section>
            <h2>
              Steuern <button type="button" class="text" onclick={() => go(taxStep)}>Ändern</button>
            </h2>
            <p>
              Kleinunternehmerregelung: {tax === "unknown"
                ? "Noch unklar – vor dem Ausstellen klären"
                : company.smallBusiness
                  ? "Ja"
                  : "Nein"}<br />Steuernummer: {company.taxNumber || "Noch offen"}<br />USt-IdNr.: {company.vatId ||
                "Nicht hinterlegt"}
            </p>
            {#if company.exemptionReason}<p>{company.exemptionReason}</p>{/if}
          </section>
          <section>
            <h2>
              Steuerprofil <button type="button" class="text" onclick={() => go(profileStep)}
                >Ändern</button
              >
            </h2>
            <p>
              {taxProfile.activity === "trade" ? "Gewerbe" : "Freiberuflich"}<br
              />{taxProfile.filing === "advisor"
                ? "Steuerkanzlei übermittelt"
                : "Du übermittelst selbst über ELSTER"}{#if !company.smallBusiness}<br
                />Voranmeldung {taxProfile.vatPeriod === "yearly"
                  ? "nur jährlich"
                  : frequencyLabels[taxProfile.vatPeriod]}{taxProfile.vatPeriod !== "yearly" &&
                taxProfile.vatExtension
                  ? " mit Dauerfristverlängerung"
                  : ""} · {taxProfile.taxBasis === "ist"
                  ? "Ist-Versteuerung"
                  : "Soll-Versteuerung"}{/if}
            </p>
          </section>
          <section>
            <h2>
              Zahlung <button type="button" class="text" onclick={() => go(paymentStep)}
                >Ändern</button
              >
            </h2>
            <p>{paymentChoices.find((choice) => choice.id === payment)?.label}</p>
            {#if transfer}<p>
                {company.iban || "IBAN noch offen"}<br />{company.bankName}
                {company.bic}
              </p>
              {#each company.additionalAccounts ?? [] as account}<p>
                  {account.iban} · {account.name}
                  {account.bic}
                </p>{/each}
            {/if}
          </section>
          <section>
            <h2>
              E-Mail <button type="button" class="text" onclick={() => go(mailStep)}>Ändern</button>
            </h2>
            <p>{mailSaved ? "Konto eingerichtet" : "Noch nicht eingerichtet (optional)"}</p>
          </section>
          <section>
            <h2>
              Zusätze <button type="button" class="text" onclick={() => go(shopStep)}>Ändern</button
              >
            </h2>
            <p>
              Shop: {setupShopSummary(shop, (id) => shopProviderByID(id)?.label)}<br />Tarif: {setupPlanSummary(
                plusBooked,
              )}<br />Newsletter: {newsletterEmail
                ? `Bestätigung an ${newsletterEmail} geschickt`
                : plusBooked
                  ? "Auf der Buchungsseite möglich"
                  : "Nicht abonniert"}
            </p>
          </section>
        </div>
        {#if missing.length}<aside class="wizard-note">
            <strong>Noch zu ergänzen</strong>
            <p>
              Du kannst das Profil speichern. Vor dem Ausstellen prüft die App die konkrete Rechnung
              noch einmal.
            </p>
            <ul>
              {#each missing as item}<li>
                  <button type="button" class="text" onclick={() => go(item.step)}
                    >{item.message}</button
                  >
                </li>{/each}
            </ul>
          </aside>{:else}<p class="wizard-note">
            Deine wichtigsten Profilangaben sind vollständig. Die Rechnung prüft die App vor dem
            Ausstellen noch einmal.
          </p>{/if}
        <p class="settings-later">
          Alles, was du hier festlegst, kannst du jederzeit unter <strong>Einstellungen</strong> ändern,
          auch welche Bereiche im Menü erscheinen, etwa Shop-Bestellungen oder Artikel.
        </p>
      {/if}
      {#if localError || error}<p
          class="wizard-error"
          role="alert"
          tabindex="-1"
          bind:this={errorElement}
        >
          {localError || error}
        </p>{/if}
      <footer class="wizard-footer">
        {#if step > 0}<button
            type="button"
            class="secondary"
            onclick={() => go(step === last && plusBooked ? plusStep : step - 1)}>Zurück</button
          >{/if}
        <span
          >{step === last
            ? "Lokal auf diesem Gerät gespeichert"
            : "Deine Angaben bleiben beim Zurückgehen erhalten."}</span
        >
        <button type="submit" class="primary" disabled={locked}
          >{checking
            ? "Wird geprüft …"
            : busy
              ? "Wird gespeichert …"
              : step === last
                ? "Profil speichern & starten"
                : (step === mailStep && !mailSaved) ||
                    (step === shopStep && !shop) ||
                    (step === plusStep && !plusBooked) ||
                    (step === newsStep && !newsletterEmail)
                  ? "Überspringen"
                  : "Weiter"}<Icon name="next" size={16} /></button
        >
      </footer>
    </fieldset>
  </form>
  {#if step === 0}<p class="restore-link">
      Schon eingerichtet? <button type="button" class="text" onclick={onrestore} disabled={busy}
        >Lokale Sicherung wiederherstellen</button
      >
    </p>{/if}
</main>

<style>
  .settings-later {
    margin: 12px 0 0;
    color: var(--theme-muted);
    font-size: 13px;
    line-height: 1.5;
  }
  .setup-wizard {
    max-width: 820px;
  }
  .wizard-help {
    margin: 16px 0;
    padding: 12px 0;
    border-bottom: 1px solid var(--theme-border);
  }
  .wizard-help summary {
    cursor: pointer;
    color: var(--theme-accent);
  }
  .wizard-note h2 {
    font-size: 17px;
    margin-top: 0;
  }
  .wizard-note li {
    margin: 8px 0;
    line-height: 1.5;
  }
  .wizard-progress {
    display: flex;
    flex-wrap: wrap;
    gap: 8px;
    padding: 0;
    margin: 16px 0 24px;
    list-style: none;
  }
  .wizard-progress button {
    display: flex;
    align-items: center;
    gap: 6px;
    padding: 8px;
    border: 0;
    background: transparent;
    color: var(--theme-muted);
    font-size: 12px;
  }
  .wizard-progress button[aria-current] {
    background: var(--theme-selected);
    color: var(--theme-text);
    border-radius: 6px;
  }
  .wizard-progress span {
    font-weight: 700;
  }
  h1 {
    font-size: 28px;
  }
  h1:focus {
    outline: none;
  }
  .wizard-choices {
    display: grid;
    gap: 10px;
    margin: 20px 0;
  }
  /* A question inside a step, smaller than the step's own heading. */
  .wizard-question {
    margin: 24px 0 0;
    font-size: var(--ui-subsection-title-size);
  }
  .wizard-question + .wizard-choices {
    margin-top: 10px;
  }
  .wizard-options {
    display: grid;
    gap: 8px;
    margin: 10px 0 0;
  }
  .wizard-options label {
    display: flex;
    align-items: flex-start;
    gap: 8px;
    font-size: 14px;
    font-weight: 400;
  }
  .wizard-options input {
    width: auto;
    min-height: 0;
    flex-shrink: 0;
  }
  .wizard-choices label {
    display: flex;
    align-items: flex-start;
    gap: 12px;
    border: 1px solid var(--theme-border);
    border-radius: 8px;
    padding: 16px;
    background: var(--theme-surface);
    cursor: pointer;
  }
  .wizard-choices label.selected {
    border-color: var(--theme-focus);
    background: var(--theme-selected);
  }
  .wizard-choices input {
    width: auto;
    min-height: 0;
    flex-shrink: 0;
  }
  .wizard-choices span {
    display: grid;
    gap: 6px;
  }
  .wizard-choices small {
    color: var(--theme-muted);
    font-weight: 400;
    line-height: 1.5;
  }
  .shop-choices {
    grid-template-columns: repeat(3, minmax(0, 1fr));
  }
  .shop-choices label {
    align-items: center;
  }
  .shop-choices .shop-none {
    grid-column: 1 / -1;
    align-items: flex-start;
  }
  .shop-note {
    display: flex;
    gap: 12px;
    align-items: flex-start;
  }
  .shop-note p {
    margin: 0;
  }
  .support-offer {
    display: flex;
    gap: 12px;
    margin: 16px 0 0;
    padding: 20px 22px;
    border: 1px solid var(--theme-border);
    border-radius: 10px;
    background: var(--theme-surface);
  }
  .support-icon {
    display: inline-flex;
    flex-shrink: 0;
    align-items: center;
    justify-content: center;
    width: 40px;
    height: 40px;
    border-radius: 50%;
    background: var(--theme-selected);
    color: var(--theme-accent);
  }
  .support-eyebrow {
    margin: 0 0 2px;
    color: var(--theme-muted);
    font-size: 11px;
    font-weight: 700;
    letter-spacing: 0.06em;
    text-transform: uppercase;
  }
  .support-offer h2 {
    margin: 0 0 8px;
    font-size: var(--ui-section-title-size);
  }
  .support-offer p {
    margin: 0 0 10px;
  }
  .support-offer ul {
    display: grid;
    gap: 7px;
    margin: 0 0 14px;
    padding: 0;
    list-style: none;
  }
  .support-offer li {
    display: flex;
    gap: 8px;
    line-height: 1.45;
  }
  .support-offer li :global(svg) {
    flex-shrink: 0;
    margin-top: 4px;
    color: var(--theme-accent);
  }
  .support-offer .form-help {
    margin: 10px 0 0;
  }
  .wizard-footer {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: 12px;
    border-top: 1px solid var(--theme-border);
    padding-top: 20px;
    margin-top: 24px;
  }
  .wizard-footer span {
    flex: 1;
    color: var(--theme-muted);
    font-size: 12px;
  }
  .wizard-review {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 16px;
  }
  .wizard-review section {
    padding: 16px;
    border: 1px solid var(--theme-border);
    border-radius: 8px;
    overflow-wrap: anywhere;
  }
  .wizard-review section:last-child:nth-child(odd) {
    grid-column: 1 / -1;
  }
  .wizard-review h2 {
    display: flex;
    justify-content: space-between;
    flex-wrap: wrap;
    gap: 8px;
    font-size: 16px;
    margin-top: 0;
  }
  .wizard-note {
    padding: 16px;
    margin: 16px 0;
    background: var(--theme-info-bg);
    color: var(--theme-info-text);
    border-radius: 8px;
  }
  .wizard-error {
    color: var(--theme-error-text);
    background: var(--theme-error-bg);
    padding: 12px;
    border-radius: 8px;
  }
  p {
    line-height: 1.5;
  }
  @media (max-width: 650px) {
    .wizard-review,
    .shop-choices {
      grid-template-columns: minmax(0, 1fr);
    }
  }
</style>
