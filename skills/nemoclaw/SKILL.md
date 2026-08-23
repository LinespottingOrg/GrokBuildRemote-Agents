---
name: gbr
description: >
  Pair a phone running Build Remote Agent to the NemoClaw HOST (not the OpenShell
  sandbox). Attach via host loopback 127.0.0.1:8788 or gbr-mcp after gbr-agent run.
compatibility: Requires gbr-agent ≥ 0.6.0 on the host. Loopback only. No mailbox keys in this file. Never install gbr-agent inside OpenShell.
metadata:
  version: "0.6.1"
  product: "Build Remote Agent"
  website: "https://grokbuildremote.com/integrations.html"
---

# Build Remote Agent — NemoClaw host attach

Same attach surface as OpenClaw: Bot API `:8788` **or** `gbr-mcp` stdio. Protocol `gbr/1`. No fourth pair protocol.

Listing skill (ClawHub / family): [skills/openclaw/SKILL.md](../openclaw/SKILL.md).
Index: https://grokbuildremote.com/integrations.html · pin: [docs/PINNED-INSTALL.md](../../docs/PINNED-INSTALL.md).

Independent product by Linespotting AB. Not affiliated with xAI, SpaceX, or NVIDIA. Community recipe dest — not a NemoClaw **core** feature.

## Host vs sandbox

| Piece | Where |
|-------|--------|
| NemoClaw / OpenShell | Sandbox for **its** agent |
| `gbr-agent` | **Host** Mac/PC only — never inside OpenShell |
| Attach | Host loopback `http://127.0.0.1:8788` or host `gbr-mcp` |

Do not copy gbr-agent into the sandbox image. Do not invent a second pair protocol.

## Pair (host)

1. Phone: [Build Remote Agent](https://grokbuildremote.com/) → Connect.
2. Host: `gbr-agent pair` — QR **and** printed 8-char code.
3. Phone scans or types the code.
4. Host: `gbr-agent run` (keep it running).

```bash
gbr-agent version    # need v0.6.0+
gbr-agent pair && gbr-agent run
```

## Attach (host loopback / MCP)

Do not `git clone` default branch.

```bash
git clone --branch v0.6.0 --depth 1 https://github.com/LinespottingOrg/GrokBuildRemote-Agents.git
cd GrokBuildRemote-Agents/mcp/gbr-mcp && npm install
node bin/gbr-mcp.js --diagnose
```

Point NemoClaw at the **host** Bot API / stdio — not at a sandbox listener.

```bash
export GBR_BOT_REQUIRE_KEY=1   # optional; require key even on loopback
curl -sS http://127.0.0.1:8788/health
curl -sS http://127.0.0.1:8788/v1/sessions
```

Loopback `:8788` is unauthenticated unless `GBR_BOT_REQUIRE_KEY=1`. Relay always needs `X-GBR-Key`. Never commit mailbox keys.

Phone is spectator + veto on **host terminal windows**. Sandboxed NemoClaw sessions are not a TTY on that roster unless they run in a discovered host terminal.
