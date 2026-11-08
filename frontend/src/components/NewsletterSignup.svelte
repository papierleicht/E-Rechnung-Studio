<script lang="ts">
  import { onMount } from "svelte";
  import { Browser } from "@wailsio/runtime";
  import SparklesIcon from "@lucide/svelte/icons/sparkles";
  import RocketIcon from "@lucide/svelte/icons/rocket";
  import WrenchIcon from "@lucide/svelte/icons/wrench";
  import LightbulbIcon from "@lucide/svelte/icons/lightbulb";
  import MessagesIcon from "@lucide/svelte/icons/messages-square";
  import MailCheckIcon from "@lucide/svelte/icons/mail-check";
  import * as AccountAPI from "../../bindings/papierleicht.de/e-rechnung-studio/internal/app/accountservice";
  import type { Newsletter } from "../../bindings/papierleicht.de/e-rechnung-studio/internal/account/models";
  import { errorDetail } from "../lib/errors";
  import { newsletterDate, validNewsletterEmail } from "../lib/newsletter";

  let {
    email: suggested = "",
    source,
    onrequested = () => {},
  }: {
    /** Prefills the field, e.g. with the company's contact address. */
    email?: string;
    /** Where the consent is given: "app-onboarding" or "app-settings". */
    source: "app-onboarding" | "app-settings";
    /** Called once a confirmation mail was requested, now or earlier. */
    onrequested?: (note: Newsletter) => void;
  } = $props();

  const privacyURL = "https://papierleicht.de/datenschutz#newsletter";
  const benefits = [
    { icon: RocketIcon, text: "Neue Funktionen, kurz vorgestellt, sobald sie da sind" },
    { icon: WrenchIcon, text: "Fehlerbehebungen und wichtige Updates auf einen Blick" },
    { icon: LightbulbIcon, text: "Tipps, mit denen du in der App schneller ans Ziel kommst" },
    {
      icon: MessagesIcon,
      text: "Regelmäßig ein Überblick, was in Chat & Markt los ist, auch ohne Abo",
    },
  ];

  // svelte-ignore state_referenced_locally
  let email = $state(suggested);
  let requested = $state<Newsletter | null>(null);
  let editing = $state(false);
  let sending = $state(false);
  let sent = $state(false);
  let error = $state("");
  const showForm = $derived(!requested?.email || editing);

  onMount(async () => {
    try {
      const note = await AccountAPI.Newsletter();
      if (note?.email) {
        requested = note;
        email = note.email;
        onrequested(note);
      }
    } catch {
      // Without the note the form simply shows again.
    }
  });

  async function subscribe() {
    if (sending) return;
    error = "";
    const address = email.trim();
    if (!validNewsletterEmail(address)) {
      error = "Bitte gib eine gültige E-Mail-Adresse ein.";
      return;
    }
    sending = true;
    try {
      requested = await AccountAPI.SubscribeNewsletter(address, source);
      onrequested(requested);
      editing = false;
      sent = true;
    } catch (e) {
      error = errorDetail(e) || "Die Anmeldung hat nicht geklappt. Versuche es noch einmal.";
    } finally {
      sending = false;
    }
  }

  async function openPrivacy() {
    try {
      await Browser.OpenURL(privacyURL);
    } catch {
      window.open(privacyURL, "_blank", "noopener,noreferrer");
    }
  }
</script>

<!-- Settings use the smaller text of their sections; the setup keeps its own. -->
<section
  class="newsletter"
  class:compact={source === "app-settings"}
  aria-labelledby={`newsletter-title-${source}`}
