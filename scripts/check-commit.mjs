import { readFileSync } from "node:fs";
import { execFileSync } from "node:child_process";
import { pathToFileURL } from "node:url";

export function validateMessage(message) {
  const [title, ...body] = message.trim().split(/\r?\n/);
  const match =
    /^(feat|fix|deps|perf|docs|style|refactor|test|build|ci|chore|revert)(?:\([a-z0-9][a-z0-9/-]*\))?(!)?: (\S.*)$/.exec(
      title,
    );
  if (!match)
    throw new Error(
      "Use type(scope): description; allowed types: feat, fix, deps, perf, docs, style, refactor, test, build, ci, chore, revert.",
    );
  if (title.length > 100)
    throw new Error("Commit/PR title must be at most 100 characters.");
  if (match[2] && !/^BREAKING CHANGE: \S.+/m.test(body.join("\n"))) {
    throw new Error(
      "A ! change needs a BREAKING CHANGE: explanation in the commit or PR body.",
    );
  }
}

if (
  process.argv[1] &&
  import.meta.url === pathToFileURL(process.argv[1]).href
) {
  try {
    const [mode, input] = process.argv.slice(2);
    if (mode === "--event") {
      const event = JSON.parse(readFileSync(input, "utf8"));
      if (event.pull_request) {
        validateMessage(
          `${event.pull_request.title}\n\n${event.pull_request.body || ""}`,
        );
      } else {
        const before = event.before;
        if (
          !/^[a-f0-9]{40}$/.test(before || "") ||
          !/^[a-f0-9]{40}$/.test(event.after || "")
        )
          throw new Error("Missing valid push commit range.");
        // A new branch has no predecessor. Include its root commit in the check.
        const range = /^0{40}$/.test(before)
          ? event.after
          : `${before}..${event.after}`;
        const messages = execFileSync(
          "git",
          ["log", "--format=%B%x00", range],
          { encoding: "utf8" },
        )
          .split("\0")
          .map((x) => x.trim())
          .filter(Boolean);
        for (const message of messages) validateMessage(message);
      }
    } else if (mode === "--file") {
      validateMessage(readFileSync(input, "utf8"));
    } else {
      throw new Error(
        "Usage: node scripts/check-commit.mjs --event EVENT.json | --file COMMIT_MESSAGE",
      );
    }
    console.log("Conventional commit policy passed.");
  } catch (error) {
    console.error(error.message);
    process.exitCode = 1;
  }
}
