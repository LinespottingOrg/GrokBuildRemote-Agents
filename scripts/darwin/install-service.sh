#!/usr/bin/env bash
# Grok Build Remote — non-interactive macOS LaunchAgent (halt default).
# Parallel to scripts/windows/install-service.ps1 (PR #54).
# Does NOT start unless --start. Does NOT clear GBR_INJECT_HALT.
# No secrets. Never com.linespotting.* as a bundle id for apps (LaunchAgent
# label com.linespotting.grok-build-remote is the existing Darwin bundle id).
set -euo pipefail

START=0
ALLOW_INJECT=0
SKIP_DISABLE_LEGACY=0
BINARY="${GBR_AGENT_BIN:-$HOME/.local/bin/gbr-agent}"
LOG_DIR="${GBR_LOG_DIR:-$HOME/pc-build/gbr-agent-out}"

NI_LABEL="com.linespotting.grok-build-remote"
LEGACY_LABEL="com.linespotting.gbr-agent"
PRODUCT="Grok Build Remote"

usage() {
  cat <<EOF
Install Grok Build Remote as a non-interactive LaunchAgent (Background).

  $0 [--start] [--allow-inject] [--skip-disable-legacy] [--binary PATH] [--log-dir PATH]

Defaults: no start, GBR_INJECT_HALT=1, GBR_NO_AUTO_OPEN=1, GBR_INBOX_WATCH=0.
Binary must be halt-capable (help lists -inject-halt). Refuse commit 6f451ac and .aiprojects/.../dist.
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --start) START=1; shift ;;
    --allow-inject) ALLOW_INJECT=1; shift ;;
    --skip-disable-legacy) SKIP_DISABLE_LEGACY=1; shift ;;
    --binary) BINARY="${2:?}"; shift 2 ;;
    --log-dir) LOG_DIR="${2:?}"; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) echo "unknown arg: $1" >&2; usage; exit 2 ;;
  esac
done

die() { echo "error: $*" >&2; exit 1; }

[[ -n "${HOME:-}" ]] || die "HOME unset"
UID_NUM="$(id -u)"
DOMAIN="gui/${UID_NUM}"
PLIST="${HOME}/Library/LaunchAgents/${NI_LABEL}.plist"
LEGACY_PLIST="${HOME}/Library/LaunchAgents/${LEGACY_LABEL}.plist"
APP_DST="${HOME}/Applications/${PRODUCT}.app"

echo "${PRODUCT} — non-interactive macOS LaunchAgent"

# --- refuse bad binaries ---
[[ -x "$BINARY" ]] || die "gbr-agent not executable at $BINARY (copy a halt-capable build to ~/.local/bin/gbr-agent)"
case "$BINARY" in
  *".aiprojects"*"gbr/agents/dist"*) die "refusing .aiprojects/gbr/agents/dist binary: $BINARY" ;;
esac

VER_OUT="$("$BINARY" version 2>&1 || true)"
echo "version: $VER_OUT"
echo "$VER_OUT" | grep -q 'commit=6f451ac' && die "refusing commit 6f451ac (wmic-flash 0.6.3 only — no PR #40 halt). Rebuild from origin/main after #40/#55/#61."

HELP_OUT="$("$BINARY" -h 2>&1 || true)"
echo "$VER_OUT$HELP_OUT" | grep -qi 'disabled after desktop popup' && die "refusing disable-stub at $BINARY"
echo "$HELP_OUT" | grep -q 'inject-halt' || die "binary does not advertise -inject-halt (PR #40 missing). Refusing."
echo "$HELP_OUT" | grep -q 'GBR_INJECT_HALT' || die "binary does not advertise GBR_INJECT_HALT. Refusing."

mkdir -p "$LOG_DIR" "${HOME}/Library/LaunchAgents" "${HOME}/Applications"
probe="${LOG_DIR}/.gbr-write-probe"
date -u +%Y-%m-%dT%H:%M:%SZ >"$probe"
rm -f "$probe"

# User env for future shells; plist also embeds the same.
if [[ "$ALLOW_INJECT" -eq 1 ]]; then
  echo "WARNING: --allow-inject — GBR_INJECT_HALT will NOT be applied. David live trial only." >&2
  launchctl setenv GBR_INJECT_HALT "" 2>/dev/null || true
else
  export GBR_INJECT_HALT=1
  launchctl setenv GBR_INJECT_HALT 1 2>/dev/null || true
fi
export GBR_NO_AUTO_OPEN=1
export GBR_INBOX_WATCH=0
export GBR_LOG_DIR="$LOG_DIR"
launchctl setenv GBR_NO_AUTO_OPEN 1 2>/dev/null || true
launchctl setenv GBR_INBOX_WATCH 0 2>/dev/null || true
launchctl setenv GBR_LOG_DIR "$LOG_DIR" 2>/dev/null || true

