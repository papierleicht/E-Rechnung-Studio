import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";

test("the shipped Lucide notice matches the installed dependency", () => {
  const shipped = readFileSync(new URL("./lucide-LICENSE.txt", import.meta.url), "utf8");
  const upstream = readFileSync(
    new URL("../../node_modules/@lucide/svelte/LICENSE", import.meta.url),
    "utf8",
  );
  assert.equal(shipped.trim(), upstream.trim());
});

test("the offline catalog identifies Lucide and includes its complete license", () => {
  const catalog = JSON.parse(
    readFileSync(new URL("./license-catalog.json", import.meta.url), "utf8"),
  );
  const pkg = JSON.parse(
    readFileSync(
      new URL("../../node_modules/@lucide/svelte/package.json", import.meta.url),
      "utf8",
    ),
  );
  const entry = catalog.find((item) => item.name === "@lucide/svelte");
  assert.equal(entry.version, pkg.version);
  assert.ok(
    entry.text.includes(
      readFileSync(new URL("./lucide-LICENSE.txt", import.meta.url), "utf8").trim(),
    ),
  );
});
