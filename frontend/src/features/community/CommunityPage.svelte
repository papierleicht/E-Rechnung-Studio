<script lang="ts">
  import { onDestroy, onMount } from "svelte";
  import { Browser } from "@wailsio/runtime";
  import MessagesIcon from "@lucide/svelte/icons/messages-square";
  import HandshakeIcon from "@lucide/svelte/icons/handshake";
  import PlusIcon from "@lucide/svelte/icons/plus";
  import MailIcon from "@lucide/svelte/icons/mail";
  import ShieldIcon from "@lucide/svelte/icons/shield-check";
  import UsersIcon from "@lucide/svelte/icons/users";
  import SparklesIcon from "@lucide/svelte/icons/sparkles";
  import CheckIcon from "@lucide/svelte/icons/check";
  import ActionTiles from "../../components/ActionTiles.svelte";
  import FormDialog from "../../components/FormDialog.svelte";
  import * as ChatAPI from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/app/chatservice";
  import * as AccountAPI from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/app/accountservice";
  import type {
    ChatPreview,
    ChatState,
    MarketDraft,
  } from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/account/models";
  import type { ActionTile } from "../../lib/actionTiles";
  import { priceOf, type PlanPrice } from "../../lib/planPrices";
  import { CONTACT_EMAIL, listingCounts, showNumbers } from "../../lib/community";
  import { errorDetail } from "../../lib/errors";
  import { cached, remember } from "../../lib/cache";
  import { watchActivation, type ActivationWatch } from "../../lib/watchActivation";
  import { setChatViewing, watchChatUnread } from "../../lib/chatUnread";
  import ChatRoom from "./ChatRoom.svelte";
  import MarketBoard from "./MarketBoard.svelte";
  import JoinDialog from "./JoinDialog.svelte";
  import ListingDialog from "./ListingDialog.svelte";
  import GuidelinesText from "./GuidelinesText.svelte";

  let {
    postalCode = "",
    city = "",
    version = "",
    onaccount,
    confirm,
  }: {
    /** The company's address: default area of "Angebote & Gesuche". */
    postalCode?: string;
    city?: string;
    version?: string;
    /** Opens Einstellungen → Konto & Abo for the sign-in. */
    onaccount: () => void;
    confirm: (title: string, text: string, action: string) => Promise<boolean>;
  } = $props();

  // The last state shows right away while the page asks the server again,
  // so opening the tab never starts from "wird geladen".
  let chat = $state<ChatState | null>(cached<ChatState>("community:state") ?? null);
  let preview = $state<ChatPreview | null>(cached<ChatPreview>("community:preview") ?? null);
  let loading = $state(!cached("community:state"));
  let error = $state("");
  let view = $state<"chat" | "market">(cached<"chat" | "market">("community:view") ?? "chat");
  let dialog = $state<"" | "join" | "rename" | "guidelines" | "listing" | "unlock">("");
  let draft = $state("");
  let online = $state(0);
  let reload = $state(0);
  let booked = $state("");
  let watch: ActivationWatch | undefined;

  const member = $derived(chat?.access === "member");
  const subscribed = $derived(chat?.access === "member" || chat?.access === "join");
  const demo = $derived(!member);
  const numbers = $derived(showNumbers(preview));

  async function loadState() {
    try {
      chat = remember("community:state", await ChatAPI.State());
      error = "";
    } catch (err) {
      error = errorDetail(err);
    } finally {
      loading = false;
    }
    return chat;
  }

  // Amounts only as the license server charges them; without them the
  // showcase names no price and the website shows it.
  let prices = $state<PlanPrice[]>(cached<PlanPrice[]>("community:prices") ?? []);
  const supportMonthly = $derived(priceOf(prices, "support", "monthly"));
  const supportYearly = $derived(priceOf(prices, "support", "yearly"));
  const plusMonthly = $derived(priceOf(prices, "plus", "monthly"));
  const plusYearly = $derived(priceOf(prices, "plus", "yearly"));
  async function loadPrices() {
    try {
      prices = remember("community:prices", (await AccountAPI.Prices()) ?? []);
    } catch {
      // Offline or an older server: the buttons work without amounts.
    }
  }

  async function loadPreview() {
    try {
      preview = remember("community:preview", await ChatAPI.Preview());
    } catch {
      // The showcase works without numbers.
    }
  }

  onMount(() => {
    void loadState();
    void loadPreview();
    void loadPrices();
  });
  $effect(() => setChatViewing(member && view === "chat"));
  // Back on the tab the customer sees the part they left.
  $effect(() => void remember("community:view", view));
  // Before joining (or signing in) the badge checks only rarely; a member
  // gets the regular checks right away.
  $effect(() => {
    if (member) watchChatUnread();
  });
  onDestroy(() => {
    watch?.stop();
    setChatViewing(false);
  });

  async function openURL(url: string) {
    try {
      await Browser.OpenURL(url);
    } catch {
      window.open(url, "_blank", "noopener,noreferrer");
    }
  }

  async function book(tier: "support" | "plus") {
    let manage = "https://papierleicht.de/konto";
    try {
      manage = (await AccountAPI.Status())?.manageUrl || manage;
    } catch {
      // The default page works as well.
    }
    await openURL(`${manage}?tarif=${tier}`);
    booked = tier;
    dialog = "";
    // Once paid, the chat opens by itself, as long as the app is signed in.
    watch?.stop();
    watch = watchActivation(async () => {
      const next = await loadState();
      return next?.access === "member" || next?.access === "join";
    });
  }

  function mail() {
    const subject = encodeURIComponent("Frage zu E-Rechnung-Studio");
    const body = encodeURIComponent(
      `\n\n\n---\nApp-Version: ${version || "unbekannt"}\nSystem: ${navigator.platform}`,
    );
    void openURL(`mailto:${CONTACT_EMAIL}?subject=${subject}&body=${body}`);
  }

  function locked() {
    if (chat?.access === "join") dialog = "join";
    else dialog = "unlock";
  }

  function create() {
    if (member) dialog = "listing";
    else locked();
  }

  async function join(name: string) {
    chat = remember("community:state", await ChatAPI.Join(name, chat?.guidelines ?? ""));
    dialog = "";
  }

  async function publish(listing: MarketDraft) {
    await ChatAPI.CreateListing(listing);
    dialog = "";
    view = "market";
    reload++;
  }

  const tiles: ActionTile[] = $derived([
    {
      id: "chat",
      primary: true,
      icon: MessagesIcon,
      label: "Community-Chat",
      hint: member
        ? online > 1
          ? `${online} Mitglieder gerade online`
          : "Fragen stellen, Erfahrungen teilen"
        : "Exklusiv mit Support oder Plus",
      current: view === "chat",
      onclick: () => (view = "chat"),
    },
    {
      id: "market",
      icon: HandshakeIcon,
      label: "Angebote & Gesuche",
      hint:
        preview && preview.offers + preview.requests > 0
          ? listingCounts(preview.offers, preview.requests)
          : "Aufträge unter Unternehmern",
      current: view === "market",
      onclick: () => (view = "market"),
    },
    {
      id: "create",
      icon: PlusIcon,
      label: "Eintrag veröffentlichen",
      hint: "Angebot oder Gesuch aufgeben",
      onclick: create,
    },
    {
      id: "mail",
      icon: MailIcon,
      label: "Support per E-Mail",
      hint: CONTACT_EMAIL,
      title: `E-Mail an ${CONTACT_EMAIL} schreiben`,
      onclick: mail,
    },
    {
      id: "guidelines",
      icon: ShieldIcon,
      label: "Community-Richtlinien",
      hint: "So gehen wir miteinander um",
      onclick: () => (dialog = "guidelines"),
    },
  ]);
