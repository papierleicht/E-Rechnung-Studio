/**
 * Reads the German release notes (a small Markdown subset: "## Abschnitt",
 * "- Punkt", paragraphs, **fett**) into blocks the app renders without HTML.
 * Everything after a "---" line (the link to the technical changelog on
 * GitHub) is left out.
 */
export type Span = { text: string; strong: boolean };
export type NotesBlock =
  | { kind: "heading"; text: string }
  | { kind: "list"; items: Span[][] }
  | { kind: "paragraph"; spans: Span[] };

export function spans(text: string): Span[] {
  const out: Span[] = [];
  const parts = text.split("**");
  parts.forEach((part, i) => {
    if (part) out.push({ text: part, strong: i % 2 === 1 && i < parts.length - 1 });
  });
  return out;
}

export function parseReleaseNotes(markdown: string): NotesBlock[] {
  const blocks: NotesBlock[] = [];
  let paragraph: string[] = [];
  const flush = () => {
    if (paragraph.length) blocks.push({ kind: "paragraph", spans: spans(paragraph.join(" ")) });
    paragraph = [];
  };
  for (const raw of (markdown ?? "").replace(/\r\n?/g, "\n").split("\n")) {
    const line = raw.trim();
    if (/^-{3,}$/.test(line)) break;
    const heading = /^#{1,6}\s+(.*)$/.exec(line);
    const item = /^[-*]\s+(.*)$/.exec(line);
    if (heading) {
      flush();
      blocks.push({ kind: "heading", text: heading[1].replace(/\*\*/g, "") });
    } else if (item) {
      flush();
      const last = blocks.at(-1);
      if (last?.kind === "list") last.items.push(spans(item[1]));
      else blocks.push({ kind: "list", items: [spans(item[1])] });
    } else if (!line) {
      flush();
    } else if (blocks.at(-1)?.kind === "list" && raw.startsWith("  ") && !paragraph.length) {
      // A wrapped line continues the last item.
      const list = blocks.at(-1) as { kind: "list"; items: Span[][] };
      list.items[list.items.length - 1].push({ text: " ", strong: false }, ...spans(line));
    } else {
      paragraph.push(line);
    }
  }
  flush();
  return blocks;
}
