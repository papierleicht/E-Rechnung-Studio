"""Compare the app's XRechnung validation against the latest official KoSIT releases.

Downloads the newest itplr-kosit/validator (standalone jar), validator-configuration-xrechnung
and xrechnung-testsuite (cached, sha256-verified) and checks
  1. drift:    bundled scenarios.xml / schematron XSLs equal the latest configuration,
  2. fixtures: the app's own exported XRechnung documents get the same verdict, rule IDs and
               severities from the real KoSIT validator as from the app,
  3. corpus:   the official KoSIT testsuite instances (standard + CIUS technical cases) get the
               same verdict, rule IDs and severities from KoSIT and from the app.
Requires Java, Go and network access (GitHub API). Python stdlib only. QA tool, not shipped.

  E_RECHNUNG_STUDIO_FORMAT_TEST_OUTPUT=/tmp/e-rechnung-studio-format-qa go test ./internal/app -run 'TestSupportedFormats|TestProfileSpecificFailures' -count=1
  python3 scripts/kosit_parity.py /tmp/e-rechnung-studio-format-qa [--cache DIR] [--offline]

Exit code 0 = parity, 1 = drift or mismatch, 2 = could not run.
"""
from collections import Counter
from pathlib import Path
import argparse
import json
import subprocess
import sys
import xml.etree.ElementTree as ET
import zipfile

from parity_common import DEFAULT_CACHE, JAVA, app_validate, build_cli, fetch, root, sha256, workdir

rules = root / "internal/validation/rules/xrechnung"
REP = "{http://www.xoev.de/de/validator/varl/1}"
SCN = "{http://www.xoev.de/de/validator/framework/1/scenarios}"
# bundled file (relative to rules/xrechnung) -> file in the KoSIT configuration zip
BUNDLED = {
    "scenarios.xml": "scenarios.xml",
    "resources/cii/16b/xsl/EN16931-CII-validation.xsl": "resources/cii/16b/xsl/EN16931-CII-validation.xsl",
    "resources/ubl/2.1/xsl/EN16931-UBL-validation.xsl": "resources/ubl/2.1/xsl/EN16931-UBL-validation.xsl",
}
# Corpus folders of the testsuite that the app supports (extension and CVD are not selectable).
CORPUS = ["instances/standard", "instances/technical-cases/cius"]


def kosit_messages(report):
    """(accepted, [(ruleId, effective severity)]) from a KoSIT report.

    The report keeps the raw level in rep:message/@level; the matched scenario's customLevel
    entries override it (rep:custom-level in KoSIT's default-report.xsl), and acceptance uses that.
    """
    tree = ET.parse(report).getroot()
    accepted = tree.find(f"{REP}assessment/{REP}accept") is not None
    custom = {}
    for entry in tree.iter(f"{SCN}customLevel"):
        for code in (entry.text or "").split():
            custom[code] = entry.get("level")
    found = []
    for message in tree.iter(f"{REP}message"):
        code = message.get("code")
        level = custom.get(code) or message.get("level", "error")
        found.append((code, "error" if level == "fatal" else level))
    return accepted, found


def run_kosit(jar, config, out, files):
    subprocess.run([*JAVA, "-jar", str(jar), "-s", str(config / "scenarios.xml"), "-r", str(config), "-o", str(out), *map(str, files)],
                   stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, check=False)


LOCAL_STAGE = "input"  # app-only safeguards (APP-*), not part of the KoSIT ruleset
local_notes = Counter()


