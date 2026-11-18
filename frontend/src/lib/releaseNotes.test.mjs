import assert from "node:assert/strict";
import test from "node:test";
import { parseReleaseNotes } from "./releaseNotes.ts";

test("reads sections, items, bold text and stops at the changelog link", () => {
  const blocks = parseReleaseNotes(
    "## Neu\n\n- Die App heißt jetzt **papierleicht**.\n- Umzug\n  aus anderen Programmen.\n\n## Behoben\n\nEin Satz\nüber zwei Zeilen.\n\n---\n\nTechnische Änderungen: [CHANGELOG.md](https://example.org)\n",
  );
  assert.deepEqual(blocks, [
    { kind: "heading", text: "Neu" },
    {
      kind: "list",
      items: [
        [
          { text: "Die App heißt jetzt ", strong: false },
          { text: "papierleicht", strong: true },
          { text: ".", strong: false },
        ],
        [
          { text: "Umzug", strong: false },
          { text: " ", strong: false },
          { text: "aus anderen Programmen.", strong: false },
        ],
      ],
    },
    { kind: "heading", text: "Behoben" },
    { kind: "paragraph", spans: [{ text: "Ein Satz über zwei Zeilen.", strong: false }] },
  ]);
});

test("an unclosed ** stays plain text and empty notes give nothing", () => {
  assert.deepEqual(parseReleaseNotes("- a **b"), [
    {
      kind: "list",
      items: [
        [
          { text: "a ", strong: false },
          { text: "b", strong: false },
        ],
      ],
    },
  ]);
  assert.deepEqual(parseReleaseNotes(""), []);
});
