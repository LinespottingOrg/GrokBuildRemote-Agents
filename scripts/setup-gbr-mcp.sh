#!/usr/bin/env bash
# Pin gbr-mcp from GitHub tag v0.6.2 and print Hermes / OpenClaw / NemoClaw attach commands.
# Default target is Grok Build CLI (`grok`) via gbr_open — not a SideCar, not Bot API as MCP.
set -euo pipefail
VER="${GBR_VERSION:-v0.6.2}"
DEST="${GBR_MCP_SRC:-$HOME/.gbr/gbr-mcp-src}"
REPO="https://github.com/LinespottingOrg/GrokBuildRemote-Agents.git"

die() { echo "error: $*" >&2; exit 1; }
ok() { echo "    $*"; }
info() { echo "==> $*"; }

command -v node >/dev/null || die "need Node ≥20"
command -v git >/dev/null || die "need git"
command -v gbr-agent >/dev/null || die "gbr-agent not on PATH — pin install: https://grokbuildremote.com/PINNED-INSTALL.md"
command -v grok >/dev/null || echo "warn: grok CLI not on PATH — gbr_open will fail until Grok Build is installed" >&2

gv="$(gbr-agent version 2>/dev/null || true)"
ok "gbr-agent: ${gv:-unknown}"

info "pin ${REPO} @ ${VER} → ${DEST}"
mkdir -p "$(dirname "$DEST")"
if [[ -d "$DEST/.git" ]]; then
  git -C "$DEST" fetch --tags --depth 1 origin "$VER"
  git -C "$DEST" checkout -q FETCH_HEAD
else
  rm -rf "$DEST"
  git clone --branch "$VER" --depth 1 "$REPO" "$DEST"
fi

cd "$DEST/mcp/gbr-mcp"
npm install --omit=dev
chmod +x bin/gbr-mcp.js
MCP="$DEST/mcp/gbr-mcp/bin/gbr-mcp.js"
[[ -f "$MCP" ]] || die "missing $MCP"
node "$MCP" --diagnose || true

info "Hermes (stdio MCP — do NOT point Hermes at :8788; that is Bot API REST, not MCP)"
echo "  hermes mcp add gbr -- stdio -- node $MCP"

info "OpenClaw"
echo "  copy $DEST/skills/openclaw/SKILL.md into the OpenClaw skills dir"
echo "  or: clawhub skill publish (human login) — slug gbr"
echo "  MCP stdio: node $MCP"

info "NemoClaw (host tool, not inside OpenShell)"
echo "  Keep gbr-agent + this node process on the HOST."
echo "  Point the sandboxed agent at host loopback via stdio MCP on the host, or HTTP Bot API"
echo "  http://127.0.0.1:8788 only after: export GBR_BOT_REQUIRE_KEY=1"
echo "  Do not copy gbr-agent into the sandbox."

info "Then on this host"
echo "  gbr-agent pair && gbr-agent run     # keep running"
echo "  # MCP client: gbr_diagnose → gbr_open (spawns grok) → gbr_inject → gbr_result"
echo "  # Phone spectates that grok TTY after pair."
echo "Docs: https://grokbuildremote.com/use-cases/claw.html"
echo "MCP:  $MCP"
