// Regenerate the offline catalog from installed dependencies and preserved notices.
import { readFileSync, readdirSync, existsSync } from "node:fs";
import { resolve, dirname } from "node:path";
import { fileURLToPath } from "node:url";
import { execFileSync } from "node:child_process";

const root = resolve(dirname(fileURLToPath(import.meta.url)), "../..");
const entries = [{
  name: "E-Rechnung-Studio",
  version: "",
  group: "Anwendung",
  text: "Copyright 2026 papierleicht.\n\nApplies to the original application code. Third-party components retain their respective licenses.\n\n" + readFileSync(resolve(root, "LICENSE"), "utf8"),
}];
function collect(name, version, group, dir) {
  const files = readdirSync(dir, { withFileTypes: true })
    .filter(
      (f) =>
        (f.isFile() || f.isSymbolicLink()) &&
        /^(licen[cs]e|copying|notice|copyright|third-party)([._-]|$)/i.test(f.name),
    )
    .map((f) => f.name)
    .sort();
  let text = files
    .map((file) => `${file}\n\n${readFileSync(resolve(dir, file), "utf8")}`)
    .join("\n\n────────\n\n");
  if (!text && name === "@wailsio/runtime")
    text = entries.find((e) => e.name === "github.com/wailsapp/wails/v3")?.text ?? "";
  const fallback = {
    "@napi-rs/canvas-darwin-arm64": "frontend/node_modules/@napi-rs/canvas/LICENSE",
    "@rolldown/binding-darwin-arm64": "frontend/node_modules/rolldown/LICENSE",
    "is-reference": "docs/ui-licenses/is-reference.txt",
    "locate-character": "docs/ui-licenses/locate-character.txt",
  }[name];
  if (!text && fallback && existsSync(resolve(root, fallback)))
    text = readFileSync(resolve(root, fallback), "utf8");
  if (!text && existsSync(resolve(dir, "package.json"))) {
    const pkg = JSON.parse(readFileSync(resolve(dir, "package.json"), "utf8"));
    text = `Lizenz laut Paketmetadaten: ${pkg.license ?? "nicht angegeben"}\nAutor: ${typeof pkg.author === "string" ? pkg.author : (pkg.author?.name ?? "nicht angegeben")}\nQuelle: ${pkg.repository?.url ?? pkg.homepage ?? "Paketdokumentation"}\n\nDas veröffentlichte Paket enthält keinen separaten Lizenztext. Dieser Eintrag dokumentiert die vorhandenen Metadaten und ersetzt den fehlenden Originaltext nicht.`;
  }
  if (!text) throw new Error(`Missing license: ${name}`);
  entries.push({ name, version, group, text });
}
// Internal adaptations are no longer Go modules, but their upstream notices
// must still ship in the embedded frontend on every supported platform.
collect(
  "papierleicht Rechnungskern (speedata/einvoice-Ableitung)",
  "Basis 4042b0c17014239cb9da1e1042f5b9951dc1d49a, lokal weiterentwickelt",
  "Go-Komponenten",
  resolve(root, "internal/einvoice"),
);
const goRoot = execFileSync("go", ["env", "GOROOT"], { encoding: "utf8" }).trim();
collect(
  "Go Standardbibliothek",
  execFileSync("go", ["env", "GOVERSION"], { encoding: "utf8" }).trim(),
  "Go-Komponenten",
  existsSync(resolve(goRoot, "LICENSE")) ? goRoot : dirname(goRoot),
);
const modules = execFileSync("go", ["list", "-m", "-json", "all"], { cwd: root, encoding: "utf8" });
for (const module of JSON.parse(`[${modules.trim().replace(/}\s*\{/g, "},{")}]`)) {
  if (module.Main) continue;
  const dir = module.Replace?.Dir ?? module.Dir;
  if (dir && existsSync(dir))
    collect(module.Path, module.Version ?? "lokal", "Go-Komponenten", dir);
}
const lock = JSON.parse(readFileSync(resolve(root, "frontend/package-lock.json"), "utf8"));
for (const [path, pkg] of Object.entries(lock.packages)) {
  if (!path) continue;
  const dir = resolve(root, "frontend", path);
  if (!existsSync(dir)) continue;
  const metadata = JSON.parse(readFileSync(resolve(dir, "package.json"), "utf8"));
  collect(metadata.name, pkg.version, pkg.dev ? "Frontend & Build-Werkzeuge" : "Frontend", dir);
}
for (const folder of ["pdf-licenses", "xml-licenses"]) {
  for (const file of readdirSync(resolve(root, "docs", folder)).sort()) {
    entries.push({
      name: file.replace(/\.txt$/, ""),
      version: "",
      group: folder === "pdf-licenses" ? "PDF & Schriften" : "XML & Validierung",
      text: readFileSync(resolve(root, "docs", folder, file), "utf8"),
    });
  }
}
entries.sort((a, b) => a.group.localeCompare(b.group, "en") || a.name.localeCompare(b.name, "en"));
process.stdout.write(JSON.stringify(entries, null, 2) + "\n");
