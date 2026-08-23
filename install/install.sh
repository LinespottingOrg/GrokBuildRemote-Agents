#!/usr/bin/env bash
# Build Remote Agent — gbr-agent installer (macOS / Linux / Git Bash)
#
# This file is a GitHub Release asset. The website copy at
# https://grokbuildremote.com/install.sh is a convenience mirror and CAN CHANGE.
# Do not pipe a live URL into bash as a trust root.
#
# Canonical (tag v0.6.0):
#   https://github.com/LinespottingOrg/GrokBuildRemote-Agents/releases/download/v0.6.0/install.sh
# Verify THIS script against the SHA-256 in docs/PINNED-INSTALL.md, then run it.
# The binary it downloads is also SHA-256 checked (SHA256SUMS on that release).
#
# Usage (pinned):
#   See docs/PINNED-INSTALL.md  (download install.sh → checksum → bash)
# Convenience (mutable website URL — humans on OUR site only):
#   curl -fsSL https://grokbuildremote.com/install.sh | bash
#
set -euo pipefail

PRODUCT="Build Remote Agent"
BINARY="gbr-agent"
REPO="${GBR_REPO:-LinespottingOrg/GrokBuildRemote-Agents}"
VERSION="${GBR_VERSION:-v0.6.0}"
SITE="${GBR_SITE:-https://grokbuildremote.com}"
INSTALL_DIR="${GBR_INSTALL_DIR:-}"

die()  { echo "error: $*" >&2; exit 1; }
info() { echo "==> $*"; }
ok()   { echo "    $*"; }

while [[ $# -gt 0 ]]; do
  case "$1" in
    --version|-v) VERSION="$2"; shift 2 ;;
    --dir) INSTALL_DIR="$2"; shift 2 ;;
    --help|-h)
      echo "gbr-agent installer — pin a release. See docs/PINNED-INSTALL.md"
      echo "  --version v0.6.0   --dir ~/.local/bin"
      exit 0
      ;;
    *) shift ;;
  esac
done

case "$VERSION" in
  latest|LATEST|Latest|main|master|HEAD)
    die "refusing mutable version '$VERSION'. Pin GBR_VERSION=v0.6.0 (docs/PINNED-INSTALL.md)"
    ;;
esac
[[ "$VERSION" =~ ^v[0-9]+\.[0-9]+ ]] || die "version must look like v0.6.0 (got '$VERSION')"

# Tagged GitHub release — not the live website /downloads/latest/ tree.
DOWNLOAD_BASE="${GBR_DOWNLOAD_BASE:-https://github.com/${REPO}/releases/download/${VERSION}}"

# Piped curl|bash: $0 is "bash". File run: $0 is the path. Warn either way if stdin is a pipe
# and the operator did not set GBR_I_TRUST_MUTABLE=1.
piped=0
if [[ "${0##*/}" == "bash" || "${0##*/}" == "sh" ]]; then
  piped=1
fi
if [[ "$piped" -eq 1 && "${GBR_I_TRUST_MUTABLE:-0}" != "1" ]]; then
  info "piped installer (mutable URL). Binary SHA-256 is still verified."
  ok "pin this script: https://github.com/${REPO}/blob/${VERSION}/docs/PINNED-INSTALL.md"
fi

detect_os() {
  local u
  u="$(uname -s | tr '[:upper:]' '[:lower:]')"
  case "$u" in
    linux*)  echo "linux" ;;
    darwin*) echo "darwin" ;;
    msys*|mingw*|cygwin*) echo "windows" ;;
    *) die "unsupported OS: $u (supported: linux, darwin, windows via Git Bash)" ;;
  esac
}

detect_arch() {
  local m
  m="$(uname -m)"
  case "$m" in
    x86_64|amd64)  echo "amd64" ;;
    aarch64|arm64) echo "arm64" ;;
    *) die "unsupported architecture: $m (need amd64 or arm64)" ;;
  esac
}

default_install_dir() {
  local os="$1"
  if [[ "$os" == "windows" ]]; then
    if [[ -n "${LOCALAPPDATA:-}" ]]; then
      echo "${LOCALAPPDATA}/GrokBuildRemote"
    else
      echo "${HOME}/.local/bin"
    fi
  else
    echo "${HOME}/.local/bin"
  fi
}

asset_name() {
  local os="$1" arch="$2"
  if [[ "$os" == "windows" ]]; then
    echo "${BINARY}-${os}-${arch}.exe"
  else
    echo "${BINARY}-${os}-${arch}"
  fi
}

download() {
  local url="$1" dest="$2"
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL --retry 3 --retry-delay 1 -o "$dest" "$url"
  elif command -v wget >/dev/null 2>&1; then
    wget -q -O "$dest" "$url"
  else
    die "need curl or wget"
  fi
}

