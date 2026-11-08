#!/usr/bin/env bash
# Signs an app bundle with a "Developer ID Application" certificate, submits it
# to Apple's notary service and staples the ticket. The Release workflow runs it
# on the macOS runners; it works on a developer Mac as well.
#
# Usage: build/darwin/sign-and-notarize.sh bin/e-rechnung-studio.app
#
# Environment:
#   APPLE_CERTIFICATE_P12       Base64 of the exported certificate including its
#                               private key (.p12). It is imported into a
#                               temporary keychain that is deleted on exit.
#                               Export it with Keychain Access; a .p12 written by
#                               OpenSSL 3 needs -legacy, or the import fails with
#                               "MAC verification failed".
#   APPLE_CERTIFICATE_PASSWORD  Password of that .p12 file.
#   APPLE_SIGNING_IDENTITY      Optional. Identity to sign with instead of the
#                               first "Developer ID Application" certificate
#                               found; "-" signs ad hoc for local tests.
#   APPLE_API_KEY_P8            Contents of the App Store Connect API key file
#                               (AuthKey_<key id>.p8), role Developer or higher.
#   APPLE_API_KEY_ID            Key ID of that API key.
#   APPLE_API_ISSUER_ID         Issuer ID shown with the team's API keys.
#   SKIP_NOTARIZATION=1         Sign only, for local tests.
set -euo pipefail

app=${1:?usage: sign-and-notarize.sh path/to/App.app}
[ -d "$app/Contents" ] || { echo "$app is not an app bundle" >&2; exit 1; }

tmp=$(mktemp -d)
keychain=""
original_keychains=()
cleanup() {
  if [ -n "$keychain" ]; then
    security list-keychains -d user -s ${original_keychains[@]+"${original_keychains[@]}"} 2>/dev/null || true
    security delete-keychain "$keychain" 2>/dev/null || true
  fi
  rm -rf "$tmp"
}
trap cleanup EXIT

if [ -n "${APPLE_CERTIFICATE_P12:-}" ]; then
  : "${APPLE_CERTIFICATE_PASSWORD:?APPLE_CERTIFICATE_PASSWORD is required with APPLE_CERTIFICATE_P12}"
  while IFS= read -r line; do
    original_keychains+=("$(printf '%s' "$line" | sed -E 's/^[[:space:]]*"//; s/"[[:space:]]*$//')")
  done < <(security list-keychains -d user)
  keychain="$tmp/signing.keychain-db"
  keychain_password=$(uuidgen)
  security create-keychain -p "$keychain_password" "$keychain"
  security set-keychain-settings -lut 21600 "$keychain"
  security unlock-keychain -p "$keychain_password" "$keychain"
  printf '%s' "$APPLE_CERTIFICATE_P12" | base64 --decode > "$tmp/certificate.p12"
  security import "$tmp/certificate.p12" -k "$keychain" -P "$APPLE_CERTIFICATE_PASSWORD" \
    -T /usr/bin/codesign -T /usr/bin/security >/dev/null
  rm -f "$tmp/certificate.p12"
  # Let codesign use the key without a UI prompt and find the keychain.
  security set-key-partition-list -S apple-tool:,apple:,codesign: -s -k "$keychain_password" "$keychain" >/dev/null
  security list-keychains -d user -s "$keychain" ${original_keychains[@]+"${original_keychains[@]}"}
fi

identity=${APPLE_SIGNING_IDENTITY:-}
if [ -z "$identity" ]; then
  if [ -n "$keychain" ]; then
    identity=$(security find-identity -v -p codesigning "$keychain" | awk -F'"' '/Developer ID Application/ {print $2; exit}')
  else
    identity=$(security find-identity -v -p codesigning | awk -F'"' '/Developer ID Application/ {print $2; exit}')
  fi
  if [ -z "$identity" ]; then
    echo 'No "Developer ID Application" identity found. Set APPLE_CERTIFICATE_P12 and APPLE_CERTIFICATE_PASSWORD (plus the APPLE_API_* keys for notarization), or APPLE_SIGNING_IDENTITY for a certificate already in the keychain.' >&2
    exit 1
  fi
fi
echo "Signing $app with: $identity"

# One signature covers the bundle and its main executable; there is no nested
# code. The hardened runtime is required for notarization.
sign_args=(--force --options runtime --sign "$identity")
[ "$identity" = "-" ] || sign_args+=(--timestamp)
codesign "${sign_args[@]}" "$app"
codesign --verify --deep --strict --verbose=2 "$app"

if [ "$identity" = "-" ] || [ "${SKIP_NOTARIZATION:-0}" = 1 ]; then
  echo "Notarization skipped."
  exit 0
fi

: "${APPLE_API_KEY_P8:?APPLE_API_KEY_P8 is required for notarization}"
: "${APPLE_API_KEY_ID:?APPLE_API_KEY_ID is required for notarization}"
: "${APPLE_API_ISSUER_ID:?APPLE_API_ISSUER_ID is required for notarization}"
printf '%s\n' "$APPLE_API_KEY_P8" > "$tmp/AuthKey.p8"
key_args=(--key "$tmp/AuthKey.p8" --key-id "$APPLE_API_KEY_ID" --issuer "$APPLE_API_ISSUER_ID")

ditto -c -k --sequesterRsrc --keepParent "$app" "$tmp/notarize.zip"
echo "Submitting to the notary service..."
xcrun notarytool submit "$tmp/notarize.zip" "${key_args[@]}" --wait --timeout 45m \
  --output-format json > "$tmp/submission.json" || true
cat "$tmp/submission.json"
status=$(sed -nE 's/.*"status" *: *"([^"]+)".*/\1/p' "$tmp/submission.json" | head -1)
submission=$(sed -nE 's/.*"id" *: *"([^"]+)".*/\1/p' "$tmp/submission.json" | head -1)
if [ "$status" != "Accepted" ]; then
  echo "Notarization did not succeed (status: ${status:-unknown})." >&2
  if [ -n "$submission" ]; then
    xcrun notarytool log "$submission" "${key_args[@]}" >&2 || true
  fi
  exit 1
fi

xcrun stapler staple "$app"
xcrun stapler validate "$app"
spctl --assess --type execute --verbose=2 "$app"
echo "Signed, notarized and stapled: $app"
