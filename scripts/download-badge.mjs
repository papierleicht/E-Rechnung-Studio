// Builds the shields.io endpoint JSON for the download badge in the README.
// Only installers of published releases count: the in-app updater fetches the
// package's .sig file on every update check, SHA256SUMS is a checksum list and
// third-party-licenses.zip is no installer.
// Usage: node scripts/download-badge.mjs RELEASES_JSON
// RELEASES_JSON holds `gh api --paginate --slurp repos/OWNER/REPO/releases`.
import { readFileSync } from "node:fs";
import { pathToFileURL } from "node:url";

const installerPrefix = "e-rechnung-studio-";
const units = ["k", "M", "G"];

export function countInstallerDownloads(releases) {
  let total = 0;
  for (const release of releases.flat()) {
    if (release.draft) continue;
    for (const asset of release.assets)
      if (
        asset.name.startsWith(installerPrefix) &&
        !asset.name.endsWith(".sig")
      )
        total += asset.download_count;
  }
  return total;
}

// Same rounding as the shields.io download badges: 999, 1.2k, 12k, 1.2M.
export function formatCount(count) {
  for (let power = units.length; power >= 1; power--) {
    const scaled = count / 1000 ** power;
    if (scaled < 1) continue;
    if (scaled < 10) {
      const oneDecimal = scaled.toFixed(1);
      if (!oneDecimal.endsWith("0")) return oneDecimal + units[power - 1];
    }
    const rounded = Math.round(scaled);
    if (rounded < 1000 || power === units.length)
      return rounded + units[power - 1];
    return "1" + units[power];
  }
  return String(count);
}

export function downloadBadge(releases) {
  return {
    schemaVersion: 1,
    label: "downloads",
    message: formatCount(countInstallerDownloads(releases)),
    color: "blue",
  };
}

if (
  process.argv[1] &&
  import.meta.url === pathToFileURL(process.argv[1]).href
) {
  try {
    if (process.argv.length !== 3)
      throw new Error("Usage: node scripts/download-badge.mjs RELEASES_JSON");
    const releases = JSON.parse(readFileSync(process.argv[2], "utf8"));
    console.log(JSON.stringify(downloadBadge(releases)));
  } catch (error) {
    console.error(error.message);
    process.exitCode = 1;
  }
}