file_sha256() {
  local f="$1" h=""
  if command -v sha256sum >/dev/null 2>&1; then
    h="$(sha256sum "$f" | awk '{print $1}')"
  elif command -v shasum >/dev/null 2>&1; then
    h="$(shasum -a 256 "$f" | awk '{print $1}')"
  elif command -v openssl >/dev/null 2>&1; then
    h="$(openssl dgst -sha256 "$f" | awk '{print $NF}')"
  else
    die "need sha256sum, shasum, or openssl to verify the download"
  fi
  printf '%s' "$h" | tr 'A-F' 'a-f'
}

expected_sha256() {
  local sums="$1" asset="$2"
  awk -v a="$asset" '
    NF>=2 {
      h=$1; n=$2
      sub(/^\*/, "", n)
      if (n==a) { print h; exit }
    }
    NF==1 && length($1)==64 { print $1; exit }
  ' "$sums" | tr 'A-F' 'a-f'
}

verify_download() {
  local bin="$1" asset="$2"
  local dir sums sidecar got want
  dir="$(dirname "$bin")"
  sums="${dir}/SHA256SUMS"
  sidecar="${dir}/${asset}.sha256"
  want=""
  if download "${DOWNLOAD_BASE}/SHA256SUMS" "$sums" 2>/dev/null; then
    want="$(expected_sha256 "$sums" "$asset")"
  fi
  if [[ -z "$want" ]] && download "${DOWNLOAD_BASE}/${asset}.sha256" "$sidecar" 2>/dev/null; then
    want="$(expected_sha256 "$sidecar" "$asset")"
  fi
  [[ -n "$want" ]] || die "no SHA-256 for ${asset} at ${DOWNLOAD_BASE} (need SHA256SUMS or ${asset}.sha256)"
  [[ "$want" =~ ^[0-9a-f]{64}$ ]] || die "malformed checksum for ${asset}"
  got="$(file_sha256 "$bin")"
  if [[ "$got" != "$want" ]]; then
    die "SHA-256 mismatch for ${asset}
    expected ${want}
    got      ${got}
    url      ${DOWNLOAD_BASE}/${asset}"
  fi
  ok "checksum ok ${got}"
}

OS="$(detect_os)"
ARCH="$(detect_arch)"
ASSET="$(asset_name "$OS" "$ARCH")"
if [[ -z "$INSTALL_DIR" ]]; then
  INSTALL_DIR="$(default_install_dir "$OS")"
fi

DEST_NAME="$BINARY"
if [[ "$OS" == "windows" ]]; then
  DEST_NAME="${BINARY}.exe"
fi

URL="${DOWNLOAD_BASE}/${ASSET}"
WORKDIR="$(mktemp -d "${TMPDIR:-/tmp}/gbr-agent.XXXXXX")"
TMP="${WORKDIR}/${ASSET}"
trap 'rm -rf "$WORKDIR"' EXIT

info "${PRODUCT} agent installer"
ok "os=${OS} arch=${ARCH} version=${VERSION}"
ok "download ${URL}"

download "$URL" "$TMP" || die "download failed — check ${SITE}/#download or GitHub Releases ${VERSION}"
[[ -s "$TMP" ]] || die "downloaded file is empty"
verify_download "$TMP" "$ASSET"

mkdir -p "$INSTALL_DIR"
DEST="${INSTALL_DIR}/${DEST_NAME}"
NEW="${DEST}.new.$$"
cp "$TMP" "$NEW"
chmod +x "$NEW" 2>/dev/null || true
mv -f "$NEW" "$DEST"
ok "installed: ${DEST}"

case ":${PATH}:" in
  *":${INSTALL_DIR}:"*) ok "PATH already includes ${INSTALL_DIR}" ;;
  *)
    ok "Add to PATH for this shell:"
    echo "      export PATH=\"${INSTALL_DIR}:\$PATH\""
    if [[ -f "${HOME}/.zshrc" ]]; then
      if ! grep -qF "${INSTALL_DIR}" "${HOME}/.zshrc" 2>/dev/null; then
        echo "export PATH=\"${INSTALL_DIR}:\$PATH\"  # Build Remote Agent" >> "${HOME}/.zshrc"
        ok "appended PATH to ~/.zshrc"
      fi
    elif [[ -f "${HOME}/.bashrc" ]]; then
      if ! grep -qF "${INSTALL_DIR}" "${HOME}/.bashrc" 2>/dev/null; then
        echo "export PATH=\"${INSTALL_DIR}:\$PATH\"  # Build Remote Agent" >> "${HOME}/.bashrc"
        ok "appended PATH to ~/.bashrc"
      fi
    fi
    ;;
esac

echo ""
info "Next commands"
cat <<EOF
  # 1) PC generates QR — phone camera scans it
  ${DEST_NAME} pair
  # browser opens QR; mobile app → Scan QR from computer

  # 2) Run the agent (keep this running)
  ${DEST_NAME} run
  # or auto-start at login (after pair):
  # ${DEST_NAME} service install

  # Useful
  ${DEST_NAME} sessions
  ${DEST_NAME} version   # expect ${VERSION}
  ${DEST_NAME} status

Pinned install: https://github.com/${REPO}/blob/${VERSION}/docs/PINNED-INSTALL.md
Docs: ${SITE}/integrations.html#trust
Support: ${SITE}/support.html
EOF