>
  {#if showForm}
    <div class="newsletter-head">
      <span class="newsletter-badge" aria-hidden="true"><SparklesIcon size={20} /></span>
      <div>
        <p class="newsletter-eyebrow">Newsletter · kostenlos</p>
        <h2 id={`newsletter-title-${source}`}>Bleib auf dem Laufenden</h2>
      </div>
    </div>
    <p class="newsletter-lead">
      Erfahre zuerst, was E-Rechnung-Studio Neues kann und was sich in der Community tut. Wir
      schreiben nur, wenn es wirklich etwas zu berichten gibt.
    </p>
    <ul class="newsletter-benefits">
      {#each benefits as benefit}
        <li>
          <span aria-hidden="true"><benefit.icon size={16} /></span>{benefit.text}
        </li>
      {/each}
    </ul>
    <div class="newsletter-form">
      <label for={`newsletter-email-${source}`} class="visually-hidden">E-Mail-Adresse</label>
      <input
        id={`newsletter-email-${source}`}
        type="email"
        autocomplete="email"
        placeholder="name@beispiel.de"
        bind:value={email}
        aria-invalid={error ? "true" : undefined}
        aria-describedby={`newsletter-consent-${source}`}
        oninput={() => (error = "")}
        onkeydown={(event) => {
          // The field can sit inside the setup form; Enter subscribes here
          // instead of saving the profile.
          if (event.key === "Enter") {
            event.preventDefault();
            void subscribe();
          }
        }}
      />
      <button type="button" class="primary" disabled={sending} onclick={subscribe}
        >{sending ? "Wird gesendet …" : "Kostenlos abonnieren"}</button
      >
      {#if editing}<button type="button" class="secondary" onclick={() => (editing = false)}
          >Abbrechen</button
        >{/if}
    </div>
    {#if error}<p class="newsletter-error" role="alert">{error}</p>{/if}
    <p class="newsletter-consent" id={`newsletter-consent-${source}`}>
      Mit „Kostenlos abonnieren“ willigst du ein, dass papierleicht dir Neuigkeiten zu
      E-Rechnung-Studio und einen Überblick aus Chat &amp; Markt per E-Mail schickt. Du bekommst
      zuerst eine E-Mail zur Bestätigung. Abmelden kannst du dich jederzeit mit einem Klick in jeder
      E-Mail.
      <button type="button" class="text" onclick={openPrivacy}>Datenschutz</button>
    </p>
  {:else if requested}
    <div class="newsletter-head" role="status">
      <span class="newsletter-badge done" aria-hidden="true"><MailCheckIcon size={20} /></span>
      <div>
        <p class="newsletter-eyebrow">Newsletter</p>
        <h2 id={`newsletter-title-${source}`}>
          {sent ? "Fast geschafft! Schau in dein Postfach" : "Newsletter angefordert"}
        </h2>
      </div>
    </div>
    <p class="newsletter-lead">
      {#if sent}
        Wir haben dir eine E-Mail an <strong>{requested.email}</strong> geschickt. Klicke auf den Link
        darin, dann bist du dabei.
      {:else}
        Am {newsletterDate(requested.requestedAt)} an <strong>{requested.email}</strong>. Hast du
        den Link in der Bestätigungsmail geklickt, bist du dabei. Abmelden kannst du dich über den
        Link in jeder Newsletter-Mail.
      {/if}
    </p>
    {#if error}<p class="newsletter-error" role="alert">{error}</p>{/if}
    <p class="newsletter-actions">
      <button type="button" class="text" disabled={sending} onclick={subscribe}
        >{sending ? "Wird gesendet …" : "Bestätigungsmail erneut senden"}</button
      >
      <button
        type="button"
        class="text"
        onclick={() => {
          editing = true;
          error = "";
        }}>Andere Adresse verwenden</button
      >
    </p>
    {#if sent}<p class="newsletter-consent">
        Keine E-Mail da? Schau auch im Spam-Ordner nach. Ohne deine Bestätigung schicken wir dir
        nichts.
      </p>{/if}
  {/if}
</section>

<style>
  .newsletter {
    margin: 24px 0 0;
    padding: 20px 22px;
    border: 1px solid var(--theme-border);
    border-radius: 10px;
    background: var(--theme-surface);
    box-shadow: inset 4px 0 0 var(--theme-accent);
  }
  .newsletter-head {
    display: flex;
    align-items: center;
    gap: 12px;
  }
  .newsletter-badge {
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
  .newsletter-badge.done {
    background: var(--theme-success-bg);
    color: var(--theme-success-text);
  }
  .newsletter-eyebrow {
    margin: 0 0 2px;
    color: var(--theme-muted);
    font-size: 11px;
    font-weight: 700;
    letter-spacing: 0.06em;
    text-transform: uppercase;
  }
  .newsletter h2 {
    margin: 0;
    font-size: var(--ui-section-title-size);
  }
  .compact .newsletter-lead,
  .compact .newsletter-benefits,
  .compact .newsletter-error {
    font-size: var(--ui-control-font-size);
  }
  .newsletter-lead {
    margin: 12px 0 0;
    line-height: 1.5;
  }
  .newsletter-benefits {
    display: grid;
    gap: 8px;
    margin: 14px 0 0;
    padding: 0;
    list-style: none;
  }
  .newsletter-benefits li {
    display: flex;
    align-items: flex-start;
    gap: 10px;
    line-height: 1.45;
  }
  .newsletter-benefits span {
    display: inline-flex;
    flex-shrink: 0;
    margin-top: 2px;
    color: var(--theme-accent);
  }
  .newsletter-form {
    display: flex;
    flex-wrap: wrap;
    gap: 8px;
    margin-top: 16px;
  }
  .newsletter-form input {
    flex: 1 1 240px;
    min-width: 0;
  }
  .newsletter-form button {
    flex-shrink: 0;
  }
  .newsletter-consent {
    margin: 10px 0 0;
    color: var(--theme-muted);
    font-size: 12px;
    line-height: 1.5;
  }
  .newsletter-consent button {
    padding: 0;
    font-size: inherit;
  }
  .newsletter-actions {
    display: flex;
    flex-wrap: wrap;
    gap: 6px 18px;
    margin: 12px 0 0;
  }
  .newsletter-actions button {
    padding: 0;
  }
  .newsletter-error {
    margin: 10px 0 0;
    padding: 10px 12px;
    border-radius: 8px;
    background: var(--theme-error-bg);
    color: var(--theme-error-text);
  }
  .visually-hidden {
    position: absolute;
    width: 1px;
    height: 1px;
    overflow: hidden;
    clip: rect(0 0 0 0);
    white-space: nowrap;
  }
</style>
