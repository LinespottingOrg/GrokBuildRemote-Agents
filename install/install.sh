#!/usr/bin/env bash
# Grok Build Remote — gbr-agent installer
# Detects OS/arch, downloads the latest free binary, installs to a user or system path.
#
# Usage:
#   curl -fsSL https://grokbuildremote.com/install.sh | bash
#   # or, from a clone:
#   ./install/install.sh
#
# Env overrides:
#   GBR_VERSION          Pin version (e.g. v0.1.0). Default: latest release tag.
#   GBR_INSTALL_DIR      Install directory. Defaults below.
#   GBR_DOWNLOAD_BASE    Base URL for binaries (no trailing slash).
#                        Default: public free-binary CDN / GitHub Releases mirror.
#   GBR_REPO             owner/repo for GitHub Releases API (private source; public assets).
#   GBR_SKIP_SERVICE     If set to 1, do not print service-install hints.
#
# Binary name: gbr-agent
set -euo pipefail

PRODUCT="Grok Build Remote"
BINARY="gbr-agent"
REPO="${GBR_REPO:-LinespottingOrg/GrokBuildRemote-Agents}"
# Free binaries are published for end users (website + releases). Source stays private.
DEFAULT_CDN="https://github.com/${REPO}/releases/download"
DOWNLOAD_BASE="${GBR_DOWNLOAD_BASE:-$DEFAULT_CDN}"

die()  { echo "error: $*" >&2; exit 1; }
info() { echo "==> $*"; }
ok()   { echo "    $*"; }

need_cmd() {
  command -v "$1" >/dev/null 2>&1 || die "required command not found: $1"
}

detect_os() {
  local u
  u="$(uname -s | tr '[:upper:]' '[:lower:]')"
  case "$u" in
    linux*)  echo "linux" ;;
    darwin*) echo "darwin" ;;
    msys*|mingw*|cygwin*) echo "windows" ;;
    *) die "unsupported OS: $u (supported: linux, darwin, windows)" ;;
  esac
}

detect_arch() {
  local m
  m="$(uname -m)"
  case "$m" in
    x86_64|amd64)  echo "amd64" ;;
    aarch64|arm64) echo "arm64" ;;
    armv7l|armv6l) die "32-bit ARM is not supported; use amd64 or arm64" ;;
    i386|i686)     die "32-bit x86 is not supported" ;;
    *) die "unsupported architecture: $m" ;;
  esac
}

default_install_dir() {
  local os="$1"
  case "$os" in
    windows)
      # Git-Bash / MSYS path for Program Files when elevated; else user LocalAppData.
      if [[ -n "${PROGRAMFILES:-}" ]]; then
        # Prefer user-local when not admin (no write to Program Files).
        if [[ -w "${PROGRAMFILES}" ]] 2>/dev/null; then
          echo "${PROGRAMFILES}/GrokBuildRemote"
        else
          echo "${LOCALAPPDATA:-$HOME/AppData/Local}/GrokBuildRemote"
        fi
      else
        echo "${HOME}/.local/bin"
      fi
      ;;
    *)
      echo "${HOME}/.local/bin"
      ;;
  esac
}

latest_tag() {
  # Prefer GitHub API; fall back to "latest" path segment if API unavailable.
  if command -v curl >/dev/null 2>&1; then
    local tag
    tag="$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" \
      | sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' \
      | head -n1 || true)"
    if [[ -n "${tag}" ]]; then
      echo "${tag}"
      return
    fi
  fi
  echo "latest"
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
    die "need curl or wget to download ${url}"
  fi
}

install_binary() {
  local src="$1" dest_dir="$2" dest_name="$3"
  mkdir -p "$dest_dir"
  # Atomic-ish replace
  local tmp="${dest_dir}/.${dest_name}.new.$$"
  cp "$src" "$tmp"
  chmod 755 "$tmp"
  mv -f "$tmp" "${dest_dir}/${dest_name}"
}

