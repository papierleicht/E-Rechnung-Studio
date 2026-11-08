// Write the offline license catalog as one plain-text file (stdout) for the release bundle.
import { readFileSync } from "node:fs";
import { resolve, dirname } from "node:path";
import { fileURLToPath } from "node:url";

const catalog = JSON.parse(
  readFileSync(resolve(dirname(fileURLToPath(import.meta.url)), "../src/lib/license-catalog.json"), "utf8"),
);
const rule = "=".repeat(78);
const out = ["E-Rechnung-Studio: Lizenzen und Drittanbieterhinweise", `${catalog.length} Einträge`, ""];
let group = "";
for (const entry of catalog) {
  if (entry.group !== group) {
    group = entry.group;
    out.push(rule, group, rule, "");
  }
  out.push(`--- ${entry.name}${entry.version ? ` ${entry.version}` : ""}`, "", entry.text.trim(), "");
}
process.stdout.write(out.join("\n") + "\n");