def compare(name, app, accepted, reference):
    """Deviations between one app result and one KoSIT report; empty list = parity.

    App-only safeguards (stage "input") are not compared; when KoSIT accepts a document that
    they reject, they are tallied in local_notes and reported, not counted as deviations.
    """
    issues = [i for i in app["issues"] if i["stage"] != LOCAL_STAGE]
    local = [i["ruleId"] for i in app["issues"] if i["stage"] == LOCAL_STAGE and i["severity"] in ("error", "fatal")]
    actual = [(i["ruleId"], "error" if i["severity"] == "fatal" else i["severity"]) for i in issues if i["stage"] == "rules"]
    # KoSIT lists schema findings in the same message list; the app reports them as another stage.
    schema_codes = {i["ruleId"] for i in issues if i["stage"] != "rules"}
    expected = Counter({k: v for k, v in Counter(reference).items() if k[0] not in schema_codes})
    app_accepts = app["state"] in ("valid", "warning") or (
        app["state"] == "invalid" and not any(i["severity"] in ("error", "fatal") for i in issues))
    problems = []
    if app_accepts != accepted:
        problems.append(f"{name}: KoSIT {'accepts' if accepted else 'rejects'}, app state={app['state']}")
    if Counter(actual) != expected:
        problems.append(f"{name}: only app {dict(Counter(actual) - expected)}; only KoSIT {dict(expected - Counter(actual))}")
    if accepted and local:
        local_notes.update(set(local))
    return problems


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("fixtures", type=Path)
    parser.add_argument("--cache", type=Path, default=DEFAULT_CACHE)
    parser.add_argument("--offline", action="store_true", help="use the newest cached KoSIT releases")
    args = parser.parse_args()
    args.cache.mkdir(parents=True, exist_ok=True)

    try:
        jar_tag, jar = fetch("itplr-kosit/validator", lambda n: n.endswith("-standalone.jar"), args.cache, args.offline)
        cfg_tag, cfg_zip = fetch("itplr-kosit/validator-configuration-xrechnung",
                                 lambda n: n.endswith(".zip") and "validator-configuration" in n, args.cache, args.offline)
        suite_tag, suite_zip = fetch("itplr-kosit/xrechnung-testsuite", lambda n: n.endswith(".zip") and "testsuite" in n,
                                     args.cache, args.offline)
    except Exception as err:
        print("could not obtain KoSIT:", err)
        return 2
    print(f"KoSIT validator {jar_tag}, configuration {cfg_tag}, testsuite {suite_tag}")

    failures = []
    work = workdir("kosit-parity-")
    config, suite = work / "config", work / "testsuite"
    for archive_path, target in ((cfg_zip, config), (suite_zip, suite)):
        with zipfile.ZipFile(archive_path) as archive:
            archive.extractall(target)
    cli = build_cli(work)

    for bundled, upstream in BUNDLED.items():
        if sha256(rules / bundled) != sha256(config / upstream):
            failures.append(f"DRIFT {bundled} differs from KoSIT configuration {cfg_tag}")
    for sheet in sorted((rules / "resources/xrechnung").glob("*/xsl/XRechnung-*-validation.xsl")):
        upstream = config / "resources/xrechnung" / sheet.relative_to(rules / "resources/xrechnung")
        if not upstream.exists():
            failures.append(f"DRIFT {sheet.name}: version {sheet.parent.parent.name} missing in configuration {cfg_tag}")
        elif sha256(sheet) != sha256(upstream):
            failures.append(f"DRIFT {sheet.name} differs from KoSIT configuration {cfg_tag}")
    upstream_versions = {p.parent.parent.name for p in (config / "resources/xrechnung").glob("*/xsl/XRechnung-*-validation.xsl")}
    bundled_versions = {p.parent.parent.name for p in (rules / "resources/xrechnung").glob("*/xsl/XRechnung-*-validation.xsl")}
    if upstream_versions != bundled_versions:
        failures.append(f"DRIFT XRechnung versions: bundled {sorted(bundled_versions)}, KoSIT {sorted(upstream_versions)}")

    # 2. the app's own exported documents (verdicts come from the app run that exported them)
    fixtures = sorted(p for p in args.fixtures.glob("xrechnung-*.xml") if p.with_suffix(".json").exists())
    if not fixtures:
        print("no xrechnung-*.xml fixtures with .json in", args.fixtures)
        return 2
    out = work / "out-fixtures"
    run_kosit(jar, config, out, fixtures)
    for source in fixtures:
        report = out / f"{source.stem}-report.xml"
        if not report.exists():
            failures.append(f"{source.name}: KoSIT produced no report")
            continue
        accepted, reference = kosit_messages(report)
        failures += compare(source.name, json.loads(source.with_suffix(".json").read_text()), accepted, reference)

    # 3. official KoSIT testsuite instances
    corpus = sorted(p for folder in CORPUS for p in (suite / folder).rglob("*.xml"))
    if not corpus:
        print("no testsuite instances found in", suite)
        return 2
    out = work / "out-corpus"
    run_kosit(jar, config, out, corpus)
    results = app_validate(cli, corpus)
    for source in corpus:
        report = out / f"{source.stem}-report.xml"
        if not report.exists():
            failures.append(f"testsuite {source.name}: KoSIT produced no report")
            continue
        accepted, reference = kosit_messages(report)
        failures += compare("testsuite " + source.name, results[source], accepted, reference)

    for line in failures:
        print(line)
    if local_notes:
        print("NOTE app-only safeguards reject documents that KoSIT accepts (not a parity failure): "
              + ", ".join(f"{rule} x{count}" for rule, count in sorted(local_notes.items())))
    print(f"{len(fixtures)} fixtures + {len(corpus)} testsuite instances checked against KoSIT validator {jar_tag} / "
          f"configuration {cfg_tag} / testsuite {suite_tag}: " + ("PARITY" if not failures else f"{len(failures)} deviation(s)"))
    return 1 if failures else 0


if __name__ == "__main__":
    sys.exit(main())
