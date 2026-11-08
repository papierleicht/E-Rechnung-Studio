<script lang="ts">
  import { untrack } from "svelte";
  import MapPinIcon from "@lucide/svelte/icons/map-pin";
  import AtSignIcon from "@lucide/svelte/icons/at-sign";
  import GlobeIcon from "@lucide/svelte/icons/globe";
  import ClockIcon from "@lucide/svelte/icons/clock";
  import PlusIcon from "@lucide/svelte/icons/plus";
  import LockIcon from "@lucide/svelte/icons/lock";
  import MailIcon from "@lucide/svelte/icons/mail";
  import FlagIcon from "@lucide/svelte/icons/flag";
  import ChevronLeftIcon from "@lucide/svelte/icons/chevron-left";
  import ChevronRightIcon from "@lucide/svelte/icons/chevron-right";
  import * as ChatAPI from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/app/chatservice";
  import type {
    ChatPreview,
    MarketListing,
  } from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/account/models";
  import {
    areaOptions,
    avatarTone,
    categoryLabel,
    daysLeftLabel,
    initials,
    marketCategories,
    nearness,
    pageButtons,
    pageRange,
    sampleListings,
  } from "../../lib/community";
  import { errorDetail } from "../../lib/errors";
  import { cached, remember } from "../../lib/cache";
  import CategoryIcon from "./CategoryIcon.svelte";
  import ReportDialog from "./ReportDialog.svelte";
  import ContactDialog from "./ContactDialog.svelte";

  let {
    demo = false,
    postalCode = "",
    preview = null,
    reload = 0,
    oncreate,
    onlocked,
    onaccesslost,
    confirmEnd,
  }: {
    demo?: boolean;
    /** The company's postal code: default area and "in deiner Nähe". */
    postalCode?: string;
    preview?: ChatPreview | null;
    /** Changing the number loads the board again, e.g. after a new listing. */
    reload?: number;
    oncreate: () => void;
    onlocked: () => void;
    onaccesslost?: () => void;
    confirmEnd?: () => Promise<boolean>;
  } = $props();

  // Filters and pages seen before show right away on the next visit, while
  // the board asks the server again.
  type Filter = { kind: string; category: string; area: string; mine: boolean; page: number };
  type Kept = { items: MarketListing[]; total: number; pageSize: number };
  const keyOf = (f: Filter) => `community:market:${JSON.stringify(f)}`;
  const last = untrack(() => (demo ? undefined : cached<Filter>("community:market")));
  let kind = $state(last?.kind ?? "");
  let category = $state(last?.category ?? "");
  // Members see their region first; everyone can widen the area.
  let area = $state(last?.area ?? untrack(() => areaOptions(postalCode)[1]?.value ?? ""));
  let mine = $state(last?.mine ?? false);
  const first = last && cached<Kept>(keyOf(last));
  let listings: MarketListing[] = $state(first?.items ?? []);
  // The server sends one page at a time; a filter starts again at page 1.
  let page = $state(last?.page ?? 1);
  let total = $state(first?.total ?? 0);
  let pageSize = $state(first?.pageSize ?? 24);
  let board: HTMLElement | undefined = $state();
  let loading = $state(untrack(() => !demo) && !first);
  let error = $state("");
  let notice = $state("");
  let reporting: MarketListing | null = $state(null);
  let contacting: MarketListing | null = $state(null);
  let expanded: Record<string, boolean> = $state({});
  const areas = $derived(areaOptions(postalCode));
  const samples = $derived(
    sampleListings.map((s) => ({
      ...s,
      contact: "",
      role: "member",
      mine: false,
      created: "",
      expiresAt: new Date(Date.now() + 21 * 86_400_000).toISOString(),
    })) as MarketListing[],
  );
  const shown = $derived(demo ? samples : listings);
  const realCount = $derived((preview?.offers ?? 0) + (preview?.requests ?? 0));
  const pages = $derived(demo ? 1 : Math.max(1, Math.ceil(total / pageSize)));

  async function load() {
    if (demo) return;
    const filter: Filter = { kind, category, area, mine, page };
    remember("community:market", filter);
    const seen = cached<Kept>(keyOf(filter));
    if (seen) ({ items: listings, total, pageSize } = seen);
    loading = true;
    try {
      const result = await ChatAPI.Listings(filter);
      listings = result.items ?? [];
      // An older server sends everything without a total.
      total = result.total || listings.length;
      pageSize = result.pageSize || Math.max(listings.length, 1);
      // A page past the end (e.g. after ending its last listing) comes back
      // as the last page.
      if (result.page && result.page !== page) page = result.page;
      else remember<Kept>(keyOf(filter), { items: $state.snapshot(listings), total, pageSize });
      error = "";
    } catch (err) {
      error = errorDetail(err);
      if (/Community Support und papierleicht Plus|Community-Richtlinien zu/.test(error))
        onaccesslost?.();
    } finally {
      loading = false;
    }
  }

  function keepPage() {
    remember<Kept>(keyOf({ kind, category, area, mine, page }), {
      items: $state.snapshot(listings),
      total,
      pageSize,
    });
  }

  $effect(() => {
    void [kind, category, area, mine, page, reload];
    void load();
  });

  async function end(listing: MarketListing) {
    if (confirmEnd && !(await confirmEnd())) return;
    try {
      await ChatAPI.DeleteListing(listing.id);
      listings = listings.filter((l) => l.id !== listing.id);
      notice = "Dein Eintrag ist beendet.";
      // Fills the page up again from the next one.
      if (total > pageSize) void load();
      else total = Math.max(total - 1, 0);
      keepPage();
    } catch (err) {
      error = errorDetail(err);
    }
  }

  function goTo(next: number) {
    if (next < 1 || next > pages || next === page) return;
    page = next;
    expanded = {};
    board?.scrollIntoView({ block: "start", behavior: "smooth" });
  }

  async function renew(listing: MarketListing) {
    try {
      const next = await ChatAPI.RenewListing(listing.id);
      listings = listings.map((l) => (l.id === next.id ? next : l));
      keepPage();
      notice = "Dein Eintrag läuft wieder 30 Tage.";
    } catch (err) {
      error = errorDetail(err);
    }
  }
