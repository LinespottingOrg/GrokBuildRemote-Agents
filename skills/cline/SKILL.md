---
name: cline
description: >
  Pair Build Remote Agent to Cline. If Cline runs in a TTY the phone sees that
  window; otherwise add gbr-mcp on the desktop Cline. Pin gbr-agent v0.6.0.
compatibility: Requires gbr-agent ≥ 0.6.0. Loopback only. No mailbox keys. Unaffiliated with cline/cline.
metadata:
  version: "0.6.0"
  product: "Build Remote Agent"
  website: "https://grokbuildremote.com/integrations.html"
---

# Cline + Build Remote Agent

Unaffiliated. This skill lives **here**, not in `cline/cline`.
Index: https://grokbuildremote.com/integrations.html

| You run | Phone sees |
|---------|------------|
| `cline` **in a terminal** (iTerm / Windows Terminal / gnome-terminal) | That window. Inject types into it. |
| Cline VS Code / JetBrains panel (not a TTY) | Nothing useful. Add **gbr-mcp** on the **desktop** Cline. Phone still only lists TTYs. |

Cline Kanban / Tailscale remote-access is not `gbr/1`. Do not wrap it.

## Pair (pin v0.6.0)

Checksum the installer, then the binary — [docs/PINNED-INSTALL.md](../../docs/PINNED-INSTALL.md). Then:

```bash
gbr-agent version    # v0.6.0+
gbr-agent pair && gbr-agent run
```

Loopback `:8788` is unauthenticated unless `export GBR_BOT_REQUIRE_KEY=1`.

## Desktop MCP (when Cline is not a TTY)

```bash
git clone --branch v0.6.0 --depth 1 https://github.com/LinespottingOrg/GrokBuildRemote-Agents.git
cd GrokBuildRemote-Agents/mcp/gbr-mcp && npm install
```

CLI (writes `~/.cline/data/settings/cline_mcp_settings.json`):

```bash
cline mcp add gbr -- node /ABSOLUTE/PATH/GrokBuildRemote-Agents/mcp/gbr-mcp/bin/gbr-mcp.js
```

IDE: Cline panel → **MCP Servers** → **Configure MCP Servers**:

```json
{
  "mcpServers": {
    "gbr": {
      "command": "node",
      "args": ["/ABSOLUTE/PATH/GrokBuildRemote-Agents/mcp/gbr-mcp/bin/gbr-mcp.js"],
      "disabled": false
    }
  }
}
```

Desktop Cline can call Bot API. The phone still lists **terminal windows** on this PC.

## Attach (only these)

| How | Where |
|-----|--------|
| Bot API | `http://127.0.0.1:8788` after `gbr-agent run` |
| MCP | `gbr-mcp` stdio (above) |

Phone is spectator + veto. Never commit `mailbox_key`.
