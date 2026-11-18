/**
 * The move guides are short Markdown texts shipped with the app (and shown on
 * the website). Only the small subset they use is read, into blocks the page
 * renders itself: no HTML from the text ever reaches the page.
 */

export type Inline = { text: string; strong?: boolean; code?: boolean };
export type GuideBlock =
  | { kind: "heading"; level: 2 | 3; text: string }
  | { kind: "paragraph"; parts: Inline[] }
  | { kind: "list"; ordered: boolean; items: Inline[][] }
  | { kind: "checklist"; items: Inline[][] };

/** Reads **bold** and `code`; links keep only their text. */
export function inline(text: string): Inline[] {
  const parts: Inline[] = [];
  const pattern = /\*\*(.+?)\*\*|`([^`]+)`|\[([^\]]+)\]\([^)]*\)/g;
  let last = 0;
  for (const match of text.matchAll(pattern)) {
    if (match.index > last) parts.push({ text: text.slice(last, match.index) });
    if (match[1] !== undefined) parts.push({ text: match[1], strong: true });
    else if (match[2] !== undefined) parts.push({ text: match[2], code: true });
    else parts.push({ text: match[3] });
    last = match.index + match[0].length;
  }
  if (last < text.length) parts.push({ text: text.slice(last) });
  return parts;
}

export function parseGuide(markdown: string): GuideBlock[] {
  const blocks: GuideBlock[] = [];
  let paragraph: string[] = [];
  const flush = () => {
    if (paragraph.length) blocks.push({ kind: "paragraph", parts: inline(paragraph.join(" ")) });
    paragraph = [];
  };
  for (const raw of markdown.replace(/\r\n?/g, "\n").split("\n")) {
    const line = raw.trim();
    const heading = /^(#{2,3})\s+(.*)$/.exec(line);
    const check = /^[-*]\s+\[[ xX]\]\s+(.*)$/.exec(line);
    const bullet = /^[-*]\s+(.*)$/.exec(line);
    const numbered = /^\d+[.)]\s+(.*)$/.exec(line);
    const previous = blocks.at(-1);
    if (!line) {
      flush();
    } else if (heading) {
      flush();
      blocks.push({ kind: "heading", level: heading[1].length as 2 | 3, text: heading[2] });
    } else if (check) {
      flush();
      if (previous?.kind === "checklist" && !paragraph.length)
        previous.items.push(inline(check[1]));
      else blocks.push({ kind: "checklist", items: [inline(check[1])] });
    } else if (bullet || numbered) {
      flush();
      const ordered = !!numbered;
      const text = (numbered ?? bullet)![1];
      if (previous?.kind === "list" && previous.ordered === ordered)
        previous.items.push(inline(text));
      else blocks.push({ kind: "list", ordered, items: [inline(text)] });
    } else if (
      (previous?.kind === "list" || previous?.kind === "checklist") &&
      !paragraph.length &&
      /^\s{2,}\S/.test(raw)
    ) {
      // A continued list item.
      const items = previous.items;
      items[items.length - 1].push(...inline(" " + line));
    } else {
      paragraph.push(line);
    }
  }
  flush();
  return blocks;
}

/** The plain text of inline parts, e.g. for keys and labels. */
export function plain(parts: Inline[]): string {
  return parts.map((part) => part.text).join("");
}
