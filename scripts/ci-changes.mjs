import { appendFileSync, readFileSync } from "node:fs";
import { execFileSync } from "node:child_process";
import { pathToFileURL } from "node:url";

// Files the app neither builds, embeds, ships nor tests. A change that touches
// only these skips the build and test steps of CI. Everything else counts as
// relevant, including docs/ (license texts and notices ship with the app).
const NOT_BUILT = [
  /^(README|CHANGELOG|AGENTS|CODE_OF_CONDUCT|SECURITY|SUPPORT|TRADEMARK)\.md$/,
  /^docs\/screenshots\//,
  /^\.github\/ISSUE_TEMPLATE\//,
  /^\.github\/PULL_REQUEST_TEMPLATE\.md$/,
];

export function needsBuild(files) {
  if (files.length === 0) return true;
  return files.some((file) => !NOT_BUILT.some((pattern) => pattern.test(file)));
}

const sha = (value) => /^[a-f0-9]{40}$/.test(value || "");

// The commit range a change covers, or null when it cannot be told: manual
// runs, release builds, new branches and force pushes then build everything.
export function changeRange(event) {
  if (event.pull_request) {
    const base = event.pull_request.base?.sha;
    const head = event.pull_request.head?.sha;
    return sha(base) && sha(head) ? `${base}...${head}` : null;
  }
  if (
    sha(event.before) &&
    sha(event.after) &&
    !/^0{40}$/.test(event.before) &&
    !event.forced
  )
    return `${event.before}..${event.after}`;
  return null;
}

if (
  process.argv[1] &&
  import.meta.url === pathToFileURL(process.argv[1]).href
) {
  const [mode, input] = process.argv.slice(2);
  if (mode !== "--event") {
    console.error("Usage: node scripts/ci-changes.mjs --event EVENT.json");
    process.exit(1);
  }
  let build = true;
  let reason = "no commit range for this event";
  const range = changeRange(JSON.parse(readFileSync(input, "utf8")));
  if (range) {
    try {
      const files = execFileSync("git", ["diff", "--name-only", range], {
        encoding: "utf8",
        stdio: ["ignore", "pipe", "ignore"],
      })
        .split("\n")
        .filter(Boolean);
      build = needsBuild(files);
      reason = build
        ? `${files.length} changed file(s), some of them built or shipped`
        : `only documentation changed: ${files.join(", ")}`;
    } catch {
      reason = `commit range ${range} not available`;
    }
  }
  console.log(`build=${build} (${reason})`);
  if (process.env.GITHUB_OUTPUT)
    appendFileSync(process.env.GITHUB_OUTPUT, `build=${build}\n`);
}
