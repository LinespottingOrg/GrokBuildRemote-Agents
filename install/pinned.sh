#!/usr/bin/env bash
# Verify the tagged installer, then run it.
# Copy this file from a git tag (v0.6.0) or from docs/PINNED-INSTALL.md.
# Do not curl the live website as the trust root.
set -euo pipefail
VER="${GBR_VERSION:-v0.6.0}"
SHA="${GBR_INSTALLER_SHA256:-0a7963dc668750bfcb907bb72f6f6f8db30881b02636e417e08e102352309301}"
REPO="${GBR_REPO:-LinespottingOrg/GrokBuildRemote-Agents}"
BASE="https://github.com/${REPO}/releases/download/${VER}"
tmp="$(mktemp "${TMPDIR:-/tmp}/gbr-install.XXXXXX.sh")"
trap 'rm -f "$tmp"' EXIT
if command -v curl >/dev/null 2>&1; then
  curl -fsSL -o "$tmp" "${BASE}/install.sh"
else
  wget -q -O "$tmp" "${BASE}/install.sh"
fi
got=""
if command -v sha256sum >/dev/null 2>&1; then
  got="$(sha256sum "$tmp" | awk '{print $1}')"
elif command -v shasum >/dev/null 2>&1; then
  got="$(shasum -a 256 "$tmp" | awk '{print $1}')"
else
  echo "error: need sha256sum or shasum" >&2
  exit 1
fi
got="$(printf '%s' "$got" | tr 'A-F' 'a-f')"
if [[ "$got" != "$SHA" ]]; then
  echo "error: installer SHA-256 mismatch
  expected ${SHA}
  got      ${got}
  url      ${BASE}/install.sh" >&2
  exit 1
fi
echo "installer checksum ok ${got}"
bash "$tmp" "$@"
