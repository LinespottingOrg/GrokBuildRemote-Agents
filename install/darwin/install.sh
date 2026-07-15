#!/usr/bin/env bash
# Grok Build Remote — macOS install
set -euo pipefail
PRODUCT="Grok Build Remote"
INSTALL_DIR="${HOME}/.local/bin"
BINARY_NAME="gbr-agent"
REPO="${GBR_REPO:-LinespottingOrg/GrokBuildRemote-Agents}"
VERSION="${GBR_VERSION:-latest}"

echo "==> ${PRODUCT} (macOS)"
mkdir -p "${INSTALL_DIR}" "${HOME}/.gbr"

ARCH="$(uname -m)"
case "${ARCH}" in
  arm64|aarch64) GOARCH=arm64 ;;
  x86_64|amd64)  GOARCH=amd64 ;;
  *) echo "unsupported arch: ${ARCH}"; exit 1 ;;
esac
ASSET="gbr-agent-darwin-${GOARCH}"

download() {
  local url="$1" dest="$2"
  if [[ -n "${GH_TOKEN:-${GITHUB_TOKEN:-}}" ]]; then
    curl -fsSL -H "Authorization: Bearer ${GH_TOKEN:-$GITHUB_TOKEN}" -H "Accept: application/octet-stream" -o "${dest}" "${url}"
  else
    curl -fsSL -o "${dest}" "${url}"
  fi
}

if [[ -n "${GBR_AGENT_PATH:-}" && -f "${GBR_AGENT_PATH}" ]]; then
  cp -f "${GBR_AGENT_PATH}" "${INSTALL_DIR}/${BINARY_NAME}"
elif [[ -f "./dist/${ASSET}" ]]; then
  cp -f "./dist/${ASSET}" "${INSTALL_DIR}/${BINARY_NAME}"
else
  if [[ "${VERSION}" == "latest" ]]; then
    API="https://api.github.com/repos/${REPO}/releases/latest"
    if [[ -n "${GH_TOKEN:-${GITHUB_TOKEN:-}}" ]]; then
      TAG=$(curl -fsSL -H "Authorization: Bearer ${GH_TOKEN:-$GITHUB_TOKEN}" "${API}" | sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p' | head -1)
    else
      TAG=$(curl -fsSL "${API}" | sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p' | head -1)
    fi
    VERSION="${TAG:-latest}"
  fi
  URL="https://github.com/${REPO}/releases/download/${VERSION}/${ASSET}"
  echo "--> download ${URL}"
  download "${URL}" "${INSTALL_DIR}/${BINARY_NAME}"
fi

chmod +x "${INSTALL_DIR}/${BINARY_NAME}"
# PATH
if ! echo ":$PATH:" | grep -q ":${INSTALL_DIR}:"; then
  echo "export PATH=\"${INSTALL_DIR}:\$PATH\"" >> "${HOME}/.zprofile"
  echo "export PATH=\"${INSTALL_DIR}:\$PATH\"" >> "${HOME}/.bash_profile" 2>/dev/null || true
  export PATH="${INSTALL_DIR}:$PATH"
fi

echo "installed: ${INSTALL_DIR}/${BINARY_NAME}"
"${INSTALL_DIR}/${BINARY_NAME}" version || true
echo ""
echo "Next:"
echo "  gbr-agent doctor"
echo "  gbr-agent pair -code YOURCODE"
echo "  gbr-agent service install   # LaunchAgent"
echo "  # Grant Accessibility + Automation for Terminal/iTerm if using UI inject"