</script>

<section class="market" aria-label="Angebote und Gesuche" bind:this={board}>
  <div class="market-toolbar">
    <div class="market-filters" role="group" aria-label="Einträge filtern">
      <div class="market-kind" role="radiogroup" aria-label="Art">
        {#each [["", "Alle"], ["offer", "Angebote"], ["request", "Gesuche"]] as [value, label]}
          <button
            type="button"
            role="radio"
            aria-checked={kind === value && !mine}
            class:active={kind === value && !mine}
            disabled={demo}
            onclick={() => {
              kind = value;
              mine = false;
              page = 1;
            }}>{label}</button
          >
        {/each}
        {#if !demo}
          <button
            type="button"
            role="radio"
            aria-checked={mine}
            class:active={mine}
            onclick={() => {
              mine = !mine;
              page = 1;
            }}>Meine</button
          >
        {/if}
      </div>
      <label class="market-select"
        ><span>Branche</span><select
          bind:value={category}
          disabled={demo || mine}
          onchange={() => (page = 1)}
        >
          <option value="">Alle Branchen</option>
          {#each marketCategories as c}<option value={c.id}>{c.label}</option>{/each}
        </select></label
      >
      <label class="market-select"
        ><span>Umkreis</span><select
          bind:value={area}
          disabled={demo || mine}
          onchange={() => (page = 1)}
        >
          {#each areas as option}<option value={option.value}>{option.label}</option>{/each}
        </select></label
      >
    </div>
    <button
      type="button"
      class="primary market-create"
      onclick={() => (demo ? onlocked() : oncreate())}
      >{#if demo}<LockIcon size={16} aria-hidden="true" />{:else}<PlusIcon
          size={16}
          aria-hidden="true"
        />{/if}<span>Eintrag veröffentlichen</span></button
    >
  </div>

  {#if demo}
    <button type="button" class="market-teaser" onclick={onlocked}>
      <LockIcon size={18} aria-hidden="true" />
      <span class="market-teaser-text"
        ><strong
          >{realCount > 0
            ? `${realCount} echte ${realCount === 1 ? "Eintrag wartet" : "Einträge warten"}`
            : "Sei unter den Ersten"}</strong
        ><small
          >{#if preview?.categories?.length}Gerade gefragt: {preview.categories
              .map((c) => categoryLabel(c.category))
              .join(" · ")}{:else}Exklusiv für Mitglieder mit Community Support oder Plus. Die
            Karten unten sind Beispiele.{/if}</small
        ></span
      >
      <span class="market-teaser-cta">Freischalten</span>
    </button>
  {/if}

  {#if error}<p class="market-feedback error" role="alert">{error}</p>{/if}
  {#if notice}<p class="market-feedback" role="status">{notice}</p>{/if}

  {#if shown.length}
    <ul class="market-grid" aria-busy={loading}>
      {#each shown as listing (listing.id)}
        {@const near = nearness(postalCode, listing.postalCode, listing.remote)}
        <li>
          <article class="listing" class:request={listing.kind === "request"} class:sample={demo}>
            <header>
              <span class="listing-kind">{listing.kind === "offer" ? "Angebot" : "Gesuch"}</span>
              <span class="listing-category"
                ><CategoryIcon category={listing.category} size={14} /><span
                  >{categoryLabel(listing.category)}</span
                ></span
              >
              {#if demo}<span class="sample-tag">Beispiel</span>{/if}
            </header>
            <h3 title={listing.title}>{listing.title}</h3>
            {#if listing.body}
              <p class="listing-body" class:open={expanded[listing.id]}>{listing.body}</p>
              {#if listing.body.length > 140 && !expanded[listing.id]}
                <button
                  type="button"
                  class="text listing-more"
                  onclick={() => (expanded = { ...expanded, [listing.id]: true })}
                  >Mehr lesen</button
                >
              {/if}
            {/if}
            <ul class="listing-meta">
              <li>
                <MapPinIcon size={13} aria-hidden="true" /><span
                  >{listing.postalCode} {listing.city}</span
                >
              </li>
              {#if near}<li class="near">
                  {#if listing.remote && near === "auch online"}<GlobeIcon
                      size={13}
                      aria-hidden="true"
                    />{/if}<span>{near}</span>
                </li>{:else if listing.remote}<li>
                  <GlobeIcon size={13} aria-hidden="true" /><span>auch online</span>
                </li>{/if}
              {#if listing.contact}<li class="contact" title={listing.contact}>
                  <AtSignIcon size={13} aria-hidden="true" /><span>{listing.contact}</span>
                </li>{/if}
            </ul>
            <footer>
              <span class="listing-author">
                <span class="avatar tone-{avatarTone(listing.author)}" aria-hidden="true"
                  >{initials(listing.author)}</span
                >
                <span class="listing-author-name"
                  >{listing.mine ? "Du" : listing.author}{#if listing.role === "team"}
                    · Team{/if}</span
                >
              </span>
              <span class="listing-days"
                ><ClockIcon size={12} aria-hidden="true" />{daysLeftLabel(listing.expiresAt)}</span
              >
            </footer>
            <div class="listing-actions">
              {#if listing.mine}
                <button type="button" class="secondary" onclick={() => renew(listing)}
                  >Verlängern</button
                >
                <button type="button" class="text danger-text" onclick={() => end(listing)}
                  >Beenden</button
                >
              {:else}
                <button
                  type="button"
                  class="secondary"
                  onclick={() => (demo ? onlocked() : (contacting = listing))}
                  ><MailIcon size={14} aria-hidden="true" /> Anfrage senden</button
                >
                {#if !demo}
                  <button
                    type="button"
                    class="icon-button listing-report"
                    title="Eintrag melden"
                    aria-label="Eintrag melden"
                    onclick={() => (reporting = listing)}><FlagIcon size={14} /></button
                  >
                {/if}
              {/if}
            </div>
          </article>
        </li>
      {/each}
    </ul>
    {#if pages > 1}
      <nav class="market-pager" aria-label="Seiten der Einträge">
        <span class="market-range">{pageRange(page, pageSize, total)}</span>
        <div class="market-pages">
          <button
            type="button"
            class="market-page"
            aria-label="Vorherige Seite"
            title="Vorherige Seite"
            disabled={page <= 1 || loading}
            onclick={() => goTo(page - 1)}><ChevronLeftIcon size={16} aria-hidden="true" /></button
          >
          {#each pageButtons(page, pages) as n, i (`${n}-${i}`)}
            {#if n === "…"}
              <span class="market-gap" aria-hidden="true">…</span>
            {:else}
              <button
                type="button"
                class="market-page"
                class:active={n === page}
                aria-current={n === page ? "page" : undefined}
                aria-label="Seite {n}"
                disabled={loading}
                onclick={() => goTo(n)}>{n}</button
              >
            {/if}
          {/each}
          <button
            type="button"
            class="market-page"
            aria-label="Nächste Seite"
            title="Nächste Seite"
            disabled={page >= pages || loading}
            onclick={() => goTo(page + 1)}><ChevronRightIcon size={16} aria-hidden="true" /></button
          >
        </div>
      </nav>
    {/if}
  {:else if !demo && !loading && !error}
    <div class="market-empty">
      <p>
        {mine
          ? "Du hast gerade keine Einträge. Veröffentliche dein erstes Angebot oder Gesuch."
          : area
            ? "In diesem Umkreis gibt es gerade nichts. Wähle „Ganz Deutschland“ oder veröffentliche selbst etwas."
            : "Noch keine Einträge. Mach den Anfang!"}
      </p>
      <button type="button" class="secondary" onclick={oncreate}>Eintrag veröffentlichen</button>
    </div>
  {/if}
</section>

{#if contacting}
  {@const target = contacting}
  <ContactDialog
    listing={target}
    onclose={() => (contacting = null)}
    onsend={async (message) => {
      // target follows contacting, so read it before the dialog closes.
      const { id, author } = target;
      await ChatAPI.ContactListing(id, message);
      contacting = null;
      error = "";
      notice = `Deine Anfrage an ${author} ist unterwegs. Die Antwort kommt per E-Mail.`;
    }}
  />
{/if}

{#if reporting}
  {@const target = reporting}
  <ReportDialog
    what="Eintrag"
    onclose={() => (reporting = null)}
    onsend={async (reason) => {
      await ChatAPI.ReportListing(target.id, reason);
      reporting = null;
      notice = "Danke, wir sehen uns den Eintrag an.";
    }}
  />
{/if}

<style>
  /* Defensive CSS: rows wrap, flex and grid items may shrink below their
     content (min-width: 0), long words break, and nothing has a fixed
     height. See https://defensivecss.dev. */
  .market {
    container-type: inline-size;
    display: grid;
    gap: 12px;
  }
  .market-toolbar {
    display: flex;
    flex-wrap: wrap;
    align-items: flex-end;
    gap: 10px 16px;
  }
  .market-filters {
    display: flex;
    flex: 1 1 32rem;
    flex-wrap: wrap;
    align-items: flex-end;
    gap: 10px 14px;
    min-width: 0;
  }
  .market-kind {
    display: inline-flex;
    flex-wrap: wrap;
    padding: 3px;
    border: 1px solid var(--theme-border);
    border-radius: 10px;
    background: var(--theme-surface);
  }
  .market-kind button {
    padding: 5px 12px;
    border: 0;
    border-radius: 7px;
    background: transparent;
    color: var(--theme-muted);
    font: inherit;
    font-size: 13px;
    white-space: nowrap;
    cursor: pointer;
  }
  .market-kind button.active {
    background: var(--theme-selected);
    color: var(--theme-text);
    font-weight: 600;
  }
  .market-kind button:disabled {
    cursor: default;
  }
  .market-select {
    display: grid;
    flex: 1 1 9.5rem;
    gap: 3px;
    min-width: 0;
    max-width: 16rem;
    font-size: 12px;
    color: var(--theme-muted);
  }
  .market-select select {
    width: 100%;
    min-width: 0;
  }
  /* margin-left: auto keeps the button on the right, also when it wraps
     into its own row (instead of justify-content: space-between). */
  .market-create {
    display: inline-flex;
    flex: none;
    margin-left: auto;
    align-items: center;
    gap: 6px;
    min-width: max-content;
    white-space: nowrap;
  }
  .market-teaser {
    display: flex;
    align-items: center;
    gap: 12px;
    width: 100%;
    padding: 12px 14px;
    border: 1px dashed var(--theme-accent);
    border-radius: 12px;
    background: var(--theme-selected);
    color: var(--theme-text);
    font: inherit;
    text-align: left;
    cursor: pointer;
  }
  .market-teaser > :global(svg) {
    flex: none;
    color: var(--theme-accent);
  }
  .market-teaser-text {
    display: grid;
    flex: 1;
    gap: 2px;
    min-width: 0;
  }
  .market-teaser-text small {
    color: var(--theme-muted);
    font-size: 12px;
    line-height: 1.45;
  }
  .market-teaser-cta {
    flex: none;
    padding: 6px 12px;
    border-radius: 8px;
    background: var(--theme-primary);
    color: var(--theme-on-primary);
    font-size: 13px;
    font-weight: 600;
  }
  @container (max-width: 520px) {
    .market-teaser {
      flex-wrap: wrap;
    }
  }
  .market-feedback {
    margin: 0;
    padding: 8px 12px;
    border-radius: 8px;
    background: var(--theme-success-bg);
    color: var(--theme-success-text);
    font-size: 13px;
    overflow-wrap: anywhere;
  }
  .market-feedback.error {
    background: var(--theme-error-bg);
    color: var(--theme-error-text);
  }
  /* auto-fill keeps a lone card at card width instead of stretching it over
     the whole row; min() lets the column shrink below 17rem when narrow. */
  .market-grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(min(17rem, 100%), 1fr));
    gap: 10px;
    margin: 0;
    padding: 0;
    list-style: none;
  }
  .market-grid > li {
    display: grid;
    min-width: 0;
  }
  .market-pager {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 8px 16px;
    padding-top: 4px;
  }
  .market-range {
    color: var(--theme-muted);
    font-size: 13px;
    font-variant-numeric: tabular-nums;
  }
  .market-pages {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 4px;
    margin-left: auto;
  }
  .market-page {
    display: inline-grid;
    place-items: center;
    min-width: 32px;
    min-height: 32px;
    padding: 0 8px;
    border: 1px solid var(--theme-border);
    border-radius: 8px;
    background: var(--theme-surface);
    color: var(--theme-text);
    font: inherit;
    font-size: 13px;
    font-variant-numeric: tabular-nums;
    cursor: pointer;
  }
  .market-page.active {
    border-color: var(--theme-accent);
    background: var(--theme-selected);
    font-weight: 600;
  }
  .market-page:disabled {
    cursor: default;
    opacity: 0.5;
  }
  .market-page.active:disabled {
    opacity: 1;
  }
  .market-gap {
    min-width: 20px;
    color: var(--theme-muted);
    text-align: center;
  }
  .listing {
    display: flex;
    flex-direction: column;
    gap: 8px;
    min-width: 0;
    padding: 14px;
    border: 1px solid var(--theme-border);
    border-radius: 12px;
    background: var(--theme-surface);
  }
  .listing header {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 6px;
    min-width: 0;
  }
  .listing-kind {
    flex: none;
    padding: 1px 8px;
    border-radius: 999px;
    background: var(--theme-success-bg);
    color: var(--theme-success-text);
    font-size: 11px;
    font-weight: 700;
    letter-spacing: 0.02em;
    text-transform: uppercase;
  }
  .request .listing-kind {
    background: var(--theme-info-bg);
    color: var(--theme-info-text);
  }
  .listing-category {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    min-width: 0;
    color: var(--theme-muted);
    font-size: 12px;
  }
  .listing-category > span {
    overflow: hidden;
    white-space: nowrap;
    text-overflow: ellipsis;
  }
  .listing-category :global(svg) {
    flex: none;
  }
  .sample-tag {
    margin-left: auto;
    padding: 0 7px;
    border: 1px solid var(--theme-border);
    border-radius: 999px;
    color: var(--theme-muted);
    font-size: 10px;
    font-weight: 600;
    line-height: 16px;
  }
  .listing h3 {
    display: -webkit-box;
    margin: 0;
    overflow: hidden;
    -webkit-box-orient: vertical;
    -webkit-line-clamp: 2;
    line-clamp: 2;
    font-size: 15px;
    line-height: 1.3;
    overflow-wrap: anywhere;
  }
  .listing-body {
    display: -webkit-box;
    margin: 0;
    overflow: hidden;
    -webkit-box-orient: vertical;
    -webkit-line-clamp: 3;
    line-clamp: 3;
    color: var(--theme-muted);
    font-size: 13px;
    line-height: 1.5;
    white-space: pre-wrap;
    overflow-wrap: anywhere;
  }
  .listing-body.open {
    display: block;
  }
  .listing-more {
    align-self: flex-start;
    padding: 0;
    font-size: 12px;
  }
  /* Meta, footer and actions sit at the bottom, so cards of one row line up. */
  .listing-meta {
    display: flex;
    flex-wrap: wrap;
    gap: 4px 12px;
    margin: auto 0 0;
    padding: 0;
    list-style: none;
    font-size: 12px;
    color: var(--theme-muted);
  }
  .listing-meta li {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    min-width: 0;
    max-width: 100%;
  }
  .listing-meta li :global(svg) {
    flex: none;
  }
  .listing-meta li > span {
    overflow: hidden;
    white-space: nowrap;
    text-overflow: ellipsis;
  }
  .listing-meta .near {
    color: var(--theme-accent);
    font-weight: 600;
  }
  .listing-meta .contact {
    flex-basis: 100%;
    color: var(--theme-text);
    user-select: text;
  }
  .listing footer {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 6px 12px;
    padding-top: 8px;
    border-top: 1px solid var(--theme-border);
    font-size: 12px;
  }
  .listing-author {
    display: inline-flex;
    flex: 1 1 8rem;
    align-items: center;
    gap: 6px;
    min-width: 0;
  }
  .listing-author-name {
    overflow: hidden;
    white-space: nowrap;
    text-overflow: ellipsis;
  }
  .avatar {
    display: grid;
    place-items: center;
    flex: none;
    width: 22px;
    height: 22px;
    border-radius: 7px;
    font-size: 10px;
    font-weight: 700;
  }
  .listing-days {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    color: var(--theme-muted);
    white-space: nowrap;
  }
  .listing-actions {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 8px;
  }
  .listing-actions .secondary {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    min-width: max-content;
  }
  .listing-report {
    margin-left: auto;
  }
  .sample {
    user-select: none;
  }
  .market-empty {
    display: grid;
    justify-items: center;
    gap: 10px;
    padding: 28px 16px;
    border: 1px dashed var(--theme-border);
    border-radius: 12px;
    color: var(--theme-muted);
    font-size: 13px;
    text-align: center;
  }
  .market-empty p {
    max-width: 32rem;
    margin: 0;
  }
  .tone-0 {
    background: var(--theme-success-bg);
    color: var(--theme-success-text);
  }
  .tone-1 {
    background: var(--theme-info-bg);
    color: var(--theme-info-text);
  }
  .tone-2 {
    background: var(--theme-warning-bg);
    color: var(--theme-warning-text);
  }
  .tone-3 {
    background: var(--theme-surface-muted);
    color: var(--theme-accent);
    box-shadow: inset 0 0 0 1px var(--theme-border);
  }
</style>
