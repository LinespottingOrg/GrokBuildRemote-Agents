#!/usr/bin/env bash
# Cross-compile gbr-agent for all supported platforms.
# Run from repo root:  ./scripts/build-all.sh
# Or:                  bash scripts/build-all.sh
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

BINARY="${BINARY:-gbr-agent}"
PKG="${PKG:-./cmd/gbr-agent}"
OUT_DIR="${OUT_DIR:-${ROOT}/dist}"
VERSION="${VERSION:-$(git describe --tags --always --dirty 2>/dev/null || echo dev)}"
COMMIT="$(git rev-parse --short HEAD 2>/dev/null || echo none)"
DATE="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

LDFLAGS=(
  -s -w
  -X "main.version=${VERSION}"
  -X "main.commit=${COMMIT}"
  -X "main.date=${DATE}"
)
LDFLAGS_STR="${LDFLAGS[*]}"

TARGETS=(
  "windows/amd64"
  "darwin/amd64"
  "darwin/arm64"
  "linux/amd64"
  "linux/arm64"
)

if ! command -v go >/dev/null 2>&1; then
  echo "error: go not found on PATH" >&2
  exit 1
fi

if [[ ! -d "${ROOT}/cmd/gbr-agent" && ! -f "${ROOT}/cmd/gbr-agent/main.go" ]]; then
  # Still allow build if package path exists as file tree later
  if [[ ! -e "${ROOT}/cmd/gbr-agent" ]]; then
    echo "warn: ${PKG} not found yet — ensure A1 agent core has created cmd/gbr-agent" >&2
  fi
fi

mkdir -p "$OUT_DIR"
echo "==> building ${BINARY} version=${VERSION} commit=${COMMIT}"
echo "    out=${OUT_DIR}"

for t in "${TARGETS[@]}"; do
  os="${t%/*}"
  arch="${t#*/}"
  ext=""
  [[ "$os" == "windows" ]] && ext=".exe"
  name="${BINARY}-${os}-${arch}${ext}"
  dest="${OUT_DIR}/${name}"

  echo "--> GOOS=${os} GOARCH=${arch} -> ${name}"
  CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" \
    go build -trimpath -ldflags "${LDFLAGS_STR}" -o "$dest" "$PKG"
done

echo "==> checksums"
(
  cd "$OUT_DIR"
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum ${BINARY}-* > SHA256SUMS
  elif command -v shasum >/dev/null 2>&1; then
    shasum -a 256 ${BINARY}-* > SHA256SUMS
  else
    echo "warn: no sha256 tool; skipping SHA256SUMS" >&2
  fi
  ls -la ${BINARY}-* 2>/dev/null || ls -la
)

echo "==> done"
