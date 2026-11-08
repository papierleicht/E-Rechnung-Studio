"""Compare exported Factur-X/ZUGFeRD Go fixtures against original XSDs and unmodified XSLTs.

XRechnung fixtures are checked against the real KoSIT validator by scripts/kosit_parity.py.

Requires Java, Saxon HE jar and Python lxml. This is an offline QA tool only.
Generate fixtures with E_RECHNUNG_STUDIO_FORMAT_TEST_OUTPUT=/tmp/e-rechnung-studio-format-qa go test ./internal/app
  -run 'TestSupportedFormats|TestProfileSpecificFailures|TestBasicDetailsAndImmutablePresentation' -count=1
Run: python3 scripts/verify_format_fixtures.py /tmp/e-rechnung-studio-format-qa /path/to/Saxon-HE.jar
"""
from collections import Counter
from pathlib import Path
import json
import subprocess
import sys
from lxml import etree

root = Path(__file__).resolve().parents[1]
fixtures, saxon = Path(sys.argv[1]).resolve(), Path(sys.argv[2]).resolve()
rules = root / "internal/validation/rules"
ns = {"svrl": "http://purl.oclc.org/dsdl/svrl"}
manifest, cases, schemas = [], [], {}

for result_file in sorted(fixtures.glob("*.json")):
    source = result_file.with_suffix(".xml")
    if not source.exists():
        continue
    name = source.stem
    if not name.startswith("facturx-"):
        continue
    document = etree.parse(str(source))
    profile = next(p for p in ["facturx-basic", "facturx-extended", "facturx-en16931"] if name.startswith(p+"-"))
    level = profile.removeprefix("facturx-").upper()
    folder = rules / profile
    schema = folder / f"Factur-X_1.09.2_{level}.xsd"
    sheets = [folder / f"_XSLT_{level}/FACTUR-X_{level}.xslt"]
    if schema not in schemas:
        schemas[schema] = etree.XMLSchema(etree.parse(str(schema)))
    schemas[schema].assertValid(document)
    outputs = []
    for index, sheet in enumerate(sheets):
        output = fixtures / f"{name}-{index}.svrl"
        manifest.append("\t".join(map(str, [source, sheet, output])))
        outputs.append(output)
    cases.append((result_file, outputs))

manifest_file = fixtures / "reference-manifest.tsv"
manifest_file.write_text("\n".join(manifest))
subprocess.run(["java", "--class-path", str(saxon), str(root / "scripts/ReferenceSchematron.java"), str(manifest_file)], check=True)

for result_file, outputs in cases:
    app = json.loads(result_file.read_text())
    actual = Counter((i["ruleId"], i["severity"]) for i in app["issues"] if i["stage"] == "rules")
    reference = Counter()
    for output in outputs:
        tree = etree.parse(str(output))
        for issue in tree.xpath("//svrl:failed-assert | //svrl:successful-report", namespaces=ns):
            rule = issue.get("id")
            reference[(rule, issue.get("flag") or "error")] += 1
    assert actual == reference, (result_file.name, actual, reference)
print(f"{len(cases)} Factur-X XML fixtures: original XSD valid; Go/Saxon rule-ID and severity multisets identical.")
