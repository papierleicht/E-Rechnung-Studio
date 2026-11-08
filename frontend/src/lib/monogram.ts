/** Letter tiles used instead of third-party logos (see docs/third-party-notices.md). */

const stopWords = new Set(["gmbh", "ag", "e", "v", "ev", "und", "and", "the", "de", "com"]);

/** Up to three characters that identify a name without reproducing its mark. */
export function monogramFor(name: string): string {
  const words = name
    .replace(/[^\p{L}\p{N} .&-]/gu, " ")
    .split(/[\s.&/-]+/)
    .filter((word) => word && !stopWords.has(word.toLowerCase()));
  if (!words.length) return "?";
  const [first] = words;
  // Short brand names such as GMX, ING or DKB are shown as they are.
  if (words.length === 1 && first.length <= 3) return first.toUpperCase();
  if (words.length === 1) return first.slice(0, 2).toUpperCase();
  return words
    .slice(0, 2)
    .map((word) => word[0].toUpperCase())
    .join("");
}

/** Stable tone bucket (0–3) so the same name always gets the same tint. */
export function monogramTone(name: string): number {
  let hash = 0;
  for (const character of name) hash = (hash * 31 + character.charCodeAt(0)) >>> 0;
  return hash % 4;
}
