<script lang="ts">
  import { Browser } from "@wailsio/runtime";
  import LandmarkIcon from "@lucide/svelte/icons/landmark";
  import CheckIcon from "@lucide/svelte/icons/check";
  import SecurityMeter from "../../components/SecurityMeter.svelte";
  import * as AccountAPI from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/app/accountservice";
  import { setupPlusTiers, type SetupPlusTier } from "../../lib/setupOffers";

  let {
    booked = $bindable(""),
  }: {
    /** The plan whose booking page was opened, "" while none was. */
    booked?: "" | SetupPlusTier;
  } = $props();
  let error = $state("");

  // Booking happens on the website; the app learns about it after sign-in.
  async function book(tier: SetupPlusTier) {
    error = "";
    let manage = "https://papierleicht.de/konto";
    try {
      manage = (await AccountAPI.Status())?.manageUrl || manage;
    } catch {
      // The default page works as well.
    }
    const url = `${manage}?tarif=${tier}`;
    try {
      await Browser.OpenURL(url);
      booked = tier;
    } catch {
      error = `Die Website ließ sich nicht öffnen. Du findest die Buchung unter ${url}.`;
    }
  }
</script>

<aside class="bank-highlight">
  <span aria-hidden="true"><LandmarkIcon size={22} /></span>
  <div>
    <h2>Bank automatisch verbinden</h2>
    <p>
      Mit Plus holt die App Umsätze und Kontostand per Klick direkt von deiner Bank, über FinTS, den
      Standard der deutschen Banken. Kein CSV-Export mehr, und Zahlungseingänge ordnest du sofort
      deinen Rechnungen zu. PIN und TAN bleiben auf deinem Gerät.
    </p>
  </div>
</aside>
<div class="plus-tiers">
  {#each setupPlusTiers as tier}
    <article class:chosen={booked === tier.id}>
      {#if tier.badge}<span class="tier-badge">{tier.badge}</span>{/if}
      <h2>{tier.name}</h2>
      <p class="tier-level">
        <SecurityMeter level={tier.level} /><span>Sicherheitsstufe {tier.level} von 3</span>
      </p>
      <p class="tier-lead">{tier.lead}</p>
      <ul>
        {#each tier.items as item}<li>
            <CheckIcon size={15} aria-hidden="true" /><span>{item}</span>
          </li>{/each}
      </ul>
      <button
        type="button"
        class={booked === tier.id || (!booked && tier.id === "plus") ? "primary" : "secondary"}
        onclick={() => book(tier.id)}>{tier.name} buchen</button
      >
    </article>
  {/each}
</div>
{#if error}<p class="wizard-error" role="alert">{error}</p>{/if}
{#if booked}
  <aside class="wizard-note" aria-live="polite">
    <h2>Die Buchung ist im Browser geöffnet</h2>
    <p>
      Dort meldest du dich mit deiner E-Mail-Adresse an und kannst dich gleich auch für den
      Newsletter eintragen. Nach dem Bezahlen meldest du dich in der App unter Einstellungen → Konto
      &amp; Abo mit derselben Adresse an, dann ist Plus freigeschaltet.
    </p>
  </aside>
{:else}
  <p class="form-help">
    Gebucht wird auf papierleicht.de, monatlich oder jährlich, kündbar zum Ende der bezahlten
    Laufzeit. Ohne Plus bleibt die App vollständig nutzbar. Du kannst Plus auch später unter
    Einstellungen → Konto &amp; Abo buchen.
  </p>
{/if}

<style>
  .bank-highlight {
    display: flex;
    gap: 14px;
    margin: 20px 0;
    padding: 18px 20px;
    border: 1px solid var(--theme-border);
    border-radius: 10px;
    background: var(--theme-selected);
  }
  .bank-highlight > span {
    display: inline-flex;
    flex-shrink: 0;
    align-items: center;
    justify-content: center;
    width: 40px;
    height: 40px;
    border-radius: 50%;
    background: var(--theme-surface);
    color: var(--theme-accent);
  }
  .bank-highlight h2 {
    margin: 0 0 6px;
    font-size: 17px;
  }
  .bank-highlight p {
    margin: 0;
  }
  .plus-tiers {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 14px;
  }
  .plus-tiers article {
    position: relative;
    display: flex;
    flex-direction: column;
    gap: 10px;
    padding: 18px;
    border: 1px solid var(--theme-border);
    border-radius: 10px;
    background: var(--theme-surface);
  }
  .plus-tiers article.chosen {
    border-color: var(--theme-focus);
    box-shadow: 0 0 0 1px var(--theme-focus);
  }
  .plus-tiers h2 {
    margin: 0;
    font-size: 18px;
  }
  .tier-badge {
    position: absolute;
    top: 16px;
    right: 16px;
    padding: 1px 9px;
    border-radius: 999px;
    background: var(--theme-primary);
    color: var(--theme-on-primary);
    font-size: 12px;
    font-weight: 600;
  }
  .tier-level {
    display: flex;
    align-items: center;
    gap: 8px;
    margin: 0;
    color: var(--theme-muted);
    font-size: 13px;
  }
  .tier-lead {
    margin: 0;
  }
  .plus-tiers ul {
    display: grid;
    gap: 7px;
    margin: 0 0 auto;
    padding: 0;
    list-style: none;
    font-size: 14px;
  }
  .plus-tiers li {
    display: flex;
    align-items: flex-start;
    gap: 8px;
    line-height: 1.45;
  }
  .plus-tiers li :global(svg) {
    flex-shrink: 0;
    margin-top: 3px;
    color: var(--theme-accent);
  }
  .plus-tiers button {
    width: 100%;
    justify-content: center;
  }
  .wizard-note {
    padding: 16px;
    margin: 16px 0;
    background: var(--theme-info-bg);
    color: var(--theme-info-text);
    border-radius: 8px;
  }
  .wizard-note h2 {
    margin: 0 0 6px;
    font-size: 17px;
  }
  .wizard-note p {
    margin: 0;
  }
  .wizard-error {
    color: var(--theme-error-text);
    background: var(--theme-error-bg);
    padding: 12px;
    border-radius: 8px;
  }
  .form-help {
    margin-top: 16px;
  }
  p {
    line-height: 1.5;
  }
  @media (max-width: 760px) {
    .plus-tiers {
      grid-template-columns: minmax(0, 1fr);
    }
  }
</style>
