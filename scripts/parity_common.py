"""Shared helpers for the official-validator parity scripts (kosit_parity.py, zugferd_parity.py).

Python stdlib only. QA tooling, not shipped with the app.
"""
from concurrent.futures import ThreadPoolExecutor
from pathlib import Path
import hashlib
import json
import os
import subprocess
import sys
import tempfile
import urllib.request

root = Path(__file__).resolve().parents[1]
DEFAULT_CACHE = Path.home() / ".cache/e-rechnung-studio-kosit"
# Options that silence warnings of newer JDKs; each is kept only if the
# installed java accepts it, so an older runtime still starts the validators.
JAVA_OPTIONS = ["--enable-native-access=ALL-UNNAMED", "--enable-final-field-mutation=ALL-UNNAMED"]


def java_command():
    accepted = []
    for option in JAVA_OPTIONS:
        probe = subprocess.run(["java", option, "-version"], capture_output=True, text=True, check=False)
        if probe.returncode == 0 and "Unrecognized option" not in probe.stderr:
            accepted.append(option)
    return ["java", *accepted]


JAVA = java_command()


def api(path):
    request = urllib.request.Request("https://api.github.com/" + path, headers={"Accept": "application/vnd.github+json"})
    if os.environ.get("GITHUB_TOKEN"):
        request.add_header("Authorization", "Bearer " + os.environ["GITHUB_TOKEN"])
    with urllib.request.urlopen(request, timeout=60) as response:
        return json.load(response)


def sha256(path):
    return hashlib.sha256(Path(path).read_bytes()).hexdigest()


def fetch(repo, asset_ok, cache, offline):
    """Download the matching asset of the latest release of repo (cached by tag, sha256-verified)."""
    name = repo.split("/")[1]
    if offline:
        cached = sorted(cache.glob(name + "-*/*"))
        cached = [p for p in cached if asset_ok(p.name)]
        if not cached:
            sys.exit(f"offline: nothing cached for {repo}")
        return cached[-1].parent.name.removeprefix(name + "-"), cached[-1]
    release = api(f"repos/{repo}/releases/latest")
    asset = next(a for a in release["assets"] if asset_ok(a["name"]))
    target = cache / f"{name}-{release['tag_name']}" / asset["name"]
    if not target.exists():
        target.parent.mkdir(parents=True, exist_ok=True)
        urllib.request.urlretrieve(asset["browser_download_url"], target)
    digest = (asset.get("digest") or "").removeprefix("sha256:")
    if digest and sha256(target) != digest:
        target.unlink()
        sys.exit(f"sha256 mismatch for {asset['name']}")
    return release["tag_name"], target


def build_cli(work):
    """Build the app's validate CLI (same validator as the desktop app) and return its path."""
    binary = work / ("e-rechnung-studio-cli" + (".exe" if os.name == "nt" else ""))
    subprocess.run(["go", "build", "-o", str(binary), "./cmd/e-rechnung-studio-cli"], cwd=root, check=True)
    return binary


def app_validate(cli, paths, workers=4):
    """Run the app validator on files; returns {path: result dict} (state, issues)."""
    def one(path):
        run = subprocess.run([str(cli), "validate", str(path)], capture_output=True, text=True, check=False)
        try:
            return path, json.loads(run.stdout)
        except json.JSONDecodeError:
            return path, {"state": "error", "issues": [{"stage": "cli", "ruleId": "CLI", "severity": "error", "message": run.stderr[-300:]}]}
    with ThreadPoolExecutor(workers) as pool:
        return dict(pool.map(one, paths))


def workdir(prefix):
    return Path(tempfile.mkdtemp(prefix=prefix))
