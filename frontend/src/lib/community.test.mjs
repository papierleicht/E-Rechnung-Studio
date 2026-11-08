import assert from "node:assert/strict";
import test from "node:test";
import {
  GUIDELINES_VERSION,
  areaOptions,
  avatarTone,
  categoryLabel,
  daysLeftLabel,
  guidelines,
  initials,
  marketCategories,
  mentions,
  mergeMessages,
  nearness,
  sampleListings,
  showNumbers,
  listingCounts,
  pageButtons,
  pageRange,
  timeLabel,
  unreadStep,
} from "./community.ts";

const line = (id, seq, extra = {}) => ({
  id,
  seq,
  author: "Anna",
  role: "member",
  mine: false,
  body: id,
  created: "2026-11-02T10:00:00Z",
  removed: false,
  ...extra,
});

test("changes add, replace and drop messages in order", () => {
  const current = [line("a", 1), line("b", 2)];
  const next = mergeMessages(current, [
    line("c", 3),
    line("a", 1, { removed: true }),
    line("b", 2, { body: "neu" }),
  ]);
  assert.deepEqual(
    next.map((m) => [m.id, m.body]),
    [
      ["b", "neu"],
      ["c", "c"],
    ],
  );
  assert.equal(current.length, 2, "the current list stays unchanged");
  assert.deepEqual(
    mergeMessages([line("z", 9)], [line("y", 4)]).map((m) => m.id),
    ["y", "z"],
  );
});

test("the radius filter offers region and zone of the own postal code", () => {
  assert.deepEqual(
    areaOptions("80331").map((o) => o.value),
    ["", "80", "8"],
  );
  assert.equal(areaOptions("80331")[1].label, "Meine Region (PLZ 80…)");
  assert.deepEqual(areaOptions(""), [{ value: "", label: "Ganz Deutschland" }]);
  assert.deepEqual(areaOptions("A-1010").length, 1);
});

test("nearness names close listings", () => {
  assert.equal(nearness("80331", "80335", false), "ganz in deiner Nähe");
  assert.equal(nearness("80331", "80999", false), "in deiner Region");
  assert.equal(nearness("80331", "10115", true), "auch online");
  assert.equal(nearness("80331", "10115", false), "");
  assert.equal(nearness("", "80331", false), "");
});

test("labels for time, end and categories", () => {
  const now = new Date("2026-11-02T12:00:00Z");
  assert.equal(daysLeftLabel("2026-11-12T12:00:00Z", now), "Noch 10 Tage");
  assert.equal(daysLeftLabel("2026-11-03T11:00:00Z", now), "Endet morgen");
  assert.equal(daysLeftLabel("2026-11-01T12:00:00Z", now), "Beendet");
  assert.equal(daysLeftLabel("kaputt", now), "");
  assert.match(timeLabel("2026-11-02T09:05:00Z", now), /^\d\d:\d\d$/);
  assert.match(timeLabel("2026-11-01T09:05:00Z", now), /^Gestern, /);
  assert.match(timeLabel("2026-10-20T09:05:00Z", now), /^20\.10\.2026, /);
  assert.equal(categoryLabel("it"), "IT & Software");
  assert.equal(categoryLabel("unbekannt"), "Sonstiges");
});

test("mentions and avatars", () => {
  assert.equal(mentions("Danke @anna!", "Anna"), true);
  assert.equal(mentions("Danke Anna!", "Anna"), false);
  assert.equal(mentions("@Anna", ""), false);
  assert.equal(initials("Murat Elektro"), "ME");
  assert.equal(initials("Ida_Shop-Berlin"), "IB");
  assert.equal(initials("lena"), "LE");
  assert.equal(avatarTone("Anna"), avatarTone("Anna"));
  assert.ok([0, 1, 2, 3].includes(avatarTone("Anna")));
});

test("guidelines, categories and examples fit the server", () => {
  assert.match(GUIDELINES_VERSION, /^\d{4}-\d\d-\d\d$/);
  assert.ok(guidelines.length >= 5);
  const ids = marketCategories.map((c) => c.id);
  assert.equal(new Set(ids).size, ids.length);
  // Same list as marketCategories in the license server.
  assert.deepEqual(ids, [
    "handwerk",
    "bau",
    "it",
    "design",
    "marketing",
    "text",
    "beratung",
    "buero",
    "finanzen",
    "transport",
    "gastro",
    "handel",
    "gesundheit",
    "bildung",
    "foto",
    "sonstiges",
  ]);
  for (const l of sampleListings)
    assert.ok(ids.includes(l.category) && /^\d{5}$/.test(l.postalCode));
  assert.equal(showNumbers(null), false);
  assert.equal(showNumbers({ members: 3 }), false);
  assert.equal(showNumbers({ members: 25 }), true);
});

test("the menu badge starts quietly and counts later messages", () => {
  assert.deepEqual(unreadStep(0, { count: 0, latest: 120 }), { mark: 120, count: 0 });
  assert.deepEqual(unreadStep(120, { count: 3, latest: 125 }), { mark: 120, count: 3 });
  assert.deepEqual(unreadStep(125, { count: 0, latest: 125 }), { mark: 125, count: 0 });
  // A mark beyond the newest message (e.g. another server) starts over.
  assert.deepEqual(unreadStep(500, { count: 0, latest: 40 }), { mark: 40, count: 0 });
});

test("the board tile counts in singular and plural", () => {
  assert.equal(listingCounts(1, 0), "1 Angebot · 0 Gesuche");
  assert.equal(listingCounts(3, 1), "3 Angebote · 1 Gesuch");
});

test("the pager shows the ends, the neighbours and gaps", () => {
  assert.deepEqual(pageButtons(1, 1), [1]);
  assert.deepEqual(pageButtons(1, 5), [1, 2, 3, 4, 5]);
  assert.deepEqual(pageButtons(1, 9), [1, 2, 3, 4, "…", 9]);
  assert.deepEqual(pageButtons(4, 9), [1, 2, 3, 4, 5, "…", 9]);
  assert.deepEqual(pageButtons(5, 9), [1, "…", 4, 5, 6, "…", 9]);
  assert.deepEqual(pageButtons(9, 9), [1, "…", 6, 7, 8, 9]);
  assert.deepEqual(pageButtons(5, 7), [1, "…", 4, 5, 6, 7]);
});

test("the pager names the shown range", () => {
  assert.equal(pageRange(1, 24, 0), "");
  assert.equal(pageRange(1, 24, 5), "1–5 von 5");
  assert.equal(pageRange(2, 24, 200), "25–48 von 200");
  assert.equal(pageRange(9, 24, 200), "193–200 von 200");
});
