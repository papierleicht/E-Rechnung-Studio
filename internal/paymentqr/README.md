# Payment QR encoder

Project-owned Go implementation for the EPC payment QR feature. Production code
uses only the standard library and was written for this application; it does not
copy or wrap `github.com/speedata/barcode` or Nayuki's encoder.

The supported subset is QR Code Model 2, versions 1–13, error correction M and a
single UTF-8 byte-mode segment, at most 331 bytes. This is intentionally not a
general barcode library. It implements segment padding, Reed–Solomon block
generation/interleaving, function patterns, BCH format/version information,
placement, and evaluation of all eight masks. The image has one pixel per module;
the PDF renderer supplies integer scaling and a four-module quiet zone. EPC's
payload character-set marker declares UTF-8, so no QR ECI segment is inserted.

QR format facts are specified by ISO/IEC 18004. DENSO WAVE publishes the
[version/capacity tables](https://www.qrcode.com/en/about/version.html) and
[error correction overview](https://www.qrcode.com/en/about/error_correction.html).

## Independent verification

`testdata/reference.json` contains SHA-256 fingerprints of complete matrices
generated with **Project Nayuki QR-Code-generator v1.8.0**, an independent MIT
implementation. Each fingerprint covers row-major ASCII `0`/`1` modules without
newlines or a quiet zone. Every fixture includes all eight masks at level M,
without error correction boosting. Fixtures exercise every capacity transition,
all 13 supported versions, UTF-8, and the 331-byte maximum. Both the internal
matrix encoder and public image API are checked against these reference outputs.

The Go tests need no external tools, network access, or QR dependency. To
regenerate the fixtures, run:

```sh
python3 internal/paymentqr/testdata/generate_reference.py
go test ./internal/paymentqr
```

The generator fetches only this version-pinned reference into a temporary
directory and verifies its digest before loading it:

- [Reference source](https://github.com/nayuki/QR-Code-generator/blob/v1.8.0/python/qrcodegen.py)
- SHA-256: `b089855caf16185c61421ea4927c1b213cf9468940d71fa8ab11ef83662dcc84`
- [Reference tool license](testdata/NAYUKI-LICENSE.txt)

The reference implementation is not bundled in the product. Its license is
recorded for the provenance of generated test fixtures. The existing barcode
module may still be required by the HTML/PDF renderer; replacing the payment QR
encoder alone does not remove that transitive dependency or its license duties.
