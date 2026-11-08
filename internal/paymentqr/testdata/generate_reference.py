#!/usr/bin/env python3
"""Regenerate the independent QR matrix fingerprints (Python 3, network access).

The reference implementation is fetched into a temporary directory, verified
against its SHA-256, and never becomes an application or test dependency.
Run from any directory: python3 internal/paymentqr/testdata/generate_reference.py
"""

import hashlib
import importlib.util
import json
from pathlib import Path
import tempfile
import urllib.request

URL = "https://raw.githubusercontent.com/nayuki/QR-Code-generator/v1.8.0/python/qrcodegen.py"
SHA256 = "b089855caf16185c61421ea4927c1b213cf9468940d71fa8ab11ef83662dcc84"


def main():
    source = urllib.request.urlopen(URL, timeout=30).read()
    if hashlib.sha256(source).hexdigest() != SHA256:
        raise RuntimeError("Reference implementation checksum mismatch")
    with tempfile.TemporaryDirectory(prefix="paymentqr-reference-") as directory:
        module_path = Path(directory) / "qrcodegen.py"
        module_path.write_bytes(source)
        spec = importlib.util.spec_from_file_location("qrcodegen", module_path)
        reference = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(reference)
        cases = []
        # At and just above every byte-capacity boundary; v9 -> v10 also
        # exercises the character-count field changing from 8 to 16 bits.
        for length in [0, 1, 14, 15, 26, 27, 42, 43, 62, 63, 84, 85, 106,
                       107, 122, 123, 152, 153, 180, 181, 213, 214, 251,
                       252, 287, 288, 331]:
            payload = "".join(chr(32 + (i * 47 + length) % 95) for i in range(length))
            cases.append((f"bytes-{length}", payload))
        cases.extend([
            ("epc-utf8", "BCD\n002\n1\nSCT\nGENODEF1S01\nMüller & Söhne GmbH\n"
             "DE89370400440532013000\nEUR1234.56\n\n\nRechnung R-2026-42"),
            ("unicode-max", "ä" * 163 + "€AB"),
            ("embedded-zero", "a\x00b\n€"),
        ])
        fixtures = []
        for name, payload in cases:
            hashes = []
            for mask in range(8):
                code = reference.QrCode.encode_segments(
                    [reference.QrSegment.make_bytes(payload.encode("utf-8"))],
                    reference.QrCode.Ecc.MEDIUM, minversion=1, maxversion=13,
                    mask=mask, boostecl=False)
                matrix = "".join("1" if code.get_module(x, y) else "0"
                                 for y in range(code.get_size())
                                 for x in range(code.get_size()))
                hashes.append(hashlib.sha256(matrix.encode("ascii")).hexdigest())
            fixtures.append({"name": name, "payload": payload,
                             "version": code.get_version(), "hashes": hashes})
        Path(__file__).with_name("reference.json").write_text(
            json.dumps(fixtures, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")


if __name__ == "__main__":
    main()
