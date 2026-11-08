"""Compare the app's ZUGFeRD / Factur-X validation with the latest Mustang validator.

Mustang (github.com/ZUGFeRD/mustangproject) is the open-source reference validator used in the
ZUGFeRD community; it checks the XML (XSD + Schematron) and, for PDFs, PDF/A-3 conformance
(veraPDF) and the embedded Factur-X XML. The newest release is downloaded (cached,
sha256-verified) and run on
  1. the app's exported Factur-X XML documents: same verdict and rule IDs as the app,
  2. deliberately broken copies of them: both must reject for the same rule IDs,
  3. the app's issued Factur-X PDFs: Mustang must report the PDF as valid.
Requires Java, Go and network access (GitHub API). Python stdlib only. QA tool, not shipped.

  E_RECHNUNG_STUDIO_FORMAT_TEST_OUTPUT=/tmp/e-rechnung-studio-format-qa go test ./internal/app -run 'TestSupportedFormats|TestProfileSpecificFailures' -count=1
  python3 scripts/zugferd_parity.py /tmp/e-rechnung-studio-format-qa [--cache DIR] [--offline]

Exit code 0 = parity, 1 = mismatch, 2 = could not run.
"""
from concurrent.futures import ThreadPoolExecutor
from pathlib import Path
import argparse
import json
import re
import subprocess
import sys
import xml.etree.ElementTree as ET

from parity_common import DEFAULT_CACHE, JAVA, app_validate, build_cli, fetch, workdir

MUTATIONS = {
    "wrong-total": (r"(<ram:GrandTotalAmount>)[^<]*", r"\g<1>1.00"),
    "bad-type-code": (r"(<ram:TypeCode>)\d+(</ram:TypeCode>)", r"\g<1>999\2"),
}
RULE_ID = re.compile(r"\[ID ([A-Za-z0-9-]+)\]")


def mustang(jar, path):
    """(valid, [rule IDs]) of the Mustang verdict for an XML or PDF file."""
    run = subprocess.run([*JAVA, "-jar", str(jar), "--action", "validate", "--no-notices", "--disable-file-logging", "--source", str(path)],
                         capture_output=True, text=True, check=False)
    match = re.search(r"<\?xml.*?</validation>", run.stdout, re.S)
    if not match:
        return None, [run.stderr[-200:] or run.stdout[-200:]]
    tree = ET.fromstring(match.group(0).split("?>", 1)[1])
    errors = [e for e in tree.iter("error")]
    ids = sorted(i for e in errors for i in RULE_ID.findall("".join(e.itertext())))
    return tree.findall("summary")[-1].get("status") == "valid", ids


def app_accepts(result):
    issues = [i for i in result["issues"] if i["stage"] != "input"]
    return result["state"] in ("valid", "warning") or (
        result["state"] == "invalid" and not any(i["severity"] in ("error", "fatal") for i in issues))


def app_rule_ids(result):
    return sorted(i["ruleId"] for i in result["issues"] if i["stage"] == "rules" and i["severity"] in ("error", "fatal"))


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("fixtures", type=Path)
    parser.add_argument("--cache", type=Path, default=DEFAULT_CACHE)
    parser.add_argument("--offline", action="store_true", help="use the newest cached Mustang release")
    args = parser.parse_args()
    args.cache.mkdir(parents=True, exist_ok=True)
    try:
        tag, jar = fetch("ZUGFeRD/mustangproject", lambda n: n.startswith("Mustang-CLI") and n.endswith(".jar"), args.cache, args.offline)
    except Exception as err:
        print("could not obtain Mustang:", err)
        return 2
    print(f"Mustang validator {tag}")

    work = workdir("zugferd-parity-")
    cli = build_cli(work)
    exports = sorted(p for p in args.fixtures.glob("facturx-*.xml")
                     if p.with_suffix(".json").exists() and not p.stem.endswith("-embedded"))
    pdfs = sorted(args.fixtures.glob("facturx-*.pdf"))
    if not exports or not pdfs:
        print("no facturx-*.xml/.pdf fixtures in", args.fixtures)
        return 2

    broken = []
    for source in (p for p in exports if p.stem.endswith("-standard") or p.stem.endswith("-export")):
        for name, (pattern, replacement) in MUTATIONS.items():
            text, count = re.subn(pattern, replacement, source.read_text(), count=1)
            if count:
                target = work / f"{source.stem}-{name}.xml"
                target.write_text(text)
                broken.append(target)
    app_broken = app_validate(cli, broken)

    jobs = [*exports, *broken, *pdfs]
    with ThreadPoolExecutor(4) as pool:
        verdicts = dict(zip(jobs, pool.map(lambda p: mustang(jar, p), jobs)))

    failures = []
    for source in exports:
        valid, ids = verdicts[source]
        app = json.loads(source.with_suffix(".json").read_text())
        if valid is None:
            failures.append(f"{source.name}: Mustang produced no result: {ids}")
        elif valid != app_accepts(app) or ids != app_rule_ids(app):
            failures.append(f"{source.name}: Mustang valid={valid} {ids}; app state={app['state']} {app_rule_ids(app)}")
    for source in broken:
        valid, ids = verdicts[source]
        app = app_broken[source]
        if valid is None:
            failures.append(f"{source.name}: Mustang produced no result: {ids}")
        elif valid or app_accepts(app):
            failures.append(f"{source.name}: broken document accepted (Mustang valid={valid}, app state={app['state']})")
        elif ids and ids != app_rule_ids(app):
            failures.append(f"{source.name}: Mustang rules {ids}; app rules {app_rule_ids(app)}")
    for source in pdfs:
        valid, ids = verdicts[source]
        if valid is not True:
            failures.append(f"{source.name}: Mustang rejects the issued PDF (valid={valid}) {ids}")

    for line in failures:
        print(line)
    print(f"{len(exports)} XML + {len(broken)} broken XML + {len(pdfs)} PDF checked against Mustang {tag}: "
          + ("PARITY" if not failures else f"{len(failures)} deviation(s)"))
    return 1 if failures else 0


if __name__ == "__main__":
    sys.exit(main())
