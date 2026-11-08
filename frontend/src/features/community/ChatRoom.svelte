<script lang="ts">
  import { onMount, tick, untrack } from "svelte";
  import SendIcon from "@lucide/svelte/icons/send";
  import LockIcon from "@lucide/svelte/icons/lock";
  import FlagIcon from "@lucide/svelte/icons/flag";
  import TrashIcon from "@lucide/svelte/icons/trash-2";
  import * as ChatAPI from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/app/chatservice";
  import type { ChatMessage } from "../../../bindings/papierleicht.de/e-rechnung-studio/internal/account/models";
  import {
    avatarTone,
    initials,
    mentions,
    mergeMessages,
    sampleMessages,
    timeLabel,
  } from "../../lib/community";
  import { errorDetail } from "../../lib/errors";
  import { cached, remember } from "../../lib/cache";
  import { markSeen } from "../../lib/chatUnread";
  import { fillWindow } from "../../lib/fillWindow";
  import ReportDialog from "./ReportDialog.svelte";

  let {
    demo = false,
    me = "",
    team = false,
    mutedUntil = "",
    draft = $bindable(""),
    online = $bindable(0),
    onlocked,
    onaccesslost,
    confirmDelete,
  }: {
    /** Shows made-up example messages for customers without the chat. */
    demo?: boolean;
    me?: string;
    team?: boolean;
    mutedUntil?: string;
    draft?: string;
    online?: number;
    onlocked?: () => void;
    /** The server no longer lets the customer in, e.g. the plan ended. */
    onaccesslost?: () => void;
    confirmDelete?: () => Promise<boolean>;
  } = $props();

  const POLL = 4000;
  const POLL_AFTER_ERROR = 15000;
  const MAX = 2000;
  // The messages of the last visit show right away; the first poll brings
  // the changes since their cursor.
  type Kept = { messages: ChatMessage[]; cursor: number; hasOlder: boolean; online: number };
  const kept = untrack(() => (demo ? undefined : cached<Kept>("community:chat")));
  let messages: ChatMessage[] = $state(kept?.messages ?? []);
  let cursor = kept?.cursor ?? 0;
  let hasOlder = $state(kept?.hasOlder ?? false);
  let loading = $state(untrack(() => !demo) && !kept);
  let loadingOlder = $state(false);
  let sending = $state(false);
  let error = $state("");
  let reporting: ChatMessage | null = $state(null);
  let list: HTMLDivElement | undefined = $state();
  let input: HTMLTextAreaElement | undefined = $state();
  let timer: ReturnType<typeof setTimeout> | undefined;
  let stopped = false;
  const muted = $derived(!!mutedUntil && new Date(mutedUntil).getTime() > Date.now());

  // Example times relative to now, so the showcase always looks current.
  const samples = sampleMessages.map((m, i) => ({
    ...m,
    seq: i + 1,
    removed: false,
    created: new Date(Date.now() - (sampleMessages.length - i) * 7 * 60_000).toISOString(),
  }));
  const shown = $derived(demo ? samples : messages);

  function nearBottom() {
    return !list || list.scrollHeight - list.scrollTop - list.clientHeight < 80;
  }
  async function scrollToEnd() {
    await tick();
    if (list) list.scrollTop = list.scrollHeight;
  }
  function lostAccess(message: string) {
    return /Community Support und papierleicht Plus|Community-Richtlinien zu|Anmeldung ist abgelaufen/.test(
      message,
    );
  }
  function fail(err: unknown) {
    const message = errorDetail(err);
    if (lostAccess(message)) {
      stopped = true;
      onaccesslost?.();
    }
    error = message;
  }

  function keep() {
    if (demo) return;
    remember<Kept>("community:chat", {
      messages: $state.snapshot(messages),
      cursor,
      hasOlder,
      online,
    });
  }

  async function loadLatest() {
    loading = true;
    try {
      const page = await ChatAPI.Messages(0, 0);
      messages = page.items ?? [];
      cursor = page.cursor;
      hasOlder = page.hasOlder;
      online = page.online;
      keep();
      error = "";
      await scrollToEnd();
    } catch (err) {
      fail(err);
    } finally {
      loading = false;
    }
  }

  async function poll(): Promise<boolean> {
    if (stopped) return true;
    // A hidden window does not need fresh messages; the next poll catches up.
    if (typeof document !== "undefined" && document.visibilityState === "hidden") return true;
    try {
      let more = true;
      let changed = false;
      while (more && !stopped) {
        const page = await ChatAPI.Messages(cursor, 0);
        const stick = nearBottom();
        if (page.items?.length) messages = mergeMessages(messages, page.items);
        changed ||= !!page.items?.length || page.cursor !== cursor || page.online !== online;
        cursor = page.cursor;
        online = page.online;
        more = page.more;
        if (stick && page.items?.length) await scrollToEnd();
      }
      if (changed) keep();
      error = "";
      return true;
    } catch (err) {
      fail(err);
      return false;
    }
  }

  function schedule(delay: number) {
    clearTimeout(timer);
    if (stopped) return;
    timer = setTimeout(async () => schedule((await poll()) ? POLL : POLL_AFTER_ERROR), delay);
  }

  async function loadOlder() {
    if (loadingOlder || !messages.length) return;
    loadingOlder = true;
    const height = list?.scrollHeight ?? 0;
    try {
      const page = await ChatAPI.Messages(0, messages[0].seq);
      messages = mergeMessages(messages, page.items ?? []);
      hasOlder = page.hasOlder;
      keep();
      await tick();
      // Keep the message the reader looked at in place.
      if (list) list.scrollTop += list.scrollHeight - height;
    } catch (err) {
      fail(err);
    } finally {
      loadingOlder = false;
    }
  }

  async function send() {
    if (demo) return onlocked?.();
    const body = draft.trim();
    if (!body || sending || muted) return;
    sending = true;
    try {
      const sent = await ChatAPI.Send(body);
      messages = mergeMessages(messages, [sent]);
      keep();
      draft = "";
      error = "";
      await scrollToEnd();
      input?.focus();
    } catch (err) {
      fail(err);
    } finally {
      sending = false;
    }
  }

  async function remove(message: ChatMessage) {
    if (confirmDelete && !(await confirmDelete())) return;
    try {
      await ChatAPI.Delete(message.id);
      messages = messages.filter((m) => m.id !== message.id);
      keep();
    } catch (err) {
      fail(err);
    }
  }

  function keydown(event: KeyboardEvent) {
    if (event.key === "Enter" && !event.shiftKey && !event.isComposing) {
      event.preventDefault();
      void send();
    }
  }

  // What the chat shows counts as read for the menu badge.
  $effect(() => {
    const last = messages.at(-1);
    if (!demo && last) markSeen(last.seq);
  });

  // A reply prepared elsewhere (e.g. to a listing) lands in the input.
  $effect(() => {
    if (draft && input && !demo) {
      input.focus();
      input.setSelectionRange(draft.length, draft.length);
    }
  });

  onMount(() => {
    if (demo) return;
    if (kept) {
      online = kept.online;
      // Once more after fillWindow has sized the room in the next frame.
      void scrollToEnd().then(() => requestAnimationFrame(() => void scrollToEnd()));
      schedule(0);
    } else void loadLatest().then(() => schedule(POLL));
    const wake = () => {
      if (document.visibilityState === "visible") schedule(0);
    };
    document.addEventListener("visibilitychange", wake);
    return () => {
      stopped = true;
      clearTimeout(timer);
      document.removeEventListener("visibilitychange", wake);
    };
  });
