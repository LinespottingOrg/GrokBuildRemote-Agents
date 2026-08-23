---
name: goose
description: >
  Pair Build Remote Agent to Goose. If Goose runs in a TTY the phone sees that
  window; otherwise add gbr-mcp on the desktop Goose. Pin gbr-agent v0.6.0.
compatibility: Requires gbr-agent ≥ 0.6.0. Loopback only. No mailbox keys. Unaffiliated with block/goose.
metadata:
  version: "0.6.0"
  product: "Build Remote Agent"
  website: "https://grokbuildremote.com/integrations.html"
---

# Goose + Build Remote Agent

Unaffiliated. This skill lives **here**, not in `block/goose` / `aaif-goose/goose`.
Index: https://grokbuildremote.com/integrations.html

| You run | Phone sees |
|---------|------------|
| `goose` **in a terminal** (iTerm / Windows Terminal / gnome-terminal) | That window. Inject types into it. |
| Goose Desktop, `goose serve`, HTTP / Telegram gateway | Nothing useful. Add **gbr-mcp** on the **desktop** Goose. Phone still only lists TTYs. |

`goose-ios` is Goose’s own remote. Do not merge that tunnel with `gbr/1`.

## Pair (pin v0.6.0)

Checksum the installer, then the binary — [docs/PINNED-INSTALL.md](../../docs/PINNED-INSTALL.md). Then:

```bash
gbr-agent version    # v0.6.0+
gbr-agent pair && gbr-agent run
```

Loopback `:8788` is unauthenticated unless `export GBR_BOT_REQUIRE_KEY=1`.

## Desktop MCP (when Goose is not a TTY)

```bash
git clone --branch v0.6.0 --depth 1 https://github.com/LinespottingOrg/GrokBuildRemote-Agents.git
cd GrokBuildRemote-Agents/mcp/gbr-mcp && npm install
```

`goose configure` → Add Extension → Command-line Extension, or `~/.config/goose/config.yaml`:

```yaml
extensions:
  gbr:
    enabled: true
    type: stdio
    name: gbr
    cmd: node
    args: ["/ABSOLUTE/PATH/GrokBuildRemote-Agents/mcp/gbr-mcp/bin/gbr-mcp.js"]
    timeout: 300
```

Desktop Goose can call Bot API. The phone still lists **terminal windows** on this PC.

## Attach (only these)

| How | Where |
|-----|--------|
| Bot API | `http://127.0.0.1:8788` after `gbr-agent run` |
| MCP | `gbr-mcp` stdio (above) |

Phone is spectator + veto. Never commit `mailbox_key`.