</script>

<div class="community">
  <header class="topbar community-topbar">
    <div>
      <h1>Chat &amp; Markt</h1>
    </div>
  </header>
  <p class="page-intro community-intro">
    {#if member && chat?.member}
      Du bist als <strong>{chat.member.displayName}</strong> dabei.
      <button type="button" class="text inline-link" onclick={() => (dialog = "rename")}
        >Namen ändern</button
      >
    {:else}
      Die Community für Unternehmer: Fragen klären, Aufträge finden, Hilfe bekommen.
    {/if}
  </p>

  <ActionTiles {tiles} label="Chat und Support" />

  {#if error}
    <div class="community-feedback" role="alert">
      <p>{error}</p>
      <button
        type="button"
        class="text"
        onclick={() => {
          loading = true;
          void loadState();
        }}>Erneut versuchen</button
      >
    </div>
  {/if}

  {#if !loading && !member}
    <section class="exclusive" aria-labelledby="exclusive-title">
      <div class="exclusive-text">
        <p class="eyebrow">
          <SparklesIcon size={14} aria-hidden="true" /> Exklusiv für Unternehmer
        </p>
        <h2 id="exclusive-title">
          {chat?.access === "join"
            ? "Dein Platz in der Community ist frei"
            : "Werde Teil der papierleicht-Community"}
        </h2>
        <p>
          {chat?.access === "join"
            ? "Dein Abo schaltet Chat und Angebote & Gesuche frei. Stimm den Richtlinien zu, wähle deinen Anzeigenamen, und los geht's."
            : "Ein geschützter Raum nur für Selbstständige und Firmen mit Community Support oder Plus. Hier hilft man sich, empfiehlt sich weiter und findet Aufträge in der Nähe."}
        </p>
        <ul>
          <li><CheckIcon size={14} aria-hidden="true" />Live-Chat mit anderen Unternehmern</li>
          <li>
            <CheckIcon size={14} aria-hidden="true" />Angebote &amp; Gesuche aus deiner Region
          </li>
          <li><CheckIcon size={14} aria-hidden="true" />Software-Support per E-Mail inklusive</li>
          <li><CheckIcon size={14} aria-hidden="true" />Moderiert, ohne Werbeflut und Fremde</li>
        </ul>
      </div>
      <div class="exclusive-side">
        {#if numbers && preview}
          <dl class="exclusive-numbers" aria-label="Die Community gerade">
            <div>
              <dt>Mitglieder</dt>
              <dd>{preview.members}</dd>
            </div>
            <div>
              <dt>Nachrichten diese Woche</dt>
              <dd>{preview.messagesWeek}</dd>
            </div>
            <div>
              <dt>Offene Einträge</dt>
              <dd>{preview.offers + preview.requests}</dd>
            </div>
          </dl>
        {:else}
          <p class="founding">
            <UsersIcon size={16} aria-hidden="true" />
            Die Community startet gerade. Wer jetzt dabei ist, prägt sie mit.
          </p>
        {/if}
        {#if chat?.access === "join"}
          <button type="button" class="primary" onclick={() => (dialog = "join")}
            >Community beitreten</button
          >
        {:else}
          <button type="button" class="primary" onclick={() => book("support")}
            >Community Support{supportMonthly ? ` · ${supportMonthly} im Monat` : " buchen"}</button
          >
          <button type="button" class="secondary" onclick={() => book("plus")}
            >Mit Plus freischalten</button
          >
          {#if chat?.access === "signed-out"}
            <button type="button" class="text" onclick={onaccount}
              >Schon gebucht? In der App anmelden</button
            >
          {/if}
        {/if}
        {#if booked}
          <p class="booked" role="status">
            Die Buchung ist im Browser geöffnet. {chat?.access === "signed-out"
              ? "Melde dich danach unter Einstellungen → Konto & Abo mit derselben E-Mail-Adresse an, dann öffnet sich die Community."
              : "Sobald die Zahlung da ist, öffnet sich die Community hier von selbst."}
          </p>
        {/if}
      </div>
    </section>
  {/if}

  {#if loading}
    <p class="community-loading" role="status">Community wird geladen …</p>
  {:else if view === "chat"}
    {#key member}
      <ChatRoom
        {demo}
        me={chat?.member?.displayName ?? ""}
        team={chat?.member?.role === "team"}
        mutedUntil={chat?.member?.mutedUntil ?? ""}
        bind:draft
        bind:online
        onlocked={locked}
        onaccesslost={() => void loadState()}
        confirmDelete={() =>
          confirm(
            "Nachricht löschen",
            "Die Nachricht verschwindet für alle Mitglieder.",
            "Löschen",
          )}
      />
    {/key}
  {:else}
    {#key member}
      <MarketBoard
        {demo}
        {postalCode}
        {preview}
        {reload}
        oncreate={create}
        onlocked={locked}
        onaccesslost={() => void loadState()}
        confirmEnd={() =>
          confirm("Eintrag beenden", "Der Eintrag verschwindet für alle Mitglieder.", "Beenden")}
      />
    {/key}
  {/if}

  <!-- Members reach the support by the tile above; in the chat the window
       fits the screen without this section. -->
  {#if !(member && view === "chat")}
    <section class="contact" aria-labelledby="contact-title">
      <span class="contact-icon" aria-hidden="true"><MailIcon size={20} /></span>
      <div>
        <h2 id="contact-title">Support &amp; Kontakt</h2>
        <p>
          {#if subscribed}
            Software-Support per E-Mail ist in deinem Abo enthalten. Schreib uns am besten von der
            Adresse deines Kontos, dann finden wir dein Abo sofort.
          {:else}
            Fragen zu Abo, Rechnung oder Datenschutz? Schreib uns. Software-Support per E-Mail bei
            Fehlern, Einrichtung und Bedienung gehört zu Community Support und Plus.
          {/if}
          Steuerliche und rechtliche Fragen beantwortet der Support nicht.
        </p>
      </div>
      <div class="contact-actions">
        <button type="button" class="primary" onclick={mail}
          ><MailIcon size={16} aria-hidden="true" /> E-Mail schreiben</button
        >
        <a href={`mailto:${CONTACT_EMAIL}`} onclick={(event) => (event.preventDefault(), mail())}
          >{CONTACT_EMAIL}</a
        >
      </div>
    </section>
  {/if}
</div>

{#if dialog === "join" || dialog === "rename"}
  <JoinDialog
    serverVersion={chat?.guidelines ?? ""}
    currentName={chat?.member?.displayName ?? ""}
    rename={dialog === "rename"}
    onjoin={join}
    onclose={() => (dialog = "")}
  />
{:else if dialog === "listing"}
  <ListingDialog {postalCode} {city} oncreate={publish} onclose={() => (dialog = "")} />
{:else if dialog === "guidelines"}
  <FormDialog
    title="Community-Richtlinien"
    description="Gelten im Chat und unter Angebote & Gesuche."
    onclose={() => (dialog = "")}
    onsubmit={() => (dialog = "")}
  >
    <GuidelinesText />
    {#snippet footer()}
      <button type="submit" class="primary">Verstanden</button>
    {/snippet}
  </FormDialog>
{:else if dialog === "unlock"}
  <FormDialog
    title="Community freischalten"
    description="Chat und Angebote & Gesuche sind exklusiv für Unternehmer mit Community Support oder Plus."
    onclose={() => (dialog = "")}
    onsubmit={() => book("support")}
  >
    <div class="unlock-options">
      <article>
        <h3>Community Support</h3>
        {@render price(supportMonthly, supportYearly)}
        <ul>
          <li>Community-Chat und Angebote &amp; Gesuche</li>
          <li>Software-Support per E-Mail</li>
          <li>Die App bleibt kostenlos</li>
        </ul>
      </article>
      <article>
        <h3>Plus</h3>
        {@render price(plusMonthly, plusYearly)}
        <ul>
          <li>Alles aus Community Support</li>
          <li>Bank automatisch verbinden</li>
          <li>Protokoll täglich extern bestätigt</li>
        </ul>
      </article>
    </div>
    <p class="form-help">
      Nur für Unternehmer. Die Buchung öffnet sich im Browser auf papierleicht.de.
    </p>
    {#snippet footer()}
      <button type="button" class="secondary" onclick={() => book("plus")}>Plus ansehen</button>
      <button type="submit" class="primary">Community Support buchen</button>
    {/snippet}
  </FormDialog>
{/if}

{#snippet price(monthly: string, yearly: string)}
  {#if monthly}<p class="price">
      {monthly} <small>im Monat{yearly ? ` · oder ${yearly} im Jahr` : ""}</small>
    </p>
  {:else if yearly}<p class="price">{yearly} <small>im Jahr</small></p>
  {:else}<p class="price-note">Den aktuellen Preis zeigt papierleicht.de.</p>{/if}
{/snippet}

<style>
  .community-intro {
    display: flex;
    flex-wrap: wrap;
    align-items: baseline;
    gap: 0 6px;
  }
  .inline-link {
    padding: 0;
    font-size: inherit;
  }
  .community-feedback {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 12px;
    margin: 0 0 14px;
    padding: 10px 14px;
    border-radius: 10px;
    background: var(--theme-warning-bg);
    color: var(--theme-warning-text);
    font-size: 13px;
  }
  .community-feedback p {
    margin: 0;
  }
  .community-loading {
    color: var(--theme-muted);
    font-size: 13px;
  }
  /* Defensive CSS: columns shrink (minmax/min), rows wrap, long words break.
     The page is a container, so the layout follows its own width. */
  .community {
    container-type: inline-size;
    min-width: 0;
  }
  .exclusive {
    display: grid;
    grid-template-columns: minmax(0, 1.4fr) minmax(min(15rem, 100%), 1fr);
    gap: 18px;
    margin: 0 0 14px;
    padding: 18px 20px;
    border: 1px solid color-mix(in srgb, var(--theme-accent) 35%, var(--theme-border));
    border-radius: 14px;
    background:
      radial-gradient(
        120% 140% at 100% 0%,
        color-mix(in srgb, var(--theme-accent) 14%, transparent),
        transparent 60%
      ),
      var(--theme-surface);
  }
  .eyebrow {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    margin: 0 0 6px;
    color: var(--theme-accent);
    font-size: 12px;
    font-weight: 700;
    letter-spacing: 0.04em;
    text-transform: uppercase;
  }
  .exclusive h2 {
    margin: 0 0 6px;
    font-size: 19px;
  }
  .exclusive-text > p:not(.eyebrow) {
    margin: 0 0 10px;
    color: var(--theme-muted);
    font-size: 13px;
    line-height: 1.55;
  }
  .exclusive ul {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(min(14rem, 100%), 1fr));
    gap: 4px 14px;
    margin: 0;
    padding: 0;
    list-style: none;
    font-size: 13px;
  }
  .exclusive li {
    display: flex;
    align-items: flex-start;
    gap: 6px;
  }
  .exclusive li :global(svg) {
    flex: none;
    margin-top: 3px;
    color: var(--theme-accent);
  }
  .exclusive-side {
    display: flex;
    flex-direction: column;
    justify-content: flex-end;
    gap: 8px;
  }
  .exclusive-numbers {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(min(6.5rem, 100%), 1fr));
    gap: 6px;
    margin: 0 0 4px;
  }
  .exclusive-numbers div {
    padding: 8px 10px;
    border: 1px solid var(--theme-border);
    border-radius: 10px;
    background: var(--theme-surface);
  }
  .exclusive-numbers dt {
    color: var(--theme-muted);
    font-size: 11px;
  }
  .exclusive-numbers dd {
    margin: 2px 0 0;
    font-size: 18px;
    font-weight: 700;
  }
  .founding {
    display: flex;
    gap: 8px;
    margin: 0 0 4px;
    color: var(--theme-muted);
    font-size: 13px;
    line-height: 1.5;
  }
  .founding :global(svg) {
    flex: none;
    margin-top: 2px;
    color: var(--theme-accent);
  }
  .booked {
    margin: 4px 0 0;
    padding: 8px 10px;
    border-radius: 8px;
    background: var(--theme-info-bg);
    color: var(--theme-info-text);
    font-size: 12px;
    line-height: 1.5;
  }
  .contact {
    display: grid;
    grid-template-columns: auto minmax(0, 1fr) minmax(0, auto);
    align-items: center;
    gap: 14px;
    margin-top: 14px;
    padding: 16px 18px;
    border: 1px solid var(--theme-border);
    border-radius: 12px;
    background: var(--theme-surface);
  }
  .contact-icon {
    display: grid;
    place-items: center;
    width: 40px;
    height: 40px;
    border-radius: 10px;
    background: var(--theme-surface-muted);
    color: var(--theme-accent);
  }
  .contact h2 {
    margin: 0 0 2px;
    font-size: 15px;
  }
  .contact p {
    margin: 0;
    overflow-wrap: anywhere;
    color: var(--theme-muted);
    font-size: 13px;
    line-height: 1.5;
  }
  .contact-actions {
    display: grid;
    justify-items: end;
    gap: 4px;
  }
  .contact-actions .primary {
    display: inline-flex;
    align-items: center;
    gap: 6px;
  }
  .contact-actions a {
    overflow-wrap: anywhere;
    color: var(--theme-accent);
    font-size: 12px;
    user-select: text;
  }
  .unlock-options {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(min(13rem, 100%), 1fr));
    gap: 10px;
    margin-bottom: 10px;
  }
  .unlock-options article {
    padding: 12px 14px;
    border: 1px solid var(--theme-border);
    border-radius: 10px;
  }
  .unlock-options article:first-child {
    border-color: var(--theme-accent);
    background: var(--theme-selected);
  }
  .unlock-options h3 {
    margin: 0;
    font-size: 14px;
  }
  .price {
    margin: 4px 0 8px;
    font-size: 18px;
    font-weight: 700;
  }
  .price small {
    color: var(--theme-muted);
    font-size: 11px;
    font-weight: 400;
  }
  .price-note {
    margin: 4px 0 8px;
    color: var(--theme-muted);
    font-size: 13px;
  }
  .unlock-options ul {
    margin: 0;
    padding-left: 16px;
    font-size: 12px;
    line-height: 1.6;
  }
  @container (max-width: 760px) {
    .exclusive {
      grid-template-columns: minmax(0, 1fr);
    }
  }
  @container (max-width: 620px) {
    .contact {
      grid-template-columns: auto minmax(0, 1fr);
    }
    .contact-actions {
      grid-column: 1 / -1;
      justify-items: start;
    }
  }
</style>
