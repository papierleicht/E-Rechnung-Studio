// The number next to "Chat & Markt" in the menu: new messages of others
// since the newest one this installation showed. The app asks rarely and
// the question does not count as a visit, so nobody shows as online.
import { writable } from "svelte/store";
import * as ChatAPI from "../../bindings/papierleicht.de/e-rechnung-studio/internal/app/chatservice";
import { unreadStep } from "./community";

const KEY = "papierleicht.chat.seen";
/** Check interval while the community is open to this customer. */
export const UNREAD_EVERY = 60_000;
/** Without access (not signed in, no plan) the app asks much more rarely. */
export const UNREAD_IDLE = 10 * 60_000;

export const chatUnread = writable(0);

let viewing = false;
let timer: ReturnType<typeof setTimeout> | undefined;
let started = false;

function readMark(): number {
  try {
    return Number(localStorage.getItem(KEY)) || 0;
  } catch {
    return 0;
  }
}

function writeMark(seq: number) {
  try {
    localStorage.setItem(KEY, String(seq));
  } catch {
    // Without storage the badge only counts until the next start.
  }
}

/** The chat showed messages up to seq; they are read. */
export function markSeen(seq: number) {
  if (seq > readMark()) writeMark(seq);
  chatUnread.set(0);
}

/** While the chat is on screen the badge stays empty. */
export function setChatViewing(value: boolean) {
  viewing = value;
  if (value) chatUnread.set(0);
}

async function check(): Promise<number> {
  if (viewing) return UNREAD_EVERY;
  try {
    const answer = await ChatAPI.Unread(readMark());
    const next = unreadStep(readMark(), answer);
    if (next.mark !== readMark()) writeMark(next.mark);
    chatUnread.set(viewing ? 0 : next.count);
    return UNREAD_EVERY;
  } catch {
    chatUnread.set(0);
    return UNREAD_IDLE;
  }
}

/** Starts the checks once; later calls check right away (e.g. after sign-in). */
export function watchChatUnread() {
  clearTimeout(timer);
  const loop = async () => {
    timer = setTimeout(loop, await check());
  };
  if (!started) {
    started = true;
    // Give the start of the app a moment before the first request.
    timer = setTimeout(loop, 5_000);
  } else void loop();
}
