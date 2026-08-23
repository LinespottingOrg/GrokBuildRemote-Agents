---
name: gbr
description: >
  Pair a phone running Build Remote Agent to Hermes. Requires gbr-agent run on
  the host. Attach via hermes mcp add gbr stdio gbr-mcp, or Bot API 127.0.0.1:8788
  only when gbr-agent run is on this same host.
compatibility: Requires gbr-agent ≥ 0.6.0 on the host. Loopback only. No mailbox keys in this file.
metadata:
  version: "0.6.1"
  product: "Build Remote Agent"
  website: "https://grokbuildremote.com/integrations.html"
---

# Build Remote Agent — Hermes attach

Same attach surface as OpenClaw: Bot API `:8788` **or** `gbr-mcp` stdio. Protocol `gbr/1`. No fourth pair protocol.

Listing skill (ClawHub / family): [skills/openclaw/SKILL.md](../openclaw/SKILL.md).
Index: https://grokbuildremote.com/integrations.html · pin: [docs/PINNED-INSTALL.md](../../docs/PINNED-INSTALL.md).

Independent product by Linespotting AB. Not affiliated with xAI or SpaceX. Not an official Hermes core feature.

## Pair (host)

1. Phone: [Build Remote Agent](https://grokbuildremote.com/) → Connect.
2. PC: `gbr-agent pair` — QR **and** printed 8-char code.
3. Phone scans or types the code.
4. PC: `gbr-agent run` (keep it running).

```bash
gbr-agent version    # need v0.6.0+
gbr-agent pair && gbr-agent run
```

Hermes does **not** need to live on the GBR host. Chat channels are not `gbr/1`.

## MCP — pin the clone, then add

Do not `git clone` default branch.

```bash
git clone --branch v0.6.0 --depth 1 https://github.com/LinespottingOrg/GrokBuildRemote-Agents.git
cd GrokBuildRemote-Agents/mcp/gbr-mcp && npm install
hermes mcp add gbr stdio gbr-mcp
```

**HTTP `:8788` only if `gbr-agent run` is on this same host:**

```bash
# same host only — do not point Hermes at another machine's loopback
hermes mcp add gbr --url http://127.0.0.1:8788
```

Loopback is unauthenticated unless `GBR_BOT_REQUIRE_KEY=1`. Never commit mailbox keys.

```bash
export GBR_BOT_REQUIRE_KEY=1
curl -sS http://127.0.0.1:8788/health
```

Phone is spectator + veto on **host terminal windows**, not the Hermes chat UI.
