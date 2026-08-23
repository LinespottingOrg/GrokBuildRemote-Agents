---
name: gbr
description: >
  Drive local Grok Build / CLI sessions through Build Remote Agent (gbr-agent).
  Pair the phone app, then attach via Bot API 127.0.0.1:8788 or gbr-mcp.
  Use when the user wants OpenClaw / Hermes / a host agent to inject into Grok Build.
compatibility: Requires gbr-agent run on the host. Loopback only. No mailbox keys in this file.
metadata:
  version: "0.6.1"
---

# Build Remote Agent — OpenClaw / claw attach

One adapter. Not 50 one-off integrations.

## Pair (unchanged — no fourth protocol)

1. Phone: Build Remote Agent app → Connect.
2. PC: `gbr-agent pair` — browser shows QR **and** prints the 8-char code.
3. Phone scans QR **or** types the 8-char code.
4. PC: `gbr-agent run` (or LaunchAgent). Keep it running.

Unpair on the phone before a new mailbox. Force-close is not enough.

## Attach surface (only these)

| How | Where |
|-----|--------|
| Bot API | `http://127.0.0.1:8788` after `gbr-agent run` |
| MCP | `gbr-mcp` stdio (same JSON as Bot API) |

Remote bots use the relay Bot URL + `X-GBR-Key` copied from phone **Settings → Bot API**. Never commit that key.

Loopback `:8788` is unauthenticated unless `GBR_BOT_REQUIRE_KEY=1`. MCP skill (pin `--branch v0.6.0`): [skills/gbr-mcp/SKILL.md](../gbr-mcp/SKILL.md). Index: https://grokbuildremote.com/integrations.html

```bash
curl -sS http://127.0.0.1:8788/health
curl -sS http://127.0.0.1:8788/v1/sessions
curl -sS -X POST http://127.0.0.1:8788/v1/inject \
  -H 'Content-Type: application/json' \
  -d '{"session_id":"SESSION","text":"hello","submit":true}'
```

## Hermes

Hermes does not need to be installed on the GBR host. On a Hermes box:

```bash
git clone --branch v0.6.0 --depth 1 https://github.com/LinespottingOrg/GrokBuildRemote-Agents.git
cd GrokBuildRemote-Agents/mcp/gbr-mcp && npm install
hermes mcp add gbr -- stdio -- node ./bin/gbr-mcp.js
# If gbr-agent run is already on that same host:
# hermes mcp add gbr -- http://127.0.0.1:8788
```

## NemoClaw

NemoClaw sandboxes **its** agent. GBR is the **host tool** (gbr-agent on the Mac/PC), not the sandbox. Point NemoClaw at loopback `:8788` / `gbr-mcp` on the host. Do not copy gbr-agent into the sandbox and do not invent a second pair protocol.

## Inbox watch (kills email paste)

With `gbr-agent run` and `gh` on PATH, the agent polls `LinespottingOrg/grok-build-inbox` label `boss-steer`. If a live Grok Build window title equals the issue title, it injects the newest non-report comment and submits. If no window, it opens `grok`, `/rename` to the title, and injects the issue body.

Disable: `GBR_INBOX_WATCH=0`.