print_path_hint() {
  local dir="$1" os="$2"
  case "$os" in
    windows)
      ok "Ensure this directory is on your PATH (User environment variables):"
      ok "  ${dir}"
      ;;
    *)
      if ! echo ":${PATH}:" | grep -q ":${dir}:"; then
        ok "Add to PATH (e.g. in ~/.bashrc or ~/.zshrc):"
        ok "  export PATH=\"${dir}:\$PATH\""
      fi
      ;;
  esac
}

print_service_hints() {
  local os="$1" install_dir="$2"
  [[ "${GBR_SKIP_SERVICE:-0}" == "1" ]] && return
  echo
  info "Optional: run as a background service"
  case "$os" in
    linux)
      ok "systemd unit: install/linux/gbr-agent.service"
      ok "  sudo cp install/linux/gbr-agent.service /etc/systemd/system/"
      ok "  sudo systemctl daemon-reload && sudo systemctl enable --now gbr-agent"
      ;;
    darwin)
      ok "launchd example: install/darwin/launchd.plist.example"
      ok "  cp install/darwin/launchd.plist.example ~/Library/LaunchAgents/com.linespotting.gbr-agent.plist"
      ok "  # edit ProgramArguments path, then:"
      ok "  launchctl load ~/Library/LaunchAgents/com.linespotting.gbr-agent.plist"
      ;;
    windows)
      ok "WinSW docs: install/windows/service.md"
      ok "  Sample XML: install/windows/gbr-agent.xml"
      ok "  Binary installed at: ${install_dir}\\${BINARY}.exe"
      ;;
  esac
}

main() {
  need_cmd uname
  local os arch version install_dir asset url work
  os="$(detect_os)"
  arch="$(detect_arch)"
  version="${GBR_VERSION:-$(latest_tag)}"
  install_dir="${GBR_INSTALL_DIR:-$(default_install_dir "$os")}"
  asset="$(asset_name "$os" "$arch")"

  if [[ "$version" == "latest" ]]; then
    # CDN layout may use /latest/ or a resolved tag; prefer tag when known.
    url="${DOWNLOAD_BASE}/latest/${asset}"
  else
    url="${DOWNLOAD_BASE}/${version}/${asset}"
  fi

  info "${PRODUCT} — installing ${BINARY}"
  ok "os=${os} arch=${arch} version=${version}"
  ok "source=${url}"
  ok "dest=${install_dir}/${BINARY}$( [[ "$os" == "windows" ]] && echo ".exe" || true )"

  work="$(mktemp -d 2>/dev/null || mktemp -d -t gbr-install)"
  trap 'rm -rf "$work"' EXIT

  local dl="${work}/${asset}"
  info "Downloading…"
  if ! download "$url" "$dl"; then
    die "download failed: ${url}
Hint: set GBR_VERSION to a published tag (vX.Y.Z) or GBR_DOWNLOAD_BASE to your free-binary host.
Free binaries: product website + Microsoft Store (planned). Source repo is private company IP."
  fi

  # Basic sanity: non-empty file
  [[ -s "$dl" ]] || die "downloaded file is empty"

  local dest_name="$BINARY"
  [[ "$os" == "windows" ]] && dest_name="${BINARY}.exe"

  info "Installing to ${install_dir}"
  install_binary "$dl" "$install_dir" "$dest_name"

  print_path_hint "$install_dir" "$os"
  print_service_hints "$os" "$install_dir"

  echo
  info "Done. Verify:"
  if command -v "${install_dir}/${dest_name}" >/dev/null 2>&1 \
    || [[ -x "${install_dir}/${dest_name}" ]]; then
    if "${install_dir}/${dest_name}" version 2>/dev/null \
      || "${install_dir}/${dest_name}" --version 2>/dev/null \
      || "${install_dir}/${dest_name}" -version 2>/dev/null; then
      :
    else
      ok "${install_dir}/${dest_name}  (binary present; version flag may not be implemented yet)"
    fi
  else
    ok "${install_dir}/${dest_name}"
  fi
  ok "Configure Grok/xAI key in ~/.grok/config.json (or %USERPROFILE%\\.grok\\config.json)."
  ok "Protocol: gbr/1 — see protocol/v1.md in the product docs."
}

main "$@"
