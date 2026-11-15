import assert from "node:assert/strict";
import test from "node:test";
import { installProblemText, installTargetName } from "./installLocation.ts";

test("names the programs folder on every system", () => {
  assert.equal(installTargetName("/Applications/E-Rechnung-Studio.app"), "Programme");
  assert.equal(installTargetName("/Users/a/Applications/E-Rechnung-Studio.app"), "Programme");
  assert.equal(
    installTargetName(
      "C:\\Users\\a\\AppData\\Local\\Programs\\E-Rechnung-Studio\\e-rechnung-studio.exe",
    ),
    "Programme",
  );
  assert.equal(installTargetName("/home/a/Apps/x.AppImage"), "Apps");
});

test("explains every problem", () => {
  for (const problem of ["translocated", "diskImage", "temporary", "readOnly", ""]) {
    assert.match(installProblemText(problem), /Update/);
  }
});
