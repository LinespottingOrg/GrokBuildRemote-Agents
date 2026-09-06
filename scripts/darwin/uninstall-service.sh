#!/usr/bin/env bash
# Uninstall Grok Build Remote NI LaunchAgent. Does not delete the CLI binary.
# --delete-legacy removes com.linespotting.gbr-agent.plist (David yes only).
set -euo pipefail

DELETE_LEGACY=0
CLEAR_HALT=0
while [[ $# -gt 0 ]]; do
  case "$1" in
    --delete-legacy) DELETE_LEGACY=1; shift ;;
    --clear-halt) CLEAR_HALT=1; shift ;;
    *) echo "unknown arg: $1" >&2; exit 2 ;;
  esac
done

NI_LABEL="com.linespotting.grok-build-remote"
LEGACY_LABEL="com.linespotting.gbr-agent"
PRODUCT="Grok Build Remote"
UID_NUM="$(id -u)"
DOMAIN="gui/${UID_NUM}"
PLIST="${HOME}/Library/LaunchAgents/${NI_LABEL}.plist"
LEGACY_PLIST="${HOME}/Library/LaunchAgents/${LEGACY_LABEL}.plist"
APP_DST="${HOME}/Applications/${PRODUCT}.app"

echo "${PRODUCT} — non-interactive macOS uninstall"

launchctl bootout "${DOMAIN}/${NI_LABEL}" 2>/dev/null || true
launchctl unload "$PLIST" 2>/dev/null || true
rm -f "$PLIST"
echo "Removed ${NI_LABEL} plist."

if [[ "$DELETE_LEGACY" -eq 1 ]]; then
  launchctl bootout "${DOMAIN}/${LEGACY_LABEL}" 2>/dev/null || true
  launchctl unload "$LEGACY_PLIST" 2>/dev/null || true
  rm -f "$LEGACY_PLIST"
  echo "Deleted legacy ${LEGACY_LABEL} (--delete-legacy)."
else
  echo "Left legacy ${LEGACY_LABEL} on disk (pass --delete-legacy only with David yes)."
fi

rm -rf "$APP_DST"

if [[ "$CLEAR_HALT" -eq 1 ]]; then
  launchctl unsetenv GBR_INJECT_HALT 2>/dev/null || true
  echo "Cleared launchctl GBR_INJECT_HALT."
fi

echo "Done. ~/.local/bin/gbr-agent was not removed."
