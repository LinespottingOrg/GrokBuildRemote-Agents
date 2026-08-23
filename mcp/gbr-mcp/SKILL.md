---
name: gbr-mcp
description: >
  MCP stdio server for Build Remote Agent (gbr-agent Bot API).
  Pin git clone --branch v0.6.0, then npm install from that tag.
  Loopback :8788 vs HTTPS relay X-GBR-Key. Harden loopback with GBR_BOT_REQUIRE_KEY=1.
  Use when attaching Claude Code, Grok CLI, Cursor, OpenCode, or any MCP host to desktop terminals.
compatibility: Requires Node ≥ 20 and gbr-agent ≥ 0.6.0. No mailbox keys in this file. Do not clone the default branch.
metadata:
  version: "0.6.1"
  product: "Build Remote Agent"
  website: "https://grokbuildremote.com/"
  integrations: "https://grokbuildremote.com/integrations.html"
---

# gbr-mcp — MCP attach

Protocol `gbr/1`. Independent Linespotting AB product. Not affiliated with xAI or SpaceX.

| | |
|--|--|
| Index | https://grokbuildremote.com/integrations.html |
| Pin | [docs/PINNED-INSTALL.md](../../docs/PINNED-INSTALL.md) |
| Pair | `skills/gbr/SKILL.md` |
| Server | `mcp/gbr-mcp` (this tree, tag **v0.6.0**) |

Phone is spectator + veto. Roster = **terminal windows** on the paired PC, not this MCP client.

## Do not (unpinned clone)

```bash
git clone https://github.com/LinespottingOrg/GrokBuildRemote-Agents.git
cd GrokBuildRemote-Agents/mcp/gbr-mcp && npm install
```

That clones the **default branch**. It is not a trust root. Do not `npm install` from it.

## Pin `--branch v0.6.0`, then npm from that tag

```bash
git clone --branch v0.6.0 --depth 1 https://github.com/LinespottingOrg/GrokBuildRemote-Agents.git
cd GrokBuildRemote-Agents/mcp/gbr-mcp
npm install
node bin/gbr-mcp.js --diagnose
```

Agent binary: checksum the installer, then the binary — same [PINNED-INSTALL](../../docs/PINNED-INSTALL.md). Need `gbr-agent version` → `v0.6.0+`.

```bash
gbr-agent pair && gbr-agent run
```

## Loopback vs relay (`X-GBR-Key`)

| Surface | URL | Auth |
|---------|-----|------|
| Loopback (same PC) | `http://127.0.0.1:8788` after `gbr-agent run` | Unauthenticated **unless** `GBR_BOT_REQUIRE_KEY=1`. Then send the mailbox key as `X-GBR-Key`. |
| HTTPS relay | `https://gbr-relay.ekobrott.workers.dev/v1/mb/{id}/bot` | **Always** `X-GBR-Key` or `Authorization: Bearer`. Copy from phone **Settings → Bot API**. |
| MCP stdio | `gbr-mcp` on this machine | Local process. Default talks to loopback `:8788`. Relay mode uses the env below. |

Harden the agent (loopback):

```bash
export GBR_BOT_REQUIRE_KEY=1
gbr-agent run
```

Relay mode for gbr-mcp (env only — never commit values):

```text
GBR_RELAY_URL=https://gbr-relay.ekobrott.workers.dev
GBR_MAILBOX_ID=gbr-xxxx
GBR_MAILBOX_KEY=          # phone Settings → Bot API; never in git
```

When those three are set, gbr-mcp sends `X-GBR-Key` on every request. Never paste the key into this file, an issue, or a screenshot.

## MCP host config

The `args` path must be the **v0.6.0** tree from the pin step.

```json
{
  "mcpServers": {
    "gbr": {
      "command": "node",
      "args": ["/ABSOLUTE/PATH/GrokBuildRemote-Agents/mcp/gbr-mcp/bin/gbr-mcp.js"],
      "env": { "GBR_MCP_LOG_LEVEL": "info" }
    }
  }
}
```

## Pair (unchanged)

1. Phone: [Build Remote Agent](https://grokbuildremote.com/) → Connect.
2. PC: `gbr-agent pair` — QR **and** printed 8-char code.
3. Phone scans or types the code.
4. PC: `gbr-agent run`.

Unpair on the phone before a new mailbox. Force-close is not enough.

## Loop

diagnose → open/attach → lock → inject → wait idle → harvest excerpt → iterate or close

Docs: [INSTALL.md](INSTALL.md) · `docs/BOT-API.md` · https://grokbuildremote.com/integrations.html
