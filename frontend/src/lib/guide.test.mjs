import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync, readdirSync } from "node:fs";
import { inline, parseGuide, plain } from "./guide.ts";

test("guide text becomes blocks without HTML", () => {
  const blocks = parseGuide(
    [
      "## Vorher wissen",
      "",
      "- **invoiz** endet <b>bald</b>.",
      "- Zweiter Punkt",
      "  geht weiter.",
      "",
      "## In invoiz",
      "",
      "1. Datei › Exportieren",
      "2. `Dokumente` öffnen",
      "",
      "Ein Absatz",
      "über zwei Zeilen mit [Link](https://example.com).",
      "",
      "- [ ] Erledigt?",
      "- [x] Auch das",
    ].join("\n"),
  );
  assert.deepEqual(
    blocks.map((block) => block.kind),
    ["heading", "list", "heading", "list", "paragraph", "checklist"],
  );
  assert.equal(blocks[1].ordered, false);
  assert.deepEqual(blocks[1].items[0], [
    { text: "invoiz", strong: true },
    { text: " endet <b>bald</b>." },
  ]);
  assert.equal(plain(blocks[1].items[1]), "Zweiter Punkt geht weiter.");
  assert.equal(blocks[3].ordered, true);
  assert.deepEqual(blocks[3].items[1][0], { text: "Dokumente", code: true });
  assert.equal(plain(blocks[4].parts), "Ein Absatz über zwei Zeilen mit Link.");
  assert.equal(blocks[5].items.length, 2);
});

test("inline keeps plain text as it is", () => {
  assert.deepEqual(inline("a * b"), [{ text: "a * b" }]);
});

test("every shipped guide parses into headed sections", () => {
  const root = new URL("../../../internal/umzug/quellen/", import.meta.url);
  for (const id of readdirSync(root)) {
    const blocks = parseGuide(readFileSync(new URL(`${id}/anleitung.md`, root), "utf8"));
    assert.equal(blocks[0].kind, "heading", id);
    assert.ok(
      blocks.some((block) => block.kind === "list" && block.ordered),
      `${id} has steps`,
    );
  }
});
