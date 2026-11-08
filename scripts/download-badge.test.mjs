import test from "node:test";
import assert from "node:assert/strict";
import {
  countInstallerDownloads,
  downloadBadge,
  formatCount,
} from "./download-badge.mjs";

const asset = (name, download_count) => ({ name, download_count });

test("counts installers of published releases across all pages", () => {
  const pages = [
    [
      {
        draft: false,
        assets: [
          asset("e-rechnung-studio-darwin-arm64.zip", 40),
          asset("e-rechnung-studio-darwin-arm64.zip.sig", 600),
          asset("e-rechnung-studio-windows-amd64.zip", 25),
          asset("SHA256SUMS", 900),
          asset("third-party-licenses.zip", 3),
        ],
      },
      {
        draft: true,
        assets: [asset("e-rechnung-studio-linux-amd64.AppImage", 7)],
      },
    ],
    [
      {
        draft: false,
        assets: [asset("e-rechnung-studio-linux-arm64.AppImage", 5)],
      },
    ],
  ];
  assert.equal(countInstallerDownloads(pages), 70);
  assert.equal(countInstallerDownloads([]), 0);
});

test("formats counts like the shields.io download badges", () => {
  for (const [count, text] of [
    [0, "0"],
    [999, "999"],
    [1000, "1k"],
    [1234, "1.2k"],
    [9999, "10k"],
    [12345, "12k"],
    [999499, "999k"],
    [999500, "1M"],
    [1234567, "1.2M"],
  ])
    assert.equal(formatCount(count), text, `count ${count}`);
});

test("builds the endpoint badge JSON", () => {
  const releases = [
    [
      {
        draft: false,
        assets: [asset("e-rechnung-studio-darwin-amd64.zip", 1500)],
      },
    ],
  ];
  assert.deepEqual(downloadBadge(releases), {
    schemaVersion: 1,
    label: "downloads",
    message: "1.5k",
    color: "blue",
  });
});
