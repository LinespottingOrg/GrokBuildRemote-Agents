---
name: gbr
description: >
  Drive local Grok Build / CLI sessions through Build Remote Agent (gbr-agent).
  Pair the phone app, then attach via Bot API 127.0.0.1:8788 or gbr-mcp.
  Use when the user wants OpenClaw / Hermes / a host agent to inject into Grok Build.
compatibility: Requires gbr-agent ≥ 0.6.0 on the host. Loopback only. No mailbox keys in this file.
metadata:
  version: "0.6.1"
  product: "Build Remote Agent"
  website: "https://grokbuildremote.com/integrations.html"
---

# Build Remote Agent — OpenClaw / claw attach

One adapter. Protocol `gbr/1`. Not 50 one-off integrations.

**This file is the one ClawHub / OpenClaw SKILL listing.** Hermes and NemoClaw use the same attach surface. Family notes: [skills/hermes/SKILL.md](../hermes/SKILL.md) · [skills/nemoclaw/SKILL.md](../nemoclaw/SKILL.md).

Independent product by Linespotting AB. Not affiliated with xAI or SpaceX.

Index: https://grokbuildremote.com/integrations.html · pin: [docs/PINNED-INSTALL.md](../../docs/PINNED-INSTALL.md)

## Pair (unchanged — no fourth protocol)

1. Phone: [Build Remote Agent](https://grokbuildremote.com/) → Connect.
2. PC: `gbr-agent pair` — browser shows QR **and** prints the 8-char code.
3. Phone scans QR **or** types the 8-char code.
4. PC: `gbr-agent run` (or LaunchAgent). Keep it running.

```bash
gbr-agent version    # need v0.6.0+
gbr-agent pair && gbr-agent run
```

Unpair on the phone before a new mailbox. Force-close is not enough.

**OpenClaw phone nodes stay on OpenClaw pair.** Do not hook `extensions/device-pair` / iOS / Android nodes. The Build Remote Agent app is a **desktop TTY spectator** on this host’s terminal windows — not an OpenClaw node.

## Attach surface (only these)

| How | Where |
|-----|--------|
| Bot API | `http://127.0.0.1:8788` after `gbr-agent run` |
| MCP | `gbr-mcp` stdio (same JSON as Bot API) |

Loopback `:8788` is unauthenticated unless `GBR_BOT_REQUIRE_KEY=1`. Relay always needs `X-GBR-Key`. **Never** put mailbox keys in this skill, git, or ClawHub publish.

```bash
export GBR_BOT_REQUIRE_KEY=1   # optional; require key even on loopback
curl -sS http://127.0.0.1:8788/health
curl -sS http://127.0.0.1:8788/v1/sessions
curl -sS -X POST http://127.0.0.1:8788/v1/inject \
  -H 'Content-Type: application/json' \
  -d '{"session_id":"SESSION","text":"hello","submit":true}'
```

Remote bots: phone **Settings → Bot API** copies the relay URL + mailbox id + key. Never commit that key.

## MCP (`gbr-mcp`) — pin the clone

Do not `git clone` default branch.

```bash
git clone --branch v0.6.0 --depth 1 https://github.com/LinespottingOrg/GrokBuildRemote-Agents.git
cd GrokBuildRemote-Agents/mcp/gbr-mcp && npm install
node bin/gbr-mcp.js --diagnose
```

## Hermes

Hermes does not need to be installed on the GBR host. On a Hermes box:

```bash
git clone --branch v0.6.0 --depth 1 https://github.com/LinespottingOrg/GrokBuildRemote-Agents.git
cd GrokBuildRemote-Agents/mcp/gbr-mcp && npm install
hermes mcp add gbr stdio gbr-mcp
# HTTP :8788 only if gbr-agent run is on THIS same host:
# hermes mcp add gbr --url http://127.0.0.1:8788
```

Chat channels are not `gbr/1`. Detail: [skills/hermes/SKILL.md](../hermes/SKILL.md).

## NemoClaw

NemoClaw sandboxes **its** agent (OpenShell). GBR is the **host** tool — `gbr-agent` stays on the Mac/PC, **not** inside the sandbox. Attach host loopback `:8788` / `gbr-mcp`. Do not copy gbr-agent into OpenShell and do not invent a second pair protocol.

Detail: [skills/nemoclaw/SKILL.md](../nemoclaw/SKILL.md). Community recipe only — not a NemoClaw core PR.

## Inbox watch (kills email paste)

With `gbr-agent run` and `gh` on PATH, the agent polls `LinespottingOrg/grok-build-inbox` label `boss-steer`. If a live Grok Build window title equals the issue title, it injects the newest non-report comment and submits. If no window, it opens `grok`, `/rename` to the title, and injects the issue body.

Disable: `GBR_INBOX_WATCH=0`.