</script>

<section class="chat-room" class:demo aria-label="Community-Chat" use:fillWindow={260}>
  {#if demo}<span class="sample-tag">Beispiel</span>{/if}
  <div
    class="chat-messages"
    bind:this={list}
    role="log"
    aria-live={demo ? "off" : "polite"}
    aria-busy={loading}
  >
    {#if hasOlder && !demo}
      <button type="button" class="text chat-older" disabled={loadingOlder} onclick={loadOlder}
        >{loadingOlder ? "Wird geladen …" : "Ältere Nachrichten laden"}</button
      >
    {/if}
    {#if loading}
      <p class="chat-empty">Nachrichten werden geladen …</p>
    {:else if !shown.length}
      <div class="chat-empty">
        <strong>Noch ist es ruhig hier.</strong>
        <p>Stell dich kurz vor oder stell die erste Frage. Die anderen sehen sie sofort.</p>
      </div>
    {/if}
    {#each shown as message (message.id)}
      <article
        class="chat-message"
        class:mine={message.mine}
        class:team={message.role === "team"}
        class:mentioned={!message.mine && mentions(message.body, me)}
      >
        <span class="chat-avatar tone-{avatarTone(message.author)}" aria-hidden="true"
          >{initials(message.author)}</span
        >
        <div class="chat-bubble">
          <header>
            <strong>{message.mine ? "Du" : message.author}</strong>
            {#if message.role === "team"}<span class="team-badge">papierleicht-Team</span>{/if}
            <time datetime={message.created}>{timeLabel(message.created)}</time>
          </header>
          <p>{message.body}</p>
          {#if !demo}
            <div class="chat-actions">
              {#if message.mine || team}
                <button
                  type="button"
                  class="chat-action"
                  title="Nachricht löschen"
                  aria-label="Nachricht löschen"
                  onclick={() => remove(message)}><TrashIcon size={14} /></button
                >
              {/if}
              {#if !message.mine}
                <button
                  type="button"
                  class="chat-action"
                  title="Nachricht melden"
                  aria-label="Nachricht melden"
                  onclick={() => (reporting = message)}><FlagIcon size={14} /></button
                >
              {/if}
            </div>
          {/if}
        </div>
      </article>
    {/each}
  </div>
  {#if error}<p class="chat-error" role="alert">{error}</p>{/if}
  {#if demo}
    <button type="button" class="chat-locked" onclick={() => onlocked?.()}>
      <LockIcon size={16} aria-hidden="true" />
      <span
        ><strong>Mitlesen und mitschreiben</strong> mit Community Support oder Plus. Echte Nachrichten
        sehen nur Mitglieder.</span
      >
      <span class="chat-locked-cta">Freischalten</span>
    </button>
  {:else if muted}
    <p class="chat-muted" role="status">
      Das Team hat das Schreiben für dich bis {timeLabel(mutedUntil)} Uhr pausiert. Lesen geht weiter.
    </p>
  {:else}
    <form
      class="chat-composer"
      onsubmit={(event) => {
        event.preventDefault();
        void send();
      }}
    >
      <textarea
        bind:this={input}
        bind:value={draft}
        rows="2"
        maxlength={MAX}
        placeholder="Nachricht an die Community … (Enter sendet, Umschalt+Enter für eine neue Zeile)"
        aria-label="Nachricht an die Community"
        onkeydown={keydown}
        disabled={loading}></textarea>
      <div class="chat-send">
        {#if draft.length > MAX - 200}<small>{draft.length} / {MAX}</small>{/if}
        <button
          type="submit"
          class="primary"
          disabled={sending || !draft.trim() || loading}
          aria-label="Senden"><SendIcon size={16} aria-hidden="true" /> Senden</button
        >
      </div>
    </form>
  {/if}
</section>

{#if reporting}
  {@const target = reporting}
  <ReportDialog
    what="Nachricht"
    onclose={() => (reporting = null)}
    onsend={async (reason) => {
      await ChatAPI.Report(target.id, reason);
      reporting = null;
    }}
  />
{/if}

<style>
  .chat-room {
    position: relative;
    display: grid;
    grid-template-rows: minmax(0, 1fr) auto auto;
    /* Until fillWindow has measured: roughly the space below the tiles. */
    height: clamp(260px, calc(100vh - 420px), 760px);
    border: 1px solid var(--theme-border);
    border-radius: 12px;
    background: var(--theme-surface);
    overflow: hidden;
  }
  /* Defensive CSS: the log scrolls on its own (no scroll chaining, no
     layout shift when the scrollbar appears), items shrink and long words
     break. */
  .chat-messages {
    overflow-y: auto;
    overscroll-behavior-y: contain;
    scrollbar-gutter: stable;
    padding: 14px 16px 6px;
    display: flex;
    flex-direction: column;
    gap: 10px;
  }
  .demo .chat-messages {
    mask-image: linear-gradient(to bottom, transparent 0, black 60px);
    user-select: none;
  }
  .demo .chat-message:first-child {
    filter: blur(1.5px);
  }
  .sample-tag {
    position: absolute;
    top: 10px;
    right: 12px;
    z-index: 1;
    padding: 2px 8px;
    border-radius: 999px;
    background: var(--theme-surface-muted);
    border: 1px solid var(--theme-border);
    color: var(--theme-muted);
    font-size: 11px;
    font-weight: 600;
  }
  .chat-older {
    align-self: center;
    font-size: 12px;
  }
  .chat-empty {
    margin: auto;
    max-width: 340px;
    color: var(--theme-muted);
    font-size: 13px;
    text-align: center;
  }
  .chat-empty strong {
    color: var(--theme-text);
  }
  .chat-message {
    display: flex;
    min-width: 0;
    align-items: flex-start;
    gap: 10px;
    max-width: min(640px, 88%);
  }
  .chat-message.mine {
    align-self: flex-end;
    flex-direction: row-reverse;
  }
  .chat-avatar {
    flex: none;
    display: grid;
    place-items: center;
    width: 32px;
    height: 32px;
    border-radius: 10px;
    font-size: 12px;
    font-weight: 700;
  }
  .team .chat-avatar {
    background: var(--theme-primary);
    color: var(--theme-on-primary);
  }
  .chat-bubble {
    position: relative;
    min-width: 0;
    padding: 8px 12px;
    border: 1px solid var(--theme-border);
    border-radius: 4px 12px 12px 12px;
    background: var(--theme-surface-muted);
  }
  .mine .chat-bubble {
    border-radius: 12px 4px 12px 12px;
    border-color: transparent;
    background: var(--theme-selected);
  }
  .team .chat-bubble {
    border-color: color-mix(in srgb, var(--theme-primary) 35%, var(--theme-border));
  }
  .mentioned .chat-bubble {
    box-shadow: inset 3px 0 0 var(--theme-accent);
  }
  .chat-bubble header {
    display: flex;
    min-width: 0;
    flex-wrap: wrap;
    align-items: baseline;
    gap: 4px 8px;
    font-size: 12px;
  }
  .chat-bubble time {
    color: var(--theme-muted);
    font-size: 11px;
  }
  .chat-bubble strong {
    overflow-wrap: anywhere;
  }
  .team-badge {
    padding: 0 6px;
    border-radius: 999px;
    background: var(--theme-primary);
    color: var(--theme-on-primary);
    font-size: 10px;
    font-weight: 600;
    line-height: 16px;
  }
  .chat-bubble p {
    margin: 3px 0 0;
    font-size: 13px;
    line-height: 1.5;
    white-space: pre-wrap;
    overflow-wrap: anywhere;
  }
  .chat-actions {
    position: absolute;
    top: -10px;
    right: 6px;
    display: none;
    gap: 2px;
    padding: 2px;
    border: 1px solid var(--theme-border);
    border-radius: 8px;
    background: var(--theme-surface);
  }
  .mine .chat-actions {
    right: auto;
    left: 6px;
  }
  .chat-message:hover .chat-actions,
  .chat-message:focus-within .chat-actions {
    display: flex;
  }
  .chat-action {
    display: grid;
    place-items: center;
    width: 24px;
    height: 22px;
    padding: 0;
    border: 0;
    border-radius: 6px;
    background: transparent;
    color: var(--theme-muted);
    cursor: pointer;
  }
  .chat-action:hover {
    background: var(--theme-hover);
    color: var(--theme-text);
  }
  .chat-error,
  .chat-muted {
    margin: 0;
    overflow-wrap: anywhere;
    padding: 8px 16px;
    font-size: 12px;
    border-top: 1px solid var(--theme-border);
  }
  .chat-error {
    background: var(--theme-error-bg);
    color: var(--theme-error-text);
  }
  .chat-muted {
    background: var(--theme-warning-bg);
    color: var(--theme-warning-text);
  }
  .chat-composer {
    display: flex;
    align-items: flex-end;
    gap: 10px;
    padding: 10px 12px;
    border-top: 1px solid var(--theme-border);
    background: var(--theme-surface);
  }
  .chat-composer textarea {
    flex: 1;
    min-width: 0;
    min-height: 44px;
    max-height: 160px;
    resize: vertical;
  }
  .chat-send {
    display: grid;
    justify-items: end;
    gap: 4px;
  }
  .chat-send small {
    color: var(--theme-muted);
    font-size: 11px;
  }
  .chat-locked {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 10px;
    margin: 0;
    padding: 12px 16px;
    border: 0;
    border-top: 1px solid var(--theme-border);
    background: var(--theme-surface-muted);
    color: var(--theme-text);
    font: inherit;
    font-size: 13px;
    text-align: left;
    cursor: pointer;
  }
  .chat-locked:hover {
    background: var(--theme-hover);
  }
  .chat-locked :global(svg) {
    flex: none;
    color: var(--theme-accent);
  }
  .chat-locked span:nth-child(2) {
    flex: 1;
    min-width: 0;
  }
  .chat-locked-cta {
    flex: none;
    padding: 6px 12px;
    border-radius: 8px;
    background: var(--theme-primary);
    color: var(--theme-on-primary);
    font-weight: 600;
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