ARGS_HALT=""
if [[ "$ALLOW_INJECT" -eq 0 ]]; then
  ARGS_HALT="      <string>-inject-halt</string>"
fi

# ProcessType Background = no UI (not Interactive).
cat >"$PLIST" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>${NI_LABEL}</string>
  <key>AssociatedBundleIdentifiers</key>
  <array>
    <string>${NI_LABEL}</string>
  </array>
  <key>ProgramArguments</key>
  <array>
    <string>${BINARY}</string>
    <string>-log=info</string>
    <string>run</string>
${ARGS_HALT}
  </array>
  <key>RunAtLoad</key>
  <true/>
  <key>KeepAlive</key>
  <true/>
  <key>ProcessType</key>
  <string>Background</string>
  <key>WorkingDirectory</key>
  <string>${HOME}</string>
  <key>StandardOutPath</key>
  <string>${LOG_DIR}/gbr-agent.out.log</string>
  <key>StandardErrorPath</key>
  <string>${LOG_DIR}/gbr-agent.err.log</string>
  <key>EnvironmentVariables</key>
  <dict>
    <key>PATH</key>
    <string>${HOME}/.local/bin:${HOME}/bin:/usr/local/bin:/opt/homebrew/bin:/usr/bin:/bin</string>
    <key>HOME</key>
    <string>${HOME}</string>
    <key>GBR_LOG_DIR</key>
    <string>${LOG_DIR}</string>
    <key>GBR_NO_AUTO_OPEN</key>
    <string>1</string>
    <key>GBR_INBOX_WATCH</key>
    <string>0</string>
EOF
if [[ "$ALLOW_INJECT" -eq 0 ]]; then
  cat >>"$PLIST" <<EOF
    <key>GBR_INJECT_HALT</key>
    <string>1</string>
EOF
fi
cat >>"$PLIST" <<EOF
  </dict>
</dict>
</plist>
EOF

# Login Items display name: copy app bundle next to this script's repo tree if present.
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
SRC_APP="${REPO_ROOT}/install/darwin/${PRODUCT}.app"
if [[ -d "$SRC_APP" ]]; then
  rm -rf "$APP_DST"
  cp -R "$SRC_APP" "$APP_DST"
  echo "Installed ${APP_DST} (Login Items show ${PRODUCT})"
else
  echo "warn: missing $SRC_APP — skip app bundle copy" >&2
fi

# Ad-hoc sign / drop quarantine on the CLI binary (launchd codesign).
xattr -cr "$BINARY" 2>/dev/null || true
codesign --force --sign - "$BINARY" 2>/dev/null || true

disable_legacy() {
  if [[ ! -f "$LEGACY_PLIST" ]]; then
    echo "Legacy LaunchAgent ${LEGACY_LABEL} not present — nothing to disable."
    return
  fi
  launchctl bootout "${DOMAIN}/${LEGACY_LABEL}" 2>/dev/null || true
  launchctl unload "$LEGACY_PLIST" 2>/dev/null || true
  echo "Disabled legacy LaunchAgent ${LEGACY_LABEL} (plist left on disk; do not delete without David yes)."
}

if [[ "$SKIP_DISABLE_LEGACY" -eq 0 ]]; then
  disable_legacy
fi

if [[ "$START" -eq 1 ]]; then
  launchctl bootout "${DOMAIN}/${NI_LABEL}" 2>/dev/null || true
  launchctl bootstrap "$DOMAIN" "$PLIST"
  launchctl enable "${DOMAIN}/${NI_LABEL}"
  launchctl kickstart -k "${DOMAIN}/${NI_LABEL}" || launchctl start "$NI_LABEL" || true
  echo "Started ${NI_LABEL} (Background, halt default)."
else
  echo "Plist written: $PLIST"
  echo "NOT started (pass --start). Next login will RunAtLoad unless you bootstrap later."
fi

echo "Done."
if [[ "$ALLOW_INJECT" -eq 0 ]]; then echo "  Halt default:  GBR_INJECT_HALT=1"; else echo "  Halt default:  off (--allow-inject)"; fi
echo "  No auto-open:  GBR_NO_AUTO_OPEN=1"
echo "  Logs:          $LOG_DIR"
echo "  Human name:    ${PRODUCT}"
echo "  Label:         ${NI_LABEL}"
echo "  Uninstall:     scripts/darwin/uninstall-service.sh"
echo "  Do NOT delete ${LEGACY_PLIST} without David yes."
