import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
const read = (p) => readFileSync(p, "utf8");
const json = (p) => JSON.parse(read(p));
const version = read("version.txt").trim();
assert.match(version, /^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$/);
const versions = {
  manifest: json(".release-please-manifest.json")["."],
  frontend: json("frontend/package.json").version,
  lockfile: json("frontend/package-lock.json").version,
  lockRoot: json("frontend/package-lock.json").packages[""].version,
  windowsFile: json("build/windows/info.json").fixed.file_version,
  windowsProduct: json("build/windows/info.json").info["0000"].ProductVersion,
  wails: read("build/config.yml").match(
    /version: "([\d.]+)" # x-release-please-version/,
  )?.[1],
  installer: read("build/windows/nsis/wails_tools.nsh").match(
    /INFO_PRODUCTVERSION "([\d.]+)"/,
  )?.[1],
  linuxPackage: read("build/linux/nfpm/nfpm.yaml").match(
    /^version: "([\d.]+)" # x-release-please-version$/m,
  )?.[1],
};
for (const file of ["build/darwin/Info.plist", "build/darwin/Info.dev.plist"]) {
  for (const key of ["CFBundleVersion", "CFBundleShortVersionString"]) {
    versions[`${file}:${key}`] = read(file).match(
      new RegExp(`<key>${key}</key>\\s*<string>([^<]+)</string>`),
    )?.[1];
  }
}
for (const [name, actual] of Object.entries(versions))
  assert.equal(actual, version, `${name} differs from version.txt`);
console.log(`All product versions match ${version}.`);
